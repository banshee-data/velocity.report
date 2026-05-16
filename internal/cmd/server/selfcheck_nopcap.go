//go:build !pcap

package server

// selfCheckLibpcap is a no-op when this binary was built without the
// pcap tag. The static-build path always sets -tags=pcap, so this stub
// only runs in dev-host builds where libpcap is dynamically linked or
// absent.
func selfCheckLibpcap(_ *selfCheckReport) {}
