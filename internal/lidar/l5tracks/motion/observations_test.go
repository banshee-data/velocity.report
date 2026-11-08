package motion

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func filterStep(id string, seq, nanos int64, x, y float32) l5tracks.FilterStep {
	return l5tracks.FilterStep{
		TrackID: id, CreationSequence: seq, FrameUnixNanos: nanos, StateUnixNanos: nanos,
		Linked: true, Observed: true,
		Observation: l5tracks.FilterObservation{
			ClusterID: seq*100 + nanos/frameNanos, MeasurementUnixNanos: nanos, X: x, Y: y,
			NIS: 1.5, HasInnovation: true,
			GeometryCovariance: l5tracks.MeasurementCovariance{XX: 0.2, XY: 0.05, YY: 0.1},
		},
	}
}

// The adapter keeps applied observations in capture order per chain, and
// drops coasted, refused and unapplied steps and repeated capture times.
func TestObservationsFromFilterFrames(t *testing.T) {
	founded := filterStep("a", 1, 0, 1, 2)
	founded.Linked, founded.Observation.HasInnovation = false, false
	coasted := filterStep("a", 1, frameNanos, 0, 0)
	coasted.Observed = false
	refused := filterStep("a", 1, 2*frameNanos, 0, 0)
	refused.Observed, refused.UpdateRefused = false, true
	unapplied := filterStep("a", 1, 3*frameNanos, 9, 9)
	unapplied.Observation.Disposition = l5tracks.ResidualNotApplied
	observed := filterStep("a", 1, 4*frameNanos, 1.5, 2.25)
	repeat := filterStep("a", 1, 4*frameNanos, 7, 7)
	reused := filterStep("a", 2, 4*frameNanos, 5, 5) // same ID, new chain
	other := filterStep("b", 3, 4*frameNanos, -1, 0)
	other.Observation.MeasurementUnixNanos, other.StateUnixNanos = 0, 4*frameNanos-5

	frames := []l5tracks.FilterFrame{
		{FrameUnixNanos: 0, Steps: []l5tracks.FilterStep{founded}},
		{FrameUnixNanos: frameNanos, Steps: []l5tracks.FilterStep{coasted}},
		{FrameUnixNanos: 2 * frameNanos, Steps: []l5tracks.FilterStep{refused}},
		{FrameUnixNanos: 3 * frameNanos, Steps: []l5tracks.FilterStep{unapplied}},
		{FrameUnixNanos: 4 * frameNanos, Steps: []l5tracks.FilterStep{observed, repeat, reused, other}},
	}
	tracks := ObservationsFromFilterFrames(frames, AdapterConfig{})
	if len(tracks) != 3 {
		t.Fatalf("got %d chains, want 3: %+v", len(tracks), tracks)
	}
	a := tracks[0]
	if a.TrackID != "a" || a.CreationSequence != 1 || len(a.Observations) != 2 || a.Skipped != 1 {
		t.Fatalf("chain a: %+v", a)
	}
	first, second := a.Observations[0], a.Observations[1]
	if first.UnixNanos != 0 || first.X != 1 || first.Y != 2 || first.HasLiveNIS {
		t.Errorf("founding observation %+v", first)
	}
	if second.UnixNanos != 4*frameNanos || second.X != 1.5 || second.Y != 2.25 || !second.HasLiveNIS || second.LiveNIS != 1.5 {
		t.Errorf("observed step %+v", second)
	}
	if second.R != IsotropicR(DefaultMeasurementVariance) {
		t.Errorf("default R %v", second.R)
	}
	if tracks[1].CreationSequence != 2 || len(tracks[1].Observations) != 1 {
		t.Errorf("reused-ID chain %+v", tracks[1])
	}
	if b := tracks[2]; b.Observations[0].UnixNanos != 4*frameNanos-5 {
		t.Errorf("time fallback to the state time: %+v", b.Observations[0])
	}

	geo := ObservationsFromFilterFrames(frames, AdapterConfig{UseGeometryCovariance: true, MeasurementVariance: 0.08})
	r := geo[0].Observations[1].R
	if r[0][0] != float64(float32(0.2)) || r[0][1] != float64(float32(0.05)) || r[1][1] != float64(float32(0.1)) {
		t.Errorf("geometry R %v", r)
	}
	// The founding observation has no valid geometry covariance here (zero
	// would be singular), so it keeps the scalar.
	founded.Observation.GeometryCovariance = l5tracks.MeasurementCovariance{}
	zero := ObservationsFromFilterFrames([]l5tracks.FilterFrame{{Steps: []l5tracks.FilterStep{founded}}},
		AdapterConfig{UseGeometryCovariance: true, MeasurementVariance: 0.08})
	if zero[0].Observations[0].R != IsotropicR(0.08) {
		t.Errorf("invalid geometry covariance not replaced by the scalar: %v", zero[0].Observations[0].R)
	}

	// The collected sequence runs through every estimator.
	if _, err := RunFilter(DefaultCAConfig(), a.Observations); err != nil {
		t.Fatal(err)
	}
	if _, err := RunIMM(DefaultIMMConfig(), a.Observations); err != nil {
		t.Fatal(err)
	}
}
