//go:build pcap
// +build pcap

package pcapanalyse

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

const testPCAPPort = 2369

// testCapture returns a small truncated copy of the lidar_20Hz perf capture
// (the first maxPackets UDP/2369 packets), so the full pipeline runs in well
// under a second. It skips the test if the perf capture is unavailable.
func testCapture(t *testing.T, maxPackets int) string {
	t.Helper()
	src := filepath.Join("..", "perf", "pcap", "lidar_20Hz.pcapng")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("perf capture not available: %v", err)
	}
	h, err := pcap.OpenOffline(src)
	if err != nil {
		t.Skipf("open %s: %v", src, err)
	}
	defer h.Close()
	if err := h.SetBPFFilter("udp port 2369"); err != nil {
		t.Fatalf("bpf: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "trunc.pcap")
	f, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	w := pcapgo.NewWriterNanos(f)
	if err := w.WriteFileHeader(65536, h.LinkType()); err != nil {
		t.Fatalf("header: %v", err)
	}
	n := 0
	for p := range gopacket.NewPacketSource(h, h.LinkType()).Packets() {
		if n >= maxPackets {
			break
		}
		if err := w.WritePacket(p.Metadata().CaptureInfo, p.Data()); err != nil {
			t.Fatalf("write: %v", err)
		}
		n++
	}
	if n == 0 {
		t.Skip("no packets in capture")
	}
	return dst
}

// captureOutput redirects stdout while fn runs (Run and the print helpers are
// chatty) and restores the log destination afterwards (Run mutates it globally
// in stats/benchmark mode).
func captureOutput(t *testing.T, fn func()) {
	t.Helper()
	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() {
		os.Stdout = old
		_ = devnull.Close()
		log.SetOutput(os.Stderr)
	}()
	fn()
}

func baseConfig(t *testing.T, pcapFile string) Config {
	return Config{
		PCAPFile:  pcapFile,
		OutputDir: t.TempDir(),
		SensorID:  "hesai-pandar40p",
		UDPPort:   testPCAPPort,
		FrameRate: 10.0,
	}
}

func TestRun_NormalMode(t *testing.T) {
	cfg := baseConfig(t, testCapture(t, 1500))
	cfg.ExportJSON = true
	cfg.ExportCSV = true
	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 0 {
		t.Fatalf("Run normal = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "trunc_analysis.json")); err != nil {
		t.Errorf("expected analysis JSON: %v", err)
	}
}

func TestRun_StatsModes(t *testing.T) {
	pcapFile := testCapture(t, 1500)
	for _, mode := range []string{"stats", "stats10s", "motion"} {
		cfg := baseConfig(t, pcapFile)
		switch mode {
		case "stats":
			cfg.Stats = true
		case "stats10s":
			cfg.Stats10s = true
		case "motion":
			cfg.Motion = true
		}
		var code int
		captureOutput(t, func() { code = Run(cfg) })
		if code != 0 {
			t.Errorf("Run %s = %d, want 0", mode, code)
		}
	}
}

func TestRun_Benchmark(t *testing.T) {
	cfg := baseConfig(t, testCapture(t, 1500))
	cfg.Benchmark = true
	cfg.Quiet = true
	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 0 {
		t.Fatalf("Run benchmark = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "trunc_benchmark.json")); err != nil {
		t.Errorf("expected benchmark JSON: %v", err)
	}
}

func TestRun_BenchmarkCompareBaseline(t *testing.T) {
	pcapFile := testCapture(t, 1500)
	baseline := filepath.Join(t.TempDir(), "baseline.json")

	cfg := baseConfig(t, pcapFile)
	cfg.Benchmark = true
	cfg.Quiet = true
	cfg.BenchmarkOutput = baseline
	captureOutput(t, func() { Run(cfg) })
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("baseline not written: %v", err)
	}

	cfg2 := baseConfig(t, pcapFile)
	cfg2.Benchmark = true
	cfg2.CompareBaseline = baseline
	var code int
	captureOutput(t, func() { code = Run(cfg2) })
	if code != 0 && code != 1 { // 1 if timing drift trips the regression gate
		t.Errorf("Run compare = %d, want 0 or 1", code)
	}
}

func TestRun_Errors(t *testing.T) {
	if code := Run(Config{}); code != 1 {
		t.Errorf("Run empty pcap = %d, want 1", code)
	}
	if code := Run(Config{PCAPFile: "/nonexistent/x.pcap"}); code != 1 {
		t.Errorf("Run nonexistent = %d, want 1", code)
	}
}
