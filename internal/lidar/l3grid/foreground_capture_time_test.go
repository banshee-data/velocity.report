package l3grid

import (
	"testing"
	"time"
)

func TestProcessFramePolarWithMaskAtUsesCaptureTimeForFreeze(t *testing.T) {
	g := makeCoverageGrid(1, 1)
	g.Params.WarmupMinFrames = 0
	g.Params.WarmupDurationNanos = 0
	g.Params.FreezeDurationNanos = int64(5 * time.Second)
	g.Params.NoiseRelativeFraction = 0.001
	g.Params.SafetyMarginMetres = 0
	g.Params.ClosenessSensitivityMultiplier = 1
	g.Params.NeighbourConfirmationCount = 0
	bm := g.Manager
	t0 := time.Unix(1_000, 0)

	if mask, err := bm.ProcessFramePolarWithMaskAt([]PointPolar{{Channel: 1, Distance: 10}}, t0); err != nil || mask[0] {
		t.Fatalf("seed mask=%v err=%v", mask, err)
	}
	if mask, err := bm.ProcessFramePolarWithMaskAt([]PointPolar{{Channel: 1, Distance: 100}}, t0.Add(time.Second)); err != nil || !mask[0] {
		t.Fatalf("divergent mask=%v err=%v", mask, err)
	}
	if mask, err := bm.ProcessFramePolarWithMaskAt([]PointPolar{{Channel: 1, Distance: 10}}, t0.Add(2*time.Second)); err != nil || !mask[0] {
		t.Fatalf("frozen mask=%v err=%v", mask, err)
	}
	if mask, err := bm.ProcessFramePolarWithMaskAt([]PointPolar{{Channel: 1, Distance: 10}}, t0.Add(7*time.Second)); err != nil || mask[0] {
		t.Fatalf("expired capture-time freeze mask=%v err=%v", mask, err)
	}
}

func TestGetFrameSettlingMetricsAtUsesCaptureTime(t *testing.T) {
	g := makeCoverageGrid(1, 1)
	t0 := time.Unix(2_000, 0)
	g.Cells[0].FrozenUntilUnixNanos = t0.Add(5 * time.Second).UnixNano()

	if got := g.Manager.GetFrameSettlingMetricsAt(1, t0); got.FrozenCells != 1 || got.PercentFrozen != 1 {
		t.Fatalf("at freeze start: %+v", got)
	}
	if got := g.Manager.GetFrameSettlingMetricsAt(1, t0.Add(6*time.Second)); got.FrozenCells != 0 || got.PercentFrozen != 0 {
		t.Fatalf("after capture-time expiry: %+v", got)
	}
	if got := g.Manager.GetFrameSettlingMetricsAt(1, time.Time{}); got.TotalCells != 1 {
		t.Fatalf("zero timestamp fallback returned invalid metrics: %+v", got)
	}
}
