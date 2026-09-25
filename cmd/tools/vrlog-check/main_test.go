package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// An observation container is recognised by its root and verified; damage to
// it fails the check rather than being probed as FrameBundles.
func TestReportVerifiesObservationContainers(t *testing.T) {
	calibration := l4bobserve.Calibration{SensorID: "s", FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "capture.vrlog")
	w, err := vrlog.Create(dir, vrlog.Manifest{
		Capture:     vrlog.CaptureIdentity{SensorID: "s", SourceType: "synthetic"},
		Extraction:  vrlog.ExtractionIdentity{SourceID: "source/v1/check", CalibrationID: calibrationID, CoordinateFrame: "site/s", ExtractorID: "l4.test/v1"},
		Calibration: calibration,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AppendFrame(l4bobserve.FrameRecord{Sequence: 0, CaptureEndUnixNanos: 1,
		Disposition: l4bobserve.Disposition{Kind: l4bobserve.DispositionFailed, Stage: l4bobserve.StageL3Foreground, Reason: "test"},
		Stages:      []l4bobserve.StageCount{{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(0)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := report(dir, 10); err != nil {
		t.Fatalf("a sound container failed: %v", err)
	}
	chunk := filepath.Join(dir, "chunks", "00000000.chunk")
	data, err := os.ReadFile(chunk)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/3] ^= 1
	if err := os.WriteFile(chunk, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := report(dir, 10); err == nil || !strings.Contains(err.Error(), "verify") {
		t.Fatalf("a damaged container passed: %v", err)
	}
}
