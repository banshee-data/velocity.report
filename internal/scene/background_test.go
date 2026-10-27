package scene

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/version"
)

// writeBackgroundVRLOG records rotations at baseNs + i*100 ms with one settled
// background snapshot after the third. The snapshot carries the replay's wall
// clock, as GenerateBackgroundSnapshot stamps it, which is not capture time.
func writeBackgroundVRLOG(t *testing.T, rotations int) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bg.vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	for i := 0; i < rotations; i++ {
		if err := rec.Record(&l9endpoints.FrameBundle{
			FrameID: uint64(i), TimestampNanos: baseNs + int64(i)*100_000_000,
			SensorID: "hesai-pandar40p", FrameType: l9endpoints.FrameTypeForeground,
		}); err != nil {
			t.Fatalf("record rotation: %v", err)
		}
		if i != 2 {
			continue
		}
		wallClock := time.Now().UnixNano()
		if err := rec.Record(&l9endpoints.FrameBundle{
			FrameID: 10_000, TimestampNanos: wallClock, SensorID: "hesai-pandar40p",
			FrameType: l9endpoints.FrameTypeBackground,
			Background: &l9endpoints.BackgroundSnapshot{
				TimestampNanos: wallClock,
				X:              []float32{1.234, 5, -3},
				Y:              []float32{10.5, 12, 8},
				Z:              []float32{-1.6, 2.5, 0.4},
				Confidence:     []uint32{40, 900, 7},
				GridMetadata: l9endpoints.GridMetadata{
					Rings: 40, AzimuthBins: 1800, SettlingComplete: true,
				},
			},
		}); err != nil {
			t.Fatalf("record background: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return dir
}

func readBackground(t *testing.T, outDir string) BackgroundExport {
	t.Helper()
	f, err := os.Open(filepath.Join(outDir, "background.json.gz"))
	if err != nil {
		t.Fatalf("open background: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip background: %v", err)
	}
	var bg BackgroundExport
	if err := json.NewDecoder(gz).Decode(&bg); err != nil {
		t.Fatalf("decode background: %v", err)
	}
	return bg
}

// A copied background.json.gz must say what it is on its own: the frame and
// units of its points, when they were gathered, how they are known, and the
// code that made them.
func TestExportBackgroundCarriesItsMeasurementContext(t *testing.T) {
	oldVersion, oldSHA := version.Version, version.GitSHA
	version.Version, version.GitSHA = "0.5.1-vrlog-generator-test", "abc1234"
	t.Cleanup(func() { version.Version, version.GitSHA = oldVersion, oldSHA })
	src := writeBackgroundVRLOG(t, 6)
	// Exporting with a different binary must not rewrite recording provenance.
	version.Version, version.GitSHA = "0.5.1-scene-exporter-test", "def5678"

	out := filepath.Join(t.TempDir(), "out")
	res, err := ExportBackground(Options{VRLOGPath: src, OutDir: out, Site: "soma1", Title: "SoMa 1"})
	if err != nil {
		t.Fatalf("ExportBackground: %v", err)
	}
	bg := readBackground(t, out)

	if bg.SensorID != "hesai-pandar40p" {
		t.Errorf("sensor_id %q, want the recording's sensor", bg.SensorID)
	}
	if bg.CoordinateFrame.FrameID != "site/hesai-pandar40p" || bg.CoordinateFrame.ReferenceFrame != "ENU" {
		t.Errorf("coordinate_frame %+v, want the recording's frame", bg.CoordinateFrame)
	}
	if bg.Units != (Units{Length: "m"}) {
		t.Errorf("units %+v, want lengths in metres and nothing a background does not carry", bg.Units)
	}
	if bg.CoordinateRoundingM != 0.01 {
		t.Errorf("coordinate_rounding_m %v, want 0.01", bg.CoordinateRoundingM)
	}
	if bg.Rings != 40 || bg.AzimuthBinDeg != 0.2 {
		t.Errorf("grid %d rings at %v deg, want 40 rings at 0.2 deg", bg.Rings, bg.AzimuthBinDeg)
	}
	if bg.Evidence != (Evidence{Points: EvidenceInferred, GroundZ: EvidenceInferred}) {
		t.Errorf("evidence %+v, want points and ground_z inferred from cells", bg.Evidence)
	}

	// The interval is the capture's rotations, not the snapshot's wall clock.
	want := Interval{
		StartNs: strconv.FormatInt(baseNs, 10),
		EndNs:   strconv.FormatInt(baseNs+5*100_000_000, 10),
	}
	if bg.AcquiredInterval == nil || *bg.AcquiredInterval != want {
		t.Errorf("acquired_interval %+v, want the capture's rotation span %+v", bg.AcquiredInterval, want)
	}

	if bg.BuildVersion != "0.5.1-vrlog-generator-test" || bg.BuildGitSHA != "abc1234" {
		t.Errorf("build %q @ %q, want the binary that generated the VRLOG", bg.BuildVersion, bg.BuildGitSHA)
	}
	if bg.SourceVRLOGSHA256 == "" {
		t.Error("source_vrlog_sha256 missing; an export must name the run it came from")
	}

	if bg.PointCount != 3 || len(bg.Points) != 3 {
		t.Fatalf("%d points (count %d), want 3", len(bg.Points), bg.PointCount)
	}
	if p := bg.Points[0]; p[0] != 1.23 || p[3] != 40 {
		t.Errorf("first point %v, want x rounded to 1.23 and times seen 40", p)
	}

	if res.Header.Evidence != bg.Evidence || res.Header.BuildGitSHA != "abc1234" {
		t.Errorf("result header %+v does not describe the file it reports", res.Header)
	}
}

// A recording that names no interval gets none, rather than one made up from
// the export time or the snapshot's clock.
func TestCaptureIntervalOmittedWithoutRotations(t *testing.T) {
	if got := captureInterval(recorder.LogHeader{}); got != nil {
		t.Errorf("interval %+v from an empty header, want none", got)
	}
	if got := captureInterval(recorder.LogHeader{StartNs: 10, EndNs: 5}); got != nil {
		t.Errorf("interval %+v from an inverted span, want none", got)
	}
	if got := captureInterval(recorder.LogHeader{StartNs: 10, EndNs: 10}); got == nil || got.StartNs != "10" || got.EndNs != "10" {
		t.Errorf("interval %+v from a one-rotation span, want 10..10", got)
	}
}

func TestExportEvidenceByKind(t *testing.T) {
	cases := map[Kind]Evidence{
		KindTracks:     {Tracks: EvidenceInferred},
		KindClip:       {Points: EvidenceObserved, Tracks: EvidenceInferred},
		KindBackground: {Points: EvidenceInferred, GroundZ: EvidenceInferred},
	}
	for kind, want := range cases {
		if got := exportEvidence(kind); got != want {
			t.Errorf("%s evidence %+v, want %+v", kind, got, want)
		}
	}
}
