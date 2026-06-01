package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withOverpassMirrors temporarily replaces the package mirror list for the
// duration of a test and restores it afterwards.
func withOverpassMirrors(t *testing.T, mirrors []overpassMirror) {
	t.Helper()
	orig := overpassMirrors
	overpassMirrors = mirrors
	t.Cleanup(func() { overpassMirrors = orig })
}

func postOverpass(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/map/overpass", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.handleMapOverpass(rec, req)
	return rec
}

func TestHandleMapOverpass_ProxiesQueryWithProperUserAgent(t *testing.T) {
	var gotUA, gotData, gotContentType string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotContentType = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotData = r.Form.Get("data")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"elements":[{"type":"way","id":1}]}`))
	}))
	defer fake.Close()

	withOverpassMirrors(t, []overpassMirror{{ID: "fake", URL: fake.URL}})
	server := NewServer(nil, nil, "mph", "UTC")

	rec := postOverpass(t, server, `{"query":"[out:json];way(1);out geom;"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, `"elements"`) {
		t.Errorf("expected proxied Overpass JSON, got %q", got)
	}
	if mirror := rec.Header().Get("X-Overpass-Mirror"); mirror != "fake" {
		t.Errorf("expected X-Overpass-Mirror=fake, got %q", mirror)
	}
	// The whole point of the proxy: a descriptive, non-browser User-Agent that
	// the public mirrors accept (they 406/403 generic Mozilla/* agents).
	if gotUA != overpassUserAgent {
		t.Errorf("expected User-Agent %q, got %q", overpassUserAgent, gotUA)
	}
	if !strings.Contains(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("expected form-encoded upstream request, got Content-Type %q", gotContentType)
	}
	if gotData == "" {
		t.Error("expected upstream to receive a non-empty data= parameter")
	}
}

func TestHandleMapOverpass_FallsBackToNextMirror(t *testing.T) {
	// First mirror rejects browser-style requests the way overpass-api.de does.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
	}))
	defer failing.Close()
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"elements":[]}`))
	}))
	defer working.Close()

	withOverpassMirrors(t, []overpassMirror{
		{ID: "first", URL: failing.URL},
		{ID: "second", URL: working.URL},
	})
	server := NewServer(nil, nil, "mph", "UTC")

	rec := postOverpass(t, server, `{"query":"[out:json];out;"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 via fallback, got %d (body %q)", rec.Code, rec.Body.String())
	}
	if mirror := rec.Header().Get("X-Overpass-Mirror"); mirror != "second" {
		t.Errorf("expected fallback to second mirror, got X-Overpass-Mirror=%q", mirror)
	}
}

func TestHandleMapOverpass_PreferredMirrorTriedFirst(t *testing.T) {
	var firstHit, secondHit bool
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"elements":["first"]}`))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"elements":["second"]}`))
	}))
	defer second.Close()

	withOverpassMirrors(t, []overpassMirror{
		{ID: "first", URL: first.URL},
		{ID: "second", URL: second.URL},
	})
	server := NewServer(nil, nil, "mph", "UTC")

	// Prefer "second" — it must be tried first and serve the request.
	rec := postOverpass(t, server, `{"query":"[out:json];out;","mirror":"second"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !secondHit {
		t.Error("preferred mirror 'second' was not hit")
	}
	if firstHit {
		t.Error("non-preferred mirror 'first' should not have been hit when 'second' succeeded")
	}
	if mirror := rec.Header().Get("X-Overpass-Mirror"); mirror != "second" {
		t.Errorf("expected X-Overpass-Mirror=second, got %q", mirror)
	}
}

func TestHandleMapOverpass_AllMirrorsFail(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer down.Close()

	withOverpassMirrors(t, []overpassMirror{
		{ID: "a", URL: down.URL},
		{ID: "b", URL: down.URL},
	})
	server := NewServer(nil, nil, "mph", "UTC")

	rec := postOverpass(t, server, `{"query":"[out:json];out;"}`)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when all mirrors fail, got %d (body %q)", rec.Code, rec.Body.String())
	}
}

func TestHandleMapOverpass_HTMLResponseIsRejected(t *testing.T) {
	// Mirrors return an HTML error page (HTTP 200) when overloaded; the proxy
	// must treat that as a failure rather than passing HTML to the SVG builder.
	htmlMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>timeout</body></html>"))
	}))
	defer htmlMirror.Close()
	jsonMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"elements":[]}`))
	}))
	defer jsonMirror.Close()

	withOverpassMirrors(t, []overpassMirror{
		{ID: "html", URL: htmlMirror.URL},
		{ID: "json", URL: jsonMirror.URL},
	})
	server := NewServer(nil, nil, "mph", "UTC")

	rec := postOverpass(t, server, `{"query":"[out:json];out;"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after skipping HTML mirror, got %d", rec.Code)
	}
	if mirror := rec.Header().Get("X-Overpass-Mirror"); mirror != "json" {
		t.Errorf("expected to skip HTML mirror and use json, got %q", mirror)
	}
}

func TestHandleMapOverpass_RejectsBadRequests(t *testing.T) {
	server := NewServer(nil, nil, "mph", "UTC")

	t.Run("empty query", func(t *testing.T) {
		rec := postOverpass(t, server, `{"query":"   "}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for empty query, got %d", rec.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		rec := postOverpass(t, server, `{not json`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/map/overpass", nil)
		rec := httptest.NewRecorder()
		server.handleMapOverpass(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 for GET, got %d", rec.Code)
		}
	})
}

// Guard: the frontend dropdown (OVERPASS_MIRRORS in map-svg.ts) and the Go
// mirror list share ids. This documents the default set so an accidental
// divergence is at least visible in one place.
func TestOrderedOverpassMirrors_PreferredMovesToFront(t *testing.T) {
	withOverpassMirrors(t, []overpassMirror{
		{ID: "de", URL: "u1"}, {ID: "fr", URL: "u2"}, {ID: "ch", URL: "u3"},
	})

	got := orderedOverpassMirrors("ch")
	if len(got) != 3 || got[0].ID != "ch" {
		t.Fatalf("expected 'ch' first, got %+v", got)
	}
	// Remaining order preserved.
	if got[1].ID != "de" || got[2].ID != "fr" {
		t.Errorf("expected de,fr to follow in order, got %s,%s", got[1].ID, got[2].ID)
	}

	// Unknown / empty preference keeps the original order untouched.
	if got := orderedOverpassMirrors(""); got[0].ID != "de" {
		t.Errorf("empty preference should keep de first, got %s", got[0].ID)
	}
	if got := orderedOverpassMirrors("zz"); got[0].ID != "de" {
		t.Errorf("unknown preference should keep de first, got %s", got[0].ID)
	}
}

// Sanity: ensure the proxy doesn't accidentally swallow the body when the
// upstream returns a large-ish JSON payload.
func TestHandleMapOverpass_PassesThroughBody(t *testing.T) {
	payload := `{"elements":[` + strings.Repeat(`{"type":"node","id":1,"lat":1.0,"lon":2.0},`, 100) + `{"type":"node","id":2,"lat":3.0,"lon":4.0}]}`
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	defer mirror.Close()

	withOverpassMirrors(t, []overpassMirror{{ID: "big", URL: mirror.URL}})
	server := NewServer(nil, nil, "mph", "UTC")

	rec := postOverpass(t, server, `{"query":"[out:json];out;"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != payload {
		t.Errorf("proxied body mismatch: got %d bytes, want %d", rec.Body.Len(), len(payload))
	}
}
