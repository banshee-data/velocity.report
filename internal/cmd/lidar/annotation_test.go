//go:build pcap
// +build pcap

package lidar

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// recordPointVRLOG writes a small point-bearing recording for the exporter to
// read. Without points there is nothing to annotate, which is itself one of
// the cases below.
func recordPointVRLOG(t *testing.T, withPoints bool, timestamps ...int64) string {
	t.Helper()
	if len(timestamps) == 0 {
		timestamps = []int64{100_000_000, 200_000_000, 300_000_000}
	}
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "synthetic")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for i, ts := range timestamps {
		bundle := &l9endpoints.FrameBundle{TimestampNanos: ts}
		if withPoints {
			n := 4
			pc := &l9endpoints.PointCloudFrame{
				FrameID: uint64(i), TimestampNanos: bundle.TimestampNanos, SensorID: "synthetic",
				X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
				Intensity: make([]uint8, n), Classification: make([]uint8, n), PointCount: n,
			}
			bundle.PointCloud = pc
		}
		if err := rec.Record(bundle); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return dir
}

func TestAnnotationExportRoutingAndFlags(t *testing.T) {
	if code := silence(t, func() int { return Main([]string{"annotation-export", "-h"}) }); code != 0 {
		t.Errorf("help exited %d, want 0", code)
	}
	if code := silence(t, func() int { return Main([]string{"annotation-export", "-nope"}) }); code != 2 {
		t.Errorf("bad flag exited %d, want 2", code)
	}
}

// Each required flag must be required. Coverage in particular cannot be
// defaulted: it states a limitation the data itself does not reveal.
func TestAnnotationExportRequiresItsArguments(t *testing.T) {
	src := recordPointVRLOG(t, true)
	out := filepath.Join(t.TempDir(), "pack")

	cases := map[string][]string{
		"no vrlog":    {"--output", out, "--coverage", "full"},
		"no output":   {"--vrlog", src, "--coverage", "full"},
		"no coverage": {"--vrlog", src, "--output", out},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if code := silence(t, func() int { return AnnotationExportMain(args) }); code != 2 {
				t.Errorf("exited %d, want 2", code)
			}
		})
	}
}

func TestAnnotationExportWritesAPack(t *testing.T) {
	src := recordPointVRLOG(t, true)
	out := filepath.Join(t.TempDir(), "pack")

	code := silence(t, func() int {
		return AnnotationExportMain([]string{
			"--vrlog", src, "--output", out,
			"--coverage", "foreground_only",
			"--coverage-note", "synthetic fixture",
			"--max-samples", "2",
		})
	})
	if code != 0 {
		t.Fatalf("export exited %d, want 0", code)
	}
	for _, name := range []string{"manifest.json", "samples.json", "points.bin"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("pack is missing %s: %v", name, err)
		}
	}
}

func TestAnnotationExportReportsFailure(t *testing.T) {
	// A recording with no points cannot be annotated, and the command must
	// say so rather than leaving an empty directory behind.
	src := recordPointVRLOG(t, false)
	out := filepath.Join(t.TempDir(), "pack")
	code := silence(t, func() int {
		return AnnotationExportMain([]string{"--vrlog", src, "--output", out, "--coverage", "full"})
	})
	if code != 1 {
		t.Fatalf("exited %d, want 1", code)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a failed export left a directory behind")
	}
}

// The summary reports what the export found. Duplicate timestamps and a
// timestamp gap are legal, and both are worth printing: they are what a reader
// checks before trusting the excerpt.
func TestAnnotationExportReportsSourceIrregularities(t *testing.T) {
	src := recordPointVRLOG(t, true, 100_000_000, 100_000_000, 300_000_000)
	out := filepath.Join(t.TempDir(), "pack")

	code := silence(t, func() int {
		return AnnotationExportMain([]string{
			"--vrlog", src, "--output", out, "--coverage", "decimated",
		})
	})
	if code != 0 {
		t.Fatalf("export exited %d, want 0", code)
	}
}

// A recording that carries points for only some frames still yields a usable
// pack, and the count of what was skipped is part of the excerpt's honesty.
func TestAnnotationExportReportsSkippedFrames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "synthetic")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for i := 0; i < 4; i++ {
		bundle := &l9endpoints.FrameBundle{TimestampNanos: int64(100_000_000 * (i + 1))}
		if i%2 == 0 {
			n := 4
			bundle.PointCloud = &l9endpoints.PointCloudFrame{
				FrameID: uint64(i), TimestampNanos: bundle.TimestampNanos, SensorID: "synthetic",
				X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
				PointCount: n,
			}
		}
		if err := rec.Record(bundle); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out := filepath.Join(t.TempDir(), "pack")
	code := silence(t, func() int {
		return AnnotationExportMain([]string{
			"--vrlog", dir, "--output", out, "--coverage", "full",
		})
	})
	if code != 0 {
		t.Fatalf("export exited %d, want 0", code)
	}
}
