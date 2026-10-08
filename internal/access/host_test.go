package access

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostPolicyAllows(t *testing.T) {
	p, err := NewHostPolicy([]string{"velocity.example.com", ".sensors.example.org", " ", "Proxy.Example.NET."})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"", true},
		{"192.168.1.10", true},
		{"192.168.1.10:80", true},
		{"100.100.1.2:8080", true},
		{"[::1]:8080", true},
		{"[fe80::1%25eth0]:80", true},
		{"localhost", true},
		{"LOCALHOST:5173", true},
		{"app.localhost", true},
		{"velocity", true},
		{"velocity:80", true},
		{"velocity.local", true},
		{"velocity.local.", true},
		{"velocity.lan", true},
		{"velocity.home", true},
		{"velocity.home.arpa", true},
		{"velocity.internal", true},
		{"velocity.localdomain", true},
		{"velocity.tail1234.ts.net", true},
		{"velocity.example.com", true},
		{"VELOCITY.EXAMPLE.COM:443", true},
		{"pi.sensors.example.org", true},
		{"proxy.example.net", true},
		// A rebinding page's own name, and look-alikes of the allowed ones.
		{"attacker.example", false},
		{"rebind.attacker.example:80", false},
		{"velocity.local.attacker.example", false},
		{"sensors.example.org", false},
		{"other.example.com", false},
		{"ts.net.attacker.example", false},
		{"bad host", false},
		{"-bad.local", false},
		{"a..local", false},
		{strings.Repeat("a", 64) + ".local", false},
	} {
		if got := p.Allows(tc.host); got != tc.want {
			t.Errorf("Allows(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
	if DefaultHostPolicy().Allows("velocity.example.com") {
		t.Error("the default policy answered an operator's name")
	}
}

func TestHostPolicyRefusesEntriesThatAreNotNames(t *testing.T) {
	for _, entry := range []string{"https://velocity.example.com", "velocity.example.com:8080", "velocity.example.com/app", "::1", "*.example.com", strings.Repeat("a.", 127) + "com"} {
		if _, err := NewHostPolicy([]string{entry}); err == nil {
			t.Errorf("accepted %q", entry)
		}
	}
	p, err := ParseAllowedHosts("a.example.com, .b.example.com,,")
	if err != nil || !p.Allows("a.example.com") || !p.Allows("x.b.example.com") {
		t.Fatalf("ParseAllowedHosts: %v", err)
	}
	if p, err := ParseAllowedHosts(""); err != nil || !p.Allows("velocity.local") {
		t.Fatalf("empty value: %v", err)
	}
}

func TestHostPolicyHandler(t *testing.T) {
	p := DefaultHostPolicy()
	served := 0
	h := p.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served++ }))
	for _, tc := range []struct {
		host string
		code int
	}{
		{"velocity.local", 200},
		{"rebind.attacker.example", http.StatusForbidden},
		{"rebind.attacker.example", http.StatusForbidden},
	} {
		r := httptest.NewRequest("POST", "/api/tailscale/enable", nil)
		r.Host = tc.host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != tc.code {
			t.Fatalf("%s: HTTP %d, want %d", tc.host, rec.Code, tc.code)
		}
		if tc.code == http.StatusForbidden && !strings.Contains(rec.Body.String(), "host_not_allowed") {
			t.Fatalf("%s: body %q", tc.host, rec.Body.String())
		}
	}
	if served != 1 {
		t.Fatalf("served %d requests, want 1", served)
	}
}
