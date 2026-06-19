//go:build pcap
// +build pcap

package pcapsplit

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

// buildUDP serialises a minimal Ethernet/IPv4/UDP packet to the given dst port.
func buildUDP(t *testing.T, port int, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.IPv4(10, 0, 0, 1),
		DstIP:    net.IPv4(10, 0, 0, 2),
	}
	udp := &layers.UDP{SrcPort: 40000, DstPort: layers.UDPPort(port)}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatalf("checksum setup: %v", err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload)); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return buf.Bytes()
}

// writeInputPCAP writes a legacy PCAP with one UDP packet per timestamp.
func writeInputPCAP(t *testing.T, path string, port int, times []time.Time) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	defer f.Close()
	w := pcapgo.NewWriterNanos(f)
	if err := w.WriteFileHeader(65536, layers.LinkTypeEthernet); err != nil {
		t.Fatalf("file header: %v", err)
	}
	for i, ts := range times {
		data := buildUDP(t, port, []byte{byte(i)})
		ci := gopacket.CaptureInfo{Timestamp: ts, CaptureLength: len(data), Length: len(data)}
		if err := w.WritePacket(ci, data); err != nil {
			t.Fatalf("write packet %d: %v", i, err)
		}
	}
}

// countPackets re-reads a PCAP and returns the packet count and last timestamp.
func countPackets(t *testing.T, path string, port int) (int, time.Time) {
	t.Helper()
	h, err := pcap.OpenOffline(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer h.Close()
	if err := h.SetBPFFilter(fmt.Sprintf("udp port %d", port)); err != nil {
		t.Fatalf("bpf: %v", err)
	}
	src := gopacket.NewPacketSource(h, h.LinkType())
	n := 0
	var last time.Time
	for p := range src.Packets() {
		last = p.Metadata().Timestamp
		n++
	}
	return n, last
}

func TestWriteSegments_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	const port = 2369
	input := filepath.Join(dir, "in.pcap")

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	times := make([]time.Time, 10)
	for i := range times {
		times[i] = base.Add(time.Duration(i) * time.Second) // 0..9 s
	}
	writeInputPCAP(t, input, port, times)

	// static [0,5 s), motion [5,9 s]; the t=9 packet exercises the end clamp.
	periods := []MotionPeriod{
		{Type: StaticLabel, StartTime: base, EndTime: base.Add(5 * time.Second)},
		{Type: MotionLabel, StartTime: base.Add(5 * time.Second), EndTime: base.Add(9 * time.Second)},
	}
	segs := BuildSegments(periods, "out")
	cfg := SplitConfig{PCAPFile: input, OutputDir: dir, UDPPort: port}

	if err := WriteSegments(cfg, segs); err != nil {
		t.Fatalf("WriteSegments: %v", err)
	}

	const wantStatic, wantMotion = 5, 5
	if segs[0].PacketCount != wantStatic {
		t.Errorf("static PacketCount=%d, want %d", segs[0].PacketCount, wantStatic)
	}
	if segs[1].PacketCount != wantMotion {
		t.Errorf("motion PacketCount=%d, want %d", segs[1].PacketCount, wantMotion)
	}

	// Re-read each segment file and confirm the counts and no packet loss.
	n0, last0 := countPackets(t, filepath.Join(dir, segs[0].Filename), port)
	n1, _ := countPackets(t, filepath.Join(dir, segs[1].Filename), port)
	if n0 != wantStatic || n1 != wantMotion {
		t.Errorf("re-read counts %d/%d, want %d/%d", n0, n1, wantStatic, wantMotion)
	}
	if n0+n1 != len(times) {
		t.Errorf("packets lost: %d written, %d total input", n0+n1, len(times))
	}
	// Every packet in the static file must fall before the 5 s boundary.
	if !last0.Before(base.Add(5 * time.Second)) {
		t.Errorf("static file last ts %v not before boundary", last0)
	}
}

func TestWriteSegments_Empty(t *testing.T) {
	if err := WriteSegments(SplitConfig{}, nil); err != nil {
		t.Errorf("empty segments should be a no-op, got %v", err)
	}
}
