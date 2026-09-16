package annotation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// writeVRLOG records a small synthetic recording. withPoints controls whether
// frames carry a point cloud, which is the difference between a recording that
// can be annotated and one that cannot.
func writeVRLOG(t *testing.T, withPoints bool, timestamps ...int64) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "synthetic")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for i, ts := range timestamps {
		bundle := &l9endpoints.FrameBundle{TimestampNanos: ts}
		if withPoints {
			n := 3 + i
			pc := &l9endpoints.PointCloudFrame{
				FrameID: uint64(i), TimestampNanos: ts, SensorID: "synthetic",
				X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
				Intensity: make([]uint8, n), Classification: make([]uint8, n),
				PointCount: n,
			}
			for j := range pc.X {
				pc.X[j], pc.Y[j], pc.Z[j] = float32(j), float32(i), 1
				pc.Intensity[j] = uint8(j)
			}
			bundle.PointCloud = pc
		}
		if err := rec.Record(bundle); err != nil {
			t.Fatalf("record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	return dir
}

func TestExportProducesAnAnnotatablePack(t *testing.T) {
	src := writeVRLOG(t, true, 1_000_000_000, 1_100_000_000, 1_200_000_000)
	out := filepath.Join(t.TempDir(), "pack")

	pack, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: out, Coverage: CoverageFull,
		CoverageNote: "synthetic fixture",
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if pack.Manifest.SampleCount != 3 || pack.Manifest.PointCount != 3+4+5 {
		t.Fatalf("pack holds %d samples and %d points", pack.Manifest.SampleCount, pack.Manifest.PointCount)
	}
	if pack.Manifest.Source.VRLOGHeaderSHA == "" || pack.Manifest.Source.VRLOGFramesSHA == "" {
		t.Fatal("export did not record the source digests it will later be checked against")
	}
	if !pack.Manifest.HasIntensity || !pack.Manifest.HasClassification {
		t.Fatal("attribute availability was not recorded")
	}
	if pack.Manifest.Completeness.ActualStartNs != 1_000_000_000 ||
		pack.Manifest.Completeness.ActualEndNs != 1_200_000_000 {
		t.Fatalf("window wrong: %+v", pack.Manifest.Completeness)
	}
	if pack.Manifest.Completeness.MaxTimestampGapNs != 100_000_000 {
		t.Fatalf("gap = %d ns, want 100000000", pack.Manifest.Completeness.MaxTimestampGapNs)
	}

	// The pack must be independently reopenable: annotation happens later, in
	// another process, against the bytes on disk.
	reopened, err := OpenPack(out)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	pts, err := reopened.PointsAt(2)
	if err != nil || len(pts.X) != 5 || pts.Y[0] != 2 {
		t.Fatalf("sample 2 decoded wrong: %v %+v", err, pts)
	}
}

// A recording made without points cannot be annotated, and saying so is more
// use than an empty pack that looks like a successful export.
func TestExportRefusesAPointlessRecording(t *testing.T) {
	src := writeVRLOG(t, false, 1_000_000_000, 1_100_000_000)
	_, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageFull,
	})
	if err == nil {
		t.Fatal("exported a pack with nothing in it")
	}
	if !strings.Contains(err.Error(), "--include-points") {
		t.Fatalf("error does not say how to fix it: %v", err)
	}
}

// Coverage cannot be inferred from the data, so it cannot be defaulted.
func TestExportRequiresCoverage(t *testing.T) {
	src := writeVRLOG(t, true, 1_000_000_000)
	for _, cov := range []CaptureCoverage{"", "mostly"} {
		_, err := Export(ExportConfig{
			VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: cov,
		})
		if err == nil {
			t.Fatalf("coverage %q was accepted", cov)
		}
	}
}

func TestExportWindowAndSampleCap(t *testing.T) {
	src := writeVRLOG(t, true, 100, 200, 300, 400, 500)

	windowed, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "w"),
		Coverage: CoverageFull, StartNs: 200, EndNs: 400,
	})
	if err != nil {
		t.Fatalf("windowed export: %v", err)
	}
	if windowed.Manifest.SampleCount != 3 {
		t.Fatalf("window held %d samples, want 3", windowed.Manifest.SampleCount)
	}
	// Source ordinals survive, so a reader can find the original frames.
	if windowed.Samples[0].SourceOrdinal != 1 {
		t.Fatalf("first sample came from ordinal %d, want 1", windowed.Samples[0].SourceOrdinal)
	}

	capped, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "c"),
		Coverage: CoverageFull, MaxSamples: 2,
	})
	if err != nil {
		t.Fatalf("capped export: %v", err)
	}
	if capped.Manifest.SampleCount != 2 {
		t.Fatalf("cap held %d samples, want 2", capped.Manifest.SampleCount)
	}
}

// Duplicate timestamps are legal and are counted, not rejected: the pack-local
// sample id keeps every reference unambiguous regardless.
func TestExportRecordsDuplicateTimestamps(t *testing.T) {
	src := writeVRLOG(t, true, 100, 100, 200)
	pack, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageFull,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if pack.Manifest.Completeness.DuplicateTimestamps != 1 {
		t.Fatalf("duplicate timestamps = %d, want 1", pack.Manifest.Completeness.DuplicateTimestamps)
	}
	if pack.Manifest.SampleCount != 3 {
		t.Fatalf("a duplicate timestamp lost a sample: %d", pack.Manifest.SampleCount)
	}
	if pack.Samples[0].SampleID == pack.Samples[1].SampleID {
		t.Fatal("two samples share an id; references would be ambiguous")
	}
}
