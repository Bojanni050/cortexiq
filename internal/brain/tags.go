// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// MaxTagsPerMemory caps how many tags one memory may carry. A handful is enough
// to describe a memory; more usually means the writer is using tags as a
// namespace, which is what namespaces are for.
const MaxTagsPerMemory = 8

// maxTagLength bounds a single tag so it stays readable in a UI chip and in a
// report column.
const maxTagLength = 32

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// NormalizeTags lowercases, trims and de-duplicates tags, and rejects anything
// that would make the vocabulary drift. Without a single normal form "Bug",
// "bug " and "bugs" become three tags that fragment every filter and every
// report, and there is no way to clean that up afterwards without a manual
// migration. Inner whitespace becomes '-', the way people write tags anyway.
//
// The order of the first occurrence is kept so callers see their own input
// order, and an empty input returns nil (not an empty slice) so the insert can
// fall back to the column default.
func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		tag = strings.Join(strings.Fields(tag), "-")
		if tag == "" {
			continue
		}
		if len(tag) > maxTagLength {
			return nil, fmt.Errorf("brain: tag %q is longer than %d characters", tag, maxTagLength)
		}
		if !tagPattern.MatchString(tag) {
			return nil, fmt.Errorf("brain: invalid tag %q: use lowercase letters, digits, '-' or '_', starting with a letter or digit", tag)
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) > MaxTagsPerMemory {
		return nil, fmt.Errorf("brain: too many tags (%d), at most %d per memory", len(out), MaxTagsPerMemory)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// tagsOrEmpty keeps a NULL out of a NOT NULL array column: NormalizeTags returns
// nil when nothing is left, which pgx would encode as SQL NULL.
func tagsOrEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

// TagCount is one entry of the tag vocabulary: how many live facts and episodes
// carry the tag. A tag can appear on facts only (consolidated knowledge) or on
// episodes only (raw memories that have not been consolidated yet).
type TagCount struct {
	Tag      string `json:"tag"`
	Facts    int    `json:"facts"`
	Episodes int    `json:"episodes"`
	Total    int    `json:"total"`
}

// ListTags returns the tags in use, most used first, optionally restricted to
// some namespaces. It exists so an agent can reuse the existing vocabulary
// instead of inventing "bug", "Bug" and "bugs" in the same store — the same
// reason list_namespaces exists. An empty namespace list means all namespaces.
func (b *Brain) ListTags(ctx context.Context, namespaces []string, limit int) ([]TagCount, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var nsIDs []int64
	if len(namespaces) > 0 {
		var err error
		nsIDs, err = b.resolveNamespaceIDs(ctx, namespaces)
		if err != nil {
			return nil, err
		}
	}
	rows, err := b.pool.Query(ctx, `
		SELECT tag,
		       count(*) FILTER (WHERE is_fact)::int AS facts,
		       count(*) FILTER (WHERE NOT is_fact)::int AS episodes
		FROM (
		    SELECT unnest(tags) AS tag, true AS is_fact
		      FROM facts
		     WHERE deleted_at IS NULL AND cardinality(tags) > 0
		       AND ($1::bigint[] IS NULL OR namespace_id = ANY($1))
		    UNION ALL
		    SELECT unnest(tags) AS tag, false AS is_fact
		      FROM episodes
		     WHERE deleted_at IS NULL AND cardinality(tags) > 0
		       AND ($1::bigint[] IS NULL OR namespace_id = ANY($1))
		) s
		GROUP BY tag
		ORDER BY count(*) DESC, tag
		LIMIT $2`,
		nsIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("brain: list tags: %w", err)
	}
	defer rows.Close()

	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Facts, &t.Episodes); err != nil {
			return nil, fmt.Errorf("brain: list tags: %w", err)
		}
		t.Total = t.Facts + t.Episodes
		out = append(out, t)
	}
	return out, rows.Err()
}
