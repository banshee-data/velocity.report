package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/access"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/tailscale"
)

func hardenedRequest(method, target, source string) *http.Request {
	r := httptest.NewRequest(method, "http://100.64.0.2"+target, nil)
	r.RemoteAddr = source
	return r
}

func hardenedTestServer(identity tailscale.PeerIdentity, lookupErr error) *Server {
	s := &Server{}
	s.SetAuthGate(&fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) { return identity, lookupErr }}, EnforcementHardened)
	return s
}

func TestHardenedOperationMatrix(t *testing.T) {
	operations := []struct {
		method, path             string
		anonymous, viewer, admin int
	}{
		{"GET", "/api/config", 200, 200, 200},
		{"GET", "/api/charts/timeseries", 200, 200, 200},
		{"GET", "/api/sites", 200, 200, 200},
		{"GET", "/api/reports/", 200, 200, 200},
		{"GET", "/api/reports/1/download/report.pdf", 200, 200, 200},
		{"GET", "/api/reports/1/download/sources.zip", 403, 403, 200},
		{"HEAD", "/api/reports/1/download/sources.zip", 403, 403, 200},
		{"GET", "/api/reports/1/download/sources.ZIP", 403, 403, 403},
		{"GET", "/api/reports/1/download/sources.zip/extra", 403, 403, 403},
		{"DELETE", "/api/reports/1", 403, 403, 200},
		{"GET", "/api/events", 403, 403, 200},
		{"GET", "/api/serial/configs", 403, 403, 200},
		{"GET", "/api/db_stats", 403, 403, 200},
		{"GET", "/api/timeline", 403, 403, 200},
		{"POST", "/api/generate_report", 403, 403, 200},
		{"POST", "/admin/radar/command", 403, 403, 200},
		{"PUT", "/api/sites/1", 403, 403, 200},
		{"POST", "/api/tailscale/disable", 403, 403, 403},
		{"GET", "/debug/backup", 403, 403, 403},
		{"GET", "/api/forgotten", 403, 403, 403},
		{"POST", "/app/", 403, 403, 403},
		{"GET", "/docs/security", 200, 200, 200},
	}
	for _, actor := range []struct {
		name, source string
		identity     tailscale.PeerIdentity
		column       int
	}{
		{"LAN", "192.168.1.50:1234", tailscale.PeerIdentity{}, 0},
		{"loopback", "127.0.0.1:1234", tailscale.PeerIdentity{}, 0},
		{"viewer", "100.64.0.5:1234", tailscale.PeerIdentity{View: true}, 1},
		{"admin", "100.64.0.5:1234", tailscale.PeerIdentity{View: true, Admin: true}, 2},
	} {
		t.Run(actor.name, func(t *testing.T) {
			s := hardenedTestServer(actor.identity, nil)
			for _, op := range operations {
				t.Run(op.method+op.path, func(t *testing.T) {
					called := false
					w := httptest.NewRecorder()
					s.authWrapper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						called = true
						if _, ok := requestPrincipal(r); !ok {
							t.Error("missing resolved principal")
						}
						w.WriteHeader(200)
					})).ServeHTTP(w, hardenedRequest(op.method, op.path, actor.source))
					want := []int{op.anonymous, op.viewer, op.admin}[actor.column]
					if w.Code != want || called != (want == 200) {
						t.Fatalf("HTTP %d called=%v want %d: %s", w.Code, called, want, w.Body.String())
					}
				})
			}
		})
	}
}

func TestHardenedIngressRefusal(t *testing.T) {
	for _, tc := range []struct {
		name, source, header, value string
		err                         error
		want                        int
	}{
		{"public", "203.0.113.5:1234", "", "", nil, 403},
		{"malformed source", "invalid", "", "", nil, 403},
		{"LAN forged XFF", "192.168.1.50:1234", "X-Forwarded-For", "100.64.0.5", nil, 403},
		{"loopback forged cap", "127.0.0.1:1234", serveCapabilitiesHeader, `{"velocity.report/cap/admin":[]}`, nil, 403},
		{"Funnel", "127.0.0.1:1234", funnelRequestHeader, "?1", nil, 403},
		{"invalid credential", "192.168.1.50:1234", "Authorization", "Bearer invalid", nil, 403},
		{"unknown peer", "100.64.0.5:1234", "", "", tailscale.ErrPeerNotFound, 403},
		{"daemon failure", "100.64.0.5:1234", "", "", errors.New("unavailable"), 503},
		{"cross origin", "100.64.0.5:1234", "Origin", "https://hostile.example", nil, 403},
		{"cross site", "100.64.0.5:1234", "Sec-Fetch-Site", "cross-site", nil, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := hardenedTestServer(tailscale.PeerIdentity{Admin: true}, tc.err)
			r := hardenedRequest("GET", "/api/config", tc.source)
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			w := httptest.NewRecorder()
			s.authWrapper(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("refused request reached handler") })).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("HTTP %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestHardenedServeClaims(t *testing.T) {
	for _, tc := range []struct {
		name, claims, source, target string
		want                         int
	}{
		{"admin", `{"velocity.report/cap/admin":[]}`, "127.0.0.1:1234", "/api/serial/configs", 200},
		{"null grant", `{"velocity.report/cap/view":null}`, "[::1]:1234", "/api/config", 200},
		{"viewer write", `{"velocity.report/cap/view":[]}`, "127.0.0.1:1234", "/api/serial/configs", 403},
		{"missing", "", "127.0.0.1:1234", "/api/config", 403},
		{"empty", `{}`, "127.0.0.1:1234", "/api/config", 403},
		{"unknown", `{"other/cap":[]}`, "127.0.0.1:1234", "/api/config", 403},
		{"duplicate", `{"velocity.report/cap/admin":[],"velocity.report/cap/admin":[]}`, "127.0.0.1:1234", "/api/config", 403},
		{"bad arguments", `{"velocity.report/cap/admin":true}`, "127.0.0.1:1234", "/api/config", 403},
		{"trailing", `{"velocity.report/cap/admin":[]} {}`, "127.0.0.1:1234", "/api/config", 403},
		{"nonloopback", `{"velocity.report/cap/admin":[]}`, "192.168.1.50:1234", "/api/config", 403},
		{"oversized", strings.Repeat(" ", 8193), "127.0.0.1:1234", "/api/config", 403},
		{"encoded", mime.QEncoding.Encode("utf-8", `{"velocity.report/cap/admin":["é"]}`), "127.0.0.1:1234", "/api/serial/configs", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := hardenedTestServer(tailscale.PeerIdentity{}, errors.New("WhoIs must not supply Serve claims"))
			r := hardenedRequest("GET", tc.target, tc.source)
			r.Host = "device.tailnet.ts.net"
			if tc.claims != "" {
				r.Header.Set(serveCapabilitiesHeader, tc.claims)
			}
			w := httptest.NewRecorder()
			s.hardenedWrapper(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }), true).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("HTTP %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestHardenedAliasDoesNotInheritCollectionPermission(t *testing.T) {
	s := hardenedTestServer(tailscale.PeerIdentity{Admin: true}, nil)
	s.ServeMux().HandleFunc("/api/sites/secret", func(http.ResponseWriter, *http.Request) { t.Fatal("unregistered alias admitted") })
	w := httptest.NewRecorder()
	s.authWrapper(s.ServeMux()).ServeHTTP(w, hardenedRequest("GET", "/api/sites/secret", "100.64.0.5:1234"))
	if w.Code != 403 {
		t.Fatalf("got %d", w.Code)
	}
	s.RegisterOperation("POST /api/example/control", "", access.Configure)
	s.ServeMux().HandleFunc("POST /api/example/control", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	w = httptest.NewRecorder()
	s.authWrapper(s.ServeMux()).ServeHTTP(w, hardenedRequest("POST", "/api/example/control", "100.64.0.5:1234"))
	if w.Code != 204 {
		t.Fatalf("registered operation refused: %d %s", w.Code, w.Body.String())
	}
}

func TestHardenedStatusAndDisclosure(t *testing.T) {
	s := hardenedTestServer(tailscale.PeerIdentity{Admin: true}, nil)
	for _, principal := range []access.Principal{access.Viewer("anonymous-lan", false), access.Administrator("tailscale")} {
		r := hardenedRequest("GET", "/api/tailscale/status", "127.0.0.1:1234")
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, principal))
		st := s.tailscaleStatusFor(r, tailscale.Status{LoginURL: "secret", Hostname: "private", ServeError: "private error", DaemonRunning: true})
		if st.LoginURL != "" || !st.Redacted {
			t.Fatal("enrolment URL disclosed")
		}
		if !principal.Authenticated && (st.Hostname != "" || st.ServeError != "") {
			t.Fatal("anonymous status disclosed names/errors")
		}
		encoded, err := json.Marshal(s.reportResponse(r, &db.SiteReport{Filepath: "private/path", ZipFilepath: ptr("private/zip"), ZipFilename: ptr("source.zip"), Filename: "report.pdf"}))
		if err != nil || strings.Contains(string(encoded), "filepath") {
			t.Fatalf("report path disclosed: %s", encoded)
		}
		if !principal.Authenticated && strings.Contains(string(encoded), "zip_filename") {
			t.Fatal("source metadata disclosed")
		}
		encoded, _ = json.Marshal(s.siteResponse(r, &db.Site{Name: "site", Contact: "secret", Surveyor: "secret"}))
		if !principal.Authenticated && strings.Contains(string(encoded), "secret") {
			t.Fatalf("site contact disclosed: %s", encoded)
		}
	}
}

func TestHardenedAccessResponseAndNilAdapter(t *testing.T) {
	s := hardenedTestServer(tailscale.PeerIdentity{}, nil)
	w := httptest.NewRecorder()
	s.authWrapper(s.ServeMux()).ServeHTTP(w, hardenedRequest("GET", "/api/access", "192.168.1.50:1234"))
	var result struct {
		Profile string
		access.Principal
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Profile != "hardened" || result.Authenticated || len(result.Permissions) != 2 {
		t.Fatalf("unexpected access: %+v", result)
	}
	s.SetAuthGate(nil, EnforcementHardened)
	if s.authGate.mode != EnforcementHardened {
		t.Fatal("nil adapter disabled hardened profile")
	}
	w = httptest.NewRecorder()
	s.authWrapper(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("nil adapter permitted admin") })).ServeHTTP(w, hardenedRequest("POST", "/admin/radar/command", "100.64.0.5:1234"))
	if w.Code != 503 {
		t.Fatalf("got %d", w.Code)
	}
}

func ptr(value string) *string { return &value }

func TestHardenedBrowserAuthority(t *testing.T) {
	for _, tc := range []struct {
		host, origin, fetchSite string
		serve, want             bool
	}{
		{"100.64.0.2:8080", "http://100.64.0.2:8080", "same-origin", false, true},
		{"localhost:8080", "", "", false, true},
		{"evil.example", "http://evil.example", "", false, false},
		{"velocity.local", "http://velocity.local", "", false, false},
		{"device.tailnet.ts.net", "", "same-site", true, false},
		{"100.64.0.2", "null", "", false, false},
		{"100.64.0.2", "http://100.64.0.2/extra", "", false, false},
		{"device.tailnet.ts.net", "https://device.tailnet.ts.net", "same-origin", true, true},
		{"device.tailnet.ts.net", "http://device.tailnet.ts.net", "", true, false},
		{"device.tailnet.ts.net", "https://evil.example", "", true, false},
		{"device.tailnet.ts.net", "", "cross-site", true, false},
	} {
		r := hardenedRequest("POST", "/api/generate_report", "100.64.0.5:1234")
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if tc.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", tc.fetchSite)
		}
		if got := browserRequestAllowed(r, tc.serve); got != tc.want {
			t.Errorf("%+v: allowed=%v", tc, got)
		}
	}
	s := hardenedTestServer(tailscale.PeerIdentity{Admin: true}, nil)
	r := hardenedRequest("GET", "/api/config", "127.0.0.1:1234")
	r.Header.Add(serveCapabilitiesHeader, `{"velocity.report/cap/admin":[]}`)
	r.Header.Add(serveCapabilitiesHeader, `{"velocity.report/cap/view":[]}`)
	w := httptest.NewRecorder()
	s.hardenedWrapper(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("multiple claims reached handler") }), true).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("multiple claims HTTP %d", w.Code)
	}
}

func TestHardenedListenerWiringAndShutdown(t *testing.T) {
	s, database := setupTestServer(t)
	defer cleanupTestServer(t, database)
	s.SetAuthGate(&fakePeerAuth{}, EnforcementHardened)
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	backendAddress := reserved.Addr().String()
	reserved.Close()
	s.SetServeListen(backendAddress)
	address, cancel, done := startServerOnFreePort(t, s, false)
	defer cancel()
	for _, tc := range []struct {
		address, path, claims string
		want                  int
	}{
		{address, "/api/access", "", 200},
		{address, "/api/db_stats", "", 403},
		{address, "/api/db_stats", `{"velocity.report/cap/admin":[]}`, 403},
		{backendAddress, "/api/config", "", 403},
		{backendAddress, "/api/config", `{"velocity.report/cap/view":[]}`, 200},
		{backendAddress, "/api/db_stats", `{"velocity.report/cap/admin":[]}`, 200},
		{backendAddress, "/debug/backup", `{"velocity.report/cap/admin":[]}`, 403},
	} {
		r, err := http.NewRequest("GET", "http://"+tc.address+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.claims != "" {
			r.Header.Set(serveCapabilitiesHeader, tc.claims)
		}
		response, err := (&http.Client{Timeout: time.Second}).Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Errorf("%s%s HTTP %d want %d: %s", tc.address, tc.path, response.StatusCode, tc.want, body)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listeners did not shut down")
	}
	for _, target := range []string{address, backendAddress} {
		connection, err := net.DialTimeout("tcp", target, time.Second)
		if err == nil {
			connection.Close()
			t.Errorf("listener remained open: %s", target)
		}
	}
}

func TestHardenedStartupRefusesMissingAdapterAndPublicBackend(t *testing.T) {
	for _, tc := range []struct {
		adapter PeerAuthClient
		backend string
	}{
		{nil, "127.0.0.1:0"}, {&fakePeerAuth{}, "0.0.0.0:0"},
	} {
		s := &Server{}
		s.SetAuthGate(tc.adapter, EnforcementHardened)
		s.SetServeListen(tc.backend)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.startWithListener(context.Background(), listener, true); err == nil {
			t.Fatal("invalid hardened startup succeeded")
		}
	}
}

func TestHardenedMalformedCapabilityEncoding(t *testing.T) {
	for _, claims := range []string{`null`, `{"broken`, `{"a":[], invalid}`, `{"a":[]`, `=?utf-8?b?!!!!?=`} {
		r := hardenedRequest("GET", "/api/config", "127.0.0.1:1234")
		r.Header.Set(serveCapabilitiesHeader, claims)
		if _, err := parseServeCapabilities(r); err == nil {
			t.Errorf("malformed claim accepted: %q", claims)
		}
	}
}

func TestHardenedRemainingRouteMatrix(t *testing.T) {
	s := hardenedTestServer(tailscale.PeerIdentity{Admin: true}, nil)
	s.RegisterOperation("GET /api/protected/", access.ExportData, "")
	s.RegisterOperation("GET /api/protected/secret/", access.Maintenance, "")
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/api/site_config_periods", 200},
		{"GET", "/api/transit_worker", 200},
		{"POST", "/api/transit_worker", 200},
		{"PUT", "/api/serial/configs/1", 200},
		{"GET", "/api/reports/site/1", 200},
		{"GET", "/api/reports/1", 200},
		{"GET", "/api/sites/../config", 403},
		{"GET", "/api/protected/source", 200},
		{"HEAD", "/api/protected/source", 200},
		{"POST", "/api/protected/source", 403},
		{"GET", "/api/protected/secret/source", 403},
	} {
		w := httptest.NewRecorder()
		s.authWrapper(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, hardenedRequest(tc.method, tc.path, "100.64.0.5:1234"))
		if w.Code != tc.want {
			t.Errorf("%s %s: HTTP %d want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}

func TestCallerAccessCompatibilityAndMissingContext(t *testing.T) {
	for _, tc := range []struct {
		mode                    CapEnforcement
		source                  string
		identity                tailscale.PeerIdentity
		err                     error
		wantStatus, permissions int
	}{
		{EnforcementOff, "192.168.1.5:1234", tailscale.PeerIdentity{}, nil, 200, 8},
		{EnforcementOn, "192.168.1.5:1234", tailscale.PeerIdentity{}, nil, 200, 8},
		{EnforcementOn, "100.64.0.5:1234", tailscale.PeerIdentity{Admin: true}, nil, 200, 8},
		{EnforcementOn, "100.64.0.5:1234", tailscale.PeerIdentity{View: true}, nil, 200, 2},
		{EnforcementOn, "100.64.0.5:1234", tailscale.PeerIdentity{}, nil, 200, 0},
		{EnforcementOn, "100.64.0.5:1234", tailscale.PeerIdentity{}, errors.New("offline"), 503, 0},
		{EnforcementHardened, "127.0.0.1:1234", tailscale.PeerIdentity{}, nil, 503, 0},
	} {
		s := &Server{}
		s.SetAuthGate(&fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) { return tc.identity, tc.err }}, tc.mode)
		w := httptest.NewRecorder()
		s.handleAccess(w, hardenedRequest("GET", "/api/access", tc.source))
		if w.Code != tc.wantStatus {
			t.Fatalf("%+v: HTTP %d", tc, w.Code)
		}
		if w.Code == 200 {
			var body struct{ access.Principal }
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Permissions) != tc.permissions {
				t.Errorf("%+v: permissions %+v", tc, body.Permissions)
			}
		}
	}
	w := httptest.NewRecorder()
	(&Server{}).handleAccess(w, hardenedRequest("POST", "/api/access", "127.0.0.1:1234"))
	if w.Code != 405 {
		t.Fatal("access endpoint allowed mutation")
	}
}

func TestHardenedReportListDisclosure(t *testing.T) {
	s := hardenedTestServer(tailscale.PeerIdentity{}, nil)
	r := hardenedRequest("GET", "/api/reports/", "192.168.1.5:1234")
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, access.Viewer("anonymous-lan", false)))
	reports := []db.SiteReport{{ID: 1, Filename: "public.pdf", Filepath: "private/path", ZipFilename: ptr("private.zip"), ZipFilepath: ptr("private/zip")}}
	encoded, err := json.Marshal(s.reportListResponse(r, reports))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "public.pdf") || strings.Contains(string(encoded), "private") {
		t.Fatalf("report list disclosure: %s", encoded)
	}
}

func TestExplicitAliasPermissionsOverrideCollectionAndAssetPolicy(t *testing.T) {
	s := hardenedTestServer(tailscale.PeerIdentity{Admin: true}, nil)
	mux := s.ServeMux()
	mux.HandleFunc("GET /api/sites/private/{item}", func(http.ResponseWriter, *http.Request) { t.Fatal("maintenance alias admitted") })
	s.RegisterOperation("GET /api/sites/private/{item}", access.Maintenance, "")
	mux.HandleFunc("/docs/control", func(http.ResponseWriter, *http.Request) { t.Fatal("unregistered asset alias admitted") })
	for _, target := range []string{"/api/sites/private/item", "/docs/control"} {
		w := httptest.NewRecorder()
		s.authWrapper(mux).ServeHTTP(w, hardenedRequest("GET", target, "100.64.0.5:1234"))
		if w.Code != 403 {
			t.Fatalf("alias %s: HTTP %d", target, w.Code)
		}
	}
}
