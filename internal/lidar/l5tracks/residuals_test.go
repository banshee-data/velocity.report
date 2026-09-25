package l5tracks

import (
	"math"
	"testing"
	"time"
)

func TestWindowBaselineExcludesWarmupAndSurvivesTrackDeletion(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxMisses = 1
	cfg.MaxMissesConfirmed = 1
	tracker := NewTracker(cfg)
	now := time.Unix(100, 0)
	cluster := WorldCluster{ClusterID: 1, CentroidX: 10, CentroidY: 3, PointsCount: 20}
	tracker.Update([]WorldCluster{cluster}, now)
	tracker.Update([]WorldCluster{cluster}, now.Add(100*time.Millisecond))
	if len(tracker.GetWindowBaseline().Residuals) != 0 {
		t.Fatal("warm-up was counted before the window opened")
	}
	var track *TrackedObject
	for _, v := range tracker.Tracks {
		track = v
	}
	before := track.ObservationCount
	tracker.BeginTrackingBaseline()
	if track.ObservationCount != before {
		t.Fatal("begin reset tracker state")
	}
	tracker.Update([]WorldCluster{cluster}, now.Add(200*time.Millisecond))
	got := tracker.GetWindowBaseline()
	if len(got.Residuals) != 1 || got.Residuals[0].Count != 1 || got.Association[0].Matched != 1 {
		t.Fatalf("scoring window = %+v", got)
	}
	tracker.Update(nil, now.Add(300*time.Millisecond))
	if track.TrackState != TrackDeleted {
		t.Fatal("expected terminal miss")
	}
	delete(tracker.Tracks, track.TrackID)
	got = tracker.GetWindowBaseline()
	if got.Residuals[0].Count != 1 || got.Association[0].Missed != 1 {
		t.Fatalf("terminal contribution lost: %+v", got)
	}
	tracker.BeginTrackingBaseline()
	if len(tracker.GetWindowBaseline().Residuals) != 0 || len(tracker.GetWindowBaseline().Association) != 0 {
		t.Fatal("new window inherited old counts")
	}
}

func TestWindowBaselineCountsEmptyFramesAndResetDisablesIt(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxMisses = 1
	tracker := NewTracker(cfg)
	now := time.Unix(100, 0)
	tracker.Update([]WorldCluster{{ClusterID: 1, CentroidX: 10, PointsCount: 20}}, now)
	tracker.RecordBaselineEmptyFrame() // Disabled during warm-up.
	tracker.BeginTrackingBaseline()
	tracker.RecordBaselineEmptyFrame()
	tracker.AdvanceMisses(now.Add(time.Second))
	tracker.RecordBaselineEmptyFrame() // Deleted tracks are not opportunities.
	tracker.AdvanceMisses(now.Add(2 * time.Second))
	got := tracker.GetWindowBaseline()
	if len(got.Association) != 1 || got.Association[0].Missed != 2 || got.Association[0].Matched != 0 {
		t.Fatalf("empty/terminal frame denominator: %+v", got)
	}
	tracker.Reset()
	if tracker.baselineEnabled || len(tracker.GetWindowBaseline().Association) != 0 {
		t.Fatal("Reset retained baseline")
	}
}

// identity inverse covariance, so NIS reduces to the squared innovation
// magnitude and the arithmetic is checkable by hand.
func observeUnit(b *ResidualBands, yX, yY, vx, vy float32) {
	b.Observe(yX, yY, vx, vy, 1, 0, 0, 1)
}

func TestResidualBandsSplitByDirectionOfTravel(t *testing.T) {
	var b ResidualBands
	// Travelling along +X at 10 m/s; an innovation purely in +Y is entirely
	// lateral, and one purely in +X entirely longitudinal.
	observeUnit(&b, 0, 0.5, 10, 0)
	observeUnit(&b, 0.4, 0, 10, 0)

	s := b.Summarise()
	if len(s) != 1 {
		t.Fatalf("summarised %d bands, want 1", len(s))
	}
	band := s[0]
	if band.SpeedFloorMps != 10 {
		t.Fatalf("landed in the %g band", band.SpeedFloorMps)
	}
	// Tolerance reflects float32 innovations accumulated into float64 sums.
	if math.Abs(band.LateralRMSMetres-math.Sqrt(0.25/2)) > 1e-6 {
		t.Fatalf("lateral RMS = %v", band.LateralRMSMetres)
	}
	if math.Abs(band.LongitudinalRMSMetres-math.Sqrt(0.16/2)) > 1e-6 {
		t.Fatalf("longitudinal RMS = %v", band.LongitudinalRMSMetres)
	}
}

// A consistent filter's NIS averages the measurement's degrees of freedom.
// With an identity inverse covariance an innovation of magnitude sqrt(2) is
// exactly on that expectation, which is what makes the number readable.
func TestNISMatchesTheInnovationAgainstItsCovariance(t *testing.T) {
	var b ResidualBands
	observeUnit(&b, 1, 1, 10, 0) // NIS = 1 + 1 = 2

	s := b.Summarise()
	if math.Abs(s[0].MeanNIS-2) > 1e-9 {
		t.Fatalf("mean NIS = %v, want 2", s[0].MeanNIS)
	}
	if s[0].NISExceedanceRatio != 0 {
		t.Fatalf("a consistent observation exceeded the 95%% bound")
	}

	var over ResidualBands
	observeUnit(&over, 3, 3, 10, 0) // NIS = 18, well past 5.991
	if got := over.Summarise()[0].NISExceedanceRatio; got != 1 {
		t.Fatalf("exceedance = %v, want 1", got)
	}
}

// Below the decomposition speed the direction of travel is estimator noise.
// The lateral figure must be reported as absent rather than as zero error.
func TestSlowTracksReportNoLateralComponent(t *testing.T) {
	var b ResidualBands
	observeUnit(&b, 0.3, 0.4, 0.1, 0) // 0.5 m magnitude, near-stationary

	s := b.Summarise()
	if s[0].Decomposed != 0 {
		t.Fatalf("decomposed %d observations below the threshold", s[0].Decomposed)
	}
	if s[0].LateralRMSMetres != 0 || s[0].LateralBiasMetres != 0 {
		t.Fatal("a lateral value was invented for a track with no usable heading")
	}
	// The magnitude is not discarded: it goes to the longitudinal slot.
	if math.Abs(s[0].LongitudinalRMSMetres-0.5) > 1e-6 {
		t.Fatalf("longitudinal RMS = %v, want 0.5", s[0].LongitudinalRMSMetres)
	}
}

// Undecomposed observations must not dilute the lateral RMS toward zero: the
// average is over the observations that actually have a lateral component.
func TestLateralAveragesOverDecomposedObservationsOnly(t *testing.T) {
	var b ResidualBands
	// One fast observation with a 1 m lateral error, then nine slow ones in
	// the same band. Averaging over all ten would report 0.32 m.
	observeUnit(&b, 0, 1, 3, 0)
	for i := 0; i < 9; i++ {
		observeUnit(&b, 0.01, 0, 2.5, 0)
	}
	s := b.Summarise()
	if s[0].Decomposed != 10 {
		t.Fatalf("decomposed = %d, want 10: all ten are above the threshold", s[0].Decomposed)
	}

	var mixed ResidualBands
	mixed.Observe(0, 1, 3, 0, 1, 0, 0, 1)   // decomposed, 1 m lateral
	mixed.Observe(0, 1, 0.1, 0, 1, 0, 0, 1) // slow: lands in band 0
	first := mixed.Summarise()[0]
	if first.Decomposed != 0 || first.LateralRMSMetres != 0 {
		t.Fatalf("slow band reported lateral evidence: %+v", first)
	}
}

func TestResidualBandBoundaries(t *testing.T) {
	for speed, want := range map[float32]int{0: 0, 1.9: 0, 2: 1, 4.9: 1, 5: 2, 10: 3, 15: 4, 40: 4} {
		if got := residualBand(speed); got != want {
			t.Errorf("speed %g landed in band %d, want %d", speed, got, want)
		}
	}
}

func TestNonFiniteInnovationIsNotRecorded(t *testing.T) {
	var b ResidualBands
	nan := float32(math.NaN())
	b.Observe(nan, 0, 10, 0, 1, 0, 0, 1)
	b.Observe(0, 0, 10, 0, float32(math.Inf(1)), 0, 0, 1)
	if len(b.Summarise()) != 0 {
		t.Fatal("a non-finite innovation entered the baseline")
	}
}

func TestResidualBandsAddIsAdditive(t *testing.T) {
	var a, b ResidualBands
	observeUnit(&a, 1, 1, 10, 0)
	observeUnit(&b, 1, 1, 10, 0)
	a.Add(b)
	s := a.Summarise()
	if s[0].Count != 2 || s[0].Decomposed != 2 {
		t.Fatalf("counts did not add: %+v", s[0])
	}
	if math.Abs(s[0].MeanNIS-2) > 1e-9 {
		t.Fatalf("mean NIS changed on rollup: %v", s[0].MeanNIS)
	}
}

// An empty band is omitted rather than reported as a perfect score for a speed
// nothing travelled at.
func TestEmptyBandsAreOmitted(t *testing.T) {
	var b ResidualBands
	observeUnit(&b, 1, 0, 12, 0)
	s := b.Summarise()
	if len(s) != 1 || s[0].SpeedFloorMps != 10 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestAssociationBandsCountBySpeed(t *testing.T) {
	var a AssociationBands
	a.Observe(3, true)
	a.Observe(3, true)
	a.Observe(3, false)
	a.Observe(20, false)

	s := a.Summarise()
	if len(s) != 2 {
		t.Fatalf("summarised %d bands, want 2", len(s))
	}
	if s[0].Matched != 2 || s[0].Missed != 1 || math.Abs(s[0].Rate-2.0/3) > 1e-9 {
		t.Fatalf("slow band = %+v", s[0])
	}
	if s[1].Rate != 0 {
		t.Fatalf("fast band rate = %v, want 0", s[1].Rate)
	}

	var other AssociationBands
	other.Observe(3, true)
	a.Add(other)
	if got := a.Summarise()[0].Matched; got != 3 {
		t.Fatalf("matched = %d after rollup, want 3", got)
	}
}
