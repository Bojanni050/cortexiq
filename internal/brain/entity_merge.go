package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrMergeInvalid means a merge request names no spelling to keep or no spelling
// to fold into it.
var ErrMergeInvalid = errors.New("brain: merge needs a spelling to keep and at least one other")

const maxEntityNameLen = 200

// MergeReport says what a merge changed, or in a dry run what it would change.
type MergeReport struct {
	Keep       string   `json:"keep"`
	Variants   []string `json:"variants"`
	Facts      int      `json:"facts"`
	Relations  int      `json:"relations"`
	Duplicates int      `json:"duplicates"`
	SelfLoops  int      `json:"self_loops"`
	DryRun     bool     `json:"dry_run"`
}

// MergeEntities makes every spelling in variants read as keep: the entity of each
// fact and both ends of each relation that carry one of them are rewritten.
// Matching ignores capitals and surrounding spaces, like the rest of the entity
// code. Afterwards the relations that became identical collapse into the most
// confident one, and a relation that now points from an entity to itself is
// removed (both as soft deletes). Fact texts, embeddings and history rows such as
// contradictions are left as they were.
//
// The work runs in one transaction. A dry run does all of it and rolls back, so
// the counts are exactly what the real run would do.
func (b *Brain) MergeEntities(ctx context.Context, namespaceSlugs []string, keep string, variants []string, dryRun bool) (*MergeReport, error) {
	if !dryRun {
		if err := requireWrite(ctx); err != nil {
			return nil, err
		}
	}
	keep = strings.TrimSpace(keep)
	if keep == "" || utf8.RuneCountInString(keep) > maxEntityNameLen {
		return nil, ErrMergeInvalid
	}
	keepKey := strings.ToLower(keep)
	seen := map[string]bool{}
	var keys, names []string
	for _, v := range variants {
		v = strings.TrimSpace(v)
		if v == "" || utf8.RuneCountInString(v) > maxEntityNameLen {
			return nil, ErrMergeInvalid
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
		names = append(names, v)
	}
	if len(keys) == 0 {
		return nil, ErrMergeInvalid
	}

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("merge entities: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a rollback after commit is a no-op

	rep := &MergeReport{Keep: keep, Variants: names, DryRun: dryRun}

	tag, err := tx.Exec(ctx,
		`UPDATE facts SET entity = $3
		  WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND entity IS NOT NULL
		    AND lower(btrim(entity)) = ANY($2) AND entity <> $3`, nsIDs, keys, keep)
	if err != nil {
		return nil, fmt.Errorf("merge fact entities: %w", err)
	}
	rep.Facts = int(tag.RowsAffected())

	for _, column := range []string{"from_entity", "to_entity"} {
		tag, err := tx.Exec(ctx,
			`UPDATE relationships SET `+column+` = $3
			  WHERE namespace_id = ANY($1) AND deleted_at IS NULL
			    AND lower(btrim(`+column+`)) = ANY($2) AND `+column+` <> $3`, nsIDs, keys, keep)
		if err != nil {
			return nil, fmt.Errorf("merge relation entities: %w", err)
		}
		rep.Relations += int(tag.RowsAffected())
	}

	// A relation between two spellings of one thing is now a relation from the
	// thing to itself, which says nothing.
	tag, err = tx.Exec(ctx,
		`UPDATE relationships SET deleted_at = now()
		  WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		    AND lower(btrim(from_entity)) = $2 AND lower(btrim(to_entity)) = $2`, nsIDs, keepKey)
	if err != nil {
		return nil, fmt.Errorf("merge self loops: %w", err)
	}
	rep.SelfLoops = int(tag.RowsAffected())

	// Only rows that touch the kept entity can have become duplicates, so the
	// rest of the graph is not looked at.
	tag, err = tx.Exec(ctx,
		`WITH ranked AS (
		   SELECT id, row_number() OVER (
		     PARTITION BY namespace_id, lower(btrim(from_entity)), lower(btrim(to_entity)),
		                  COALESCE(canonical_type, lower(btrim(relation_type)))
		     ORDER BY confidence DESC, id) AS rn
		   FROM relationships
		   WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		     AND (lower(btrim(from_entity)) = $2 OR lower(btrim(to_entity)) = $2)
		 )
		 UPDATE relationships r SET deleted_at = now()
		 FROM ranked WHERE r.id = ranked.id AND ranked.rn > 1`, nsIDs, keepKey)
	if err != nil {
		return nil, fmt.Errorf("merge duplicate relations: %w", err)
	}
	rep.Duplicates = int(tag.RowsAffected())

	if dryRun {
		return rep, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("merge entities: commit: %w", err)
	}
	return rep, nil
}
