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
	args := map[string]any{
		"vector": vector,
		"limit":  limit,
	}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setAsOf(args, asOf)
	return q.tpl.Execute("recall_episodes", args)
}

// RecallFacts returns SQL + args for fact vector search.
// If namespaceIDs is nil, searches all namespaces.
func (q *Queries) RecallFacts(namespaceIDs []int64, vector pgvector.Vector, limit int, asOf *time.Time) (string, []any, error) {
	args := map[string]any{
		"vector": vector,
		"limit":  limit,
	}
	if len(namespaceIDs) > 0 {
		args["namespace_ids"] = namespaceIDs
	}
	setAsOf(args, asOf)
	return q.tpl.Execute("recall_facts", args)
}

// RecallEpisodesKeyword returns SQL + args for episode full-text search.
// query is a websearch-syntax string (see brain.keywordQuery).
func (q *Queries) RecallEpisodesKeyword(namespaceIDs []int64, query string, limit int, asOf *time.Time) (string, []any, error) {
	return q.tpl.Execute("recall_episodes_keyword", keywordArgs(namespaceIDs, query, limit, asOf))
}

// RecallFactsKeyword returns SQL + args for fact full-text search.
func (q *Queries) RecallFactsKeyword(namespaceIDs []int64, query string, limit int, asOf *time.Time) (string, []any, error) {
	return q.tpl.Execute("recall_facts_keyword", keywordArgs(namespaceIDs, query, limit, asOf))
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
	args := keywordArgs(namespaceIDs, query, limit, asOf)
	args["vector"] = vector
	return q.tpl.Execute("recall_facts_graph", args)
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

// setAsOf switches a recall template to point-in-time mode: facts valid at that
// moment (instead of "valid now") and episodes the system already knew then.
func setAsOf(args map[string]any, asOf *time.Time) {
	if asOf != nil {
		args["as_of"] = asOf.UTC()
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
