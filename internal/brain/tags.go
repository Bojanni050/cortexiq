// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
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

// tagWhere appends a tag filter to a WHERE clause that already uses positional
// placeholders, returning the clause and the extended argument list. Every
// requested tag must be present (AND), matching recall.
func tagWhere(where string, args []any, tags []string) (string, []any) {
	if len(tags) == 0 {
		return where, args
	}
	args = append(args, tags)
	return fmt.Sprintf("%s AND tags @> $%d", where, len(args)), args
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
// some namespaces and to memories stored since a moment. It exists so an agent
// can reuse the existing vocabulary instead of inventing "bug", "Bug" and "bugs"
// in the same store — the same reason list_namespaces exists. An empty namespace
// list means all namespaces; a nil since means all time.
//
// The since bound is on created_at, not occurred_at: reporting asks "what did we
// record in this period", and occurred_at may be any moment the writer chose.
func (b *Brain) ListTags(ctx context.Context, namespaces []string, since *time.Time, limit int) ([]TagCount, error) {
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
		       AND ($2::timestamptz IS NULL OR created_at >= $2)
		    UNION ALL
		    SELECT unnest(tags) AS tag, false AS is_fact
		      FROM episodes
		     WHERE deleted_at IS NULL AND cardinality(tags) > 0
		       AND ($1::bigint[] IS NULL OR namespace_id = ANY($1))
		       AND ($2::timestamptz IS NULL OR created_at >= $2)
		) s
		GROUP BY tag
		ORDER BY count(*) DESC, tag
		LIMIT $3`,
		nsIDs, since, limit)
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

// TagTrend is one tag's count inside one month: how much of that tag was
// recorded then. It answers "how does this tag develop over time", which a
// single since-bounded total cannot.
type TagTrend struct {
	Tag      string `json:"tag"`
	Month    string `json:"month"` // YYYY-MM, UTC
	Facts    int    `json:"facts"`
	Episodes int    `json:"episodes"`
	Total    int    `json:"total"`
}

// TagTimeline returns per-month counts for the tags in use, oldest month first.
// months is how far back it looks (counting the current month); zero means 12.
// An empty namespace list means all namespaces; an empty tag list means every
// tag. Months without memories are left out rather than returned as zero rows,
// so a caller that wants an unbroken axis has to fill the gaps itself.
func (b *Brain) TagTimeline(ctx context.Context, namespaces []string, tags []string, months int) ([]TagTrend, error) {
	if months <= 0 {
		months = 12
	}
	if months > 60 {
		months = 60
	}
	tagFilter, err := NormalizeTags(tags)
	if err != nil {
		return nil, err
	}
	var nsIDs []int64
	if len(namespaces) > 0 {
		nsIDs, err = b.resolveNamespaceIDs(ctx, namespaces)
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)

	rows, err := b.pool.Query(ctx, `
		SELECT to_char(month, 'YYYY-MM') AS month, tag,
		       count(*) FILTER (WHERE is_fact)::int AS facts,
		       count(*) FILTER (WHERE NOT is_fact)::int AS episodes
		FROM (
		    SELECT unnest(tags) AS tag, date_trunc('month', created_at) AS month, true AS is_fact
		      FROM facts
		     WHERE deleted_at IS NULL AND cardinality(tags) > 0
		       AND ($1::bigint[] IS NULL OR namespace_id = ANY($1))
		       AND created_at >= $2
		    UNION ALL
		    SELECT unnest(tags) AS tag, date_trunc('month', created_at) AS month, false AS is_fact
		      FROM episodes
		     WHERE deleted_at IS NULL AND cardinality(tags) > 0
		       AND ($1::bigint[] IS NULL OR namespace_id = ANY($1))
		       AND created_at >= $2
		) s
		WHERE ($3::text[] IS NULL OR tag = ANY($3))
		GROUP BY month, tag
		ORDER BY month, count(*) DESC, tag`,
		nsIDs, since, tagFilter)
	if err != nil {
		return nil, fmt.Errorf("brain: tag timeline: %w", err)
	}
	defer rows.Close()

	out := []TagTrend{}
	for rows.Next() {
		var t TagTrend
		if err := rows.Scan(&t.Month, &t.Tag, &t.Facts, &t.Episodes); err != nil {
			return nil, fmt.Errorf("brain: tag timeline: %w", err)
		}
		t.Total = t.Facts + t.Episodes
		out = append(out, t)
	}
	return out, rows.Err()
}
