package l8behaviour

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Published boxes filed as an evidence database files them, built from the
// analytic scenarios: each observed sample's body becomes the box drawn at
// that frame, beside an online estimate with the sample's velocity and
// covariance, and every track is labelled a car.

func boxConfidence(c float32) *float32 { return &c }

// boxRow is one sample's body filed as a published box.
func boxRow(trackID, sensorID string, s TrajectorySample) PersistedBox {
	var cov [16]float32
	for i, v := range s.Covariance {
		cov[i] = float32(v)
	}
	return PersistedBox{
		TrackID: trackID, SensorID: sensorID, FrameUnixNanos: s.CaptureUnixNanos,
		EstimatorID: "cv_kf_v1", ObsModelID: "obb_centre_v1", ParamHash: "sha256:boxes", Stage: "online",
		Support: l5tracks.SupportObserved, MeasurementUnixNanos: s.CaptureUnixNanos - 3,
		VX: float32(s.VX), VY: float32(s.VY), Covariance: cov,
		HasBox: true, BoxX: float32(s.X), BoxY: float32(s.Y), BoxHeadingRad: float32(s.Heading.Rad),
		BoxLength: float32(s.Length.Metres), BoxWidth: float32(s.Width.Metres),
		ClassLabel: "car", ClassConfidence: boxConfidence(0.875),
	}
}

// scenarioBoxes files every observed sample of a scenario. A coasted frame
// files no row, as the tracker writes none.
func scenarioBoxes(sc EncounterScenario) []PersistedBox {
	var rows []PersistedBox
	for _, tr := range sc.Trajectories {
		for _, s := range tr.Samples {
			if s.Support == SupportObserved {
				rows = append(rows, boxRow(tr.Passage.TrackID, tr.Passage.SensorID, s))
			}
		}
	}
	return rows
}

func boxTrajectories(t *testing.T, rows []PersistedBox) []Trajectory {
	t.Helper()
	out, err := TrajectoriesFromBoxes(rows, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTrajectoriesFromBoxesStatesWhatABoxCarries: every sample is the box at
// the body centre, with no face, accumulated extents and heading, the
// estimate's velocity and support, online, and established once both extents
// have three boxes behind them.
func TestTrajectoriesFromBoxesStatesWhatABoxCarries(t *testing.T) {
	sc := ScenarioSteadyApproach()
	rows := scenarioBoxes(sc)
	// Out of order, as a query need not return them.
	rows[0], rows[len(rows)-1] = rows[len(rows)-1], rows[0]
	trajectories := boxTrajectories(t, rows)
	if len(trajectories) != 2 {
		t.Fatalf("%d trajectories, want 2", len(trajectories))
	}
	want := EstimateIdentity{EstimatorID: "cv_kf_v1", ObsModelID: "published_box/obb_centre_v1", ParamHash: "sha256:boxes"}
	bounds := l5tracks.DefaultConvergenceBounds()
	for _, tr := range trajectories {
		if tr.Estimate != want {
			t.Fatalf("estimate %+v, want %+v", tr.Estimate, want)
		}
		p := tr.Passage
		if p.MotionClass != MotionRigidVehicle || p.ClassLabel != "car" || p.ClassConfidence == nil || *p.ClassConfidence != 0.875 ||
			p.SensorID != fixtureSensorID {
			t.Fatalf("passage %+v", p)
		}
		source, _ := trajectoryOf(sc, p.TrackID)
		if len(tr.Samples) != len(source.Samples) {
			t.Fatalf("%s: %d samples from %d boxes", p.TrackID, len(tr.Samples), len(source.Samples))
		}
		for i, s := range tr.Samples {
			box := source.Samples[i]
			if s.Reference != ReferenceBodyCentre || s.X != box.X || s.Y != box.Y || s.VX != box.VX ||
				s.Covariance != box.Covariance || s.AnchorToCentre != (BodyOffset{}) {
				t.Fatalf("%s sample %d pose %+v from box %+v", p.TrackID, i, s, box)
			}
			if s.Faces != (FaceVisibility{}) {
				t.Fatalf("%s sample %d claims a face %+v", p.TrackID, i, s.Faces)
			}
			if !s.Heading.Resolved() || s.Heading.Rad != 0 || s.Heading.VarianceRad2 != 0 ||
				s.Heading.Provenance != ProvenanceAccumulated {
				t.Fatalf("%s sample %d heading %+v", p.TrackID, i, s.Heading)
			}
			converged := i+1 >= bounds.MinAdmissibleFrames
			for _, e := range []struct {
				got  ExtentBelief
				want float64
			}{{s.Length, box.Length.Metres}, {s.Width, box.Width.Metres}} {
				if e.got.Metres != e.want || e.got.SigmaMetres != 0 || e.got.AdmissibleFrames != i+1 ||
					e.got.Provenance != ProvenanceAccumulated || e.got.Converged != converged {
					t.Fatalf("%s sample %d extent %+v from %g m", p.TrackID, i, e.got, e.want)
				}
			}
			wantState := EstimationEstablished
			if !converged {
				wantState = EstimationGeometryConverging
			}
			if s.Estimation != wantState || s.Stage != StageOnline || s.Support != SupportObserved ||
				s.LastObservedUnixNanos != s.CaptureUnixNanos || s.AcquisitionUnixNanos != s.CaptureUnixNanos-3 {
				t.Fatalf("%s sample %d: %s %s %s, observed %d, acquired %d", p.TrackID, i, s.Estimation, s.Stage, s.Support,
					s.LastObservedUnixNanos, s.AcquisitionUnixNanos)
			}
		}
	}
	if out, err := TrajectoriesFromBoxes(nil, bounds); err != nil || out != nil {
		t.Errorf("no rows = %v, %v; want nothing and no error", out, err)
	}
}

// TestTrajectoriesFromBoxesOnTheAnalyticScenarios: read as boxes, every
// scenario fits the paths and finds the encounters its bodies do. What
// changes is only what a box cannot say: the first two boxes of each track
// have not converged, every endpoint is temporally inferred, and a box's
// endpoint sigma is the position's alone.
func TestTrajectoriesFromBoxesOnTheAnalyticScenarios(t *testing.T) {
	for _, sc := range EncounterScenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			a, err := AnalyseFollowing(boxTrajectories(t, scenarioBoxes(sc)), sc.Params)
			if err != nil {
				t.Fatal(err)
			}
			if len(a.Paths) != len(sc.Paths) {
				t.Fatalf("%d paths, want %d", len(a.Paths), len(sc.Paths))
			}
			for i, want := range sc.Paths {
				got := a.Paths[i]
				if got.FollowerTrackID != want.FollowerTrackID || !reflect.DeepEqual(got.MemberTrackIDs, want.MemberTrackIDs) ||
					!reflect.DeepEqual(got.Conditions, want.Conditions) || (got.Path == nil) != (len(want.Conditions) > 0) {
					t.Errorf("path %s members %v conditions %v, want %v %v", got.FollowerTrackID, got.MemberTrackIDs,
						got.Conditions, want.MemberTrackIDs, want.Conditions)
				}
			}
			if len(a.Encounters) != len(sc.Encounters) {
				t.Fatalf("%d encounters, want %d", len(a.Encounters), len(sc.Encounters))
			}
			for i, e := range a.Encounters {
				want := sc.Encounters[i]
				if e.LeaderTrackID != want.LeaderTrackID || e.FollowerTrackID != want.FollowerTrackID || e.Stage != StageOnline {
					t.Fatalf("encounter %s -> %s (%s), want %s -> %s", e.LeaderTrackID, e.FollowerTrackID, e.Stage,
						want.LeaderTrackID, want.FollowerTrackID)
				}
				for _, inst := range e.Instants {
					if inst.Point == nil || inst.Point.Gap == nil {
						continue
					}
					if g := inst.Point.Gap; g.LeaderSource != EndpointTemporallyInferred || g.FollowerSource != EndpointTemporallyInferred {
						t.Fatalf("gap at %d from %s and %s endpoints", inst.CaptureUnixNanos, g.LeaderSource, g.FollowerSource)
					}
				}
			}
		})
	}
}

// TestTrajectoriesFromBoxesSteadyApproach pins one scenario's figures. The
// bodies are the scenario's, so its gaps are; its first two instants wait for
// the extents to converge, and the gap sigma is the two positions' alone,
// sqrt(2/64) m, where the scenario's bodies add their length sigmas.
func TestTrajectoriesFromBoxesSteadyApproach(t *testing.T) {
	sc := ScenarioSteadyApproach()
	a, err := AnalyseFollowing(boxTrajectories(t, scenarioBoxes(sc)), sc.Params)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Encounters) != 1 {
		t.Fatalf("%d encounters", len(a.Encounters))
	}
	e := a.Encounters[0]
	acc := e.Accounting
	wantSuppressed := []ReasonTally{{Reason: ReasonExtentNotConverged, Instants: 2, Nanos: nanosOf(2)}}
	if len(e.Instants) != 50 || acc.ValidNanos != nanosOf(47) || !reflect.DeepEqual(acc.Suppressions, wantSuppressed) {
		t.Fatalf("%d instants, valid %d, suppressed %+v", len(e.Instants), acc.ValidNanos, acc.Suppressions)
	}
	if len(e.SpatialGapSeries) != 48 {
		t.Fatalf("%d gaps in the series, want 48", len(e.SpatialGapSeries))
	}
	sigma := math.Sqrt(2.0 / 64)
	for _, p := range e.SpatialGapSeries {
		k := (p.CaptureUnixNanos - FixtureBaseUnixNanos) / FixtureFramePeriodNanos
		if want := 20 - 0.25*float64(k); math.Abs(p.Value-want) > 1e-9 || math.Abs(p.Sigma-sigma) > 1e-9 {
			t.Fatalf("frame %d gap %g +/- %g, want %g +/- %g", k, p.Value, p.Sigma, want, sigma)
		}
	}
}

// TestBoxHeadingNamesTheFrontFromTheCourse: the box axis is the heading when
// it lies along the course, is turned by half a turn when it lies against
// it, and names no front across it; below the heading-observability speed
// the track's last named front is held the same way, and with none there is
// no front.
func TestBoxHeadingNamesTheFrontFromTheCourse(t *testing.T) {
	bounds := l5tracks.DefaultConvergenceBounds()
	deg := func(d float64) float32 { return float32(d * math.Pi / 180) }
	held := deg(10)
	for name, c := range map[string]struct {
		box, vx, vy float32
		front       *float32
		resolved    bool
		psi         float32
	}{
		"along the course":         {box: deg(44), vx: 10, resolved: true, psi: deg(44)},
		"against the course":       {box: deg(-170), vx: 10, resolved: true, psi: deg(10)},
		"across the course":        {box: deg(60), vx: 10, psi: deg(60)},
		"slow with no front":       {box: 0, vx: 0.25},
		"slow along the front":     {box: deg(-20), vx: 0.25, front: &held, resolved: true, psi: deg(-20)},
		"slow against the front":   {box: deg(175), vy: 0.25, front: &held, resolved: true, psi: deg(-5)},
		"slow across the front":    {box: deg(100), front: &held, psi: deg(100)},
		"the course over the held": {box: deg(90), vy: 10, front: &held, resolved: true, psi: deg(90)},
	} {
		o := boxHeading(PersistedBox{BoxHeadingRad: c.box, VX: c.vx, VY: c.vy}, c.front, bounds)
		if o.Provenance != l5tracks.ProvenanceAccumulated || o.IsResolved() != c.resolved ||
			math.Abs(float64(o.PsiRad-c.psi)) > 1e-6 || (!c.resolved && o.AmbiguousModeWeight != 0.5) {
			t.Errorf("%s: %+v, want resolved %v at %g", name, o, c.resolved, c.psi)
		}
	}
}

// TestTrajectoriesFromBoxesHoldsTheFrontAtRest: a car that stops keeps the
// front it was given while moving and stays established; one whose box turns
// across that front at rest is temporarily degraded rather than established
// without a front; one that never moved names no front at all.
func TestTrajectoriesFromBoxesHoldsTheFrontAtRest(t *testing.T) {
	var rows []PersistedBox
	for k := int64(0); k < 8; k++ {
		speed := 5.0
		if k >= 4 {
			speed = 0
		}
		r := boxRow("trk_stops", fixtureSensorID, fixtureCar(fixtureAt(k), 20+float64(k), 0, speed).sample())
		if k == 7 {
			r.BoxHeadingRad = math.Pi / 2
		}
		rows = append(rows, r, boxRow("trk_parked", fixtureSensorID, fixtureCar(fixtureAt(k), 0, 5, 0).sample()))
	}
	trajectories := boxTrajectories(t, rows)
	parked, stops := trajectories[0], trajectories[1]
	for i, s := range stops.Samples {
		want := EstimationEstablished
		switch {
		case i < 2:
			want = EstimationGeometryConverging
		case i == 7:
			want = EstimationTemporarilyDegraded
		}
		if s.Estimation != want || s.Heading.Resolved() != (i < 7) {
			t.Errorf("stopping car sample %d: %s, heading %+v; want %s", i, s.Estimation, s.Heading, want)
		}
	}
	for i, s := range parked.Samples {
		if s.Heading.Resolved() || s.Estimation != EstimationInitialising {
			t.Errorf("parked car sample %d: %s, heading %+v", i, s.Estimation, s.Heading)
		}
	}
}

// TestTrajectoriesFromBoxesClassAndGaps: the class is the track's label; a
// track without one is unknown with no confidence; a frame whose observation
// records no box is a gap, and a track with no box at all has no trajectory.
func TestTrajectoriesFromBoxesClassAndGaps(t *testing.T) {
	var rows []PersistedBox
	for k := int64(0); k < 6; k++ {
		s := fixtureCar(fixtureAt(k), 20+float64(k), 0, 10).sample()
		walker := boxRow("trk_walker", fixtureSensorID, s)
		walker.ClassLabel, walker.ClassConfidence = "pedestrian", boxConfidence(0.5)
		unlabelled := boxRow("trk_unlabelled", fixtureSensorID, s)
		unlabelled.ClassLabel, unlabelled.ClassConfidence = "", nil
		boxless := boxRow("trk_boxless", fixtureSensorID, s)
		boxless.HasBox = false
		if k == 2 {
			unlabelled.HasBox = false
		}
		rows = append(rows, walker, unlabelled, boxless)
	}
	trajectories := boxTrajectories(t, rows)
	if len(trajectories) != 2 {
		t.Fatalf("%d trajectories, want the walker and the unlabelled track", len(trajectories))
	}
	unlabelled, walker := trajectories[0], trajectories[1]
	if p := unlabelled.Passage; p.MotionClass != MotionUnknown || p.ClassLabel != "" || p.ClassConfidence != nil {
		t.Errorf("unlabelled passage %+v", p)
	}
	if p := walker.Passage; p.MotionClass != MotionPedestrian || p.ClassLabel != "pedestrian" || *p.ClassConfidence != 0.5 {
		t.Errorf("walker passage %+v", p)
	}
	if n := len(unlabelled.Samples); n != 5 || unlabelled.Samples[2].CaptureUnixNanos != fixtureAt(3) {
		t.Fatalf("%d samples; the boxless frame is not a gap", n)
	}
	// The boxes read, not the frames, are what converge an extent.
	if unlabelled.Samples[2].Length.AdmissibleFrames != 3 {
		t.Errorf("third box admits %d frames", unlabelled.Samples[2].Length.AdmissibleFrames)
	}
}

// TestTrajectoriesFromBoxesRefusesWhatARowCannotSay: a mixed version, a
// refined stage, an unobserved row, two sensors, two labels, the fixture's
// identity, an unknown filter, a degenerate box and an asymmetric covariance
// are errors.
func TestTrajectoriesFromBoxesRefusesWhatARowCannotSay(t *testing.T) {
	for name, c := range map[string]struct {
		mutate func([]PersistedBox)
		says   string
	}{
		"mixed parameters": {func(r []PersistedBox) { r[1].ParamHash = "sha256:other" }, "mix versions"},
		"final stage":      {func(r []PersistedBox) { setAll(r, func(b *PersistedBox) { b.Stage = "final" }) }, "online"},
		"unobserved":       {func(r []PersistedBox) { r[3].Support = l5tracks.SupportMissedUnknown }, "observed"},
		"two sensors":      {func(r []PersistedBox) { r[len(r)-1].SensorID = "sensor_b" }, "sensors"},
		"two labels":       {func(r []PersistedBox) { r[0].ClassLabel = "truck" }, "two labels"},
		"no param hash":    {func(r []PersistedBox) { r[0].ParamHash = "" }, "identity"},
		"unknown filter":   {func(r []PersistedBox) { setAll(r, func(b *PersistedBox) { b.EstimatorID = "ukf_v9" }) }, "layout"},
		"fixture identity": {func(r []PersistedBox) {
			f := FixtureEstimate()
			setAll(r, func(b *PersistedBox) {
				b.EstimatorID, b.ObsModelID, b.ParamHash = f.EstimatorID, f.ObsModelID, f.ParamHash
			})
		}, "fixture"},
		"flat box":   {func(r []PersistedBox) { r[1].BoxWidth = 0 }, "not a body"},
		"asymmetric": {func(r []PersistedBox) { r[1].Covariance[1] = 0.5 }, "round-off"},
	} {
		t.Run(name, func(t *testing.T) {
			rows := scenarioBoxes(ScenarioSteadyApproach())
			c.mutate(rows)
			_, err := TrajectoriesFromBoxes(rows, l5tracks.DefaultConvergenceBounds())
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("error %v, want one saying %q", err, c.says)
			}
		})
	}
}

func setAll(rows []PersistedBox, f func(*PersistedBox)) {
	for i := range rows {
		f(&rows[i])
	}
}
