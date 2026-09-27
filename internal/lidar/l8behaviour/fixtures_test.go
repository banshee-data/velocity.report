package l8behaviour

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// fixtureTolerance absorbs the last-bit differences between this package's
// trigonometry and the external computation that produced the literals. The
// exactly representable fixtures match to the bit regardless.
const fixtureTolerance = 1e-12

func evaluateExpected(t *testing.T, f Fixture, e ExpectedFollowing) FollowingPoint {
	t.Helper()
	lt, ok := f.Trajectory(e.LeaderTrackID)
	if !ok {
		t.Fatalf("%s: no trajectory %s", f.Name, e.LeaderTrackID)
	}
	ft, ok := f.Trajectory(e.FollowerTrackID)
	if !ok {
		t.Fatalf("%s: no trajectory %s", f.Name, e.FollowerTrackID)
	}
	leader, ok := PartyAt(lt, e.CaptureUnixNanos)
	if !ok {
		t.Fatalf("%s: leader has no sample at %d", f.Name, e.CaptureUnixNanos)
	}
	follower, ok := PartyAt(ft, e.CaptureUnixNanos)
	if !ok {
		t.Fatalf("%s: follower has no sample at %d", f.Name, e.CaptureUnixNanos)
	}
	pt, err := EvaluateFollowing(f.Path, leader, follower, f.Params)
	if err != nil {
		t.Fatalf("%s: evaluate: %v", f.Name, err)
	}
	return pt
}

func assertValue(t *testing.T, what string, m Measurement, want *ExpectedValue, reason SuppressionReason) {
	t.Helper()
	if err := m.Validate(); err != nil {
		t.Fatalf("%s: invalid measurement: %v", what, err)
	}
	if want == nil {
		if !m.Suppressed || m.Value != nil || m.Reason != reason {
			t.Fatalf("%s: want suppression %s, got suppressed=%v reason=%s value=%v", what, reason, m.Suppressed, m.Reason, m.Value)
		}
		return
	}
	if m.Suppressed || m.Value == nil {
		t.Fatalf("%s: want a value, got suppression %s", what, m.Reason)
	}
	if math.Abs(*m.Value-want.Value) > fixtureTolerance {
		t.Errorf("%s value = %.17g, want %.17g", what, *m.Value, want.Value)
	}
	if m.Uncertainty == nil || m.Uncertainty.Sigma == nil {
		t.Fatalf("%s: no sigma", what)
	}
	if math.Abs(*m.Uncertainty.Sigma-want.Sigma) > fixtureTolerance {
		t.Errorf("%s sigma = %.17g, want %.17g", what, *m.Uncertainty.Sigma, want.Sigma)
	}
	if m.Uncertainty.Method != MethodLinearised || m.Uncertainty.Kind != UncertaintySigma {
		t.Errorf("%s: uncertainty %s/%s, want sigma/linearised", what, m.Uncertainty.Kind, m.Uncertainty.Method)
	}
}

func TestFixturesMatchFrozenExpectations(t *testing.T) {
	for _, f := range Fixtures() {
		t.Run(f.Name, func(t *testing.T) {
			if len(f.Expected) == 0 {
				t.Fatal("fixture has no expectations")
			}
			for _, e := range f.Expected {
				pt := evaluateExpected(t, f, e)
				what := e.LeaderTrackID + "->" + e.FollowerTrackID

				if e.Gap == nil {
					if pt.Gap != nil {
						t.Fatalf("%s: unexpected gap arithmetic %+v", what, *pt.Gap)
					}
				} else {
					if pt.Gap == nil {
						t.Fatalf("%s: no gap arithmetic", what)
					}
					if math.Abs(pt.Gap.ValueM-e.Gap.Value) > fixtureTolerance ||
						math.Abs(pt.Gap.SigmaM-e.Gap.Sigma) > fixtureTolerance {
						t.Errorf("%s gap = %.17g +- %.17g, want %.17g +- %.17g",
							what, pt.Gap.ValueM, pt.Gap.SigmaM, e.Gap.Value, e.Gap.Sigma)
					}
					if pt.Gap.LeaderSource != e.LeaderSource || pt.Gap.FollowerSource != e.FollowerSource {
						t.Errorf("%s sources = %s/%s, want %s/%s", what,
							pt.Gap.LeaderSource, pt.Gap.FollowerSource, e.LeaderSource, e.FollowerSource)
					}
				}

				var gapWant *ExpectedValue
				if e.SpatialGapReason == ReasonUnspecified {
					gapWant = e.Gap
				}
				assertValue(t, what+" spatial gap", pt.SpatialGap, gapWant, e.SpatialGapReason)
				assertValue(t, what+" net time gap", pt.NetTimeGap, e.NetTimeGap, e.NetTimeGapReason)

				if e.PredictedGap != nil {
					if pt.PredictedGap == nil {
						t.Fatalf("%s: no predicted gap", what)
					}
					assertValue(t, what+" predicted gap", *pt.PredictedGap, e.PredictedGap, ReasonUnspecified)
				} else if pt.PredictedGap != nil {
					t.Errorf("%s: unexpected predicted gap %+v", what, *pt.PredictedGap)
				}
				if pt.SupportedOpportunity != e.SupportedOpportunity {
					t.Errorf("%s supported opportunity = %v, want %v", what, pt.SupportedOpportunity, e.SupportedOpportunity)
				}
				if pt.CoastAgeNanos != e.CoastAgeNanos {
					t.Errorf("%s coast age = %d, want %d", what, pt.CoastAgeNanos, e.CoastAgeNanos)
				}
				if pt.SpatialGap.Provenance.Version.EstimateStage != StageFinal ||
					pt.SpatialGap.Provenance.Version.GeometryID != f.Path.ID {
					t.Errorf("%s provenance = %+v", what, pt.SpatialGap.Provenance.Version)
				}
			}
		})
	}
}

// TestNoSuppressedValueIsZero is the contract's central rule, checked over
// every measurement every fixture produces: a suppressed result carries no
// value at all, and its encoding has no value key to be read as zero.
func TestNoSuppressedValueIsZero(t *testing.T) {
	suppressed := 0
	for _, f := range Fixtures() {
		for _, e := range f.Expected {
			pt := evaluateExpected(t, f, e)
			ms := []Measurement{pt.SpatialGap, pt.NetTimeGap}
			if pt.PredictedGap != nil {
				ms = append(ms, *pt.PredictedGap)
			}
			for _, m := range ms {
				raw, err := json.Marshal(m)
				if err != nil {
					t.Fatalf("%s %s: marshal: %v", f.Name, m.Name, err)
				}
				if m.Suppressed {
					suppressed++
					if m.Value != nil || m.Uncertainty != nil {
						t.Errorf("%s %s: suppressed but carries a value or uncertainty", f.Name, m.Name)
					}
					if strings.Contains(string(raw), `"value"`) {
						t.Errorf("%s %s: suppressed encoding has a value key: %s", f.Name, m.Name, raw)
					}
					if !strings.Contains(string(raw), `"reason":"`+m.Reason.String()+`"`) {
						t.Errorf("%s %s: suppressed encoding lacks its reason: %s", f.Name, m.Name, raw)
					}
				} else if *m.Value == 0 {
					t.Errorf("%s %s: a supported value of exactly zero", f.Name, m.Name)
				}
			}
		}
	}
	if suppressed == 0 {
		t.Fatal("no fixture exercised a suppression")
	}
}

// TestFixtureSectionNinePointOneEquivalence checks the aligned fixture's sigma
// against Section 9.1 written out term by term, independently of endpoints.
func TestFixtureSectionNinePointOneEquivalence(t *testing.T) {
	f := FixtureAlignedPair()
	l, _ := f.Trajectory("trk_fixture_aligned_leader")
	fo, _ := f.Trajectory("trk_fixture_aligned_follower")
	ls, fs := l.Samples[0], fo.Samples[0]
	want := math.Sqrt(ls.Covariance[0] + fs.Covariance[0] +
		sq(ls.Length.SigmaMetres/2) + sq(fs.Length.SigmaMetres/2))
	pt := evaluateExpected(t, f, f.Expected[0])
	// Endpoint sigmas are square roots combined by hypot, so agreement is to
	// rounding rather than to the bit.
	if math.Abs(pt.Gap.SigmaM-want) > 4e-16 {
		t.Fatalf("aligned sigma = %.17g, Section 9.1 gives %.17g", pt.Gap.SigmaM, want)
	}
	// And the gap is bumper to bumper: centre to centre would be 10 m.
	if centre := ls.X - fs.X; pt.Gap.ValueM != centre-(ls.Length.Metres+fs.Length.Metres)/2 {
		t.Fatalf("aligned gap %.17g is not the centre distance less the half-lengths", pt.Gap.ValueM)
	}
}

func TestFixtureOcclusionExcludesCoastedTime(t *testing.T) {
	f := FixtureOcclusion()
	for id, want := range f.SupportSeconds {
		tr, ok := f.Trajectory(id)
		if !ok {
			t.Fatalf("no trajectory %s", id)
		}
		got, err := tr.SupportSeconds(f.SupportMaxIntervalNanos)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s support = %v, want %v", id, got, want)
		}
	}
	follower, _ := f.Trajectory("trk_fixture_occlusion_follower")
	seconds, _ := follower.SupportSeconds(f.SupportMaxIntervalNanos)
	if seconds.ExposureSeconds() != 0.9 || seconds.TotalSeconds() != 1.4 {
		t.Fatalf("follower exposure %v of %v, want 0.9 of 1.4", seconds.ExposureSeconds(), seconds.TotalSeconds())
	}

	supported, lastSigma := 0, 0.0
	for _, e := range f.Expected {
		pt := evaluateExpected(t, f, e)
		if pt.SupportedOpportunity {
			supported++
		}
		if pt.PredictedGap != nil {
			// The bound widens monotonically while the follower coasts.
			if s := *pt.PredictedGap.Uncertainty.Sigma; !(s > lastSigma) {
				t.Errorf("predicted sigma %v did not widen past %v", s, lastSigma)
			} else {
				lastSigma = s
			}
			if pt.PredictedGap.Provenance.Input.CoastedFrames != 1 {
				t.Errorf("predicted gap provenance %+v does not record the coasted sample", pt.PredictedGap.Provenance.Input)
			}
		}
	}
	// Ten observed instants: supported opportunity counts observed time only.
	if supported != 10 {
		t.Fatalf("supported instants = %d, want 10", supported)
	}
}

func TestFixtureAmbiguousPairIsGenuinelyAmbiguous(t *testing.T) {
	f := FixtureAmbiguousLeader()
	if len(f.Pairing) != 1 || f.Pairing[0].Reason != ReasonAmbiguousLeader || f.Pairing[0].LeaderTrackID != "" {
		t.Fatalf("pairing expectation = %+v", f.Pairing)
	}
	var gaps []GapEstimate
	for _, e := range f.Expected {
		pt := evaluateExpected(t, f, e)
		if pt.SpatialGap.Suppressed || !pt.SupportedOpportunity {
			t.Fatalf("%s: each candidate pair is individually well formed", e.LeaderTrackID)
		}
		for _, b := range []*BodyOnPath{pt.Leader, pt.Follower} {
			if math.Abs(b.LateralM) > f.Params.CorridorHalfWidthM {
				t.Fatalf("%s is outside the corridor", b.TrackID)
			}
		}
		gaps = append(gaps, *pt.Gap)
	}
	// Ambiguous in the documented sense: the gaps differ by less than either
	// gap's own one-sigma, so picking the nearer is picking noise.
	diff := math.Abs(gaps[0].ValueM - gaps[1].ValueM)
	if !(diff < math.Min(gaps[0].SigmaM, gaps[1].SigmaM)) {
		t.Fatalf("gap difference %v is not inside one sigma", diff)
	}
}

func TestFixtureDistractorIsNotALeader(t *testing.T) {
	f := FixtureLaneAdjacentDistractor()
	want := f.Pairing[0]
	var nearest string
	nearestGap := math.Inf(1)
	for _, e := range f.Expected {
		pt := evaluateExpected(t, f, e)
		if !pt.SpatialGap.Suppressed && pt.Gap.ValueM < nearestGap {
			nearest, nearestGap = e.LeaderTrackID, pt.Gap.ValueM
		}
	}
	// The distractor is nearer, but only the in-lane car yields a supported
	// gap, so the nearest supported leader is the expected one.
	if nearest != want.LeaderTrackID {
		t.Fatalf("nearest supported leader = %s, want %s", nearest, want.LeaderTrackID)
	}
}

func TestFixtureBandMembership(t *testing.T) {
	cases := []struct {
		fixture Fixture
		index   int
		below   [3]bool // 2.0, 1.5, 1.0 s
	}{
		{FixtureAlignedPair(), 0, [3]bool{true, true, false}},              // 1.15 s
		{FixtureLaneAdjacentDistractor(), 0, [3]bool{true, true, false}},   // 1.375 s
		{FixtureObliqueHeading(), 0, [3]bool{true, true, true}},            // 0.648 s
		{FixtureOffsetAnchors(), 0, [3]bool{true, true, false}},            // 1.15 s
		{FixtureAmbiguousLeader(), 1, [3]bool{true, true, true}},           // 0.5875 s
		{FixturePartialViews(), 1, [3]bool{true, true, true}},              // 0.575 s
		{FixtureOcclusion(), 0, [3]bool{true, true, true}},                 // 0.575 s
		{FixtureStandstill(), 0, [3]bool{false, false, false}},             // suppressed
		{FixturePartialViews(), 0, [3]bool{false, false, false}},           // suppressed
		{FixtureLaneAdjacentDistractor(), 1, [3]bool{false, false, false}}, // not a pair
	}
	for _, c := range cases {
		pt := evaluateExpected(t, c.fixture, c.fixture.Expected[c.index])
		for i, band := range FollowingBands() {
			// A suppressed time gap is in no band: suppressed time never
			// becomes a zero that falls below every threshold.
			got := !pt.NetTimeGap.Suppressed && band.Contains(*pt.NetTimeGap.Value)
			if got != c.below[i] {
				t.Errorf("%s[%d] below %.1f s = %v, want %v", c.fixture.Name, c.index, band.Seconds, got, c.below[i])
			}
		}
	}
}

func TestFixturesAreValidDeterministicAndSerialisable(t *testing.T) {
	a, b := Fixtures(), Fixtures()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("fixtures differ between two builds")
	}
	names := map[string]bool{}
	for _, f := range a {
		if names[f.Name] {
			t.Fatalf("duplicate fixture %s", f.Name)
		}
		names[f.Name] = true
		if err := f.Path.Validate(); err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if err := f.Params.Validate(); err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		for _, tr := range f.Trajectories {
			if err := tr.Validate(); err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			for _, s := range tr.Samples {
				if s.Stage != StageFinal {
					t.Fatalf("%s: fixture trajectories are final-stage", f.Name)
				}
			}
		}
		raw, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("%s: marshal: %v", f.Name, err)
		}
		var back Fixture
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("%s: unmarshal: %v", f.Name, err)
		}
		if !reflect.DeepEqual(back, f) {
			t.Fatalf("%s: JSON round trip changed the fixture", f.Name)
		}
	}
	if _, ok := a[0].Trajectory("no_such_track"); ok {
		t.Fatal("lookup of an unknown track succeeded")
	}
}
