// Modifications copyright (c) 2026 Bojan van der Heide
package llmreg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Provider is one OpenAI-compatible endpoint CortexIQ can call. The key is
// write-only: it goes in, it never comes back out of the API.
type Provider struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"-"`

	// KeyHint is the only trace of a stored key the dashboard receives: four
	// characters, enough to tell which key is loaded, useless to anyone else.
	KeyHint string    `json:"key_hint,omitempty"`
	Created time.Time `json:"created_at,omitempty"`
	Updated time.Time `json:"updated_at,omitempty"`
}

// Masked returns the provider as the API is allowed to show it.
func (p Provider) Masked() Provider {
	if len(p.APIKey) >= 4 {
		p.KeyHint = "…" + p.APIKey[len(p.APIKey)-4:]
	} else if p.APIKey != "" {
		p.KeyHint = "…"
	}
	p.APIKey = ""
	return p
}

// HasKey reports whether a key is stored, without exposing it.
func (p Provider) HasKey() bool { return strings.TrimSpace(p.APIKey) != "" }

// Model is one entry of a provider's catalogue, in the shape the dashboard
// needs. Anything the provider does not publish stays zero: an unknown price
// and an unknown context length are honest, a guessed one is not.
type Model struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Context      int      `json:"context,omitempty"`
	// Prices are dollars per million tokens, converted from whatever unit the
	// provider uses.
	InputPerMTok  float64 `json:"input_per_mtok,omitempty"`
	OutputPerMTok float64 `json:"output_per_mtok,omitempty"`
	Deprecated    bool    `json:"deprecated,omitempty"`
	ReplacedBy    string  `json:"replaced_by,omitempty"`
}

// TestResult is what a connection check reports back.
type TestResult struct {
	OK       bool   `json:"ok"`
	Status   int    `json:"status"`
	Models   int    `json:"models"`
	Message  string `json:"message,omitempty"`
	Endpoint string `json:"endpoint"`
}

// ErrModelRequired means a binding was saved without naming a model.
var ErrModelRequired = fmt.Errorf("llmreg: a model is required")

// modelsPath is where a provider lists its catalogue. DeepInfra publishes a
// richer catalogue on the host root (pricing, tags, descriptions) while its
// OpenAI-compatible path only has the standard endpoint, so the two differ.
func modelsPath(p Provider) string {
	base := strings.TrimRight(p.BaseURL, "/")
	if strings.Contains(base, "deepinfra.com") {
		u, err := url.Parse(base)
		if err == nil {
			u.Path = "/models/list"
			u.RawQuery = ""
			return u.String()
		}
	}
	return base + "/models"
}

// ListModels fetches and normalises a provider's catalogue.
func ListModels(ctx context.Context, c *http.Client, p Provider) ([]Model, error) {
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	endpoint := modelsPath(p)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	if p.HasKey() {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("list models: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models: %s returned %d: %s", p.Name, resp.StatusCode, brief(body))
	}
	models, err := parseCatalogue(body, strings.Contains(endpoint, "/models/list"))
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("list models: %s returned no models", p.Name)
	}
	return models, nil
}

// Test asks a provider for its catalogue and reports what came back. It is the
// same call ListModels makes, because a provider that answers with a list has
// both a working key and a working URL; anything else is noise.
func Test(ctx context.Context, c *http.Client, p Provider) (TestResult, error) {
	res := TestResult{Endpoint: modelsPath(p)}
	models, err := ListModels(ctx, c, p)
	if err != nil {
		res.Message = err.Error()
		if strings.Contains(res.Message, "401") || strings.Contains(res.Message, "403") {
			res.Message = "the provider rejected the key"
		}
		return res, nil
	}
	res.OK = true
	res.Status = http.StatusOK
	res.Models = len(models)
	res.Message = fmt.Sprintf("%d models", len(models))
	return res, nil
}

// parseCatalogue understands both shapes we meet in the wild: DeepInfra's flat
// list with pricing and tags, and the OpenAI/Mistral envelope with a data array.
func parseCatalogue(body []byte, deepInfra bool) ([]Model, error) {
	if deepInfra {
		var rows []struct {
			Name        string   `json:"model_name"`
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
			MaxTokens   int      `json:"max_tokens"`
			Deprecated  any      `json:"deprecated"`
			ReplacedBy  string   `json:"replaced_by"`
			Pricing     struct {
				Input  float64 `json:"cents_per_input_token"`
				Output float64 `json:"cents_per_output_token"`
			} `json:"pricing"`
		}
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, fmt.Errorf("list models: decode: %w", err)
		}
		out := make([]Model, 0, len(rows))
		for _, r := range rows {
			out = append(out, Model{
				ID:            r.Name,
				Kind:          r.Type,
				Description:   strings.TrimSpace(r.Description),
				Capabilities:  r.Tags,
				Context:       r.MaxTokens,
				InputPerMTok:  centsPerMTok(r.Pricing.Input),
				OutputPerMTok: centsPerMTok(r.Pricing.Output),
				Deprecated:    truthy(r.Deprecated),
				ReplacedBy:    r.ReplacedBy,
			})
		}
		sortModels(out)
		return out, nil
	}

	var envelope struct {
		Data []struct {
			ID            string          `json:"id"`
			Name          string          `json:"name"`
			Description   string          `json:"description"`
			MaxContext    int             `json:"max_context_length"`
			Deprecated    bool            `json:"deprecated"`
			Capabilities  json.RawMessage `json:"capabilities"`
			Pricing       json.RawMessage `json:"pricing"`
			Type          string          `json:"type"`
			OwnedBy       string          `json:"owned_by"`
			ContextLength int             `json:"context_length"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("list models: decode: %w", err)
	}
	out := make([]Model, 0, len(envelope.Data))
	for _, r := range envelope.Data {
		m := Model{
			ID:          r.ID,
			Kind:        r.Type,
			Description: strings.TrimSpace(r.Description),
			Context:     firstNonZero(r.MaxContext, r.ContextLength),
			Deprecated:  r.Deprecated,
		}
		if m.Description == "" && r.Name != "" && r.Name != r.ID {
			m.Description = r.Name
		}
		m.Capabilities = capabilities(r.Capabilities)
		if p := modelPricing(r.Pricing); p != nil {
			m.InputPerMTok, m.OutputPerMTok = p[0], p[1]
		}
		out = append(out, m)
	}
	sortModels(out)
	return out, nil
}

// capabilities accepts both a map of booleans (Mistral) and a list of tags.
func capabilities(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	var flags map[string]bool
	if err := json.Unmarshal(raw, &flags); err != nil {
		return nil
	}
	out := make([]string, 0, len(flags))
	for name, on := range flags {
		if on {
			out = append(out, strings.ReplaceAll(name, "_", " "))
		}
	}
	sort.Strings(out)
	return out
}

// modelPricing reads the few price shapes providers actually send. Unknown
// shapes yield nil, so the dashboard shows "unknown" rather than a wrong number.
func modelPricing(raw json.RawMessage) *[2]float64 {
	if len(raw) == 0 {
		return nil
	}
	var p struct {
		Input  float64 `json:"input_per_mtok"`
		Output float64 `json:"output_per_mtok"`
		Prompt float64 `json:"prompt"`
		Comp   float64 `json:"completion"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	in, out := p.Input, p.Output
	if in == 0 && out == 0 {
		in, out = p.Prompt, p.Comp
	}
	if in == 0 && out == 0 {
		return nil
	}
	return &[2]float64{in, out}
}

func centsPerMTok(cents float64) float64 {
	if cents <= 0 {
		return 0
	}
	// cents per token -> dollars per million tokens
	return cents / 100 * 1_000_000
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func sortModels(models []Model) {
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
}

// truthy covers deprecation flags that arrive as a bool, a string or a date.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(t)
		return s != "" && s != "null"
	default:
		return false
	}
}

func brief(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
