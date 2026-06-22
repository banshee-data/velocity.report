package pcapsplit

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestNewMotionClassifierValidation(t *testing.T) {
	if _, err := NewMotionClassifier("", "capture.pcapng", nil); err == nil {
		t.Fatal("expected empty sensor ID to fail")
	}
	// A nil tuning falls back to the validated embedded defaults. (Threshold
	// validation now lives in config.Validate, covered by the config package.)
	if _, err := NewMotionClassifier("sensor", "capture.pcapng", nil); err != nil {
		t.Fatalf("nil tuning should use embedded defaults: %v", err)
	}
}

func TestNewMotionClassifierFallsBackToEmbeddedConfig(t *testing.T) {
	// Running from a directory without config/tuning.defaults.json must not
	// panic (as MustLoadDefaultConfig would): the classifier loads the embedded
	// defaults instead. This mirrors the Pi images, which embed the file.
	t.Chdir(t.TempDir())
	c, err := NewMotionClassifier("sensor", "capture.pcapng", nil)
	if err != nil {
		t.Fatalf("NewMotionClassifier without an on-disk tuning config: %v", err)
	}
	if c == nil || c.bg == nil {
		t.Fatal("expected an initialised classifier from embedded defaults")
	}
}

func TestMotionClassifierUsesActiveTuningAndCaptureTime(t *testing.T) {
	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := classifier.SetRingElevations(make([]float64, gridRings)); err != nil {
		t.Fatal(err)
	}
	params := classifier.bg.GetParams()
	active := config.MustLoadDefaultConfig().L3.EmaBaselineV1
	if math.Abs(float64(params.NoiseRelativeFraction)-active.NoiseRelative) > 1e-6 {
		t.Fatalf("noise relative=%f, want active tuning=%f", params.NoiseRelativeFraction, active.NoiseRelative)
	}
	if params.WarmupDurationNanos != active.WarmupDurationNanos || params.WarmupMinFrames != active.WarmupMinFrames {
		t.Fatalf("offline classifier changed live warmup tuning: %+v", params)
	}
	if !classifier.bg.IsReplayMode() {
		t.Fatal("offline classifier must enable replay mode")
	}
	wantParams := l3grid.BackgroundConfigFromActiveTuning(config.MustLoadDefaultConfig()).ToBackgroundParams()
	if params != wantParams {
		t.Fatalf("offline params differ from live tuning\n got: %+v\nwant: %+v", params, wantParams)
	}

	t0 := time.Unix(1_000, 0)
	point := []l3grid.PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}}
	evidence, err := classifier.Observe(t0, point)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.T.Equal(t0) || evidence.TotalPoints != 1 {
		t.Fatalf("unexpected initial evidence: %+v", evidence)
	}
	// The motion decision is supplied by the shared L3 evaluator, and Stable is
	// always its inverse.
	if evidence.Stable == evidence.Moving {
		t.Fatalf("decision inconsistent with motion signals: %+v", evidence)
	}
}

func TestMotionClassifierPreservesTuningAndUsesCommonEvaluator(t *testing.T) {
	tuning := config.MustLoadDefaultConfig()
	original := tuning.L3.EmaBaselineV1.LockedBaselineThreshold
	tuning.L3.EmaBaselineV1.SensorMovementForegroundThreshold = 0.6
	tuning.L3.EmaBaselineV1.SensorMovementDriftRatioThreshold = 0.5

	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", tuning)
	if err != nil {
		t.Fatal(err)
	}
	params := classifier.bg.GetParams()
	if params.SensorMovementForegroundThreshold != 0.6 || params.SensorMovementDriftRatioThreshold != 0.5 {
		t.Fatalf("shared evaluator params = %+v", params)
	}
	if tuning.L3.EmaBaselineV1.LockedBaselineThreshold != original {
		t.Fatal("classifier mutated caller-owned tuning")
	}
	if classifier.bg.EvaluateSensorMotion([]bool{true, false}).Moving {
		t.Fatal("custom foreground/drift thresholds were not applied")
	}
}

func TestMotionClassifierNilGuards(t *testing.T) {
	var classifier *MotionClassifier
	if err := classifier.SetRingElevations(nil); err == nil {
		t.Fatal("expected nil classifier elevations error")
	}
	if _, err := classifier.Observe(time.Time{}, nil); err == nil {
		t.Fatal("expected nil classifier observe error")
	}
	if _, err := (&MotionClassifier{}).Observe(time.Time{}, nil); err == nil {
		t.Fatal("expected classifier without a background manager to fail")
	}
}

func TestMotionClassifierObserveEmptyFrame(t *testing.T) {
	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := classifier.Observe(time.Unix(3_000, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	// An empty frame has zero foreground, so it is classified static (a frame
	// with no points cannot exhibit sensor motion).
	if evidence.TotalPoints != 0 || evidence.ForegroundFraction != 0 || !evidence.Stable || evidence.Moving {
		t.Fatalf("unexpected empty-frame evidence: %+v", evidence)
	}
}

func TestMotionClassifierCachesGridEvidenceForOneCaptureSecond(t *testing.T) {
	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", nil)
	if err != nil {
		t.Fatal(err)
	}
	point := []l3grid.PointPolar{{Channel: 1, Azimuth: 0, Distance: 10}}
	t0 := time.Unix(4_000, 0)
	if _, err := classifier.Observe(t0, point); err != nil {
		t.Fatal(err)
	}
	if !classifier.metricsAt.Equal(t0) {
		t.Fatalf("metrics timestamp=%v, want %v", classifier.metricsAt, t0)
	}
	if _, err := classifier.Observe(t0.Add(999*time.Millisecond), point); err != nil {
		t.Fatal(err)
	}
	if !classifier.metricsAt.Equal(t0) {
		t.Fatalf("sub-second frame refreshed metrics at %v", classifier.metricsAt)
	}
	if _, err := classifier.Observe(t0.Add(time.Second), point); err != nil {
		t.Fatal(err)
	}
	if !classifier.metricsAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("one-second frame did not refresh metrics at %v", classifier.metricsAt)
	}
	if _, err := classifier.Observe(t0.Add(500*time.Millisecond), point); err != nil {
		t.Fatal(err)
	}
	if !classifier.metricsAt.Equal(t0.Add(500 * time.Millisecond)) {
		t.Fatalf("out-of-order capture time did not refresh metrics at %v", classifier.metricsAt)
	}
}
