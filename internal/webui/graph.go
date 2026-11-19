package webui

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/brain"
)

// graph returns a slice of the knowledge graph (entities and relations) for the
// Graph tab. Query: namespaces, canonical (comma separated), mapped=1 (only edges
// with a canonical type), min_confidence, q (focus on an entity), limit (max entities).
func (h *Handler) graph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	q := r.URL.Query()
	opts := brain.GraphOptions{
		Query:      q.Get("q"),
		OnlyMapped: q.Get("mapped") == "1" || q.Get("mapped") == "true",
		MaxNodes:   intQuery(r, "limit"),
	}
	for _, v := range q["canonical"] {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				opts.Canonical = append(opts.Canonical, p)
			}
		}
	}
	if s := q.Get("min_confidence"); s != "" {
		f, err := strconv.ParseFloat(s, 32)
		if err != nil || f < 0 || f > 1 {
			writeErr(w, http.StatusBadRequest, errors.New("min_confidence must be a number between 0 and 1"))
			return
		}
		opts.MinConfidence = float32(f)
	}
	g, err := h.Brain.QueryGraph(r.Context(), namespacesParam(r), opts)
	if !hdl(w, err) {
		return
	}
	g.Nodes = nonNil(g.Nodes)
	g.Edges = nonNil(g.Edges)
	writeJSON(w, http.StatusOK, g)
}

// timeline lays the live facts out over time, one row per entity, for the
// Timeline tab. Query: namespaces, rows (entities with a row of their own).
func (h *Handler) timeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	tl, err := h.Brain.FactTimeline(r.Context(), namespacesParam(r), brain.TimelineOptions{Rows: intQuery(r, "rows")})
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

// relationshipItem acts on one relation from the Clean-up tab.
// POST /api/v1/relationships/{id}/dismiss removes it (soft delete).
func (h *Handler) relationshipItem(w http.ResponseWriter, r *http.Request) {
	id, action, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if action != "dismiss" {
		writeErr(w, http.StatusNotFound, errors.New("unknown action"))
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !hdl(w, h.Brain.DismissRelationship(r.Context(), id)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "message": "Relation removed"})
}

// entityContext returns the facts, relations and source episodes behind one
// spelling of an entity, for the Clean-up tab's context column.
// GET /api/v1/entities/context?namespaces=...&name=...&limit=...
func (h *Handler) entityContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	c, err := h.Brain.EntityContext(r.Context(), namespacesParam(r), r.URL.Query().Get("name"), intQuery(r, "limit"))
	if !hdl(w, err) {
		return
	}
	facts := make([]factJSON, 0, len(c.Facts))
	for _, f := range c.Facts {
		facts = append(facts, factToJSON(f))
	}
	episodes := make([]episodeJSON, 0, len(c.Episodes))
	for _, e := range c.Episodes {
		episodes = append(episodes, episodeToJSON(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":            c.Name,
		"facts":           facts,
		"facts_total":     c.FactsTotal,
		"relations":       c.Relations,
		"relations_total": c.RelationsTotal,
		"episodes":        episodes,
		"episodes_total":  c.EpisodesTotal,
	})
}

// mergeEntities folds spellings of one entity into the one to keep, for the
// Clean-up tab. POST /api/v1/entities/merge?namespaces=... with
// {"keep": "...", "variants": ["..."], "dry_run": true|false}. A dry run reports
// what would change and changes nothing.
func (h *Handler) mergeEntities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var body struct {
		Keep     string   `json:"keep"`
		Variants []string `json:"variants"`
		DryRun   bool     `json:"dry_run"`
	}
	if err := readBody(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rep, err := h.Brain.MergeEntities(r.Context(), namespacesParam(r), body.Keep, body.Variants, body.DryRun)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// cleanup reports where the memory is untidy, for the Clean-up tab. Read only.
// Query: namespaces, min_confidence (relations below it are weak, default 0.8),
// limit (rows per list).
func (h *Handler) cleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	opts := brain.CleanupOptions{Limit: intQuery(r, "limit")}
	if s := r.URL.Query().Get("min_confidence"); s != "" {
		f, err := strconv.ParseFloat(s, 32)
		if err != nil || f <= 0 || f > 1 {
			writeErr(w, http.StatusBadRequest, errors.New("min_confidence must be a number above 0 and at most 1"))
			return
		}
		opts.MinConfidence = float32(f)
	}
	c, err := h.Brain.Cleanup(r.Context(), namespacesParam(r), opts)
	if !hdl(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// serveVendor serves the bundled third-party scripts (Cytoscape.js and its layout)
// from the embedded filesystem, so the dashboard works without internet access.
func (h *Handler) serveVendor(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.URL.Path)
	if name == "." || name == "/" || !strings.HasSuffix(name, ".js") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(staticFS, "static/vendor/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	// The file names carry no version, so keep the cache short.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}
