package webui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/Bojanni050/cortexiq/internal/scope"
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

// remember is a write, so only POST is allowed and a read-only token is refused
// before the handler ever runs.
func TestRememberMethodAndScope(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/remember", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/v1/remember = %d, want 405", method, rec.Code)
		}
	}

	// No brain here, so a read-only request must be rejected by the middleware
	// with 403 rather than reaching the handler.
	ro := scope.With(context.Background(), scope.Principal{Name: "ro", ReadOnly: true})
	rec := httptest.NewRecorder()
	readOnlyWeb(http.HandlerFunc((&Handler{}).remember)).ServeHTTP(
		rec, httptest.NewRequest(http.MethodPost, "/api/v1/remember", strings.NewReader(`{"content":"x"}`)).WithContext(ro))
	if rec.Code != http.StatusForbidden {
		t.Errorf("read-only POST /api/v1/remember = %d, want 403", rec.Code)
	}
}

// An explicit slug wins; otherwise the first entry of the array; otherwise the
// root namespace.
func TestRememberNamespace(t *testing.T) {
	cases := []struct {
		single   string
		many     []string
		wantSlug string
	}{
		{"/projects/gaia", nil, "/projects/gaia"},
		{"", []string{"", "/projects/gaia"}, "/projects/gaia"},
		{"", nil, "/"},
	}
	for _, c := range cases {
		if got := rememberNamespace(c.single, c.many); got != c.wantSlug {
			t.Errorf("rememberNamespace(%q, %v) = %q, want %q", c.single, c.many, got, c.wantSlug)
		}
	}
}

// A client has to be able to tell its own mistake from a server failure: a
// missing resource is 404, a bad request is 400, and only a real fault is 500.
func TestErrStatus(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{brain.ErrFactNotFound, http.StatusNotFound},
		{brain.ErrEpisodeNotFound, http.StatusNotFound},
		{brain.ErrGoalNotFound, http.StatusNotFound},
		{brain.ErrHypothesisNotFound, http.StatusNotFound},
		{brain.ErrEmptyContent, http.StatusBadRequest},
		{brain.ErrInvalidPath, http.StatusBadRequest},
		{brain.ErrNamespacesRequired, http.StatusBadRequest},
		{brain.ErrNamespaceNotFound, http.StatusBadRequest},
		{brain.ErrReasonRequired, http.StatusBadRequest},
		{brain.ErrAlreadyRetracted, http.StatusConflict},
		{brain.ErrReadOnly, http.StatusForbidden},
		{errors.New("connection refused"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := errStatus(c.err); got != c.want {
			t.Errorf("errStatus(%v) = %d, want %d", c.err, got, c.want)
		}
	}

	// Errors from deeper layers arrive wrapped, so the mapping must unwrap.
	if got := errStatus(fmt.Errorf("namespace %q: %w", "/x", brain.ErrInvalidPath)); got != http.StatusBadRequest {
		t.Errorf("wrapped validation error = %d, want 400", got)
	}
}

// A list without a total lets a dashboard stop at one page without saying so.
func TestTotalHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	total(rec, 1240)
	if got := rec.Header().Get("X-Total-Count"); got != "1240" {
		t.Errorf("X-Total-Count = %q, want %q", got, "1240")
	}

	// The header is set before the body, so it survives the write.
	writeJSON(rec, http.StatusOK, map[string]any{"items": []any{}})
	if got := rec.Header().Get("X-Total-Count"); got != "1240" {
		t.Errorf("X-Total-Count after writeJSON = %q, want %q", got, "1240")
	}
}

func TestRecallRejectsBadTypeAndAsOf(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	for name, body := range map[string]string{
		"unknown type": `{"query":"x","types":["hypothesis"]}`,
		"bad as_of":    `{"query":"x","as_of":"yesterday"}`,
		"bad graph":    `{"query":"x","graph":"sure"}`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/recall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400 (%s)", name, rec.Code, rec.Body.String())
		}
	}
}

func TestParseTimeBound(t *testing.T) {
	got, err := parseTimeBound("2026-10-20")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("parseTimeBound(date) = %v, want %v", got, want)
	}
	got, err = parseTimeBound("2026-10-20T12:30:00+02:00")
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("parseTimeBound(RFC3339) = %v, want %v", got, want)
	}
	if _, err := parseTimeBound("yesterday"); err == nil {
		t.Fatal("parseTimeBound accepted a natural-language phrase")
	}
}

func TestErrStatusMentalModels(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{brain.ErrMentalModelNotFound, http.StatusNotFound},
		{brain.ErrMentalModelNotGrounded, http.StatusBadRequest},
		{brain.ErrMentalModelRefreshInFlight, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := errStatus(tc.err); got != tc.want {
			t.Errorf("errStatus(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestParseIDList(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []int64
		wantErr bool
	}{
		{name: "single", in: "5", want: []int64{5}},
		{name: "several with spaces", in: "5, 9 ,5", want: []int64{5, 9}},
		{name: "not a number", in: "5,abc", wantErr: true},
		{name: "zero", in: "0", wantErr: true},
		{name: "only separators", in: " , ", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIDList(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseIDList(%q) = %v, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIDList(%q): %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseIDList(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseIDList(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

// One dashboard page holds at most 100 links, and a link names two facts.
func TestParseIDListCapsTheRequest(t *testing.T) {
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}
	if _, err := parseIDList(strings.Join(ids, ",")); err == nil {
		t.Fatal("201 ids should be refused")
	}
}

// A malformed ids list is refused before the handler touches the database, so a
// nil Brain still exercises the path.
func TestFactsByIDsRejectsBadID(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts?ids=5,abc", nil)
	rec := httptest.NewRecorder()
	h.facts(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// The mark is served from the embedded filesystem: the dashboard should not
// reach for a CDN, and a fresh install should not show a missing favicon.
func TestBrandAssetsAreServed(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	for _, name := range []string{"mark-96.png", "mark-192.png"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/brand/"+name, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() < 1000 {
			t.Errorf("GET /ui/brand/%s = %d (%d bytes)", name, rec.Code, rec.Body.Len())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("%s Content-Type = %q", name, ct)
		}
	}
	for _, name := range []string{"mark.svg", "nope.png", "../index.html", "%2e%2e/index.html"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/brand/"+name, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET /ui/brand/%s should not be served, got 200", name)
		}
	}
}
