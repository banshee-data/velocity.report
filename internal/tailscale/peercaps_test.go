package tailscale

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

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
		return nil, errors.New("no match for IP 192.168.1.1")
	}}
	m := newMgr(c)
	_, err := m.LookupPeer(context.Background(), "192.168.1.1")
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("expected ErrPeerNotFound, got %v", err)
	}
}

func TestLookupPeer_TransientErrorsNotMisclassified(t *testing.T) {
	// Errors that merely contain "not found" or "404" must NOT be
	// classified as authoritative ErrPeerNotFound — otherwise a
	// transport blip with a generic message causes the api layer
	// to fail closed (403) instead of fail open.
	cases := []string{
		"some upstream said: not found",
		"http 404 while reading response",
		"resource not found at /local/v0/whois",
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
		return nil, errors.New("no match for IP")
	}}
	m := newMgr(c)
	for i := 0; i < 3; i++ {
		_, _ = m.LookupPeer(context.Background(), "10.0.0.1")
	}
	if got := c.whoIsCalls.Load(); got != 1 {
		t.Errorf("ErrPeerNotFound should be cached: got %d daemon calls, want 1", got)
	}
}
