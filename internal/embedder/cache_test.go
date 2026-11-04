package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// fixedEmbedder is the smallest possible Embedder: a stable vector per text.
type fixedEmbedder struct{ model string }

func (f fixedEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0}, nil
}
func (f fixedEmbedder) Model() string { return f.model }
func (f fixedEmbedder) Dims() int     { return 3 }

func TestCacheKeyIsSHA256OfText(t *testing.T) {
	text := "the queue is kafka"
	want := sha256.Sum256([]byte(text))
	if got := cacheKey(text); got != hex.EncodeToString(want[:]) {
		t.Fatalf("cacheKey(%q) = %q; want the sha256 hex digest", text, got)
	}
}

func TestCacheKeyIsStableAndDistinct(t *testing.T) {
	a := cacheKey("the queue is kafka")
	if a != cacheKey("the queue is kafka") {
		t.Fatal("cacheKey is not deterministic")
	}
	if a == cacheKey("the queue is redis") {
		t.Fatal("cacheKey collided for two different texts")
	}
	if len(a) != 64 {
		t.Fatalf("cacheKey length = %d; want 64 hex characters", len(a))
	}
}

func TestCachedDelegatesMetadata(t *testing.T) {
	c := NewCached(fixedEmbedder{model: "openai/text-embedding-3-small"}, nil)
	if got := c.Model(); got != "openai/text-embedding-3-small" {
		t.Fatalf("Model() = %q; want the wrapped embedder's model", got)
	}
	if got := c.Dims(); got != 3 {
		t.Fatalf("Dims() = %d; want 3", got)
	}
}
