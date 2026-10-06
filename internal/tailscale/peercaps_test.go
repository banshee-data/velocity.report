package tailscale

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

// peercapsClient is a minimal LocalClient that only implements the
// methods LookupPeer touches.  Reusing the fakeClient from
// manager_test.go would work but pulls in too much noise; this stub
// keeps the peercaps tests focused.
type peercapsClient struct {
	whoIsFn  func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
	statusFn func(ctx context.Context) (*ipnstate.Status, error)

	whoIsCalls  atomic.Int32
	statusCalls atomic.Int32
}

func (c *peercapsClient) Status(ctx context.Context) (*ipnstate.Status, error) {
	return c.StatusWithoutPeers(ctx)
}
func (c *peercapsClient) StatusWithoutPeers(ctx context.Context) (*ipnstate.Status, error) {
	c.statusCalls.Add(1)
	if c.statusFn != nil {
		return c.statusFn(ctx)
	}
	return &ipnstate.Status{}, nil
}
func (c *peercapsClient) GetPrefs(_ context.Context) (*ipn.Prefs, error) { return &ipn.Prefs{}, nil }
func (c *peercapsClient) EditPrefs(_ context.Context, _ *ipn.MaskedPrefs) (*ipn.Prefs, error) {
	return &ipn.Prefs{}, nil
}
func (c *peercapsClient) GetServeConfig(_ context.Context) (*ipn.ServeConfig, error) {
	return &ipn.ServeConfig{}, nil
}
func (c *peercapsClient) SetServeConfig(_ context.Context, _ *ipn.ServeConfig) error { return nil }
func (c *peercapsClient) StartLoginInteractive(_ context.Context) error              { return nil }
func (c *peercapsClient) Start(_ context.Context, _ ipn.Options) error               { return nil }
func (c *peercapsClient) WatchIPNBus(_ context.Context, _ ipn.NotifyWatchOpt) (BusWatcher, error) {
	return nil, errors.New("not used")
}
func (c *peercapsClient) WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	c.whoIsCalls.Add(1)
	if c.whoIsFn != nil {
		return c.whoIsFn(ctx, remoteAddr)
	}
	return nil, errors.New("not configured")
}

func newMgr(c LocalClient) *Manager {
	return New(WithLocalClient(c))
}

func TestLookupPeer_AdminGrantImpliesView(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{
			tailcfg.PeerCapability(CapAdmin): nil,
		}}, nil
	}}
	m := newMgr(c)
	id, err := m.LookupPeer(context.Background(), "100.64.0.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !id.Admin || !id.View {
		t.Errorf("admin grant should imply view: %+v", id)
	}
}

func TestLookupPeer_ViewOnly(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{
			tailcfg.PeerCapability(CapView): nil,
		}}, nil
	}}
	m := newMgr(c)
	id, _ := m.LookupPeer(context.Background(), "100.64.0.5")
	if id.Admin {
		t.Errorf("view-only should not have admin: %+v", id)
	}
	if !id.View {
		t.Errorf("view-only should have view: %+v", id)
	}
}

func TestLookupPeer_NoCapsIsAuthoritative(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{}}, nil
	}}
	m := newMgr(c)
	id, err := m.LookupPeer(context.Background(), "100.64.0.5")
	if err != nil {
		t.Fatalf("nil error expected (peer is found, just has no caps): %v", err)
	}
	if id.View || id.Admin {
		t.Errorf("peer with no caps should have neither: %+v", id)
	}
}

func TestLookupPeer_NotFound(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return nil, fmt.Errorf("whois: %w", local.ErrPeerNotFound)
	}}
	m := newMgr(c)
	_, err := m.LookupPeer(context.Background(), "192.168.1.1")
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("expected ErrPeerNotFound, got %v", err)
	}
}

func TestLookupPeer_TransientErrorsNotMisclassified(t *testing.T) {
	// Only local.ErrPeerNotFound is authoritative.  Errors that merely
	// contain "not found" or "404" are transport failures: the api
	// layer answers them 503 rather than caching a 403 for the peer.
	cases := []string{
		"some upstream said: not found",
		"http 404 while reading response",
		"resource not found at /local/v0/whois",
		"no match for IP 100.64.0.5",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
				return nil, errors.New(msg)
			}}
			m := newMgr(c)
			_, err := m.LookupPeer(context.Background(), "100.64.0.5")
			if errors.Is(err, ErrPeerNotFound) {
				t.Fatalf("err %q was misclassified as ErrPeerNotFound", msg)
			}
			if err == nil {
				t.Fatalf("expected a transient error, got nil")
			}
		})
	}
}

func TestLookupPeer_TransientErrorNotCached(t *testing.T) {
	calls := 0
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		calls++
		return nil, errors.New("connection refused")
	}}
	m := newMgr(c)
	if _, err := m.LookupPeer(context.Background(), "100.64.0.5"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := m.LookupPeer(context.Background(), "100.64.0.5"); err == nil {
		t.Fatal("expected error on second call")
	}
	if calls != 2 {
		t.Fatalf("transient errors must not be cached: got %d daemon calls, want 2", calls)
	}
}

func TestLookupPeer_SuccessCached(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{
			tailcfg.PeerCapability(CapView): nil,
		}}, nil
	}}
	m := newMgr(c)
	for i := 0; i < 5; i++ {
		if _, err := m.LookupPeer(context.Background(), "100.64.0.5"); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if got := c.whoIsCalls.Load(); got != 1 {
		t.Errorf("expected 1 daemon call (rest cached), got %d", got)
	}
}

func TestLookupPeer_NotFoundCached(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return nil, local.ErrPeerNotFound
	}}
	m := newMgr(c)
	for i := 0; i < 3; i++ {
		_, _ = m.LookupPeer(context.Background(), "10.0.0.1")
	}
	if got := c.whoIsCalls.Load(); got != 1 {
		t.Errorf("ErrPeerNotFound should be cached: got %d daemon calls, want 1", got)
	}
}

// The real local client, talking to a local API that answers whois with
// 404, yields ErrPeerNotFound.  This is the path the string match on
// "no match for IP" never reached: the client maps the 404 to
// local.ErrPeerNotFound and drops the body.
func TestLookupPeer_RealClientNotFound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/localapi/v0/whois" {
			http.Error(w, "no match for IP:port", http.StatusNotFound)
			return
		}
		http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	lc := &local.Client{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", ln.Addr().String())
		},
		OmitAuth: true,
	}
	m := New(WithLocalClient(&realLocalClient{c: lc}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.LookupPeer(ctx, "100.64.0.9:443"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("got %v, want ErrPeerNotFound", err)
	}
}

// ageEntry makes the short-lived cache entry for k look older than its
// TTL, so the next lookup asks the daemon again.
func ageEntry(m *Manager, k string, by time.Duration) {
	m.peerCache.mu.Lock()
	defer m.peerCache.mu.Unlock()
	if e, ok := m.peerCache.m[k]; ok {
		e.at = e.at.Add(-by)
		m.peerCache.m[k] = e
	}
	if e, ok := m.peerCache.good[k]; ok {
		e.at = e.at.Add(-by)
		m.peerCache.good[k] = e
	}
}

// A transient failure after a successful lookup returns the identity the
// peer had, for lastGoodGrace; after that it is an error again.
func TestLookupPeer_TransientFailureUsesLastGoodIdentity(t *testing.T) {
	failing := false
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		if failing {
			return nil, errors.New("dial unix /var/run/tailscale/tailscaled.sock: connection refused")
		}
		return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{tailcfg.PeerCapability(CapAdmin): nil}}, nil
	}}
	m := newMgr(c)
	const peer = "100.64.0.5"
	if id, err := m.LookupPeer(context.Background(), peer); err != nil || !id.Admin {
		t.Fatalf("first lookup: %+v, %v", id, err)
	}

	failing = true
	ageEntry(m, peer, peerCacheTTL+time.Second)
	id, err := m.LookupPeer(context.Background(), peer)
	if err != nil || !id.Admin {
		t.Fatalf("lookup during a blip: %+v, %v; want the last good admin identity", id, err)
	}

	ageEntry(m, peer, lastGoodGrace)
	if id, err := m.LookupPeer(context.Background(), peer); err == nil {
		t.Fatalf("lookup after lastGoodGrace: %+v, want the transient error", id)
	}
}

// A peer never resolved gets no stand-in identity from a transient failure.
func TestLookupPeer_TransientFailureWithoutHistoryIsAnError(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return nil, context.DeadlineExceeded
	}}
	if id, err := newMgr(c).LookupPeer(context.Background(), "100.64.0.7"); err == nil {
		t.Fatalf("got %+v, want an error", id)
	}
}

// An authoritative not-found forgets the last good identity, so a node
// removed from the tailnet is not served from it during a later blip.
func TestLookupPeer_NotFoundForgetsLastGoodIdentity(t *testing.T) {
	answer := "admin"
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		switch answer {
		case "admin":
			return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{tailcfg.PeerCapability(CapAdmin): nil}}, nil
		case "gone":
			return nil, local.ErrPeerNotFound
		}
		return nil, errors.New("timeout")
	}}
	m := newMgr(c)
	const peer = "100.64.0.8"
	_, _ = m.LookupPeer(context.Background(), peer)
	answer = "gone"
	ageEntry(m, peer, peerCacheTTL+time.Second)
	if _, err := m.LookupPeer(context.Background(), peer); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("got %v, want ErrPeerNotFound", err)
	}
	answer = "blip"
	ageEntry(m, peer, peerCacheTTL+time.Second)
	if id, err := m.LookupPeer(context.Background(), peer); err == nil {
		t.Fatalf("got %+v after removal and a blip, want an error", id)
	}
}

// The cache stays bounded however many addresses are looked up.
func TestPeerCacheIsBounded(t *testing.T) {
	var c peerCache
	for i := 0; i < maxPeerCacheEntries+50; i++ {
		k := fmt.Sprintf("100.64.%d.%d", i/256, i%256)
		c.set(k, peerCacheEntry{})
		c.setLastGood(k, PeerIdentity{View: true})
	}
	if len(c.m) > maxPeerCacheEntries || len(c.good) > maxPeerCacheEntries {
		t.Fatalf("cache sizes %d and %d, want at most %d", len(c.m), len(c.good), maxPeerCacheEntries)
	}
	if _, ok := c.lastGood(fmt.Sprintf("100.64.%d.%d", (maxPeerCacheEntries+49)/256, (maxPeerCacheEntries+49)%256)); !ok {
		t.Fatal("the newest entry was evicted")
	}
}
