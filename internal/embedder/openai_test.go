package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewOpenAIValidatesInput(t *testing.T) {
	cases := []struct {
		name   string
		apiKey string
		model  string
		dims   int
	}{
		{"no api key", "", "text-embedding-3-small", 1536},
		{"no model", "sk-test", "", 1536},
		{"no dims", "sk-test", "text-embedding-3-small", 0},
		{"negative dims", "sk-test", "text-embedding-3-small", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewOpenAI("https://example.invalid/v1", tc.apiKey, tc.model, tc.dims); err == nil {
				t.Fatal("NewOpenAI accepted an invalid configuration")
			}
		})
	}
}

func TestOpenAIModelAndDims(t *testing.T) {
	e, err := NewOpenAI("https://example.invalid/v1", "sk-test", "openai/text-embedding-3-small", 1536)
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	if got := e.Model(); got != "openai/text-embedding-3-small" {
		t.Fatalf("Model() = %q; the model string must be passed through unchanged", got)
	}
	if got := e.Dims(); got != 1536 {
		t.Fatalf("Dims() = %d; want 1536", got)
	}
}

func TestOpenAIEmbedCallsEndpointAndConverts(t *testing.T) {
	var gotPath, gotModel string
	var gotDims int64
	var gotInput []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		var req struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions int64    `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotModel, gotInput, gotDims = req.Model, req.Input, req.Dimensions

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [{"object": "embedding", "index": 0, "embedding": [0.25, -0.5, 1.0]}],
			"model": "test-model",
			"usage": {"prompt_tokens": 1, "total_tokens": 1}
		}`))
	}))
	defer srv.Close()

	e, err := NewOpenAI(srv.URL+"/v1", "sk-test", "test-model", 3)
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}

	vec, err := e.Embed(context.Background(), "the queue is kafka")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if gotPath != "/v1/embeddings" {
		t.Fatalf("path = %q; want /v1/embeddings", gotPath)
	}
	if gotModel != "test-model" || gotDims != 3 {
		t.Fatalf("request model=%q dims=%d; want test-model/3", gotModel, gotDims)
	}
	if len(gotInput) != 1 || gotInput[0] != "the queue is kafka" {
		t.Fatalf("request input = %v; want the text as a single-element array", gotInput)
	}

	want := []float32{0.25, -0.5, 1.0}
	if len(vec) != len(want) {
		t.Fatalf("vector length = %d; want %d", len(vec), len(want))
	}
	for i := range want {
		if vec[i] != want[i] {
			t.Fatalf("vector = %v; want %v", vec, want)
		}
	}
}

func TestOpenAIEmbedPropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": {"message": "invalid api key"}}`))
	}))
	defer srv.Close()

	e, err := NewOpenAI(srv.URL+"/v1", "sk-wrong", "test-model", 3)
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	if _, err := e.Embed(context.Background(), "anything"); err == nil {
		t.Fatal("Embed returned no error for a 401 response")
	} else if !strings.Contains(strings.ToLower(err.Error()), "api") && !strings.Contains(err.Error(), "401") {
		t.Logf("error was: %v", err)
	}
}
