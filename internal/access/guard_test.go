package access

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubForwards struct {
	mu    sync.Mutex
	ports map[uint16]bool
	err   error
	calls atomic.Int32
}

func (s *stubForwards) UnmanagedServePorts(context.Context) (map[uint16]bool, error) {
	s.calls.Add(1)
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
	closed        bool
}

func (c *fakeConn) LocalAddr() net.Addr  { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr { return c.remote }
func (c *fakeConn) Close() error         { c.closed = true; return nil }

type fakeListener struct {
	conns []*fakeConn
}

func (l *fakeListener) Accept() (net.Conn, error) {
	if len(l.conns) == 0 {
		return nil, net.ErrClosed
	}
	c := l.conns[0]
	l.conns = l.conns[1:]
	return c, nil
}
func (l *fakeListener) Close() error   { return nil }
func (l *fakeListener) Addr() net.Addr { return &net.TCPAddr{} }

func tcp(ip string, port int) *net.TCPAddr { return &net.TCPAddr{IP: net.ParseIP(ip), Port: port} }

func TestGuardListenerChecksOnlyConnectionsFromThisHost(t *testing.T) {
	f := &stubForwards{ports: map[uint16]bool{80: true}}
	lan := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.50", 40000)}
	self := &fakeConn{local: tcp("192.0.2.1", 80), remote: tcp("192.0.2.1", 40001)}
	mapped := &fakeConn{local: tcp("::ffff:192.0.2.1", 80), remote: tcp("::ffff:192.0.2.1", 40002)}
	loop6 := &fakeConn{local: tcp("::1", 80), remote: tcp("::1", 40003)}
	odd := &fakeConn{local: &net.UnixAddr{Name: "x", Net: "unix"}, remote: &net.UnixAddr{Name: "y", Net: "unix"}}
	tailnet := &fakeConn{local: tcp("100.100.1.1", 80), remote: tcp("100.100.1.2", 40004)}
	l := GuardListener(&fakeListener{conns: []*fakeConn{self, mapped, loop6, odd, lan, tailnet}}, f, "test")

	first, err := l.Accept()
	if err != nil || first != lan {
		t.Fatalf("first admitted = %v, %v; want the LAN connection", first, err)
	}
	if f.calls.Load() != 3 {
		t.Fatalf("readings = %d, want one per connection from this host", f.calls.Load())
	}
	for _, c := range []*fakeConn{self, mapped, loop6, odd} {
		if !c.closed {
			t.Fatalf("connection %v -> %v was admitted", c.remote, c.local)
		}
	}
	if second, err := l.Accept(); err != nil || second != tailnet {
		t.Fatalf("second admitted = %v, %v; want the tailnet connection", second, err)
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed listener: %v", err)
	}
}

func TestGuardListenerWithoutForwardsIsUnchanged(t *testing.T) {
	raw := &fakeListener{}
	if GuardListener(raw, nil, "test") != net.Listener(raw) {
		t.Fatal("a nil ServeForwards must leave the listener as it was")
	}
}

func TestGuardListenerRateLimitsItsLog(t *testing.T) {
	g := &guardedListener{name: "test"}
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
