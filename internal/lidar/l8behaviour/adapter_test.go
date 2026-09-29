package l8behaviour

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestL5VocabularyMappings(t *testing.T) {
	for in, want := range map[l5tracks.MotionClass]MotionClass{
		l5tracks.MotionUnknown:      MotionUnknown,
		l5tracks.MotionRigidVehicle: MotionRigidVehicle,
		l5tracks.MotionTwoWheeler:   MotionTwoWheeler,
		l5tracks.MotionPedestrian:   MotionPedestrian,
		l5tracks.MotionClass(77):    MotionUnknown,
	} {
		got := MotionClassFromL5(in)
		if got != want {
			t.Errorf("motion class %d -> %s, want %s", in, got, want)
		}
		// The tokens are l5tracks' own for every real class.
		if in <= l5tracks.MotionPedestrian && got.String() != in.String() {
			t.Errorf("token %s differs from l5tracks %s", got, in)
		}
	}

	for in, want := range map[l5tracks.EstimationState]EstimationState{
		l5tracks.EstimationInitialising:        EstimationInitialising,
		l5tracks.EstimationGeometryConverging:  EstimationGeometryConverging,
		l5tracks.EstimationEstablished:         EstimationEstablished,
		l5tracks.EstimationTemporarilyDegraded: EstimationTemporarilyDegraded,
		l5tracks.EstimationModelInvalid:        EstimationModelInvalid,
	} {
		got, err := EstimationStateFromL5(in)
		if err != nil || got != want || got.String() != in.String() {
			t.Errorf("estimation %s -> %s, %v", in, got, err)
		}
	}
	if _, err := EstimationStateFromL5(l5tracks.EstimationState(42)); err == nil {
		t.Error("unknown estimation state mapped")
	}

	// Final is never inferred from an in-memory estimate.
	if EstimateStageFromL5(l5tracks.StageLive) != StageOnline || EstimateStageFromL5(l5tracks.StageSmoothed) != StageFixedLag {
		t.Error("stage mapping")
	}

	for in, want := range map[l5tracks.Provenance]BeliefProvenance{
		l5tracks.ProvenanceNone:        ProvenanceUnspecified,
		l5tracks.ProvenanceClassPrior:  ProvenanceClassPrior,
		l5tracks.ProvenanceAccumulated: ProvenanceAccumulated,
		l5tracks.ProvenanceObserved:    ProvenanceObserved,
	} {
		if got := BeliefProvenanceFromL5(in); got != want {
			t.Errorf("provenance %s -> %s, want %s", in, got, want)
		}
	}

	for _, c := range []struct {
		in   l5tracks.SupportState
		want SupportState
	}{
		{l5tracks.SupportState{PointCount: 80}, SupportObserved},
		{l5tracks.SupportState{PointCount: 80, Truncated: true}, SupportObserved},
		{l5tracks.SupportState{PointCount: 12, Fragmented: true}, SupportClusterSplit},
		{l5tracks.SupportState{CoastedFrames: 2}, SupportCoasted},
		{l5tracks.SupportState{CoastedFrames: 2, Fragmented: true}, SupportCoasted},
		// An absence keeps the tracker's explanation of it.
		{l5tracks.SupportState{CoastedFrames: 2, Instant: l5tracks.SupportOccludedInferred}, SupportOccludedInferred},
		{l5tracks.SupportState{CoastedFrames: 2, Instant: l5tracks.SupportMissedUnknown}, SupportMissedUnknown},
		{l5tracks.SupportState{CoastedFrames: 1, Instant: l5tracks.SupportOutOfFOV}, SupportOutOfFOV},
		{l5tracks.SupportState{CoastedFrames: 2, Instant: l5tracks.SupportCoasted}, SupportCoasted},
		// The coasted count decides observation; a token cannot overrule it
		// either way, and a token the tracker never claims is not adopted.
		{l5tracks.SupportState{CoastedFrames: 2, Instant: l5tracks.SupportObserved}, SupportCoasted},
		{l5tracks.SupportState{CoastedFrames: 2, Instant: l5tracks.SupportClusterMerged}, SupportCoasted},
		{l5tracks.SupportState{PointCount: 80, Instant: l5tracks.SupportOccludedInferred}, SupportObserved},
		{l5tracks.SupportState{PointCount: 12, Instant: l5tracks.SupportObserved, Fragmented: true}, SupportClusterSplit},
	} {
		if got := SupportStateFromL5(c.in); got != c.want {
			t.Errorf("support %+v -> %s, want %s", c.in, got, c.want)
		}
	}
}

func TestPassageFromL5TakesTheEffectiveClass(t *testing.T) {
	split := l5tracks.MotionClassBelief{
		Class: l5tracks.MotionRigidVehicle, Posterior: 0.5,
		Runner: l5tracks.MotionTwoWheeler, RunnerPosterior: 0.45,
	}
	p := PassageFromL5("trk_a", "site", "hesai-01", split, "car")
	// A split posterior runs with the weaker class, which following does
	// not support: a doubtful car is not quietly treated as one.
	if p.MotionClass != MotionTwoWheeler || p.ClassConfidence == nil || *p.ClassConfidence != 0.5 {
		t.Fatalf("passage %+v", p)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if r, _ := ClassApplicability(MetricFollowingSpatialGap, p.MotionClass, MotionRigidVehicle); r != ReasonClassNotSupported {
		t.Fatalf("split class applicability %s", r)
	}
	bad := PassageFromL5("trk_b", "", "hesai-01", l5tracks.MotionClassBelief{Class: l5tracks.MotionPedestrian, Posterior: 2}, "")
	if bad.ClassConfidence != nil || bad.MotionClass != MotionPedestrian {
		t.Fatalf("out-of-range posterior kept as confidence: %+v", bad)
	}
}

func establishedBody(x float32) (l5tracks.SolidBodyEstimate, DynamicState) {
	cov := [16]float32{
		0.015625, 0, 0, 0,
		0, 0.015625, 0, 0,
		0, 0, 0.0625, 0,
		0, 0, 0, 0.0625,
	}
	body := l5tracks.SolidBodyEstimate{
		StateModel:         l5tracks.StateModelCVCartesianV1,
		Reference:          l5tracks.ReferenceBodyCentre,
		X:                  x,
		PositionCovariance: [4]float32{0.015625, 0, 0, 0.015625},
		Orientation:        l5tracks.OrientationBelief{PsiRad: 0, VarianceRad2: 0.0625, Provenance: l5tracks.ProvenanceObserved},
		Length:             l5tracks.DimensionBelief{Metres: 4.5, SigmaMetres: 0.25, AdmissibleFrames: 6, Provenance: l5tracks.ProvenanceAccumulated},
		Width:              l5tracks.DimensionBelief{Metres: 2, SigmaMetres: 0.125, AdmissibleFrames: 6, Provenance: l5tracks.ProvenanceAccumulated},
		Height:             l5tracks.DimensionBelief{Metres: 1.5, SigmaMetres: 1.5, Provenance: l5tracks.ProvenanceClassPrior},
		Motion:             l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9},
		Estimation:         l5tracks.EstimationEstablished,
		Stage:              l5tracks.StageLive,
		// The measurement's own acquisition time, a little before capture.
		LastObservedUnixNanos: FixtureBaseUnixNanos - 1000,
	}
	return body, DynamicState{VX: 10, Covariance: cov}
}

func TestSampleFromSolidBody(t *testing.T) {
	body, dyn := establishedBody(30)
	s, err := SampleFromSolidBody(body, dyn, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	if s.Reference != ReferenceBodyCentre || s.X != 30 || s.VX != 10 || s.Covariance[10] != 0.0625 ||
		s.Support != SupportObserved || s.Stage != StageOnline || s.Estimation != EstimationEstablished ||
		!s.Length.Converged || !s.Width.Converged || s.Length.Provenance != ProvenanceAccumulated ||
		!s.Heading.Resolved() || s.Faces != (FaceVisibility{}) || s.LastObservedUnixNanos != FixtureBaseUnixNanos {
		t.Fatalf("sample %+v", s)
	}
	// An online tracker estimate can never pass the production guard.
	if s.ProductionReason() != ReasonEstimateNotFinal {
		t.Fatalf("production reason %s", s.ProductionReason())
	}

	// Two adapted tracker estimates plug straight into the evaluator, and
	// the only thing between them and publication is the stage.
	fBody, fDyn := establishedBody(20)
	fBody.Length.Metres = 4
	f, err := SampleFromSolidBody(fBody, fDyn, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	passage := func(id string) Passage {
		return PassageFromL5(id, "site", "hesai-01", body.Motion, "car")
	}
	est := EstimateIdentity{EstimatorID: "cv_kf_v1", ObsModelID: "obb_centre_v1", ParamHash: "params/test"}
	pt := mustEvaluate(t, fixturePathX(),
		Party{Passage: passage("trk_l"), Estimate: est, Sample: s},
		Party{Passage: passage("trk_f"), Estimate: est, Sample: f})
	if !reflect.DeepEqual(pt.Reasons, []SuppressionReason{ReasonEstimateNotFinal}) ||
		pt.SpatialGap.Reason != ReasonEstimateNotFinal || pt.Gap.ValueM != 5.75 {
		t.Fatalf("adapted pair: reasons %v gap %+v", pt.Reasons, pt.Gap)
	}
	// No face is claimed, so the best the adapter yields is inference.
	if pt.Gap.Source() != EndpointTemporallyInferred {
		t.Fatalf("adapted gap source %s", pt.Gap.Source())
	}

	// A freshly seeded track: a medoid reference, no extent belief at all, no
	// orientation. It maps faithfully and is refused by the guard.
	seed := body
	seed.Reference, seed.Estimation = l5tracks.ReferenceClusterMedoid, l5tracks.EstimationInitialising
	seed.Length, seed.Width, seed.Orientation = l5tracks.DimensionBelief{}, l5tracks.DimensionBelief{}, l5tracks.OrientationBelief{}
	m, err := SampleFromSolidBody(seed, dyn, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
	if err != nil || m.Reference != ReferenceClusterMedoid || m.Length.Present() || m.Heading.Provenance.Valid() {
		t.Fatalf("seed sample %+v err %v", m, err)
	}
	if m.ProductionReason() != ReasonInsufficientObservation {
		t.Fatalf("seed production reason %s", m.ProductionReason())
	}

	// Coasting keeps the tracker's last-observed time.
	coasted := body
	coasted.Support = l5tracks.SupportState{CoastedFrames: 3}
	coasted.Estimation = l5tracks.EstimationTemporarilyDegraded
	c, err := SampleFromSolidBody(coasted, dyn, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
	if err != nil || c.Support != SupportCoasted || c.LastObservedUnixNanos != body.LastObservedUnixNanos {
		t.Fatalf("coasted sample %+v err %v", c, err)
	}
}

func TestSampleFromSolidBodyRefusesWhatItCannotRepresent(t *testing.T) {
	for _, c := range []struct {
		name string
		fn   func(*l5tracks.SolidBodyEstimate, *DynamicState)
		want string
	}{
		{"state model", func(b *l5tracks.SolidBodyEstimate, d *DynamicState) { b.StateModel = "ctrv_v1" }, "state model"},
		{"covariance from another frame", func(b *l5tracks.SolidBodyEstimate, d *DynamicState) { d.Covariance[0] = 1 }, "disagrees"},
		{"near-face reference", func(b *l5tracks.SolidBodyEstimate, d *DynamicState) {
			b.Reference = l5tracks.ReferenceNearFaceCentre
		}, "declared offset"},
		{"unknown reference", func(b *l5tracks.SolidBodyEstimate, d *DynamicState) {
			b.Reference = l5tracks.ReferenceUnknown
		}, "declared offset"},
		{"unknown estimation state", func(b *l5tracks.SolidBodyEstimate, d *DynamicState) {
			b.Estimation = l5tracks.EstimationState(9)
		}, "estimation state"},
		{"established under other bounds", func(b *l5tracks.SolidBodyEstimate, d *DynamicState) {
			b.Length.AdmissibleFrames = 1
		}, "established"},
	} {
		body, dyn := establishedBody(30)
		c.fn(&body, &dyn)
		_, err := SampleFromSolidBody(body, dyn, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

func TestSampleFromTrack(t *testing.T) {
	track := &l5tracks.TrackedObject{
		TrackID: "trk_live", X: 12, Y: 1, VX: 6,
		P: [16]float32{
			0.25, 0, 0, 0,
			0, 0.25, 0, 0,
			0, 0, 0.5, 0,
			0, 0, 0, 0.5,
		},
		OBBHeadingRad:            0,
		HeadingSource:            l5tracks.HeadingSourceVelocity,
		LastMeasurementSource:    l5tracks.MeasurementOBBCentreV1,
		LastMeasurementUnixNanos: FixtureBaseUnixNanos - 3_000_000,
	}
	track.ObservationCount = 4
	class := l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9}
	s, err := SampleFromTrack(track, class, l5tracks.EstimationGeometryConverging, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	// No accumulated extent yet: the class prior, marked as such and never
	// converged.
	if s.Length.Provenance != ProvenanceClassPrior || s.Length.Converged || s.Estimation != EstimationGeometryConverging {
		t.Fatalf("sample %+v", s)
	}
	// The measurement's acquisition time survives beside the capture time
	// the observed instant is stamped with.
	if s.LastObservedUnixNanos != FixtureBaseUnixNanos || s.AcquisitionUnixNanos != track.LastMeasurementUnixNanos {
		t.Fatalf("last observed %d, acquired %d", s.LastObservedUnixNanos, s.AcquisitionUnixNanos)
	}
	// R1: the tracked position is the centre of the box the frame saw, not
	// the body centre, so no endpoint is projected from it.
	if s.Reference != ReferenceVisibleOBBCentre {
		t.Fatalf("an OBB-centre track refers to %s, want %s", s.Reference, ReferenceVisibleOBBCentre)
	}
	if _, reason, err := ProjectBody(fixturePathX(), track.TrackID, s); err != nil || reason != ReasonInsufficientObservation {
		t.Fatalf("projection from the visible box centre: reason %s err %v", reason, err)
	}
	medoid := *track
	medoid.LastMeasurementSource = l5tracks.MeasurementMedoidV0
	if m, err := SampleFromTrack(&medoid, class, l5tracks.EstimationGeometryConverging, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds()); err != nil || m.Reference != ReferenceClusterMedoid {
		t.Fatalf("a medoid track refers to %s (err %v), want %s", m.Reference, err, ReferenceClusterMedoid)
	}
	// A track whose measurement source was not recorded names no point.
	unrecorded := *track
	unrecorded.LastMeasurementSource = ""
	if _, err := SampleFromTrack(&unrecorded, class, l5tracks.EstimationGeometryConverging, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Fatal("a track with no recorded measurement source was given a reference")
	}

	if _, err := SampleFromTrack(nil, class, l5tracks.EstimationInitialising, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Fatal("nil track accepted")
	}
	// An established claim the extents cannot support is reported.
	if _, err := SampleFromTrack(track, class, l5tracks.EstimationEstablished, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Fatal("established with class-prior extents accepted")
	}
}

// persistedRow is a lidar_track_estimates row of the replay's online
// estimator under the OBB-centre model, moving along +x at vx: observed, and
// stating the centre of the visible box as the OBB-centre filter does.
func persistedRow(track string, frame int, x, vx float32) PersistedEstimate {
	return PersistedEstimate{
		TrackID: track, SensorID: "sensor_a",
		FrameUnixNanos: FixtureBaseUnixNanos + int64(frame)*FixtureFramePeriodNanos,
		EstimatorID:    "cv_kf_v1", ObsModelID: "obb_centre_v1", ParamHash: "sha256:online", Stage: "online",
		Reference: l5tracks.ReferenceVisibleOBBCentre, Support: l5tracks.SupportObserved, X: x, VX: vx,
		Covariance: [16]float32{0.04, 0.01, 0, 0, 0.01, 0.04, 0, 0, 0, 0, 0.25, 0, 0, 0, 0, 0.25},
	}
}

// TestTrajectoriesFromEstimates: a row becomes an observed sample at its own
// frame, with its own stage, the reference the row states, a lifecycle from
// its speed, and no heading, extent or class, because none is persisted.
func TestTrajectoriesFromEstimates(t *testing.T) {
	rows := []PersistedEstimate{
		persistedRow("trk_b", 1, 11, 10), persistedRow("trk_b", 0, 10, 10),
		persistedRow("trk_a", 0, 0, 0.25), persistedRow("trk_a", 1, 0.025, 0.25),
	}
	rows[3].Reference = l5tracks.ReferenceClusterMedoid
	rows[0].MeasurementUnixNanos = rows[0].FrameUnixNanos + 4_000_000
	trajectories, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	if len(trajectories) != 2 || trajectories[0].Passage.TrackID != "trk_a" || trajectories[1].Passage.TrackID != "trk_b" {
		t.Fatalf("trajectories are not one per track in id order: %+v", trajectories)
	}
	want := EstimateIdentity{EstimatorID: "cv_kf_v1", ObsModelID: "obb_centre_v1", ParamHash: "sha256:online"}
	for _, tr := range trajectories {
		if tr.Estimate != want || tr.Passage.SensorID != "sensor_a" || tr.Passage.MotionClass != MotionUnknown ||
			tr.Passage.ClassLabel != "" || tr.Passage.ClassConfidence != nil || tr.Passage.SiteID != "" {
			t.Errorf("%s identity = %+v, %+v", tr.Passage.TrackID, tr.Passage, tr.Estimate)
		}
		for i, s := range tr.Samples {
			if s.CaptureUnixNanos != FixtureBaseUnixNanos+int64(i)*FixtureFramePeriodNanos ||
				s.LastObservedUnixNanos != s.CaptureUnixNanos || s.Support != SupportObserved || s.Stage != StageOnline {
				t.Errorf("%s sample %d = %+v", tr.Passage.TrackID, i, s)
			}
			if s.Heading.Provenance.Valid() || s.Length.Present() || s.Width.Present() || s.Faces != (FaceVisibility{}) {
				t.Errorf("%s sample %d claims geometry nothing persisted: %+v", tr.Passage.TrackID, i, s)
			}
		}
	}
	b := trajectories[1].Samples
	// The acquisition time is the row's, when it recorded one; the frame
	// time stays the capture and last-observed time.
	if b[1].AcquisitionUnixNanos != rows[0].MeasurementUnixNanos || b[0].AcquisitionUnixNanos != 0 ||
		b[1].LastObservedUnixNanos != b[1].CaptureUnixNanos {
		t.Errorf("acquisition times %d and %d", b[0].AcquisitionUnixNanos, b[1].AcquisitionUnixNanos)
	}
	if b[0].X != 10 || b[1].X != 11 || b[0].Reference != ReferenceVisibleOBBCentre || b[0].Estimation != EstimationGeometryConverging {
		t.Errorf("moving OBB-centre samples = %+v", b)
	}
	a := trajectories[0].Samples
	if a[0].Estimation != EstimationInitialising {
		t.Errorf("a row below the heading-observability speed is %s, want initialising", a[0].Estimation)
	}
	if a[1].Reference != ReferenceClusterMedoid {
		t.Errorf("a row stating the cluster medoid refers to %s", a[1].Reference)
	}
	if b[0].Covariance[1] != float64(float32(0.01)) || b[0].Covariance[10] != 0.25 {
		t.Errorf("covariance not carried: %v", b[0].Covariance)
	}
}

// TestTrajectoriesFromEstimatesReadsTheStatedReference: the sample refers to
// whatever the row states, a place on the body included, which is how a
// point estimate under near_edge_track will reach the field run. Nothing the
// row does not carry is supplied with it: there is still no heading, so no
// endpoint is projected.
func TestTrajectoriesFromEstimatesReadsTheStatedReference(t *testing.T) {
	rows := []PersistedEstimate{persistedRow("trk_a", 0, 0, 10), persistedRow("trk_a", 1, 1, 10)}
	rows[1].Reference = l5tracks.ReferenceBodyCentre
	trajectories, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	s := trajectories[0].Samples
	if s[0].Reference != ReferenceVisibleOBBCentre || s[1].Reference != ReferenceBodyCentre || !s[1].Reference.IsPhysical() {
		t.Fatalf("references %s and %s, want the rows' own", s[0].Reference, s[1].Reference)
	}
	if s[1].Support != SupportObserved || s[1].LastObservedUnixNanos != s[1].CaptureUnixNanos {
		t.Fatalf("a body-centre row is not an observed sample: %+v", s[1])
	}
	if _, reason, err := ProjectBody(fixturePathX(), "trk_a", s[1]); err != nil || reason != ReasonOrientationUnresolved {
		t.Fatalf("a body-centre row without a heading: reason %s err %v, want %s", reason, err, ReasonOrientationUnresolved)
	}
}

// TestTrajectoriesFromEstimatesAveragesRoundOff: a float32 filter's
// covariance pairs that differ in their last places, as kirk0's online rows
// do, are averaged into one symmetric matrix rather than refused.
func TestTrajectoriesFromEstimatesAveragesRoundOff(t *testing.T) {
	rows := []PersistedEstimate{persistedRow("trk_a", 0, 0, 10), persistedRow("trk_a", 1, 1, 10)}
	// 0.01 and the float32 two units above it: a 2e-9 disagreement.
	rows[1].Covariance[4] = math.Nextafter32(math.Nextafter32(0.01, 1), 1)
	trajectories, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatalf("round-off refused: %v", err)
	}
	c := trajectories[0].Samples[1].Covariance
	if c[1] != c[4] || c[1] != float64(math.Nextafter32(0.01, 1)) {
		t.Errorf("pair = %v and %v, want both the float32 between them", c[1], c[4])
	}
}

// TestTrajectoriesFromEstimatesReadsRefinedStages: fixed_lag and final rows,
// written by a smoother over the online filter, are read at their own stage.
// This is the only path by which a sample is final.
func TestTrajectoriesFromEstimatesReadsRefinedStages(t *testing.T) {
	for _, stage := range []EstimateStage{StageFixedLag, StageFinal} {
		rows := []PersistedEstimate{persistedRow("trk_a", 0, 0, 10), persistedRow("trk_a", 1, 1, 10)}
		for i := range rows {
			rows[i].EstimatorID = "cv_kf_v1+" + l5tracks.SmootherID
			rows[i].Stage = stage.String()
		}
		trajectories, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds())
		if err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		for _, s := range trajectories[0].Samples {
			if s.Stage != stage || s.StateModel != StateModelCVCartesianV1 {
				t.Errorf("%s row read as stage %s, model %s", stage, s.Stage, s.StateModel)
			}
		}
		// R1: a refined stage revises the state it was given, and no stage
		// turns the visible box centre or the medoid into a place on the
		// body, so no endpoint is projected from a final row either.
		for _, reference := range []l5tracks.ReferencePoint{l5tracks.ReferenceVisibleOBBCentre, l5tracks.ReferenceClusterMedoid} {
			for i := range rows {
				rows[i].Reference = reference
			}
			trajectories, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds())
			if err != nil {
				t.Fatalf("%s %s: %v", stage, reference, err)
			}
			s := trajectories[0].Samples[1]
			if s.Reference.IsPhysical() || s.ProductionReason() != ReasonInsufficientObservation {
				t.Errorf("a %s %s row refers to %s, production reason %s", stage, reference, s.Reference, s.ProductionReason())
			}
			if _, reason, err := ProjectBody(fixturePathX(), "trk_a", s); err != nil || reason != ReasonInsufficientObservation {
				t.Errorf("a %s %s row projected a body: reason %s err %v", stage, reference, reason, err)
			}
		}
	}
	if out, err := TrajectoriesFromEstimates(nil, l5tracks.DefaultConvergenceBounds()); err != nil || out != nil {
		t.Errorf("no rows = %v, %v; want nothing and no error", out, err)
	}
}

// TestTrajectoriesFromEstimatesRefusesWhatARowCannotSay: an unknown layout,
// stage or geometry, a mixed version, a missing sensor, a repeated frame and
// the fixture's identity are errors, never guesses.
func TestTrajectoriesFromEstimatesRefusesWhatARowCannotSay(t *testing.T) {
	for name, mutate := range map[string]func([]PersistedEstimate){
		"unknown filter":        func(r []PersistedEstimate) { r[0].EstimatorID, r[1].EstimatorID = "imm_cv_ca_v2", "imm_cv_ca_v2" },
		"unknown stage":         func(r []PersistedEstimate) { r[0].Stage, r[1].Stage = "smoothed", "smoothed" },
		"unknown reference":     func(r []PersistedEstimate) { r[1].Reference = l5tracks.ReferenceUnknown },
		"near-face reference":   func(r []PersistedEstimate) { r[1].Reference = l5tracks.ReferenceNearFaceCentre },
		"unrecorded support":    func(r []PersistedEstimate) { r[1].Support = l5tracks.SupportUnrecorded },
		"coasted row":           func(r []PersistedEstimate) { r[1].Support = l5tracks.SupportCoasted },
		"explained absence":     func(r []PersistedEstimate) { r[1].Support = l5tracks.SupportOccludedInferred },
		"mixed stages":          func(r []PersistedEstimate) { r[1].Stage = "final" },
		"mixed parameters":      func(r []PersistedEstimate) { r[1].ParamHash = "sha256:other" },
		"no sensor":             func(r []PersistedEstimate) { r[0].SensorID, r[1].SensorID = "", "" },
		"two sensors":           func(r []PersistedEstimate) { r[1].SensorID = "sensor_b" },
		"repeated frame":        func(r []PersistedEstimate) { r[1].FrameUnixNanos = r[0].FrameUnixNanos },
		"no parameter hash":     func(r []PersistedEstimate) { r[0].ParamHash, r[1].ParamHash = "", "" },
		"asymmetric covariance": func(r []PersistedEstimate) { r[1].Covariance[4] = 0.02 },
		"fixture identity": func(r []PersistedEstimate) {
			for i := range r {
				f := FixtureEstimate()
				r[i].EstimatorID, r[i].ObsModelID, r[i].ParamHash = f.EstimatorID, f.ObsModelID, f.ParamHash
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			rows := []PersistedEstimate{persistedRow("trk_a", 0, 0, 10), persistedRow("trk_a", 1, 1, 10)}
			mutate(rows)
			if _, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds()); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	if _, err := PersistedStateModel("cv_kf_v1+" + l5tracks.SmootherID); err != nil {
		t.Errorf("a smoother over cv_kf_v1 keeps its layout: %v", err)
	}
}

// A row near_edge_track wrote on a body-centre frame states the body centre,
// whatever geometry entered the filter, and is read as stated.
func TestTrajectoriesFromEstimatesReadNearEdgeRowsAsTheBodyCentre(t *testing.T) {
	rows := []PersistedEstimate{persistedRow("trk", 0, 10, 10), persistedRow("trk", 1, 11, 10)}
	for i := range rows {
		rows[i].ObsModelID, rows[i].Reference = "near_edge_candidate_v1", l5tracks.ReferenceBodyCentre
	}
	trajectories, err := TrajectoriesFromEstimates(rows, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range trajectories[0].Samples {
		if s.Reference != ReferenceBodyCentre {
			t.Errorf("sample %d reference %s, want the body centre", i, s.Reference)
		}
	}
}
