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
	"sync"
	"time"

	"tailscale.com/client/local"
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
//     "no" — the api layer fails closed.
//   - (PeerIdentity{}, other error) on transport/timeout/socket
//     failures, when no recent identity is known for the address.
//     The api layer fails closed on these too: an unresolved peer is
//     never granted anything.
//
// A transient failure does not lock out a peer that was resolved
// recently: within lastGoodGrace of its last successful lookup, the
// identity it had then is returned instead of the error.  A tailscaled
// blip therefore costs nobody access they had a moment ago, and a
// peer the daemon has never resolved gains none.
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
	switch {
	case err == nil:
		m.peerCache.set(remoteAddr, peerCacheEntry{id: id})
		m.peerCache.setLastGood(remoteAddr, id)
	case errors.Is(err, ErrPeerNotFound):
		// Authoritative: cache it, and forget any identity the address
		// had, so a removed node is not served from lastGood.
		m.peerCache.set(remoteAddr, peerCacheEntry{err: err})
		m.peerCache.forgetLastGood(remoteAddr)
	default:
		// Transient errors are not cached, so a 1-second outage is not
		// amplified to peerCacheTTL of refusals.
		if last, ok := m.peerCache.lastGood(remoteAddr); ok {
			return last, nil
		}
	}
	return id, err
}

func (m *Manager) lookupPeerUncached(ctx context.Context, remoteAddr string) (PeerIdentity, error) {
	resp, err := m.lc.WhoIs(ctx, remoteAddr)
	if err != nil {
		// The local client maps the local API's 404 to
		// local.ErrPeerNotFound; anything else is transient.
		if errors.Is(err, local.ErrPeerNotFound) {
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

// peerCacheTTL bounds how long a successful or NotFound result is
// reused.  Short enough that a grant change in the tailnet ACL
// propagates within a session; long enough to absorb the per-peer
// burst of polling from the Settings page.
const peerCacheTTL = 5 * time.Second

// lastGoodGrace bounds how long a peer's last successful identity
// stands in for a transient lookup failure.  It is long enough to ride
// out a tailscaled restart and short enough that a revoked grant is not
// honoured for long while the daemon is unreachable.
const lastGoodGrace = 10 * time.Minute

// maxPeerCacheEntries bounds each of the cache's maps.  The tailnet
// has a handful of peers; only a local process forging
// X-Forwarded-For across the tailnet range could add more, and it
// would not get past the gate by doing so.
const maxPeerCacheEntries = 1024

type peerCacheEntry struct {
	id  PeerIdentity
	err error
	at  time.Time
}

type peerCache struct {
	mu   sync.Mutex
	m    map[string]peerCacheEntry
	good map[string]peerCacheEntry
}

func (c *peerCache) get(k string) (peerCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	boundEntries(c.m, peerCacheTTL)
}

func (c *peerCache) setLastGood(k string, id PeerIdentity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.good == nil {
		c.good = make(map[string]peerCacheEntry)
	}
	c.good[k] = peerCacheEntry{id: id, at: time.Now()}
	boundEntries(c.good, lastGoodGrace)
}

func (c *peerCache) lastGood(k string) (PeerIdentity, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.good[k]
	if !ok || time.Since(e.at) > lastGoodGrace {
		return PeerIdentity{}, false
	}
	return e.id, true
}

func (c *peerCache) forgetLastGood(k string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.good, k)
}

// boundEntries keeps m within maxPeerCacheEntries: it drops expired
// entries first and, if that is not enough, the oldest.  The caller
// holds the lock.
func boundEntries(m map[string]peerCacheEntry, ttl time.Duration) {
	if len(m) <= maxPeerCacheEntries {
		return
	}
	for k, e := range m {
		if time.Since(e.at) > ttl {
			delete(m, k)
		}
	}
	for len(m) > maxPeerCacheEntries {
		oldestKey, oldest := "", time.Time{}
		for k, e := range m {
			if oldestKey == "" || e.at.Before(oldest) {
				oldestKey, oldest = k, e.at
			}
		}
		delete(m, oldestKey)
	}
}
