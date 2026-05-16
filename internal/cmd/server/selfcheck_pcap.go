//go:build pcap

package server

import (
	"context"
	"fmt"

	"github.com/google/gopacket/pcap"
)

// selfCheckLibpcap verifies that libpcap.a is correctly linked into
// this binary. We don't require root: pcap.Version() proves linkage
// without any syscalls; pcap.FindAllDevs() exercises the actual API
// surface but is allowed to return zero devices in unprivileged
// containers.
func selfCheckLibpcap(r *selfCheckReport) {
	r.run("libpcap-version", true, func(_ context.Context) error {
		v := pcap.Version()
		if v == "" {
			return fmt.Errorf("pcap.Version() returned empty string")
		}
		fmt.Fprintf(r.out, "       %s\n", v)
		return nil
	})

	// Best-effort: per-device ioctls (ETHTOOL_GLINK and friends) can fail
	// under qemu user-mode emulation and in locked-down containers without
	// indicating a real linkage problem. pcap.Version() above is the
	// authoritative "libpcap.a is linked and callable" check.
	r.run("libpcap-find-devs", false, func(_ context.Context) error {
		_, err := pcap.FindAllDevs()
		if err != nil {
			return fmt.Errorf("pcap.FindAllDevs: %w", err)
		}
		return nil
	})
}
