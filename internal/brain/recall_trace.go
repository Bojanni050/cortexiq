// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"time"
)

// RecallOptions narrows a detailed recall.
type RecallOptions struct {
	// AsOf answers "what did we know at that moment"; see RecallAsOf.
	AsOf *time.Time
	// KnownFrom and KnownTo select what the system held at some point in the
	// half-open window [KnownFrom, KnownTo): a fact counts when it was stored
	// before KnownTo and not yet superseded or expired at KnownFrom. AsOf is the
	// point form of this axis, so combining them is an error. From and To stay the
	// valid-time axis and compose with either.
	KnownFrom *time.Time
	KnownTo   *time.Time
	// From and To select memories whose event/validity interval overlaps this
	// half-open time window. They are explicit bounds and override query parsing.
	From *time.Time
	To   *time.Time
	// Types limits the search to "fact", "episode", "failure" and/or "goal". Empty
	// means facts and episodes; failures and goals are only searched when named.
	Types []string
	// Graph overrides the configured graph arm for this call: true forces it on,
	// false forces it off, nil keeps the configuration. The recall evaluation uses
	// it to compare the same corpus with and without the arm.
	Graph *bool
	// Route overrides the configured routing for this call, like Graph: true
	// forces it on, false forces it off, nil keeps the configuration. Routing only
	// applies when Types is empty.
	Route *bool
	// MMR overrides the configured MMR lambda for this call: 0 disables MMR,
	// 1 maximises diversity, 0.5 balances. nil keeps the configuration.
	MMR *float64
}

// includes reports which of the two stores the options ask for.
func (o RecallOptions) includes() (facts, episodes bool) {
	if len(o.Types) == 0 {
		return true, true
	}
	for _, t := range o.Types {
		switch t {
		case "fact":
			facts = true
		case "episode":
			episodes = true
		}
	}
	return facts, episodes
}

// RecallStage is one step of a recall: how many rows it produced and how long
// the step took since the previous one.
type RecallStage struct {
	Name   string  `json:"name"`
	Count  int     `json:"count"`
	Millis float64 `json:"ms"`
}

// RecallTrace explains one recall: the parameters it ran with and what each step
// contributed. It is what the dashboard's recall analyzer shows.
type RecallTrace struct {
	Query        string `json:"query"`
	KeywordQuery string `json:"keyword_query,omitempty"`
	Hybrid       bool   `json:"hybrid"`
	// Graph says the graph arm ran; GraphEntities are the entities the query named.
	Graph         bool     `json:"graph"`
	GraphEntities []string `json:"graph_entities,omitempty"`
	// Routed lists the extra types the query's wording added ("failure", "goal").
	Routed       []string `json:"routed,omitempty"`
	Limit        int      `json:"limit"`
	Candidates   int      `json:"candidates"`
	Namespaces   []string `json:"namespaces"`
	NamespaceIDs int      `json:"namespace_ids"`
	AsOf         string   `json:"as_of,omitempty"`
	// Inferred names the time arguments that came from the query's wording rather
	// than from the caller ("known_from", "known_to", "from", "to").
	Inferred      []string `json:"inferred,omitempty"`
	KnownFrom     string   `json:"known_from,omitempty"`
	KnownTo       string   `json:"known_to,omitempty"`
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	Temporal      bool     `json:"temporal"`
	MinSimilarity float32  `json:"min_similarity"`
	RRFK          int      `json:"rrf_k"`
	// Recency and importance weighting reorder the fused list; zero means off.
	RecencyWeight    float64 `json:"recency_weight"`
	RecencyHalfLife  string  `json:"recency_half_life"`
	ImportanceWeight float64 `json:"importance_weight"`
	// MMRLambda is the MMR diversification parameter that ran; 0 means MMR was off.
	MMRLambda   float64       `json:"mmr_lambda"`
	Stages      []RecallStage `json:"stages"`
	TotalMillis float64       `json:"total_ms"`
}

// ResultTrace says how one result got its rank: where each arm placed it, what
// the fusion made of that, and what recency and importance did to the score.
type ResultTrace struct {
	// VectorRank and KeywordRank are 1-based positions in the arm's own ranking;
	// 0 means the arm did not return the result.
	VectorRank       int     `json:"vector_rank,omitempty"`
	VectorSimilarity float32 `json:"vector_similarity,omitempty"`
	KeywordRank      int     `json:"keyword_rank,omitempty"`
	KeywordScore     float32 `json:"keyword_score,omitempty"`
	// GraphScore is 1 for a fact about an entity the query named and at most 0.5
	// for a fact about a neighbour of such an entity.
	GraphRank    int     `json:"graph_rank,omitempty"`
	GraphScore   float32 `json:"graph_score,omitempty"`
	TemporalRank int     `json:"temporal_rank,omitempty"`
	// FusedScore is the score after fusion and before recency and importance.
	FusedScore       float32 `json:"fused_score"`
	RecencyFactor    float32 `json:"recency_factor"`
	ImportanceFactor float32 `json:"importance_factor"`
}

func (r RecallResult) trace() *ResultTrace {
	return &ResultTrace{
		VectorRank: r.vecRank, VectorSimilarity: r.Similarity,
		KeywordRank: r.kwRank, KeywordScore: r.kwScore,
		GraphRank: r.graphRank, GraphScore: r.graphScore,
		TemporalRank:  r.temporalRank,
		FusedScore:    r.fusedScore,
		RecencyFactor: float32(r.recencyF), ImportanceFactor: float32(r.importanceF),
	}
}

// stage returns a recorder that appends a RecallStage with the time since the
// previous call. On a nil trace it records nothing.
func (t *RecallTrace) stage(start time.Time) func(name string, count int) {
	last := start
	return func(name string, count int) {
		if t == nil {
			return
		}
		now := time.Now()
		t.Stages = append(t.Stages, RecallStage{Name: name, Count: count, Millis: float64(now.Sub(last).Microseconds()) / 1000})
		last = now
	}
}

// RecallWith is Recall with options: a point in time and a choice of stores. It
// counts the returned facts as used, like Recall does for a current lookup.
func (b *Brain) RecallWith(ctx context.Context, namespaces []string, query string, limit int, opts RecallOptions) ([]RecallResult, error) {
	return b.recall(ctx, namespaces, query, limit, opts, nil)
}

// RecallDetailed is Recall with an explanation. It returns the same ranking a
// plain recall would, with a Trace on every result and a RecallTrace for the
// whole run. Unlike Recall it does not count the returned facts as used, so
// inspecting a ranking never changes how facts decay.
func (b *Brain) RecallDetailed(ctx context.Context, namespaces []string, query string, limit int, opts RecallOptions) ([]RecallResult, *RecallTrace, error) {
	tr := &RecallTrace{}
	results, err := b.recall(ctx, namespaces, query, limit, opts, tr)
	if err != nil {
		return nil, nil, err
	}
	return results, tr, nil
}

// matchedEntities lists the entities a query names. It only feeds the trace, so
// a failure leaves the list empty instead of failing the recall.
func (b *Brain) matchedEntities(ctx context.Context, nsIDs []int64, query string) []string {
	sql, args, err := b.queries.RecallMatchedEntities(nsIDs, query)
	if err != nil {
		return nil
	}
	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	return names
}
