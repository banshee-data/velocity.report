//go:build pcap
// +build pcap

package pcapanalyse

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = old
		log.SetOutput(os.Stderr)
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// fileAsDir returns a path whose parent component is a regular file, so any
// MkdirAll / Create / open beneath it fails — a portable way to exercise I/O
// error branches.
func fileAsDir(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(f, "sub")
}

func TestExport_ErrorPaths(t *testing.T) {
	bad := fileAsDir(t)

	if err := exportTracksCSV(filepath.Join(bad, "t.csv"), []*TrackExport{{TrackID: "a"}}); err == nil {
		t.Error("exportTracksCSV: expected error for un-creatable path")
	}
	if err := exportTrainingData(bad, []*TrainingFrame{{FrameID: 1, ForegroundBlob: []byte{1}}}); err == nil {
		t.Error("exportTrainingData: expected MkdirAll error")
	}
	cfg := Config{PCAPFile: "cap.pcap", OutputDir: bad, ExportJSON: true, ExportCSV: true}
	if err := exportResults(cfg, sampleResult()); err == nil {
		t.Error("exportResults: expected write error")
	}
	if err := persistToDatabase(filepath.Join(bad, "x.db"), sampleResult(), nil); err == nil {
		t.Error("persistToDatabase: expected open error")
	}
}

func TestNewAnalysisFrameBuilder_BadDB(t *testing.T) {
	// DBPath under a file -> db.NewDB fails -> dbConn stays nil (warning logged).
	fb := newAnalysisFrameBuilder(Config{SensorID: "s", DBPath: filepath.Join(fileAsDir(t), "x.db")},
		&AnalysisResult{TracksByClass: map[string]int{}})
	if fb.dbConn != nil {
		_ = fb.dbConn.Close()
		t.Error("expected nil dbConn when db open fails")
	}
}

func TestRun_OutputDirError(t *testing.T) {
	if code := Run(Config{PCAPFile: "x", OutputDir: fileAsDir(t)}); code != 1 {
		t.Errorf("Run with bad output dir = %d, want 1", code)
	}
}

func TestRun_StatsModeSurfacesAnalysisErrors(t *testing.T) {
	pcapDir := t.TempDir()
	var code int
	stderr := captureStderr(t, func() {
		code = Run(Config{PCAPFile: pcapDir, Stats: true})
	})
	if code != 1 {
		t.Fatalf("Run stats mode on unreadable pcap source = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Analysis failed:") {
		t.Fatalf("stderr missing analysis failure, got %q", stderr)
	}
	if !strings.Contains(stderr, pcapDir) {
		t.Fatalf("stderr missing source path, got %q", stderr)
	}
}

func TestRun_WithDBPersistence(t *testing.T) {
	cfg := baseConfig(t, testCapture(t, 1200))
	cfg.DBPath = filepath.Join(t.TempDir(), "run.db")
	cfg.ExportTraining = true
	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 0 {
		t.Fatalf("Run with DB = %d, want 0", code)
	}
	if _, err := os.Stat(cfg.DBPath); err != nil {
		t.Errorf("expected DB written: %v", err)
	}
}

func TestHandleBenchmarkOutput_WithBaselineRegression(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "base.json")
	baseline := BenchmarkResult{Metrics: PerformanceMetrics{
		WallClockMs: 100, FrameTimeStats: FrameTimeStats{AvgMs: 10, P95Ms: 20},
		FramesPerSecond: 100, HeapAllocBytes: 1000, ClusterTimeMs: 10, TrackingTimeMs: 10,
	}}
	data, _ := json.Marshal(baseline)
	_ = os.WriteFile(baselinePath, data, 0o644)

	cfg := Config{PCAPFile: "cap.pcap", OutputDir: dir, CompareBaseline: baselinePath}
	worse := &PerformanceMetrics{WallClockMs: 300, FrameTimeStats: FrameTimeStats{AvgMs: 30, P95Ms: 60}, FramesPerSecond: 30, HeapAllocBytes: 3000, ClusterTimeMs: 30, TrackingTimeMs: 30}
	var code int
	captureOutput(t, func() { code = handleBenchmarkOutput(cfg, sampleResult(), worse) })
	if code != 1 {
		t.Errorf("expected exit 1 on regression, got %d", code)
	}
}

func TestPrintComparisonSummary_NoChanges(t *testing.T) {
	captureOutput(t, func() {
		printComparisonSummary(&BenchmarkComparison{BaselineFile: "b.json"}, 0.10)
	})
}

func TestPrintSummary_WithTraining(t *testing.T) {
	r := sampleResult()
	r.TrainingFrames = 3
	captureOutput(t, func() { printSummary(r, "frames") })
}

func TestCollectTrackResults_SpeedSamples(t *testing.T) {
	tracks := map[string]*l5tracks.TrackedObject{
		"t1": {TrackID: "t1", TrackMeasurement: l5tracks.TrackMeasurement{
			TrackState: l5tracks.TrackConfirmed, ObjectClass: "vehicle",
			AvgSpeedMps: 12.5, ObservationCount: 8,
		}},
	}
	fb := makeFrameBuilder(tracks)
	r := newResult()
	collectTrackResults(fb, r)
	if r.SpeedStats.MaxSpeed != 12.5 {
		t.Errorf("MaxSpeed = %v, want 12.5", r.SpeedStats.MaxSpeed)
	}
}
