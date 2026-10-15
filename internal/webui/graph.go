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
