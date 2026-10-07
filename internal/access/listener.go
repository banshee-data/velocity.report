package access

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// ValidateLoopbackListen rejects wildcard/DNS bindings except literal localhost.
// Startup containment must not depend on an arbitrary hostname's current DNS answer.
func ValidateLoopbackListen(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid loopback listener %q: %w", address, err)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 0 || p > 65535 {
		return fmt.Errorf("invalid listener port %q", port)
	}
	if host == "localhost" {
		return nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Unmap().IsLoopback() {
		return fmt.Errorf("listener %q must use loopback", address)
	}
	return nil
}
