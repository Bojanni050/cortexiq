// Package webui serves a single-page dashboard and a read/write JSON API
// on top of the brain layer. It is mounted on the existing HTTP server.
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/schedule"
	"github.com/Bojanni050/cortexiq/internal/scope"
)

//go:embed static/index.html static/vendor/*
var staticFS embed.FS

// Handler serves the dashboard UI and the /api/v1 JSON endpoints.
type Handler struct {
	Brain *brain.Brain
	// Auth, when enabled, requires a Bearer token on every /api/v1 call and
	// carries the token's read/write scope. The static dashboard shell itself
	// holds no data and stays public.
	Auth *auth.Set
}

// Register mounts the UI and API routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// The bare root has no page of its own; send it to the dashboard.
	mux.HandleFunc("/{$}", h.redirectToUI)
	mux.HandleFunc("/ui", h.serveIndex)
	mux.HandleFunc("/ui/", h.serveIndex)
	mux.HandleFunc("/ui/vendor/", h.serveVendor)
	api := http.NewServeMux()
	api.HandleFunc("/api/v1/recall", h.recall)
	api.HandleFunc("/api/v1/reflect", h.reflect)
	api.HandleFunc("/api/v1/remember", h.remember)
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
	api.HandleFunc("/api/v1/mental-models", h.mentalModels)
	api.HandleFunc("/api/v1/mental-models/", h.mentalModelItem)
	api.HandleFunc("/api/v1/runs", h.runs)
	api.HandleFunc("/api/v1/consolidation", h.consolidation)
	api.HandleFunc("/api/v1/graph", h.graph)
	api.HandleFunc("/api/v1/stats", h.stats)
	api.HandleFunc("/api/v1/settings/schedule", h.schedule)
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
		// Recall and reflect are reads: they retrieve memories and synthesize
		// an answer, but store nothing. Mental model search is also a read.
		path := strings.TrimSuffix(r.URL.Path, "/")
		return path == "/api/v1/recall" || path == "/api/v1/reflect" || path == "/api/v1/mental-models/search"
	default:
		return false
	}
}

// redirectToUI sends the bare root path to the dashboard, so a visitor who
// opens the host without a path lands on the UI instead of a 404.
func (h *Handler) redirectToUI(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui", http.StatusFound)
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
	// A resource addressed by the path does not exist.
	case errors.Is(err, brain.ErrFactNotFound),
		errors.Is(err, brain.ErrEpisodeNotFound),
		errors.Is(err, brain.ErrGoalNotFound),
		errors.Is(err, brain.ErrFailureNotFound),
		errors.Is(err, brain.ErrContradictionNotFound),
		errors.Is(err, brain.ErrCausalLinkNotFound),
		errors.Is(err, brain.ErrHypothesisNotFound),
		errors.Is(err, brain.ErrMentalModelNotFound):
		return http.StatusNotFound
	// The request itself is wrong, so a client can tell its own mistake from a
	// server failure.
	case errors.Is(err, brain.ErrEmptyContent),
		errors.Is(err, brain.ErrContentTooLong),
		errors.Is(err, brain.ErrInvalidPath),
		errors.Is(err, brain.ErrNamespacesRequired),
		errors.Is(err, brain.ErrNamespaceNotFound),
		errors.Is(err, brain.ErrMentalModelNotGrounded),
		errors.Is(err, brain.ErrMentalModelRefreshInFlight):
		return http.StatusBadRequest
	case errors.Is(err, brain.ErrResolutionRequired):
		return http.StatusBadRequest
	case errors.Is(err, brain.ErrReadOnly):
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

// pathID reads the id (and optional action) from /api/v1/<resource>/<id>[/<action>].
// It used to index the whole path, so it read the resource name as the id and every
// GET/DELETE/POST on an item answered "invalid id".
func pathID(r *http.Request) (int64, string, error) {
	rest := strings.TrimPrefix(strings.Trim(r.URL.Path, "/"), "api/v1/")
	parts := strings.Split(rest, "/") // <resource>, <id>, <action>
	if len(parts) < 2 {
		return 0, "", errors.New("missing id")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", errors.New("invalid id")
	}
	action := ""
	if len(parts) > 2 {
		action = parts[2]
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

// total tells the client how many rows the unpaged list holds, so a dashboard
// can show "showing 101-200 of 1,240" instead of silently stopping at one page.
func total(w http.ResponseWriter, n int) {
	w.Header().Set("X-Total-Count", strconv.Itoa(n))
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
	n, err := h.Brain.CountEpisodes(r.Context(), namespacesParam(r))
	if !hdl(w, err) {
		return
	}
	total(w, n)
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
	query := r.URL.Query()
	from, err := parseTimeBound(query.Get("from"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid from: %w", err))
		return
	}
	to, err := parseTimeBound(query.Get("to"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid to: %w", err))
		return
	}
	asOf, err := parseTimeBound(query.Get("as_of"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid as_of: %w", err))
		return
	}
	knownFrom, err := parseTimeBound(query.Get("known_from"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid known_from: %w", err))
		return
	}
	knownTo, err := parseTimeBound(query.Get("known_to"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid known_to: %w", err))
		return
	}
	items, err := h.Brain.QueryFactsWindow(r.Context(), namespacesParam(r), nil, nil, asOf, from, to, knownFrom, knownTo, pagination(r))
	if !hdl(w, err) {
		return
	}
	count, err := h.Brain.CountFactsWindow(r.Context(), namespacesParam(r), nil, nil, asOf, from, to, knownFrom, knownTo)
	if !hdl(w, err) {
		return
	}
	total(w, count)
	out := make([]factJSON, 0, len(items))
	for _, f := range items {
		out = append(out, factToJSON(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": count})
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
		// Trace asks for an explanation of the ranking next to the results: per
		// result where each search arm placed it, plus the steps of the recall.
		// A traced recall does not count as use of the facts it returns.
		Trace bool `json:"trace"`
		// Types limits the search to "fact", "episode", "failure" and/or "goal";
		// empty means facts and episodes.
		Types []string `json:"types"`
		// AsOf is an RFC 3339 moment: recall what was known then.
		AsOf string `json:"as_of"`
		From string `json:"from"`
		To   string `json:"to"`
		// KnownFrom and KnownTo are a half-open knowledge-time window; as_of is its
		// point form and cannot be combined with them.
		KnownFrom string `json:"known_from"`
		KnownTo   string `json:"known_to"`
		// Graph overrides the configured graph arm for this call: true forces it
		// on, false forces it off, absent keeps the configuration.
		Graph *bool `json:"graph"`
		// MMR overrides the configured MMR lambda for this call: 0 disables MMR,
		// 1 maximises diversity, 0.5 balances. Absent keeps the configuration.
		MMR *float64 `json:"mmr"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(body.Namespaces) == 0 {
		body.Namespaces = []string{"/"}
	}
	opts := brain.RecallOptions{Types: body.Types}
	for _, t := range body.Types {
		if t != "fact" && t != "episode" && t != "failure" && t != "goal" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid type %q: use fact, episode, failure or goal", t))
			return
		}
	}
	if body.AsOf != "" {
		t, err := time.Parse(time.RFC3339, body.AsOf)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid as_of: %w", err))
			return
		}
		opts.AsOf = &t
	}
	from, err := parseTimeBound(body.From)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid from: %w", err))
		return
	}
	opts.From = from
	to, err := parseTimeBound(body.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid to: %w", err))
		return
	}
	opts.To = to
	knownFrom, err := parseTimeBound(body.KnownFrom)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid known_from: %w", err))
		return
	}
	opts.KnownFrom = knownFrom
	knownTo, err := parseTimeBound(body.KnownTo)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid known_to: %w", err))
		return
	}
	opts.KnownTo = knownTo
	opts.Graph = body.Graph
	opts.MMR = body.MMR
	if body.Trace {
		results, trace, err := h.Brain.RecallDetailed(r.Context(), body.Namespaces, body.Query, body.Limit, opts)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results, "trace": trace})
		return
	}
	results, err := h.Brain.RecallWith(r.Context(), body.Namespaces, body.Query, body.Limit, opts)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// reflect answers a question by recalling relevant memories and synthesizing
// an answer with the LLM. It is a read operation: it recalls memories and calls
// the reasoner, but stores nothing.
func (h *Handler) reflect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var body struct {
		Query      string   `json:"query"`
		Namespaces []string `json:"namespaces"`
		Limit      int      `json:"limit"`
		Context    string   `json:"context"`
		Types      []string `json:"types"`
		AsOf       string   `json:"as_of"`
		From       string   `json:"from"`
		To         string   `json:"to"`
		KnownFrom  string   `json:"known_from"`
		KnownTo    string   `json:"known_to"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(body.Query) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("query is required"))
		return
	}
	if len(body.Namespaces) == 0 {
		body.Namespaces = []string{"/"}
	}
	for _, t := range body.Types {
		if t != "fact" && t != "episode" && t != "failure" && t != "goal" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid type %q: use fact, episode, failure or goal", t))
			return
		}
	}
	opts := brain.ReflectOptions{
		Namespaces: body.Namespaces,
		Limit:      body.Limit,
		Context:    body.Context,
		Types:      body.Types,
	}
	if body.AsOf != "" {
		t, err := time.Parse(time.RFC3339, body.AsOf)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid as_of: %w", err))
			return
		}
		opts.AsOf = &t
	}
	from, err := parseTimeBound(body.From)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid from: %w", err))
		return
	}
	opts.From = from
	to, err := parseTimeBound(body.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid to: %w", err))
		return
	}
	opts.To = to
	knownFrom, err := parseTimeBound(body.KnownFrom)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid known_from: %w", err))
		return
	}
	opts.KnownFrom = knownFrom
	knownTo, err := parseTimeBound(body.KnownTo)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid known_to: %w", err))
		return
	}
	opts.KnownTo = knownTo

	result, err := h.Brain.Reflect(r.Context(), body.Query, opts)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseTimeBound(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("use RFC3339 or YYYY-MM-DD")
}

// remember stores a new episode (a memory). It mirrors the MCP "remember" tool:
// content is required; namespace defaults to the token's first allowed namespace
// (or "/"), importance to 0.5, and occurred_at to now. The response shape is the
// same as the MCP tool's, so headless clients can share one parser.
func (h *Handler) remember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var body struct {
		Content    string   `json:"content"`
		Namespace  string   `json:"namespace"`
		Namespaces []string `json:"namespaces"`
		Importance *float32 `json:"importance"`
		OccurredAt string   `json:"occurred_at"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var occurredAt *time.Time
	if body.OccurredAt != "" {
		t, err := time.Parse(time.RFC3339, body.OccurredAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid occurred_at: %w", err))
			return
		}
		occurredAt = &t
	}
	importance := brain.DefaultImportance
	if body.Importance != nil {
		importance = *body.Importance
		if importance < 0 || importance > 1 || importance != importance {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("importance must be between 0 and 1, got %v", importance))
			return
		}
	}
	id, err := h.Brain.RememberWithImportance(r.Context(), rememberNamespace(body.Namespace, body.Namespaces), body.Content, occurredAt, importance)
	if errors.Is(err, brain.ErrSocialCloser) {
		// Nothing is wrong with the request; the content just is not a memory.
		// 200 (not 201) keeps clients from reporting a row that was never created,
		// and "skipped" tells an automatic capture loop to move on quietly.
		writeJSON(w, http.StatusOK, map[string]any{
			"skipped": true,
			"reason":  brain.ErrSocialCloser.Error(),
		})
		return
	}
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Memory remembered successfully"})
}

// rememberNamespace picks the target namespace for a write: the first explicit
// slug in the request, or the first entry of the array, or the root namespace.
func rememberNamespace(single string, many []string) string {
	if s := strings.TrimSpace(single); s != "" {
		return s
	}
	for _, s := range many {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return "/"
}

func (h *Handler) namespaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.Brain.ListNamespaces(r.Context(), nil, pagination(r))
		if !hdl(w, err) {
			return
		}
		n, err := h.Brain.CountNamespaces(r.Context(), nil)
		if !hdl(w, err) {
			return
		}
		total(w, n)
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
	// /api/v1/namespaces/{slug}/config is the namespace's configuration.
	if len(parts) >= 4 && parts[3] == "config" {
		h.namespaceConfig(w, r)
		return
	}
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

// namespaceConfig serves /api/v1/namespaces/{slug}/config: GET returns the
// effective configuration (defaults merged with ancestor and own overrides) and
// the namespace's own overrides; PUT replaces the own overrides; DELETE removes
// them so the namespace inherits again. DELETE with ?key= removes one key.
func (h *Handler) namespaceConfig(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/")
	slug = strings.TrimSuffix(strings.TrimSuffix(slug, "/config"), "/")
	switch r.Method {
	case http.MethodGet:
		cfg, err := h.Brain.GetNamespaceConfig(r.Context(), slug)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		var body struct {
			Overrides map[string]any `json:"overrides"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := h.Brain.SetNamespaceConfig(r.Context(), slug, body.Overrides); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := h.Brain.GetNamespaceConfig(r.Context(), slug)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodDelete:
		if key := r.URL.Query().Get("key"); key != "" {
			if err := h.Brain.DeleteNamespaceConfigKey(r.Context(), slug, key); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		} else {
			if err := h.Brain.DeleteNamespaceConfig(r.Context(), slug); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) goals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.Brain.ListGoals(r.Context(), namespacesParam(r), r.URL.Query().Get("status"), ptrInt64(r, "parent_id"), pagination(r))
		if !hdl(w, err) {
			return
		}
		n, err := h.Brain.CountGoals(r.Context(), namespacesParam(r), r.URL.Query().Get("status"), ptrInt64(r, "parent_id"))
		if !hdl(w, err) {
			return
		}
		total(w, n)
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
		n, err := h.Brain.CountHypotheses(r.Context(), namespacesParam(r), r.URL.Query().Get("status"))
		if !hdl(w, err) {
			return
		}
		total(w, n)
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
	items, err := h.Brain.ListContradictions(r.Context(), namespacesParam(r), r.URL.Query().Get("status"), pagination(r))
	if !hdl(w, err) {
		return
	}
	n, err := h.Brain.CountContradictions(r.Context(), namespacesParam(r), r.URL.Query().Get("status"))
	if !hdl(w, err) {
		return
	}
	total(w, n)
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
	case "unresolve":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		if !hdl(w, h.Brain.UnresolveContradiction(r.Context(), id)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "open"})
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
		n, err := h.Brain.CountFailures(r.Context(), namespacesParam(r), ptrInt64(r, "goal_id"))
		if !hdl(w, err) {
			return
		}
		total(w, n)
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
	n, err := h.Brain.CountCausalLinks(r.Context(), namespacesParam(r))
	if !hdl(w, err) {
		return
	}
	total(w, n)
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
	n, err := h.Brain.CountConsolidationRuns(r.Context(), namespacesParam(r))
	if !hdl(w, err) {
		return
	}
	total(w, n)
	writeJSON(w, http.StatusOK, nonNil(items))
}

// scheduleView is the stored schedule plus the choices the dashboard may offer,
// so the UI never has to hardcode them.
type scheduleView struct {
	schedule.Config
	Modes     []schedule.Mode `json:"modes"`
	Intervals []int           `json:"intervals"`
}

// schedule reads or replaces the global consolidation schedule. The background
// consolidator picks the new value up immediately; no restart is needed.
func (h *Handler) schedule(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := h.Brain.ScheduleConfig(r.Context())
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, scheduleView{
			Config:    cfg,
			Modes:     schedule.Modes(),
			Intervals: schedule.IntervalChoices(),
		})
	case http.MethodPut:
		var req schedule.Config
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid schedule: %w", err))
			return
		}
		if err := h.Brain.SetScheduleConfig(r.Context(), req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// consolidation reports the extraction backlog: how many episodes are waiting
// for consolidation and when it last ran. A memory that was just written is only
// an episode, so without this the dashboard looks broken for a few minutes.
func (h *Handler) consolidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	st, err := h.Brain.ConsolidationStatus(r.Context(), namespacesParam(r))
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	st, err := h.Brain.Stats(r.Context())
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// generateMentalModelInBackground runs a refresh after the response is sent.
// The request context dies with the response, so the refresh gets its own
// lifetime; it keeps the request's values (trace/scope), not its cancellation.
func (h *Handler) generateMentalModelInBackground(ctx context.Context, id int64) {
	base := context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(base, 5*time.Minute)
		defer cancel()
		// A generation may already be running for this model (the sweep, or the
		// refresh that preceded an edit). Wait for it and try again: dropping the
		// request would leave the content answering the old question.
		for attempt := 0; attempt < 10; attempt++ {
			_, err := h.Brain.RefreshMentalModel(ctx, id)
			if err == nil {
				return
			}
			if errors.Is(err, brain.ErrMentalModelRefreshInFlight) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}
			log.Printf("mental model %d: background generation failed: %v", id, err)
			return
		}
		log.Printf("mental model %d: background generation still in flight after 30s", id)
	}()
}

func (h *Handler) mentalModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var tags []string
		for _, v := range r.URL.Query()["tags"] {
			for _, t := range strings.Split(v, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tags = append(tags, t)
				}
			}
		}
		items, err := h.Brain.ListMentalModels(r.Context(), namespacesParam(r), tags, pagination(r))
		if !hdl(w, err) {
			return
		}
		n, err := h.Brain.CountMentalModels(r.Context(), namespacesParam(r), tags)
		if !hdl(w, err) {
			return
		}
		total(w, n)
		writeJSON(w, http.StatusOK, nonNil(items))
	case http.MethodPost:
		var body struct {
			Name        string   `json:"name"`
			SourceQuery string   `json:"source_query"`
			Namespaces  []string `json:"namespaces"`
			Tags        []string `json:"tags"`
			MaxTokens   int      `json:"max_tokens"`
			// Async returns as soon as the row exists and generates its content
			// in the background. The dashboard uses it so the dialog can close
			// immediately instead of holding the user for a 30-second reflect.
			Async bool `json:"async"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.SourceQuery) == "" {
			writeErr(w, http.StatusBadRequest, errors.New("name and source_query are required"))
			return
		}
		// No namespace (or "/") means a global model: it reads every namespace and
		// is available to reflect everywhere. A named namespace scopes it there.
		var nsID *int64
		if len(body.Namespaces) > 0 {
			nsPath := strings.TrimSpace(body.Namespaces[0])
			if nsPath != "" && nsPath != "/" {
				id, err := h.Brain.NamespaceIDByPath(r.Context(), nsPath)
				if !hdl(w, err) {
					return
				}
				nsID = &id
			}
		}
		opts := brain.CreateMentalModelOptions{
			Name:        body.Name,
			SourceQuery: body.SourceQuery,
			Tags:        body.Tags,
			MaxTokens:   body.MaxTokens,
			NamespaceID: nsID,
		}
		if body.Async {
			mm, err := h.Brain.CreateMentalModelQueued(r.Context(), opts)
			if !hdl(w, err) {
				return
			}
			// The request context dies with the response, so the refresh needs
			// its own lifetime. It keeps the request's values (trace/scope) but
			// not its cancellation.
			h.generateMentalModelInBackground(r.Context(), mm.ID)
			writeJSON(w, http.StatusAccepted, mm)
			return
		}
		mm, err := h.Brain.CreateMentalModel(r.Context(), opts)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, mm)
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) mentalModelItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			mm, err := h.Brain.GetMentalModel(r.Context(), id)
			if !hdl(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, mm)
		case http.MethodDelete:
			if !hdl(w, h.Brain.DeleteMentalModel(r.Context(), id)) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		case http.MethodPatch:
			var body struct {
				Name        string   `json:"name"`
				SourceQuery string   `json:"source_query"`
				Tags        []string `json:"tags"`
				MaxTokens   int      `json:"max_tokens"`
			}
			if err := readBody(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			mm, err := h.Brain.UpdateMentalModel(r.Context(), id, brain.UpdateMentalModelOptions{
				Name:        body.Name,
				SourceQuery: body.SourceQuery,
				Tags:        body.Tags,
				MaxTokens:   body.MaxTokens,
			})
			if !hdl(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, mm)
		default:
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	case "refresh":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var body struct {
			Async bool `json:"async"`
		}
		if r.ContentLength > 0 {
			if err := readBody(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if _, err := h.Brain.GetMentalModel(r.Context(), id); !hdl(w, err) {
			return
		}
		if body.Async {
			h.generateMentalModelInBackground(r.Context(), id)
			writeJSON(w, http.StatusAccepted, map[string]any{"mental_model_id": id, "status": "queued"})
			return
		}
		mm, err := h.Brain.RefreshMentalModel(r.Context(), id)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, mm)
	case "search":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var body struct {
			Query      string   `json:"query"`
			Namespaces []string `json:"namespaces"`
			Tags       []string `json:"tags"`
			Limit      int      `json:"limit"`
		}
		if err := readBody(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(body.Query) == "" {
			writeErr(w, http.StatusBadRequest, errors.New("query is required"))
			return
		}
		if len(body.Namespaces) == 0 {
			body.Namespaces = []string{"/"}
		}
		results, err := h.Brain.SearchMentalModels(r.Context(), body.Namespaces, body.Query, body.Limit, body.Tags)
		if !hdl(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": nonNil(results), "count": len(results)})
	default:
		writeErr(w, http.StatusNotFound, errors.New("not found"))
	}
}
