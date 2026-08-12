// Peer-capability lookups for HTTP authorization.
//
// The Manager exposes a narrow surface that the api layer uses to
// decide whether an inbound request originated on the tailnet, and
// whether the originating peer has any velocity.report/cap/* grants.
// The api package depends only on the types defined here, not on
// tailscale.com/client/tailscale/apitype, so an upstream API change
// in apitype is contained to this file.

package tailscale

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"tailscale.com/tailcfg"
)

// Capability names recognised by velocity.report.  The presence of
// either grant on a peer authorises that peer for the corresponding
// access level; the value (the JSON array on the right-hand side of
// a grant) is unused in v1 and reserved for future per-grant
// arguments.
const (
	CapView  = "velocity.report/cap/view"
	CapAdmin = "velocity.report/cap/admin"
)

// PeerIdentity is the trimmed-down view of a tailnet peer that the
// api layer needs.  Defining our own type (rather than re-exposing
// apitype.WhoIsResponse) keeps the api package free of the
// tailscale.com/client/tailscale/apitype dependency.
type PeerIdentity struct {
	// View is true when the peer has the velocity.report/cap/view
	// grant (or velocity.report/cap/admin, which implies view).
	View bool
	// Admin is true when the peer has velocity.report/cap/admin.
	Admin bool
}

// ErrPeerNotFound is returned by LookupPeer when the daemon is
// reachable and authoritative but does not know the address — i.e.
// the address is not a tailnet peer.  This is distinct from a
// transient lookup error (network/socket/timeout) and the api layer
// uses the distinction to fail closed only on an authoritative
// "no such peer" result.
var ErrPeerNotFound = errors.New("tailscale: peer not found")

// LookupPeer resolves a remote address to a PeerIdentity.
//
// Three return paths:
//
//   - (PeerIdentity{...}, nil) on success, including the case where
//     the peer exists but has no velocity.report grants (both fields
//     false).
//   - (PeerIdentity{}, ErrPeerNotFound) when the daemon is reachable
//     and reports the address is not a tailnet peer.  Authoritative
//     "no" — the api layer can fail closed.
//   - (PeerIdentity{}, other error) on transport/timeout/socket
//     failures.  The api layer should fail open on these to avoid
//     a tailscaled blip locking everyone out simultaneously.
//
// Results are cached for a short TTL keyed on remoteAddr to keep
// daemon traffic proportional to peer count rather than request
// rate.  The cache is process-local, so a restart re-reads grants
// fresh.
func (m *Manager) LookupPeer(ctx context.Context, remoteAddr string) (PeerIdentity, error) {
	if v, ok := m.peerCache.get(remoteAddr); ok {
		return v.id, v.err
	}
	id, err := m.lookupPeerUncached(ctx, remoteAddr)
	// Only cache definitive answers (success or NotFound).  Transient
	// errors must be retried promptly, otherwise a 1-second outage
	// gets amplified to peerCacheTTL of 403s.
	if err == nil || errors.Is(err, ErrPeerNotFound) {
		m.peerCache.set(remoteAddr, peerCacheEntry{id: id, err: err})
	}
	return id, err
}

func (m *Manager) lookupPeerUncached(ctx context.Context, remoteAddr string) (PeerIdentity, error) {
	resp, err := m.lc.WhoIs(ctx, remoteAddr)
	if err != nil {
		// The local API does not give us a typed "not found" today;
		// the conventional shape of the error string is matched
		// here.  Anything else is treated as a transient error.
		if isPeerNotFound(err) {
			return PeerIdentity{}, ErrPeerNotFound
		}
		return PeerIdentity{}, err
	}
	if resp == nil {
		return PeerIdentity{}, ErrPeerNotFound
	}
	id := PeerIdentity{}
	for name := range resp.CapMap {
		switch tailcfg.PeerCapability(name) {
		case CapView:
			id.View = true
		case CapAdmin:
			id.Admin = true
		}
	}
	// Admin implies view at the policy level so the api layer can
	// just check Admin/View; we still record both so a future change
	// can distinguish them.
	if id.Admin {
		id.View = true
	}
	return id, nil
}

// isPeerNotFound reports whether err looks like the local API's
// authoritative "no such peer" response.  The local client today
// returns a wrapped error whose message contains "no match for IP";
// match only that exact phrase.  Broader substrings like "not found"
// or "404" would catch unrelated transport errors and incorrectly
// fail closed (403) when the safe degradation is fail open.  If
// upstream renames the phrase, we'll classify NotFound as transient
// and fail open — the right way round if we must be wrong.
func isPeerNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no match for IP")
}

// peerCacheTTL bounds how long a successful or NotFound result is
// reused.  Short enough that a grant change in the tailnet ACL
// propagates within a session; long enough to absorb the per-peer
// burst of polling from the Settings page.
const peerCacheTTL = 5 * time.Second

type peerCacheEntry struct {
	id  PeerIdentity
	err error
	at  time.Time
}

type peerCache struct {
	mu sync.Mutex
	// Entries are bounded by callers' peer set size in the typical
	// case (a handful).  We do not cap the map: a runaway here
	// implies a request flood from many distinct IPs, which is a
	// separate problem worth detecting elsewhere.
	m map[string]peerCacheEntry
}

func (c *peerCache) get(k string) (peerCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		return peerCacheEntry{}, false
	}
	e, ok := c.m[k]
	if !ok {
		return peerCacheEntry{}, false
	}
	if time.Since(e.at) > peerCacheTTL {
		delete(c.m, k)
		return peerCacheEntry{}, false
	}
	return e, true
}

func (c *peerCache) set(k string, e peerCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]peerCacheEntry)
	}
	e.at = time.Now()
	c.m[k] = e
}
