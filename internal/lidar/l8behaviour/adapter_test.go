package l8behaviour

import (
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
		LastMeasurementUnixNanos: FixtureBaseUnixNanos,
	}
	track.ObservationCount = 4
	class := l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9}
	s, err := SampleFromTrack(track, class, l5tracks.EstimationGeometryConverging, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	// No accumulated extent yet: the class prior, marked as such and never
	// converged, so the projected endpoint is prior-dominated.
	if s.Length.Provenance != ProvenanceClassPrior || s.Length.Converged || s.Estimation != EstimationGeometryConverging {
		t.Fatalf("sample %+v", s)
	}
	body, reason, err := ProjectBody(fixturePathX(), track.TrackID, s)
	if err != nil || reason != ReasonUnspecified || body.Leading.Source != EndpointPriorDominated || body.Leading.ExtentConverged {
		t.Fatalf("projection %+v reason %s err %v", body.Leading, reason, err)
	}

	if _, err := SampleFromTrack(nil, class, l5tracks.EstimationInitialising, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Fatal("nil track accepted")
	}
	// An established claim the extents cannot support is reported.
	if _, err := SampleFromTrack(track, class, l5tracks.EstimationEstablished, FixtureBaseUnixNanos, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Fatal("established with class-prior extents accepted")
	}
}
