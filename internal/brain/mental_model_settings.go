// Copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

const (
	defaultMentalModelSourceLimit = 20
	maxMentalModelSourceLimit     = 100
	// maxMentalModelRefreshIntervalSeconds is 30 days. Anything rarer is better
	// expressed by switching automatic refresh off.
	maxMentalModelRefreshIntervalSeconds = 30 * 24 * 60 * 60
	// maxMentalModelVersions is how many replaced versions a model keeps.
	maxMentalModelVersions = 50
)

// MentalModelSettings decide when a model is rewritten automatically and which
// memories it is written from.
type MentalModelSettings struct {
	// AutoRefresh lets consolidation refresh the model. An explicit refresh
	// works either way.
	AutoRefresh bool
	// MinRefreshIntervalSeconds is the shortest time between two automatic
	// refreshes. Nil means the default of one hour.
	MinRefreshIntervalSeconds *int
	// SourceTags limits the sources to memories carrying at least one of these.
	SourceTags []string
	// SourceTypes limits the sources to fact, episode, failure and/or goal.
	// Empty means facts and episodes.
	SourceTypes []string
	// SourceLimit is how many memories are recalled to write the content.
	SourceLimit int
}

// MentalModelSettingsPatch changes only the settings that are set.
type MentalModelSettingsPatch struct {
	AutoRefresh *bool
	// MinRefreshIntervalSeconds below zero restores the default.
	MinRefreshIntervalSeconds *int
	SourceTags                *[]string
	SourceTypes               *[]string
	// SourceLimit of zero or less restores the default.
	SourceLimit *int
}

func defaultMentalModelSettings() MentalModelSettings {
	return MentalModelSettings{
		AutoRefresh: true,
		SourceTags:  []string{},
		SourceTypes: []string{},
		SourceLimit: defaultMentalModelSourceLimit,
	}
}

func mentalModelSettingsOf(m models.MentalModel) MentalModelSettings {
	return MentalModelSettings{
		AutoRefresh:               m.AutoRefresh,
		MinRefreshIntervalSeconds: m.MinRefreshIntervalSeconds,
		SourceTags:                m.SourceTags,
		SourceTypes:               m.SourceTypes,
		SourceLimit:               m.SourceLimit,
	}
}

// apply merges a patch into s and validates the result.
func (s MentalModelSettings) apply(p MentalModelSettingsPatch) (MentalModelSettings, error) {
	if p.AutoRefresh != nil {
		s.AutoRefresh = *p.AutoRefresh
	}
	if v := p.MinRefreshIntervalSeconds; v != nil {
		switch {
		case *v < 0:
			s.MinRefreshIntervalSeconds = nil
		case *v > maxMentalModelRefreshIntervalSeconds:
			return s, fmt.Errorf("brain: mental model refresh interval may be at most %d seconds", maxMentalModelRefreshIntervalSeconds)
		default:
			seconds := *v
			s.MinRefreshIntervalSeconds = &seconds
		}
	}
	if p.SourceTags != nil {
		tags, err := NormalizeTags(*p.SourceTags)
		if err != nil {
			return s, err
		}
		s.SourceTags = tags
	}
	if p.SourceTypes != nil {
		var types []string
		for _, t := range *p.SourceTypes {
			t = strings.ToLower(strings.TrimSpace(t))
			switch t {
			case "":
				continue
			case "fact", "episode", "failure", "goal":
			default:
				return s, fmt.Errorf("brain: unknown mental model source type %q: use fact, episode, failure or goal", t)
			}
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
		slices.Sort(types)
		s.SourceTypes = types
	}
	if v := p.SourceLimit; v != nil {
		switch {
		case *v <= 0:
			s.SourceLimit = defaultMentalModelSourceLimit
		case *v > maxMentalModelSourceLimit:
			return s, fmt.Errorf("brain: mental model source limit may be at most %d", maxMentalModelSourceLimit)
		default:
			s.SourceLimit = *v
		}
	}
	// Both columns are NOT NULL.
	if s.SourceTags == nil {
		s.SourceTags = []string{}
	}
	if s.SourceTypes == nil {
		s.SourceTypes = []string{}
	}
	return s, nil
}

// reflectOptions are the options that write a mental model's content.
func (s MentalModelSettings) reflectOptions(scope []string, maxTokens int) ReflectOptions {
	return ReflectOptions{
		Namespaces: scope,
		Limit:      s.SourceLimit,
		MaxTokens:  maxTokens,
		TagsAny:    s.SourceTags,
		Types:      s.SourceTypes,
		// A mental model is synthesized from raw memories. Its siblings would
		// make it a copy of what other models already believe.
		SkipMentalModels: true,
		// A curated summary holds settled knowledge; an open belief is exactly
		// what it must not contain, or it would launder a guess into a fact.
		SkipHypotheses: true,
		// Patterns are derived across a whole namespace and carry no tags or
		// type, so they would walk straight past a source filter.
		SkipPatterns: len(s.SourceTags) > 0 || len(s.SourceTypes) > 0,
		Instructions: mentalModelInstructions,
	}
}

// ListMentalModelVersions returns the content a model held before, newest first.
func (b *Brain) ListMentalModelVersions(ctx context.Context, id int64) ([]models.MentalModelVersion, error) {
	if _, err := b.getMentalModel(ctx, id); err != nil {
		return nil, err
	}
	rows, err := b.pool.Query(ctx, `
		SELECT id, mental_model_id, content, source_query, based_on, refreshed_at, replaced_at, replaced_by
		FROM mental_model_versions WHERE mental_model_id = $1 ORDER BY id DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("list mental model versions: %w", err)
	}
	defer rows.Close()

	out := []models.MentalModelVersion{}
	for rows.Next() {
		var (
			v       models.MentalModelVersion
			basedOn []byte
		)
		if err := rows.Scan(&v.ID, &v.MentalModelID, &v.Content, &v.SourceQuery, &basedOn,
			&v.RefreshedAt, &v.ReplacedAt, &v.ReplacedBy); err != nil {
			return nil, fmt.Errorf("scan mental model version: %w", err)
		}
		if len(basedOn) > 0 {
			if err := json.Unmarshal(basedOn, &v.BasedOn); err != nil {
				return nil, fmt.Errorf("decode mental model version based_on: %w", err)
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ClearMentalModel empties a model's content and keeps its definition. The old
// content goes to the history. The model is stale afterwards, so an automatic
// or explicit refresh writes it again from nothing.
func (b *Brain) ClearMentalModel(ctx context.Context, id int64) (*models.MentalModel, error) {
	release, busy := b.beginMentalModelRefresh(id)
	if busy {
		return nil, ErrMentalModelRefreshInFlight
	}
	defer release()

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("clear mental model: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := archiveMentalModelContent(ctx, tx, id, "", "clear"); err != nil {
		return nil, err
	}
	var m models.MentalModel
	// last_refreshed_at stays: it is what tells a cleared model apart from one
	// whose first generation is still running.
	err = scanMentalModel(&m, tx.QueryRow(ctx, `
		UPDATE mental_models
		SET content = '', based_on = '{}'::jsonb, last_memory_seen_at = NULL,
		    last_refresh_failed_at = NULL, last_refresh_error = NULL, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+mentalModelColumns, id))
	if err == pgx.ErrNoRows {
		return nil, ErrMentalModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("clear mental model: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("clear mental model: commit: %w", err)
	}
	return &m, nil
}

// archiveMentalModelContent stores the content that is about to be replaced by
// next. Empty and unchanged content is not worth a version. Content written from
// a memory that has since been purged is not kept either: the history would
// otherwise outlive the erasure.
func archiveMentalModelContent(ctx context.Context, tx pgx.Tx, id int64, next, replacedBy string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO mental_model_versions (mental_model_id, content, source_query, based_on, refreshed_at, replaced_by)
		SELECT m.id, m.content, m.last_refreshed_source_query, m.based_on, m.last_refreshed_at, $3
		FROM mental_models m
		WHERE m.id = $1 AND m.deleted_at IS NULL AND m.content <> '' AND m.content <> $2
		  AND NOT EXISTS (
		    SELECT 1 FROM jsonb_array_elements_text(coalesce(m.based_on->'facts', '[]'::jsonb)) AS ref(id)
		    WHERE NOT EXISTS (SELECT 1 FROM facts f WHERE f.id = ref.id::bigint))
		  AND NOT EXISTS (
		    SELECT 1 FROM jsonb_array_elements_text(coalesce(m.based_on->'episodes', '[]'::jsonb)) AS ref(id)
		    WHERE NOT EXISTS (SELECT 1 FROM episodes e WHERE e.id = ref.id::bigint))`,
		id, next, replacedBy); err != nil {
		return fmt.Errorf("archive mental model content: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM mental_model_versions
		WHERE mental_model_id = $1 AND id NOT IN (
		  SELECT id FROM mental_model_versions WHERE mental_model_id = $1 ORDER BY id DESC LIMIT $2)`,
		id, maxMentalModelVersions); err != nil {
		return fmt.Errorf("trim mental model versions: %w", err)
	}
	return nil
}

// purgeMentalModelVersions deletes the versions written from any of the given
// memories. It runs inside the purge transaction.
func purgeMentalModelVersions(ctx context.Context, tx pgx.Tx, factIDs, episodeIDs []int64) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM mental_model_versions v
		WHERE EXISTS (
		    SELECT 1 FROM jsonb_array_elements_text(coalesce(v.based_on->'facts', '[]'::jsonb)) AS ref(id)
		    WHERE ref.id::bigint = ANY($1::bigint[]))
		   OR EXISTS (
		    SELECT 1 FROM jsonb_array_elements_text(coalesce(v.based_on->'episodes', '[]'::jsonb)) AS ref(id)
		    WHERE ref.id::bigint = ANY($2::bigint[]))`, factIDs, episodeIDs); err != nil {
		return fmt.Errorf("purge: delete mental model versions: %w", err)
	}
	return nil
}
