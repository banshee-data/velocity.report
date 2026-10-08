package access

import (
	"context"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"
)

// ServeForwards reports the local ports tailscaled forwards to through Serve
// handlers velocity.report did not install.  tailscale.Manager implements it.
type ServeForwards interface {
	UnmanagedServePorts(ctx context.Context) (map[uint16]bool, error)
}

// guardTimeout bounds the wait for a reading on one accepted connection.
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
func GuardListener(l net.Listener, forwards ServeForwards, name string) net.Listener {
	if forwards == nil {
		return l
	}
	return &guardedListener{Listener: l, forwards: forwards, name: name}
}

type guardedListener struct {
	net.Listener
	forwards ServeForwards
	name     string

	logMu  sync.Mutex
	logged time.Time
}

func (g *guardedListener) Accept() (net.Conn, error) {
	for {
		c, err := g.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if g.admit(c) {
			return c, nil
		}
		_ = c.Close()
	}
}

func (g *guardedListener) admit(c net.Conn) bool {
	remote, rok := connAddr(c.RemoteAddr())
	local, lok := connAddr(c.LocalAddr())
	if !rok || !lok {
		g.refuse("connection without a TCP address")
		return false
	}
	if !remote.Addr().IsLoopback() && remote.Addr() != local.Addr() {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
	defer cancel()
	ports, err := g.forwards.UnmanagedServePorts(ctx)
	if err != nil {
		g.refuse("cannot read tailscaled's Serve configuration (" + err.Error() + ")")
		return false
	}
	if ports[local.Port()] {
		g.refuse("tailscaled forwards to this port through a Serve handler velocity.report did not install (see `tailscale serve status`)")
		return false
	}
	return true
}

func (g *guardedListener) refuse(reason string) {
	g.logMu.Lock()
	defer g.logMu.Unlock()
	if now := time.Now(); now.Sub(g.logged) >= guardLogInterval {
		g.logged = now
		log.Printf("access: %s refusing connections from this host: %s", g.name, reason)
	}
}

func connAddr(a net.Addr) (netip.AddrPort, bool) {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		return netip.AddrPort{}, false
	}
	ap := tcp.AddrPort()
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), ap.IsValid()
}
