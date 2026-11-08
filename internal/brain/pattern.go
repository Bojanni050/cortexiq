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
