//go:build pcap

package lidarbench

import (
	"bufio"
	"io"
	"strings"
)

// parseCPUInfo returns the processor model named in a Linux /proc/cpuinfo.
//
// An x86 kernel names it on every core's "model name" line, and the first is
// kept. A Raspberry Pi's arm64 kernel has no such line and names the board on
// a closing "Model" line instead, which says more about the device than the
// core type would. Nothing else in the file is read: it also carries the
// board's serial number, which a committed baseline has no use for.
func parseCPUInfo(r io.Reader) string {
	var board string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "model name":
			if value != "" {
				return value
			}
		case "Model":
			if board == "" {
				board = value
			}
		}
	}
	return board
}
