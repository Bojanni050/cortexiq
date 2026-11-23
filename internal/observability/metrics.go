// Modifications copyright (c) 2026 Bojan van der Heide
package observability

import (
	"time"

	"github.com/Bojanni050/cortexiq/internal/schedule"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	eventsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_events_processed_total",
			Help: "Total number of events processed by consolidation",
		}, []string{"namespace"},
	)
	factsCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_facts_created_total",
			Help: "Total number of facts created during consolidation",
		}, []string{"namespace"},
	)
	factsDeduplicated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_facts_deduplicated_total",
			Help: "Total number of facts skipped due to semantic deduplication",
		}, []string{"namespace"},
	)
	relationshipsCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_relationships_created_total",
			Help: "Total number of relationships extracted",
		}, []string{"namespace"},
	)
	llmCalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_llm_calls_total",
			Help: "Total number of LLM calls made during consolidation",
		}, []string{"namespace"},
	)
	clustersFound = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_clusters_found_total",
			Help: "Total number of clusters evaluated",
		}, []string{"namespace"},
	)
	eventsRead = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_events_read_total",
			Help: "Total number of events read during consolidation",
		}, []string{"namespace"},
	)
	duration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "consolidation_duration_seconds",
			Help:    "Duration of consolidation runs",
			Buckets: prometheus.DefBuckets,
		}, []string{"namespace"},
	)
	errorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consolidation_errors_total",
			Help: "Number of errors encountered during consolidation",
		}, []string{"namespace"},
	)
	reasonerRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_requests_total",
			Help: "Total number of reasoner API requests, including retries",
		}, []string{"stage", "status"},
	)
	reasonerPromptTokens = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_prompt_tokens_total",
			Help: "Prompt tokens reported by reasoner API responses",
		}, []string{"stage"},
	)
	reasonerCompletionTokens = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_completion_tokens_total",
			Help: "Completion tokens reported by reasoner API responses",
		}, []string{"stage"},
	)
	rerankRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "recall_rerank_requests_total",
			Help: "Cross-encoder rerank requests made by recall; status error means recall fell back to the fused order",
		}, []string{"status"},
	)
	// The loop's own state. A run row is only written when a run ends, so a run
	// in flight is invisible in the database; the dashboard reads these from the
	// consolidator's metrics endpoint to say "running" and to count down to the
	// next round.
	scheduleInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "consolidation_schedule_info",
			Help: "The configured consolidation schedule; the value is always 1 and the mode is the label",
		}, []string{"mode"},
	)
	scheduleIntervalSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "consolidation_schedule_interval_seconds",
			Help: "Timer period of the consolidation schedule; 0 in events-only mode",
		},
	)
	runActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "consolidation_run_active",
			Help: "1 while a consolidation round is in progress, 0 otherwise",
		},
	)
	runStartedAt = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "consolidation_run_started_timestamp_seconds",
			Help: "Unix time the current consolidation round started; 0 when idle",
		},
	)
	runNamespace = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "consolidation_run_namespace_id",
			Help: "Namespace being consolidated right now; 0 when idle",
		},
	)
	nextRunAt = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "consolidation_next_run_timestamp_seconds",
			Help: "Unix time the timer starts the next round; 0 when no timer is armed",
		},
	)
	retryAt = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "consolidation_retry_timestamp_seconds",
			Help: "Unix time a run is retried because work is waiting behind a failed batch; 0 when none is armed",
		},
	)
)

var reasonerStages = []string{
	"same_fact",
	"structured",
	"relationships",
	"patterns",
	"contradiction",
	"causal_links",
	"goal_progress",
	"failure_patterns",
	"hypothesis_evidence",
}

func init() {
	// Materialise zero-valued series so dashboards show the complete reasoner
	// metric set before the first consolidation request is made.
	for _, stage := range reasonerStages {
		for _, status := range []string{"success", "error"} {
			reasonerRequests.WithLabelValues(stage, status)
		}
		reasonerPromptTokens.WithLabelValues(stage)
		reasonerCompletionTokens.WithLabelValues(stage)
	}
	for _, status := range []string{"success", "error"} {
		rerankRequests.WithLabelValues(status)
	}
}

// RecordRerank counts one rerank request made by recall. A failed request does
// not fail the recall, which falls back to the fused order, so this counter is
// the only place a dead reranker (an exhausted account, a bad key) shows up.
func RecordRerank(success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	rerankRequests.WithLabelValues(status).Inc()
}

// Observation carries the metrics that should be exported for a run.
type Observation struct {
	Namespace          string
	EventsRead         int
	EventsProcessed    int
	ClustersFound      int
	FactsCreated       int
	FactsDeduplicated  int
	RelationshipsFound int
	LLMCalls           int
	Duration           time.Duration
	Errors             int
}

// RecordScheduleStatus publishes the loop's live state: whether a round is
// running, which namespace it is on, and when the timer fires next. The
// dashboard in the split deployment reads these gauges from this process,
// because the timer exists here and nowhere else.
func RecordScheduleStatus(st schedule.Status) {
	scheduleInfo.Reset()
	scheduleInfo.WithLabelValues(string(st.Mode)).Set(1)
	interval := st.Interval.Seconds()
	if st.Mode == schedule.ModeEvents {
		interval = 0
	}
	scheduleIntervalSeconds.Set(interval)
	nextRunAt.Set(unixSeconds(st.NextRun))
	retryAt.Set(unixSeconds(st.RetryAt))
	if st.Running {
		runActive.Set(1)
		runStartedAt.Set(unixSeconds(st.RunningSince))
		runNamespace.Set(float64(st.Namespace))
		return
	}
	runActive.Set(0)
	runStartedAt.Set(0)
	runNamespace.Set(0)
}

// unixSeconds renders a moment as a Prometheus timestamp, and the zero time as
// 0 so a reader can tell "no timer armed" from 1970.
func unixSeconds(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UnixNano()) / float64(time.Second)
}

// RecordConsolidation exports the provided observation to Prometheus.
func RecordConsolidation(obs Observation) {
	if obs.Namespace == "" {
		obs.Namespace = "default"
	}
	eventsProcessed.WithLabelValues(obs.Namespace).Add(float64(obs.EventsProcessed))
	eventsRead.WithLabelValues(obs.Namespace).Add(float64(obs.EventsRead))
	clustersFound.WithLabelValues(obs.Namespace).Add(float64(obs.ClustersFound))
	factsCreated.WithLabelValues(obs.Namespace).Add(float64(obs.FactsCreated))
	factsDeduplicated.WithLabelValues(obs.Namespace).Add(float64(obs.FactsDeduplicated))
	relationshipsCreated.WithLabelValues(obs.Namespace).Add(float64(obs.RelationshipsFound))
	llmCalls.WithLabelValues(obs.Namespace).Add(float64(obs.LLMCalls))
	duration.WithLabelValues(obs.Namespace).Observe(obs.Duration.Seconds())
	errorsTotal.WithLabelValues(obs.Namespace).Add(float64(obs.Errors))
}

// RecordReasonerCall records one actual provider request. Failed requests are
// counted too, so retry storms are visible instead of looking like silence.
func RecordReasonerCall(stage, model string, promptTokens, completionTokens int64, success bool) {
	if stage == "" {
		stage = "unknown"
	}
	if model == "" {
		model = "unknown"
	}
	status := "success"
	if !success {
		status = "error"
	}
	reasonerRequests.WithLabelValues(stage, status).Inc()
	if promptTokens > 0 {
		reasonerPromptTokens.WithLabelValues(stage).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		reasonerCompletionTokens.WithLabelValues(stage).Add(float64(completionTokens))
	}
	// Only price requests that reported usage; failures without usage are not
	// cost-bearing and would otherwise inflate the unpriced counter.
	if promptTokens+completionTokens > 0 {
		recordReasonerCost(stage, model, promptTokens, completionTokens)
	}
}
