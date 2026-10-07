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
// uses the distinction to report forbidden versus temporarily unavailable.
// Both results fail closed.
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
//     failures. Once the short cache expires, previous grants never
//     stand in for a failed lookup.
//
// Results are cached for a short TTL keyed on remoteAddr to keep
// daemon traffic proportional to peer count rather than request
// rate.  The cache is process-local, so a restart re-reads grants
// fresh. Only one lookup for a given peer may be in flight: callers
// share its answer rather than allowing an older answer to arrive
// after a newer authoritative denial. Active lookups are bounded and
// are never evicted to make room for another peer.
func (m *Manager) LookupPeer(ctx context.Context, remoteAddr string) (PeerIdentity, error) {
	return m.peerCache.lookup(ctx, remoteAddr, m.lookupPeerUncached)
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

// maxPeerCacheEntries bounds both cached results and active peer lookups.
const maxPeerCacheEntries = 1024

var errPeerLookupBusy = errors.New("tailscale: peer lookup capacity exhausted")
var errPeerLookupExpired = errors.New("tailscale: peer lookup expired before use")

type peerCacheEntry struct {
	id  PeerIdentity
	err error
	// at is when the lookup started, so a slow lookup cannot extend
	// the lifetime of an old grant by the time it spent in flight.
	at time.Time
}

type peerLookup struct {
	done chan struct{}
	id   PeerIdentity
	err  error
}

type peerCache struct {
	mu       sync.Mutex
	m        map[string]peerCacheEntry
	inflight map[string]*peerLookup
}

func (c *peerCache) lookup(ctx context.Context, k string, lookup func(context.Context, string) (PeerIdentity, error)) (PeerIdentity, error) {
	if err := ctx.Err(); err != nil {
		return PeerIdentity{}, err
	}
	c.mu.Lock()
	if e, ok := c.getLocked(k); ok {
		c.mu.Unlock()
		return e.id, e.err
	}
	if pending, ok := c.inflight[k]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return PeerIdentity{}, ctx.Err()
		case <-pending.done:
			return c.completed(ctx, k, pending)
		}
	}
	if len(c.inflight) >= maxPeerCacheEntries {
		c.mu.Unlock()
		return PeerIdentity{}, errPeerLookupBusy
	}
	if c.inflight == nil {
		c.inflight = make(map[string]*peerLookup)
	}
	pending := &peerLookup{done: make(chan struct{})}
	c.inflight[k] = pending
	started := time.Now()
	c.mu.Unlock()

	id, err := lookup(ctx, k)
	if ctxErr := ctx.Err(); ctxErr != nil {
		id, err = PeerIdentity{}, ctxErr
	}
	c.mu.Lock()
	// Transient failures are never cached and never reuse an old grant.
	if err == nil || errors.Is(err, ErrPeerNotFound) {
		c.setLocked(k, peerCacheEntry{id: id, err: err, at: started})
	}
	pending.id, pending.err = id, err
	delete(c.inflight, k)
	close(pending.done)
	c.mu.Unlock()
	return c.completed(ctx, k, pending)
}

// completed rechecks the current cache before returning a grant. A waiter
// may resume after the shared answer expired and a later lookup revoked the
// grant; returning pending.id directly would authorise that older answer.
func (c *peerCache) completed(ctx context.Context, k string, pending *peerLookup) (PeerIdentity, error) {
	if err := ctx.Err(); err != nil {
		return PeerIdentity{}, err
	}
	if pending.err != nil {
		return PeerIdentity{}, pending.err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.getLocked(k); ok {
		return e.id, e.err
	}
	return PeerIdentity{}, errPeerLookupExpired
}

// getLocked and setLocked are called while c.mu is held. Cache eviction
// never touches inflight, so eviction cannot permit competing lookups.
func (c *peerCache) getLocked(k string) (peerCacheEntry, bool) {
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

func (c *peerCache) setLocked(k string, e peerCacheEntry) {
	if c.m == nil {
		c.m = make(map[string]peerCacheEntry)
	}
	c.m[k] = e
	boundEntries(c.m, peerCacheTTL)
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
