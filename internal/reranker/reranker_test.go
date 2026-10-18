// Modifications copyright (c) 2026 Bojan van der Heide
package reranker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPWithoutModelIsOff(t *testing.T) {
	if NewHTTP("http://x", "key", "") != nil {
		t.Error("empty model must disable the reranker")
	}
}

func TestRerankScoresCandidates(t *testing.T) {
	var gotBody struct {
		Model     string   `json:"model"`
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
		TopN      int      `json:"top_n"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Errorf("path = %q, want /rerank", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth = %q, want Bearer k", got)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"results":[{"index":2,"relevance_score":0.9},{"index":0,"relevance_score":0.4}]}`))
	}))
	defer srv.Close()

	rr := NewHTTP(srv.URL, "k", "jina-reranker-v2")
	scored, err := rr.Rerank(context.Background(), "q", []Candidate{
		{Content: "a", Fused: 0.8}, {Content: "b", Fused: 0.6}, {Content: "c", Fused: 0.4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody.Model != "jina-reranker-v2" || gotBody.Query != "q" || gotBody.TopN != 3 {
		t.Errorf("request = %+v", gotBody)
	}
	if len(gotBody.Documents) != 3 || gotBody.Documents[2] != "c" {
		t.Errorf("documents = %v", gotBody.Documents)
	}
	want := []Scored{{Index: 2, Score: 0.9}, {Index: 0, Score: 0.4}}
	if len(scored) != len(want) {
		t.Fatalf("scored = %+v", scored)
	}
	for i := range want {
		if scored[i] != want[i] {
			t.Errorf("scored[%d] = %+v, want %+v", i, scored[i], want[i])
		}
	}
}

func TestRerankOutOfBoundIndexIsDropped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"index":7,"relevance_score":1}]}`))
	}))
	defer srv.Close()
	scored, err := NewHTTP(srv.URL, "", "m").Rerank(context.Background(), "q", []Candidate{{Content: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(scored) != 0 {
		t.Errorf("scored = %+v, want empty", scored)
	}
}

func TestRerankSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no model", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := NewHTTP(srv.URL, "", "m").Rerank(context.Background(), "q", []Candidate{{Content: "a"}}); err == nil {
		t.Error("want error on 404")
	}
}
