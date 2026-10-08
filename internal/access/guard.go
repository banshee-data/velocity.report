package access

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// ServeForwards reports the local ports tailscaled forwards to through Serve
// handlers velocity.report did not install.  Port 0 stands for a target whose
// port could not be decided and matches every listener.  tailscale.Manager
// implements it.
type ServeForwards interface {
	UnmanagedServePorts(ctx context.Context) (map[uint16]bool, error)
}

// guardTimeout bounds the wait for a reading on one connection or request.
const guardTimeout = 750 * time.Millisecond

// guardLogInterval rate-limits the refusal log line per listener.
const guardLogInterval = time.Minute

// GuardListener drops connections that originate on this host while
// tailscaled forwards to the listener's port through a handler
// velocity.report did not install, or while that cannot be established.
//
// Such a forward (`tailscale serve --tcp`, a TCP Funnel, another web
// handler) arrives from tailscaled on this host and carries whatever its
// client wrote, including X-Forwarded-For or a Serve capability header.
// Neither the trust given to a local connection nor a forwarded identity can
// then be believed, so the listener fails closed for local connections until
// the forward is removed.  Connections from other hosts pass through
// unchecked: the request policy classifies them by their address.
//
// Local connections are checked off the Accept path, so a slow daemon delays
// only them.  A connection admitted before a forward is noticed stays open;
// GuardHandler checks each HTTP request on it again.
func GuardListener(l net.Listener, forwards ServeForwards, name string) net.Listener {
	if forwards == nil {
		return l
	}
	return &guardedListener{
		Listener: l,
		guard:    &guard{forwards: forwards, name: name},
		results:  make(chan acceptResult),
		done:     make(chan struct{}),
	}
}

// GuardHandler refuses, with 403, an HTTP request from this host while
// tailscaled forwards to the port it arrived on through a handler
// velocity.report did not install, or while that cannot be established.  It
// rechecks each request on a kept-alive connection that GuardListener
// admitted before the forward was noticed.
func GuardHandler(next http.Handler, forwards ServeForwards, name string) http.Handler {
	if forwards == nil {
		return next
	}
	g := &guard{forwards: forwards, name: name}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
		remote, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil || !g.admit(r.Context(), local, net.TCPAddrFromAddrPort(remote)) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(struct {
				Error string `json:"error"`
			}{"unmanaged_serve_forward"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type guard struct {
	forwards ServeForwards
	name     string

	logMu  sync.Mutex
	logged time.Time
}

// admit reports whether a connection between these addresses may proceed.
func (g *guard) admit(ctx context.Context, localAddr, remoteAddr net.Addr) bool {
	remote, rok := connAddr(remoteAddr)
	local, lok := connAddr(localAddr)
	if !rok || !lok {
		g.refuse("connection without a TCP address")
		return false
	}
	if !fromThisHost(local, remote) {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()
	ports, err := g.forwards.UnmanagedServePorts(ctx)
	if err != nil {
		g.refuse("cannot read tailscaled's Serve configuration (" + err.Error() + ")")
		return false
	}
	if ports[local.Port()] || ports[0] {
		g.refuse("tailscaled forwards to this port through a Serve handler velocity.report did not install (see `tailscale serve status`)")
		return false
	}
	return true
}

func (g *guard) refuse(reason string) {
	g.logMu.Lock()
	defer g.logMu.Unlock()
	if now := time.Now(); now.Sub(g.logged) >= guardLogInterval {
		g.logged = now
		log.Printf("access: %s refusing connections from this host: %s", g.name, reason)
	}
}

func fromThisHost(local, remote netip.AddrPort) bool {
	return remote.Addr().IsLoopback() || remote.Addr() == local.Addr()
}

func connAddr(a net.Addr) (netip.AddrPort, bool) {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || tcp == nil {
		return netip.AddrPort{}, false
	}
	ap := tcp.AddrPort()
	return netip.AddrPortFrom(ap.Addr().Unmap().WithZone(""), ap.Port()), ap.IsValid()
}

type acceptResult struct {
	conn net.Conn
	err  error
}

type guardedListener struct {
	net.Listener
	guard *guard

	start     sync.Once
	results   chan acceptResult
	done      chan struct{}
	closeOnce sync.Once
}

func (g *guardedListener) Accept() (net.Conn, error) {
	g.start.Do(func() { go g.acceptLoop() })
	select {
	case r := <-g.results:
		return r.conn, r.err
	case <-g.done:
		return nil, net.ErrClosed
	}
}

func (g *guardedListener) Close() error {
	g.closeOnce.Do(func() { close(g.done) })
	return g.Listener.Close()
}

// acceptLoop hands connections from other hosts over in order, and checks
// each local connection in its own goroutine.  Errors pass through, so the
// server sees and handles them as it would without the guard.
func (g *guardedListener) acceptLoop() {
	for {
		c, err := g.Listener.Accept()
		if err != nil {
			if !g.deliver(acceptResult{err: err}) || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		local, lok := connAddr(c.LocalAddr())
		remote, rok := connAddr(c.RemoteAddr())
		if lok && rok && !fromThisHost(local, remote) {
			if !g.deliver(acceptResult{conn: c}) {
				_ = c.Close()
				return
			}
			continue
		}
		go func() {
			if g.guard.admit(context.Background(), c.LocalAddr(), c.RemoteAddr()) && g.deliver(acceptResult{conn: c}) {
				return
			}
			_ = c.Close()
		}()
	}
}

func (g *guardedListener) deliver(r acceptResult) bool {
	select {
	case g.results <- r:
		return true
	case <-g.done:
		return false
	}
}
