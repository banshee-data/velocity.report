//go:build pcap
// +build pcap

package settlingeval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

func truncatedCapture(t *testing.T, maxPackets int) string {
	t.Helper()
	src := filepath.Join("..", "perf", "pcap", "kirk0.pcapng")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("perf capture unavailable: %v", err)
	}
	h, err := pcap.OpenOffline(src)
	if err != nil {
		t.Skipf("open: %v", err)
	}
	defer h.Close()
	_ = h.SetBPFFilter("udp port 2369")
	dst := filepath.Join(t.TempDir(), "trunc.pcap")
	f, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	defer f.Close()
	w := pcapgo.NewWriterNanos(f)
	if err := w.WriteFileHeader(65536, h.LinkType()); err != nil {
		t.Fatalf("write pcap header: %v", err)
	}
	n := 0
	for p := range gopacket.NewPacketSource(h, h.LinkType()).Packets() {
		if n >= maxPackets {
			break
		}
		_ = w.WritePacket(p.Metadata().CaptureInfo, p.Data())
		n++
	}
	if n == 0 {
		t.Skip("no packets")
	}
	return dst
}

func TestRun_Integration(t *testing.T) {
	report, err := Run(truncatedCapture(t, 3000), "", "test-sensor", 2369)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report == nil {
		t.Fatal("nil report")
	}
	if report.TotalFrames == 0 {
		t.Error("expected frames processed (kirk0 yields frames via the l2frames builder)")
	}
}

func TestRun_CorruptPCAP(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "junk.pcap")
	if err := os.WriteFile(junk, []byte("not a pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(junk, "", "s", 2369); err == nil {
		t.Error("expected error replaying a corrupt pcap")
	}
}
