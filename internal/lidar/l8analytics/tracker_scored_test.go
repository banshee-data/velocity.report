package l8analytics

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The real tracker, driven with scripted clusters, scored against known truth.
//
// This is the end-to-end check the unit tests cannot give: the metrics are
// exercised on output an actual Tracker produced, with its real association,
// gating and lifecycle. It is also how an association claim stops being an
// argument. Gap-analysis row S3 says the shipped cost lets a coasting track
// outbid a freshly updated one; whether that costs identities on a crossing is
// a number, and this is where it is counted.
//
// L1 to L4 are not involved. Clusters are placed exactly on the true object
// positions, so every error the metrics report is L5's.

// scenarioCluster builds a vehicle-sized cluster centred on an object.
func scenarioCluster(state SyntheticObjectState) l5tracks.WorldCluster {
	x, y := float32(state.X), float32(state.Y)
	return l5tracks.WorldCluster{
		CentroidX: x, CentroidY: y,
		BoundingBoxLength: float32(state.LengthMetres),
		BoundingBoxWidth:  float32(state.WidthMetres),
		BoundingBoxHeight: 1.5,
		PointsCount:       180,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: x, CenterY: y,
			Length: float32(state.LengthMetres), Width: float32(state.WidthMetres), Height: 1.5,
		},
	}
}

// runScenario drives a tracker through a scenario and returns its output as
// hypothesis series keyed by creation sequence, never the random track ID.
func runScenario(t *testing.T, cfg l5tracks.TrackerConfig, scenario SyntheticScenario) []TrackSeries {
	t.Helper()
	tracker := l5tracks.NewTracker(cfg)

	// Drive at the scenario's own absolute timestamps: the metrics match on
	// exact frame time, so truth and hypothesis must share a clock.
	for frame := 0; frame < scenario.FrameCount(); frame++ {
		var clusters []l5tracks.WorldCluster
		for _, state := range scenario.At(frame) {
			clusters = append(clusters, scenarioCluster(state))
		}
		tracker.Update(clusters, time.Unix(0, scenario.FrameTimestamp(frame)))
	}

	byID := map[string]*TrackSeries{}
	for id, track := range tracker.Tracks {
		key := fmt.Sprintf("seq-%04d", track.CreationSequence)
		series := &TrackSeries{ID: key}
		for _, p := range track.History {
			series.Points = append(series.Points, SeriesPoint{
				TimestampNanos: p.Timestamp, X: p.X, Y: p.Y,
			})
		}
		if len(series.Points) > 0 {
			byID[id] = series
		}
	}
	out := make([]TrackSeries, 0, len(byID))
	for _, s := range byID {
		sort.Slice(s.Points, func(i, j int) bool { return s.Points[i].TimestampNanos < s.Points[j].TimestampNanos })
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// A well-separated pair must be tracked cleanly: this establishes that the
// harness itself does not manufacture errors before it is used to attribute
// any.
func TestTrackerScoresCleanlyOnAWellSeparatedPair(t *testing.T) {
	scenario := CrossingScenario(40, 12.0, 1.0) // never closer than 12 m
	truth := scenario.Truth()
	hyp := runScenario(t, l5tracks.DefaultTrackerConfig(), scenario)

	m := ComputeTrackMetrics(truth, hyp, 2.0)
	if m.IDSwitches != 0 {
		t.Errorf("IDSwitches = %d on a 12 m separation, want 0: %+v", m.IDSwitches, m)
	}
	if m.Fragmentations != 0 {
		t.Errorf("Fragmentations = %d, want 0", m.Fragmentations)
	}
	// Confirmation costs the first few frames of each track, so recall is high
	// rather than perfect; the point is that association is clean.
	if m.MOTA < 0.5 {
		t.Errorf("MOTA = %.3f, want above 0.5 on an easy scene: %+v", m.MOTA, m)
	}
	t.Logf("well-separated: %+v", m)
}

// The measurement the S3 row asks for. Two objects pass close together; the
// question is whether identities survive, and whether the likelihood cost
// changes the answer. Both arms are reported rather than asserted, because the
// point is to produce the number, not to presume it.
func TestCrossingIdentitiesUnderBothAssociationCosts(t *testing.T) {
	for _, closest := range []float64{1.0, 2.0, 3.0} {
		scenario := CrossingScenario(40, closest, 1.0)
		truth := scenario.Truth()

		shipped := l5tracks.DefaultTrackerConfig()
		likelihood := l5tracks.DefaultTrackerConfig()
		likelihood.LikelihoodAssociationCost = true

		mShipped := ComputeTrackMetrics(truth, runScenario(t, shipped, scenario), 2.0)
		mLikelihood := ComputeTrackMetrics(truth, runScenario(t, likelihood, scenario), 2.0)
		hShipped := ComputeHOTA(truth, runScenario(t, shipped, scenario), 2.0, nil)
		hLikelihood := ComputeHOTA(truth, runScenario(t, likelihood, scenario), 2.0, nil)

		t.Logf("closest=%.1fm shipped:    IDSW=%d FM=%d FN=%d FP=%d MOTA=%.3f DetA=%.3f AssA=%.3f",
			closest, mShipped.IDSwitches, mShipped.Fragmentations, mShipped.FN, mShipped.FP,
			mShipped.MOTA, hShipped.DetA, hShipped.AssA)
		t.Logf("closest=%.1fm likelihood: IDSW=%d FM=%d FN=%d FP=%d MOTA=%.3f DetA=%.3f AssA=%.3f",
			closest, mLikelihood.IDSwitches, mLikelihood.Fragmentations, mLikelihood.FN, mLikelihood.FP,
			mLikelihood.MOTA, hLikelihood.DetA, hLikelihood.AssA)

		// The option must not make association worse. It may legitimately leave
		// it unchanged: whether it helps is what the measurement is for.
		if mLikelihood.IDSwitches > mShipped.IDSwitches {
			t.Errorf("closest=%.1fm: likelihood cost raised identity switches from %d to %d",
				closest, mShipped.IDSwitches, mLikelihood.IDSwitches)
		}
	}
}

// Coasting through an occlusion and re-acquiring the same object should not
// read as a new object. This is the continuity half of sprint 0.5.2.2, and the
// metric that will judge it.
func TestOcclusionCoastIsMeasurable(t *testing.T) {
	for _, gap := range []int{2, 5, 12} {
		scenario := OcclusionScenario(40, 15, gap, 1.0)
		truth := scenario.Truth()
		m := ComputeTrackMetrics(truth, runScenario(t, l5tracks.DefaultTrackerConfig(), scenario), 2.0)
		t.Logf("gap=%2d frames: IDSW=%d FM=%d FN=%d MOTA=%.3f", gap, m.IDSwitches, m.Fragmentations, m.FN, m.MOTA)

		// The truth omits the occluded frames, so a coasting tracker is not
		// credited or penalised for them; what is measured is whether the same
		// identity resumes afterwards.
		if gap <= 2 && m.IDSwitches != 0 {
			t.Errorf("gap=%d: identity lost across a %d-frame occlusion (IDSW=%d)", gap, gap, m.IDSwitches)
		}
	}
}

// The S3 measurement. A track coasts through an occlusion while a neighbour
// keeps producing returns; the question is whether the coasting track takes the
// neighbour's cluster, and whether the likelihood cost prevents it.
//
// Both arms are reported. The assertion is only that the option never makes
// association worse, because the number is the point.
func TestContestedClusterUnderBothAssociationCosts(t *testing.T) {
	type arm struct {
		name string
		cfg  l5tracks.TrackerConfig
	}
	shipped := l5tracks.DefaultTrackerConfig()
	likelihood := l5tracks.DefaultTrackerConfig()
	likelihood.LikelihoodAssociationCost = true

	worseAt := 0
	for _, sep := range []float64{1.5, 2.5, 4.0} {
		for _, gap := range []int{3, 8} {
			scenario := ContestedScenario(40, sep, 15, gap, 1.0)
			truth := scenario.Truth()
			results := map[string]TrackMetrics{}
			for _, a := range []arm{{"shipped", shipped}, {"likelihood", likelihood}} {
				m := ComputeTrackMetrics(truth, runScenario(t, a.cfg, scenario), 2.0)
				results[a.name] = m
				t.Logf("sep=%.1fm gap=%d %-10s IDSW=%d FM=%d FN=%d FP=%d MOTA=%.3f",
					sep, gap, a.name, m.IDSwitches, m.Fragmentations, m.FN, m.FP, m.MOTA)
			}
			if results["likelihood"].IDSwitches > results["shipped"].IDSwitches {
				worseAt++
				t.Errorf("sep=%.1fm gap=%d: likelihood cost raised identity switches from %d to %d",
					sep, gap, results["shipped"].IDSwitches, results["likelihood"].IDSwitches)
			}
		}
	}
	if worseAt == 0 {
		t.Log("likelihood cost never increased identity switches on these scenarios")
	}
}

// The decisive S3 geometry: a coasting track whose prediction converges onto a
// neighbour's cluster, so the assignment must choose between it and a freshly
// updated track. If the bias costs an identity anywhere, it costs one here.
func TestConvergingContestedClusterUnderBothAssociationCosts(t *testing.T) {
	shipped := l5tracks.DefaultTrackerConfig()
	likelihood := l5tracks.DefaultTrackerConfig()
	likelihood.LikelihoodAssociationCost = true

	shippedSwitches, likelihoodSwitches := 0, 0
	for _, sep := range []float64{2.0, 3.5} {
		for _, gap := range []int{4, 10} {
			scenario := ConvergingContestedScenario(44, sep, 18, gap, 1.0)
			truth := scenario.Truth()
			ms := ComputeTrackMetrics(truth, runScenario(t, shipped, scenario), 2.0)
			ml := ComputeTrackMetrics(truth, runScenario(t, likelihood, scenario), 2.0)
			shippedSwitches += ms.IDSwitches
			likelihoodSwitches += ml.IDSwitches

			t.Logf("sep=%.1fm gap=%2d shipped    IDSW=%d FM=%d FN=%d FP=%d MOTA=%.3f",
				sep, gap, ms.IDSwitches, ms.Fragmentations, ms.FN, ms.FP, ms.MOTA)
			t.Logf("sep=%.1fm gap=%2d likelihood IDSW=%d FM=%d FN=%d FP=%d MOTA=%.3f",
				sep, gap, ml.IDSwitches, ml.Fragmentations, ml.FN, ml.FP, ml.MOTA)

			if ml.IDSwitches > ms.IDSwitches {
				t.Errorf("sep=%.1fm gap=%d: likelihood cost raised identity switches from %d to %d",
					sep, gap, ms.IDSwitches, ml.IDSwitches)
			}
		}
	}
	t.Logf("total identity switches: shipped %d, likelihood %d", shippedSwitches, likelihoodSwitches)
	if shippedSwitches == 0 {
		t.Log("the shipped cost lost no identities even in the converging geometry: " +
			"S3's association-level bias is real but did not reach track identity in this scene class")
	}
}
