// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// FactsFromEpisode returns the facts consolidated from one episode, oldest
// first. The dashboard shows them beside the episode so a reader can see what
// the raw memory turned into, or that extraction has not run yet.
//
// A fact can outlive its episode (merge and purge keep the fact while dropping
// a source), so the join is the only link that answers this question: content
// similarity is not the same as derivation.
func (b *Brain) FactsFromEpisode(ctx context.Context, episodeID int64) ([]models.Fact, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT f.id, f.namespace_id, f.content, f.confidence, f.entity, f.property, f.value,
		       f.valid_from, f.valid_until, f.created_at, f.updated_at, f.tags
		FROM fact_sources s
		JOIN facts f ON f.id = s.fact_id
		WHERE s.episode_id = $1 AND f.deleted_at IS NULL
		ORDER BY f.id`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("facts from episode: %w", err)
	}
	defer rows.Close()

	out := make([]models.Fact, 0, 8)
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Confidence,
			&f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil,
			&f.CreatedAt, &f.UpdatedAt, &f.Tags); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("facts from episode: %w", err)
	}
	return out, nil
}

// EpisodesForFact is the other direction: the episodes a fact was consolidated
// from. A merged fact can name several; a fact with none was derived without a
// recorded source and says so rather than guessing.
func (b *Brain) EpisodesForFact(ctx context.Context, factID int64) ([]models.Episode, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT e.id, e.namespace_id, e.content, e.occurred_at, e.created_at, e.tags
		FROM fact_sources s
		JOIN episodes e ON e.id = s.episode_id
		WHERE s.fact_id = $1 AND e.deleted_at IS NULL
		ORDER BY e.occurred_at`, factID)
	if err != nil {
		return nil, fmt.Errorf("episodes for fact: %w", err)
	}
	defer rows.Close()

	out := make([]models.Episode, 0, 4)
	for rows.Next() {
		var e models.Episode
		if err := rows.Scan(&e.ID, &e.NamespaceID, &e.Content, &e.OccurredAt, &e.CreatedAt, &e.Tags); err != nil {
			return nil, fmt.Errorf("scan episode: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("episodes for fact: %w", err)
	}
	return out, nil
}
