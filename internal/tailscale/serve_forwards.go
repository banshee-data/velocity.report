package tailscale

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"tailscale.com/ipn"
)

// serveForwardsTTL bounds how long a reading of tailscaled's Serve
// configuration is reused.  A forward added out of band is noticed within
// this window, the same bound the peer-grant cache gives a revocation.
const serveForwardsTTL = 5 * time.Second

// serveForwardsErrorTTL is shorter, so a daemon that answers again is
// believed again promptly, without a lookup on every connection meanwhile.
const serveForwardsErrorTTL = time.Second

// serveForwardsTimeout bounds one read of the Serve configuration.  It is
// not tied to the caller's context: concurrent callers share the read.
const serveForwardsTimeout = 2 * time.Second

type serveForwardsResult struct {
	ports map[uint16]bool
	err   error
	at    time.Time
}

type serveForwardsCache struct {
	mu      sync.Mutex
	last    *serveForwardsResult
	pending chan struct{}
	// now is time.Now outside tests.
	now func() time.Time
}

// UnmanagedServePorts returns the local TCP ports that tailscaled sends
// traffic to through a Serve handler or TCP forward this manager did not
// install: `tailscale serve --tcp`, a TCP Funnel, another web handler, a
// foreground session or a Tailscale Service.  Traffic through any of those
// reaches the port from tailscaled on this host, carrying whatever headers
// its client wrote, so a listener on such a port cannot believe that a
// connection from this host is local or that its forwarded identity is real.
//
// Port 0 in the result stands for a target that may be this host on a port
// that could not be decided; it matches every listener.
//
// A daemon whose socket is missing or refuses connections is not running and
// forwards nothing; that answer is not cached, so a daemon that starts is read
// at once.  Any other failure is returned, and the caller must fail closed.
func (m *Manager) UnmanagedServePorts(ctx context.Context) (map[uint16]bool, error) {
	c := &m.serveForwards
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	for {
		c.mu.Lock()
		if r := c.last; r != nil {
			ttl := serveForwardsTTL
			if r.err != nil {
				ttl = serveForwardsErrorTTL
			}
			if age := now().Sub(r.at); age >= 0 && age < ttl {
				c.mu.Unlock()
				return r.ports, r.err
			}
		}
		if wait := c.pending; wait != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				continue
			}
		}
		done := make(chan struct{})
		c.pending = done
		started := now()
		c.mu.Unlock()

		readCtx, cancel := context.WithTimeout(context.Background(), serveForwardsTimeout)
		ports, absent, err := m.readUnmanagedServePorts(readCtx)
		cancel()

		c.mu.Lock()
		switch {
		case absent:
			c.last = nil
		case err != nil:
			// Stamped on completion, so a read that timed out is not
			// already expired when stored and retried by every caller.
			c.last = &serveForwardsResult{err: err, at: now()}
		default:
			// Stamped with the start of the read, so a slow read cannot
			// stretch the time a reading is believed.
			c.last = &serveForwardsResult{ports: ports, at: started}
		}
		c.pending = nil
		close(done)
		c.mu.Unlock()
		return ports, err
	}
}

// readUnmanagedServePorts reads the Serve configuration.  absent reports a
// daemon that is not running: its socket is missing or refuses connections.
// A socket this process may not open is a failure, not an absence.
func (m *Manager) readUnmanagedServePorts(ctx context.Context) (ports map[uint16]bool, absent bool, err error) {
	cfg, err := m.lc.GetServeConfig(ctx)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return map[uint16]bool{}, true, nil
		}
		return nil, false, err
	}
	local := localAddrs()
	ports = map[uint16]bool{}
	m.collectUnmanagedServePorts(cfg, true, local, ports)
	return ports, false, nil
}

// collectUnmanagedServePorts adds the local ports cfg forwards to, skipping
// only the web handler this manager installs (top-level, at :443 "/",
// proxying to its own target).  Foreground sessions and Services are never
// managed here.
func (m *Manager) collectUnmanagedServePorts(cfg *ipn.ServeConfig, top bool, local map[netip.Addr]bool, ports map[uint16]bool) {
	if cfg == nil {
		return
	}
	addTCP := func(handlers map[uint16]*ipn.TCPPortHandler) {
		for _, h := range handlers {
			if h == nil || h.TCPForward == "" {
				continue
			}
			if p, ok := localTargetPort(h.TCPForward, local); ok {
				ports[p] = true
			}
		}
	}
	addWeb := func(web map[ipn.HostPort]*ipn.WebServerConfig, managed bool) {
		for hp, w := range web {
			if w == nil {
				continue
			}
			for mount, h := range w.Handlers {
				if h == nil || h.Proxy == "" {
					continue
				}
				if managed && m.isManagedWebHandler(hp, mount, h) {
					continue
				}
				if p, ok := localTargetPort(h.Proxy, local); ok {
					ports[p] = true
				}
			}
		}
	}
	addTCP(cfg.TCP)
	addWeb(cfg.Web, top)
	for _, svc := range cfg.Services {
		if svc == nil {
			continue
		}
		addTCP(svc.TCP)
		addWeb(svc.Web, false)
	}
	for _, fg := range cfg.Foreground {
		m.collectUnmanagedServePorts(fg, false, local, ports)
	}
}

func (m *Manager) isManagedWebHandler(hp ipn.HostPort, mount string, h *ipn.HTTPHandler) bool {
	if mount != "/" {
		return false
	}
	if port, err := hp.Port(); err != nil || port != 443 {
		return false
	}
	target := m.serveTarget
	if target == "" {
		target = LocalServeHTTPTarget
	}
	return h.Proxy == target
}

// localTargetPort returns the TCP port a Serve target names, and whether the
// target may be this host.  Targets take the forms tailscaled accepts:
// "3000", "localhost:3000", "127.0.0.1:http", "http://127.0.0.1:8080/",
// "https+insecure://host" and "unix:/path".  A unix socket is not one of our
// listeners, and an address literal for another host is not ours.  A name
// other than localhost might resolve to this host, so it counts.  A target
// that may be this host on a port that cannot be decided returns port 0,
// which matches every listener: it fails closed.
func localTargetPort(target string, local map[netip.Addr]bool) (uint16, bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "unix:") {
		return 0, false
	}
	if p, err := strconv.ParseUint(target, 10, 16); err == nil {
		return uint16(p), true
	}
	host, port := target, ""
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return 0, true
		}
		host, port = u.Hostname(), u.Port()
		if port == "" {
			switch u.Scheme {
			case "http":
				port = "80"
			case "https", "https+insecure":
				port = "443"
			}
		}
	} else if h, p, err := net.SplitHostPort(target); err == nil {
		host, port = h, p
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		// Interface addresses carry no zone.
		ip = ip.Unmap().WithZone("")
		if !ip.IsLoopback() && !ip.IsUnspecified() && !local[ip] {
			return 0, false
		}
	}
	if p, err := strconv.ParseUint(port, 10, 16); err == nil {
		return uint16(p), true
	}
	// A service name, as tailscaled's dialler resolves it.
	if p, err := net.LookupPort("tcp", port); err == nil && port != "" {
		return uint16(p), true
	}
	return 0, true
}

// interfaceAddrs is net.InterfaceAddrs outside tests.
var interfaceAddrs = net.InterfaceAddrs

// localAddrs returns this host's interface addresses.  A failure leaves the
// set empty, which only narrows what an address-literal target can match:
// loopback, unspecified and named targets still count.
func localAddrs() map[netip.Addr]bool {
	out := map[netip.Addr]bool{}
	addrs, err := interfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if addr, ok := netip.AddrFromSlice(ip); ok {
			out[addr.Unmap()] = true
		}
	}
	return out
}
