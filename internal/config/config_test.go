package config

import (
	"path/filepath"
	"testing"
)

// setRequired sets every field the config marks `required`, so a test can
// isolate the optional ones.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CORTEXIQ_POSTGRES_DSN", "postgres://stash:stash@localhost:5432/stash?sslmode=disable")
	t.Setenv("CORTEXIQ_VECTOR_DIM", "1536")
	t.Setenv("CORTEXIQ_MAX_RESULT_SIZE", "10000")
	t.Setenv("CORTEXIQ_OPENAI_API_KEY", "test-key")
	t.Setenv("CORTEXIQ_OPENAI_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("CORTEXIQ_EMBEDDING_MODEL", "text-embedding-3-small")
	t.Setenv("CORTEXIQ_REASONER_MODEL", "gpt-4o-mini")
	t.Setenv("CORTEXIQ_CONTEXT_TTL", "1h")
	t.Setenv("CORTEXIQ_HTTP_ADDR", ":8080")
	t.Setenv("CORTEXIQ_LOG_LEVEL", "info")
	t.Setenv("CORTEXIQ_LOG_FORMAT", "text")
}

// A missing file is fine (dev/compose pass env directly) — the config just
// parses the environment.
func parseWithoutFile(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewFromFile(filepath.Join(t.TempDir(), "absent.env"))
	if err != nil {
		t.Fatalf("NewFromFile returned an error: %v", err)
	}
	return cfg
}

// Regression: config parsing runs with RequiredIfNoDef, so the embedding
// overrides — which the bootstrap treats as optional fallbacks — must not be
// required. Leaving them unset must parse cleanly and stay empty.
func TestEmbeddingOverridesAreOptional(t *testing.T) {
	setRequired(t)
	t.Setenv("CORTEXIQ_EMBEDDING_BASE_URL", "")
	t.Setenv("CORTEXIQ_EMBEDDING_API_KEY", "")

	cfg := parseWithoutFile(t)
	if cfg.EmbeddingBaseURL != "" || cfg.EmbeddingAPIKey != "" {
		t.Fatalf("expected empty embedding overrides, got %q / %q", cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey)
	}
}

func TestEmbeddingOverridesAreReadWhenSet(t *testing.T) {
	setRequired(t)
	t.Setenv("CORTEXIQ_EMBEDDING_BASE_URL", "http://ollama:11434/v1")
	t.Setenv("CORTEXIQ_EMBEDDING_API_KEY", "ollama")

	cfg := parseWithoutFile(t)
	if cfg.EmbeddingBaseURL != "http://ollama:11434/v1" || cfg.EmbeddingAPIKey != "ollama" {
		t.Fatalf("embedding overrides not read: %q / %q", cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey)
	}
}

// CORTEXIQ_API_TOKEN is optional: unset must parse cleanly (auth off), set is read.
func TestAPITokenIsOptional(t *testing.T) {
	setRequired(t)
	if cfg := parseWithoutFile(t); cfg.APIToken != "" {
		t.Fatalf("expected empty APIToken, got %q", cfg.APIToken)
	}
	t.Setenv("CORTEXIQ_API_TOKEN", "s3cret")
	if cfg := parseWithoutFile(t); cfg.APIToken != "s3cret" {
		t.Fatalf("APIToken = %q, want s3cret", cfg.APIToken)
	}
}

// Post-rebrand: a pre-rebrand .env or deployment (STASH_*, STASHCONFIG) must
// keep working without any edit.
func TestLegacyStashEnvNamesAreRead(t *testing.T) {
	legacy := map[string]string{
		"POSTGRES_DSN":    "postgres://stash:stash@localhost:5432/stash?sslmode=disable",
		"VECTOR_DIM":      "1024",
		"MAX_RESULT_SIZE": "10000",
		"OPENAI_API_KEY":  "legacy-key",
		"OPENAI_BASE_URL": "https://api.openai.com/v1",
		"EMBEDDING_MODEL": "bge-m3",
		"REASONER_MODEL":  "gpt-4o-mini",
		"CONTEXT_TTL":     "2h",
		"HTTP_ADDR":       ":9999",
		"LOG_LEVEL":       "debug",
		"LOG_FORMAT":      "json",
	}
	for name, value := range legacy {
		t.Setenv(EnvPrefix+name, "") // prove the value comes from the legacy name
		t.Setenv(LegacyEnvPrefix+name, value)
	}

	cfg := parseWithoutFile(t)
	if cfg.StoreDSN != legacy["POSTGRES_DSN"] || cfg.VectorDim != 1024 || cfg.LogLevel != "debug" {
		t.Fatalf("legacy STASH_* names not read: %+v", cfg)
	}
	if cfg.ContextTTL.String() != "2h0m0s" || cfg.ReasonerModel != "gpt-4o-mini" {
		t.Fatalf("legacy STASH_* names not read: %+v", cfg)
	}
}

// A current CORTEXIQ_* value always beats the legacy STASH_* name.
func TestCurrentEnvWinsOverLegacy(t *testing.T) {
	setRequired(t)
	t.Setenv(LegacyEnvPrefix+"LOG_LEVEL", "info")
	t.Setenv(EnvPrefix+"LOG_LEVEL", "debug")

	if cfg := parseWithoutFile(t); cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLegacyEnvPairs(t *testing.T) {
	pairs := legacyEnvPairs([]string{
		"STASH_POSTGRES_DSN=postgres://stash@localhost/stash",
		"STASHCONFIG=/opt/stash/.env",
		"STASH_EMPTY=",
	})
	want := map[string]string{
		"CORTEXIQ_POSTGRES_DSN": "postgres://stash@localhost/stash",
		"CORTEXIQCONFIG":        "/opt/stash/.env",
	}
	if len(pairs) != len(want) {
		t.Fatalf("pairs = %v, want %v", pairs, want)
	}
	for _, pair := range pairs {
		if want[pair[0]] != pair[1] {
			t.Fatalf("unexpected pair %v (want %v)", pair, want)
		}
	}

	if got := legacyEnvPairs([]string{"STASH_LOG_LEVEL=info", "CORTEXIQ_LOG_LEVEL=debug"}); len(got) != 0 {
		t.Fatalf("current value must win, got %v", got)
	}
}
