// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateNamespaceConfigOverrides(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		wantErr   string
	}{
		{"empty is valid", map[string]any{}, ""},
		{"nil value is allowed", map[string]any{KeyRecallHybrid: nil}, ""},
		{"number key with float", map[string]any{KeyConsolidationSimilarityThreshold: 0.9}, ""},
		{"integer key with int-valued float", map[string]any{KeyConsolidationBatchSize: float64(50)}, ""},
		{"boolean key with bool", map[string]any{KeyRecallHybrid: false}, ""},
		{"unknown key", map[string]any{"consolidation.typo": 1.0}, `unknown namespace config key "consolidation.typo"`},
		{"number key with string", map[string]any{KeyConsolidationSimilarityThreshold: "high"}, "must be a number"},
		{"integer key with fraction", map[string]any{KeyConsolidationBatchSize: 1.5}, "must be an integer"},
		{"boolean key with string", map[string]any{KeyRecallHybrid: "yes"}, "must be a boolean"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNamespaceConfigOverrides(tc.overrides)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validateNamespaceConfigOverrides(%v) = %v, want nil", tc.overrides, err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("validateNamespaceConfigOverrides(%v) = %v, want error containing %q", tc.overrides, err, tc.wantErr)
			}
		})
	}
}

func TestNamespaceAncestorPaths(t *testing.T) {
	tests := []struct {
		slug string
		want []string
	}{
		{"/", []string{"/"}},
		{"/clients", []string{"/", "/clients"}},
		{"/clients/white-vision", []string{"/", "/clients", "/clients/white-vision"}},
		{"/a/b/c", []string{"/", "/a", "/a/b", "/a/b/c"}},
	}
	for _, tc := range tests {
		got := namespaceAncestorPaths(tc.slug)
		if len(got) != len(tc.want) {
			t.Fatalf("namespaceAncestorPaths(%q) = %v, want %v", tc.slug, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("namespaceAncestorPaths(%q) = %v, want %v", tc.slug, got, tc.want)
			}
		}
	}
}

func TestEffectiveNamespaceConfig(t *testing.T) {
	defaults := DefaultConfig()
	defaults.SimilarityThreshold = 0.85
	defaults.BatchSize = 100
	defaults.HybridRecall = true
	defaults.SocialFilter = true
	defaults.MinEpisodeLength = 24

	t.Run("defaults when no overrides", func(t *testing.T) {
		eff := &EffectiveNamespaceConfig{overrides: map[string]any{}, defaults: defaults}
		if got := eff.SimilarityThreshold(); got != 0.85 {
			t.Fatalf("SimilarityThreshold() = %v, want 0.85", got)
		}
		if got := eff.BatchSize(); got != 100 {
			t.Fatalf("BatchSize() = %v, want 100", got)
		}
		if !eff.HybridRecall() {
			t.Fatal("HybridRecall() = false, want true")
		}
		effective := eff.Effective()
		if len(effective) != len(KnownNamespaceConfigKeys()) {
			t.Fatalf("Effective() has %d keys, want %d", len(effective), len(KnownNamespaceConfigKeys()))
		}
	})

	t.Run("own override wins over default", func(t *testing.T) {
		eff := &EffectiveNamespaceConfig{
			overrides: map[string]any{
				KeyConsolidationSimilarityThreshold: 0.7,
				KeyRecallHybrid:                     false,
			},
			defaults: defaults,
		}
		if got := eff.SimilarityThreshold(); got != 0.7 {
			t.Fatalf("SimilarityThreshold() = %v, want 0.7", got)
		}
		if eff.HybridRecall() {
			t.Fatal("HybridRecall() = true, want false")
		}
		// Untouched keys keep the default.
		if got := eff.BatchSize(); got != 100 {
			t.Fatalf("BatchSize() = %v, want 100", got)
		}
	})

	t.Run("nil override falls back to default", func(t *testing.T) {
		eff := &EffectiveNamespaceConfig{
			overrides: map[string]any{KeyConsolidationBatchSize: nil},
			defaults:  defaults,
		}
		if got := eff.BatchSize(); got != 100 {
			t.Fatalf("BatchSize() = %v, want 100 (nil override must not shadow the default)", got)
		}
	})

	t.Run("Overrides returns a copy", func(t *testing.T) {
		eff := &EffectiveNamespaceConfig{overrides: map[string]any{KeyRecallHybrid: false}, defaults: defaults}
		out := eff.Overrides()
		out[KeyRecallHybrid] = true
		if !eff.overrides[KeyRecallHybrid].(bool) == true {
			// original unchanged means the copy did not alias
		}
		if eff.overrides[KeyRecallHybrid].(bool) != false {
			t.Fatal("Overrides() aliased the internal map")
		}
	})
}

func TestEffectiveNamespaceConfigRequiresValidPath(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.EffectiveNamespaceConfig(e.ctx, "no-leading-slash"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("EffectiveNamespaceConfig(bad path) = %v, want ErrInvalidPath", err)
	}
}

// TestIntegrationNamespaceConfig covers the override CRUD and the ancestor
// merge: a namespace without overrides inherits the default, an ancestor's
// override applies to descendants, and the nearest ancestor wins.
func TestIntegrationNamespaceConfig(t *testing.T) {
	e := newEnv(t, nil)

	parent := e.ns + "/parent"
	child := parent + "/child"
	if _, err := e.b.CreateNamespace(e.ctx, parent, "Parent", ""); err != nil {
		t.Fatalf("CreateNamespace(%q): %v", parent, err)
	}
	if _, err := e.b.CreateNamespace(e.ctx, child, "Child", ""); err != nil {
		t.Fatalf("CreateNamespace(%q): %v", child, err)
	}

	// No overrides anywhere: the effective config equals the global default.
	cfg, err := e.b.GetNamespaceConfig(e.ctx, child)
	if err != nil {
		t.Fatalf("GetNamespaceConfig(%q): %v", child, err)
	}
	if len(cfg.Overrides) != 0 {
		t.Fatalf("fresh namespace has %d overrides, want 0", len(cfg.Overrides))
	}
	if got := cfg.Effective[KeyConsolidationSimilarityThreshold]; got != e.b.config.SimilarityThreshold {
		t.Fatalf("effective similarity threshold = %v, want default %v", got, e.b.config.SimilarityThreshold)
	}

	// An override on the parent applies to the child.
	if err := e.b.SetNamespaceConfig(e.ctx, parent, map[string]any{
		KeyConsolidationSimilarityThreshold: 0.7,
		KeyRecallHybrid:                     false,
	}); err != nil {
		t.Fatalf("SetNamespaceConfig(%q): %v", parent, err)
	}
	cfg, err = e.b.GetNamespaceConfig(e.ctx, child)
	if err != nil {
		t.Fatalf("GetNamespaceConfig(%q): %v", child, err)
	}
	if got := cfg.Effective[KeyConsolidationSimilarityThreshold]; got != 0.7 {
		t.Fatalf("child effective similarity threshold = %v, want 0.7 from parent", got)
	}
	if got := cfg.Effective[KeyRecallHybrid]; got != false {
		t.Fatalf("child effective hybrid recall = %v, want false from parent", got)
	}
	if len(cfg.Overrides) != 0 {
		t.Fatalf("child has %d own overrides, want 0 (the override lives on the parent)", len(cfg.Overrides))
	}

	// The child's own override wins over the parent's.
	if err := e.b.SetNamespaceConfig(e.ctx, child, map[string]any{
		KeyConsolidationSimilarityThreshold: 0.6,
	}); err != nil {
		t.Fatalf("SetNamespaceConfig(%q): %v", child, err)
	}
	cfg, err = e.b.GetNamespaceConfig(e.ctx, child)
	if err != nil {
		t.Fatalf("GetNamespaceConfig(%q): %v", child, err)
	}
	if got := cfg.Effective[KeyConsolidationSimilarityThreshold]; got != 0.6 {
		t.Fatalf("child effective similarity threshold = %v, want 0.6 (own override beats parent)", got)
	}
	// The parent's other key still applies.
	if got := cfg.Effective[KeyRecallHybrid]; got != false {
		t.Fatalf("child effective hybrid recall = %v, want false from parent", got)
	}
	if got := cfg.Overrides[KeyConsolidationSimilarityThreshold]; got != 0.6 {
		t.Fatalf("child own overrides = %v, want similarity threshold 0.6", cfg.Overrides)
	}

	// Deleting one key leaves the other overrides and falls back per key.
	if err := e.b.SetNamespaceConfig(e.ctx, child, map[string]any{
		KeyConsolidationSimilarityThreshold: 0.6,
		KeyRecallHybrid:                     true,
	}); err != nil {
		t.Fatalf("SetNamespaceConfig(%q): %v", child, err)
	}
	if err := e.b.DeleteNamespaceConfigKey(e.ctx, child, KeyConsolidationSimilarityThreshold); err != nil {
		t.Fatalf("DeleteNamespaceConfigKey: %v", err)
	}
	cfg, err = e.b.GetNamespaceConfig(e.ctx, child)
	if err != nil {
		t.Fatalf("GetNamespaceConfig(%q): %v", child, err)
	}
	if _, ok := cfg.Overrides[KeyConsolidationSimilarityThreshold]; ok {
		t.Fatalf("similarity threshold still in own overrides after delete: %v", cfg.Overrides)
	}
	if got := cfg.Effective[KeyConsolidationSimilarityThreshold]; got != 0.7 {
		t.Fatalf("after deleting own key, effective threshold = %v, want 0.7 from parent", got)
	}
	if got := cfg.Effective[KeyRecallHybrid]; got != true {
		t.Fatalf("after deleting one key, hybrid recall = %v, want true (own override kept)", got)
	}

	// Deleting all overrides makes the namespace inherit again.
	if err := e.b.DeleteNamespaceConfig(e.ctx, child); err != nil {
		t.Fatalf("DeleteNamespaceConfig(%q): %v", child, err)
	}
	cfg, err = e.b.GetNamespaceConfig(e.ctx, child)
	if err != nil {
		t.Fatalf("GetNamespaceConfig(%q): %v", child, err)
	}
	if len(cfg.Overrides) != 0 {
		t.Fatalf("after delete, %d overrides remain, want 0", len(cfg.Overrides))
	}
	if got := cfg.Effective[KeyConsolidationSimilarityThreshold]; got != 0.7 {
		t.Fatalf("after delete, effective threshold = %v, want 0.7 from parent", got)
	}

	// Unknown keys and wrong types are rejected before any write.
	if err := e.b.SetNamespaceConfig(e.ctx, child, map[string]any{"nope": 1.0}); err == nil {
		t.Fatal("SetNamespaceConfig with unknown key succeeded, want error")
	}
	if err := e.b.SetNamespaceConfig(e.ctx, child, map[string]any{KeyRecallHybrid: "yes"}); err == nil {
		t.Fatal("SetNamespaceConfig with wrong type succeeded, want error")
	}
	if err := e.b.DeleteNamespaceConfigKey(e.ctx, child, "nope"); err == nil {
		t.Fatal("DeleteNamespaceConfigKey with unknown key succeeded, want error")
	}
}

// TestNamespaceRunConfigFallsBackToGlobal verifies that a namespace without
// overrides resolves to exactly the process config, and that a namespace with
// an override gets only that key changed.
func TestNamespaceRunConfigFallsBackToGlobal(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.SimilarityThreshold = 0.77 })

	cfg := e.b.namespaceRunConfig(e.ctx, e.nsID)
	if cfg.SimilarityThreshold != 0.77 {
		t.Fatalf("no overrides: threshold = %v, want the global 0.77", cfg.SimilarityThreshold)
	}
	if cfg.BatchSize != e.b.config.BatchSize {
		t.Fatalf("no overrides: batch size = %v, want the global %v", cfg.BatchSize, e.b.config.BatchSize)
	}

	if err := e.b.SetNamespaceConfig(e.ctx, e.ns, map[string]any{
		KeyConsolidationSimilarityThreshold: 0.6,
	}); err != nil {
		t.Fatalf("SetNamespaceConfig: %v", err)
	}
	cfg = e.b.namespaceRunConfig(e.ctx, e.nsID)
	if cfg.SimilarityThreshold != 0.6 {
		t.Fatalf("with override: threshold = %v, want 0.6", cfg.SimilarityThreshold)
	}
	if cfg.BatchSize != e.b.config.BatchSize {
		t.Fatalf("with override: batch size = %v, want the global %v (untouched key)", cfg.BatchSize, e.b.config.BatchSize)
	}
}

// TestRememberHonorsNamespaceSocialFilter checks that the social filter is
// per-namespace: the global default refuses a greeting, an override on the
// namespace allows it through (the write then needs a real embedder + pool,
// which newEnv provides).
func TestRememberHonorsNamespaceSocialFilter(t *testing.T) {
	e := newEnv(t, nil)

	// Global default: social filter on, the greeting is refused.
	if _, err := e.b.Remember(e.ctx, e.ns, "dank je wel!", nil); !errors.Is(err, ErrSocialCloser) {
		t.Fatalf("default: err = %v, want ErrSocialCloser", err)
	}

	// Override on this namespace: filter off, the write goes through.
	if err := e.b.SetNamespaceConfig(e.ctx, e.ns, map[string]any{
		KeyRememberSocialFilter: false,
	}); err != nil {
		t.Fatalf("SetNamespaceConfig: %v", err)
	}
	id, err := e.b.Remember(e.ctx, e.ns, "dank je wel!", nil)
	if err != nil {
		t.Fatalf("override off: Remember = %v, want the write to go through", err)
	}
	if id == 0 {
		t.Fatal("override off: episode id = 0, want a stored episode")
	}

	// A child of the overridden namespace inherits the override: the nearest
	// ancestor that sets a key wins, so the filter stays off there.
	child := e.ns + "/child"
	if _, err := e.b.CreateNamespace(e.ctx, child, "Child", ""); err != nil {
		t.Fatalf("CreateNamespace(%q): %v", child, err)
	}
	if _, err := e.b.Remember(e.ctx, child, "dank je wel!", nil); err != nil {
		t.Fatalf("child inheriting the override: err = %v, want the write to go through", err)
	}

	// A namespace outside that subtree has no override anywhere on its path, so
	// the global default refuses.
	outside := e.ns + "-outside"
	if _, err := e.b.CreateNamespace(e.ctx, outside, "Outside", ""); err != nil {
		t.Fatalf("CreateNamespace(%q): %v", outside, err)
	}
	if _, err := e.b.Remember(e.ctx, outside, "dank je wel!", nil); !errors.Is(err, ErrSocialCloser) {
		t.Fatalf("outside the override: err = %v, want ErrSocialCloser", err)
	}
}
