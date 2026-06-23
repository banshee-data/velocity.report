//go:build pcap
// +build pcap

package lidarbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFrameTimeStats(t *testing.T) {
	if got := computeFrameTimeStats(nil); got != (FrameTimeStats{}) {
		t.Fatalf("empty input = %+v, want zero value", got)
	}
	// 10 samples 1..10 ms.
	times := []float64{5, 3, 1, 9, 7, 2, 8, 4, 10, 6}
	s := computeFrameTimeStats(times)
	if s.MinMs != 1 || s.MaxMs != 10 {
		t.Fatalf("min/max = %v/%v, want 1/10", s.MinMs, s.MaxMs)
	}
	if s.AvgMs != 5.5 {
		t.Fatalf("avg = %v, want 5.5", s.AvgMs)
	}
	if s.Samples != 10 {
		t.Fatalf("samples = %d, want 10", s.Samples)
	}
	// Floor-based indexing: p50 = sorted[5] = 6, p95 = sorted[9] = 10.
	if s.P50Ms != 6 || s.P95Ms != 10 {
		t.Fatalf("p50/p95 = %v/%v, want 6/10", s.P50Ms, s.P95Ms)
	}
}

func TestPerSecond(t *testing.T) {
	if got := perSecond(100, 0); got != 0 {
		t.Fatalf("zero seconds = %v, want 0", got)
	}
	if got := perSecond(100, 2); got != 50 {
		t.Fatalf("100/2s = %v, want 50", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{
		512:             "512 B",
		2048:            "2.0 KiB",
		5 * 1024 * 1024: "5.0 MiB",
	}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestCompareWithBaseline(t *testing.T) {
	baseline := BenchmarkResult{
		Metrics: PerformanceMetrics{
			WallClockMs:     1000,
			FrameTimeStats:  FrameTimeStats{AvgMs: 2.0, P95Ms: 3.0},
			FramesPerSecond: 200,
			HeapAllocBytes:  1000,
			ClusterTimeMs:   10,
			TrackingTimeMs:  10,
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	data, _ := json.Marshal(baseline)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// 50% slower wall clock + 50% fewer fps → regressions on both.
	current := &PerformanceMetrics{
		WallClockMs:     1500,
		FrameTimeStats:  FrameTimeStats{AvgMs: 2.0, P95Ms: 3.0},
		FramesPerSecond: 100,
		HeapAllocBytes:  1000,
		ClusterTimeMs:   10,
		TrackingTimeMs:  10,
	}
	comparison, hasRegression := compareWithBaseline(path, current, 0.10)
	if !hasRegression {
		t.Fatal("expected a regression")
	}
	var sawWall, sawFPS bool
	for _, r := range comparison.Regressions {
		switch r.Metric {
		case "wall_clock_ms":
			sawWall = true
		case "frames_per_second":
			sawFPS = true
		}
	}
	if !sawWall || !sawFPS {
		t.Fatalf("expected wall_clock_ms and frames_per_second regressions, got %+v", comparison.Regressions)
	}

	// Identical metrics → no regression, no improvement.
	same, reg := compareWithBaseline(path, &baseline.Metrics, 0.10)
	if reg || len(same.Regressions) != 0 || len(same.Improvements) != 0 {
		t.Fatalf("identical metrics should be neutral, got reg=%v %+v", reg, same)
	}

	// Missing baseline file → nil, no regression.
	if c, r := compareWithBaseline(filepath.Join(dir, "nope.json"), current, 0.10); c != nil || r {
		t.Fatalf("missing baseline = (%v, %v), want (nil, false)", c, r)
	}
}
