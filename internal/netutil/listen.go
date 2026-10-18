// Package netutil holds small networking helpers shared by the servers.
package netutil

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

// ListenTCPWithFallback binds addr, and when another process already holds it
// binds a free port on the same host instead. fellBack reports which happened,
// so the caller can log the address it actually got: a client that dials the
// configured port is talking to the other process.
//
// Only an address already in use falls back. Any other failure — a host that
// does not resolve, a privileged port — is returned as it is, because a second
// port would not fix it.
func ListenTCPWithFallback(addr string) (ln net.Listener, fellBack bool, err error) {
	ln, err = net.Listen("tcp", addr)
	if err == nil {
		return ln, false, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, false, err
	}
	host, _, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		return nil, false, err
	}
	fallback, fbErr := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if fbErr != nil {
		return nil, false, fmt.Errorf("%w; fallback to a free port failed: %v", err, fbErr)
	}
	return fallback, true, nil
}
