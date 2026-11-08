// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// The shape sweep is the second way the patterns stage finds regularity.
//
// consolidateToPatterns reads facts in id order, so it can only see recurrence
// that happens to fall inside thirty consecutive memories. A regularity such as
// "every MCP client reaches its server over the http-first transport" lived in
// eleven relationships spread over weeks, so no window ever contained it — the
// stage correctly reported nothing while the regularity sat in the graph.
//
// The sweep turns that around. The database groups relationships by (relation
// type, target) and counts distinct sources; three or more sources is evidence
// that something recurs. The model is then asked only to phrase a candidate or
// reject it. Grouping is cheap and exact, so the LLM call is spent on wording
// instead of search, and a summary cannot slip through: there is nothing to
// summarise, only a claim to accept or refuse.
const (
	// shapeMinSources is how many distinct sources must point at the same target
	// before the grouping counts as recurrence. Two is a coincidence, three is a
	// habit.
	shapeMinSources = 3
	// shapeBatch is how many candidates one LLM call carries.
	shapeBatch = 4
	// shapeMaxPerRun bounds the work a single consolidation run spends on the
	// sweep: a backlog is consumed over several ticks instead of in one spike.
	shapeMaxPerRun = 8
)

// shapeClusterSQL finds candidates for one namespace and skips the ones already
// judged, unless new edges landed since. The cursor lives in pattern_clusters,
// not in the consolidation checkpoint: a cluster is not a position in a stream.
const shapeClusterSQL = `
WITH grouped AS (
    SELECT coalesce(r.canonical_type, r.relation_type) AS rtype,
           lower(r.to_entity)                          AS target,
           count(DISTINCT r.from_entity)               AS sources,
           max(r.id)                                   AS max_rel_id,
           min(r.confidence)                           AS min_conf,
           array_agg(DISTINCT r.from_entity)           AS source_names,
           array_agg(DISTINCT r.id)                    AS rel_ids,
           array_agg(DISTINCT r.source_fact_id) FILTER (WHERE r.source_fact_id IS NOT NULL) AS fact_ids
    FROM relationships r
    WHERE r.namespace_id = $1 AND r.deleted_at IS NULL
    GROUP BY 1, 2
    HAVING count(DISTINCT r.from_entity) >= $2
)
SELECT g.rtype, g.target, g.max_rel_id, g.min_conf, g.source_names, g.rel_ids, g.fact_ids
FROM grouped g
LEFT JOIN pattern_clusters pc
       ON pc.namespace_id = $1
      AND pc.signature = g.rtype || '|to|' || g.target
WHERE pc.id IS NULL
   OR g.max_rel_id > pc.max_rel_id
   OR pc.verdict = 'error'
ORDER BY g.sources DESC, g.max_rel_id DESC
LIMIT $3`

const shapeSampleSQL = `
SELECT id, left(content, 200)
FROM facts
WHERE id = ANY($1) AND deleted_at IS NULL`

// shapeCluster is one candidate plus the evidence needed to store or record it.
type shapeCluster struct {
	relationType  string
	target        string
	sources       []string
	samples       []string
	relIDs        []int64
	factIDs       []int64
	maxRelID      int64
	minConfidence float32
}

func shapeSignature(relationType, target string) string {
	return relationType + "|to|" + target
}

// consolidateToShapePatterns asks the model about relationship clusters the
// window cannot see. It returns how many patterns were stored, how many LLM
// calls it spent, and any errors, so it can sit beside the existing stage.
func (b *Brain) consolidateToShapePatterns(ctx context.Context, nsID int64) (count, llmCalls int, errs []string) {
	clusters, err := b.fetchShapeClusters(ctx, nsID, shapeMinSources, shapeMaxPerRun)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch shape clusters: %v", err))
		return
	}
	if len(clusters) == 0 {
		return
	}

	if err := b.attachShapeSamples(ctx, clusters); err != nil {
		// Samples only sharpen the wording; a candidate without them is still
		// worth judging, so this is not fatal.
		log.Printf("shape sweep: no samples for namespace %d: %v", nsID, err)
	}

	for start := 0; start < len(clusters); start += shapeBatch {
		end := start + shapeBatch
		if end > len(clusters) {
			end = len(clusters)
		}
		batch := clusters[start:end]

		candidates := make([]reasoner.ShapeCandidate, len(batch))
		for i, c := range batch {
			candidates[i] = reasoner.ShapeCandidate{
				RelationType: c.relationType,
				Target:       c.target,
				Sources:      c.sources,
				Samples:      c.samples,
			}
		}

		statements, err := b.reasoner.ReasonShapes(ctx, candidates)
		llmCalls++
		if err != nil {
			errs = append(errs, fmt.Sprintf("reason shapes: %v", err))
			// Leave the clusters unjudged: the next run retries them.
			return
		}

		for i, c := range batch {
			var st *reasoner.ShapeStatement
			if i < len(statements) {
				st = statements[i]
			}

			if st == nil || st.Coherence <= 0 || st.Statement == "" {
				if err := b.recordShapeCluster(ctx, nsID, c, "rejected"); err != nil {
					errs = append(errs, fmt.Sprintf("record rejected cluster: %v", err))
				}
				continue
			}

			// The cluster minimum times the share of sources that fit, the same
			// way the window stage scores its patterns.
			confidence := c.minConfidence * st.Coherence
			if _, err := b.storeShapePattern(ctx, nsID, c.target, st.Statement, confidence, c.factIDs, c.relIDs, c.sources, st.Coherence); err != nil {
				errs = append(errs, fmt.Sprintf("store shape pattern: %v", err))
				if err := b.recordShapeCluster(ctx, nsID, c, "error"); err != nil {
					errs = append(errs, fmt.Sprintf("record failed cluster: %v", err))
				}
				continue
			}
			if err := b.recordShapeCluster(ctx, nsID, c, "pattern"); err != nil {
				errs = append(errs, fmt.Sprintf("record cluster: %v", err))
			}
			count++
		}
	}

	return
}

func (b *Brain) fetchShapeClusters(ctx context.Context, nsID int64, minSources, limit int) ([]*shapeCluster, error) {
	rows, err := b.pool.Query(ctx, shapeClusterSQL, nsID, minSources, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clusters []*shapeCluster
	for rows.Next() {
		var c shapeCluster
		if err := rows.Scan(&c.relationType, &c.target, &c.maxRelID, &c.minConfidence,
			&c.sources, &c.relIDs, &c.factIDs); err != nil {
			return nil, err
		}
		c.sources = uniqueStrings(c.sources)
		clusters = append(clusters, &c)
	}
	return clusters, rows.Err()
}

// attachShapeSamples adds a couple of statements behind each cluster's edges, so
// the model words the regularity in the vocabulary the memory actually uses.
func (b *Brain) attachShapeSamples(ctx context.Context, clusters []*shapeCluster) error {
	var ids []int64
	for _, c := range clusters {
		if len(c.factIDs) > 3 {
			ids = append(ids, c.factIDs[:3]...)
			continue
		}
		ids = append(ids, c.factIDs...)
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := b.pool.Query(ctx, shapeSampleSQL, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	content := make(map[int64]string)
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return err
		}
		content[id] = text
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, c := range clusters {
		for _, id := range c.factIDs {
			if text, ok := content[id]; ok {
				c.samples = append(c.samples, text)
			}
			if len(c.samples) == 3 {
				break
			}
		}
	}
	return nil
}

const shapeRecordSQL = `
INSERT INTO pattern_clusters (namespace_id, signature, relation_type, target, sources_seen, max_rel_id, verdict)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (namespace_id, signature) DO UPDATE
SET relation_type = EXCLUDED.relation_type,
    target        = EXCLUDED.target,
    sources_seen  = GREATEST(pattern_clusters.sources_seen, EXCLUDED.sources_seen),
    max_rel_id    = GREATEST(pattern_clusters.max_rel_id, EXCLUDED.max_rel_id),
    -- A cluster that already produced a pattern must never be demoted by a second
    -- opinion. The scheduler and a manual run can judge the same cluster at the
    -- same time; production showed the later rejection overwriting an accepted
    -- verdict, so the stored pattern and its cursor disagreed.
    verdict       = CASE
                      WHEN pattern_clusters.verdict = 'pattern' AND EXCLUDED.verdict <> 'pattern'
                      THEN 'pattern'
                      ELSE EXCLUDED.verdict
                    END,
    last_run_at   = now()`

// recordShapeCluster advances the cursor for one candidate whether it produced a
// pattern or was rejected. A rejection that this statement did not advance would
// be asked again on every tick, which is the cheapest way to burn a budget.
func (b *Brain) recordShapeCluster(ctx context.Context, nsID int64, c *shapeCluster, verdict string) error {
	_, err := b.pool.Exec(ctx, shapeRecordSQL, nsID, shapeSignature(c.relationType, c.target),
		c.relationType, c.target, len(c.sources), c.maxRelID, verdict)
	return err
}

// shapeSameTargetSQL finds live patterns whose own evidence points at the same
// target, in the namespace.
const shapeSameTargetSQL = `
SELECT p.id, p.source_entities
FROM patterns p
WHERE p.namespace_id = $1
  AND p.deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM relationships r
    WHERE r.id = ANY(p.source_rel_ids)
      AND lower(r.to_entity) = $2
  )`

// shapeMergeSQL folds a second sighting into the row that already says the same
// thing about the same things.
const shapeMergeSQL = `
UPDATE patterns
SET source_entities = COALESCE((
      SELECT array_agg(DISTINCT x) FROM unnest(source_entities || $2::text[]) AS x
    ), '{}'),
    source_rel_ids = COALESCE((
      SELECT array_agg(DISTINCT x) FROM unnest(source_rel_ids || $3::bigint[]) AS x
    ), '{}'),
    source_fact_ids = COALESCE((
      SELECT array_agg(DISTINCT x) FROM unnest(source_fact_ids || $4::bigint[]) AS x
    ), '{}'),
    confidence      = GREATEST(confidence, $5),
    coherence_score = GREATEST(coherence_score, $6),
    updated_at      = now()
WHERE id = $1`

// storeShapePattern stores a statement the sweep derived from a cluster of edges,
// merging it into a pattern that already states the same thing about the same
// sources.
//
// The content key only catches identical wording, and production showed the same
// regularity arriving twice: "Nmbrs, Yuki and … handle document chasing" next to
// "Yuki, Nmbrs and … handle document chasing for SME/ZZP accountants". Two
// conditions have to hold together, because either one alone is wrong:
//
//   - the evidence points at the same target. Clusters partition the edges, so
//     this is really "the statement is about the same thing" (a cluster of
//     `handles` edges and one of `handles_task` edges at the same target).
//   - at least half of the smaller source set overlaps. On sources alone it would
//     be wrong: the same four suppliers both chase documents and categorise
//     receipts, and those are two regularities, not one.
func (b *Brain) storeShapePattern(ctx context.Context, nsID int64, target, content string, confidence float32, factIDs, relIDs []int64, sources []string, coherence float32) (inserted bool, err error) {
	if len(sources) > 0 {
		var (
			bestID   int64
			bestSet  []string
			bestHits int
		)
		rows, err := b.pool.Query(ctx, shapeSameTargetSQL, nsID, strings.ToLower(target))
		if err != nil {
			return false, fmt.Errorf("find patterns at the same target: %w", err)
		}
		for rows.Next() {
			var (
				id      int64
				matched []string
			)
			if err := rows.Scan(&id, &matched); err != nil {
				rows.Close()
				return false, err
			}
			if hits := countOverlap(sources, matched); hits > bestHits {
				bestID, bestSet, bestHits = id, matched, hits
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return false, err
		}

		smaller := len(sources)
		if len(bestSet) < smaller {
			smaller = len(bestSet)
		}
		if bestID != 0 && bestHits*2 >= smaller {
			if err := b.mergeShapePattern(ctx, bestID, relIDs, factIDs, sources, confidence, coherence); err != nil {
				return false, err
			}
			return false, nil
		}
	}
	return b.storePattern(ctx, nsID, content, confidence, factIDs, relIDs, sources, coherence)
}

// mergeShapePattern keeps the wording that is already there and adds what the
// second sighting proved.
func (b *Brain) mergeShapePattern(ctx context.Context, id int64, relIDs, factIDs []int64, sources []string, confidence, coherence float32) error {
	if relIDs == nil {
		relIDs = []int64{}
	}
	if factIDs == nil {
		factIDs = []int64{}
	}
	if sources == nil {
		sources = []string{}
	}
	if _, err := b.pool.Exec(ctx, shapeMergeSQL, id, sources, relIDs, factIDs, confidence, coherence); err != nil {
		return fmt.Errorf("merge shape pattern: %w", err)
	}
	return nil
}

// countOverlap is how many items the two sets share.
func countOverlap(a, b []string) int {
	have := make(map[string]bool, len(b))
	for _, s := range b {
		have[s] = true
	}
	hits := 0
	for _, s := range a {
		if have[s] {
			hits++
		}
	}
	return hits
}

// uniqueStrings drops duplicates while keeping the order the query produced.
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
