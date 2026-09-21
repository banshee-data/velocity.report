package annotation

import (
	"os"
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

// writeClassedVRLOG records two frames whose points all carry `class`, except
// that `strays` of them in the second frame are background.
func writeClassedVRLOG(t *testing.T, class uint8, strays int) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "synthetic")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for i := 0; i < 2; i++ {
		n := 4
		pc := &l9endpoints.PointCloudFrame{
			FrameID: uint64(i), TimestampNanos: int64(i+1) * 1000, SensorID: "synthetic",
			X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
			Intensity: make([]uint8, n), Classification: make([]uint8, n),
			PointCount: n,
		}
		for j := range pc.Classification {
			pc.Classification[j] = class
		}
		if i == 1 {
			for j := 0; j < strays; j++ {
				pc.Classification[j] = 0
			}
		}
		if err := rec.Record(&l9endpoints.FrameBundle{TimestampNanos: pc.TimestampNanos, PointCloud: pc}); err != nil {
			t.Fatalf("record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	return dir
}

// A recording that kept foreground only was being exported as "full" because
// the operator's sheet offered it. The recorder's own class bytes say
// otherwise, and a pack that claims the whole scene while holding none of the
// background is the mislabelling coverage exists to prevent.
func TestExportRefusesFullCoverageOfAForegroundOnlyRecording(t *testing.T) {
	src := writeClassedVRLOG(t, 1, 0)

	_, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageFull,
	})
	if err == nil {
		t.Fatal("exported an all-foreground recording as full coverage")
	}
	if !strings.Contains(err.Error(), "contradicts the recording") ||
		!strings.Contains(err.Error(), string(CoverageForegroundOnly)) {
		t.Errorf("error %q does not say what is wrong or what to state instead", err)
	}

	// The same recording, honestly described, exports.
	if _, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageForegroundOnly,
	}); err != nil {
		t.Errorf("foreground_only export of the same recording: %v", err)
	}
}

func TestExportAcceptsFullCoverageWhenAnyBackgroundWasRecorded(t *testing.T) {
	// One background return is enough: the check is for a contradiction, not
	// a judgement of how much background a full scene ought to have.
	src := writeClassedVRLOG(t, 1, 1)
	if _, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageFull,
	}); err != nil {
		t.Errorf("full export with background present: %v", err)
	}
}

func TestExportRecordsTheRunsHeightBand(t *testing.T) {
	src := writeVRLOG(t, true, 1000, 2000)
	band := &HeightBand{FloorM: -2.8, CeilingM: 1.5, RemoveGround: true}

	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: out, Coverage: CoverageFull, HeightBand: band,
	}); err != nil {
		t.Fatalf("export: %v", err)
	}
	// Re-opened from disk: what a client reads, not what Export held in memory.
	pack, err := OpenPack(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got := pack.Manifest.Source.HeightBand
	if got == nil || *got != *band {
		t.Errorf("manifest height band = %+v, want %+v", got, band)
	}

	// Unknown stays absent rather than becoming a zero band, which would
	// read as "everything above the sensor was removed".
	bare := filepath.Join(t.TempDir(), "bare")
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: bare, Coverage: CoverageFull}); err != nil {
		t.Fatalf("export without a band: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(bare, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(raw), "height_band") {
		t.Error("a manifest with no known band still wrote a height_band key")
	}
}
