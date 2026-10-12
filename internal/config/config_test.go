package config

import (
	"path/filepath"
	"testing"
)

// setRequired sets every field the config marks `required`, so a test can
// isolate the optional ones.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("STASH_POSTGRES_DSN", "postgres://stash:stash@localhost:5432/stash?sslmode=disable")
	t.Setenv("STASH_VECTOR_DIM", "1536")
	t.Setenv("STASH_MAX_RESULT_SIZE", "10000")
	t.Setenv("STASH_OPENAI_API_KEY", "test-key")
	t.Setenv("STASH_OPENAI_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("STASH_EMBEDDING_MODEL", "text-embedding-3-small")
	t.Setenv("STASH_REASONER_MODEL", "gpt-4o-mini")
	t.Setenv("STASH_CONTEXT_TTL", "1h")
	t.Setenv("STASH_HTTP_ADDR", ":8080")
	t.Setenv("STASH_LOG_LEVEL", "info")
	t.Setenv("STASH_LOG_FORMAT", "text")
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
	t.Setenv("STASH_EMBEDDING_BASE_URL", "")
	t.Setenv("STASH_EMBEDDING_API_KEY", "")

	cfg := parseWithoutFile(t)
	if cfg.EmbeddingBaseURL != "" || cfg.EmbeddingAPIKey != "" {
		t.Fatalf("expected empty embedding overrides, got %q / %q", cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey)
	}
}

func TestEmbeddingOverridesAreReadWhenSet(t *testing.T) {
	setRequired(t)
	t.Setenv("STASH_EMBEDDING_BASE_URL", "http://ollama:11434/v1")
	t.Setenv("STASH_EMBEDDING_API_KEY", "ollama")

	cfg := parseWithoutFile(t)
	if cfg.EmbeddingBaseURL != "http://ollama:11434/v1" || cfg.EmbeddingAPIKey != "ollama" {
		t.Fatalf("embedding overrides not read: %q / %q", cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey)
	}
}

// STASH_API_TOKEN is optional: unset must parse cleanly (auth off), set is read.
func TestAPITokenIsOptional(t *testing.T) {
	setRequired(t)
	if cfg := parseWithoutFile(t); cfg.APIToken != "" {
		t.Fatalf("expected empty APIToken, got %q", cfg.APIToken)
	}
	t.Setenv("STASH_API_TOKEN", "s3cret")
	if cfg := parseWithoutFile(t); cfg.APIToken != "s3cret" {
		t.Fatalf("APIToken = %q, want s3cret", cfg.APIToken)
	}
}
