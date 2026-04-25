package tailscale

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

func TestNodeShortName(t *testing.T) {
	cases := []struct {
		name string
		st   *ipnstate.Status
		want string
	}{
		{"nil status", nil, ""},
		{"nil self", &ipnstate.Status{}, ""},
		{
			"fqdn with trailing dot",
			&ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "velocity.tailfoo.ts.net."}},
			"velocity",
		},
		{
			"single label",
			&ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "velocity"}},
			"velocity",
		},
		{
			"empty dnsname",
			&ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: ""}},
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeShortName(tc.st); got != tc.want {
				t.Fatalf("nodeShortName: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestNodeFQDN(t *testing.T) {
	cases := []struct {
		name string
		st   *ipnstate.Status
		want string
	}{
		{"nil", nil, ""},
		{"trailing dot stripped", &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "v.tailfoo.ts.net."}}, "v.tailfoo.ts.net"},
		{"no trailing dot", &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "v.tailfoo.ts.net"}}, "v.tailfoo.ts.net"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeFQDN(tc.st); got != tc.want {
				t.Fatalf("nodeFQDN: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSudoActionString(t *testing.T) {
	if got := sudoActionEnable.String(); got != "enable-tailscaled" {
		t.Fatalf("enable: got %q", got)
	}
	if got := sudoActionDisable.String(); got != "disable-tailscaled" {
		t.Fatalf("disable: got %q", got)
	}
	if got := sudoAction(99).String(); got != "unknown" {
		t.Fatalf("bogus: got %q", got)
	}
}

// fakeBusWatcher is a hand-driven IPN bus.  Tests push notifications via
// emit() and call failNext() to simulate watcher disconnects.  Bind the
// watcher's lifetime to the WatchIPNBus context with bind() so Manager
// shutdown unblocks pending Next() calls (mirrors the real local.Client
// behaviour).
type fakeBusWatcher struct {
	ch     chan ipn.Notify
	closed chan struct{}
	err    chan error
	ctx    context.Context
}

func newFakeBusWatcher() *fakeBusWatcher {
	return &fakeBusWatcher{
		ch:     make(chan ipn.Notify, 16),
		closed: make(chan struct{}),
		err:    make(chan error, 1),
	}
}

func (f *fakeBusWatcher) bind(ctx context.Context) *fakeBusWatcher {
	f.ctx = ctx
	return f
}

func (f *fakeBusWatcher) Next() (ipn.Notify, error) {
	if f.ctx == nil {
		select {
		case n := <-f.ch:
			return n, nil
		case err := <-f.err:
			return ipn.Notify{}, err
		case <-f.closed:
			return ipn.Notify{}, errors.New("watcher closed")
		}
	}
	select {
	case n := <-f.ch:
		return n, nil
	case err := <-f.err:
		return ipn.Notify{}, err
	case <-f.closed:
		return ipn.Notify{}, errors.New("watcher closed")
	case <-f.ctx.Done():
		return ipn.Notify{}, f.ctx.Err()
	}
}

func (f *fakeBusWatcher) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

func (f *fakeBusWatcher) emit(n ipn.Notify) {
	f.ch <- n
}

func (f *fakeBusWatcher) failNext(err error) {
	f.err <- err
}

// fakeClient is a minimal LocalClient used by the manager state-machine
// tests.  Methods record their calls in counters; specific behaviours
// are pluggable via function fields so individual tests can shape them.
type fakeClient struct {
	mu sync.Mutex

	statusFn      func(ctx context.Context) (*ipnstate.Status, error)
	statusNoPeers func(ctx context.Context) (*ipnstate.Status, error)
	getPrefs      func(ctx context.Context) (*ipn.Prefs, error)
	editPrefs     func(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error)
	getServeCfg   func(ctx context.Context) (*ipn.ServeConfig, error)
	setServeCfg   func(ctx context.Context, cfg *ipn.ServeConfig) error
	startLogin    func(ctx context.Context) error
	watchBus      func(ctx context.Context) (BusWatcher, error)

	editPrefsCalls    int32
	setServeCfgCalls  int32
	startLoginCalls   int32
	watchBusCalls     int32
	statusCalls       int32
	editedPrefs       []*ipn.MaskedPrefs
	setServeConfigArg *ipn.ServeConfig
}

func (f *fakeClient) Status(ctx context.Context) (*ipnstate.Status, error) {
	atomic.AddInt32(&f.statusCalls, 1)
	if f.statusFn != nil {
		return f.statusFn(ctx)
	}
	return &ipnstate.Status{BackendState: ipn.NoState.String()}, nil
}

func (f *fakeClient) StatusWithoutPeers(ctx context.Context) (*ipnstate.Status, error) {
	if f.statusNoPeers != nil {
		return f.statusNoPeers(ctx)
	}
	return f.Status(ctx)
}

func (f *fakeClient) GetPrefs(ctx context.Context) (*ipn.Prefs, error) {
	if f.getPrefs != nil {
		return f.getPrefs(ctx)
	}
	return &ipn.Prefs{}, nil
}

func (f *fakeClient) EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error) {
	atomic.AddInt32(&f.editPrefsCalls, 1)
	f.mu.Lock()
	f.editedPrefs = append(f.editedPrefs, mp)
	f.mu.Unlock()
	if f.editPrefs != nil {
		return f.editPrefs(ctx, mp)
	}
	return &ipn.Prefs{}, nil
}

func (f *fakeClient) GetServeConfig(ctx context.Context) (*ipn.ServeConfig, error) {
	if f.getServeCfg != nil {
		return f.getServeCfg(ctx)
	}
	return &ipn.ServeConfig{}, nil
}

func (f *fakeClient) SetServeConfig(ctx context.Context, cfg *ipn.ServeConfig) error {
	atomic.AddInt32(&f.setServeCfgCalls, 1)
	f.mu.Lock()
	f.setServeConfigArg = cfg
	f.mu.Unlock()
	if f.setServeCfg != nil {
		return f.setServeCfg(ctx, cfg)
	}
	return nil
}

func (f *fakeClient) StartLoginInteractive(ctx context.Context) error {
	atomic.AddInt32(&f.startLoginCalls, 1)
	if f.startLogin != nil {
		return f.startLogin(ctx)
	}
	return nil
}

func (f *fakeClient) WatchIPNBus(ctx context.Context, mask ipn.NotifyWatchOpt) (BusWatcher, error) {
	atomic.AddInt32(&f.watchBusCalls, 1)
	if f.watchBus != nil {
		return f.watchBus(ctx)
	}
	return newFakeBusWatcher(), nil
}

type fakeSystemd struct {
	enableCalls  int32
	disableCalls int32
	enableErr    error
	disableErr   error
}

func (f *fakeSystemd) EnableTailscaled(ctx context.Context) error {
	atomic.AddInt32(&f.enableCalls, 1)
	return f.enableErr
}

func (f *fakeSystemd) DisableTailscaled(ctx context.Context) error {
	atomic.AddInt32(&f.disableCalls, 1)
	return f.disableErr
}

// waitFor polls fn until it returns true or the deadline elapses.
// Used to coordinate with the bus-watcher goroutine in tests.
func waitFor(t *testing.T, d time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition not met within %v", d)
}

func TestStartStopIdempotent(t *testing.T) {
	fc := &fakeClient{
		watchBus: func(ctx context.Context) (BusWatcher, error) {
			return newFakeBusWatcher().bind(ctx), nil
		},
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))
	ctx := context.Background()
	m.Start(ctx)
	m.Start(ctx) // must be a no-op
	m.Start(ctx)
	// Wait until the goroutine has actually entered WatchIPNBus before
	// asserting.
	waitFor(t, time.Second, func() bool {
		return atomic.LoadInt32(&fc.watchBusCalls) >= 1
	})
	m.Stop()
	m.Stop() // must not panic or deadlock
	if got := atomic.LoadInt32(&fc.watchBusCalls); got != 1 {
		t.Fatalf("expected exactly one WatchIPNBus call, got %d", got)
	}
}

func TestStopBeforeStart(t *testing.T) {
	m := New()
	m.Stop() // must not panic
}

func TestEnableNoExistingIdentityKicksLogin(t *testing.T) {
	bus := newFakeBusWatcher()
	fc := &fakeClient{
		statusFn: func(ctx context.Context) (*ipnstate.Status, error) {
			return &ipnstate.Status{BackendState: ipn.NeedsLogin.String()}, nil
		},
		watchBus: func(ctx context.Context) (BusWatcher, error) { return bus.bind(ctx), nil },
	}
	sd := &fakeSystemd{}
	m := New(WithLocalClient(fc), WithSystemdActor(sd))
	m.Start(context.Background())
	defer m.Stop()

	// Arrange: emit BrowseToURL after a tiny delay so Enable's
	// waitForLoginURL has something to find.
	go func() {
		time.Sleep(20 * time.Millisecond)
		url := "https://login.tailscale.com/a/abc"
		bus.emit(ipn.Notify{BrowseToURL: &url})
	}()

	if err := m.Enable(context.Background()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if got := atomic.LoadInt32(&sd.enableCalls); got != 1 {
		t.Fatalf("EnableTailscaled calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&fc.startLoginCalls); got != 1 {
		t.Fatalf("StartLoginInteractive calls: got %d want 1", got)
	}
	st := m.Status(context.Background())
	if st.LoginURL == "" {
		t.Fatal("expected LoginURL to be populated after Enable")
	}
	if !st.LoginInProgress {
		t.Fatal("expected LoginInProgress=true after Enable")
	}
}

func TestEnableExistingIdentitySkipsLogin(t *testing.T) {
	fc := &fakeClient{
		statusFn: func(ctx context.Context) (*ipnstate.Status, error) {
			return &ipnstate.Status{BackendState: ipn.Starting.String()}, nil
		},
	}
	sd := &fakeSystemd{}
	m := New(WithLocalClient(fc), WithSystemdActor(sd))
	if err := m.Enable(context.Background()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startLoginCalls); got != 0 {
		t.Fatalf("StartLoginInteractive must not be called when already authenticated, got %d", got)
	}
	// WantRunning=true must still have been issued.
	if got := atomic.LoadInt32(&fc.editPrefsCalls); got == 0 {
		t.Fatal("expected EditPrefs(WantRunning=true) to be called")
	}
}

func TestEnableWantRunningFailureIsFatal(t *testing.T) {
	fc := &fakeClient{
		statusFn: func(ctx context.Context) (*ipnstate.Status, error) {
			return &ipnstate.Status{BackendState: ipn.Starting.String()}, nil
		},
		editPrefs: func(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error) {
			return nil, errors.New("permission denied")
		},
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))
	err := m.Enable(context.Background())
	if err == nil {
		t.Fatal("expected Enable to fail when EditPrefs(WantRunning) fails")
	}
}

func TestDisableClearsTransientStateAndStopsDaemon(t *testing.T) {
	fc := &fakeClient{}
	sd := &fakeSystemd{}
	m := New(WithLocalClient(fc), WithSystemdActor(sd))

	// Seed state as if a login were in progress.
	m.mu.Lock()
	m.browseURL = "https://login.tailscale.com/x"
	m.urlSetAt = time.Now()
	m.loginInProg = true
	m.policyErr = "stale error"
	m.enableEpoch = 7
	m.mu.Unlock()

	if err := m.Disable(context.Background()); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if got := atomic.LoadInt32(&sd.disableCalls); got != 1 {
		t.Fatalf("DisableTailscaled calls: got %d want 1", got)
	}
	m.mu.RLock()
	browseURL := m.browseURL
	loginInProg := m.loginInProg
	policyErr := m.policyErr
	enableEpoch := m.enableEpoch
	m.mu.RUnlock()
	if browseURL != "" || loginInProg || policyErr != "" || enableEpoch != 0 {
		t.Fatalf("Disable did not clear transient state: url=%q inProg=%v policyErr=%q epoch=%d",
			browseURL, loginInProg, policyErr, enableEpoch)
	}
}

func TestStatusEvictsStaleLoginURL(t *testing.T) {
	fc := &fakeClient{
		statusFn: func(ctx context.Context) (*ipnstate.Status, error) {
			return &ipnstate.Status{BackendState: ipn.NeedsLogin.String()}, nil
		},
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))

	// Cached URL older than loginURLMaxAge.
	m.mu.Lock()
	m.browseURL = "https://login.tailscale.com/old"
	m.urlSetAt = time.Now().Add(-2 * loginURLMaxAge)
	m.mu.Unlock()

	st := m.Status(context.Background())
	if st.LoginURL != "" {
		t.Fatalf("stale URL leaked into Status: %q", st.LoginURL)
	}
	// Confirm the manager dropped the stale URL too, not just the
	// returned snapshot.
	m.mu.RLock()
	leftover := m.browseURL
	m.mu.RUnlock()
	if leftover != "" {
		t.Fatalf("stale URL not evicted from manager state: %q", leftover)
	}
}

func TestStatusOnDaemonUnreachable(t *testing.T) {
	fc := &fakeClient{
		statusFn: func(ctx context.Context) (*ipnstate.Status, error) {
			return nil, errors.New("dial unix: connection refused")
		},
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))
	// Pre-populate cached state that should be suppressed.
	m.mu.Lock()
	m.browseURL = "https://login.tailscale.com/x"
	m.urlSetAt = time.Now()
	m.loginInProg = true
	m.policyErr = "previous failure"
	m.mu.Unlock()

	st := m.Status(context.Background())
	if st.DaemonRunning {
		t.Fatal("expected DaemonRunning=false")
	}
	if st.LoginURL != "" || st.LoginInProgress || st.PolicyError != "" {
		t.Fatalf("expected derived fields suppressed when daemon unreachable: %+v", st)
	}
}

func TestApplyDevicePolicyRunsOncePerEnable(t *testing.T) {
	bus := newFakeBusWatcher()
	fc := &fakeClient{
		statusFn: func(ctx context.Context) (*ipnstate.Status, error) {
			return &ipnstate.Status{BackendState: ipn.NeedsLogin.String()}, nil
		},
		statusNoPeers: func(ctx context.Context) (*ipnstate.Status, error) {
			return &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "velocity.tailfoo.ts.net."}}, nil
		},
		watchBus: func(ctx context.Context) (BusWatcher, error) { return bus.bind(ctx), nil },
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))
	m.Start(context.Background())
	defer m.Stop()

	if err := m.Enable(context.Background()); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	running := ipn.Running
	// Two Running notifications: a network blip after a successful
	// enrolment.  Only the first should trigger applyDevicePolicy.
	bus.emit(ipn.Notify{State: &running})
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&fc.setServeCfgCalls) == 1
	})
	bus.emit(ipn.Notify{State: &running})
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&fc.setServeCfgCalls); got != 1 {
		t.Fatalf("applyDevicePolicy fired more than once per Enable: SetServeConfig calls=%d", got)
	}

	// Re-enabling should rearm the epoch and allow another apply.
	if err := m.Enable(context.Background()); err != nil {
		t.Fatalf("Enable (second): %v", err)
	}
	bus.emit(ipn.Notify{State: &running})
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&fc.setServeCfgCalls) == 2
	})
}

func TestEnableServeRetriesUntilMagicDNSReady(t *testing.T) {
	var statusCalls int32
	fc := &fakeClient{
		statusNoPeers: func(ctx context.Context) (*ipnstate.Status, error) {
			n := atomic.AddInt32(&statusCalls, 1)
			if n < 3 {
				// MagicDNS not ready yet.
				return &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: ""}}, nil
			}
			return &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "v.tailfoo.ts.net."}}, nil
		},
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))
	if err := m.enableServeWithRetry(context.Background()); err != nil {
		t.Fatalf("enableServeWithRetry: %v", err)
	}
	if got := atomic.LoadInt32(&fc.setServeCfgCalls); got != 1 {
		t.Fatalf("SetServeConfig calls: got %d want 1", got)
	}
}

func TestWatchLoopRecoversFromWatcherDisconnect(t *testing.T) {
	connectCalls := int32(0)
	bus1 := newFakeBusWatcher()
	bus2 := newFakeBusWatcher()
	fc := &fakeClient{
		watchBus: func(ctx context.Context) (BusWatcher, error) {
			n := atomic.AddInt32(&connectCalls, 1)
			switch n {
			case 1:
				return bus1.bind(ctx), nil
			case 2:
				return bus2.bind(ctx), nil
			default:
				return newFakeBusWatcher().bind(ctx), nil
			}
		},
	}
	m := New(WithLocalClient(fc), WithSystemdActor(&fakeSystemd{}))
	m.Start(context.Background())
	defer m.Stop()

	// Push a URL on the first bus, then knock it offline.
	url := "https://login.tailscale.com/first"
	bus1.emit(ipn.Notify{BrowseToURL: &url})
	waitFor(t, 1*time.Second, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.browseURL == url
	})
	bus1.failNext(errors.New("daemon restarted"))

	// Watcher should reconnect to bus2 and accept new notifications.
	url2 := "https://login.tailscale.com/second"
	waitFor(t, 5*time.Second, func() bool {
		return atomic.LoadInt32(&connectCalls) >= 2
	})
	bus2.emit(ipn.Notify{BrowseToURL: &url2})
	waitFor(t, 1*time.Second, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.browseURL == url2
	})
}
