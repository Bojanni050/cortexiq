// Modifications copyright (c) 2026 Bojan van der Heide
// Package queries provides sqltmpl templates for dynamic SQL generation.
// Templates are embedded at compile time.
package queries

import (
	"embed"
	"fmt"
	"text/template"
	"time"

	"github.com/alash3al/sqltmpl"
	"github.com/pgvector/pgvector-go"
)

//go:embed *.sql.tmpl
var queryFS embed.FS

// Queries wraps sqltmpl templates with typed execution helpers.
type Queries struct {
	tpl *sqltmpl.Template
}

// New parses embedded templates and returns a ready Queries instance.
func New() (*Queries, error) {
	textTpl, err := template.ParseFS(queryFS, "*.sql.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse query templates: %w", err)
	}

	tpl := sqltmpl.New(textTpl, func(i int) string {
		return fmt.Sprintf("$%d", i)
	})

	return &Queries{tpl: tpl}, nil
}

// RecallEpisodes returns SQL + args for episode vector search.
// If namespaceIDs is nil, searches all namespaces. A non-nil asOf restricts the
// search to what existed at that moment (the other Recall* queries do the same).
func (q *Queries) RecallEpisodes(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf *time.Time) (string, []any, error) {
	return q.RecallEpisodesWindow(namespaceIDs, vector, limit, asOf, nil, nil, Known{})
}

// RecallEpisodesWindow returns episode vector hits limited by stored-at asOf
// and/or by the event-time window [from, to). When includeEmbedding is true the
// embedding column is also selected (for MMR diversification).
func (q *Queries) RecallEpisodesWindow(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf, from, to *time.Time, known Known) (string, []any, error) {
	return q.RecallEpisodesWindowWithEmbedding(namespaceIDs, vector, limit, asOf, from, to, known, false, nil)
}

// RecallEpisodesWindowWithEmbedding is RecallEpisodesWindow with an option to
// include the embedding column in the result (needed for MMR).
func (q *Queries) RecallEpisodesWindowWithEmbedding(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf, from, to *time.Time, known Known, includeEmbedding bool, tags []string) (string, []any, error) {
	args := map[string]any{
		"vector":            vector,
		"limit":             limit,
		"include_embedding": includeEmbedding,
	}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setTags(args, tags)
	setAsOf(args, asOf)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_episodes", args)
}

// RecallFacts returns SQL + args for fact vector search.
// If namespaceIDs is nil, searches all namespaces.
func (q *Queries) RecallFacts(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf *time.Time) (string, []any, error) {
	return q.RecallFactsWindow(namespaceIDs, vector, limit, asOf, nil, nil, Known{})
}

// RecallFactsWindow returns fact vector hits constrained by valid-time overlap.
func (q *Queries) RecallFactsWindow(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf, from, to *time.Time, known Known) (string, []any, error) {
	return q.RecallFactsWindowWithEmbedding(namespaceIDs, vector, limit, asOf, from, to, known, false, nil)
}

// RecallFactsWindowWithEmbedding is RecallFactsWindow with an option to include
// the embedding column in the result (needed for MMR).
func (q *Queries) RecallFactsWindowWithEmbedding(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf, from, to *time.Time, known Known, includeEmbedding bool, tags []string) (string, []any, error) {
	args := map[string]any{
		"vector":            vector,
		"limit":             limit,
		"include_embedding": includeEmbedding,
	}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setTags(args, tags)
	setAsOf(args, asOf)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_facts", args)
}

// RecallEpisodesKeyword returns SQL + args for episode full-text search.
// query is a websearch-syntax string (see brain.keywordQuery).
func (q *Queries) RecallEpisodesKeyword(namespaceIDs []int64, query string, limit int, asOf *time.Time) (string, []any, error) {
	return q.RecallEpisodesKeywordWindow(namespaceIDs, query, limit, asOf, nil, nil, Known{}, nil)
}

func (q *Queries) RecallEpisodesKeywordWindow(namespaceIDs []int64, query string, limit int, asOf, from, to *time.Time, known Known, tags []string) (string, []any, error) {
	args := keywordArgs(namespaceIDs, query, limit, asOf)
	setTags(args, tags)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_episodes_keyword", args)
}

// RecallFactsKeyword returns SQL + args for fact full-text search.
func (q *Queries) RecallFactsKeyword(namespaceIDs []int64, query string, limit int, asOf *time.Time) (string, []any, error) {
	return q.RecallFactsKeywordWindow(namespaceIDs, query, limit, asOf, nil, nil, Known{}, nil)
}

func (q *Queries) RecallFactsKeywordWindow(namespaceIDs []int64, query string, limit int, asOf, from, to *time.Time, known Known, tags []string) (string, []any, error) {
	args := keywordArgs(namespaceIDs, query, limit, asOf)
	setTags(args, tags)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_facts_keyword", args)
}

// RecallFailuresKeyword returns SQL + args for failure full-text search. Rows are
// id, namespace_id, content, reason, lesson, created_at, score.
func (q *Queries) RecallFailuresKeyword(namespaceIDs []int64, query string, limit int, asOf *time.Time) (string, []any, error) {
	return q.tpl.Execute("recall_failures_keyword", keywordArgs(namespaceIDs, query, limit, asOf))
}

// RecallGoalsKeyword returns SQL + args for goal full-text search. Rows are
// id, namespace_id, content, status, notes, created_at, score.
func (q *Queries) RecallGoalsKeyword(namespaceIDs []int64, query string, limit int, asOf *time.Time) (string, []any, error) {
	return q.tpl.Execute("recall_goals_keyword", keywordArgs(namespaceIDs, query, limit, asOf))
}

// RecallFactsGraph returns SQL + args for the graph arm of fact recall: facts
// about an entity the query names, then facts about its direct neighbours in
// the relationship graph. Rows have the columns of RecallFacts; the score is 1
// for a named entity and at most 0.5 for a neighbour, and rows come ordered
// best first, which is the only thing fusion uses. Within a hop the facts are
// ordered by cosine similarity of their embedding to vector (the query's), so
// an entity with many facts puts the ones that answer the question first.
func (q *Queries) RecallFactsGraph(namespaceIDs []int64, query string, vector pgvector.Vector, limit int, asOf *time.Time) (string, []any, error) {
	return q.RecallFactsGraphWindow(namespaceIDs, query, vector, limit, 0, asOf, nil, nil, Known{}, nil)
}

// minSimilarity, when positive, keeps a neighbour only if its embedding is at
// least that close to the query. Without it the arm adds every fact whose entity
// a neighbour shares, however unrelated the fact itself is: a topic question that
// happens to name one entity then collects that entity's whole neighbourhood.
func (q *Queries) RecallFactsGraphWindow(namespaceIDs []int64, query string, vector pgvector.Vector, limit int, minSimilarity float32, asOf, from, to *time.Time, known Known, tags []string) (string, []any, error) {
	args := keywordArgs(namespaceIDs, query, limit, asOf)
	args["vector"] = vector
	if minSimilarity > 0 {
		args["min_similarity"] = minSimilarity
	}
	setTags(args, tags)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_facts_graph", args)
}

// Temporal methods return the same row shape as their vector/keyword peers,
// ordered by event/valid-time proximity within the requested window.
func (q *Queries) RecallEpisodesTemporal(namespaceIDs []int64, limit int, asOf, from, to *time.Time, known Known, tags []string) (string, []any, error) {
	args := map[string]any{"limit": limit}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setTags(args, tags)
	setAsOf(args, asOf)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_episodes_temporal", args)
}

func (q *Queries) RecallFactsTemporal(namespaceIDs []int64, limit int, asOf, from, to *time.Time, known Known, tags []string) (string, []any, error) {
	args := map[string]any{"limit": limit}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setTags(args, tags)
	setAsOf(args, asOf)
	setWindow(args, from, to)
	setKnown(args, known)
	return q.tpl.Execute("recall_facts_temporal", args)
}

// RecallMatchedEntities returns SQL + args for the entity names (lower case)
// that a query names. It is what the graph arm starts from.
func (q *Queries) RecallMatchedEntities(namespaceIDs []int64, query string) (string, []any, error) {
	return q.tpl.Execute("recall_matched_entities", keywordArgs(namespaceIDs, query, 0, nil))
}

func keywordArgs(namespaceIDs []int64, query string, limit int, asOf *time.Time) map[string]any {
	args := map[string]any{
		"query": query,
		"limit": limit,
	}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setAsOf(args, asOf)
	return args
}

func setTags(args map[string]any, tags []string) {
	if len(tags) > 0 {
		args["tags"] = tags
	}
}

// setAsOf switches a recall template to point-in-time mode: what the system
// held at that moment (facts not yet superseded, episodes already stored),
// instead of "valid now".
func setAsOf(args map[string]any, asOf *time.Time) {
	if asOf != nil {
		args["as_of"] = asOf.UTC()
	}
}

// Known is a half-open window [From, To) on the knowledge axis: it selects the
// beliefs the system held at some point in it. as_of is the point form.
type Known struct {
	From, To *time.Time
}

func setKnown(args map[string]any, k Known) {
	if k.From != nil {
		args["known_from"] = k.From.UTC()
	}
	if k.To != nil {
		args["known_to"] = k.To.UTC()
	}
}

func setWindow(args map[string]any, from, to *time.Time) {
	if from != nil {
		args["from"] = from.UTC()
	}
	if to != nil {
		args["to"] = to.UTC()
	}
}

// FetchEpisodes returns SQL + args for batch episode read after a checkpoint.
func (q *Queries) FetchEpisodes(namespaceID int64, afterID int64, limit int) (string, []any, error) {
	return q.tpl.Execute("fetch_episodes", map[string]any{
		"namespace_id": namespaceID,
		"after_id":     afterID,
		"limit":        limit,
	})
}

// FetchEpisodesByTags is FetchEpisodes narrowed to the episodes carrying every
// tag. A tag-scoped consolidation is a targeted run, not an incremental sweep:
// it reads from the start of the namespace, because the checkpoint cannot move
// over the untagged episodes in between without skipping them forever.
func (q *Queries) FetchEpisodesByTags(namespaceID int64, afterID int64, limit int, tags []string) (string, []any, error) {
	args := map[string]any{
		"namespace_id": namespaceID,
		"after_id":     afterID,
		"limit":        limit,
	}
	setTags(args, tags)
	return q.tpl.Execute("fetch_episodes", args)
}

// FetchFacts returns SQL + args for batch fact read after a checkpoint.
func (q *Queries) FetchFacts(namespaceID int64, afterID int64, limit int) (string, []any, error) {
	return q.tpl.Execute("fetch_facts", map[string]any{
		"namespace_id": namespaceID,
		"after_id":     afterID,
		"limit":        limit,
	})
}

// FetchRelationships returns SQL + args for batch relationship read after a checkpoint.
func (q *Queries) FetchRelationships(namespaceID int64, afterID int64, limit int) (string, []any, error) {
	return q.tpl.Execute("fetch_relationships", map[string]any{
		"namespace_id": namespaceID,
		"after_id":     afterID,
		"limit":        limit,
	})
}
