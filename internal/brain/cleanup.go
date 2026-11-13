package brain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	defaultCleanupLimit = 100
	maxCleanupLimit     = 500
	// defaultCleanupConfidence is the line below which a relation counts as weak.
	defaultCleanupConfidence = 0.8
)

// CleanupOptions narrows what Cleanup returns.
type CleanupOptions struct {
	// MinConfidence marks relations below it as weak (default 0.8).
	MinConfidence float32
	// Limit caps every list (default 100, max 500). Totals are never capped.
	Limit int
}

// Kinds of duplicate group, from the safest merge to the least safe one.
const (
	// DupCase: the spellings differ only in capitals. The graph already folds these
	// into one entity, but facts and relations still carry every spelling.
	DupCase = "case"
	// DupPunctuation: they differ in hyphens, slashes, spaces or similar.
	DupPunctuation = "punctuation"
	// DupQualifier: one carries a "(note)" or a leading "the". Often the same
	// thing, not always: "Foundation" and "Foundation (Gaia)" may be two things.
	DupQualifier = "qualifier"
)

// EntityVariant is one spelling of an entity and how often it is used.
type EntityVariant struct {
	Name      string `json:"name"`
	Facts     int    `json:"facts"`
	Relations int    `json:"relations"`
}

// DuplicateGroup is a set of spellings that probably name the same entity.
type DuplicateGroup struct {
	Kind      string          `json:"kind"`
	Variants  []EntityVariant `json:"variants"`
	Facts     int             `json:"facts"`
	Relations int             `json:"relations"`
}

// Island is an entity that facts talk about but no relation mentions.
type Island struct {
	Entity string `json:"entity"`
	Facts  int    `json:"facts"`
}

// WeakRelation is a relation below the confidence line.
type WeakRelation struct {
	ID         int64   `json:"id"`
	From       string  `json:"from"`
	Type       string  `json:"type"`
	To         string  `json:"to"`
	Confidence float32 `json:"confidence"`
	FactID     *int64  `json:"fact_id,omitempty"`
}

// UnmappedType is a relation wording without a normalised type. Spellings that
// only differ in punctuation ("pushed to", "pushed_to") count as one wording.
type UnmappedType struct {
	Type      string   `json:"type"`
	Count     int      `json:"count"`
	Spellings []string `json:"spellings"`
}

// CleanupTotals sizes the whole memory, so the lists can say "100 of 568".
type CleanupTotals struct {
	FactEntities      int     `json:"fact_entities"`
	RelationEntities  int     `json:"relation_entities"`
	Relations         int     `json:"relations"`
	UnmappedRelations int     `json:"unmapped_relations"`
	MinConfidence     float32 `json:"min_confidence"`
}

// Cleanup is the clean-up report returned to the UI. It only reads; nothing in it
// changes data.
type Cleanup struct {
	Duplicates     []DuplicateGroup `json:"duplicates"`
	DuplicateTotal int              `json:"duplicate_total"`
	Islands        []Island         `json:"islands"`
	IslandTotal    int              `json:"island_total"`
	Weak           []WeakRelation   `json:"weak"`
	WeakTotal      int              `json:"weak_total"`
	Unmapped       []UnmappedType   `json:"unmapped"`
	UnmappedTotal  int              `json:"unmapped_total"`
	Totals         CleanupTotals    `json:"totals"`
}

// Cleanup reports where the memory is untidy: entities spelled several ways,
// entities no relation connects, weak relations and relation wordings that were
// never normalised. Namespaces include their descendants.
func (b *Brain) Cleanup(ctx context.Context, namespaceSlugs []string, opts CleanupOptions) (*Cleanup, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	if opts.MinConfidence <= 0 || opts.MinConfidence > 1 {
		opts.MinConfidence = defaultCleanupConfidence
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultCleanupLimit
	}
	if opts.Limit > maxCleanupLimit {
		opts.Limit = maxCleanupLimit
	}

	factCounts, err := b.entityCounts(ctx,
		`SELECT btrim(entity), count(*) FROM facts
		 WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND btrim(coalesce(entity, '')) <> ''
		 GROUP BY 1`, nsIDs)
	if err != nil {
		return nil, fmt.Errorf("cleanup fact entities: %w", err)
	}
	relCounts, err := b.entityCounts(ctx,
		`SELECT e, count(*) FROM (
		   SELECT btrim(from_entity) AS e FROM relationships WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		   UNION ALL
		   SELECT btrim(to_entity) FROM relationships WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		 ) x WHERE e <> '' GROUP BY e`, nsIDs)
	if err != nil {
		return nil, fmt.Errorf("cleanup relation entities: %w", err)
	}

	c := buildCleanup(factCounts, relCounts, opts.Limit)
	c.Totals.MinConfidence = opts.MinConfidence

	if err := b.pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE canonical_type IS NULL)
		 FROM relationships WHERE namespace_id = ANY($1) AND deleted_at IS NULL`, nsIDs,
	).Scan(&c.Totals.Relations, &c.Totals.UnmappedRelations); err != nil {
		return nil, fmt.Errorf("cleanup relation totals: %w", err)
	}

	rows, err := b.pool.Query(ctx,
		`SELECT id, btrim(from_entity), relation_type, btrim(to_entity), confidence, source_fact_id, count(*) OVER()
		 FROM relationships
		 WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND confidence < $2
		 ORDER BY confidence, id LIMIT $3`, nsIDs, opts.MinConfidence, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("cleanup weak relations: %w", err)
	}
	defer rows.Close()
	c.Weak = []WeakRelation{}
	for rows.Next() {
		var w WeakRelation
		if err := rows.Scan(&w.ID, &w.From, &w.Type, &w.To, &w.Confidence, &w.FactID, &c.WeakTotal); err != nil {
			return nil, fmt.Errorf("scan weak relation: %w", err)
		}
		c.Weak = append(c.Weak, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	types, err := b.pool.Query(ctx,
		`SELECT regexp_replace(lower(btrim(relation_type)), '[^[:alnum:]]+', '_', 'g') AS k,
		        count(*), array_agg(DISTINCT btrim(relation_type)), count(*) OVER()
		 FROM relationships
		 WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND canonical_type IS NULL
		 GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT $2`, nsIDs, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("cleanup unmapped types: %w", err)
	}
	defer types.Close()
	c.Unmapped = []UnmappedType{}
	for types.Next() {
		var u UnmappedType
		if err := types.Scan(&u.Type, &u.Count, &u.Spellings, &c.UnmappedTotal); err != nil {
			return nil, fmt.Errorf("scan unmapped type: %w", err)
		}
		sort.Strings(u.Spellings)
		c.Unmapped = append(c.Unmapped, u)
	}
	return c, types.Err()
}

// entityCounts runs a query that returns (name, count) rows.
func (b *Brain) entityCounts(ctx context.Context, query string, nsIDs []int64) (map[string]int, error) {
	rows, err := b.pool.Query(ctx, query, nsIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

// buildCleanup is the pure part of Cleanup: duplicates and islands from how often
// each spelling is used in facts and in relations.
func buildCleanup(factCounts, relCounts map[string]int, limit int) *Cleanup {
	c := &Cleanup{Duplicates: []DuplicateGroup{}, Islands: []Island{}, Weak: []WeakRelation{}, Unmapped: []UnmappedType{}}

	// Group every spelling, from facts or relations, under one key.
	type spelling struct{ facts, rels int }
	all := map[string]*spelling{}
	for n, v := range factCounts {
		s := all[n]
		if s == nil {
			s = &spelling{}
			all[n] = s
		}
		s.facts = v
	}
	for n, v := range relCounts {
		s := all[n]
		if s == nil {
			s = &spelling{}
			all[n] = s
		}
		s.rels = v
	}
	groups := map[string][]string{}
	for n := range all {
		k := cleanupKey(n)
		groups[k] = append(groups[k], n)
	}

	for _, names := range groups {
		if len(names) < 2 {
			continue
		}
		g := DuplicateGroup{Kind: duplicateKind(names)}
		for _, n := range names {
			s := all[n]
			g.Variants = append(g.Variants, EntityVariant{Name: n, Facts: s.facts, Relations: s.rels})
			g.Facts += s.facts
			g.Relations += s.rels
		}
		sort.Slice(g.Variants, func(i, j int) bool {
			a, b := g.Variants[i], g.Variants[j]
			if a.Facts+a.Relations != b.Facts+b.Relations {
				return a.Facts+a.Relations > b.Facts+b.Relations
			}
			return a.Name < b.Name
		})
		c.Duplicates = append(c.Duplicates, g)
	}
	sort.Slice(c.Duplicates, func(i, j int) bool {
		a, b := c.Duplicates[i], c.Duplicates[j]
		if a.Facts+a.Relations != b.Facts+b.Relations {
			return a.Facts+a.Relations > b.Facts+b.Relations
		}
		return a.Variants[0].Name < b.Variants[0].Name
	})
	c.DuplicateTotal = len(c.Duplicates)
	if len(c.Duplicates) > limit {
		c.Duplicates = c.Duplicates[:limit]
	}

	// An island is an entity with facts that no relation mentions. "Mentions" means
	// the same entity under the rule the system itself writes relations with: a fact
	// about "Hermes agent" and a relation about "hermes" are one entity, because
	// NormalizeEntity folds both to "hermes". Comparing raw spellings would call it
	// an island once the relations are normalised and the facts are not.
	inRelation := map[string]bool{}
	for n := range relCounts {
		inRelation[identityKey(n)] = true
	}
	byKey := map[string]*Island{}
	for n, v := range factCounts {
		k := identityKey(n)
		if inRelation[k] {
			continue
		}
		is := byKey[k]
		if is == nil {
			byKey[k] = &Island{Entity: n, Facts: v}
			continue
		}
		// Several spellings of one island: add them up, name it after the busiest.
		if v > is.Facts || (v == is.Facts && n < is.Entity) {
			is.Entity = n
		}
		is.Facts += v
	}
	for _, is := range byKey {
		c.Islands = append(c.Islands, *is)
	}
	sort.Slice(c.Islands, func(i, j int) bool {
		if c.Islands[i].Facts != c.Islands[j].Facts {
			return c.Islands[i].Facts > c.Islands[j].Facts
		}
		return c.Islands[i].Entity < c.Islands[j].Entity
	})
	c.IslandTotal = len(c.Islands)
	if len(c.Islands) > limit {
		c.Islands = c.Islands[:limit]
	}

	factKeys, relKeys := map[string]bool{}, map[string]bool{}
	for n := range factCounts {
		factKeys[entityKey(n)] = true
	}
	for n := range relCounts {
		relKeys[entityKey(n)] = true
	}
	c.Totals.FactEntities, c.Totals.RelationEntities = len(factKeys), len(relKeys)
	return c
}

// identityKey is the entity a name stands for under the system's own rule
// (NormalizeEntity). A name that rule rejects as a description keeps its folded
// spelling, so it is still compared with itself.
func identityKey(name string) string {
	if canonical, ok := NormalizeEntity(name); ok {
		return canonical
	}
	return entityKey(name)
}

// cleanupKey is what makes two spellings the same entity: capitals, punctuation,
// a trailing "(note)" and a leading "the" are ignored. A name that would shrink to
// nothing (only symbols) keeps its own lower-cased form.
func cleanupKey(s string) string {
	k := stripQualifier(entityKey(s))
	var sb strings.Builder
	for _, r := range k {
		// + and # carry meaning ("C++", "C#", "F#"); every other symbol is a separator.
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '#' {
			sb.WriteRune(r)
		}
	}
	if sb.Len() == 0 {
		return entityKey(s)
	}
	return sb.String()
}

// duplicateKind says how far apart the spellings of a group are.
func duplicateKind(names []string) string {
	folded := map[string]bool{}
	qualified := false
	for _, n := range names {
		k := entityKey(n)
		folded[k] = true
		if stripQualifier(k) != k {
			qualified = true
		}
	}
	switch {
	case len(folded) == 1:
		return DupCase
	case qualified:
		return DupQualifier
	default:
		return DupPunctuation
	}
}

// stripQualifier drops a trailing "(note)" and a leading "the " from a folded name.
func stripQualifier(k string) string {
	if i := strings.LastIndex(k, "("); i > 0 && strings.HasSuffix(k, ")") {
		k = strings.TrimSpace(k[:i])
	}
	return strings.TrimPrefix(k, "the ")
}
