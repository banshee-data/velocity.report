package tailscale

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func managedServeConfig(target string) *ipn.ServeConfig {
	cfg := &ipn.ServeConfig{}
	cfg.SetWebHandler(&ipn.HTTPHandler{Proxy: target}, "velocity.tailfoo.ts.net", 443, "/", true, "")
	return cfg
}

func forwardsManager(cfg func() (*ipn.ServeConfig, error)) (*Manager, *atomic.Int32) {
	var calls atomic.Int32
	m := New(WithLocalClient(&fakeClient{getServeCfg: func(context.Context) (*ipn.ServeConfig, error) {
		calls.Add(1)
		return cfg()
	}}), WithServeTarget("http://127.0.0.1:8080"))
	return m, &calls
}

func TestUnmanagedServePorts(t *testing.T) {
	cases := []struct {
		name string
		cfg  func() *ipn.ServeConfig
		want []uint16
	}{
		{"empty", func() *ipn.ServeConfig { return &ipn.ServeConfig{} }, nil},
		{"only the managed handler", func() *ipn.ServeConfig { return managedServeConfig("http://127.0.0.1:8080") }, nil},
		{"managed mount proxying elsewhere", func() *ipn.ServeConfig { return managedServeConfig("http://127.0.0.1:8081") }, []uint16{8081}},
		{"TCP forward to the listener", func() *ipn.ServeConfig {
			cfg := managedServeConfig("http://127.0.0.1:8080")
			cfg.TCP = map[uint16]*ipn.TCPPortHandler{80: {TCPForward: "127.0.0.1:8080"}}
			return cfg
		}, []uint16{8080}},
		{"TLS-terminated TCP Funnel", func() *ipn.ServeConfig {
			return &ipn.ServeConfig{
				TCP:         map[uint16]*ipn.TCPPortHandler{8443: {TCPForward: "localhost:8082", TerminateTLS: "velocity.tailfoo.ts.net"}},
				AllowFunnel: map[ipn.HostPort]bool{"velocity.tailfoo.ts.net:8443": true},
			}
		}, []uint16{8082}},
		{"second web handler", func() *ipn.ServeConfig {
			cfg := managedServeConfig("http://127.0.0.1:8080")
			cfg.SetWebHandler(&ipn.HTTPHandler{Proxy: "http://localhost:8081"}, "velocity.tailfoo.ts.net", 8443, "/", true, "")
			cfg.SetWebHandler(&ipn.HTTPHandler{Proxy: "http://127.0.0.1:8080"}, "velocity.tailfoo.ts.net", 443, "/lidar", true, "")
			return cfg
		}, []uint16{8080, 8081}},
		{"foreground session at the managed address", func() *ipn.ServeConfig {
			return &ipn.ServeConfig{Foreground: map[string]*ipn.ServeConfig{"session": managedServeConfig("http://127.0.0.1:8080")}}
		}, []uint16{8080}},
		{"Tailscale Service", func() *ipn.ServeConfig {
			return &ipn.ServeConfig{Services: map[tailcfg.ServiceName]*ipn.ServiceConfig{
				"svc:viz": {TCP: map[uint16]*ipn.TCPPortHandler{50051: {TCPForward: "50051"}}},
			}}
		}, []uint16{50051}},
		{"forwards that cannot reach this host", func() *ipn.ServeConfig {
			return &ipn.ServeConfig{TCP: map[uint16]*ipn.TCPPortHandler{
				22: {TCPForward: "unix:/run/app.sock"},
				23: {TCPForward: "198.51.100.7:8080"},
				24: {},
			}}
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := forwardsManager(func() (*ipn.ServeConfig, error) { return tc.cfg(), nil })
			got, err := m.UnmanagedServePorts(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for _, p := range tc.want {
				if !got[p] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestLocalTargetPort(t *testing.T) {
	self := netip.MustParseAddr("192.0.2.10")
	local := map[netip.Addr]bool{self: true}
	cases := []struct {
		target string
		port   uint16
		ok     bool
	}{
		{"8080", 8080, true},
		{"localhost:8081", 8081, true},
		{"127.0.0.1:50051", 50051, true},
		{"[::1]:8082", 8082, true},
		{"0.0.0.0:8080", 8080, true},
		{"192.0.2.10:8080", 8080, true},
		{"velocity.local:8080", 8080, true},
		{"http://127.0.0.1", 80, true},
		{"http://127.0.0.1:8080/", 8080, true},
		{"https+insecure://localhost", 443, true},
		{"https://[::ffff:127.0.0.1]:8443", 8443, true},
		{"198.51.100.7:8080", 0, false},
		{"http://198.51.100.7:8080", 0, false},
		{"unix:/run/velocity.sock", 0, false},
		{"tcp://127.0.0.1", 0, false},
		{"", 0, false},
		{"localhost", 0, false},
		{"localhost:http", 0, false},
		{"://bad", 0, false},
	}
	for _, tc := range cases {
		port, ok := localTargetPort(tc.target, local)
		if port != tc.port || ok != tc.ok {
			t.Errorf("localTargetPort(%q) = %d, %v; want %d, %v", tc.target, port, ok, tc.port, tc.ok)
		}
	}
}

func TestUnmanagedServePorts_DaemonAbsentForwardsNothing(t *testing.T) {
	m, _ := forwardsManager(func() (*ipn.ServeConfig, error) {
		return nil, &net.OpError{Op: "dial", Net: "unix", Err: errors.New("connect: no such file or directory")}
	})
	got, err := m.UnmanagedServePorts(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no ports and no error", got, err)
	}
}

func TestUnmanagedServePorts_OtherFailuresAreReturned(t *testing.T) {
	m, _ := forwardsManager(func() (*ipn.ServeConfig, error) { return nil, errors.New("serve config: 500") })
	if _, err := m.UnmanagedServePorts(context.Background()); err == nil {
		t.Fatal("a failed read must be returned so the caller fails closed")
	}
}

func TestUnmanagedServePorts_CachesAndExpires(t *testing.T) {
	var failing atomic.Bool
	m, calls := forwardsManager(func() (*ipn.ServeConfig, error) {
		if failing.Load() {
			return nil, errors.New("daemon busy")
		}
		return &ipn.ServeConfig{}, nil
	})
	clock := time.Unix(1000, 0)
	m.serveForwards.now = func() time.Time { return clock }
	ctx := context.Background()

	for range 3 {
		if _, err := m.UnmanagedServePorts(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("reads within the TTL = %d, want 1", calls.Load())
	}
	clock = clock.Add(serveForwardsTTL)
	failing.Store(true)
	if _, err := m.UnmanagedServePorts(ctx); err == nil {
		t.Fatal("expired reading was reused")
	}
	if _, err := m.UnmanagedServePorts(ctx); err == nil || calls.Load() != 2 {
		t.Fatalf("an error is reused for its own TTL: err %v, reads %d", err, calls.Load())
	}
	clock = clock.Add(serveForwardsErrorTTL)
	failing.Store(false)
	if _, err := m.UnmanagedServePorts(ctx); err != nil || calls.Load() != 3 {
		t.Fatalf("after the error TTL: err %v, reads %d", err, calls.Load())
	}
	// A clock that steps backwards does not extend a reading.
	clock = clock.Add(-time.Hour)
	if _, err := m.UnmanagedServePorts(ctx); err != nil || calls.Load() != 4 {
		t.Fatalf("after a backwards step: err %v, reads %d", err, calls.Load())
	}
}

func TestUnmanagedServePorts_ConcurrentCallersShareOneRead(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	m := New(WithLocalClient(&fakeClient{getServeCfg: func(context.Context) (*ipn.ServeConfig, error) {
		calls.Add(1)
		<-release
		return &ipn.ServeConfig{TCP: map[uint16]*ipn.TCPPortHandler{80: {TCPForward: "127.0.0.1:8080"}}}, nil
	}}))
	var wg sync.WaitGroup
	results := make(chan map[uint16]bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := m.UnmanagedServePorts(context.Background())
			if err != nil {
				t.Error(err)
			}
			results <- got
		}()
	}
	// Let the waiters queue behind the first read before releasing it.
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)
	for got := range results {
		if !got[8080] {
			t.Fatalf("a waiter saw %v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("daemon reads = %d, want 1", calls.Load())
	}

	// A waiter whose own context ends stops waiting; the read continues.
	m2 := New(WithLocalClient(&fakeClient{getServeCfg: func(ctx context.Context) (*ipn.ServeConfig, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}))
	m2.serveForwards.pending = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m2.UnmanagedServePorts(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter: %v", err)
	}
}

func TestUnmanagedServePorts_SkipsEmptyEntries(t *testing.T) {
	cfg := &ipn.ServeConfig{
		TCP: map[uint16]*ipn.TCPPortHandler{1: nil},
		Web: map[ipn.HostPort]*ipn.WebServerConfig{
			"velocity.tailfoo.ts.net:443":  nil,
			"velocity.tailfoo.ts.net:8443": {Handlers: map[string]*ipn.HTTPHandler{"/a": nil, "/b": {Path: "/srv"}, "/c": {Text: "hi"}}},
			"velocity.tailfoo.ts.net:bad":  {Handlers: map[string]*ipn.HTTPHandler{"/": {Proxy: LocalServeHTTPTarget}}},
		},
		Services:   map[tailcfg.ServiceName]*ipn.ServiceConfig{"svc:empty": nil},
		Foreground: map[string]*ipn.ServeConfig{"gone": nil},
	}
	m := New(WithLocalClient(&fakeClient{getServeCfg: func(context.Context) (*ipn.ServeConfig, error) { return cfg, nil }}))
	got, err := m.UnmanagedServePorts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Only the handler on an unparseable host-port, which cannot be the
	// managed one at :443, names a port.
	if len(got) != 1 || !got[8080] {
		t.Fatalf("got %v, want only port 8080", got)
	}
}

func TestUnmanagedServePorts_DefaultTargetIsManaged(t *testing.T) {
	m := New(WithLocalClient(&fakeClient{getServeCfg: func(context.Context) (*ipn.ServeConfig, error) {
		return managedServeConfig(LocalServeHTTPTarget), nil
	}}))
	m.serveTarget = ""
	if got, err := m.UnmanagedServePorts(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; the default target is the managed one", got, err)
	}
}

func TestLocalAddrs(t *testing.T) {
	defer func(f func() ([]net.Addr, error)) { interfaceAddrs = f }(interfaceAddrs)
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)},
			&net.IPAddr{IP: net.ParseIP("2001:db8::10")},
			&net.UnixAddr{Name: "/run/x", Net: "unix"},
		}, nil
	}
	got := localAddrs()
	if len(got) != 2 || !got[netip.MustParseAddr("192.0.2.10")] || !got[netip.MustParseAddr("2001:db8::10")] {
		t.Fatalf("got %v", got)
	}
	interfaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("no interfaces") }
	if got := localAddrs(); len(got) != 0 {
		t.Fatalf("a failed listing gave %v", got)
	}
}

func TestLocalAddrsIncludesLoopback(t *testing.T) {
	addrs := localAddrs()
	if !addrs[netip.MustParseAddr("127.0.0.1")] && !addrs[netip.MustParseAddr("::1")] {
		t.Skipf("no loopback interface address reported: %v", addrs)
	}
}
