package brain

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pgvector/pgvector-go"
)

// dedupDecision is what to do with a newly synthesized fact, given how similar it
// is to the closest existing active fact.
type dedupDecision int

const (
	dedupNew       dedupDecision = iota // different enough: store it
	dedupDuplicate                      // clearly the same: reinforce the existing fact
	dedupReview                         // borderline: let the LLM decide
)

// decideDedup maps a cosine similarity to a decision. At or above high a fact is a
// duplicate without further ado. From review up to (not including) high the case is
// ambiguous — rewordings and translations land here — so the LLM is asked. A review
// threshold of 0, or one that is not below high, disables the LLM review.
func decideDedup(score, high, review float32) dedupDecision {
	switch {
	case score >= high:
		return dedupDuplicate
	case review > 0 && review < high && score >= review:
		return dedupReview
	default:
		return dedupNew
	}
}

type nearestFact struct {
	ID      int64
	Content string
	Score   float32
}

// defaultDedupCandidates bounds how many nearest facts one new fact is weighed
// against. Comparing against a single neighbour was not enough: when that
// neighbour is a generic statement that sits close to everything, the real twin
// at rank two is invisible to both the vector test and the LLM review.
const defaultDedupCandidates = 5

// dedupNearMissFloor is the similarity from which a candidate counts as a
// near-miss when the new fact is stored as new anyway. That band is where
// duplicates are born, and it is the one nothing reported on. Like the
// thresholds above it, it is not calibrated on our own data.
const dedupNearMissFloor float32 = 0.75

// nearestActiveFacts returns up to k closest active (not deleted, not expired)
// facts of the namespace, nearest first. Superseded facts are ignored so a
// re-observed value is not swallowed by them.
func (b *Brain) nearestActiveFacts(ctx context.Context, nsID int64, vec []float32, k int) ([]nearestFact, error) {
	rows, err := b.pool.Query(ctx,
		`SELECT id, content, 1 - (embedding <=> $2) AS score FROM facts
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL AND embedding IS NOT NULL
		 ORDER BY embedding <=> $2 LIMIT $3`,
		nsID, pgvector.NewVector(vec), k,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []nearestFact
	for rows.Next() {
		var n nearestFact
		if err := rows.Scan(&n.ID, &n.Content, &n.Score); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// dedupCandidates is how many neighbours a new fact is compared against.
func (b *Brain) dedupCandidates() int {
	if b.config.DedupCandidates > 0 {
		return b.config.DedupCandidates
	}
	return defaultDedupCandidates
}

// dedupOutcome is what comparing a new fact against its namespace concluded.
type dedupOutcome struct {
	// DupID is the fact to fold the new one into; 0 when the new fact is stored.
	DupID int64
	// Merged says whether DupID holds a duplicate of the new fact.
	Merged bool
	// LLMCalls counts the review calls made while walking the candidates.
	LLMCalls int
	// Candidates is how many neighbours were weighed.
	Candidates int
	// BestScore is the highest similarity among them, 0 without candidates. It
	// tells the caller whether a new fact was born next to a near-twin.
	BestScore float32
}

// findDuplicate reports whether summary duplicates an existing active fact and,
// if so, which one. It weighs the nearest neighbours in score order, so a twin
// that is not the closest fact still gets its review.
func (b *Brain) findDuplicate(ctx context.Context, nsID int64, vec []float32, summary string) (dedupOutcome, error) {
	var out dedupOutcome
	candidates, err := b.nearestActiveFacts(ctx, nsID, vec, b.dedupCandidates())
	if err != nil {
		return out, err
	}
	if len(candidates) == 0 {
		return out, nil
	}
	out.Candidates = len(candidates)
	out.BestScore = candidates[0].Score

	out.DupID, out.Merged, out.LLMCalls, err = pickDedupTarget(
		candidates,
		float32(b.config.DedupThreshold),
		float32(b.config.DedupReviewThreshold),
		func(c nearestFact) (bool, error) {
			return b.reasoner.ReasonSameFact(ctx, c.Content, summary)
		},
	)
	return out, err
}

// pickDedupTarget walks candidates in descending score order and returns the
// first one that counts as the same fact: at or above high without asking, or
// accepted by review inside the borderline band. The walk stops at the first
// candidate below that band, since none after it can score higher. review is
// only called for borderline candidates, so the number of LLM calls stays
// bounded by the number of candidates.
func pickDedupTarget(candidates []nearestFact, high, review float32, ask func(nearestFact) (bool, error)) (dupID int64, merged bool, llmCalls int, err error) {
	for _, c := range candidates {
		switch decideDedup(c.Score, high, review) {
		case dedupDuplicate:
			return c.ID, true, llmCalls, nil
		case dedupReview:
			llmCalls++
			same, err := ask(c)
			if err != nil {
				return 0, false, llmCalls, err
			}
			if same {
				return c.ID, true, llmCalls, nil
			}
			// The LLM calls this neighbour a different fact; the next one may
			// still be the twin, so keep walking.
		default:
			return 0, false, llmCalls, nil
		}
	}
	return 0, false, llmCalls, nil
}

// namespaceSubject names what a namespace is about ("CortexIQ"), so a synthesized fact
// can name its project instead of saying "the system". It is only context for the
// LLM. It is deliberately just the NAME: a description is free text, and models
// turned statements from it into "facts" that no episode ever stated.
func (b *Brain) namespaceSubject(ctx context.Context, nsID int64) string {
	var slug, name string
	if err := b.pool.QueryRow(ctx,
		"SELECT slug, name FROM namespaces WHERE id = $1", nsID,
	).Scan(&slug, &name); err != nil {
		return ""
	}
	return formatSubject(slug, name)
}

// formatSubject returns the namespace name, or, when the name is just the slug (or
// empty), the last path segment of the slug. The root namespace has no subject.
func formatSubject(slug, name string) string {
	name = strings.TrimSpace(name)
	if name != "" && name != slug {
		return name
	}
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		return slug[i+1:]
	}
	return slug
}

// --- Cleaning up existing duplicates ---

// DedupeOptions controls DedupeFacts.
type DedupeOptions struct {
	// Namespaces to clean (each path includes its descendants).
	Namespaces []string
	// Threshold is the cosine similarity from which two facts are merged without asking.
	Threshold float32
	// ReviewThreshold, when > 0 and below Threshold, sends pairs from that similarity up
	// to Threshold to the LLM, and merges the ones it calls the same. 0 disables review.
	ReviewThreshold float32
	// MaxReviews bounds the LLM calls of one run (default 100).
	MaxReviews int
	// Apply writes the merge; without it DedupeFacts only reports what it would do.
	Apply bool
}

// DedupeGroup is a set of facts that state the same thing: Keep survives and the
// others are merged into it.
type DedupeGroup struct {
	Keep        int64    `json:"keep"`
	KeepContent string   `json:"keep_content"`
	Merge       []int64  `json:"merge"`
	Merged      []string `json:"merged_content"`
}

// DedupeResult describes a DedupeFacts run.
type DedupeResult struct {
	Applied     bool          `json:"applied"`
	Groups      []DedupeGroup `json:"groups"`
	FactsMerged int           `json:"facts_merged"`
	LLMCalls    int           `json:"llm_calls"`
	Errors      []string      `json:"errors,omitempty"`
}

type dedupeMember struct {
	id         int64
	confidence float32
	sources    int
	content    string
}

// DedupeFacts finds active facts that duplicate each other and, with Apply, merges
// them: the survivor takes over the sources of the others and gets reinforced, and
// the others are soft-deleted. Without Apply nothing is written.
//
// The comparison is pairwise per namespace, which is fine for hundreds to a few
// thousand facts per namespace.
func (b *Brain) DedupeFacts(ctx context.Context, opts DedupeOptions) (DedupeResult, error) {
	res := DedupeResult{Applied: opts.Apply}
	if opts.Threshold <= 0 || opts.Threshold > 1 {
		return res, fmt.Errorf("dedupe: threshold must be in (0, 1], got %v", opts.Threshold)
	}
	if opts.MaxReviews <= 0 {
		opts.MaxReviews = 100
	}
	reviewOn := opts.ReviewThreshold > 0 && opts.ReviewThreshold < opts.Threshold
	minSim := opts.Threshold
	if reviewOn {
		minSim = opts.ReviewThreshold
	}

	nsIDs, err := b.resolveNamespaceIDs(ctx, opts.Namespaces)
	if err != nil {
		return res, err
	}

	rows, err := b.pool.Query(ctx,
		`SELECT a.id, b.id, 1 - (a.embedding <=> b.embedding) AS sim, a.content, b.content
		 FROM facts a JOIN facts b ON b.id > a.id AND b.namespace_id = a.namespace_id
		 WHERE a.namespace_id = ANY($1)
		   AND a.deleted_at IS NULL AND b.deleted_at IS NULL
		   AND a.valid_until IS NULL AND b.valid_until IS NULL
		   AND a.embedding IS NOT NULL AND b.embedding IS NOT NULL
		   AND 1 - (a.embedding <=> b.embedding) >= $2
		 ORDER BY sim DESC`, nsIDs, minSim)
	if err != nil {
		return res, fmt.Errorf("dedupe: find pairs: %w", err)
	}
	type pair struct {
		a, b   int64
		sim    float32
		ca, cb string
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.a, &p.b, &p.sim, &p.ca, &p.cb); err != nil {
			rows.Close()
			return res, fmt.Errorf("dedupe: scan pair: %w", err)
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("dedupe: pairs: %w", err)
	}

	uf := newUnionFind()
	contents := map[int64]string{}
	for _, p := range pairs {
		contents[p.a], contents[p.b] = p.ca, p.cb
		if p.sim >= opts.Threshold {
			uf.union(p.a, p.b)
			continue
		}
		// Borderline pair: only the LLM can say whether it is a rewording.
		if res.LLMCalls >= opts.MaxReviews {
			continue
		}
		res.LLMCalls++
		same, err := b.reasoner.ReasonSameFact(ctx, p.ca, p.cb)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("review facts %d and %d: %v", p.a, p.b, err))
			continue
		}
		if same {
			uf.union(p.a, p.b)
		}
	}

	groups := uf.groups()
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	for _, ids := range groups {
		members, err := b.loadDedupeMembers(ctx, ids)
		if err != nil {
			return res, err
		}
		keep := pickSurvivor(members)
		g := DedupeGroup{Keep: keep.id, KeepContent: keep.content}
		var merge []int64
		for _, m := range members {
			if m.id == keep.id {
				continue
			}
			merge = append(merge, m.id)
			g.Merged = append(g.Merged, m.content)
		}
		sort.Slice(merge, func(i, j int) bool { return merge[i] < merge[j] })
		g.Merge = merge
		res.Groups = append(res.Groups, g)
		res.FactsMerged += len(merge)

		if opts.Apply {
			if err := b.mergeFacts(ctx, keep, merge); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func (b *Brain) loadDedupeMembers(ctx context.Context, ids []int64) ([]dedupeMember, error) {
	rows, err := b.pool.Query(ctx,
		`SELECT f.id, f.confidence, f.content,
		        (SELECT count(*) FROM fact_sources s WHERE s.fact_id = f.id)
		 FROM facts f WHERE f.id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("dedupe: load group: %w", err)
	}
	defer rows.Close()
	var out []dedupeMember
	for rows.Next() {
		var m dedupeMember
		if err := rows.Scan(&m.id, &m.confidence, &m.content, &m.sources); err != nil {
			return nil, fmt.Errorf("dedupe: scan member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// mergeFacts folds merge into keep atomically: sources move over, keep is
// reinforced once per merged fact, the merged facts are soft-deleted.
func (b *Brain) mergeFacts(ctx context.Context, keep dedupeMember, merge []int64) error {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dedupe: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO fact_sources (fact_id, episode_id)
		 SELECT $1, episode_id FROM fact_sources WHERE fact_id = ANY($2)
		 ON CONFLICT DO NOTHING`, keep.id, merge); err != nil {
		return fmt.Errorf("dedupe: move sources: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE facts SET confidence = $2, updated_at = now() WHERE id = $1",
		keep.id, reinforcedConfidence(keep.confidence, len(merge))); err != nil {
		return fmt.Errorf("dedupe: reinforce: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE facts SET deleted_at = now(), updated_at = now() WHERE id = ANY($1) AND deleted_at IS NULL", merge); err != nil {
		return fmt.Errorf("dedupe: delete merged: %w", err)
	}
	return tx.Commit(ctx)
}

// reinforcedConfidence applies the per-observation reinforcement n times.
func reinforcedConfidence(c float32, n int) float32 {
	for i := 0; i < n; i++ {
		c = c + (1-c)*reinforceStep
	}
	if c > 1 {
		c = 1
	}
	return c
}

// pickSurvivor chooses which fact of a duplicate group to keep: the one backed by
// the most sources, then the most confident, then the oldest (lowest ID).
func pickSurvivor(members []dedupeMember) dedupeMember {
	best := members[0]
	for _, m := range members[1:] {
		switch {
		case m.sources != best.sources:
			if m.sources > best.sources {
				best = m
			}
		case m.confidence != best.confidence:
			if m.confidence > best.confidence {
				best = m
			}
		case m.id < best.id:
			best = m
		}
	}
	return best
}

// unionFind groups IDs into connected sets.
type unionFind struct{ parent map[int64]int64 }

func newUnionFind() *unionFind { return &unionFind{parent: map[int64]int64{}} }

func (u *unionFind) find(x int64) int64 {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int64) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

// groups returns the sets with more than one member, each sorted ascending.
func (u *unionFind) groups() [][]int64 {
	by := map[int64][]int64{}
	for id := range u.parent {
		r := u.find(id)
		by[r] = append(by[r], id)
	}
	var out [][]int64
	for _, ids := range by {
		if len(ids) < 2 {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		out = append(out, ids)
	}
	return out
}
