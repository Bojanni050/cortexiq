package webui

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

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

func gatherMetricSamples() ([]metricSampleJSON, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}

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
	return out, nil
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
	metrics, err := gatherMetricSamples()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated_at": time.Now().UTC(),
		"metrics":    metrics,
	})
}
