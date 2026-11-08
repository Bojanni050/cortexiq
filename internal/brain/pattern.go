// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
)

// ErrPatternNotFound is what DeletePattern answers for an unknown id.
var ErrPatternNotFound = fmt.Errorf("brain: pattern not found")

// CountPatterns counts what QueryPatterns would return for the same namespaces,
// ignoring paging. Without it the dashboard cannot page through them honestly.
func (b *Brain) CountPatterns(ctx context.Context, namespaceSlugs []string) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	return b.countWhere(ctx, "patterns", "namespace_id = ANY($1) AND deleted_at IS NULL", nsIDs)
}

// DeletePattern soft-deletes one pattern. They are written by consolidation and
// were write-only until now: if they turn out to be fact paraphrases rather than
// regularities, the only honest thing is to be able to remove them.
func (b *Brain) DeletePattern(ctx context.Context, id int64) error {
	if err := requireWrite(ctx); err != nil {
		return err
	}
	tag, err := b.pool.Exec(ctx,
		`UPDATE patterns SET deleted_at = now(), updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete pattern: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPatternNotFound
	}
	return nil
}

// storePattern saves one derived pattern. A statement that already lives in the
// namespace is reinforced, not duplicated: the same regularity seen again is
// better evidence, not a second row. The unique index on
// (namespace_id, content_key) makes that true even when two runs race, and the
// merge keeps what each sighting contributed — the stronger confidence, the
// higher coherence and the union of the sources.
//
// sources is the set of things the statement is about (empty for a pattern the
// window stage derived from facts alone). The sweep uses it to recognise the same
// regularity in another wording, which the content key cannot see.
//
// It reports whether the row was new, so a run can say "3 new, 4 reinforced"
// instead of pretending every derivation was an insight.
func (b *Brain) storePattern(ctx context.Context, nsID int64, content string, confidence float32, factIDs, relIDs []int64, sources []string, coherence float32) (inserted bool, err error) {
	// A nil slice reaches the driver as NULL and the id columns are NOT NULL: the
	// row would be rejected. This is the regression the old stage test guarded,
	// now guarded where the row is actually written.
	if factIDs == nil {
		factIDs = []int64{}
	}
	if relIDs == nil {
		relIDs = []int64{}
	}
	if sources == nil {
		sources = []string{}
	}
	var created bool
	err = b.pool.QueryRow(ctx,
		`INSERT INTO patterns (namespace_id, content, confidence, source_fact_ids, source_rel_ids, source_entities, coherence_score)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (namespace_id, content_key) WHERE deleted_at IS NULL
		 DO UPDATE SET
		   confidence = GREATEST(patterns.confidence, EXCLUDED.confidence),
		   coherence_score = GREATEST(patterns.coherence_score, EXCLUDED.coherence_score),
		   source_fact_ids = COALESCE((
		     SELECT array_agg(DISTINCT x)
		     FROM unnest(patterns.source_fact_ids || EXCLUDED.source_fact_ids) AS x
		   ), '{}'),
		   source_rel_ids = COALESCE((
		     SELECT array_agg(DISTINCT x)
		     FROM unnest(patterns.source_rel_ids || EXCLUDED.source_rel_ids) AS x
		   ), '{}'),
		   source_entities = COALESCE((
		     SELECT array_agg(DISTINCT x)
		     FROM unnest(patterns.source_entities || EXCLUDED.source_entities) AS x
		   ), '{}'),
		   updated_at = now()
		 RETURNING (xmax = 0)`,
		nsID, content, confidence, factIDs, relIDs, sources, coherence,
	).Scan(&created)
	if err != nil {
		return false, fmt.Errorf("store pattern: %w", err)
	}
	return created, nil
}
