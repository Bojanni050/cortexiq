// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// includesWork reports whether the options ask for failures or goals. Unlike
// facts and episodes they are never searched by default: they have no embedding,
// so they could only ever contribute keyword hits, and adding them to every
// recall would change what existing callers get back.
func (o RecallOptions) includesWork() (failures, goals bool) {
	for _, t := range o.Types {
		switch t {
		case "failure":
			failures = true
		case "goal":
			goals = true
		}
	}
	return failures, goals
}

// recallWork searches failures and/or goals with full-text search only. Scores sit
// on the same 0..1 scale as a hybrid fact (rank 1 in one of two arms is 0.5), so a
// keyword-only hit never outranks something both rankers agreed on.
func (b *Brain) recallWork(ctx context.Context, nsIDs []int64, query string, limit int, asOf *time.Time, failures, goals bool) ([]RecallResult, error) {
	kw := keywordQuery(query)
	if kw == "" {
		return nil, nil
	}
	var out []RecallResult

	if failures {
		sql, args, err := b.queries.RecallFailuresKeyword(nsIDs, kw, limit, asOf)
		if err != nil {
			return nil, fmt.Errorf("build failure query: %w", err)
		}
		rows, err := b.pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, fmt.Errorf("query failures: %w", err)
		}
		for rank := 0; rows.Next(); rank++ {
			r := RecallResult{Type: "failure", Importance: DefaultImportance, Match: "keyword", kwRank: rank + 1}
			var content, reason, lesson string
			var created time.Time
			if err := rows.Scan(&r.ID, &r.NamespaceID, &content, &reason, &lesson, &created, &r.kwScore); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan failure: %w", err)
			}
			r.Content = joinWork(content, "reason", reason, "lesson", lesson)
			r.finishWork(created, rank)
			out = append(out, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("query failures: %w", err)
		}
	}

	if goals {
		sql, args, err := b.queries.RecallGoalsKeyword(nsIDs, kw, limit, asOf)
		if err != nil {
			return nil, fmt.Errorf("build goal query: %w", err)
		}
		rows, err := b.pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, fmt.Errorf("query goals: %w", err)
		}
		for rank := 0; rows.Next(); rank++ {
			r := RecallResult{Type: "goal", Importance: DefaultImportance, Match: "keyword", kwRank: rank + 1}
			var content, status, notes string
			var created time.Time
			if err := rows.Scan(&r.ID, &r.NamespaceID, &content, &status, &notes, &created, &r.kwScore); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan goal: %w", err)
			}
			r.Content = joinWork(content, "status", status, "notes", notes)
			r.finishWork(created, rank)
			out = append(out, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("query goals: %w", err)
		}
	}
	return out, nil
}

// finishWork fills the fields every failure or goal hit shares.
func (r *RecallResult) finishWork(created time.Time, rank int) {
	r.typedID(r.Type)
	r.CreatedAt = created.Format(time.RFC3339)
	r.refTime = created
	r.fusedScore = float32((1.0 / float64(rrfK+rank+1)) / (2.0 / float64(rrfK+1)))
	r.Score = r.fusedScore
	r.recencyF, r.importanceF = 1, 1
}

// joinWork renders a failure or goal as one line: the content, then each
// non-empty labelled part, so the reader sees the lesson or status without a
// second lookup.
func joinWork(content string, label1, v1, label2, v2 string) string {
	var sb strings.Builder
	sb.WriteString(content)
	if v1 != "" {
		sb.WriteString(" | " + label1 + ": " + v1)
	}
	if v2 != "" {
		sb.WriteString(" | " + label2 + ": " + v2)
	}
	return sb.String()
}
