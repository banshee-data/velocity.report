//go:build pcap && linux

package lidarbench

import "os"

// cpuModel names the host's processor, or the board on a Raspberry Pi, and
// returns "" when /proc/cpuinfo cannot be read or names neither.
func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	return parseCPUInfo(f)
}
