package pcapsplit

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestDefaultMotionClassifierConfig(t *testing.T) {
	cfg := DefaultMotionClassifierConfig()
	if cfg.SettledThreshold != DefaultSettledThreshold ||
		cfg.MovementForegroundThreshold != 0.20 || cfg.NoiseBoundsThreshold != 2 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestClassifyMovingUsesThreshold(t *testing.T) {
	cfg := DefaultMotionClassifierConfig() // movement threshold 0.20
	if classifyMoving(cfg, 0.19) {
		t.Error("foreground 0.19 should be static (below threshold)")
	}
	if !classifyMoving(cfg, 0.20) {
		t.Error("foreground 0.20 should be motion (at threshold)")
	}
	if !classifyMoving(cfg, 0.55) {
		t.Error("foreground 0.55 should be motion")
	}
}

func TestNewMotionClassifierValidation(t *testing.T) {
	cfg := DefaultMotionClassifierConfig()
	if _, err := NewMotionClassifier("", "capture.pcapng", cfg); err == nil {
		t.Fatal("expected empty sensor ID to fail")
	}

	for _, mutate := range []func(*MotionClassifierConfig){
		func(c *MotionClassifierConfig) { c.MovementForegroundThreshold = 0 },
		func(c *MotionClassifierConfig) { c.MovementForegroundThreshold = 1 },
		func(c *MotionClassifierConfig) { c.NoiseBoundsThreshold = 0 },
	} {
		bad := cfg
		mutate(&bad)
		if _, err := NewMotionClassifier("sensor", "capture.pcapng", bad); err == nil {
			t.Errorf("expected invalid config %+v to fail", bad)
		}
	}
}

func TestMotionClassifierUsesActiveTuningAndCaptureTime(t *testing.T) {
	classifierCfg := DefaultMotionClassifierConfig()
	classifierCfg.SettledThreshold = 0 // zero selects the default threshold.
	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", classifierCfg)
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
	if params.WarmupDurationNanos != 0 || params.WarmupMinFrames != 0 {
		t.Fatalf("offline classifier must disable wall-clock warmup: %+v", params)
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
	// The motion decision follows the foreground fraction against the movement
	// threshold, and Stable is always its inverse.
	wantMoving := evidence.ForegroundFraction >= DefaultMotionClassifierConfig().MovementForegroundThreshold
	if evidence.Moving != wantMoving || evidence.Stable == evidence.Moving {
		t.Fatalf("decision inconsistent with foreground fraction: %+v", evidence)
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
	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", DefaultMotionClassifierConfig())
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
	classifier, err := NewMotionClassifier("sensor", "capture.pcapng", DefaultMotionClassifierConfig())
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
