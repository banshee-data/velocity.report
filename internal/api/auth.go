// Capability-grant authorization for the HTTP API.
//
// Tailscale application capability grants
// (https://tailscale.com/kb/1324/grants-app-capabilities) are used
// to split the API surface between read-only and admin clients.
// The middleware is applied to the entire mux at Start() time with
// a small allowlist of unauthenticated paths, so adding a new route
// later does not silently bypass the gate.  See server.go for the
// allowlist and how the wrapper is composed.
//
// Trust model
//
// 1. Source classification.  A request is "from the tailnet" only
//    when it actually arrived through tailscale serve, which means
//    the upstream connection lands on loopback and the original
//    peer IP is in X-Forwarded-For.  Anywhere else (LAN, direct hit
//    on :8080, host-network requests from velocity-ctl) is treated
//    as admin: those paths require already being on a network the
//    operator controls, which is the same trust boundary the device
//    has had since it shipped.
//
//    XFF is *only* trusted when r.RemoteAddr is loopback, otherwise
//    a LAN attacker who can reach :8080 directly could forge a
//    tailnet identity.
//
// 2. Authorization on the tailnet.  Once classified as tailnet, the
//    daemon is asked for the peer's grants via the local API.  Any
//    one of velocity.report/cap/{view,admin} authorises the
//    corresponding access level; admin implies view.
//
// 3. Failure modes.  A definitive "no such peer" from the daemon
//    is fail-closed (the tailnet identity is unknown — no caps).
//    A transient lookup error (socket down, timeout) is fail-open
//    so that a tailscaled hiccup does not lock every authorised
//    user out simultaneously.  A daemon outage is logged at every
//    failure for diagnostic purposes.
//
// 4. Modes.  Off disables the entire mechanism — tailnet peers
//    behave like LAN peers (admin).  On enforces caps for tailnet
//    peers.  Lockout safety is handled by the LAN bypass: an
//    operator with a misconfigured ACL drops back to LAN access,
//    fixes the grants, and tries again.  Operators should leave
//    the gate off until they have confirmed grants on a test peer.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

// CapEnforcement selects whether the auth middleware enforces
// capability grants for tailnet-sourced requests.
type CapEnforcement int

const (
	// EnforcementOff disables capability checks entirely; every
	// request is treated as admin.  This is the default for
	// existing deployments that have not configured grants.
	EnforcementOff CapEnforcement = iota
	// EnforcementOn enforces capability checks for tailnet-sourced
	// requests.  LAN/loopback requests still pass as admin.
	EnforcementOn
)

// ParseEnforcement maps "off"/"on" (and a few common synonyms) to
// a CapEnforcement value.  Empty string is treated as "off" for
// backwards compatibility with deployments that had no flag.
func ParseEnforcement(s string) (CapEnforcement, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "false", "0", "no":
		return EnforcementOff, nil
	case "on", "true", "1", "yes":
		return EnforcementOn, nil
	}
	return EnforcementOff, errors.New("invalid cap enforcement (want off|on)")
}

// CapKind is the access level a route requires.
type CapKind int

const (
	// CapView is sufficient for read-only endpoints.  Holders of
	// CapAdmin implicitly satisfy CapView.
	CapView CapKind = iota
	// CapAdmin is required for any state-mutating endpoint.  Used
	// as the default for routes that do not opt in explicitly.
	CapAdmin
)

// PeerAuthClient is the slice of the Tailscale manager that the
// auth middleware depends on.  Defined here so tests can substitute
// a stub without standing up a real tailscale.Manager.
type PeerAuthClient interface {
	// LookupPeer resolves a remote address to a PeerIdentity.
	// Returns tailscale.ErrPeerNotFound when the daemon is
	// authoritative that the address is not a tailnet peer; any
	// other error is treated as transient.
	LookupPeer(ctx context.Context, remoteAddr string) (tailscale.PeerIdentity, error)
	// LocalTailnetPrefixes returns the tailnet IPs assigned to this
	// node, used as a belt-and-braces fallback when the CGNAT-range
	// shortcut does not match (e.g. custom CGNAT ranges).  May
	// return nil when the daemon is unreachable.
	LocalTailnetPrefixes(ctx context.Context) []netip.Prefix
}

// authGate carries the per-server configuration for the middleware.
// It is stateless beyond the configuration: there is no "armed"
// flag, no run-time mode flip — restart is the only way to change
// modes, which keeps the behaviour easy to reason about in incident
// response.
type authGate struct {
	tc      PeerAuthClient
	mode    CapEnforcement
	timeout time.Duration
}

// newAuthGate constructs an authGate.  A nil PeerAuthClient is
// equivalent to EnforcementOff: every request is admin, no daemon
// calls are made.
func newAuthGate(tc PeerAuthClient, mode CapEnforcement) *authGate {
	if tc == nil {
		mode = EnforcementOff
	}
	return &authGate{tc: tc, mode: mode, timeout: 750 * time.Millisecond}
}

// requireCap returns middleware that enforces required for any
// request the gate classifies as tailnet-sourced.  See the package
// comment for the trust-model semantics.
func (g *authGate) requireCap(required CapKind, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.mode == EnforcementOff || g.tc == nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), g.timeout)
		defer cancel()

		clientIP, fromTailnet := classifySource(r, g, ctx)
		if !fromTailnet {
			// LAN/loopback peers retain full access — the LAN
			// itself is the trust boundary in those deployments.
			next.ServeHTTP(w, r)
			return
		}

		id, err := g.tc.LookupPeer(ctx, clientIP.String())
		switch {
		case err == nil:
			// fall through to grant check
		case errors.Is(err, tailscale.ErrPeerNotFound):
			// Authoritative "not a tailnet peer."  We classified
			// the address as tailnet-sourced, but the daemon
			// disagrees; fail closed.
			log.Printf("auth: tailnet-classified IP %s has no tailnet identity", clientIP)
			writeForbidden(w, "unknown_peer", required)
			return
		default:
			// Transient lookup failure.  Fail open so a tailscaled
			// blip does not deny every authorised user at once.
			log.Printf("auth: lookup %s transient error, allowing: %v", clientIP, err)
			next.ServeHTTP(w, r)
			return
		}

		ok := false
		switch required {
		case CapView:
			ok = id.View // PeerIdentity already collapses Admin into View
		case CapAdmin:
			ok = id.Admin
		}
		if !ok {
			writeForbidden(w, "missing_cap", required)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// classifySource returns the originating client IP and whether it
// arrived over the tailnet.  The XFF header is consulted only when
// r.RemoteAddr is loopback, i.e. the request came from tailscale
// serve's local proxy.  Anywhere else, only r.RemoteAddr is
// trusted, which prevents a LAN attacker who can reach :8080
// directly from forging a tailnet identity by setting XFF.
func classifySource(r *http.Request, g *authGate, ctx context.Context) (netip.Addr, bool) {
	remoteIP := remoteAddrIP(r)
	if !remoteIP.IsValid() {
		return netip.Addr{}, false
	}
	// Trust XFF only when the upstream is loopback (the only path
	// by which tailscale serve forwards requests to us).
	if remoteIP.IsLoopback() {
		if xff := firstXFF(r.Header.Get("X-Forwarded-For")); xff.IsValid() {
			if isTailnetIP(ctx, g, xff) {
				return xff, true
			}
			// XFF is set but not a tailnet IP: a misconfigured
			// reverse proxy or a client setting their own header
			// before us.  Treat as non-tailnet.
			return xff, false
		}
		// Loopback with no XFF: a process on the host (the Go
		// server itself, velocity-ctl, a local curl).  Non-tailnet,
		// hence admin.
		return remoteIP, false
	}
	// Direct connection (LAN, or :8080 exposed somewhere).  XFF is
	// untrusted here.  We still classify by the on-wire source: a
	// direct connection from a tailnet IP (rare but possible if the
	// operator explicitly bound the tailnet IP) is treated as
	// tailnet, but the much more common case is a LAN address.
	return remoteIP, isTailnetIP(ctx, g, remoteIP)
}

// firstXFF parses the first entry of an X-Forwarded-For header.
func firstXFF(xff string) netip.Addr {
	if xff == "" {
		return netip.Addr{}
	}
	first, _, _ := strings.Cut(xff, ",")
	ip, err := netip.ParseAddr(strings.TrimSpace(first))
	if err != nil {
		return netip.Addr{}
	}
	return ip
}

// remoteAddrIP returns the parsed IP of r.RemoteAddr, or an invalid
// addr if it cannot be parsed.
func remoteAddrIP(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip
	}
	return netip.Addr{}
}

// isTailnetIP reports whether ip is in Tailscale's CGNAT range
// (the static /10 for IPv4 and /48 for IPv6) or in one of the
// prefixes the local daemon reports as the node's own (a fallback
// for unusual deployments).  Cached at the daemon-call layer.
func isTailnetIP(ctx context.Context, g *authGate, ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	if isTailscaleCGNAT(ip) {
		return true
	}
	for _, p := range g.tc.LocalTailnetPrefixes(ctx) {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Tailscale's CGNAT range is fixed (RFC 6598 100.64/10 carved out
// for Tailscale, and fd7a:115c:a1e0::/48 for IPv6).
var (
	tailscale4 = netip.MustParsePrefix("100.64.0.0/10")
	tailscale6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")
)

func isTailscaleCGNAT(ip netip.Addr) bool {
	if ip.Is4() || ip.Is4In6() {
		return tailscale4.Contains(ip.Unmap())
	}
	return tailscale6.Contains(ip)
}

// forbiddenBody is the JSON body returned on 403 so the web UI can
// distinguish failure modes ("you lack the cap" vs "we couldn't
// resolve your tailnet identity").  Keep field names stable.
type forbiddenBody struct {
	Error    string `json:"error"`
	Required string `json:"required,omitempty"`
}

func writeForbidden(w http.ResponseWriter, code string, required CapKind) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	body := forbiddenBody{Error: code}
	switch required {
	case CapView:
		body.Required = "view"
	case CapAdmin:
		body.Required = "admin"
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("auth: encode forbidden body: %v", err)
	}
}
