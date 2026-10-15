package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The bare root has no page; it should hand the visitor to the dashboard
// rather than returning a 404.
func TestRootRedirectsToUI(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("GET / = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/ui" {
		t.Fatalf(`Location = %q, want "/ui"`, loc)
	}
}

// /ui serves the dashboard shell itself, with no redirect.
func TestUIServesDashboard(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ui = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
}

// The root pattern is exact ("/{$}"), so unknown paths must not fall through
// to the redirect as a catch-all would.
func TestUnknownPathIsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// The graph tab loads Cytoscape.js from the embedded bundle (no internet needed)
// and only serves the scripts that are actually bundled.
func TestVendorScriptsAreServed(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	for _, name := range []string{"cytoscape.min.js", "layout-base.js", "cose-base.js", "cytoscape-fcose.js"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/vendor/"+name, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() < 10000 {
			t.Errorf("GET /ui/vendor/%s = %d (%d bytes)", name, rec.Code, rec.Body.Len())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
			t.Errorf("%s Content-Type = %q", name, ct)
		}
	}
	for _, name := range []string{"THIRD_PARTY.txt", "nope.js", "../index.html", "%2e%2e/index.html"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/vendor/"+name, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET /ui/vendor/%s should not be served, got 200", name)
		}
	}
}

func TestGraphRejectsBadInput(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/graph?min_confidence=banana", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad min_confidence = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/graph", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /graph = %d, want 405", rec.Code)
	}
}

func TestPathID(t *testing.T) {
	tests := []struct {
		path       string
		id         int64
		action     string
		wantErrMsg string
	}{
		{"/api/v1/facts/12", 12, "", ""},
		{"/api/v1/goals/7/complete", 7, "complete", ""},
		{"/api/v1/hypotheses/3/confirm/", 3, "confirm", ""},
		{"/api/v1/facts/", 0, "", "missing id"},
		{"/api/v1/facts/abc", 0, "", "invalid id"},
		{"/api/v1/facts/0", 0, "", "invalid id"},
		{"/api/v1/facts/-4", 0, "", "invalid id"},
	}
	for _, tt := range tests {
		id, action, err := pathID(httptest.NewRequest(http.MethodGet, tt.path, nil))
		if tt.wantErrMsg != "" {
			if err == nil || err.Error() != tt.wantErrMsg {
				t.Errorf("%s: err = %v, want %q", tt.path, err, tt.wantErrMsg)
			}
			continue
		}
		if err != nil || id != tt.id || action != tt.action {
			t.Errorf("%s: got (%d, %q, %v), want (%d, %q)", tt.path, id, action, err, tt.id, tt.action)
		}
	}
}
