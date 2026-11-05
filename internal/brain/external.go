// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgconn"
)

// Limits on the caller-owned identifiers a memory may carry. They are opaque to
// CortexIQ, so the bounds only keep a runaway writer from filling a column.
const (
	MaxExternalKeyLength = 200
	MaxExternalRefs      = 16
	MaxExternalRefLength = 200
)

var (
	// ErrExternalKeyExists means the key already holds different content in this
	// namespace. Replacing it is a deliberate act, so the caller says so with
	// Supersedes instead of a plain write silently rewriting history.
	ErrExternalKeyExists = errors.New("brain: external_key already holds different content — pass supersedes to replace it")
	// ErrSupersedesNeedsKey means Supersedes was set without an ExternalKey:
	// without a key there is nothing to supersede.
	ErrSupersedesNeedsKey = errors.New("brain: supersedes needs an external_key")
	// ErrInvalidExternalKey means the key or a ref is empty-ish, too long, or
	// carries whitespace or control characters.
	ErrInvalidExternalKey = errors.New("brain: invalid external_key or external_refs")
)

// validExternalToken reports whether s is a usable identifier: non-empty after
// trimming, within max runes, and free of whitespace and control characters so
// it survives a URL, a log line and a CSV cell unchanged.
func validExternalToken(s string, max int) bool {
	if s == "" || len([]rune(s)) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// NormalizeExternalKey trims the key and rejects one that cannot be addressed
// reliably. An empty key is valid and means "not keyed".
func NormalizeExternalKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	if !validExternalToken(key, MaxExternalKeyLength) {
		return "", fmt.Errorf("%w: key must be 1-%d characters without whitespace or control characters", ErrInvalidExternalKey, MaxExternalKeyLength)
	}
	return key, nil
}

// NormalizeExternalRefs trims, drops empties and de-duplicates refs, keeping the
// order of first occurrence. No refs returns nil so the insert can store an
// empty array rather than NULL.
func NormalizeExternalRefs(refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(refs))
	out := make([]string, 0, len(refs))
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		if !validExternalToken(ref, MaxExternalRefLength) {
			return nil, fmt.Errorf("%w: a ref must be 1-%d characters without whitespace or control characters", ErrInvalidExternalKey, MaxExternalRefLength)
		}
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	if len(out) > MaxExternalRefs {
		return nil, fmt.Errorf("%w: too many refs (%d), at most %d per memory", ErrInvalidExternalKey, len(out), MaxExternalRefs)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// refsOrEmpty keeps a NULL out of a NOT NULL array column, like tagsOrEmpty.
func refsOrEmpty(refs []string) []string {
	if refs == nil {
		return []string{}
	}
	return refs
}

// isUniqueViolation reports a Postgres unique_violation (SQLSTATE 23505), which
// for a keyed write means another writer took the key between our check and our
// insert.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// sourceRefs is the union of the external refs on the given source episodes,
// sorted for a deterministic array. A fact keeps the pointers of what it was
// derived from, the same way it keeps tags. Best effort: refs are annotation and
// must never fail consolidation.
func (b *Brain) sourceRefs(ctx context.Context, episodeIDs []int64) []string {
	refs := []string{}
	if len(episodeIDs) == 0 {
		return refs
	}
	if err := b.pool.QueryRow(ctx,
		`SELECT COALESCE(array_agg(DISTINCT r ORDER BY r), '{}')
		   FROM episodes e, unnest(e.external_refs) AS r
		  WHERE e.id = ANY($1)`, episodeIDs,
	).Scan(&refs); err != nil {
		return []string{}
	}
	return refs
}

// attachProvenance fills ExternalKey and ExternalRefs on recall results with one
// query per table. It runs after the final cut, so it touches at most `limit`
// rows, and it never fails a recall: a lookup error leaves the fields empty.
func (b *Brain) attachProvenance(ctx context.Context, results []RecallResult) {
	var episodeIDs, factIDs []int64
	for _, r := range results {
		switch r.Type {
		case "episode":
			episodeIDs = append(episodeIDs, r.ID)
		case "fact":
			factIDs = append(factIDs, r.ID)
		}
	}

	type prov struct {
		key       string
		refs      []string
		origin    string
		originRef string
		strength  float32
	}
	episodes := make(map[int64]prov, len(episodeIDs))
	facts := make(map[int64]prov, len(factIDs))

	if len(episodeIDs) > 0 {
		rows, err := b.pool.Query(ctx,
			`SELECT id, COALESCE(external_key, ''), external_refs FROM episodes WHERE id = ANY($1)`, episodeIDs)
		if err == nil {
			for rows.Next() {
				var id int64
				var p prov
				if rows.Scan(&id, &p.key, &p.refs) == nil {
					episodes[id] = p
				}
			}
			rows.Close()
		}
	}
	if len(factIDs) > 0 {
		rows, err := b.pool.Query(ctx,
			`SELECT id, external_refs, COALESCE(origin, ''), COALESCE(origin_ref, ''), strength FROM facts WHERE id = ANY($1)`, factIDs)
		if err == nil {
			for rows.Next() {
				var id int64
				var p prov
				if rows.Scan(&id, &p.refs, &p.origin, &p.originRef, &p.strength) == nil {
					facts[id] = p
				}
			}
			rows.Close()
		}
	}

	for i := range results {
		var p prov
		var ok bool
		switch results[i].Type {
		case "episode":
			p, ok = episodes[results[i].ID]
		case "fact":
			p, ok = facts[results[i].ID]
		}
		if !ok {
			continue
		}
		results[i].ExternalKey = p.key
		results[i].Origin = p.origin
		results[i].OriginRef = p.originRef
		if results[i].Type == "fact" && p.strength < 1 {
			results[i].Strength = p.strength
		}
		if len(p.refs) > 0 {
			results[i].ExternalRefs = p.refs
		}
	}
}
