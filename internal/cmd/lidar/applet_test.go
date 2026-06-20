//go:build pcap
// +build pcap

package lidar

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
	src := filepath.Join("..", "..", "lidar", "perf", "pcap", "kirk0.pcapng")
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
	f, _ := os.Create(dst)
	defer f.Close()
	w := pcapgo.NewWriterNanos(f)
	_ = w.WriteFileHeader(65536, h.LinkType())
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

func quiet(t *testing.T, fn func() int) int {
	t.Helper()
	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() {
		os.Stdout = old
		_ = devnull.Close()
	}()
	return fn()
}

func TestMain_RoutesPcapAnalyse(t *testing.T) {
	pcapFile := truncatedCapture(t, 1000)
	code := quiet(t, func() int {
		return Main([]string{"pcap-analyse", "-pcap", pcapFile, "-port", "2369", "-stats", "-csv=false", "-json=false"})
	})
	if code != 0 {
		t.Errorf("pcap-analyse = %d, want 0", code)
	}
}

func TestMain_RoutesPcapSplit(t *testing.T) {
	pcapFile := truncatedCapture(t, 1000)
	dir := t.TempDir()
	code := quiet(t, func() int {
		return Main([]string{"pcap-split", "--pcap", pcapFile, "--port", "2369", "--output", dir})
	})
	if code != 0 {
		t.Errorf("pcap-split = %d, want 0", code)
	}
}

func TestSplitMain_MissingPCAP(t *testing.T) {
	if code := SplitMain([]string{"--output", t.TempDir()}); code != 2 {
		t.Errorf("split without --pcap = %d, want 2", code)
	}
}

func TestMain_RoutesSettlingEval(t *testing.T) {
	pcapFile := truncatedCapture(t, 1000)
	out := filepath.Join(t.TempDir(), "report.json")
	code := quiet(t, func() int {
		return Main([]string{"settling-eval", "--port", "2369", "--output", out, pcapFile})
	})
	if code != 0 {
		t.Errorf("settling-eval = %d, want 0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected report written: %v", err)
	}
}

func TestSettlingEvalMain_MissingArg(t *testing.T) {
	if code := SettlingEvalMain(nil); code != 2 {
		t.Errorf("settling-eval without pcap = %d, want 2", code)
	}
}

func TestSettlingEvalMain_ReplayError(t *testing.T) {
	if code := SettlingEvalMain([]string{"/nonexistent.pcap"}); code != 1 {
		t.Errorf("settling-eval bad pcap = %d, want 1", code)
	}
}
