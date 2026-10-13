// Package webui serves a single-page dashboard and a read/write JSON API
// on top of the brain layer. It is mounted on the existing HTTP server.
package webui

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alash3al/stash/internal/auth"
	"github.com/alash3al/stash/internal/brain"
	"github.com/alash3al/stash/internal/models"
	"github.com/alash3al/stash/internal/scope"
)

//go:embed static/index.html
var staticFS embed.FS

// Handler serves the dashboard UI and the /api/v1 JSON endpoints.
type Handler struct {
	Brain *brain.Brain
	// Auth, when enabled, requires a Bearer token on every /api/v1 call and
	// carries the token's scope and namespace allowlist. The static dashboard
	// shell itself holds no data and stays public.
	Auth *auth.Set
}

// Register mounts the UI and API routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/ui", h.serveIndex)
	mux.HandleFunc("/ui/", h.serveIndex)
	api := http.NewServeMux()
	api.HandleFunc("/api/v1/recall", h.recall)
	api.HandleFunc("/api/v1/namespaces", h.namespaces)
	api.HandleFunc("/api/v1/namespaces/", h.namespaceItem)
	api.HandleFunc("/api/v1/episodes", h.episodes)
	api.HandleFunc("/api/v1/episodes/", h.episodeItem)
	api.HandleFunc("/api/v1/facts", h.facts)
	api.HandleFunc("/api/v1/facts/", h.factItem)
	api.HandleFunc("/api/v1/goals", h.goals)
	api.HandleFunc("/api/v1/goals/", h.goalItem)
	api.HandleFunc("/api/v1/hypotheses", h.hypotheses)
	api.HandleFunc("/api/v1/hypotheses/", h.hypothesisItem)
	api.HandleFunc("/api/v1/contradictions", h.contradictions)
	api.HandleFunc("/api/v1/contradictions/", h.contradictionItem)
	api.HandleFunc("/api/v1/failures", h.failures)
	api.HandleFunc("/api/v1/failures/", h.failureItem)
	api.HandleFunc("/api/v1/causal-links", h.causalLinks)
	api.HandleFunc("/api/v1/causal-links/", h.causalLinkItem)
	api.HandleFunc("/api/v1/runs", h.runs)
	api.HandleFunc("/api/v1/metrics", h.metrics)
	mux.Handle("/api/v1/", auth.Require(h.Auth, readOnlyWeb(api)))
}

// readOnlyWeb blocks writes for read-only tokens. GET and HEAD are reads, and
// the dashboard's POST /api/v1/recall is a read as well.
func readOnlyWeb(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scope.ReadOnly(r.Context()) && !isReadRequest(r) {
			writeErr(w, http.StatusForbidden, brain.ErrReadOnly)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isReadRequest(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodPost:
		return strings.TrimSuffix(r.URL.Path, "/") == "/api/v1/recall"
	default:
		return false
	}
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "ui asset missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// nonNil makes sure empty lists serialise as [] instead of null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func errStatus(err error) int {
	switch {
	case errors.Is(err, brain.ErrEmptyContent),
		errors.Is(err, brain.ErrNamespaceNotFound),
		errors.Is(err, brain.ErrGoalNotFound),
		errors.Is(err, brain.ErrFailureNotFound),
		errors.Is(err, brain.ErrContradictionNotFound),
		errors.Is(err, brain.ErrCausalLinkNotFound):
		return http.StatusBadRequest
	case errors.Is(err, brain.ErrForbidden), errors.Is(err, brain.ErrReadOnly):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func readBody(r *http.Request, v any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("empty request body")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func namespacesParam(r *http.Request) []string {
	values := r.URL.Query()["namespaces"]
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	if len(out) == 0 {
		out = []string{"/"}
	}
	return out
}

func pagination(r *http.Request) brain.Pagination {
	return brain.Pagination{
		Offset: intQuery(r, "offset"),
		Limit:  intQuery(r, "limit"),
	}.Sanitize()
}

func intQuery(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func pathID(r *http.Request) (int64, string, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return 0, "", errors.New("missing id")
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", errors.New("invalid id")
	}
	action := ""
	if len(parts) > 3 {
		action = parts[3]
	}
	return id, action, nil
}

func ptrInt64(r *http.Request, name string) *int64 {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func hdl(w http.ResponseWriter, err error) bool {
	if err != nil {
		writeErr(w, errStatus(err), err)
		return false
	}
	return true
}

type factJSON struct {
	ID          int64      `json:"id"`
	NamespaceID int64      `json:"namespace_id"`
	Content     string     `json:"content"`
	Confidence  float32    `json:"confidence"`
	Entity      *string    `json:"entity"`
	Property    *string    `json:"property"`
	Value       *string    `json:"value"`
	ValidFrom   *time.Time `json:"valid_from"`
	ValidUntil  *time.Time `json:"valid_until"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func factToJSON(f models.Fact) factJSON {
	return factJSON{
		ID: f.ID, NamespaceID: f.NamespaceID, Content: f.Content,
		Confidence: f.Confidence, Entity: f.Entity, Property: f.Property,
		Value: f.Value, ValidFrom: f.ValidFrom, ValidUntil: f.ValidUntil,
		CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

type episodeJSON struct {
	ID          int64     `json:"id"`
	NamespaceID int64     `json:"namespace_id"`
	Content     string    `json:"content"`
	OccurredAt  time.Time `json:"occurred_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *Handler) episodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	items, err := h.Brain.ListEpisodes(r.Context(), namespacesParam(r), pagination(r))
	if !hdl(w, err) {
		return
	}
	out := make([]episodeJSON, 0, len(items))
	for _, e := range items {
		out = append(out, episodeJSON{ID: e.ID, NamespaceID: e.NamespaceID, Content: e.Content, OccurredAt: e.OccurredAt, CreatedAt: e.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) episodeItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if action != "" || r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !hdl(w, h.Brain.PurgeEpisode(r.Context(), id)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
}

func (h *Handler) facts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	items, err := h.Brain.QueryFacts(r.Context(), namespacesParam(r), nil, nil, pagination(r))
	if !hdl(w, err) {
		return
	}
	out := make([]factJSON, 0, len(items))
	for _, f := range items {
		out = append(out, factToJSON(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (h *Handler) factItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			f, err := h.Brain.GetFact(r.Context(), id)
			if !hdl(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, factToJSON(*f))
		case http.MethodDelete:
			if !hdl(w, h.Brain.PurgeFact(r.Context(), id)) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	case "confidence":
		var body struct {
			Confidence float32 `json:"confidence"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !hdl(w, h.Brain.UpdateFactConfidence(r.Context(), id, body.Confidence)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) recall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var body struct {
		Query      string   `json:"query"`
		Namespaces []string `json:"namespaces"`
		Limit      int      `json:"limit"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(body.Namespaces) == 0 {
		body.Namespaces = []string{"/"}
	}
	results, err := h.Brain.Recall(r.Context(), body.Namespaces, body.Query, body.Limit)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (h *Handler) namespaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.Brain.ListNamespaces(r.Context(), nil, pagination(r))
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, nonNil(items))
	case http.MethodPost:
		var body struct {
			Slug        string `json:"slug"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if body.Name == "" {
			body.Name = body.Slug
		}
		id, err := h.Brain.CreateNamespace(r.Context(), body.Slug, body.Name, body.Description)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) namespaceItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	slug := parts[2]
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	ns, err := h.Brain.GetNamespace(r.Context(), slug)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, ns)
}

func (h *Handler) goals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.Brain.ListGoals(r.Context(), namespacesParam(r), r.URL.Query().Get("status"), ptrInt64(r, "parent_id"), pagination(r))
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, nonNil(items))
	case http.MethodPost:
		var body struct {
			Namespaces []string `json:"namespaces"`
			Content    string   `json:"content"`
			ParentID   *int64   `json:"parent_id"`
			Priority   int      `json:"priority"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if len(body.Namespaces) == 0 {
			body.Namespaces = []string{"/"}
		}
		nsIDs, err := h.Brain.ResolveNamespaceIDs(r.Context(), body.Namespaces)
		if !hdl(w, err) {
			return
		}
		g, err := h.Brain.CreateGoal(r.Context(), nsIDs[0], body.Content, body.ParentID, body.Priority)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, g)
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) goalItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Notes    string `json:"notes"`
		Content  string `json:"content"`
		Priority int    `json:"priority"`
	}
	if r.Method == http.MethodPost {
		_ = readBody(r, &body)
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			g, err := h.Brain.GetGoal(r.Context(), id)
			if !hdl(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, g)
		case http.MethodDelete:
			if !hdl(w, h.Brain.DeleteGoal(r.Context(), id)) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	case "complete":
		g, err := h.Brain.CompleteGoal(r.Context(), id, body.Notes)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, g)
	case "abandon":
		g, err := h.Brain.AbandonGoal(r.Context(), id, body.Notes)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, g)
	case "update":
		g, err := h.Brain.UpdateGoal(r.Context(), id, body.Content, body.Priority, body.Notes)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, g)
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) hypotheses(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.Brain.ListHypotheses(r.Context(), namespacesParam(r), r.URL.Query().Get("status"), pagination(r))
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, nonNil(items))
	case http.MethodPost:
		var body struct {
			Namespaces       []string `json:"namespaces"`
			Content          string   `json:"content"`
			VerificationPlan string   `json:"verification_plan"`
			Confidence       float32  `json:"confidence"`
			SourceFactIDs    []int64  `json:"source_fact_ids"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if len(body.Namespaces) == 0 {
			body.Namespaces = []string{"/"}
		}
		nsIDs, err := h.Brain.ResolveNamespaceIDs(r.Context(), body.Namespaces)
		if !hdl(w, err) {
			return
		}
		hp, err := h.Brain.CreateHypothesis(r.Context(), nsIDs[0], body.Content, body.VerificationPlan, body.Confidence, body.SourceFactIDs)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, hp)
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) hypothesisItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Reason           string  `json:"reason"`
		Content          string  `json:"content"`
		VerificationPlan string  `json:"verification_plan"`
		Confidence       float32 `json:"confidence"`
		Status           string  `json:"status"`
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		_ = readBody(r, &body)
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			hp, err := h.Brain.GetHypothesis(r.Context(), id)
			if !hdl(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, hp)
		case http.MethodDelete:
			if !hdl(w, h.Brain.DeleteHypothesis(r.Context(), id)) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	case "confirm":
		hp, fact, err := h.Brain.ConfirmHypothesis(r.Context(), id)
		if !hdl(w, err) {
			return
		}
		resp := map[string]any{"hypothesis": hp}
		if fact != nil {
			resp["fact"] = factToJSON(*fact)
		}
		writeJSON(w, http.StatusOK, resp)
	case "reject":
		hp, err := h.Brain.RejectHypothesis(r.Context(), id, body.Reason)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, hp)
	case "refine":
		hp, err := h.Brain.RefineHypothesis(r.Context(), id, body.Content, body.VerificationPlan, body.Confidence)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, hp)
	case "status":
		hp, err := h.Brain.UpdateHypothesisStatus(r.Context(), id, body.Status)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, hp)
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) contradictions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	items, err := h.Brain.ListContradictions(r.Context(), namespacesParam(r), pagination(r))
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, nonNil(items))
}

func (h *Handler) contradictionItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch action {
	case "":
		c, err := h.Brain.GetContradiction(r.Context(), id)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, c)
	case "resolve":
		var body struct {
			Resolution string `json:"resolution"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !hdl(w, h.Brain.ResolveContradiction(r.Context(), id, body.Resolution)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) failures(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.Brain.ListFailures(r.Context(), namespacesParam(r), ptrInt64(r, "goal_id"), pagination(r))
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, nonNil(items))
	case http.MethodPost:
		var body struct {
			Namespaces []string `json:"namespaces"`
			GoalID     *int64   `json:"goal_id"`
			Content    string   `json:"content"`
			Reason     string   `json:"reason"`
			Lesson     string   `json:"lesson"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if len(body.Namespaces) == 0 {
			body.Namespaces = []string{"/"}
		}
		nsIDs, err := h.Brain.ResolveNamespaceIDs(r.Context(), body.Namespaces)
		if !hdl(w, err) {
			return
		}
		f, err := h.Brain.CreateFailure(r.Context(), nsIDs[0], body.Content, body.Reason, body.Lesson, body.GoalID)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, f)
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) failureItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			f, err := h.Brain.GetFailure(r.Context(), id)
			if !hdl(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, f)
		case http.MethodDelete:
			if !hdl(w, h.Brain.DeleteFailure(r.Context(), id)) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) causalLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	items, err := h.Brain.ListCausalLinks(r.Context(), namespacesParam(r), pagination(r))
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, nonNil(items))
}

func (h *Handler) causalLinkItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch action {
	case "":
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		if !hdl(w, h.Brain.DeleteCausalLink(r.Context(), id)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	case "trace":
		direction := r.URL.Query().Get("direction")
		if direction == "" {
			direction = "both"
		}
		items, err := h.Brain.TraceCausalChain(r.Context(), id, direction, intQuery(r, "depth"))
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, nonNil(items))
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) runs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	items, err := h.Brain.ListConsolidationRuns(r.Context(), namespacesParam(r), pagination(r))
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, nonNil(items))
}
