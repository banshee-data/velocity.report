package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

// fakePeerAuth is a hand-rolled stub of PeerAuthClient.  Function
// fields keep tests free of mocking ceremony.
type fakePeerAuth struct {
	lookup   func(ip string) (tailscale.PeerIdentity, error)
	prefixes []netip.Prefix
}

func (f *fakePeerAuth) LookupPeer(_ context.Context, remoteAddr string) (tailscale.PeerIdentity, error) {
	if f.lookup == nil {
		return tailscale.PeerIdentity{}, errors.New("not configured")
	}
	return f.lookup(remoteAddr)
}

func (f *fakePeerAuth) LocalTailnetPrefixes(_ context.Context) []netip.Prefix {
	return f.prefixes
}

func id(view, admin bool) tailscale.PeerIdentity {
	if admin {
		return tailscale.PeerIdentity{View: true, Admin: true}
	}
	return tailscale.PeerIdentity{View: view}
}

func mkReq(remoteAddr, xff string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func runGate(t *testing.T, g *authGate, kind CapKind, r *http.Request) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	g.requireCap(kind, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)
	return rec.Code, rec.Body.String()
}

// --- Source classification --------------------------------------------------

func TestClassifySource_XFFOnlyTrustedFromLoopback(t *testing.T) {
	// The principal-eng review's #1: XFF must NOT be honoured when
	// r.RemoteAddr is non-loopback, otherwise a LAN attacker who
	// can reach :8080 directly forges a tailnet identity.
	g := newAuthGate(&fakePeerAuth{}, EnforcementOn)

	tailnetXFF := "100.64.0.5"

	// Loopback upstream + XFF tailnet IP -> classified as tailnet.
	r := mkReq("127.0.0.1:54321", tailnetXFF)
	ip, fromTailnet := classifySource(r, g, context.Background())
	if !fromTailnet {
		t.Errorf("loopback+XFF tailnet: expected tailnet, got non-tailnet")
	}
	if ip.String() != tailnetXFF {
		t.Errorf("loopback+XFF: ip=%s, want %s", ip, tailnetXFF)
	}

	// LAN upstream + spoofed XFF tailnet IP -> NOT tailnet.
	// The XFF must be ignored entirely.
	r = mkReq("192.168.1.50:33445", tailnetXFF)
	ip, fromTailnet = classifySource(r, g, context.Background())
	if fromTailnet {
		t.Errorf("LAN+spoofed XFF: expected non-tailnet, got tailnet (forgery accepted)")
	}
	if ip.String() != "192.168.1.50" {
		t.Errorf("LAN+spoofed XFF: ip=%s, want 192.168.1.50", ip)
	}

	// Loopback + no XFF -> loopback IP, non-tailnet (host-local).
	r = mkReq("127.0.0.1:54321", "")
	ip, fromTailnet = classifySource(r, g, context.Background())
	if fromTailnet || !ip.IsLoopback() {
		t.Errorf("loopback no XFF: ip=%s tailnet=%v, want loopback non-tailnet", ip, fromTailnet)
	}
}

// --- CGNAT classifier ------------------------------------------------------

func TestIsTailscaleCGNAT(t *testing.T) {
	cases := map[string]bool{
		"100.64.0.1":             true,
		"100.127.255.255":        true,
		"100.128.0.0":            false,
		"10.0.0.1":               false,
		"192.168.1.1":            false,
		"127.0.0.1":              false,
		"fd7a:115c:a1e0::1":      true,
		"fd7a:115c:a1e0:abcd::1": true,
		"fd7b:115c:a1e0::1":      false,
		"fe80::1":                false,
	}
	for s, want := range cases {
		if got := isTailscaleCGNAT(netip.MustParseAddr(s)); got != want {
			t.Errorf("%s: got %v, want %v", s, got, want)
		}
	}
}

// --- Mode behaviour --------------------------------------------------------

func TestRequireCap_OffMode_AllowsEverything(t *testing.T) {
	tc := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		t.Fatal("LookupPeer must not be called when mode=off")
		return tailscale.PeerIdentity{}, nil
	}}
	g := newAuthGate(tc, EnforcementOff)
	r := mkReq("127.0.0.1:1234", "100.64.0.5")
	if got, _ := runGate(t, g, CapAdmin, r); got != http.StatusOK {
		t.Fatalf("off mode: got %d, want 200", got)
	}
}

func TestRequireCap_NonTailnetSource_AlwaysAdmin(t *testing.T) {
	tc := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		t.Fatal("LookupPeer must not be called for non-tailnet sources")
		return tailscale.PeerIdentity{}, nil
	}}
	g := newAuthGate(tc, EnforcementOn)

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
	}{
		{"loopback no XFF", "127.0.0.1:1234", ""},
		{"LAN direct", "192.168.1.10:5555", ""},
		{"LAN with spoofed XFF", "10.0.0.1:5555", "100.64.0.99"},
	}
	for _, tc2 := range cases {
		t.Run(tc2.name, func(t *testing.T) {
			r := mkReq(tc2.remoteAddr, tc2.xff)
			if got, _ := runGate(t, g, CapAdmin, r); got != http.StatusOK {
				t.Fatalf("non-tailnet admin: got %d, want 200", got)
			}
		})
	}
}

// --- Cap matrix ------------------------------------------------------------

func TestRequireCap_GrantMatrix(t *testing.T) {
	cases := []struct {
		name     string
		identity tailscale.PeerIdentity
		required CapKind
		want     int
	}{
		{"admin grants view", id(false, true), CapView, http.StatusOK},
		{"admin grants admin", id(false, true), CapAdmin, http.StatusOK},
		{"view allows view", id(true, false), CapView, http.StatusOK},
		{"view denies admin", id(true, false), CapAdmin, http.StatusForbidden},
		{"none denies view", id(false, false), CapView, http.StatusForbidden},
		{"none denies admin", id(false, false), CapAdmin, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pa := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
				return tc.identity, nil
			}}
			g := newAuthGate(pa, EnforcementOn)
			r := mkReq("127.0.0.1:1234", "100.64.0.5")
			if got, _ := runGate(t, g, tc.required, r); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// --- Failure modes ---------------------------------------------------------

func TestRequireCap_TransientLookupError_FailsOpen(t *testing.T) {
	// Per the trust model: transient errors fail open so a
	// tailscaled blip does not deny every authorised user at once.
	pa := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		return tailscale.PeerIdentity{}, errors.New("socket: connection refused")
	}}
	g := newAuthGate(pa, EnforcementOn)
	r := mkReq("127.0.0.1:1234", "100.64.0.5")
	if got, _ := runGate(t, g, CapAdmin, r); got != http.StatusOK {
		t.Fatalf("transient error: got %d, want 200 (fail-open)", got)
	}
}

func TestRequireCap_PeerNotFound_FailsClosed(t *testing.T) {
	// Authoritative "not a tailnet peer" -> 403 with structured body.
	pa := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		return tailscale.PeerIdentity{}, tailscale.ErrPeerNotFound
	}}
	g := newAuthGate(pa, EnforcementOn)
	r := mkReq("127.0.0.1:1234", "100.64.0.5")
	got, body := runGate(t, g, CapAdmin, r)
	if got != http.StatusForbidden {
		t.Fatalf("peer-not-found: got %d, want 403", got)
	}
	var fb forbiddenBody
	if err := json.Unmarshal([]byte(body), &fb); err != nil {
		t.Fatalf("body should be JSON: %v (body: %q)", err, body)
	}
	if fb.Error != "unknown_peer" {
		t.Errorf("error code = %q, want unknown_peer", fb.Error)
	}
	if fb.Required != "admin" {
		t.Errorf("required = %q, want admin", fb.Required)
	}
}

func TestRequireCap_MissingCap_StructuredBody(t *testing.T) {
	pa := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		return id(true, false), nil // view only
	}}
	g := newAuthGate(pa, EnforcementOn)
	r := mkReq("127.0.0.1:1234", "100.64.0.5")
	got, body := runGate(t, g, CapAdmin, r)
	if got != http.StatusForbidden {
		t.Fatalf("missing admin cap: got %d, want 403", got)
	}
	var fb forbiddenBody
	if err := json.Unmarshal([]byte(body), &fb); err != nil {
		t.Fatalf("body should be JSON: %v (body: %q)", err, body)
	}
	if fb.Error != "missing_cap" {
		t.Errorf("error code = %q, want missing_cap", fb.Error)
	}
}

func TestRequireCap_NilClient_AllowsEverything(t *testing.T) {
	g := newAuthGate(nil, EnforcementOn) // mode is forced to Off
	r := mkReq("127.0.0.1:1234", "100.64.0.5")
	if got, _ := runGate(t, g, CapAdmin, r); got != http.StatusOK {
		t.Fatalf("nil client: got %d, want 200", got)
	}
}

// --- Route classifier ------------------------------------------------------

func TestClassifyRoute(t *testing.T) {
	cases := []struct {
		path, method string
		gated        bool
		required     CapKind
	}{
		// Allowlist
		{"/", "GET", false, 0},
		{"/app/", "GET", false, 0},
		{"/app/index.html", "GET", false, 0},
		{"/favicon.ico", "GET", false, 0},
		{"/api/tailscale/status", "GET", false, 0},

		// Explicit view list
		{"/api/commands", "GET", true, CapView},
		{"/api/config", "GET", true, CapView},
		{"/api/events", "GET", true, CapView},
		{"/api/version", "GET", true, CapView},
		{"/api/timeline", "POST", true, CapView}, // method-agnostic for these
		{"/api/charts/histogram", "GET", true, CapView},

		// Mixed-method REST
		{"/api/sites", "GET", true, CapView},
		{"/api/sites", "POST", true, CapAdmin},
		{"/api/sites/42", "GET", true, CapView},
		{"/api/sites/42", "DELETE", true, CapAdmin},
		{"/api/reports/abc", "GET", true, CapView},
		{"/api/reports/abc", "DELETE", true, CapAdmin},

		// Default-deny on anything not classified
		{"/api/generate_report", "POST", true, CapAdmin},
		{"/api/transit_worker", "POST", true, CapAdmin},
		{"/api/tailscale/enable", "POST", true, CapAdmin},
		{"/api/tailscale/disable", "POST", true, CapAdmin},
		{"/admin/radar/command", "POST", true, CapAdmin},
		{"/events", "GET", true, CapView},
		{"/debug/pprof/", "GET", true, CapAdmin},          // tsweb debug routes
		{"/debug/db/backup", "POST", true, CapAdmin},      // db admin routes
		{"/api/lidar/runs/", "GET", true, CapAdmin},       // lidar routes default to admin
		{"/api/totally-new-route", "GET", true, CapAdmin}, // future routes default-deny
		{"/docs/", "GET", true, CapAdmin},                 // docsite — admin by default
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.method, func(t *testing.T) {
			gated, required := classifyRoute(tc.path, tc.method)
			if gated != tc.gated {
				t.Errorf("gated=%v, want %v", gated, tc.gated)
			}
			if gated && required != tc.required {
				t.Errorf("required=%v, want %v", required, tc.required)
			}
		})
	}
}

// --- ParseEnforcement ------------------------------------------------------

func TestParseEnforcement(t *testing.T) {
	cases := map[string]CapEnforcement{
		"":      EnforcementOff,
		"off":   EnforcementOff,
		"OFF":   EnforcementOff,
		"false": EnforcementOff,
		"0":     EnforcementOff,
		"no":    EnforcementOff,
		"on":    EnforcementOn,
		"true":  EnforcementOn,
		"1":     EnforcementOn,
		"yes":   EnforcementOn,
	}
	for in, want := range cases {
		got, err := ParseEnforcement(in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
		}
		if got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"auto", "garbage", "maybe"} {
		if _, err := ParseEnforcement(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

// --- Server-level integration ----------------------------------------------

// These tests assemble a Server with the auth wrapper installed and
// exercise it via httptest, ensuring the default-deny mux semantics
// hold for routes the gate is not aware of.
func TestServer_AuthWrapper_DefaultDenyForUnknownRoute(t *testing.T) {
	// A handler attached to the mux *after* SetAuthGate is called
	// (e.g. AttachAdminRoutes from external packages) should
	// still be subject to the gate.  Simulate that here by
	// registering an arbitrary "admin" route.
	pa := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		return id(true, false), nil // view-only peer
	}}
	s := &Server{}
	s.SetAuthGate(pa, EnforcementOn)
	mux := s.ServeMux()
	mux.HandleFunc("/debug/some-new-admin-route", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := s.authWrapper(mux)

	// Tailnet-classified peer with view-only cap hitting an
	// un-classified route -> default-deny -> 403.
	r := mkReq("127.0.0.1:1234", "100.64.0.5")
	r.URL.Path = "/debug/some-new-admin-route"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("default-deny: got %d, want 403", rec.Code)
	}

	// Same peer hitting an allowlisted route -> the gate must NOT
	// block it.  /api/tailscale/status is registered by ServeMux();
	// we ride the existing handler to confirm the wrapper passes.
	r = mkReq("127.0.0.1:1234", "100.64.0.5")
	r.URL.Path = "/api/tailscale/status"
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, r)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("allowlisted route was blocked: %d %q", rec.Code, rec.Body.String())
	}
}
