//go:build pcap
// +build pcap

package pcapanalyse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestRun_BadOutputDirWithValidFile(t *testing.T) {
	// An existing (non-PCAP) file passes the os.Stat check; the bad output dir
	// then trips the MkdirAll branch.
	f := filepath.Join(t.TempDir(), "exists")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	if code := Run(Config{PCAPFile: f, OutputDir: fileAsDir(t)}); code != 1 {
		t.Errorf("Run bad output dir = %d, want 1", code)
	}
}

func TestRun_CorruptPCAP(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "junk.pcap")
	_ = os.WriteFile(junk, []byte("not a pcap file"), 0o644)

	cfg := Config{PCAPFile: junk, OutputDir: t.TempDir(), UDPPort: testPCAPPort}
	if code := Run(cfg); code != 1 {
		t.Errorf("Run corrupt pcap = %d, want 1", code)
	}
	cfg.Benchmark = true
	cfg.Quiet = true
	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 1 {
		t.Errorf("Run corrupt pcap (benchmark) = %d, want 1", code)
	}
}

func TestRun_BenchmarkWithDB(t *testing.T) {
	cfg := baseConfig(t, testCapture(t, 1200))
	cfg.Benchmark = true
	cfg.Quiet = true
	cfg.DBPath = filepath.Join(t.TempDir(), "bench.db")
	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 0 {
		t.Fatalf("benchmark+DB = %d, want 0", code)
	}
	if _, err := os.Stat(cfg.DBPath); err != nil {
		t.Errorf("expected DB written: %v", err)
	}
}

func TestCollectTrackResults_ClassifyBranch(t *testing.T) {
	// Unclassified track with enough observations triggers ClassifyAndUpdate.
	tracks := map[string]*l5tracks.TrackedObject{
		"t1": {TrackID: "t1", TrackMeasurement: l5tracks.TrackMeasurement{
			TrackState: l5tracks.TrackConfirmed, ObjectClass: "", ObservationCount: 9,
		}},
	}
	r := newResult()
	collectTrackResults(makeFrameBuilder(tracks), r)
	if r.TotalTracks != 1 {
		t.Errorf("TotalTracks = %d, want 1", r.TotalTracks)
	}
}

func TestHandleBenchmarkOutput_NoRegression(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "base.json")
	baseline := BenchmarkResult{Metrics: PerformanceMetrics{
		WallClockMs: 100, FrameTimeStats: FrameTimeStats{AvgMs: 10, P95Ms: 20},
		FramesPerSecond: 100, HeapAllocBytes: 1000, ClusterTimeMs: 10, TrackingTimeMs: 10,
	}}
	data, _ := json.Marshal(baseline)
	_ = os.WriteFile(baselinePath, data, 0o644)

	cfg := Config{PCAPFile: "cap.pcap", OutputDir: dir, CompareBaseline: baselinePath}
	// Better metrics -> improvements, no regression -> the comparison != nil branch.
	better := &PerformanceMetrics{WallClockMs: 50, FrameTimeStats: FrameTimeStats{AvgMs: 5, P95Ms: 10}, FramesPerSecond: 200, HeapAllocBytes: 500, ClusterTimeMs: 5, TrackingTimeMs: 5}
	var code int
	captureOutput(t, func() { code = handleBenchmarkOutput(cfg, sampleResult(), better) })
	if code != 0 {
		t.Errorf("no regression = %d, want 0", code)
	}
}
