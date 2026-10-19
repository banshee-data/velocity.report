//go:build pcap && darwin

package lidarbench

import (
	"strings"

	"golang.org/x/sys/unix"
)

// cpuModel names the host's processor, such as "Apple M1 Pro", and returns ""
// when the kernel does not report one.
func cpuModel() string {
	s, err := unix.Sysctl("machdep.cpu.brand_string")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}
