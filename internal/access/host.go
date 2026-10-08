package access

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// privateNameSuffixes are DNS suffixes no public resolver answers for: mDNS,
// Tailscale's MagicDNS, and the names home routers and RFC 8375 reserve.
// A DNS-rebinding page needs a name whose answers its author controls, so it
// can use none of them.
var privateNameSuffixes = []string{
	".local", ".localhost", ".lan", ".home", ".home.arpa", ".internal", ".localdomain", ".ts.net",
}

// HostPolicy decides which Host headers a listener answers.
//
// A DNS-rebinding page is served from a name its author controls, then
// re-resolves that name to the device, so its requests are same-origin and
// carry no Origin or Sec-Fetch-Site a server could refuse.  Only the Host
// header still names the author's domain.  The policy answers an address
// literal, localhost, a single-label name, a name under a private suffix, or
// one the operator lists, and refuses anything else.
type HostPolicy struct {
	exact  map[string]bool
	suffix []string

	logMu  sync.Mutex
	logged time.Time
}

// NewHostPolicy returns the default policy plus the operator's names.  An
// entry is a host name, or one starting with "." that matches every name
// beneath it; one with a scheme, port or path is refused.
func NewHostPolicy(extra []string) (*HostPolicy, error) {
	p := &HostPolicy{exact: map[string]bool{}}
	for _, raw := range extra {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if name == "" {
			continue
		}
		bare := strings.TrimPrefix(name, ".")
		if !validHostName(bare) {
			return nil, fmt.Errorf("allowed host %q is not a host name (give names only, without scheme, port or path)", raw)
		}
		if strings.HasPrefix(name, ".") {
			p.suffix = append(p.suffix, name)
		} else {
			p.exact[name] = true
		}
	}
	return p, nil
}

// DefaultHostPolicy answers only the built-in hosts.
func DefaultHostPolicy() *HostPolicy {
	p, _ := NewHostPolicy(nil)
	return p
}

// ParseAllowedHosts splits a comma-separated --allowed-hosts value.
func ParseAllowedHosts(value string) (*HostPolicy, error) {
	return NewHostPolicy(strings.Split(value, ","))
}

// Allows reports whether a request's Host header names this device in a way
// a rebinding page cannot.  An empty Host (HTTP/1.0) comes from no browser.
func (p *HostPolicy) Allows(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if !validHostName(host) {
		return false
	}
	if host == "localhost" || !strings.Contains(host, ".") || p.exact[host] {
		return true
	}
	for _, s := range privateNameSuffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	for _, s := range p.suffix {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}

// Handler refuses, with 403 {"error":"host_not_allowed"}, a request whose
// Host the policy does not answer.
func (p *HostPolicy) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.Allows(r.Host) {
			p.refuse(r.Host)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(struct {
				Error string `json:"error"`
			}{"host_not_allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *HostPolicy) refuse(host string) {
	p.logMu.Lock()
	defer p.logMu.Unlock()
	if now := time.Now(); now.Sub(p.logged) >= guardLogInterval {
		p.logged = now
		log.Printf("access: refusing requests for host %q, which may be a DNS-rebinding page; add it with --allowed-hosts if it names this device", host)
	}
}

// validHostName accepts LDH labels separated by dots, plus underscores, which
// some local resolvers hand out.
func validHostName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}
