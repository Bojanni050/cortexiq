// Modifications copyright (c) 2026 Bojan van der Heide
package observability

// The counters in this package live in the process and start at zero after a
// restart, so a deploy looks like a drop in usage while it is only a new
// registry. The process that records them (the MCP/consolidation server) saves
// its totals to the database and adds them back before it starts serving, so
// both /metrics and the dashboard stay cumulative.
//
// The dashboard runs as its own process in the split deployment and never
// records these counters; only the process that answers tool calls restores and
// persists them, which keeps a single writer for each series. Both shipped
// topologies satisfy that (`serve` runs everything in one process, and
// `mcp serve --with-consolidation` is the whole brain); two processes that both
// persisted the same totals would count them twice.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// PersistInterval is how often the counters are written to the database. It is
// short enough that a crashed process loses little and long enough that a busy
// consolidation run does not turn into a write per metric update.
const PersistInterval = time.Minute

// persistTimeout bounds the final write on shutdown, so a broken database
// cannot hold the process open past the orchestrator's grace period.
const persistTimeout = 5 * time.Second

// CounterValue is one persisted counter series: its metric name, its label set
// and its cumulative value.
type CounterValue struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// CounterStore saves and loads counter totals. The database implementation is
// PGStore below; tests use an in-memory one.
type CounterStore interface {
	Load(ctx context.Context) ([]CounterValue, error)
	Save(ctx context.Context, values []CounterValue) error
}

// PGStore keeps the counter totals in the metric_counters table.
type PGStore struct {
	Pool *pgxpool.Pool
}

// Load reads every persisted series.
func (s PGStore) Load(ctx context.Context) ([]CounterValue, error) {
	rows, err := s.Pool.Query(ctx, `SELECT name, labels, value FROM metric_counters`)
	if err != nil {
		return nil, fmt.Errorf("query metric_counters: %w", err)
	}
	defer rows.Close()

	var out []CounterValue
	for rows.Next() {
		var (
			name, labels string
			value        float64
		)
		if err := rows.Scan(&name, &labels, &value); err != nil {
			return nil, fmt.Errorf("scan metric_counters: %w", err)
		}
		parsed, err := decodeLabels(labels)
		if err != nil {
			// A label row that cannot be parsed is not worth failing start-up
			// over: the series simply starts at zero again.
			log.Printf("metric_counters: skipping %q with invalid labels %q: %v", name, labels, err)
			continue
		}
		out = append(out, CounterValue{Name: name, Labels: parsed, Value: value})
	}
	return out, rows.Err()
}

// Save upserts the series. The update takes the greater value: counters never
// decrease, and a second writer (or a save racing a restore) must not lower a
// total that is already recorded.
func (s PGStore) Save(ctx context.Context, values []CounterValue) error {
	if len(values) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, v := range values {
		labels, err := encodeLabels(v.Labels)
		if err != nil {
			return fmt.Errorf("encode labels for %q: %w", v.Name, err)
		}
		batch.Queue(
			`INSERT INTO metric_counters (name, labels, value, updated_at)
			      VALUES ($1, $2, $3, now())
			 ON CONFLICT (name, labels) DO UPDATE
			         SET value = GREATEST(metric_counters.value, EXCLUDED.value),
			             updated_at = now()`,
			v.Name, labels, v.Value,
		)
	}
	return s.Pool.SendBatch(ctx, batch).Close()
}

// encodeLabels stores the label set as JSON. encoding/json orders map keys, so
// the same series always produces the same primary key.
func encodeLabels(labels map[string]string) (string, error) {
	if len(labels) == 0 {
		return "{}", nil
	}
	encoded, err := json.Marshal(labels)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// decodeLabels reads a label set written by encodeLabels.
func decodeLabels(raw string) (map[string]string, error) {
	labels := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// Restore adds the persisted totals back into this process's counters. It runs
// once at start-up, before the server serves a request, so no traffic is
// double counted.
func Restore(ctx context.Context, store CounterStore) error {
	values, err := store.Load(ctx)
	if err != nil {
		return err
	}
	for _, v := range values {
		vec := counterVec(v.Name)
		if vec == nil {
			continue
		}
		vec.With(prometheus.Labels(v.Labels)).Add(v.Value)
	}
	return nil
}

// Flush writes every counter series this process owns to the store. Gauges and
// the duration histogram are not persisted: the histogram cannot be seeded
// through the Prometheus API, and a duration average across restarts is less
// meaningful than the totals next to it.
func Flush(ctx context.Context, store CounterStore) error {
	values, err := snapshotCounters()
	if err != nil {
		return err
	}
	return store.Save(ctx, values)
}

// StartPersisting writes the counters every interval until ctx ends, then does
// one final flush. It is meant to run in the goroutine that owns the process
// lifetime; a failed write is logged and retried on the next tick.
func StartPersisting(ctx context.Context, store CounterStore, interval time.Duration) {
	if interval <= 0 {
		interval = PersistInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), persistTimeout)
			if err := Flush(flushCtx, store); err != nil {
				log.Printf("persist counters on shutdown: %v", err)
			}
			cancel()
			return
		case <-ticker.C:
			if err := Flush(ctx, store); err != nil {
				log.Printf("persist counters: %v", err)
			}
		}
	}
}

// snapshotCounters collects the current value of every counter series this
// package owns from the default registry.
func snapshotCounters() ([]CounterValue, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather counters: %w", err)
	}
	var out []CounterValue
	for _, family := range families {
		if family.GetType() != dto.MetricType_COUNTER || !persistedName(family.GetName()) {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			out = append(out, CounterValue{
				Name:   family.GetName(),
				Labels: labels,
				Value:  metric.GetCounter().GetValue(),
			})
		}
	}
	return out, nil
}

// persistedName limits the snapshot to the counters this package reports. Go
// runtime and process metrics are cheap to recompute and meaningless across
// restarts.
func persistedName(name string) bool {
	return strings.HasPrefix(name, "consolidation_") || strings.HasPrefix(name, "reasoner_")
}

// counterVec maps a series name back to the vector that owns it, so Restore can
// add a persisted total to the live counter. A name without a vector here is
// skipped; that includes the duration histogram, which is not persisted.
func counterVec(name string) *prometheus.CounterVec {
	switch name {
	case "consolidation_events_processed_total":
		return eventsProcessed
	case "consolidation_facts_created_total":
		return factsCreated
	case "consolidation_facts_deduplicated_total":
		return factsDeduplicated
	case "consolidation_relationships_created_total":
		return relationshipsCreated
	case "consolidation_llm_calls_total":
		return llmCalls
	case "consolidation_clusters_found_total":
		return clustersFound
	case "consolidation_events_read_total":
		return eventsRead
	case "consolidation_errors_total":
		return errorsTotal
	case "reasoner_requests_total":
		return reasonerRequests
	case "reasoner_prompt_tokens_total":
		return reasonerPromptTokens
	case "reasoner_completion_tokens_total":
		return reasonerCompletionTokens
	case "reasoner_cost_total":
		return reasonerCost
	case "reasoner_unpriced_calls_total":
		return reasonerUnpriced
	}
	return nil
}
