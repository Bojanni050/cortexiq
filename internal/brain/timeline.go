package brain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultTimelineRows = 12
	maxTimelineRows     = 40
	// maxTimelineFacts bounds how many facts one timeline request reads.
	maxTimelineFacts = 20000
)

// TimelineOptions narrows what FactTimeline returns.
type TimelineOptions struct {
	// Rows is how many entities get a row of their own (default 12, max 40).
	// Every other fact lands in one trailing "other" row.
	Rows int
}

// TimelineFact is one fact as a dot on the timeline.
type TimelineFact struct {
	ID int64 `json:"id"`
	// At is when the memory learned the fact (created_at). valid_from is not used:
	// it only repeats created_at, so it says nothing about when the fact was true.
	At time.Time `json:"at"`
	// ValidUntil is set when a newer fact replaced this one.
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	// SupersededBy is the fact that replaced it, when a contradiction records which.
	SupersededBy *int64 `json:"superseded_by,omitempty"`
}

// TimelineRow is one entity and its facts, oldest first.
type TimelineRow struct {
	// Entity is the display name; empty for the trailing "other" row.
	Entity string         `json:"entity"`
	Total  int            `json:"total"`
	Facts  []TimelineFact `json:"facts"`
}

// Timeline is the fact timeline returned to the UI.
type Timeline struct {
	Rows []TimelineRow `json:"rows"`
	From *time.Time    `json:"from,omitempty"`
	To   *time.Time    `json:"to,omitempty"`
	// Total counts every fact on the timeline; Superseded those that were replaced.
	Total      int `json:"total"`
	Superseded int `json:"superseded"`
	// Truncated is true when the fact cap cut the timeline short.
	Truncated bool `json:"truncated"`
}

// timelineRow is one facts row as read from the database.
type timelineRow struct {
	ID           int64
	CreatedAt    time.Time
	ValidUntil   *time.Time
	SupersededBy *int64
	Entity       string
}

// FactTimeline lays the live facts of the given namespaces (each includes its
// descendants) out over time, one row per entity.
func (b *Brain) FactTimeline(ctx context.Context, namespaceSlugs []string, opts TimelineOptions) (*Timeline, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	rows, err := b.pool.Query(ctx,
		`SELECT f.id, f.created_at, f.valid_until,
		        (SELECT c.new_fact_id FROM contradictions c WHERE c.old_fact_id = f.id ORDER BY c.id DESC LIMIT 1),
		        COALESCE(btrim(f.entity), '')
		 FROM facts f
		 WHERE f.namespace_id = ANY($1) AND f.deleted_at IS NULL
		 ORDER BY f.created_at, f.id LIMIT $2`, nsIDs, maxTimelineFacts+1)
	if err != nil {
		return nil, fmt.Errorf("query timeline: %w", err)
	}
	defer rows.Close()

	var all []timelineRow
	for rows.Next() {
		var r timelineRow
		if err := rows.Scan(&r.ID, &r.CreatedAt, &r.ValidUntil, &r.SupersededBy, &r.Entity); err != nil {
			return nil, fmt.Errorf("scan timeline row: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	truncated := len(all) > maxTimelineFacts
	if truncated {
		all = all[:maxTimelineFacts]
	}
	tl := buildTimeline(all, opts)
	tl.Truncated = truncated
	return tl, nil
}

// buildTimeline is the pure part of FactTimeline: group facts by entity (ignoring
// case and surrounding spaces), give the busiest entities a row each and fold the
// rest, and facts without an entity, into one trailing row.
func buildTimeline(rows []timelineRow, opts TimelineOptions) *Timeline {
	if opts.Rows <= 0 {
		opts.Rows = defaultTimelineRows
	}
	if opts.Rows > maxTimelineRows {
		opts.Rows = maxTimelineRows
	}

	type group struct {
		spellings map[string]int
		facts     []TimelineFact
	}
	groups := map[string]*group{}
	var other []TimelineFact
	tl := &Timeline{Rows: []TimelineRow{}}

	for _, r := range rows {
		f := TimelineFact{ID: r.ID, At: r.CreatedAt, ValidUntil: r.ValidUntil, SupersededBy: r.SupersededBy}
		tl.Total++
		if r.ValidUntil != nil {
			tl.Superseded++
		}
		at := r.CreatedAt
		if tl.From == nil || at.Before(*tl.From) {
			tl.From = &at
		}
		if tl.To == nil || at.After(*tl.To) {
			tl.To = &at
		}
		key := entityKey(r.Entity)
		if key == "" {
			other = append(other, f)
			continue
		}
		g := groups[key]
		if g == nil {
			g = &group{spellings: map[string]int{}}
			groups[key] = g
		}
		g.spellings[strings.TrimSpace(r.Entity)]++
		g.facts = append(g.facts, f)
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	// Busiest first; ties by name so the order does not change between requests.
	sort.Slice(keys, func(i, j int) bool {
		a, c := len(groups[keys[i]].facts), len(groups[keys[j]].facts)
		if a != c {
			return a > c
		}
		return keys[i] < keys[j]
	})

	byTime := func(fs []TimelineFact) {
		sort.SliceStable(fs, func(i, j int) bool {
			if !fs[i].At.Equal(fs[j].At) {
				return fs[i].At.Before(fs[j].At)
			}
			return fs[i].ID < fs[j].ID
		})
	}
	for i, k := range keys {
		g := groups[k]
		byTime(g.facts)
		if i >= opts.Rows {
			other = append(other, g.facts...)
			continue
		}
		tl.Rows = append(tl.Rows, TimelineRow{Entity: commonSpelling(g.spellings), Total: len(g.facts), Facts: g.facts})
	}
	if len(other) > 0 {
		byTime(other)
		tl.Rows = append(tl.Rows, TimelineRow{Total: len(other), Facts: other})
	}
	return tl
}

// commonSpelling picks the most used spelling of an entity, ties by name.
func commonSpelling(counts map[string]int) string {
	best, n := "", -1
	for s, c := range counts {
		if c > n || (c == n && s < best) {
			best, n = s, c
		}
	}
	return best
}
