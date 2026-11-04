// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

// Namespace configuration keys. Every key is optional; a namespace without an
// override inherits the global default from the process Config. Keys are
// validated on write so a typo cannot silently do nothing.
const (
	KeyConsolidationSimilarityThreshold = "consolidation.similarity_threshold"
	KeyConsolidationDedupThreshold      = "consolidation.dedup_threshold"
	KeyConsolidationBatchSize           = "consolidation.batch_size"
	KeyConsolidationMaxLLMCallsPerRun   = "consolidation.max_llm_calls_per_run"
	KeyRecallHybrid                     = "recall.hybrid"
	KeyRecallMinSimilarity              = "recall.min_similarity"
	KeyRecallRecencyWeight              = "recall.recency_weight"
	KeyRememberSocialFilter             = "remember.social_filter"
	KeyRememberMinEpisodeLength         = "remember.min_episode_length"
)

// namespaceConfigKeyTypes maps every known key to its expected JSON type.
// Used by validateNamespaceConfigOverrides to reject unknown keys and wrong
// types before they reach the database.
var namespaceConfigKeyTypes = map[string]string{
	KeyConsolidationSimilarityThreshold: "number",
	KeyConsolidationDedupThreshold:      "number",
	KeyConsolidationBatchSize:           "integer",
	KeyConsolidationMaxLLMCallsPerRun:   "integer",
	KeyRecallHybrid:                     "boolean",
	KeyRecallMinSimilarity:              "number",
	KeyRecallRecencyWeight:              "number",
	KeyRememberSocialFilter:             "boolean",
	KeyRememberMinEpisodeLength:         "integer",
}

// KnownNamespaceConfigKeys returns the sorted list of valid override keys.
func KnownNamespaceConfigKeys() []string {
	keys := make([]string, 0, len(namespaceConfigKeyTypes))
	for k := range namespaceConfigKeyTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// validateNamespaceConfigOverrides rejects unknown keys and values of the
// wrong JSON type, so a typo'd key never reaches the database where it would
// silently inherit the default instead of overriding it.
func validateNamespaceConfigOverrides(overrides map[string]any) error {
	for k, v := range overrides {
		want, ok := namespaceConfigKeyTypes[k]
		if !ok {
			return fmt.Errorf("brain: unknown namespace config key %q (known: %v)", k, KnownNamespaceConfigKeys())
		}
		if v == nil {
			continue
		}
		switch want {
		case "number":
			if _, ok := v.(float64); !ok {
				return fmt.Errorf("brain: namespace config %q must be a number, got %T", k, v)
			}
		case "integer":
			f, ok := v.(float64)
			if !ok || f != float64(int64(f)) {
				return fmt.Errorf("brain: namespace config %q must be an integer, got %v", k, v)
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("brain: namespace config %q must be a boolean, got %T", k, v)
			}
		}
	}
	return nil
}

// EffectiveNamespaceConfig is the merged configuration for one namespace:
// the namespace's own overrides on top of every ancestor's overrides (nearest
// ancestor wins), with the process Config as the fallback for every key.
type EffectiveNamespaceConfig struct {
	overrides map[string]any
	defaults  Config
}

// Overrides returns the merged override map (ancestors included).
func (e *EffectiveNamespaceConfig) Overrides() map[string]any {
	out := make(map[string]any, len(e.overrides))
	for k, v := range e.overrides {
		out[k] = v
	}
	return out
}

// Effective returns every known key with its resolved value: the nearest
// override when set, otherwise the global default.
func (e *EffectiveNamespaceConfig) Effective() map[string]any {
	out := map[string]any{}
	for _, k := range KnownNamespaceConfigKeys() {
		out[k] = e.get(k)
	}
	return out
}

func (e *EffectiveNamespaceConfig) get(key string) any {
	if v, ok := e.overrides[key]; ok && v != nil {
		return v
	}
	switch key {
	case KeyConsolidationSimilarityThreshold:
		return e.defaults.SimilarityThreshold
	case KeyConsolidationDedupThreshold:
		return e.defaults.DedupThreshold
	case KeyConsolidationBatchSize:
		return e.defaults.BatchSize
	case KeyConsolidationMaxLLMCallsPerRun:
		return e.defaults.MaxLLMCallsPerRun
	case KeyRecallHybrid:
		return e.defaults.HybridRecall
	case KeyRecallMinSimilarity:
		return e.defaults.MinRecallSimilarity
	case KeyRecallRecencyWeight:
		return e.defaults.RecencyWeight
	case KeyRememberSocialFilter:
		return e.defaults.SocialFilter
	case KeyRememberMinEpisodeLength:
		return e.defaults.MinEpisodeLength
	}
	return nil
}

// SimilarityThreshold is the consolidation clustering threshold for this namespace.
func (e *EffectiveNamespaceConfig) SimilarityThreshold() float64 {
	if v, ok := e.overrides[KeyConsolidationSimilarityThreshold].(float64); ok {
		return v
	}
	return e.defaults.SimilarityThreshold
}

// DedupThreshold is the consolidation dedup threshold for this namespace.
func (e *EffectiveNamespaceConfig) DedupThreshold() float64 {
	if v, ok := e.overrides[KeyConsolidationDedupThreshold].(float64); ok {
		return v
	}
	return e.defaults.DedupThreshold
}

// BatchSize is the consolidation batch size for this namespace.
func (e *EffectiveNamespaceConfig) BatchSize() int {
	if v, ok := e.overrides[KeyConsolidationBatchSize].(float64); ok {
		return int(v)
	}
	return e.defaults.BatchSize
}

// MaxLLMCallsPerRun caps reasoner calls in one consolidation run for this namespace.
func (e *EffectiveNamespaceConfig) MaxLLMCallsPerRun() int {
	if v, ok := e.overrides[KeyConsolidationMaxLLMCallsPerRun].(float64); ok {
		return int(v)
	}
	return e.defaults.MaxLLMCallsPerRun
}

// HybridRecall reports whether recall in this namespace fuses full-text search.
func (e *EffectiveNamespaceConfig) HybridRecall() bool {
	if v, ok := e.overrides[KeyRecallHybrid].(bool); ok {
		return v
	}
	return e.defaults.HybridRecall
}

// MinRecallSimilarity is the minimum cosine similarity for vector-only recall results.
func (e *EffectiveNamespaceConfig) MinRecallSimilarity() float32 {
	if v, ok := e.overrides[KeyRecallMinSimilarity].(float64); ok {
		return float32(v)
	}
	return e.defaults.MinRecallSimilarity
}

// RecencyWeight is the recall recency weighting for this namespace.
func (e *EffectiveNamespaceConfig) RecencyWeight() float64 {
	if v, ok := e.overrides[KeyRecallRecencyWeight].(float64); ok {
		return v
	}
	return e.defaults.RecencyWeight
}

// SocialFilter reports whether Remember refuses greetings in this namespace.
func (e *EffectiveNamespaceConfig) SocialFilter() bool {
	if v, ok := e.overrides[KeyRememberSocialFilter].(bool); ok {
		return v
	}
	return e.defaults.SocialFilter
}

// MinEpisodeLength is the shortest episode worth extracting facts from.
func (e *EffectiveNamespaceConfig) MinEpisodeLength() int {
	if v, ok := e.overrides[KeyRememberMinEpisodeLength].(float64); ok {
		return int(v)
	}
	return e.defaults.MinEpisodeLength
}

// namespaceRunConfig resolves the effective configuration for the namespace
// with the given ID and returns it as a Config the consolidation pipeline can
// use directly. A lookup failure falls back to the process config, so a broken
// override row never blocks a consolidation run.
func (b *Brain) namespaceRunConfig(ctx context.Context, nsID int64) Config {
	cfg := b.config
	var slug string
	if err := b.pool.QueryRow(ctx, "SELECT slug FROM namespaces WHERE id = $1", nsID).Scan(&slug); err != nil {
		return cfg
	}
	eff, err := b.EffectiveNamespaceConfig(ctx, slug)
	if err != nil || eff == nil {
		return cfg
	}
	cfg.SimilarityThreshold = eff.SimilarityThreshold()
	cfg.DedupThreshold = eff.DedupThreshold()
	cfg.BatchSize = eff.BatchSize()
	cfg.MaxLLMCallsPerRun = eff.MaxLLMCallsPerRun()
	cfg.MinEpisodeLength = eff.MinEpisodeLength()
	return cfg
}

// namespaceAncestorPaths returns the override-resolution chain for slug, from
// the root down to the namespace itself: ["/", "/clients", "/clients/white-vision"].
func namespaceAncestorPaths(slug string) []string {
	paths := []string{"/"}
	paths = append(paths, parentPaths(slug)...)
	if slug != "/" {
		paths = append(paths, slug)
	}
	return paths
}

// EffectiveNamespaceConfig resolves the configuration for one namespace: its
// own overrides merged over every ancestor's (nearest ancestor wins), with the
// process Config as the default for every key.
func (b *Brain) EffectiveNamespaceConfig(ctx context.Context, slug string) (*EffectiveNamespaceConfig, error) {
	if err := validatePath(slug); err != nil {
		return nil, err
	}
	paths := namespaceAncestorPaths(slug)

	rows, err := b.pool.Query(ctx,
		`SELECT n.slug, nc.overrides
		   FROM namespace_configs nc
		   JOIN namespaces n ON n.id = nc.namespace_id
		  WHERE n.slug = ANY($1)`,
		paths,
	)
	if err != nil {
		return nil, fmt.Errorf("namespace config: load overrides: %w", err)
	}
	defer rows.Close()

	bySlug := map[string]map[string]any{}
	for rows.Next() {
		var slug string
		var raw []byte
		if err := rows.Scan(&slug, &raw); err != nil {
			return nil, fmt.Errorf("namespace config: scan overrides: %w", err)
		}
		var overrides map[string]any
		if err := json.Unmarshal(raw, &overrides); err != nil {
			return nil, fmt.Errorf("namespace config: decode overrides for %s: %w", slug, err)
		}
		bySlug[slug] = overrides
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("namespace config: read overrides: %w", err)
	}

	merged := map[string]any{}
	for _, p := range paths {
		for k, v := range bySlug[p] {
			merged[k] = v
		}
	}
	return &EffectiveNamespaceConfig{overrides: merged, defaults: b.config}, nil
}

// GetNamespaceConfig returns the effective configuration and the namespace's
// own overrides for one namespace.
func (b *Brain) GetNamespaceConfig(ctx context.Context, slug string) (*models.NamespaceConfigResponse, error) {
	if err := validatePath(slug); err != nil {
		return nil, err
	}
	ns, err := b.GetNamespace(ctx, slug)
	if err != nil {
		return nil, err
	}

	eff, err := b.EffectiveNamespaceConfig(ctx, slug)
	if err != nil {
		return nil, err
	}

	var own map[string]any
	var updatedAt = ns.UpdatedAt
	err = b.pool.QueryRow(ctx,
		"SELECT overrides, updated_at FROM namespace_configs WHERE namespace_id = $1",
		ns.ID,
	).Scan(&own, &updatedAt)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("namespace config: read own overrides: %w", err)
	}
	if own == nil {
		own = map[string]any{}
	}

	return &models.NamespaceConfigResponse{
		NamespaceID: ns.ID,
		Slug:        ns.Slug,
		Effective:   eff.Effective(),
		Overrides:   own,
		UpdatedAt:   updatedAt,
	}, nil
}

// SetNamespaceConfig replaces the namespace's own overrides. The namespace is
// created if it does not exist yet, matching CreateNamespace's mkdir -p
// behavior for parents.
func (b *Brain) SetNamespaceConfig(ctx context.Context, slug string, overrides map[string]any) error {
	if err := requireWrite(ctx); err != nil {
		return err
	}
	if err := validatePath(slug); err != nil {
		return err
	}
	if overrides == nil {
		overrides = map[string]any{}
	}
	if err := validateNamespaceConfigOverrides(overrides); err != nil {
		return err
	}

	nsID, err := b.NamespaceIDByPath(ctx, slug)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("namespace config: encode overrides: %w", err)
	}
	_, err = b.pool.Exec(ctx,
		`INSERT INTO namespace_configs (namespace_id, overrides)
		 VALUES ($1, $2::jsonb)
		 ON CONFLICT (namespace_id) DO UPDATE SET overrides = EXCLUDED.overrides, updated_at = now()`,
		nsID, raw,
	)
	if err != nil {
		return fmt.Errorf("namespace config: save overrides: %w", err)
	}
	return nil
}

// DeleteNamespaceConfig removes the namespace's own overrides, so it inherits
// the global default (and any ancestor overrides) again.
func (b *Brain) DeleteNamespaceConfig(ctx context.Context, slug string) error {
	if err := requireWrite(ctx); err != nil {
		return err
	}
	if err := validatePath(slug); err != nil {
		return err
	}
	nsID, err := b.NamespaceIDByPath(ctx, slug)
	if err != nil {
		return err
	}
	_, err = b.pool.Exec(ctx, "DELETE FROM namespace_configs WHERE namespace_id = $1", nsID)
	if err != nil {
		return fmt.Errorf("namespace config: delete overrides: %w", err)
	}
	return nil
}

// DeleteNamespaceConfigKey removes one override key from the namespace,
// leaving the other overrides intact.
func (b *Brain) DeleteNamespaceConfigKey(ctx context.Context, slug, key string) error {
	if err := requireWrite(ctx); err != nil {
		return err
	}
	if _, ok := namespaceConfigKeyTypes[key]; !ok {
		return fmt.Errorf("brain: unknown namespace config key %q (known: %v)", key, KnownNamespaceConfigKeys())
	}
	nsID, err := b.NamespaceIDByPath(ctx, slug)
	if err != nil {
		return err
	}
	_, err = b.pool.Exec(ctx,
		"UPDATE namespace_configs SET overrides = overrides - $2, updated_at = now() WHERE namespace_id = $1",
		nsID, key,
	)
	if err != nil {
		return fmt.Errorf("namespace config: delete key: %w", err)
	}
	return nil
}
