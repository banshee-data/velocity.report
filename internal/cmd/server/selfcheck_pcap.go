//go:build pcap

package server

import (
	"context"
	"fmt"
	"net"
	"time"

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

// selfCheckLiveCapture opens a real AF_PACKET capture, installs a BPF filter,
// emits a UDP datagram, and proves that libpcap receives it. Unlike Version or
// FindAllDevs, this validates the kernel privilege and packet-read path used by
// live LiDAR capture.
func selfCheckLiveCapture(r *selfCheckReport, iface string) {
	r.run("libpcap-live-capture", true, func(ctx context.Context) error {
		handle, err := pcap.OpenLive(iface, 65535, false, 100*time.Millisecond)
		if err != nil {
			return fmt.Errorf("pcap.OpenLive(%q): %w", iface, err)
		}
		defer handle.Close()

		const port = 39091
		if err := handle.SetBPFFilter(fmt.Sprintf("udp dst port %d", port)); err != nil {
			return fmt.Errorf("SetBPFFilter: %w", err)
		}

		conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			return fmt.Errorf("creating UDP sender: %w", err)
		}
		defer conn.Close()
		if _, err := conn.Write([]byte("velocity-libpcap-self-check")); err != nil {
			return fmt.Errorf("sending UDP probe: %w", err)
		}

		for {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("timed out waiting for captured UDP probe: %w", err)
			}
			data, _, err := handle.ReadPacketData()
			if err == pcap.NextErrorTimeoutExpired {
				continue
			}
			if err != nil {
				return fmt.Errorf("ReadPacketData: %w", err)
			}
			if len(data) > 0 {
				return nil
			}
		}
	})
}
