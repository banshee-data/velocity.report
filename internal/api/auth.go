// Capability-grant authorisation for the HTTP API.
//
// Off preserves existing deployments. On enforces Tailscale view/admin grants
// while retaining the historical LAN and loopback bypass. Hardened instead
// grants anonymous LAN aggregate/PDF reading, resolves direct peers through
// WhoIs, and accepts per-request Serve capabilities only on a dedicated
// loopback backend. Named operation permissions cover the complete mux.
//
// Unknown peers are refused (403), unavailable lookups fail closed (503),
// and successful identities are cached for at most five seconds. There is
// no stale-grant fallback. Hardened mode never silently becomes off and never
// treats ordinary HTTP loopback traffic as OS-authorised maintenance.
// Bootstrap and recovery use the OS-controlled Tailscale CLI and service flags.

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
	// EnforcementHardened applies operation permissions to every request, with
	// anonymous LAN reading and a dedicated Serve backend. HTTP has no OS-admin bypass.
	EnforcementHardened
)

// ParseEnforcement maps "off"/"on"/"hardened" (and compatibility synonyms) to
// a CapEnforcement value.  Empty string is treated as "off" for
// backwards compatibility with deployments that had no flag.
func ParseEnforcement(s string) (CapEnforcement, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "false", "0", "no":
		return EnforcementOff, nil
	case "on", "true", "1", "yes":
		return EnforcementOn, nil
	case "hardened":
		return EnforcementHardened, nil
	}
	return EnforcementOff, errors.New("invalid cap enforcement (want off|on|hardened)")
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

// capUngated marks a refusal on a route that requires no cap; the refusal
// body then names none.  Outside the iota block, so CapView stays zero.
const capUngated CapKind = -1

// PeerAuthClient is the slice of the Tailscale manager that the
// auth middleware depends on.  Defined here so tests can substitute
// a stub without standing up a real tailscale.Manager.
type PeerAuthClient interface {
	// LookupPeer resolves a remote address to a PeerIdentity.
	// Returns tailscale.ErrPeerNotFound when the daemon is
	// authoritative that the address is not a tailnet peer; any
	// other error is treated as transient.
	LookupPeer(ctx context.Context, remoteAddr string) (tailscale.PeerIdentity, error)
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
	if tc == nil && mode != EnforcementHardened {
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

		if g.refusesOutsider(w, r, required) {
			return
		}
		clientIP, source := classifySource(r)
		if source == sourceLocal {
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
			// Transient lookup failure with no recent identity to
			// stand in: the peer is unresolved, so it gets nothing.
			log.Printf("auth: lookup %s failed, refusing: %v", clientIP, err)
			writeUnavailable(w)
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
			// The audit trail for a refused grant: the peer's tailnet
			// address and what it lacked, nothing about the request body.
			log.Printf("auth: refusing %s %s to tailnet peer %s: no %s grant", r.Method, r.URL.Path, clientIP, capName(required))
			writeForbidden(w, "missing_cap", required)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// funnelRequestHeader is set by tailscale serve on requests it
// accepted through Funnel, from the public internet.  serve strips any
// copy a client sends.
const funnelRequestHeader = "Tailscale-Funnel-Request"

// requestSource is where classifySource places a request.
type requestSource int

const (
	// sourceLocal is the host or the LAN: admin.
	sourceLocal requestSource = iota
	// sourceTailnet is a tailnet peer, directly or through serve:
	// subject to capability grants.
	sourceTailnet
	// sourceForwarded is a loopback proxy forwarding for an address
	// outside the tailnet, such as a Funnel client: refused.
	sourceForwarded
)

// refusesOutsider answers 403 for a request from outside both the tailnet
// and the host's networks, and reports whether it did: a request tailscale
// serve accepted through Funnel, or one a loopback proxy forwarded for a
// non-tailnet address.  The server's wrapper applies it to every route,
// the ungated ones included, before route classification.
func (g *authGate) refusesOutsider(w http.ResponseWriter, r *http.Request, required CapKind) bool {
	if g.mode == EnforcementOff || g.tc == nil {
		return false
	}
	if r.Header.Get(funnelRequestHeader) != "" {
		// tailscale serve marks requests it accepted from the
		// public internet; nobody there holds a grant.
		writeForbidden(w, "funnel_request", required)
		return true
	}
	if clientIP, source := classifySource(r); source == sourceForwarded {
		log.Printf("auth: refusing a request forwarded for non-tailnet address %s", clientIP)
		writeForbidden(w, "untrusted_forward", required)
		return true
	}
	return false
}

// classifySource returns the originating client IP and where the
// request came from.  The XFF header is consulted only when
// r.RemoteAddr is loopback, i.e. the request came from tailscale
// serve's local proxy.  Anywhere else, only r.RemoteAddr is
// trusted, which prevents a LAN attacker who can reach the server
// directly from forging a tailnet identity by setting XFF.
func classifySource(r *http.Request) (netip.Addr, requestSource) {
	remoteIP := remoteAddrIP(r)
	if !remoteIP.IsValid() {
		return netip.Addr{}, sourceLocal
	}
	// Trust XFF only when the upstream is loopback (the only path
	// by which tailscale serve forwards requests to us).
	if remoteIP.IsLoopback() {
		if raw := r.Header.Get("X-Forwarded-For"); raw != "" {
			xff := firstXFF(raw)
			if isTailnetIP(xff) {
				return xff, sourceTailnet
			}
			// XFF is set but is not a tailnet IP: tailscale serve
			// forwarding a Funnel request from the internet, or a
			// local reverse proxy forwarding for someone else.  One
			// that does not parse is not tailscale serve's, which
			// sets a single address: whoever wrote it is unknown.
			// None of these is the host or the LAN.
			return xff, sourceForwarded
		}
		// Loopback with no XFF: a process on the host (the Go
		// server itself, `velocity device`, a local curl).  The
		// listener refuses connections from this host while
		// tailscaled forwards to it through a Serve handler the
		// manager did not install (access.GuardListener), since such
		// a forward arrives here too, with no XFF or a forged one.
		return remoteIP, sourceLocal
	}
	// Direct connection (LAN, or the listener exposed somewhere).  XFF
	// is untrusted here.  We still classify by the on-wire source: a
	// direct connection from a tailnet IP (the listener bound to all
	// interfaces, reached at the node's tailnet address) is treated as
	// tailnet, but the much more common case is a LAN address.
	if isTailnetIP(remoteIP) {
		return remoteIP, sourceTailnet
	}
	return remoteIP, sourceLocal
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
// (the static /10 for IPv4 and /48 for IPv6).  Custom CGNAT
// remappings are not supported here; an operator running on a
// non-default range would need to extend this classifier.
func isTailnetIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	return isTailscaleCGNAT(ip)
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

// writeUnavailable answers a request whose tailnet identity could not
// be resolved.  It is retryable: the next lookup may succeed.
func writeUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "5")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(forbiddenBody{Error: "peer_lookup_unavailable"}); err != nil {
		log.Printf("auth: encode unavailable body: %v", err)
	}
}

// capName is the grant a CapKind needs, as the refusal body names it.
func capName(k CapKind) string {
	switch k {
	case CapView:
		return "view"
	case CapAdmin:
		return "admin"
	}
	return ""
}

func writeForbidden(w http.ResponseWriter, code string, required CapKind) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	body := forbiddenBody{Error: code, Required: capName(required)}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("auth: encode forbidden body: %v", err)
	}
}
