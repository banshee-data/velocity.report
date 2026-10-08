package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/access"
)

// hostRequest sends method path to address with the given Host header and
// reports the status and the refusal reason, if any.
func hostRequest(t *testing.T, address, method, path, host string, header map[string]string) (int, string) {
	t.Helper()
	r, err := http.NewRequest(method, "http://"+address+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = host
	for k, v := range header {
		r.Header.Set(k, v)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body forbiddenBody
	_ = json.NewDecoder(response.Body).Decode(&body)
	return response.StatusCode, body.Error
}

// A DNS-rebinding page re-resolves its own name to the device, so its
// requests reach the listeners same-origin, carrying the page's name as Host.
// Every profile refuses that before any route runs: it could otherwise enrol
// the device into the page author's tailnet (off and on admit the LAN as
// admin) or read what hardened shows anonymous callers.
func TestEveryProfileRefusesADNSRebindingHost(t *testing.T) {
	const rebinding = "rebind.attacker.example"
	for name, mode := range map[string]CapEnforcement{"off": EnforcementOff, "on": EnforcementOn, "hardened": EnforcementHardened} {
		t.Run(name, func(t *testing.T) {
			s, database := setupTestServer(t)
			defer cleanupTestServer(t, database)
			s.SetTailscaleController(&stubTailscale{status: matrixStatus})
			var backend string
			if mode == EnforcementHardened {
				s.SetAuthGate(&fakePeerAuth{}, mode)
				backend = reserveAddress(t)
				s.SetServeListen(backend)
			} else if mode == EnforcementOn {
				s.SetAuthGate(&fakePeerAuth{}, mode)
			}
			policy, err := access.NewHostPolicy([]string{"velocity.example.com"})
			if err != nil {
				t.Fatal(err)
			}
			s.SetHostPolicy(policy)
			address, cancel, done := startServerOnFreePort(t, s, false)
			defer func() {
				cancel()
				<-done
			}()

			for _, path := range []string{"/api/tailscale/status", "/api/version"} {
				if code, reason := hostRequest(t, address, "GET", path, rebinding, nil); code != http.StatusForbidden || reason != "host_not_allowed" {
					t.Fatalf("GET %s for %s: HTTP %d %q", path, rebinding, code, reason)
				}
			}
			if code, reason := hostRequest(t, address, "POST", "/api/tailscale/enable", rebinding+":80", nil); code != http.StatusForbidden || reason != "host_not_allowed" {
				t.Fatalf("POST enable for %s: HTTP %d %q", rebinding, code, reason)
			}
			for _, host := range []string{"velocity.local", "192.168.1.10", "velocity.example.com"} {
				if code, reason := hostRequest(t, address, "GET", "/api/tailscale/status", host, nil); code != http.StatusOK {
					t.Fatalf("GET status for %s: HTTP %d %q", host, code, reason)
				}
			}
			if backend != "" {
				admin := map[string]string{serveCapabilitiesHeader: `{"velocity.report/cap/admin":[]}`}
				if code, reason := hostRequest(t, backend, "GET", "/api/version", rebinding, admin); code != http.StatusForbidden || reason != "host_not_allowed" {
					t.Fatalf("Serve backend for %s: HTTP %d %q", rebinding, code, reason)
				}
				if code, _ := hostRequest(t, backend, "GET", "/api/version", "velocity.tail1234.ts.net", admin); code != http.StatusOK {
					t.Fatalf("Serve backend for the MagicDNS name: HTTP %d", code)
				}
			}
		})
	}
}

// Without SetHostPolicy the listeners still refuse a rebinding Host.
func TestTheDefaultHostPolicyApplies(t *testing.T) {
	s, database := setupTestServer(t)
	defer cleanupTestServer(t, database)
	address, cancel, done := startServerOnFreePort(t, s, false)
	defer func() {
		cancel()
		<-done
	}()
	if code, reason := hostRequest(t, address, "GET", "/api/version", "rebind.attacker.example", nil); code != http.StatusForbidden || reason != "host_not_allowed" {
		t.Fatalf("HTTP %d %q", code, reason)
	}
	if code, _ := hostRequest(t, address, "GET", "/api/version", "velocity.local", nil); code != http.StatusOK {
		t.Fatalf("velocity.local: HTTP %d", code)
	}
}
