// Modifications copyright (c) 2026 Bojan van der Heide
package webui

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/llmreg"
)

// manualPriceJSON is a rate entered in the dashboard, plus whether a catalogue
// has since published one for the same model. The catalogue wins, so an
// overruled row stays listed: removing it is optional, and it becomes the
// fallback again if the provider stops publishing a price.
type manualPriceJSON struct {
	llmreg.ManualPrice
	Overruled bool `json:"overruled"`
}

// prices lists the rates entered in the dashboard.
func (h *Handler) prices(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("price table is not configured"))
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	list, err := h.Registry.ManualPrices(r.Context())
	if !hdl(w, err) {
		return
	}
	out := make([]manualPriceJSON, 0, len(list))
	for _, p := range list {
		out = append(out, manualPriceJSON{ManualPrice: p, Overruled: h.cataloguePriced(p.Model)})
	}
	writeJSON(w, http.StatusOK, out)
}

// priceItem handles PUT and DELETE on /api/v1/prices/{model}. The model id is
// the whole rest of the path because ids carry slashes ("org/model").
func (h *Handler) priceItem(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("price table is not configured"))
		return
	}
	model := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/prices/"), "/")
	if model == "" {
		writeErr(w, http.StatusBadRequest, errors.New("a model name is required"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			InputPerMTok  *float64 `json:"input_per_mtok"`
			OutputPerMTok *float64 `json:"output_per_mtok"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var in, out float64
		if body.InputPerMTok != nil {
			in = *body.InputPerMTok
		}
		if body.OutputPerMTok != nil {
			out = *body.OutputPerMTok
		}
		saved, err := h.Registry.SetManualPrice(r.Context(), model, in, out)
		if !hdl(w, err) {
			return
		}
		h.republishPrices(r.Context())
		writeJSON(w, http.StatusOK, manualPriceJSON{ManualPrice: saved, Overruled: h.cataloguePriced(saved.Model)})
	case http.MethodDelete:
		if !hdl(w, h.Registry.DeleteManualPrice(r.Context(), model)) {
			return
		}
		h.republishPrices(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "model": model})
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// cataloguePriced asks the refresher whether a provider publishes this model.
// Without a refresher nothing is known here, and "not taken over" is the safe
// answer: it never hides a catalogue price behind a typed one.
func (h *Handler) cataloguePriced(model string) bool {
	return h.Prices != nil && h.Prices.CataloguePriced(model)
}

// republishPrices makes a saved rate visible in this process at once. The
// process that records the costs picks the same row up from the database within
// a minute, because that is the only thing the two processes share.
func (h *Handler) republishPrices(ctx context.Context) {
	if h.Prices != nil {
		h.Prices.Republish(ctx)
	}
}
