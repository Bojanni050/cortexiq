// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PurgeOptions describes one hard delete. Reason and Actor are written to
// purge_log; DryRun reports what would be removed without removing it.
type PurgeOptions struct {
	Reason string
	Actor  string
	DryRun bool
}

func (o PurgeOptions) normalized() PurgeOptions {
	o.Reason = strings.TrimSpace(o.Reason)
	o.Actor = strings.TrimSpace(o.Actor)
	if o.Reason == "" {
		o.Reason = "no reason given"
	}
	if o.Actor == "" {
		o.Actor = "unknown"
	}
	return o
}

// PurgeCounts is what a purge removed, or would remove for a dry run. The
// cascade (fact sources, feedback, contradictions, causal links, patterns,
// mental models of a purged namespace) is not itemised: those rows have no
// meaning without the row that owned them.
type PurgeCounts struct {
	Episodes    int `json:"episodes"`
	Facts       int `json:"facts"`
	Retractions int `json:"retractions"`
	Embeddings  int `json:"embeddings"`
}

func (c PurgeCounts) empty() bool {
	return c.Episodes == 0 && c.Facts == 0 && c.Retractions == 0 && c.Embeddings == 0
}

// factTarget is one fact caught up in an episode or namespace purge.
type factTarget struct {
	id      int64
	content string
}

// PurgeEpisode hard-deletes an episode and everything that only exists because
// of it.
//
// Facts that were sourced from this episode *and nothing else* are purged with
// it: leaving them behind would mean the memory survives a delete under a
// different name, which is exactly what the caller asked to prevent. Facts with
// another source stay, because that other source still claims them.
//
// The cache row for the purged text is removed too. The embedding cache is
// keyed by content hash and holds the original text, so without this step a
// "deleted" memory is still readable in embedding_cache.
func (b *Brain) PurgeEpisode(ctx context.Context, episodeID int64, opts PurgeOptions) (PurgeCounts, error) {
	opts = opts.normalized()
	var counts PurgeCounts

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return counts, fmt.Errorf("purge episode: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var nsID int64
	var content string
	err = tx.QueryRow(ctx,
		`SELECT namespace_id, content FROM episodes WHERE id = $1 FOR UPDATE`, episodeID,
	).Scan(&nsID, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return counts, ErrEpisodeNotFound
	}
	if err != nil {
		return counts, fmt.Errorf("purge episode: load: %w", err)
	}
	counts.Episodes = 1

	// Facts this episode is the sole source of.
	rows, err := tx.Query(ctx,
		`SELECT f.id, f.content FROM facts f
		 WHERE EXISTS (SELECT 1 FROM fact_sources fs WHERE fs.fact_id = f.id AND fs.episode_id = $1)
		   AND NOT EXISTS (SELECT 1 FROM fact_sources fs WHERE fs.fact_id = f.id AND fs.episode_id <> $1)
		 FOR UPDATE`, episodeID)
	if err != nil {
		return counts, fmt.Errorf("purge episode: load derived facts: %w", err)
	}
	var facts []factTarget
	for rows.Next() {
		var t factTarget
		if err := rows.Scan(&t.id, &t.content); err != nil {
			rows.Close()
			return counts, fmt.Errorf("purge episode: scan derived fact: %w", err)
		}
		facts = append(facts, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return counts, fmt.Errorf("purge episode: derived facts: %w", err)
	}

	texts := []string{content}
	for _, f := range facts {
		texts = append(texts, f.content)
	}
	counts.Facts = len(facts)
	counts.Embeddings = len(texts) // upper bound; corrected below by RowsAffected

	if opts.DryRun {
		counts.Retractions, err = countRetractions(ctx, tx, "episode", episodeID)
		if err != nil {
			return counts, err
		}
		return counts, tx.Rollback(ctx)
	}

	for _, f := range facts {
		if err := logPurge(ctx, tx, nsID, "facts", f.id, opts, PurgeCounts{}); err != nil {
			return counts, err
		}
	}
	counts.Retractions, err = deleteRetractions(ctx, tx, "episode", episodeID)
	if err != nil {
		return counts, err
	}
	for _, f := range facts {
		n, err := deleteRetractions(ctx, tx, "fact", f.id)
		if err != nil {
			return counts, err
		}
		counts.Retractions += n
	}
	tag, err := tx.Exec(ctx, `DELETE FROM embedding_cache WHERE text = ANY($1)`, texts)
	if err != nil {
		return counts, fmt.Errorf("purge episode: clear embedding cache: %w", err)
	}
	counts.Embeddings = int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `DELETE FROM facts WHERE id = ANY($1)`, factTargetIDs(facts)); err != nil {
		return counts, fmt.Errorf("purge episode: delete derived facts: %w", err)
	}
	if err := logPurge(ctx, tx, nsID, "episodes", episodeID, opts, counts); err != nil {
		return counts, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM episodes WHERE id = $1`, episodeID); err != nil {
		return counts, fmt.Errorf("purge episode: delete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return counts, fmt.Errorf("purge episode: commit: %w", err)
	}
	return counts, nil
}

// PurgeFact hard-deletes a fact and its own traces.
func (b *Brain) PurgeFact(ctx context.Context, factID int64, opts PurgeOptions) (PurgeCounts, error) {
	opts = opts.normalized()
	var counts PurgeCounts

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return counts, fmt.Errorf("purge fact: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var nsID int64
	var content string
	err = tx.QueryRow(ctx,
		`SELECT namespace_id, content FROM facts WHERE id = $1 FOR UPDATE`, factID,
	).Scan(&nsID, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return counts, ErrFactNotFound
	}
	if err != nil {
		return counts, fmt.Errorf("purge fact: load: %w", err)
	}
	counts.Facts = 1
	counts.Embeddings = 1

	if opts.DryRun {
		counts.Retractions, err = countRetractions(ctx, tx, "fact", factID)
		if err != nil {
			return counts, err
		}
		return counts, tx.Rollback(ctx)
	}

	counts.Retractions, err = deleteRetractions(ctx, tx, "fact", factID)
	if err != nil {
		return counts, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM embedding_cache WHERE text = $1`, content)
	if err != nil {
		return counts, fmt.Errorf("purge fact: clear embedding cache: %w", err)
	}
	counts.Embeddings = int(tag.RowsAffected())

	if err := logPurge(ctx, tx, nsID, "facts", factID, opts, counts); err != nil {
		return counts, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM facts WHERE id = $1`, factID); err != nil {
		return counts, fmt.Errorf("purge fact: delete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return counts, fmt.Errorf("purge fact: commit: %w", err)
	}
	return counts, nil
}

// PurgeNamespace hard-deletes one namespace and every row filed under it.
//
// It refuses a namespace that has children: those are separate namespaces with
// their own contents, and a typo in a parent slug should not wipe a subtree.
// Purge the children explicitly, then the parent.
func (b *Brain) PurgeNamespace(ctx context.Context, slug string, opts PurgeOptions) (PurgeCounts, error) {
	opts = opts.normalized()
	var counts PurgeCounts

	if err := validatePath(slug); err != nil {
		return counts, err
	}
	if slug == "" || slug == "/" {
		return counts, fmt.Errorf("refusing to purge the root namespace: name a namespace explicitly")
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return counts, fmt.Errorf("purge namespace: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var nsID int64
	err = tx.QueryRow(ctx, `SELECT id FROM namespaces WHERE slug = $1 FOR UPDATE`, slug).Scan(&nsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return counts, ErrNamespaceNotFound
	}
	if err != nil {
		return counts, fmt.Errorf("purge namespace: load: %w", err)
	}

	var children int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM namespaces WHERE parent_id = $1`, nsID,
	).Scan(&children); err != nil {
		return counts, fmt.Errorf("purge namespace: count children: %w", err)
	}
	if children > 0 {
		return counts, fmt.Errorf("namespace %q has %d child namespaces; purge those first", slug, children)
	}

	if err := tx.QueryRow(ctx,
		`SELECT
		   (SELECT count(*) FROM episodes WHERE namespace_id = $1),
		   (SELECT count(*) FROM facts WHERE namespace_id = $1)`, nsID,
	).Scan(&counts.Episodes, &counts.Facts); err != nil {
		return counts, fmt.Errorf("purge namespace: count: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM embedding_cache WHERE text IN (
		   SELECT content FROM episodes WHERE namespace_id = $1
		   UNION
		   SELECT content FROM facts WHERE namespace_id = $1)`, nsID,
	).Scan(&counts.Embeddings); err != nil {
		return counts, fmt.Errorf("purge namespace: count cache: %w", err)
	}

	if opts.DryRun {
		return counts, tx.Rollback(ctx)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM embedding_cache WHERE text IN (
		   SELECT content FROM episodes WHERE namespace_id = $1
		   UNION
		   SELECT content FROM facts WHERE namespace_id = $1)`, nsID,
	); err != nil {
		return counts, fmt.Errorf("purge namespace: clear embedding cache: %w", err)
	}
	if err := logPurge(ctx, tx, nsID, "namespaces", nsID, opts, counts); err != nil {
		return counts, err
	}
	// Everything else (facts, episodes, relationships, patterns, contexts,
	// progress, contradictions, causal links, hypotheses, goals, failures,
	// feedback, mental models, namespace config, retractions) hangs off the
	// namespace with ON DELETE CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM namespaces WHERE id = $1`, nsID); err != nil {
		return counts, fmt.Errorf("purge namespace: delete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return counts, fmt.Errorf("purge namespace: commit: %w", err)
	}
	return counts, nil
}

// PurgeLogEntry is one row of the purge audit trail.
type PurgeLogEntry struct {
	ID          int64          `json:"id"`
	NamespaceID *int64         `json:"namespace_id,omitempty"`
	TargetTable string         `json:"target_table"`
	TargetID    int64          `json:"target_id"`
	Reason      string         `json:"reason"`
	RequestedBy string         `json:"requested_by"`
	Details     map[string]int `json:"details,omitempty"`
	PurgedAt    string         `json:"purged_at"`
}

// PurgeLog returns the most recent purge records, newest first. It exists so
// "did someone delete this?" has an answer; it never returns the purged
// content, because a purge means there is none.
func (b *Brain) PurgeLog(ctx context.Context, limit int) ([]PurgeLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, target_table, target_id, reason, requested_by,
		        coalesce(details, '{}'::jsonb), purged_at
		 FROM purge_log ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("purge log: %w", err)
	}
	defer rows.Close()

	out := []PurgeLogEntry{}
	for rows.Next() {
		var e PurgeLogEntry
		var details []byte
		var purgedAt time.Time
		if err := rows.Scan(&e.ID, &e.NamespaceID, &e.TargetTable, &e.TargetID,
			&e.Reason, &e.RequestedBy, &details, &purgedAt); err != nil {
			return nil, fmt.Errorf("purge log: scan: %w", err)
		}
		if len(details) > 0 {
			_ = json.Unmarshal(details, &e.Details)
		}
		e.PurgedAt = purgedAt.UTC().Format(time.RFC3339)
		out = append(out, e)
	}
	return out, rows.Err()
}

func logPurge(ctx context.Context, tx pgx.Tx, nsID int64, table string, id int64, opts PurgeOptions, counts PurgeCounts) error {
	var details []byte
	if !counts.empty() {
		details, _ = json.Marshal(counts)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO purge_log (namespace_id, target_table, target_id, reason, requested_by, details)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		nsID, table, id, opts.Reason, opts.Actor, details,
	); err != nil {
		return fmt.Errorf("purge: write log: %w", err)
	}
	return nil
}

func countRetractions(ctx context.Context, tx pgx.Tx, targetType string, id int64) (int, error) {
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM retractions WHERE target_type = $1 AND target_id = $2`,
		targetType, id,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("purge: count retractions: %w", err)
	}
	return n, nil
}

func deleteRetractions(ctx context.Context, tx pgx.Tx, targetType string, id int64) (int, error) {
	tag, err := tx.Exec(ctx,
		`DELETE FROM retractions WHERE target_type = $1 AND target_id = $2`, targetType, id)
	if err != nil {
		return 0, fmt.Errorf("purge: delete retractions: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func factTargetIDs(t []factTarget) []int64 {
	ids := make([]int64, 0, len(t))
	for _, f := range t {
		ids = append(ids, f.id)
	}
	return ids
}
