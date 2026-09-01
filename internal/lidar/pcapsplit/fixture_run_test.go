//go:build pcap

package pcapsplit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

type failingAnalysisClassifier struct {
	setErr     error
	observeErr error
}

func (f *failingAnalysisClassifier) SetRingElevations([]float64) error { return f.setErr }
func (f *failingAnalysisClassifier) Observe(time.Time, []l2frames.PointPolar) (MotionEvidence, error) {
	return MotionEvidence{}, f.observeErr
}

func restoreAnalysisSeams(t *testing.T) {
	t.Helper()
	originalLoad := loadAnalysisPandarConfig
	originalNew := newAnalysisClassifier
	t.Cleanup(func() {
		loadAnalysisPandarConfig = originalLoad
		newAnalysisClassifier = originalNew
	})
}

// splitFixture is the reference multi-frame capture (UDP 2369). The analysis
// and writer paths only run against a capture carrying real rotations.
const splitFixture = "../perf/pcap/kirk0.pcapng"

func fixtureConfig(t *testing.T) SplitConfig {
	t.Helper()
	return SplitConfig{
		PCAPFile:        filepath.Clean(splitFixture),
		OutputDir:       t.TempDir(),
		SensorID:        "test-sensor",
		UDPPort:         2369,
		DurationSeconds: 10,
		TimelineUnits:   "seconds",
		ProgressSecs:    0,
	}
}

func TestNormaliseReplayDuration(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   float64
		want float64
	}{
		{name: "zero value means full capture", in: 0, want: -1},
		{name: "explicit full capture", in: -1, want: -1},
		{name: "bounded replay", in: 1.5, want: 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normaliseReplayDuration(tc.in); got != tc.want {
				t.Errorf("normaliseReplayDuration(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestAnalyseFixtureProducesSegments drives the analysis half: PCAP decode,
// frame assembly, RPM accumulation, motion/static classification and segment
// construction.
func TestAnalyseFixtureProducesSegments(t *testing.T) {
	got, err := Analyse(fixtureConfig(t))
	if err != nil {
		t.Fatalf("Analyse: %v", err)
	}
	if got == nil {
		t.Fatal("Analyse returned a nil analysis")
	}
	if got.TotalFrames <= 1 {
		t.Errorf("TotalFrames = %d, want more than 1 from the fixture", got.TotalFrames)
	}
	if got.TotalPackets <= 0 {
		t.Errorf("TotalPackets = %d, want > 0", got.TotalPackets)
	}
	// One metrics record per assembled frame.
	if len(got.Frames) != got.TotalFrames {
		t.Errorf("frame metrics = %d, want %d (one per frame)", len(got.Frames), got.TotalFrames)
	}
	// Capture stats are gathered on the same pass, so a zero frame rate means
	// the frame assembly never ran.
	if got.Capture.AvgFrameRateHz <= 0 {
		t.Errorf("AvgFrameRateHz = %v, want > 0", got.Capture.AvgFrameRateHz)
	}
	if !got.LastTime.After(got.FirstTime) {
		t.Errorf("capture window = %v..%v, want it to advance", got.FirstTime, got.LastTime)
	}
}

func TestAnalyseRejectsMissingCapture(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.PCAPFile = filepath.Join(t.TempDir(), "absent.pcapng")

	if _, err := Analyse(cfg); err == nil {
		t.Fatal("Analyse on a missing capture succeeded, want an error")
	}
}

func TestAnalyseReportsSetupAndFrameErrors(t *testing.T) {
	t.Run("parser config", func(t *testing.T) {
		restoreAnalysisSeams(t)
		loadAnalysisPandarConfig = func() (*parse.Pandar40PConfig, error) {
			return nil, errors.New("parser config failed")
		}
		if _, err := Analyse(fixtureConfig(t)); err == nil {
			t.Fatal("expected parser config error")
		}
	})

	t.Run("classifier construction", func(t *testing.T) {
		cfg := fixtureConfig(t)
		cfg.SensorID = ""
		if _, err := Analyse(cfg); err == nil {
			t.Fatal("expected classifier construction error")
		}
	})

	t.Run("ring elevations", func(t *testing.T) {
		restoreAnalysisSeams(t)
		newAnalysisClassifier = func(string, string, *config.TuningConfig) (analysisMotionClassifier, error) {
			return &failingAnalysisClassifier{setErr: errors.New("elevations failed")}, nil
		}
		if _, err := Analyse(fixtureConfig(t)); err == nil {
			t.Fatal("expected ring elevations error")
		}
	})

	t.Run("frame observation", func(t *testing.T) {
		restoreAnalysisSeams(t)
		newAnalysisClassifier = func(string, string, *config.TuningConfig) (analysisMotionClassifier, error) {
			return &failingAnalysisClassifier{observeErr: errors.New("observe failed")}, nil
		}
		if _, err := Analyse(fixtureConfig(t)); err == nil {
			t.Fatal("expected frame observation error")
		}
	})
}

// TestRunFixtureWritesSegmentCaptures drives the full split, including the
// writer that emits one capture per segment.
func TestRunFixtureWritesSegmentCaptures(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.OutputPrefix = "seg"
	cfg.ExportJSON = true

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, err := os.ReadDir(cfg.OutputDir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}
	var captures, jsons int
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case ".pcap", ".pcapng":
			captures++
		case ".json":
			jsons++
		}
	}
	if captures == 0 {
		t.Errorf("no segment captures written; dir contains %d entries", len(entries))
	}
	if jsons == 0 {
		t.Error("ExportJSON set but no JSON metadata written")
	}
}

func TestRunDryRunSkipsWriting(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.DryRun = true

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, err := os.ReadDir(cfg.OutputDir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}
	for _, e := range entries {
		if ext := filepath.Ext(e.Name()); ext == ".pcap" || ext == ".pcapng" {
			t.Errorf("dry run wrote a segment capture: %s", e.Name())
		}
	}
}

func TestRunRejectsMissingCapture(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.PCAPFile = filepath.Join(t.TempDir(), "absent.pcapng")

	if err := Run(cfg); err == nil {
		t.Fatal("Run on a missing capture succeeded, want an error")
	}
}

func TestRunExportsMetricsAndMotionTimeline(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.DryRun = true
	cfg.ExportMetrics = true
	cfg.Stats10s = true
	cfg.MotionJSONPath = filepath.Join(cfg.OutputDir, "motion.json")

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(cfg.MotionJSONPath); err != nil {
		t.Errorf("expected the motion timeline JSON: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "frame_metrics.csv")); err != nil {
		t.Errorf("expected frame_metrics.csv: %v", err)
	}
}

// TestRunWithProgressReportingEnabled covers the progress-wrapping paths on
// both passes: analysis (wrapProgress) and writing (newWriteProgress). A short
// interval guarantees at least one tick during the replay.
func TestRunWithProgressReportingEnabled(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.ProgressSecs = 0.01

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestWrapProgressPassesThroughWithoutInterval(t *testing.T) {
	var inner countingStats
	cfg := fixtureConfig(t)
	cfg.ProgressSecs = 0

	if got := wrapProgress(cfg, &inner, "test"); got != &inner {
		t.Error("wrapProgress with ProgressSecs=0 wrapped the stats, want passthrough")
	}
}

func TestWriteSegmentsRejectsUnwritableOutputDir(t *testing.T) {
	// A file where the output directory should be makes segment creation fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}

	cfg := fixtureConfig(t)
	cfg.OutputDir = blocker

	err := WriteSegments(cfg, []Segment{{StartFrame: 0, EndFrame: 10, Type: MotionLabel}})
	if err == nil {
		t.Fatal("WriteSegments into a non-directory succeeded, want an error")
	}
}

func TestWriteSegmentsWithNoSegments(t *testing.T) {
	// Nothing to write is not an error; the caller has already reported that
	// the capture produced no segments.
	if err := WriteSegments(fixtureConfig(t), nil); err != nil {
		t.Errorf("WriteSegments(nil) = %v, want nil", err)
	}
}
