package tailscale

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
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

// ageEntry expires a cached result without sleeping through the TTL.
func ageEntry(m *Manager, k string, by time.Duration) {
	m.peerCache.mu.Lock()
	defer m.peerCache.mu.Unlock()
	if e, ok := m.peerCache.m[k]; ok {
		e.at = e.at.Add(-by)
		m.peerCache.m[k] = e
	}
}

func adminPeerResponse() *apitype.WhoIsResponse {
	return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{tailcfg.PeerCapability(CapAdmin): nil}}
}

// An outage after the short cache expires never borrows a previous grant.
func TestLookupPeer_ExpiredGrantFailsClosedOnTransientFailure(t *testing.T) {
	for _, cap := range []string{CapView, CapAdmin} {
		t.Run(cap, func(t *testing.T) {
			var failing atomic.Bool
			outage := errors.New("dial unix /var/run/tailscale/tailscaled.sock: connection refused")
			c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
				if failing.Load() {
					return nil, outage
				}
				return &apitype.WhoIsResponse{CapMap: tailcfg.PeerCapMap{tailcfg.PeerCapability(cap): nil}}, nil
			}}
			m := newMgr(c)
			const peer = "100.64.0.5"
			if id, err := m.LookupPeer(context.Background(), peer); err != nil || !id.View {
				t.Fatalf("first lookup: %+v, %v", id, err)
			}
			failing.Store(true)
			ageEntry(m, peer, peerCacheTTL+time.Second)
			for i := 0; i < 2; i++ {
				if id, err := m.LookupPeer(context.Background(), peer); id != (PeerIdentity{}) || !errors.Is(err, outage) {
					t.Fatalf("outage lookup: %+v, %v; want no identity and the transport error", id, err)
				}
			}
			if got := c.whoIsCalls.Load(); got != 3 {
				t.Fatalf("got %d daemon calls, want 3; transient errors must not be cached", got)
			}
		})
	}
}

func TestLookupPeer_TransientFailureWithoutHistoryIsAnError(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		return nil, context.DeadlineExceeded
	}}
	if id, err := newMgr(c).LookupPeer(context.Background(), "100.64.0.7"); id != (PeerIdentity{}) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %+v, %v; want no identity and the deadline error", id, err)
	}
}

func TestLookupPeer_RevocationAndLaterOutageNeverRestoreGrant(t *testing.T) {
	for _, notFound := range []bool{false, true} {
		t.Run(fmt.Sprintf("notFound=%v", notFound), func(t *testing.T) {
			var answer atomic.Int32
			outage := errors.New("socket unavailable")
			c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
				switch answer.Load() {
				case 0:
					return adminPeerResponse(), nil
				case 1:
					if notFound {
						return nil, local.ErrPeerNotFound
					}
					return &apitype.WhoIsResponse{}, nil
				default:
					return nil, outage
				}
			}}
			m := newMgr(c)
			const peer = "100.64.0.8"
			if id, err := m.LookupPeer(context.Background(), peer); err != nil || !id.Admin {
				t.Fatalf("first lookup: %+v, %v", id, err)
			}
			answer.Store(1)
			ageEntry(m, peer, peerCacheTTL+time.Second)
			for i := 0; i < 2; i++ {
				id, err := m.LookupPeer(context.Background(), peer)
				if id != (PeerIdentity{}) || (notFound && !errors.Is(err, ErrPeerNotFound)) || (!notFound && err != nil) {
					t.Fatalf("revoked lookup: %+v, %v", id, err)
				}
			}
			if got := c.whoIsCalls.Load(); got != 2 {
				t.Fatalf("got %d daemon calls, want 2; authoritative denials must be cached", got)
			}
			answer.Store(2)
			ageEntry(m, peer, peerCacheTTL+time.Second)
			if id, err := m.LookupPeer(context.Background(), peer); id != (PeerIdentity{}) || !errors.Is(err, outage) {
				t.Fatalf("after revocation and outage: %+v, %v", id, err)
			}
		})
	}
}

// Done announces that a caller has reached the coalesced lookup's wait.
// This makes the concurrency tests independent of scheduling or sleeps.
type peerWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *peerWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

type peerLookupResult struct {
	id  PeerIdentity
	err error
}

func asyncPeerLookup(m *Manager, ctx context.Context, peer string) <-chan peerLookupResult {
	result := make(chan peerLookupResult, 1)
	go func() {
		id, err := m.LookupPeer(ctx, peer)
		result <- peerLookupResult{id, err}
	}()
	return result
}

func testPeerContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestLookupPeer_ConcurrentCallsShareOneLookup(t *testing.T) {
	for _, notFound := range []bool{false, true} {
		t.Run(fmt.Sprintf("notFound=%v", notFound), func(t *testing.T) {
			ctx := testPeerContext(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var revoked atomic.Bool
			c := &peercapsClient{whoIsFn: func(ctx context.Context, _ string) (*apitype.WhoIsResponse, error) {
				if revoked.Load() {
					if notFound {
						return nil, local.ErrPeerNotFound
					}
					return &apitype.WhoIsResponse{}, nil
				}
				close(entered)
				select {
				case <-release:
					return adminPeerResponse(), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}}
			m := newMgr(c)
			const peer = "100.64.0.11"
			first := asyncPeerLookup(m, ctx, peer)
			<-entered
			waitCtx := &peerWaitContext{Context: ctx, waiting: make(chan struct{})}
			second := asyncPeerLookup(m, waitCtx, peer)
			<-waitCtx.waiting
			if got := c.whoIsCalls.Load(); got != 1 {
				t.Fatalf("got %d overlapping daemon calls, want 1", got)
			}
			revoked.Store(true)
			close(release)
			for _, done := range []<-chan peerLookupResult{first, second} {
				result := <-done
				if result.err != nil || !result.id.Admin {
					t.Fatalf("shared lookup: %+v", result)
				}
			}
			ageEntry(m, peer, peerCacheTTL+time.Second)
			id, err := m.LookupPeer(ctx, peer)
			if id != (PeerIdentity{}) || (notFound && !errors.Is(err, ErrPeerNotFound)) || (!notFound && err != nil) {
				t.Fatalf("lookup after revocation: %+v, %v", id, err)
			}
			if got := c.whoIsCalls.Load(); got != 2 {
				t.Fatalf("got %d daemon calls, want 2", got)
			}
		})
	}
}

// A caller can be descheduled after the shared lookup completes. Block its
// final context check to reproduce that delay, then complete a newer denial.
type delayedPeerResultContext struct {
	context.Context
	checks  atomic.Int32
	paused  chan struct{}
	release chan struct{}
}

func (c *delayedPeerResultContext) Err() error {
	if c.checks.Add(1) == 2 {
		close(c.paused)
		<-c.release
	}
	return c.Context.Err()
}

func TestLookupPeer_DelayedCallerCannotReturnRevokedIdentity(t *testing.T) {
	for _, notFound := range []bool{false, true} {
		t.Run(fmt.Sprintf("notFound=%v", notFound), func(t *testing.T) {
			ctx := testPeerContext(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var revoked atomic.Bool
			c := &peercapsClient{whoIsFn: func(ctx context.Context, _ string) (*apitype.WhoIsResponse, error) {
				if revoked.Load() {
					if notFound {
						return nil, local.ErrPeerNotFound
					}
					return &apitype.WhoIsResponse{}, nil
				}
				close(entered)
				select {
				case <-release:
					return adminPeerResponse(), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}}
			m := newMgr(c)
			const peer = "100.64.0.12"
			first := asyncPeerLookup(m, ctx, peer)
			<-entered
			delayCtx := &delayedPeerResultContext{Context: ctx, paused: make(chan struct{}), release: make(chan struct{})}
			waitCtx := &peerWaitContext{Context: delayCtx, waiting: make(chan struct{})}
			second := asyncPeerLookup(m, waitCtx, peer)
			<-waitCtx.waiting
			close(release)
			if result := <-first; result.err != nil || !result.id.Admin {
				t.Fatalf("first lookup: %+v", result)
			}
			<-delayCtx.paused
			revoked.Store(true)
			ageEntry(m, peer, peerCacheTTL+time.Second)
			if id, _ := m.LookupPeer(ctx, peer); id != (PeerIdentity{}) {
				t.Fatalf("newer lookup still has grants: %+v", id)
			}
			close(delayCtx.release)
			result := <-second
			if result.id != (PeerIdentity{}) || (notFound && !errors.Is(result.err, ErrPeerNotFound)) || (!notFound && result.err != nil) {
				t.Fatalf("delayed older caller restored grant: %+v", result)
			}
		})
	}
}

func TestLookupPeer_WaiterCancellationDoesNotCancelOwner(t *testing.T) {
	ctx := testPeerContext(t)
	entered, release := make(chan struct{}), make(chan struct{})
	c := &peercapsClient{whoIsFn: func(ctx context.Context, _ string) (*apitype.WhoIsResponse, error) {
		close(entered)
		select {
		case <-release:
			return adminPeerResponse(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	m := newMgr(c)
	first := asyncPeerLookup(m, ctx, "100.64.0.13")
	<-entered
	cancelCtx, cancel := context.WithCancel(ctx)
	waitCtx := &peerWaitContext{Context: cancelCtx, waiting: make(chan struct{})}
	second := asyncPeerLookup(m, waitCtx, "100.64.0.13")
	<-waitCtx.waiting
	cancel()
	if result := <-second; result.id != (PeerIdentity{}) || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancelled waiter: %+v", result)
	}
	close(release)
	if result := <-first; result.err != nil || !result.id.Admin {
		t.Fatalf("owner: %+v", result)
	}
	if got := c.whoIsCalls.Load(); got != 1 {
		t.Fatalf("got %d daemon calls, want 1", got)
	}
}

func TestLookupPeer_CancelledOwnerCannotCacheGrant(t *testing.T) {
	ctx, cancel := context.WithCancel(testPeerContext(t))
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := &peercapsClient{whoIsFn: func(_ context.Context, _ string) (*apitype.WhoIsResponse, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release // deliberately ignore cancellation like a slow transport
		}
		return adminPeerResponse(), nil
	}}
	m := newMgr(c)
	first := asyncPeerLookup(m, ctx, "100.64.0.14")
	<-entered
	cancel()
	close(release)
	if result := <-first; result.id != (PeerIdentity{}) || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancelled owner: %+v", result)
	}
	if id, err := m.LookupPeer(context.Background(), "100.64.0.14"); err != nil || !id.Admin {
		t.Fatalf("retry: %+v, %v", id, err)
	}
	if got := c.whoIsCalls.Load(); got != 2 {
		t.Fatalf("got %d daemon calls, want 2; cancelled grant was cached", got)
	}
}

func TestLookupPeer_ActiveLookupsAreBounded(t *testing.T) {
	ctx := testPeerContext(t)
	entered := make(chan struct{}, maxPeerCacheEntries)
	release := make(chan struct{})
	c := &peercapsClient{whoIsFn: func(ctx context.Context, _ string) (*apitype.WhoIsResponse, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return &apitype.WhoIsResponse{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	m := newMgr(c)
	results := make([]<-chan peerLookupResult, maxPeerCacheEntries)
	for i := range results {
		results[i] = asyncPeerLookup(m, ctx, fmt.Sprintf("100.64.%d.%d", i/256, i%256))
	}
	for range results {
		<-entered
	}
	if id, err := m.LookupPeer(ctx, "100.65.0.1"); id != (PeerIdentity{}) || !errors.Is(err, errPeerLookupBusy) {
		t.Fatalf("lookup above capacity: %+v, %v", id, err)
	}
	if got := c.whoIsCalls.Load(); got != maxPeerCacheEntries {
		t.Fatalf("got %d daemon calls, want %d", got, maxPeerCacheEntries)
	}
	// Existing peers can still join at capacity, without evicting the owner.
	cancelCtx, cancel := context.WithCancel(ctx)
	waitCtx := &peerWaitContext{Context: cancelCtx, waiting: make(chan struct{})}
	joined := asyncPeerLookup(m, waitCtx, "100.64.0.0")
	<-waitCtx.waiting
	cancel()
	if result := <-joined; !errors.Is(result.err, context.Canceled) {
		t.Fatalf("existing peer at capacity: %+v", result)
	}
	close(release)
	for _, done := range results {
		if result := <-done; result.err != nil {
			t.Fatalf("active lookup: %+v", result)
		}
	}
	if _, err := m.LookupPeer(ctx, "100.65.0.1"); err != nil {
		t.Fatalf("capacity not released after completion: %v", err)
	}
	m.peerCache.mu.Lock()
	defer m.peerCache.mu.Unlock()
	if len(m.peerCache.inflight) != 0 || len(m.peerCache.m) > maxPeerCacheEntries {
		t.Fatalf("active=%d cached=%d, want no active and bounded cache", len(m.peerCache.inflight), len(m.peerCache.m))
	}
}

func TestLookupPeer_CachePressureDoesNotEvictActiveLookup(t *testing.T) {
	ctx := testPeerContext(t)
	entered, release := make(chan struct{}), make(chan struct{})
	const peer = "100.65.0.2"
	var peerCalls atomic.Int32
	c := &peercapsClient{whoIsFn: func(ctx context.Context, address string) (*apitype.WhoIsResponse, error) {
		if address == peer {
			peerCalls.Add(1)
			close(entered)
			select {
			case <-release:
				return nil, local.ErrPeerNotFound
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return adminPeerResponse(), nil
	}}
	m := newMgr(c)
	first := asyncPeerLookup(m, ctx, peer)
	<-entered
	for i := 0; i < maxPeerCacheEntries+50; i++ {
		if _, err := m.LookupPeer(ctx, fmt.Sprintf("100.64.%d.%d", i/256, i%256)); err != nil {
			t.Fatalf("cache-filling lookup: %v", err)
		}
	}
	waitCtx := &peerWaitContext{Context: ctx, waiting: make(chan struct{})}
	second := asyncPeerLookup(m, waitCtx, peer)
	<-waitCtx.waiting
	close(release)
	for _, done := range []<-chan peerLookupResult{first, second} {
		if result := <-done; result.id != (PeerIdentity{}) || !errors.Is(result.err, ErrPeerNotFound) {
			t.Fatalf("shared denial under cache pressure: %+v", result)
		}
	}
	if got := peerCalls.Load(); got != 1 {
		t.Fatalf("got %d competing peer lookups, want 1", got)
	}
}

func TestPeerCacheIsBounded(t *testing.T) {
	var c peerCache
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < maxPeerCacheEntries+50; i++ {
		c.setLocked(fmt.Sprintf("peer-%d", i), peerCacheEntry{at: time.Now()})
	}
	if len(c.m) > maxPeerCacheEntries {
		t.Fatalf("cache size %d, want at most %d", len(c.m), maxPeerCacheEntries)
	}
	if _, ok := c.m[fmt.Sprintf("peer-%d", maxPeerCacheEntries+49)]; !ok {
		t.Fatal("the newest entry was evicted")
	}
}

func TestLookupPeer_NoResponseAndPreCancelledRequest(t *testing.T) {
	c := &peercapsClient{whoIsFn: func(context.Context, string) (*apitype.WhoIsResponse, error) { return nil, nil }}
	m := newMgr(c)
	if id, err := m.LookupPeer(context.Background(), "100.64.0.99"); id != (PeerIdentity{}) || !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("empty daemon response: %+v %v", id, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.LookupPeer(ctx, "100.64.0.98"); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled request: %v", err)
	}
	if c.whoIsCalls.Load() != 1 {
		t.Fatal("cancelled request reached daemon")
	}
}

func TestDelayedGrantCannotSurviveExpiryOrEviction(t *testing.T) {
	// A resumed waiter must not use its completed grant if the cache expired
	// or capacity pressure evicted the only authoritative answer.
	for _, expired := range []bool{false, true} {
		c := &peerCache{m: map[string]peerCacheEntry{}}
		if expired {
			c.m["peer"] = peerCacheEntry{id: PeerIdentity{Admin: true}, at: time.Now().Add(-peerCacheTTL - time.Second)}
		}
		if id, err := c.completed(context.Background(), "peer", &peerLookup{id: PeerIdentity{Admin: true}}); id != (PeerIdentity{}) || !errors.Is(err, errPeerLookupExpired) {
			t.Fatalf("expired=%v: %+v %v", expired, id, err)
		}
	}
	entries := make(map[string]peerCacheEntry, maxPeerCacheEntries+1)
	for i := 0; i <= maxPeerCacheEntries; i++ {
		entries[fmt.Sprint(i)] = peerCacheEntry{at: time.Now().Add(-peerCacheTTL - time.Second)}
	}
	boundEntries(entries, peerCacheTTL)
	if len(entries) != 0 {
		t.Fatal("expired entries survived capacity cleanup")
	}
}
