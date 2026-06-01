package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// overpassMirror is a single public Overpass API endpoint.
type overpassMirror struct {
	ID  string
	URL string
}

// overpassMirrors lists public Overpass API endpoints in priority order.
//
// These IDs mirror OVERPASS_MIRRORS in web/src/lib/map-svg.ts so the frontend's
// mirror dropdown can pass a preferred-mirror hint by id.
//
// This MUST run server-side rather than as a direct browser fetch: several
// public mirrors (overpass-api.de, openstreetmap.fr) reject generic browser
// User-Agents with HTTP 406/403, and `fetch` cannot override the User-Agent
// header (it is forbidden). Proxying through the Go server lets us send a
// descriptive User-Agent the mirrors accept, and keeps the request same-origin
// so the browser is not subject to the mirrors' (absent) CORS headers.
//
// Declared as a package var so tests can substitute a fake Overpass server.
var overpassMirrors = []overpassMirror{
	{ID: "de", URL: "https://overpass-api.de/api/interpreter"},
	{ID: "fr", URL: "https://overpass.openstreetmap.fr/api/interpreter"},
	{ID: "ch", URL: "https://overpass.kumi.systems/api/interpreter"},
	{ID: "at", URL: "https://overpass.private.coffee/api/interpreter"},
	{ID: "ru", URL: "https://maps.mail.ru/osm/tools/overpass/api/interpreter"},
}

// overpassUserAgent identifies velocity.report to Overpass mirrors. A
// descriptive, non-browser User-Agent is required: the busy public mirrors
// reject generic Mozilla/* agents to discourage direct in-browser scraping.
const overpassUserAgent = "velocity.report/1.0 (+https://velocity.report; map download for radar survey reports)"

// overpassPerMirrorTimeout bounds a single mirror attempt. The Overpass query
// itself carries an in-query [timeout:25]; we allow a little headroom over that
// before moving to the next mirror.
const overpassPerMirrorTimeout = 30 * time.Second

// maxOverpassQueryBytes caps the inbound Overpass QL query size.
const maxOverpassQueryBytes = 64 * 1024

// maxOverpassResponseBytes caps the upstream Overpass response we will buffer
// (Overpass payloads for a small bbox are typically well under a few MB).
const maxOverpassResponseBytes = 32 * 1024 * 1024

// overpassHTTPClient is shared so connections are reused across mirror attempts.
// Per-attempt deadlines come from a request-scoped context, not this client's
// Timeout, so a slow body read on a healthy mirror is not cut short.
var overpassHTTPClient = &http.Client{}

// overpassProxyRequest is the JSON body accepted by POST /api/map/overpass.
type overpassProxyRequest struct {
	// Query is the Overpass QL query string (as built by buildOverpassQueries
	// on the frontend).
	Query string `json:"query"`
	// Mirror is an optional preferred mirror id ("de", "fr", ...). When set and
	// recognised, that mirror is tried first; the rest follow as fallback.
	Mirror string `json:"mirror,omitempty"`
}

// handleMapOverpass handles POST /api/map/overpass.
//
// It proxies an Overpass QL query to the public Overpass mirrors with a
// descriptive User-Agent and server-side fallback, returning the first
// successful JSON response. The frontend uses this instead of fetching Overpass
// directly because browser fetches are blocked by the mirrors' User-Agent
// filtering (406/403) and lack of CORS headers.
func (s *Server) handleMapOverpass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req overpassProxyRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOverpassQueryBytes))
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		http.Error(w, "Missing Overpass query", http.StatusBadRequest)
		return
	}

	endpoints := orderedOverpassMirrors(req.Mirror)

	var lastErr string
	for _, ep := range endpoints {
		body, servedBy, err := fetchFromOverpassMirror(r.Context(), ep, req.Query)
		if err != nil {
			// Caller hung up — stop immediately, no point trying more mirrors.
			if r.Context().Err() != nil {
				return
			}
			lastErr = fmt.Sprintf("%s: %v", ep.ID, err)
			log.Printf("overpass: mirror %s failed: %v", ep.ID, err)
			continue
		}
		w.Header().Set("Content-Type", "application/json")
		// Surface which mirror served the request so the UI can show it.
		w.Header().Set("X-Overpass-Mirror", servedBy)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			log.Printf("overpass: failed writing response body: %v", err)
		}
		return
	}

	log.Printf("overpass: all mirrors failed (last: %s)", lastErr)
	http.Error(w, "All Overpass mirrors failed: "+lastErr, http.StatusBadGateway)
}

// orderedOverpassMirrors returns the mirror list with the preferred mirror (if
// recognised) moved to the front; declaration order is preserved otherwise.
func orderedOverpassMirrors(preferred string) []overpassMirror {
	if preferred == "" {
		return overpassMirrors
	}
	ordered := make([]overpassMirror, 0, len(overpassMirrors))
	var rest []overpassMirror
	for _, m := range overpassMirrors {
		if m.ID == preferred {
			ordered = append(ordered, m)
		} else {
			rest = append(rest, m)
		}
	}
	return append(ordered, rest...)
}

// fetchFromOverpassMirror issues a single Overpass request to one mirror,
// returning the response body and the mirror id on success. A non-200 status or
// an HTML body (mirrors return HTML error pages when overloaded) is treated as a
// failure so the caller falls through to the next mirror.
func fetchFromOverpassMirror(ctx context.Context, ep overpassMirror, query string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, overpassPerMirrorTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("data", query)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", overpassUserAgent)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := overpassHTTPClient.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain a little of the body for the log without holding the whole page.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "text/html") {
		return nil, "", fmt.Errorf("non-JSON response (content-type %q) — mirror likely overloaded", ct)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOverpassResponseBytes))
	if err != nil {
		return nil, "", err
	}
	return body, ep.ID, nil
}
