//go:build pcap && !linux && !darwin

package lidarbench

// cpuModel is not implemented on this platform; the field is then omitted.
func cpuModel() string { return "" }
