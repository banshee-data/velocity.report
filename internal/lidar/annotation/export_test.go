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

// recordScript writes a recording from a script of frames: 'b' a background
// snapshot with no points, 'p' a frame of points, 'B' both at once. Point
// frames are stamped 1000, 2000, ... in order; a background is stamped bgTs,
// which a replay settled ahead of time sets later than everything after it.
func recordScript(t *testing.T, script string, bgTs int64) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "synthetic")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	pointTs := int64(0)
	for i, kind := range script {
		bundle := &l9endpoints.FrameBundle{}
		if kind == 'b' || kind == 'B' {
			// The snapshot's first X says which snapshot it is.
			bundle.TimestampNanos = bgTs
			bundle.Background = &l9endpoints.BackgroundSnapshot{
				TimestampNanos: bgTs, X: []float32{float32(i), 1}, Y: []float32{2, 3}, Z: []float32{4, 5},
				GridMetadata: l9endpoints.GridMetadata{SettlingComplete: true},
			}
		}
		if kind == 'p' || kind == 'B' {
			pointTs += 1000
			bundle.TimestampNanos = pointTs
			bundle.PointCloud = &l9endpoints.PointCloudFrame{
				FrameID: uint64(i), TimestampNanos: pointTs, SensorID: "synthetic",
				X: []float32{1}, Y: []float32{1}, Z: []float32{1},
				Intensity: []uint8{1}, Classification: []uint8{0}, PointCount: 1,
			}
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

func TestExportCarriesTheBackgroundInForce(t *testing.T) {
	// Ordinals:            0123456
	src := recordScript(t, "bppbppb", 999_999)

	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: out, Coverage: CoverageFull}); err != nil {
		t.Fatalf("export: %v", err)
	}
	pack, err := OpenPack(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// The trailing snapshot is in force for no sample, so it is not carried.
	if got := len(pack.Backgrounds); got != 2 {
		t.Fatalf("pack carries %d backgrounds, want 2", got)
	}
	if pack.Backgrounds[0].SourceOrdinal != 0 || pack.Backgrounds[1].SourceOrdinal != 3 {
		t.Errorf("background ordinals = %d, %d, want 0, 3",
			pack.Backgrounds[0].SourceOrdinal, pack.Backgrounds[1].SourceOrdinal)
	}
	if !pack.Backgrounds[0].SettlingComplete || pack.Backgrounds[0].PointCount != 2 {
		t.Errorf("background 0 = %+v", pack.Backgrounds[0])
	}
	// Samples 0 and 1 sit under the first snapshot, 2 and 3 under the second.
	for sampleID, want := range []int{0, 0, 1, 1} {
		got, ok := pack.BackgroundInForce(sampleID)
		if !ok || got.BackgroundID != want {
			t.Errorf("sample %d: background in force = %d (ok %v), want %d", sampleID, got.BackgroundID, ok, want)
		}
	}
	// Frames that carried a background are not missing points.
	if n := pack.Manifest.Completeness.FramesWithoutPoints; n != 0 {
		t.Errorf("frames without points = %d, want 0: background frames are not a gap", n)
	}
}

// The recording this was found on opens with a snapshot stamped five minutes
// after the frames that follow it. With an end time set, the exporter broke
// out of its loop on that first frame and reported no point-bearing frames.
func TestExportWindowIsNotEndedByALateStampedBackground(t *testing.T) {
	src := recordScript(t, "bpppp", 999_999)

	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: out, Coverage: CoverageFull, StartNs: 2000, EndNs: 3000,
	}); err != nil {
		t.Fatalf("windowed export: %v", err)
	}
	pack, err := OpenPack(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(pack.Samples) != 2 {
		t.Errorf("samples = %d, want the 2 inside the window", len(pack.Samples))
	}
	// Recorded before the window, and still what was in force inside it.
	if got, ok := pack.BackgroundInForce(0); !ok || got.SourceOrdinal != 0 {
		t.Errorf("background in force at the first sample = %+v (ok %v), want ordinal 0", got, ok)
	}
}

func TestExportKeepsAFramesBackgroundAndItsPointsTogether(t *testing.T) {
	src := recordScript(t, "Bp", 999_999)
	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: out, Coverage: CoverageFull}); err != nil {
		t.Fatalf("export: %v", err)
	}
	pack, err := OpenPack(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(pack.Samples) != 2 || len(pack.Backgrounds) != 1 {
		t.Fatalf("samples %d, backgrounds %d, want 2 and 1", len(pack.Samples), len(pack.Backgrounds))
	}
	if got, ok := pack.BackgroundInForce(0); !ok || got.BackgroundID != 0 {
		t.Errorf("the frame's own background is not in force for its own points: %+v (ok %v)", got, ok)
	}
}

func TestAPackWithoutBackgroundIsWrittenAsItAlwaysWas(t *testing.T) {
	src := writeVRLOG(t, true, 1000, 2000)
	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: out, Coverage: CoverageFull}); err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, name := range []string{backgroundsFile, backgroundPointsFile} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s written for a recording with no background", name)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(out, manifestFile))
	if strings.Contains(string(raw), "background") {
		t.Error("manifest mentions a background the pack does not carry")
	}
	pack, err := OpenPack(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, ok := pack.BackgroundInForce(0); ok {
		t.Error("a pack with no backgrounds reported one in force")
	}
}

func TestOpenPackRefusesATamperedBackground(t *testing.T) {
	src := recordScript(t, "bp", 999_999)
	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: out, Coverage: CoverageFull}); err != nil {
		t.Fatalf("export: %v", err)
	}
	path := filepath.Join(out, backgroundPointsFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw[0] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := OpenPack(out); err == nil || !strings.Contains(err.Error(), "background points digest") {
		t.Errorf("opened a pack whose background was altered: %v", err)
	}
}
