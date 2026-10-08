package access

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubForwards struct {
	mu    sync.Mutex
	ports map[uint16]bool
	err   error
	block chan struct{}
	calls atomic.Int32
}

func (s *stubForwards) UnmanagedServePorts(ctx context.Context) (map[uint16]bool, error) {
	s.calls.Add(1)
	s.mu.Lock()
	block := s.block
	s.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ports, s.err
}

func (s *stubForwards) set(ports map[uint16]bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ports, s.err = ports, err
}

// serveGreeting accepts on l and writes "ok" to each admitted connection.
func serveGreeting(t *testing.T, l net.Listener) {
	t.Helper()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("ok"))
			_ = c.Close()
		}
	}()
}

func greeting(t *testing.T, addr string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	b, _ := io.ReadAll(c)
	return string(b)
}

func TestGuardListenerRefusesLocalConnectionsWhileAForwardTargetsThePort(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(raw.Addr().(*net.TCPAddr).Port)
	f := &stubForwards{ports: map[uint16]bool{}}
	l := GuardListener(raw, f, "test listener")
	defer l.Close()
	serveGreeting(t, l)

	if got := greeting(t, raw.Addr().String()); got != "ok" {
		t.Fatalf("no forward: got %q", got)
	}
	f.set(map[uint16]bool{port + 1: true}, nil)
	if got := greeting(t, raw.Addr().String()); got != "ok" {
		t.Fatalf("forward to another port: got %q", got)
	}
	f.set(map[uint16]bool{port: true}, nil)
	if got := greeting(t, raw.Addr().String()); got != "" {
		t.Fatalf("forward to this port: got %q, want the connection dropped", got)
	}
	f.set(map[uint16]bool{0: true}, nil)
	if got := greeting(t, raw.Addr().String()); got != "" {
		t.Fatalf("forward to an undecided port: got %q, want the connection dropped", got)
	}
	f.set(nil, errors.New("daemon busy"))
	if got := greeting(t, raw.Addr().String()); got != "" {
		t.Fatalf("unreadable Serve configuration: got %q, want the connection dropped", got)
	}
	// The listener keeps serving once the forward is gone.
	f.set(map[uint16]bool{}, nil)
	if got := greeting(t, raw.Addr().String()); got != "ok" {
		t.Fatalf("after removal: got %q", got)
	}
}

type fakeConn struct {
	net.Conn
	local, remote net.Addr
	closed        atomic.Bool
}

func (c *fakeConn) LocalAddr() net.Addr  { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr { return c.remote }
func (c *fakeConn) Close() error         { c.closed.Store(true); return nil }

type fakeAccept struct {
	conn *fakeConn
	err  error
}

type fakeListener struct {
	mu      sync.Mutex
	accepts []fakeAccept
	closed  chan struct{}
}

func newFakeListener(accepts ...fakeAccept) *fakeListener {
	return &fakeListener{accepts: accepts, closed: make(chan struct{})}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.accepts) == 0 {
		l.mu.Unlock()
		<-l.closed
		return nil, net.ErrClosed
	}
	a := l.accepts[0]
	l.accepts = l.accepts[1:]
	l.mu.Unlock()
	if a.err != nil {
		return nil, a.err
	}
	return a.conn, nil
}

func (l *fakeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}
func (l *fakeListener) Addr() net.Addr { return &net.TCPAddr{} }

func tcp(ip string, port int) *net.TCPAddr { return &net.TCPAddr{IP: net.ParseIP(ip), Port: port} }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGuardListenerChecksOnlyConnectionsFromThisHost(t *testing.T) {
	f := &stubForwards{ports: map[uint16]bool{80: true}}
	lan := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.50", 40000)}
	self := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.1", 40001)}
	mapped := &fakeConn{local: tcp("::ffff:192.0.2.1", 80), remote: tcp("::ffff:192.0.2.1", 40002)}
	loop6 := &fakeConn{local: tcp("::1", 80), remote: tcp("::1", 40003)}
	zoned := &fakeConn{local: &net.TCPAddr{IP: net.ParseIP("fe80::1"), Port: 80, Zone: "eth0"}, remote: &net.TCPAddr{IP: net.ParseIP("fe80::1"), Port: 40004}}
	odd := &fakeConn{local: &net.UnixAddr{Name: "x", Net: "unix"}, remote: &net.UnixAddr{Name: "y", Net: "unix"}}
	tailnet := &fakeConn{local: tcp("100.100.1.1", 80), remote: tcp("100.100.1.2", 40005)}
	raw := newFakeListener(fakeAccept{conn: self}, fakeAccept{conn: mapped}, fakeAccept{conn: loop6},
		fakeAccept{conn: zoned}, fakeAccept{conn: odd}, fakeAccept{conn: lan}, fakeAccept{conn: tailnet})
	l := GuardListener(raw, f, "test")

	for _, want := range []*fakeConn{lan, tailnet} {
		got, err := l.Accept()
		if err != nil || got != want {
			t.Fatalf("admitted %v, %v; want %v", got, err, want.remote)
		}
	}
	for _, c := range []*fakeConn{self, mapped, loop6, zoned, odd} {
		eventually(t, "refusal of "+c.remote.String(), c.closed.Load)
	}
	if n := f.calls.Load(); n != 4 {
		t.Fatalf("readings = %d, want one per TCP connection from this host", n)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed listener: %v", err)
	}
}

// A slow reading delays only the local connection waiting on it.
func TestGuardListenerDoesNotHoldOtherHostsBehindASlowReading(t *testing.T) {
	f := &stubForwards{ports: map[uint16]bool{}, block: make(chan struct{})}
	local := &fakeConn{local: tcp("127.0.0.1", 80), remote: tcp("127.0.0.1", 40000)}
	lan := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.50", 40001)}
	l := GuardListener(newFakeListener(fakeAccept{conn: local}, fakeAccept{conn: lan}), f, "test")
	defer l.Close()

	if got, err := l.Accept(); err != nil || got != lan {
		t.Fatalf("first admitted %v, %v; want the LAN connection", got, err)
	}
	close(f.block)
	if got, err := l.Accept(); err != nil || got != local {
		t.Fatalf("second admitted %v, %v; want the local connection", got, err)
	}
}

type temporaryError struct{}

func (temporaryError) Error() string   { return "accept: too many open files" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

// Accept errors reach the server as they would without the guard, and the
// guard keeps accepting after one.
func TestGuardListenerPassesAcceptErrorsThrough(t *testing.T) {
	lan := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.50", 40001)}
	l := GuardListener(newFakeListener(fakeAccept{err: temporaryError{}}, fakeAccept{conn: lan}), &stubForwards{}, "test")
	defer l.Close()
	if _, err := l.Accept(); !errors.As(err, new(temporaryError)) {
		t.Fatalf("first Accept: %v", err)
	}
	if got, err := l.Accept(); err != nil || got != lan {
		t.Fatalf("second Accept: %v, %v", got, err)
	}
}

func TestGuardListenerCloseUnblocksAccept(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := GuardListener(raw, &stubForwards{}, "test")
	errc := make(chan error, 1)
	go func() {
		_, err := l.Accept()
		errc <- err
	}()
	time.Sleep(10 * time.Millisecond)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept after Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not return after Close")
	}
}

// A connection closed by the guard while its delivery waits is not leaked.
func TestGuardListenerClosesAnAdmittedConnectionNobodyAccepts(t *testing.T) {
	local := &fakeConn{local: tcp("127.0.0.1", 80), remote: tcp("127.0.0.1", 40000)}
	f := &stubForwards{ports: map[uint16]bool{}, block: make(chan struct{})}
	raw := newFakeListener(fakeAccept{conn: local})
	l := GuardListener(raw, f, "test")
	go func() { _, _ = l.Accept() }()
	eventually(t, "the reading to start", func() bool { return f.calls.Load() == 1 })
	_ = l.Close()
	close(f.block)
	eventually(t, "the undelivered connection to close", local.closed.Load)
}

// So is one from another host that is waiting to be handed over at Close.
func TestGuardListenerClosesAnUndeliveredConnectionAtClose(t *testing.T) {
	first := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.50", 40000)}
	second := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.51", 40001)}
	l := GuardListener(newFakeListener(fakeAccept{conn: first}, fakeAccept{conn: second}), &stubForwards{}, "test")
	if got, err := l.Accept(); err != nil || got != first {
		t.Fatalf("first Accept: %v, %v", got, err)
	}
	_ = l.Close()
	eventually(t, "the undelivered connection to close", second.closed.Load)
}

func TestGuardsWithoutForwardsAreUnchanged(t *testing.T) {
	raw := newFakeListener()
	if GuardListener(raw, nil, "test") != net.Listener(raw) {
		t.Fatal("a nil ServeForwards must leave the listener as it was")
	}
	next := http.NotFoundHandler()
	if h := GuardHandler(next, nil, "test"); h == nil {
		t.Fatal("nil handler")
	}
}

func TestGuardHandlerRechecksEachRequestFromThisHost(t *testing.T) {
	f := &stubForwards{ports: map[uint16]bool{}}
	served := 0
	h := GuardHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served++ }), f, "test")
	request := func(local net.Addr, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/config", nil)
		if local != nil {
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, local))
		}
		r.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	if rec := request(tcp("127.0.0.1", 8080), "127.0.0.1:40000"); rec.Code != 200 || served != 1 {
		t.Fatalf("no forward: HTTP %d, served %d", rec.Code, served)
	}
	f.set(map[uint16]bool{8080: true}, nil)
	rec := request(tcp("127.0.0.1", 8080), "127.0.0.1:40000")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "unmanaged_serve_forward") || served != 1 {
		t.Fatalf("forward to this port: HTTP %d %q, served %d", rec.Code, rec.Body.String(), served)
	}
	calls := f.calls.Load()
	if rec := request(tcp("192.0.2.1", 8080), "192.0.2.50:40000"); rec.Code != 200 || served != 2 || f.calls.Load() != calls {
		t.Fatalf("another host: HTTP %d, served %d, readings %d", rec.Code, served, f.calls.Load()-calls)
	}
	if rec := request(nil, "127.0.0.1:40000"); rec.Code != http.StatusForbidden {
		t.Fatalf("no local address: HTTP %d", rec.Code)
	}
	if rec := request(tcp("127.0.0.1", 8080), "not-an-address"); rec.Code != http.StatusForbidden {
		t.Fatalf("unparseable remote address: HTTP %d", rec.Code)
	}
}

func TestGuardRateLimitsItsLog(t *testing.T) {
	g := &guard{name: "test"}
	g.refuse("first")
	first := g.logged
	g.refuse("second")
	if g.logged != first {
		t.Fatal("a second refusal within the interval logged again")
	}
	g.logged = first.Add(-guardLogInterval)
	g.refuse("third")
	if !g.logged.After(first) {
		t.Fatal("a refusal after the interval did not log")
	}
}
