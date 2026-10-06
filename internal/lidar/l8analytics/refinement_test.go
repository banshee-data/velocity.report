package l8analytics

import (
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func refinedState(seq int64, secs float64, observed, confirmed bool, online, smoothed l5tracks.FilterMoments) l5tracks.SmoothedState {
	nanos := int64(secs * 1e9)
	s := l5tracks.SmoothedState{
		CreationSequence: seq, FrameUnixNanos: nanos, StateUnixNanos: nanos, Observed: observed, Confirmed: confirmed,
		Online: online, Smoothed: smoothed, ReleasedAtUnixNanos: nanos + 300_000_000,
	}
	dx, dy := float64(smoothed.X-online.X), float64(smoothed.Y-online.Y)
	s.Revision.PositionMetres = math.Hypot(dx, dy)
	s.Revision.VelocityMps = math.Hypot(float64(smoothed.VX-online.VX), float64(smoothed.VY-online.VY))
	if observed {
		s.Observation = l5tracks.FilterObservation{X: online.X, Y: online.Y + 0.1, NIS: 1.5, HasInnovation: true}
	}
	return s
}

func moments(x, y, vx, vy float32) l5tracks.FilterMoments {
	return l5tracks.FilterMoments{X: x, Y: y, VX: vx, VY: vy, P: [16]float32{0.01, 0, 0, 0, 0, 0.01, 0, 0, 0, 0, 0.1, 0, 0, 0, 0, 0.1}}
}

func TestQuantilesUseNearestRank(t *testing.T) {
	values := []float64{5, 1, 4, 2, 3, 6, 7, 8, 9, 10}
	q := quantilesOf(values)
	if q.N != 10 || q.P50 != 5 || q.P95 != 10 || q.P99 != 10 || q.Max != 10 {
		t.Fatalf("%+v", q)
	}
	if (quantilesOf(nil) != Quantiles{}) {
		t.Fatal("empty sample must summarise to zero")
	}
}

// The accumulator scores only confirmed states from the scoring start, keeps
// observed and coasted revisions apart, and derives every figure from the
// arm's own moments.
func TestRefinementAccumulatorScoresTheDeclaredPopulation(t *testing.T) {
	const r = 0.05
	refined := NewRefinementAccumulator("fixed_lag 3f", "fixed_lag", "3f", r, int64(1e9))
	online := NewOnlineAccumulator(r, int64(1e9))
	states := []l5tracks.SmoothedState{
		refinedState(1, 0.9, true, true, moments(0, 0, 10, 0), moments(0, 0.5, 10, 0)),   // before scoring start
		refinedState(1, 1.0, true, false, moments(1, 0, 10, 0), moments(1, 0.5, 10, 0)),  // tentative
		refinedState(1, 1.1, true, true, moments(2, 0, 10, 0), moments(2, 0.4, 10, 0)),   // observed, flagged
		refinedState(1, 1.2, false, true, moments(3, 0, 10, 0), moments(3, 0.6, 10, 0)),  // coasted, flagged
		refinedState(1, 1.3, true, true, moments(4, 0, 10, 0), moments(4.05, 0, 12, 0)),  // observed, small
		refinedState(2, 1.3, true, true, moments(50, 0, 0.5, 0), moments(50, 0, 0.5, 0)), // slow: no heading
	}
	for _, s := range states {
		refined.Add(s)
		online.Add(s)
	}
	m := refined.Metrics()
	if m.States != 4 || m.ObservedStates != 3 || m.Tracks != 2 {
		t.Fatalf("population: %+v", m)
	}
	if m.RevisionsFlagged != 1 || m.CoastedRevisionsFlagged != 1 || m.RevisionPositionM.N != 3 || m.CoastedRevisionPositionM.N != 1 {
		t.Fatalf("revision split: flagged %d/%d over %d/%d", m.RevisionsFlagged, m.CoastedRevisionsFlagged,
			m.RevisionPositionM.N, m.CoastedRevisionPositionM.N)
	}
	if math.Abs(m.RevisionPositionM.Max-0.4) > 1e-6 || math.Abs(m.PerFrameMaxRevisionP99M-0.4) > 1e-6 {
		t.Fatalf("revision maximum %.3f, frame-max p99 %.3f", m.RevisionPositionM.Max, m.PerFrameMaxRevisionP99M)
	}
	if math.Abs(m.FinalityDelaySecs.P50-0.3) > 1e-9 || m.LagReleaseDelaySecs.N != 0 {
		t.Fatalf("finality delay %+v, lag-release delay %+v", m.FinalityDelaySecs, m.LagReleaseDelaySecs)
	}
	// Lateral residual: the measurement sits 0.1 m to the left of the online
	// position; the 1.1 s refined state is 0.3 m left of it. Heading is +X.
	if m.ResidualLateralM.N != 2 || math.Abs(m.ResidualLateralM.Max-0.3) > 1e-6 {
		t.Fatalf("lateral residuals %+v", m.ResidualLateralM)
	}
	// Speed step: 10 → 10 → 12 m/s over 0.1 s steps on track 1.
	if want := math.Sqrt((0 + 20*20) / 2.0); math.Abs(m.SpeedStepRMSMps2-want) > 1e-3 {
		t.Fatalf("speed step RMS %.3f, want %.3f", m.SpeedStepRMSMps2, want)
	}
	// CV prediction: the 1.1 s estimate predicts 4.0 at 1.3 s along X, and the
	// measurement is at 4.0, 0.1: lateral error 0.3 (0.4 predicted Y minus 0.1).
	if m.CVPredictionErrorM.N != 1 || math.Abs(m.CVPredictionErrorLateralM.Max-0.3) > 1e-5 {
		t.Fatalf("CV prediction %+v lateral %+v", m.CVPredictionErrorM, m.CVPredictionErrorLateralM)
	}

	o := online.Metrics()
	if o.States != 4 || o.RevisionPositionM.N != 0 || o.FinalityDelaySecs.Max != 0 || o.InnovationNISMean != 1.5 {
		t.Fatalf("online arm: %+v", o)
	}
	// The online posterior residual is 0.1 m against R - P = 0.04: q = 0.25.
	if math.Abs(o.NormalisedResidualMean-0.25) > 1e-4 || o.NormalisedResidualSkipped != 0 {
		t.Fatalf("normalised residual mean %.4f, skipped %d", o.NormalisedResidualMean, o.NormalisedResidualSkipped)
	}
}

// A covariance larger than R leaves the smoothed-residual covariance
// indefinite; the state is counted, not scored.
func TestNormalisedResidualSkipsAnIndefiniteCovariance(t *testing.T) {
	var p [16]float32
	p[0], p[5] = 0.2, 0.2
	if _, ok := normalisedResidual(0.1, 0.1, p, 0.05); ok {
		t.Fatal("scored an indefinite residual covariance")
	}
}

func TestRefinementComparisonMarkdown(t *testing.T) {
	table := RefinementComparisonMarkdown([]RefinementArmMetrics{{Label: "online"}, {Label: "fixed_lag 3f", States: 10, ObservedStates: 8}})
	lines := strings.Split(strings.TrimSpace(table), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[3], "| fixed_lag 3f | 8 / 2 |") {
		t.Fatalf("table:\n%s", table)
	}
}
