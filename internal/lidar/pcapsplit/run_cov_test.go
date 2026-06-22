//go:build pcap
// +build pcap

package pcapsplit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

// truncatedCapture returns a small (maxPackets) copy of the lidar_20Hz perf
// capture on port 2369 so the two-pass split runs fast.
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
		if err := w.WritePacket(p.Metadata().CaptureInfo, p.Data()); err != nil {
			t.Fatalf("write packet: %v", err)
		}
		n++
	}
	if n == 0 {
		t.Skip("no packets")
	}
	return dst
}

func TestRun_Integration(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = truncatedCapture(t, 3000)
	cfg.OutputDir = dir
	cfg.UDPPort = 2369
	cfg.ExportJSON = true
	cfg.ExportMetrics = true
	cfg.Verbose = true

	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	err := Run(cfg)
	os.Stdout = old
	_ = devnull.Close()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, f := range []string{"summary.txt", "segments.json", "frame_metrics.csv"} {
		if _, e := os.Stat(filepath.Join(dir, f)); e != nil {
			t.Errorf("missing %s: %v", f, e)
		}
	}
}

func TestRun_DryRunExportsMetadataWithoutPCAPs(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = truncatedCapture(t, 3000)
	cfg.OutputDir = dir
	cfg.UDPPort = 2369
	cfg.ExportJSON = true
	cfg.ExportMetrics = true
	cfg.DryRun = true

	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	err := Run(cfg)
	os.Stdout = old
	_ = devnull.Close()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, f := range []string{"summary.txt", "segments.json", "frame_metrics.csv"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".pcap" {
			t.Errorf("dry run wrote a PCAP: %s", entry.Name())
		}
	}
}

func TestRun_OutputDirError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = "x.pcap"
	cfg.OutputDir = filepath.Join(f, "sub")
	if err := Run(cfg); err == nil {
		t.Error("expected MkdirAll error")
	}
}

func TestCountingStats(t *testing.T) {
	s := &countingStats{}
	s.AddPacket(10)
	s.AddPacket(20)
	s.AddDropped()
	s.AddPoints(5)
	s.LogStats(true)
	if s.count() != 2 {
		t.Errorf("count = %d, want 2", s.count())
	}
}

func TestWriteSegments_BadOutputDir(t *testing.T) {
	pcapFile := truncatedCapture(t, 200)
	segs := BuildSegments(periodsFixture(), "out")
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = pcapFile
	cfg.OutputDir = filepath.Join(f, "sub") // segment Create fails
	cfg.UDPPort = 2369
	if err := WriteSegments(cfg, segs); err == nil {
		t.Error("expected segment-create error")
	}
}
