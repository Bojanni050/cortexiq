// Modifications copyright (c) 2026 Bojan van der Heide
package llmreg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaskedKeepsOnlyTheTail(t *testing.T) {
	p := Provider{Slug: "deepinfra", APIKey: "sk-abcdefgh1234"}
	m := p.Masked()
	if m.APIKey != "" {
		t.Fatalf("masked provider still carries the key: %q", m.APIKey)
	}
	if m.KeyHint != "…1234" {
		t.Fatalf("key hint = %q, want …1234", m.KeyHint)
	}
	if !p.HasKey() {
		t.Fatal("HasKey = false on a provider with a key")
	}
	if (Provider{}).HasKey() {
		t.Fatal("HasKey = true without a key")
	}
	if short := (Provider{APIKey: "ab"}).Masked(); short.KeyHint != "…" {
		t.Fatalf("short key hint = %q, want …", short.KeyHint)
	}
}

func TestParseDeepInfraCatalogue(t *testing.T) {
	body := `[{"model_name":"Qwen/Qwen3-14B","type":"text-generation",
	"description":" Qwen3 is the latest. ","tags":["tools","reasoning"],
	"max_tokens":40960,"deprecated":null,
	"pricing":{"cents_per_input_token":1.2e-05,"cents_per_output_token":2.4e-05}}]`
	models, err := parseCatalogue([]byte(body), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models", len(models))
	}
	m := models[0]
	if m.ID != "Qwen/Qwen3-14B" || m.Kind != "text-generation" {
		t.Fatalf("id/kind = %q/%q", m.ID, m.Kind)
	}
	if m.Description != "Qwen3 is the latest." {
		t.Fatalf("description = %q", m.Description)
	}
	if m.Context != 40960 {
		t.Fatalf("context = %d", m.Context)
	}
	// 1.2e-05 cents per token is 0.12 dollars per million tokens.
	if m.InputPerMTok < 0.119 || m.InputPerMTok > 0.121 {
		t.Fatalf("input price = %v, want 0.12", m.InputPerMTok)
	}
	if m.OutputPerMTok < 0.239 || m.OutputPerMTok > 0.241 {
		t.Fatalf("output price = %v, want 0.24", m.OutputPerMTok)
	}
	if m.Deprecated {
		t.Fatal("null deprecated parsed as true")
	}
	if len(m.Capabilities) != 2 {
		t.Fatalf("capabilities = %v", m.Capabilities)
	}
}

func TestParseOpenAICatalogue(t *testing.T) {
	body := `{"object":"list","data":[
	{"id":"mistral-large-latest","name":"Mistral Large","description":"","max_context_length":131072,
	 "capabilities":{"completion_chat":true,"function_calling":true,"vision":false}},
	{"id":"mistral-embed","max_context_length":8192,
	 "pricing":{"input_per_mtok":0.1,"output_per_mtok":0}}]}`
	models, err := parseCatalogue([]byte(body), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models", len(models))
	}
	// Sorted by id: mistral-embed before mistral-large-latest.
	embed, large := models[0], models[1]
	if embed.ID != "mistral-embed" || embed.InputPerMTok != 0.1 {
		t.Fatalf("embed = %+v", embed)
	}
	if large.Context != 131072 {
		t.Fatalf("context = %d", large.Context)
	}
	if large.Description != "Mistral Large" {
		t.Fatalf("description should fall back to name, got %q", large.Description)
	}
	// Only the true capabilities survive, with underscores turned into spaces.
	if len(large.Capabilities) != 2 || large.Capabilities[0] != "completion chat" {
		t.Fatalf("capabilities = %v", large.Capabilities)
	}
	if large.Deprecated {
		t.Fatal("deprecated parsed as true")
	}
}

func TestParseCatalogueRejectsGarbage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		deepInfra bool
	}{
		{"not json", "<html>nope</html>", false},
		{"wrong shape", `{"data":"nope"}`, false},
		{"deepinfra object", `{"model_name":"x"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCatalogue([]byte(tc.body), tc.deepInfra); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestListModelsSendsTheKeyAndReadsBothShapes(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m1","max_context_length":1024}]}`))
	}))
	defer srv.Close()

	models, err := ListModels(context.Background(), srv.Client(), Provider{
		Slug: "mistral", Name: "Mistral", BaseURL: srv.URL + "/v1", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if len(models) != 1 || models[0].ID != "m1" || models[0].Context != 1024 {
		t.Fatalf("models = %+v", models)
	}
}

func TestTestReportsRejectionWithoutTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid API Key"}`))
	}))
	defer srv.Close()

	res, err := Test(context.Background(), srv.Client(), Provider{
		Slug: "mistral", Name: "Mistral", BaseURL: srv.URL + "/v1", APIKey: "sk-secret-value",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("a 401 reported OK")
	}
	if !strings.Contains(res.Message, "rejected the key") {
		t.Fatalf("message = %q", res.Message)
	}
	if strings.Contains(res.Message, "sk-secret-value") {
		t.Fatalf("the message leaked the key: %q", res.Message)
	}
}

func TestTestSucceedsWithACatalogue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
	}))
	defer srv.Close()

	res, err := Test(context.Background(), srv.Client(), Provider{Slug: "p", Name: "P", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Models != 2 {
		t.Fatalf("result = %+v", res)
	}
}

func TestDeepInfraPathMovesToTheHostRoot(t *testing.T) {
	got := modelsPath(Provider{BaseURL: "https://api.deepinfra.com/v1/openai"})
	if got != "https://api.deepinfra.com/models/list" {
		t.Fatalf("deepinfra path = %q", got)
	}
	if other := modelsPath(Provider{BaseURL: "https://api.mistral.ai/v1"}); other != "https://api.mistral.ai/v1/models" {
		t.Fatalf("mistral path = %q", other)
	}
}
