// Modifications copyright (c) 2026 Bojan van der Heide
package webui

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/llmreg"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// ModelEnv is what the environment configured, so the dashboard can show the
// fallback next to the value chosen in the dashboard itself.
type ModelEnv struct {
	Reasoner  string
	Triage    string
	Embedding string
	Reranker  string
	MaxTokens int
}

// modelSources ties a purpose to the environment value that applies when no
// binding exists. Order matters: it is the order the dashboard renders.
type modelPurpose struct {
	Purpose         string `json:"purpose"`
	Env             string `json:"env"`
	Live            bool   `json:"live"`
	RestartRequired bool   `json:"restart_required"`
	Source          string `json:"source"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
}

// providers handles GET (list, masked) and POST (upsert).
func (h *Handler) providers(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("provider registry is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := h.Registry.Providers(r.Context())
		if !hdl(w, err) {
			return
		}
		out := make([]llmreg.Provider, 0, len(list))
		for _, p := range list {
			out = append(out, p.Masked())
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var body struct {
			Slug    string `json:"slug"`
			Name    string `json:"name"`
			BaseURL string `json:"base_url"`
			APIKey  string `json:"api_key"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		saved, err := h.Registry.UpsertProvider(r.Context(), llmreg.Provider{
			Slug:    body.Slug,
			Name:    body.Name,
			BaseURL: body.BaseURL,
			APIKey:  body.APIKey,
		})
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, saved.Masked())
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// providerItem handles /api/v1/providers/{slug}, /{slug}/test and /{slug}/models.
func (h *Handler) providerItem(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("provider registry is not configured"))
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/providers/"), "/")
	slug, action, _ := strings.Cut(rest, "/")
	if slug == "" {
		writeErr(w, http.StatusBadRequest, errors.New("provider slug is required"))
		return
	}
	p, err := h.Registry.Provider(r.Context(), slug)
	if errors.Is(err, llmreg.ErrProviderNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if !hdl(w, err) {
		return
	}

	switch action {
	case "test":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		res, err := llmreg.Test(r.Context(), http.DefaultClient, p)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, res)
	case "models":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		models, err := llmreg.ListModels(r.Context(), http.DefaultClient, p)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, models)
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

// models reports the model in use per purpose, and where the choice came from.
// The environment value is shown alongside, so a dashboard override never hides
// what the deployment was configured with.
func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	purposes, err := h.modelPurposes(r)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, purposes)
}

func (h *Handler) modelPurposes(r *http.Request) ([]modelPurpose, error) {
	env := []modelPurpose{
		{Purpose: llmreg.PurposeReasoner, Env: h.Models.Reasoner, Live: true},
		{Purpose: llmreg.PurposeTriage, Env: h.Models.Triage, Live: true},
		// Embedding and reranker cannot change under a running process: stored
		// vectors carry the embedding model's dimensions, and the reranker has
		// its own provider shape. Saying so here is better than a dropdown that
		// quietly does nothing.
		{Purpose: llmreg.PurposeEmbedding, Env: h.Models.Embedding, RestartRequired: true},
		{Purpose: llmreg.PurposeReranker, Env: h.Models.Reranker, RestartRequired: true},
	}
	bindings := map[string]llmreg.Binding{}
	if h.Registry != nil {
		got, err := h.Registry.Bindings(r.Context())
		if err != nil {
			return nil, err
		}
		bindings = got
	}
	for i := range env {
		if b, ok := bindings[env[i].Purpose]; ok {
			env[i].Source = "dashboard"
			env[i].Provider = b.Provider
			env[i].Model = b.Model
			continue
		}
		env[i].Source = "env"
		env[i].Model = env[i].Env
	}
	return env, nil
}

// modelItem handles PUT and DELETE on /api/v1/models/{purpose}.
func (h *Handler) modelItem(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("provider registry is not configured"))
		return
	}
	purpose := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/models/"), "/")
	switch r.Method {
	case http.MethodDelete:
		if !hdl(w, h.Registry.ClearBinding(r.Context(), purpose)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cleared", "purpose": purpose})
	case http.MethodPut:
		var body struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if purpose == llmreg.PurposeEmbedding {
			writeErr(w, http.StatusConflict, errors.New(
				"the embedding model cannot change under a running instance: stored vectors have the old model's dimensions, so every memory would have to be re-embedded first"))
			return
		}
		if !hdl(w, h.Registry.SetBinding(r.Context(), purpose, body.Provider, body.Model)) {
			return
		}
		live, err := h.applyModel(r, purpose, body.Provider, body.Model)
		if err != nil {
			// The binding is stored either way; the dashboard shows why it did
			// not take effect yet instead of pretending it did.
			writeJSON(w, http.StatusAccepted, map[string]any{"status": "stored", "live": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "purpose": purpose, "live": live})
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// applyModel rebuilds the reasoner for the two purposes that can be swapped
// without a restart. Both go through the same client type; the triage model keeps
// its small token budget.
func (h *Handler) applyModel(r *http.Request, purpose, providerSlug, model string) (bool, error) {
	if purpose == llmreg.PurposeEmbedding || purpose == llmreg.PurposeReranker {
		return false, nil
	}
	p, err := h.Registry.Provider(r.Context(), providerSlug)
	if err != nil {
		return false, err
	}
	maxTokens := h.Models.MaxTokens
	if purpose == llmreg.PurposeTriage {
		maxTokens = 200
	}
	client, err := reasoner.NewOpenAI(p.BaseURL, p.APIKey, model, maxTokens, true)
	if err != nil {
		return false, err
	}
	if purpose == llmreg.PurposeTriage {
		h.Brain.SetTriage(client)
	} else {
		h.Brain.SetReasoner(client)
	}
	return true, nil
}
