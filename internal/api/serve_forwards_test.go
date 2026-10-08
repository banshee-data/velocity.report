package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

// serveForwardsStub stands in for tailscaled's Serve configuration.
type serveForwardsStub struct {
	mu    sync.Mutex
	ports map[uint16]bool
}

func (s *serveForwardsStub) UnmanagedServePorts(context.Context) (map[uint16]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ports, nil
}

func (s *serveForwardsStub) forward(addresses ...string) {
	ports := map[uint16]bool{}
	for _, a := range addresses {
		_, port, _ := net.SplitHostPort(a)
		p, _ := strconv.Atoi(port)
		ports[uint16(p)] = true
	}
	s.mu.Lock()
	s.ports = ports
	s.mu.Unlock()
}

// fetch reports the HTTP status, or 0 when the connection was dropped.
func fetch(t *testing.T, address, path string, header map[string]string) int {
	t.Helper()
	r, err := http.NewRequest("GET", "http://"+address+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		r.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Do(r)
	if err != nil {
		return 0
	}
	response.Body.Close()
	return response.StatusCode
}

func reserveAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// A Serve TCP forward reaches a listener from tailscaled on this host with
// whatever headers its client wrote.  Hardened mode must not then believe a
// capability claim on the Serve backend, nor give the main listener's local
// callers anything.
func TestHardenedListenersRefuseLocalConnectionsUnderAnUnmanagedForward(t *testing.T) {
	s, database := setupTestServer(t)
	defer cleanupTestServer(t, database)
	s.SetAuthGate(&fakePeerAuth{}, EnforcementHardened)
	backend := reserveAddress(t)
	s.SetServeListen(backend)
	forwards := &serveForwardsStub{}
	s.SetServeForwards(forwards)
	address, cancel, done := startServerOnFreePort(t, s, false)
	defer func() {
		cancel()
		<-done
	}()
	admin := map[string]string{serveCapabilitiesHeader: `{"velocity.report/cap/admin":[]}`}

	if got := fetch(t, backend, "/api/db_stats", admin); got != http.StatusOK {
		t.Fatalf("managed Serve only: backend HTTP %d, want 200", got)
	}
	forwards.forward(backend)
	if got := fetch(t, backend, "/api/db_stats", admin); got != 0 {
		t.Fatalf("forward to the backend: a forged admin claim got HTTP %d", got)
	}
	if got := fetch(t, address, "/api/access", nil); got != http.StatusOK {
		t.Fatalf("forward to the backend only: main listener HTTP %d, want 200", got)
	}
	forwards.forward(address)
	if got := fetch(t, address, "/api/access", nil); got != 0 {
		t.Fatalf("forward to the main listener: local caller got HTTP %d", got)
	}
	if got := fetch(t, backend, "/api/db_stats", admin); got != http.StatusOK {
		t.Fatalf("forward moved away: backend HTTP %d, want 200", got)
	}
}

// In on mode a loopback connection without X-Forwarded-For is the host, and
// with it is a tailnet peer whose grants are looked up.  A TCP forward lets
// its client write either, so neither is believed while one targets the port.
func TestOnModeListenerRefusesLocalConnectionsUnderAnUnmanagedForward(t *testing.T) {
	s, database := setupTestServer(t)
	defer cleanupTestServer(t, database)
	admin := &fakePeerAuth{lookup: func(string) (tailscale.PeerIdentity, error) {
		return tailscale.PeerIdentity{Admin: true, View: true}, nil
	}}
	s.SetAuthGate(admin, EnforcementOn)
	forwards := &serveForwardsStub{}
	s.SetServeForwards(forwards)
	address, cancel, done := startServerOnFreePort(t, s, false)
	defer func() {
		cancel()
		<-done
	}()
	forged := map[string]string{"X-Forwarded-For": "100.100.1.2"}

	if got := fetch(t, address, "/api/db_stats", nil); got != http.StatusOK {
		t.Fatalf("no forward: host HTTP %d, want 200", got)
	}
	forwards.forward(address)
	for _, header := range []map[string]string{nil, forged} {
		if got := fetch(t, address, "/api/db_stats", header); got != 0 {
			t.Fatalf("forward to the listener (%v): HTTP %d, want the connection dropped", header, got)
		}
	}
}

// Off mode trusts every caller, so it reads nothing from tailscaled.
func TestOffModeListenerIsNotGuarded(t *testing.T) {
	s, database := setupTestServer(t)
	defer cleanupTestServer(t, database)
	s.SetAuthGate(nil, EnforcementOff)
	forwards := &serveForwardsStub{}
	s.SetServeForwards(forwards)
	address, cancel, done := startServerOnFreePort(t, s, false)
	defer func() {
		cancel()
		<-done
	}()
	forwards.forward(address)
	if got := fetch(t, address, "/api/db_stats", nil); got != http.StatusOK {
		t.Fatalf("off mode: HTTP %d, want 200", got)
	}
}

// A kept-alive connection admitted before a forward is noticed keeps no
// trust: each request on it is checked again.
func TestGuardedListenerRechecksRequestsOnAKeptAliveConnection(t *testing.T) {
	s, database := setupTestServer(t)
	defer cleanupTestServer(t, database)
	s.SetAuthGate(&fakePeerAuth{}, EnforcementOn)
	forwards := &serveForwardsStub{}
	s.SetServeForwards(forwards)
	address, cancel, done := startServerOnFreePort(t, s, false)
	defer func() {
		cancel()
		<-done
	}()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 1}}
	get := func() (int, string) {
		response, err := client.Get("http://" + address + "/api/db_stats")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var body forbiddenBody
		_ = json.NewDecoder(response.Body).Decode(&body)
		return response.StatusCode, body.Error
	}
	if code, _ := get(); code != http.StatusOK {
		t.Fatalf("no forward: HTTP %d", code)
	}
	forwards.forward(address)
	if code, reason := get(); code != http.StatusForbidden || reason != "unmanaged_serve_forward" {
		t.Fatalf("forward added: HTTP %d %q, want 403 unmanaged_serve_forward", code, reason)
	}
}
