// Modifications copyright (c) 2026 Bojan van der Heide

// Package reranker scores query-document pairs with a cross-encoder and
// re-orders the fused recall candidates. The HTTP shape follows the
// Cohere /rerank contract, which Jina, SiliconFlow, Llama.cpp and Ollama
// compatible servers speak as well. An empty endpoint or model disables
// reranking entirely: recall then behaves exactly as before.
package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Candidate is one fused recall result the cross-encoder will judge.
type Candidate struct {
	Content string
	// Fused is the score the candidate already has (RRF). It is only used for
	// the fallback ordering when the reranker is unavailable or fails.
	Fused float32
}

// Scored pairs a candidate index with its cross-encoder relevance.
type Scored struct {
	Index int
	Score float32
}

// Reranker scores candidates against a query as pairs.
type Reranker interface {
	// Rerank scores every candidate against the query, best first.
	Rerank(ctx context.Context, query string, candidates []Candidate) ([]Scored, error)
}

// HTTP calls a Cohere-compatible /rerank endpoint.
type HTTP struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewHTTP builds a reranker for a Cohere-compatible endpoint. An empty model
// returns nil: the caller treats a nil reranker as "off".
func NewHTTP(baseURL, apiKey, model string) *HTTP {
	if model == "" {
		return nil
	}
	return &HTTP{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

type request struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
	MaxChunks int      `json:"max_chunks_per_doc"`
	// ReturnDocuments=false keeps the reply small; the caller already holds
	// the texts and only needs the index/score pairs.
	ReturnDocuments bool `json:"return_documents"`
}

type response struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float32 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank scores every candidate against the query. The reply may cover fewer
// candidates than were sent (some servers cap), so the caller falls back to the
// fused score for anything the cross-encoder did not judge.
func (h *HTTP) Rerank(ctx context.Context, query string, candidates []Candidate) ([]Scored, error) {
	docs := make([]string, len(candidates))
	for i, c := range candidates {
		docs[i] = c.Content
	}
	body, err := json.Marshal(request{
		Model: h.model, Query: query, Documents: docs,
		TopN: len(docs), ReturnDocuments: false,
	})
	if err != nil {
		return nil, fmt.Errorf("reranker: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reranker: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reranker: call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("reranker: %s: %s", resp.Status, string(msg))
	}
	var out response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("reranker: decode reply: %w", err)
	}
	scored := make([]Scored, 0, len(out.Results))
	for _, r := range out.Results {
		if r.Index >= 0 && r.Index < len(candidates) {
			scored = append(scored, Scored{Index: r.Index, Score: r.RelevanceScore})
		}
	}
	return scored, nil
}
