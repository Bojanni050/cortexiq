package webui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// metricsFetchTimeout caps the scrape of the process that owns the counters.
// It keeps a dashboard request from hanging on an unreachable sibling.
const metricsFetchTimeout = 3 * time.Second

type metricSampleJSON struct {
	Name   string            `json:"name"`
	Help   string            `json:"help,omitempty"`
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

func metricLabels(m *dto.Metric) map[string]string {
	labels := make(map[string]string, len(m.GetLabel()))
	for _, label := range m.GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

func metricType(t dto.MetricType) string {
	return strings.ToLower(t.String())
}

func appendMetric(out *[]metricSampleJSON, name, help, typ string, labels map[string]string, value float64) {
	*out = append(*out, metricSampleJSON{Name: name, Help: help, Type: typ, Labels: labels, Value: value})
}

// flattenFamilies turns collected metric families into the flat sample list the
// dashboard consumes. Both the local registry and a scraped peer arrive as
// families, so the two paths share this.
func flattenFamilies(families []*dto.MetricFamily) []metricSampleJSON {
	var out []metricSampleJSON
	for _, family := range families {
		name, help, typ := family.GetName(), family.GetHelp(), metricType(family.GetType())
		for _, metric := range family.GetMetric() {
			labels := metricLabels(metric)
			switch {
			case metric.Counter != nil:
				appendMetric(&out, name, help, typ, labels, metric.GetCounter().GetValue())
			case metric.Gauge != nil:
				appendMetric(&out, name, help, typ, labels, metric.GetGauge().GetValue())
			case metric.Untyped != nil:
				appendMetric(&out, name, help, typ, labels, metric.GetUntyped().GetValue())
			case metric.Histogram != nil:
				h := metric.GetHistogram()
				appendMetric(&out, name+"_count", help, typ, labels, float64(h.GetSampleCount()))
				appendMetric(&out, name+"_sum", help, typ, labels, h.GetSampleSum())
				for _, bucket := range h.GetBucket() {
					bucketLabels := copyLabels(labels)
					bucketLabels["le"] = formatFloat(bucket.GetUpperBound())
					appendMetric(&out, name+"_bucket", help, typ, bucketLabels, float64(bucket.GetCumulativeCount()))
				}
			case metric.Summary != nil:
				s := metric.GetSummary()
				appendMetric(&out, name+"_count", help, typ, labels, float64(s.GetSampleCount()))
				appendMetric(&out, name+"_sum", help, typ, labels, s.GetSampleSum())
				for _, quantile := range s.GetQuantile() {
					quantileLabels := copyLabels(labels)
					quantileLabels["quantile"] = formatFloat(quantile.GetQuantile())
					appendMetric(&out, name, help, typ, quantileLabels, quantile.GetValue())
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return labelsString(out[i].Labels) < labelsString(out[j].Labels)
	})
	return out
}

func gatherMetricSamples() ([]metricSampleJSON, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}
	return flattenFamilies(families), nil
}

// fetchMetricSamples scrapes another CortexIQ process's Prometheus endpoint and
// returns its samples in the same shape as the local registry.
func fetchMetricSamples(ctx context.Context, url, token string) ([]metricSampleJSON, error) {
	ctx, cancel := context.WithTimeout(ctx, metricsFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("%s: %s: %s", url, resp.Status, strings.TrimSpace(string(detail)))
	}

	// The source is served by client_golang, which writes legacy metric names.
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	slice := make([]*dto.MetricFamily, 0, len(families))
	for _, family := range families {
		slice = append(slice, family)
	}
	return flattenFamilies(slice), nil
}

// gatherMetrics reads the consolidation and reasoner counters from the process
// that produces them when a source is configured. The split production
// deployment serves the dashboard from its own process, whose registry only
// ever holds Go runtime metrics and materialised zeros; the real counters live
// on the MCP/consolidation process. It falls back to the local registry when the
// peer is unreachable, so the page stays usable and the failure is visible.
func (h *Handler) gatherMetrics(ctx context.Context) (samples []metricSampleJSON, source string, err error) {
	if h.MetricsSource != "" {
		samples, err = fetchMetricSamples(ctx, h.MetricsSource, h.MetricsToken)
		if err == nil {
			return samples, "mcp", nil
		}
		log.Printf("metrics: source %s unavailable, showing this process instead: %v", h.MetricsSource, err)
	}
	samples, err = gatherMetricSamples()
	return samples, "local", err
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func labelsString(labels map[string]string) string {
	var keys []string
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(labels[key])
		b.WriteByte(';')
	}
	return b.String()
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	metrics, source, err := h.gatherMetrics(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated_at": time.Now().UTC(),
		"source":     source,
		"metrics":    metrics,
	})
}
