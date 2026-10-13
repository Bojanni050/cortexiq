package observability

import (
	"time"

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
func RecordReasonerCall(stage string, promptTokens, completionTokens int64, success bool) {
	if stage == "" {
		stage = "unknown"
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
}
