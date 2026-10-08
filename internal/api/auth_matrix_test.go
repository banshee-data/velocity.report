package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

// The cells of the integration matrix #503's review asked for, each through
// the wrapper the server installs rather than a handler or gate in isolation:
// viewer, admin, no grant, LAN, direct tailnet address, Funnel, daemon
// failure, enrolment URL visibility, and a LAN numbered in 100.64.0.0/10.
// Serve and TCP forwards are covered at the listener in serve_forwards_test.go.

var matrixStatus = tailscale.Status{
	DaemonRunning: true, BackendState: "NeedsLogin", LoginURL: "https://login.tailscale.com/a/123",
	LoginInProgress: true, Hostname: "velocity", Version: 7,
}

type matrixPeer struct {
	identity tailscale.PeerIdentity
	err      error
}

func matrixServer(mode CapEnforcement, peer matrixPeer) (*Server, http.Handler, *stubTailscale) {
	ts := &stubTailscale{status: matrixStatus}
	s := tailscaleTestServer(ts)
	s.SetAuthGate(&fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		return peer.identity, peer.err
	}}, mode)
	return s, s.authWrapper(s.ServeMux()), ts
}

func matrixRequest(h http.Handler, method, path, remote string, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = remote
	// A literal private address, as a browser on the LAN or tailnet sends;
	// hardened mode refuses names to authenticated direct callers.
	r.Host = "192.168.1.10"
	for k, v := range header {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func refusal(rec *httptest.ResponseRecorder) string {
	var body forbiddenBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error
}

var notFound = tailscale.ErrPeerNotFound

func TestOnModeMatrixThroughTheWrapper(t *testing.T) {
	const direct = "100.100.1.2:40000"
	const loop = "127.0.0.1:40000"
	serve := map[string]string{"X-Forwarded-For": "100.100.1.2"}
	cases := []struct {
		name         string
		peer         matrixPeer
		method, path string
		remote       string
		header       map[string]string
		want         int
		reason       string
	}{
		{"direct viewer reads", matrixPeer{identity: id(true, false)}, "GET", "/api/version", direct, nil, 200, ""},
		{"direct viewer writes", matrixPeer{identity: id(true, false)}, "POST", "/api/tailscale/enable", direct, nil, 403, "missing_cap"},
		{"direct admin writes", matrixPeer{identity: id(false, true)}, "POST", "/api/tailscale/enable", direct, nil, 200, ""},
		{"direct peer without grants", matrixPeer{identity: id(false, false)}, "GET", "/api/version", direct, nil, 403, "missing_cap"},
		{"direct unknown peer", matrixPeer{err: notFound}, "GET", "/api/version", direct, nil, 403, "unknown_peer"},
		{"direct lookup failure", matrixPeer{err: errors.New("daemon down")}, "GET", "/api/version", direct, nil, 503, "peer_lookup_unavailable"},
		{"Serve viewer reads", matrixPeer{identity: id(true, false)}, "GET", "/api/version", loop, serve, 200, ""},
		{"Serve viewer reads the docs", matrixPeer{identity: id(true, false)}, "GET", "/docs/", loop, serve, 404, ""},
		{"Serve viewer writes", matrixPeer{identity: id(true, false)}, "POST", "/api/tailscale/enable", loop, serve, 403, "missing_cap"},
		{"Serve peer without grants", matrixPeer{identity: id(false, false)}, "GET", "/api/version", loop, serve, 403, "missing_cap"},
		{"Serve lookup failure", matrixPeer{err: errors.New("daemon down")}, "GET", "/api/version", loop, serve, 503, "peer_lookup_unavailable"},
		{"Funnel on a gated route", matrixPeer{identity: id(false, true)}, "GET", "/api/version", loop, map[string]string{funnelRequestHeader: "?1", "X-Forwarded-For": "203.0.113.7"}, 403, "funnel_request"},
		{"Funnel on an ungated route", matrixPeer{identity: id(false, true)}, "GET", "/api/tailscale/status", loop, map[string]string{funnelRequestHeader: "?1"}, 403, "funnel_request"},
		{"LAN is admin", matrixPeer{err: errors.New("never asked")}, "POST", "/api/tailscale/enable", "192.168.1.20:5555", nil, 200, ""},
		{"host is admin", matrixPeer{err: errors.New("never asked")}, "POST", "/api/tailscale/enable", loop, nil, 200, ""},
		// A LAN numbered in RFC 6598 shared address space reads as the
		// tailnet; the daemon does not know its hosts, so they are refused.
		{"RFC 6598 LAN host", matrixPeer{err: notFound}, "GET", "/api/version", "100.64.5.5:5555", nil, 403, "unknown_peer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, h, _ := matrixServer(EnforcementOn, c.peer)
			rec := matrixRequest(h, c.method, c.path, c.remote, c.header)
			if rec.Code != c.want || (c.reason != "" && refusal(rec) != c.reason) {
				t.Fatalf("HTTP %d %q, want %d %q", rec.Code, rec.Body.String(), c.want, c.reason)
			}
		})
	}
}

// The enrolment URL claims the device for whoever opens it.  Through the
// wrapper, on mode shows it to the host, the LAN and tailnet admins only, and
// hardened mode to no HTTP caller.
func TestEnrolmentURLVisibilityThroughTheWrapper(t *testing.T) {
	serve := map[string]string{"X-Forwarded-For": "100.100.1.2"}
	cases := []struct {
		name    string
		mode    CapEnforcement
		peer    matrixPeer
		remote  string
		header  map[string]string
		visible bool
	}{
		{"on: host", EnforcementOn, matrixPeer{}, "127.0.0.1:1", nil, true},
		{"on: LAN", EnforcementOn, matrixPeer{}, "192.168.1.20:1", nil, true},
		{"on: tailnet admin", EnforcementOn, matrixPeer{identity: id(false, true)}, "127.0.0.1:1", serve, true},
		{"on: tailnet viewer", EnforcementOn, matrixPeer{identity: id(true, false)}, "127.0.0.1:1", serve, false},
		{"on: tailnet peer without grants", EnforcementOn, matrixPeer{identity: id(false, false)}, "100.100.1.2:1", nil, false},
		{"on: lookup failure", EnforcementOn, matrixPeer{err: errors.New("down")}, "100.100.1.2:1", nil, false},
		{"hardened: LAN", EnforcementHardened, matrixPeer{}, "192.168.1.20:1", nil, false},
		{"hardened: host", EnforcementHardened, matrixPeer{}, "127.0.0.1:1", nil, false},
		{"hardened: direct admin", EnforcementHardened, matrixPeer{identity: id(false, true)}, "100.100.1.2:1", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, h, _ := matrixServer(c.mode, c.peer)
			rec := matrixRequest(h, "GET", "/api/tailscale/status", c.remote, c.header)
			if rec.Code != http.StatusOK {
				t.Fatalf("HTTP %d %q", rec.Code, rec.Body.String())
			}
			if got := strings.Contains(rec.Body.String(), matrixStatus.LoginURL); got != c.visible {
				t.Fatalf("login URL visible = %v, want %v: %s", got, c.visible, rec.Body.String())
			}
		})
	}
	// Hardened Serve callers, admin included, never see it either.
	s, _, _ := matrixServer(EnforcementHardened, matrixPeer{})
	rec := matrixRequest(s.hardenedWrapper(s.ServeMux(), true), "GET", "/api/tailscale/status", "127.0.0.1:1",
		map[string]string{serveCapabilitiesHeader: `{"velocity.report/cap/admin":[]}`})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), matrixStatus.LoginURL) {
		t.Fatalf("hardened Serve admin: HTTP %d %s", rec.Code, rec.Body.String())
	}
}

func TestHardenedMatrixThroughTheWrapper(t *testing.T) {
	admin := map[string]string{serveCapabilitiesHeader: `{"velocity.report/cap/admin":[]}`}
	cases := []struct {
		name   string
		serve  bool
		peer   matrixPeer
		remote string
		header map[string]string
		want   int
		reason string
	}{
		{"LAN reads aggregates", false, matrixPeer{}, "192.168.1.20:1", nil, 200, ""},
		{"direct peer without grants", false, matrixPeer{identity: id(false, false)}, "100.100.1.2:1", nil, 403, "permission_denied"},
		{"direct viewer", false, matrixPeer{identity: id(true, false)}, "100.100.1.2:1", nil, 200, ""},
		{"direct lookup failure", false, matrixPeer{err: errors.New("down")}, "100.100.1.2:1", nil, 503, "peer_lookup_unavailable"},
		{"RFC 6598 LAN host", false, matrixPeer{err: notFound}, "100.64.5.5:1", nil, 403, "unknown_peer"},
		{"public source", false, matrixPeer{}, "203.0.113.7:1", nil, 403, "untrusted_source"},
		{"Funnel on the LAN listener", false, matrixPeer{}, "127.0.0.1:1", map[string]string{funnelRequestHeader: "?1"}, 403, "funnel_request"},
		{"Funnel on the Serve backend", true, matrixPeer{}, "127.0.0.1:1", map[string]string{funnelRequestHeader: "?1", serveCapabilitiesHeader: `{"velocity.report/cap/admin":[]}`}, 403, "funnel_request"},
		{"Serve admin", true, matrixPeer{}, "127.0.0.1:1", admin, 200, ""},
		{"Serve without a claim", true, matrixPeer{}, "127.0.0.1:1", nil, 403, "invalid_serve_claims"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, _ := matrixServer(EnforcementHardened, c.peer)
			rec := matrixRequest(s.hardenedWrapper(s.ServeMux(), c.serve), "GET", "/api/version", c.remote, c.header)
			if rec.Code != c.want || (c.reason != "" && refusal(rec) != c.reason) {
				t.Fatalf("HTTP %d %q, want %d %q", rec.Code, rec.Body.String(), c.want, c.reason)
			}
		})
	}
}

// The on-mode view grant, route by route and method by method.  Every main
// API pattern the hardened inventory knows is listed, so a new route there
// must be placed here too.  A route not listed is admin-only.
func TestOnModeViewGrantInventory(t *testing.T) {
	const (
		ungated = "ungated"
		view    = "view"
		readish = "view to read, admin to write"
		admin   = "admin"
	)
	inventory := map[string]string{
		"/":                                  ungated,
		"/app":                               ungated,
		"/app/":                              ungated,
		"/app/settings":                      ungated,
		"/favicon.ico":                       ungated,
		"/api/access":                        ungated,
		"/api/tailscale/status":              ungated,
		"/api/commands":                      view,
		"/api/events":                        view,
		"/api/radar_stats":                   view,
		"/api/config":                        view,
		"/api/capabilities":                  view,
		"/api/version":                       view,
		"/api/timeline":                      view,
		"/api/db_stats":                      view,
		"/api/charts/timeseries":             view,
		"/api/charts/histogram":              view,
		"/api/charts/comparison":             view,
		"/api/sites":                         readish,
		"/api/sites/":                        readish,
		"/api/sites/3":                       readish,
		"/api/site_config_periods":           readish,
		"/api/reports":                       readish,
		"/api/reports/":                      readish,
		"/api/reports/9/download/report.zip": readish,
		"/docs":                              readish,
		"/docs/":                             readish,
		"/docs/guides/setup":                 readish,
		"/api/generate_report":               admin,
		"/admin/radar/command":               admin,
		"/api/scenes":                        admin,
		"/api/scenes/":                       admin,
		"/api/transit_worker":                admin,
		"/api/tailscale/enable":              admin,
		"/api/tailscale/disable":             admin,
		"/api/serial/models":                 admin,
		"/api/serial/devices":                admin,
		"/api/serial/test":                   admin,
		"/api/serial/reload":                 admin,
		"/api/serial/configs":                admin,
		"/api/serial/configs/":               admin,
		"/api/lidar/status":                  admin,
		"/api/lidar/runs":                    admin,
		"/debug/backup":                      admin,
		"/apps":                              admin,
		"/docsx":                             admin,
		"/api/sitesx":                        admin,
	}
	for pattern := range hardenedMainPatterns {
		if _, ok := inventory[pattern]; !ok {
			t.Errorf("%s is a main API route without an on-mode classification here", pattern)
		}
	}
	for path, want := range inventory {
		for _, method := range []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"} {
			gated, required := classifyRoute(path, method)
			got := admin
			switch {
			case !gated:
				got = ungated
			case required == CapView:
				got = view
			}
			expect := want
			if want == readish {
				expect = admin
				if method == "GET" || method == "HEAD" || method == "OPTIONS" {
					expect = view
				}
			}
			if got != expect {
				t.Errorf("%s %s: %s, want %s", method, path, got, expect)
			}
		}
	}
}
