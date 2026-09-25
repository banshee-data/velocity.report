package l5tracks

import (
	"math"
	"testing"
)

// Tests for the solid-body estimate contract, Section 5.4 of
// docs/plans/lidar-state-estimation-plan.md.
//
// The contract's value is entirely in what it refuses to claim, so most of
// these assert a refusal: an unbounded projection is suppressed rather than
// approximated, a class prior never passes as a measurement, and a seed's
// covariance states the bias it has rather than the precision it lacks.

// --- Initialisation --------------------------------------------------------

func TestSeedPositionCovarianceStatesTheMedoidBiasNotTheMeasurementPrecision(t *testing.T) {
	// The most important line in Section 5.6: a biased seed with an honest
	// covariance is recoverable, a biased seed with a confident covariance is
	// not. The medoid sits on the sensor-facing surface, so it is offset from
	// the body centre by up to half the body's width — far more than the
	// measurement noise would suggest.
	class := MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9}
	seed := SeedSolidBody(class, 10, 20, 1_000)

	prior := dimensionPriorFor(MotionRigidVehicle)
	wantSigma := prior.widthMetres / 2
	gotSigma := float32(math.Sqrt(float64(seed.PositionCovariance[0])))
	if math.Abs(float64(gotSigma-wantSigma)) > 1e-6 {
		t.Errorf("seeded position sigma = %v, want half a body width (%v)", gotSigma, wantSigma)
	}

	// And concretely: it must be well above the sigma today's measurement
	// noise implies, or the seed would claim precision it does not have.
	measurementSigma := float32(math.Sqrt(float64(DefaultTrackerConfig().MeasurementNoise)))
	if gotSigma <= 2*measurementSigma {
		t.Errorf("seeded sigma %v is not meaningfully wider than the measurement sigma %v: a biased seed with a confident covariance is unrecoverable",
			gotSigma, measurementSigma)
	}

	// Isotropic at birth: with no orientation there is no axis along which to
	// prefer one direction's uncertainty.
	if seed.PositionCovariance[1] != 0 || seed.PositionCovariance[2] != 0 {
		t.Errorf("seed covariance has cross terms %v, %v but no orientation to justify them",
			seed.PositionCovariance[1], seed.PositionCovariance[2])
	}
	if seed.PositionCovariance[0] != seed.PositionCovariance[3] {
		t.Errorf("seed covariance is not isotropic: %v against %v",
			seed.PositionCovariance[0], seed.PositionCovariance[3])
	}
}

func TestSeedClaimsNoOrientationAndNoMeasuredDimensions(t *testing.T) {
	seed := SeedSolidBody(MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9}, 0, 0, 1)

	// Orientation at birth is unobservable. A bimodal belief over a value
	// nobody measured would be two wrong answers rather than one.
	if seed.Orientation.Provenance != ProvenanceNone {
		t.Errorf("seed claims an orientation with provenance %v", seed.Orientation.Provenance)
	}
	if len(seed.Orientation.Modes()) != 0 {
		t.Errorf("seed offers %d orientation modes, want none", len(seed.Orientation.Modes()))
	}

	for _, d := range []struct {
		name string
		got  DimensionBelief
	}{
		{"length", seed.Length}, {"width", seed.Width}, {"height", seed.Height},
	} {
		if d.got.Provenance != ProvenanceClassPrior {
			t.Errorf("seed %s provenance = %v, want class_prior", d.name, d.got.Provenance)
		}
		if d.got.AdmissibleFrames != 0 {
			t.Errorf("seed %s claims %d admissible frames", d.name, d.got.AdmissibleFrames)
		}
		if d.got.IsConverged(DefaultConvergenceBounds().MaxDimensionSigmaMetres, 0) {
			t.Errorf("seed %s counts as converged on a class prior alone", d.name)
		}
	}

	if seed.Estimation != EstimationInitialising {
		t.Errorf("seed estimation state = %v, want initialising", seed.Estimation)
	}
	if seed.Reference != ReferenceClusterMedoid {
		t.Errorf("seed reference = %v, want cluster_medoid", seed.Reference)
	}
	if seed.Reference.IsPhysical() {
		t.Error("the medoid seed is reported as a physical point on the body")
	}
	if seed.StateModel != StateModelCVCartesianV1 {
		t.Errorf("seed state model = %q, want %q", seed.StateModel, StateModelCVCartesianV1)
	}
}

func TestSeedWidthVariesWithClassSoTheBiasDoesToo(t *testing.T) {
	// A pedestrian's medoid is a few centimetres from their centre; a bus's is
	// nearly a metre. Seeding both at the same sigma would be wrong in both
	// directions.
	wide := SeedSolidBody(MotionClassBelief{Class: MotionRigidVehicle, Posterior: 1}, 0, 0, 1)
	narrow := SeedSolidBody(MotionClassBelief{Class: MotionPedestrian, Posterior: 1}, 0, 0, 1)

	if wide.PositionCovariance[0] <= narrow.PositionCovariance[0] {
		t.Errorf("a vehicle seed (%v) is not less certain than a pedestrian seed (%v)",
			wide.PositionCovariance[0], narrow.PositionCovariance[0])
	}
}

// --- Provenance ------------------------------------------------------------

func TestClassPriorIsNeverEvidence(t *testing.T) {
	if ProvenanceClassPrior.IsEvidence() {
		t.Error("a class prior counts as evidence about this object")
	}
	if ProvenanceNone.IsEvidence() {
		t.Error("an absent value counts as evidence")
	}
	if !ProvenanceAccumulated.IsEvidence() || !ProvenanceObserved.IsEvidence() {
		t.Error("accumulated or observed values do not count as evidence")
	}

	// A tight sigma on a prior must not buy convergence: the bound is about
	// evidence concerning this object, and a prior is evidence about its class.
	prior := DimensionBelief{Metres: 4.5, SigmaMetres: 0.001, AdmissibleFrames: 99, Provenance: ProvenanceClassPrior}
	if prior.IsConverged(0.5, 3) {
		t.Error("a class prior with a tight sigma passed as converged")
	}
}

// --- Motion class ----------------------------------------------------------

func TestMotionClassForLabelMapsCoarselyAndFailsSafe(t *testing.T) {
	for label, want := range map[string]MotionClass{
		"car":          MotionRigidVehicle,
		"truck":        MotionRigidVehicle,
		"bus":          MotionRigidVehicle,
		"cyclist":      MotionTwoWheeler,
		"motorcyclist": MotionTwoWheeler,
		"pedestrian":   MotionPedestrian,
		"dynamic":      MotionUnknown,
		"bird":         MotionUnknown,
		"":             MotionUnknown,
		"spaceship":    MotionUnknown,
	} {
		if got := MotionClassForLabel(label); got != want {
			t.Errorf("label %q mapped to %v, want %v", label, got, want)
		}
	}
}

func TestPedestrianHeadingIsNotVelocityDirection(t *testing.T) {
	// The reason the pedestrian class exists. Inferring heading from velocity
	// makes ordinary sideways stepping look like a tracking failure.
	if MotionPedestrian.HeadingFollowsVelocity() {
		t.Error("a pedestrian's heading may be inferred from velocity direction")
	}
	if MotionUnknown.HeadingFollowsVelocity() {
		t.Error("an unclassified object's heading may be inferred from velocity direction")
	}
	if !MotionRigidVehicle.HeadingFollowsVelocity() || !MotionTwoWheeler.HeadingFollowsVelocity() {
		t.Error("a vehicle or two-wheeler heading may not be inferred from velocity")
	}
}

func TestPriorStrengthScalesWithPosteriorRatherThanSnapping(t *testing.T) {
	// Rule 1 of Section 5.5: classification uncertainty must not silently
	// become motion certainty.
	confident := MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.95}
	unsure := MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.35}

	_, strongWeight := confident.Effective()
	_, weakWeight := unsure.Effective()
	if strongWeight <= weakWeight {
		t.Errorf("prior strength did not scale with the posterior: %v at 0.95 against %v at 0.35",
			strongWeight, weakWeight)
	}
	if strongWeight > 1 || weakWeight < 0 {
		t.Errorf("prior strength outside [0,1]: %v, %v", strongWeight, weakWeight)
	}

	// Unknown has no priors to strengthen, whatever its posterior.
	class, weight := MotionClassBelief{Class: MotionUnknown, Posterior: 0.99}.Effective()
	if class != MotionUnknown || weight != 0 {
		t.Errorf("unknown class returned %v at strength %v, want unknown at 0", class, weight)
	}
}

func TestSplitPosteriorRunsWithTheWeakerMotionModel(t *testing.T) {
	// Rule 2: cyclist against pedestrian at walking pace is the motivating
	// case. Picking a winner on a small margin would impose forward-biased
	// dynamics on something that may step sideways.
	split := MotionClassBelief{
		Class: MotionTwoWheeler, Posterior: 0.45,
		Runner: MotionPedestrian, RunnerPosterior: 0.40,
	}
	got, _ := split.Effective()
	if got != MotionPedestrian {
		t.Errorf("a split posterior ran with %v, want the weaker pedestrian model", got)
	}

	// A clear margin keeps the leader.
	clear := MotionClassBelief{
		Class: MotionTwoWheeler, Posterior: 0.80,
		Runner: MotionPedestrian, RunnerPosterior: 0.10,
	}
	if got, _ := clear.Effective(); got != MotionTwoWheeler {
		t.Errorf("a clear posterior ran with %v, want two_wheeler", got)
	}
}

// --- Orientation -----------------------------------------------------------

func TestUnresolvedOrientationOffersBothModes(t *testing.T) {
	o := OrientationBelief{PsiRad: 0.3, AmbiguousModeWeight: 0.5, Provenance: ProvenanceObserved}
	if o.IsResolved() {
		t.Error("a bimodal orientation reports itself resolved")
	}
	modes := o.Modes()
	if len(modes) != 2 {
		t.Fatalf("got %d modes, want 2", len(modes))
	}
	if diff := math.Abs(float64(angleDifference(modes[0], modes[1]))); math.Abs(diff-math.Pi) > 1e-6 {
		t.Errorf("modes are %v apart, want half a turn", diff)
	}
}

func TestResolveWithCollapsesToTheNearerMode(t *testing.T) {
	// A heading opposing the reference must flip; one agreeing must not.
	for _, tc := range []struct {
		name      string
		psi       float32
		reference float32
		wantPsi   float32
	}{
		{"agrees with reference", 0.1, 0.2, 0.1},
		{"opposes reference", float32(math.Pi) - 0.1, 0.0, -0.1},
		{"just inside a quarter turn", 1.0, 0.0, 1.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := OrientationBelief{PsiRad: tc.psi, AmbiguousModeWeight: 0.5, Provenance: ProvenanceObserved}
			o.ResolveWith(tc.reference)
			if !o.IsResolved() {
				t.Error("orientation did not collapse")
			}
			if math.Abs(float64(angleDifference(o.PsiRad, tc.wantPsi))) > 1e-5 {
				t.Errorf("psi = %v, want %v", o.PsiRad, tc.wantPsi)
			}
		})
	}
}

func TestResolveWithCannotManufactureAnOrientation(t *testing.T) {
	// A reference direction is a cue for choosing between modes, not a source
	// of orientation in its own right.
	o := OrientationBelief{Provenance: ProvenanceNone}
	o.ResolveWith(1.2)
	if o.Provenance != ProvenanceNone || o.PsiRad != 0 {
		t.Errorf("resolving an empty belief produced psi=%v provenance=%v", o.PsiRad, o.Provenance)
	}
	if o.IsResolved() {
		t.Error("an empty orientation belief reports itself resolved")
	}
}

// --- Surface projection and suppression ------------------------------------

// projectable is an estimate with everything a bounded projection needs.
func projectable() SolidBodyEstimate {
	return SolidBodyEstimate{
		StateModel:         StateModelCVCartesianV1,
		Reference:          ReferenceBodyCentre,
		X:                  10,
		Y:                  0,
		PositionCovariance: [4]float32{0.04, 0, 0, 0.04},
		Orientation: OrientationBelief{
			PsiRad: 0, VarianceRad2: 0.01, Provenance: ProvenanceObserved,
		},
		Length: DimensionBelief{
			Metres: 4.5, SigmaMetres: 0.3, AdmissibleFrames: 5, Provenance: ProvenanceAccumulated,
		},
		Width: DimensionBelief{
			Metres: 1.9, SigmaMetres: 0.2, AdmissibleFrames: 5, Provenance: ProvenanceAccumulated,
		},
		Estimation: EstimationEstablished,
		Stage:      StageLive,
	}
}

func TestProjectSurfacePlacesFacesHalfALengthApartAlongTheBodyAxis(t *testing.T) {
	e := projectable()

	front, ok := e.ProjectSurface(SurfaceFront)
	if !ok {
		t.Fatal("a fully-evidenced estimate could not project its front surface")
	}
	rear, ok := e.ProjectSurface(SurfaceRear)
	if !ok {
		t.Fatal("a fully-evidenced estimate could not project its rear surface")
	}

	// psi = 0, so the body axis is +X.
	if math.Abs(float64(front.X-(e.X+e.Length.Metres/2))) > 1e-5 {
		t.Errorf("front X = %v, want %v", front.X, e.X+e.Length.Metres/2)
	}
	if math.Abs(float64(rear.X-(e.X-e.Length.Metres/2))) > 1e-5 {
		t.Errorf("rear X = %v, want %v", rear.X, e.X-e.Length.Metres/2)
	}
	if math.Abs(float64(front.Y)) > 1e-5 || math.Abs(float64(rear.Y)) > 1e-5 {
		t.Errorf("faces left the body axis: front Y=%v rear Y=%v", front.Y, rear.Y)
	}
	if gap := front.X - rear.X; math.Abs(float64(gap-e.Length.Metres)) > 1e-5 {
		t.Errorf("faces are %v apart, want the body length %v", gap, e.Length.Metres)
	}
	if front.FromClassPrior {
		t.Error("a projection from accumulated evidence is flagged as coming from the class prior")
	}
}

func TestProjectSurfaceRotatesWithOrientation(t *testing.T) {
	e := projectable()
	e.Orientation.PsiRad = float32(math.Pi / 2) // body axis along +Y

	front, ok := e.ProjectSurface(SurfaceFront)
	if !ok {
		t.Fatal("projection unavailable")
	}
	if math.Abs(float64(front.X-e.X)) > 1e-5 {
		t.Errorf("front X = %v, want unchanged at %v for a +Y axis", front.X, e.X)
	}
	if want := e.Y + e.Length.Metres/2; math.Abs(float64(front.Y-want)) > 1e-5 {
		t.Errorf("front Y = %v, want %v", front.Y, want)
	}
}

func TestProjectionBoundGrowsWithEveryUncertaintyItCombines(t *testing.T) {
	// The bound is the point of the type: pose, orientation and dimension are
	// each uncertain, so a projection that ignored any of them would be a
	// confident-looking number with no basis.
	base := projectable()
	baseProj, ok := base.ProjectSurface(SurfaceFront)
	if !ok {
		t.Fatal("projection unavailable")
	}

	for _, tc := range []struct {
		name   string
		mutate func(*SolidBodyEstimate)
	}{
		{"position covariance", func(e *SolidBodyEstimate) {
			e.PositionCovariance = [4]float32{0.25, 0, 0, 0.25}
		}},
		{"length sigma", func(e *SolidBodyEstimate) { e.Length.SigmaMetres = 1.2 }},
		{"orientation variance", func(e *SolidBodyEstimate) { e.Orientation.VarianceRad2 = 0.3 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := projectable()
			tc.mutate(&e)
			got, ok := e.ProjectSurface(SurfaceFront)
			if !ok {
				t.Fatal("projection unavailable")
			}
			if got.SigmaMetres <= baseProj.SigmaMetres {
				t.Errorf("widening the %s did not widen the bound: %v against %v",
					tc.name, got.SigmaMetres, baseProj.SigmaMetres)
			}
		})
	}
}

func TestProjectionBoundUsesTheCovarianceCrossTerm(t *testing.T) {
	// Section 5.4 requires updates to preserve correlation between pose,
	// heading and dimensions well enough to bound a projected bumper. A bound
	// that read only the diagonal would discard exactly that correlation.
	e := projectable()
	e.Orientation.PsiRad = float32(math.Pi / 4) // diagonal, so xy contributes

	withoutCross, ok := e.ProjectSurface(SurfaceFront)
	if !ok {
		t.Fatal("projection unavailable")
	}

	e.PositionCovariance = [4]float32{0.04, 0.03, 0.03, 0.04}
	withCross, ok := e.ProjectSurface(SurfaceFront)
	if !ok {
		t.Fatal("projection unavailable")
	}

	if withCross.SigmaMetres <= withoutCross.SigmaMetres {
		t.Errorf("a positive cross term did not widen the bound along the diagonal: %v against %v",
			withCross.SigmaMetres, withoutCross.SigmaMetres)
	}
}

func TestProjectSurfaceIsSuppressedRatherThanApproximated(t *testing.T) {
	// Every one of these must suppress. Section 5.4: if the joint bound is
	// unavailable, headway is suppressed — not substituted with a looser
	// number, and never with the current OBB.
	for _, tc := range []struct {
		name   string
		mutate func(*SolidBodyEstimate)
	}{
		{"still initialising", func(e *SolidBodyEstimate) { e.Estimation = EstimationInitialising }},
		{"model invalid", func(e *SolidBodyEstimate) { e.Estimation = EstimationModelInvalid }},
		{"reference is the cluster medoid", func(e *SolidBodyEstimate) { e.Reference = ReferenceClusterMedoid }},
		{"reference unnamed", func(e *SolidBodyEstimate) { e.Reference = ReferenceUnknown }},
		{"orientation unresolved", func(e *SolidBodyEstimate) { e.Orientation.AmbiguousModeWeight = 0.5 }},
		{"orientation absent", func(e *SolidBodyEstimate) { e.Orientation = OrientationBelief{} }},
		{"length absent", func(e *SolidBodyEstimate) { e.Length = DimensionBelief{} }},
		{"length not positive", func(e *SolidBodyEstimate) { e.Length.Metres = 0 }},
		{"position covariance absent", func(e *SolidBodyEstimate) { e.PositionCovariance = [4]float32{} }},
		{"position covariance negative", func(e *SolidBodyEstimate) {
			e.PositionCovariance = [4]float32{-1, 0, 0, -1}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := projectable()
			tc.mutate(&e)
			if got, ok := e.ProjectSurface(SurfaceFront); ok {
				t.Errorf("projected a surface at sigma %v when %s", got.SigmaMetres, tc.name)
			}
		})
	}
}

func TestAClassPriorLengthProjectsButSaysSo(t *testing.T) {
	// A weaker claim, not no claim: the extent was assumed, and the consumer
	// is told.
	e := projectable()
	e.Length = DimensionBelief{Metres: 4.5, SigmaMetres: 1.5, Provenance: ProvenanceClassPrior}

	got, ok := e.ProjectSurface(SurfaceFront)
	if !ok {
		t.Fatal("a class-prior length suppressed the projection entirely, want a flagged one")
	}
	if !got.FromClassPrior {
		t.Error("a projection built on the class prior is not flagged as such")
	}
}

func TestDegradedEstimatesStillProjectBecauseCoastingIsNotIgnorance(t *testing.T) {
	// temporarily_degraded consumes as "coasted states, marked". Suppressing
	// it entirely would lose the track's own last good belief during a bounded
	// occlusion, which is what the solid body exists to carry across.
	e := projectable()
	e.Estimation = EstimationTemporarilyDegraded
	e.Support.CoastedFrames = 3

	if _, ok := e.ProjectSurface(SurfaceFront); !ok {
		t.Error("a temporarily degraded estimate could not project a surface")
	}
	if !e.Estimation.MetricsAreProvisional() {
		t.Error("a degraded estimate's metrics are not marked provisional")
	}
	if e.Support.IsObserved() {
		t.Error("a coasted frame reports itself as observed")
	}
}

// --- Estimation lifecycle --------------------------------------------------

func TestEstimationStateGovernsWhatMayBeConsumed(t *testing.T) {
	for _, tc := range []struct {
		state       EstimationState
		pose        bool
		metric      bool
		provisional bool
	}{
		{EstimationInitialising, false, false, false},
		{EstimationGeometryConverging, true, true, true},
		{EstimationEstablished, true, true, false},
		{EstimationTemporarilyDegraded, true, false, true},
		{EstimationModelInvalid, false, false, false},
	} {
		t.Run(tc.state.String(), func(t *testing.T) {
			if got := tc.state.AllowsReportedPose(); got != tc.pose {
				t.Errorf("AllowsReportedPose = %v, want %v", got, tc.pose)
			}
			if got := tc.state.AllowsBehaviourMetric(); got != tc.metric {
				t.Errorf("AllowsBehaviourMetric = %v, want %v", got, tc.metric)
			}
			if got := tc.state.MetricsAreProvisional(); got != tc.provisional {
				t.Errorf("MetricsAreProvisional = %v, want %v", got, tc.provisional)
			}
		})
	}
}

func TestATrackMayNotLeaveInitialisingOnFrameCountAlone(t *testing.T) {
	bounds := DefaultConvergenceBounds()
	seed := SeedSolidBody(MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9}, 0, 0, 1)

	// Sustained association but below the heading-observability floor: the
	// geometry cannot be converging, because heading is not yet observable.
	slow := NextEstimationState(EstimationInitialising, seed, EstimationEvidence{
		SustainedAssociation: true,
		SpeedMps:             bounds.MinSpeedForHeadingMps - 0.01,
	}, bounds)
	if slow != EstimationInitialising {
		t.Errorf("a slow but well-associated track advanced to %v", slow)
	}

	// Moving, but association is not yet credible.
	unassociated := NextEstimationState(EstimationInitialising, seed, EstimationEvidence{
		SustainedAssociation: false,
		SpeedMps:             10,
	}, bounds)
	if unassociated != EstimationInitialising {
		t.Errorf("a moving track with weak association advanced to %v", unassociated)
	}

	// Both, and it advances.
	both := NextEstimationState(EstimationInitialising, seed, EstimationEvidence{
		SustainedAssociation: true, SpeedMps: 10,
	}, bounds)
	if both != EstimationGeometryConverging {
		t.Errorf("a moving, well-associated track stayed at %v", both)
	}
}

func TestEstablishedRequiresConvergedGeometryNotJustMotion(t *testing.T) {
	bounds := DefaultConvergenceBounds()
	ev := EstimationEvidence{SustainedAssociation: true, SpeedMps: 10}

	// Everything converged: established.
	good := projectable()
	if got := NextEstimationState(EstimationGeometryConverging, good, ev, bounds); got != EstimationEstablished {
		t.Errorf("a converged estimate reached %v, want established", got)
	}

	// Each missing piece individually holds it back.
	for _, tc := range []struct {
		name   string
		mutate func(*SolidBodyEstimate)
	}{
		{"length still a prior", func(e *SolidBodyEstimate) { e.Length.Provenance = ProvenanceClassPrior }},
		{"width sigma too wide", func(e *SolidBodyEstimate) { e.Width.SigmaMetres = 5 }},
		{"too few admissible frames", func(e *SolidBodyEstimate) { e.Length.AdmissibleFrames = 1 }},
		{"orientation unresolved", func(e *SolidBodyEstimate) { e.Orientation.AmbiguousModeWeight = 0.5 }},
		{"orientation variance too wide", func(e *SolidBodyEstimate) { e.Orientation.VarianceRad2 = 1 }},
		{"reference not physical", func(e *SolidBodyEstimate) { e.Reference = ReferenceClusterMedoid }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := projectable()
			tc.mutate(&e)
			if got := NextEstimationState(EstimationGeometryConverging, e, ev, bounds); got == EstimationEstablished {
				t.Errorf("reached established with %s", tc.name)
			}
		})
	}
}

func TestDegradedIsOnlyReachableFromEarnedBelief(t *testing.T) {
	bounds := DefaultConvergenceBounds()
	seed := SeedSolidBody(MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9}, 0, 0, 1)
	inadequate := EstimationEvidence{EvidenceInadequate: true}

	// A track that never established has nothing to degrade from: it goes on
	// waiting rather than being described as having lost something.
	if got := NextEstimationState(EstimationInitialising, seed, inadequate, bounds); got != EstimationInitialising {
		t.Errorf("an initialising track degraded to %v", got)
	}
	if got := NextEstimationState(EstimationGeometryConverging, seed, inadequate, bounds); got != EstimationGeometryConverging {
		t.Errorf("a converging track degraded to %v", got)
	}
	// One that did establish degrades.
	if got := NextEstimationState(EstimationEstablished, projectable(), inadequate, bounds); got != EstimationTemporarilyDegraded {
		t.Errorf("an established track with inadequate evidence went to %v", got)
	}
	// And recovers when the evidence returns.
	recovered := NextEstimationState(EstimationTemporarilyDegraded, projectable(),
		EstimationEvidence{SustainedAssociation: true, SpeedMps: 10}, bounds)
	if recovered != EstimationEstablished {
		t.Errorf("a degraded track with good evidence recovered to %v", recovered)
	}
}

func TestModelInvalidIsTerminalHere(t *testing.T) {
	bounds := DefaultConvergenceBounds()
	// It overrides every other signal.
	if got := NextEstimationState(EstimationEstablished, projectable(), EstimationEvidence{
		ModelInvalid: true, SustainedAssociation: true, SpeedMps: 10,
	}, bounds); got != EstimationModelInvalid {
		t.Errorf("model invalidity was overridden, got %v", got)
	}
	// And nothing in this function re-establishes belief: recovery is
	// Section 12's business, not a threshold comparison.
	if got := NextEstimationState(EstimationModelInvalid, projectable(), EstimationEvidence{
		SustainedAssociation: true, SpeedMps: 10,
	}, bounds); got != EstimationModelInvalid {
		t.Errorf("an invalid model recovered to %v on thresholds alone", got)
	}
}

func TestAnEstablishedTrackThatSlowsKeepsWhatItEarned(t *testing.T) {
	// The speed floor governs whether heading is newly observable, not whether
	// a body that was already measured still exists.
	bounds := DefaultConvergenceBounds()
	e := projectable()
	e.Orientation.AmbiguousModeWeight = 0.5 // no longer converged

	got := NextEstimationState(EstimationEstablished, e, EstimationEvidence{
		SustainedAssociation: true, SpeedMps: 0,
	}, bounds)
	if got != EstimationEstablished {
		t.Errorf("an established track that stopped fell back to %v", got)
	}
}

// --- Reading the estimate off a track --------------------------------------

func TestSolidBodyFromTrackSlicesTheFiltersOwnCovariance(t *testing.T) {
	// Recomputing it would let the projected bound disagree with the
	// covariance the filter is actually gating on.
	tr := &TrackedObject{
		X: 3, Y: 4,
		P: [16]float32{
			0.11, 0.02, 9, 9,
			0.03, 0.14, 9, 9,
			9, 9, 9, 9,
			9, 9, 9, 9,
		},
	}
	tr.ObservationCount = 5

	e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
		DefaultConvergenceBounds())

	want := [4]float32{0.11, 0.02, 0.03, 0.14}
	if e.PositionCovariance != want {
		t.Errorf("position covariance = %v, want the filter's 2x2 position block %v",
			e.PositionCovariance, want)
	}
	if e.X != 3 || e.Y != 4 {
		t.Errorf("pose = (%v,%v), want the track's (3,4)", e.X, e.Y)
	}
	if e.StateModel != StateModelCVCartesianV1 {
		t.Errorf("state model = %q, want the discriminator", e.StateModel)
	}
}

func TestSolidBodyFromTrackMarksAccumulatedDimensionsAsSuch(t *testing.T) {
	tr := &TrackedObject{X: 1, Y: 1}
	tr.ObservationCount = 10
	// A span seen enough times to be believed.
	for i := 0; i < 5; i++ {
		tr.lengthBelief.Observe(4.4)
		tr.widthBelief.Observe(1.8)
	}

	e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
		DefaultConvergenceBounds())

	if e.Length.Provenance != ProvenanceAccumulated {
		t.Errorf("length provenance = %v, want accumulated: a partial extent constrains without measuring",
			e.Length.Provenance)
	}
	if e.Length.AdmissibleFrames != 5 {
		t.Errorf("length admissible frames = %d, want 5", e.Length.AdmissibleFrames)
	}
	if e.Length.Metres <= 0 {
		t.Errorf("length = %v, want the believed span", e.Length.Metres)
	}
	// Height has no accumulator: the OBB's vertical extent is corrupted by the
	// P11 grade artefact, so it stays the prior until an admissibility rule
	// exists for it.
	if e.Height.Provenance != ProvenanceClassPrior {
		t.Errorf("height provenance = %v, want class_prior", e.Height.Provenance)
	}
}

func TestSolidBodyFromTrackFallsBackToThePriorWithoutSupport(t *testing.T) {
	tr := &TrackedObject{X: 1, Y: 1}
	tr.ObservationCount = 1

	e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
		DefaultConvergenceBounds())

	if e.Length.Provenance != ProvenanceClassPrior {
		t.Errorf("length provenance = %v, want class_prior with no accumulated support", e.Length.Provenance)
	}
	prior := dimensionPriorFor(MotionRigidVehicle)
	if e.Length.Metres != prior.lengthMetres {
		t.Errorf("length = %v, want the class prior %v", e.Length.Metres, prior.lengthMetres)
	}
}

func TestSolidBodyFromTrackRefusesAHeldHeading(t *testing.T) {
	// A locked heading is the tracker declining to believe the measurement.
	// Carrying it into the estimate as observed orientation would launder a
	// refusal into evidence.
	tr := &TrackedObject{X: 1, Y: 1, OBBHeadingRad: 0.7}
	tr.ObservationCount = 5
	tr.HeadingSource = HeadingSourceLocked

	e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
		DefaultConvergenceBounds())

	if e.Orientation.Provenance != ProvenanceNone {
		t.Errorf("a locked heading was recorded with provenance %v", e.Orientation.Provenance)
	}
	if _, ok := e.ProjectSurface(SurfaceFront); ok {
		t.Error("an estimate built on a locked heading projected a surface")
	}
}

func TestSolidBodyFromTrackKeepsPCAHeadingBimodal(t *testing.T) {
	// PCA and the axis path recover an axis, not a direction, so front and
	// rear are interchangeable and a bumper cannot be projected.
	for _, source := range []HeadingSource{HeadingSourcePCA, HeadingSourceAxis} {
		tr := &TrackedObject{X: 1, Y: 1, OBBHeadingRad: 0.7}
		tr.ObservationCount = 5
		tr.HeadingSource = source
		tr.HeadingJitterCount = 10
		tr.HeadingJitterSumSq = 0.01

		e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
			DefaultConvergenceBounds())

		if e.Orientation.IsResolved() {
			t.Errorf("%v heading was treated as a resolved direction", source)
		}
		if len(e.Orientation.Modes()) != 2 {
			t.Errorf("%v heading offered %d modes, want 2", source, len(e.Orientation.Modes()))
		}
		if _, ok := e.ProjectSurface(SurfaceFront); ok {
			t.Errorf("%v heading projected a front surface despite the direction ambiguity", source)
		}
	}
}

func TestSolidBodyFromTrackAcceptsADisambiguatedHeading(t *testing.T) {
	// Velocity and displacement resolve the direction, so the ambiguity has
	// genuinely collapsed and a bumper becomes projectable.
	for _, source := range []HeadingSource{HeadingSourceVelocity, HeadingSourceDisplacement} {
		tr := &TrackedObject{
			X: 1, Y: 1, VX: 10, OBBHeadingRad: 0.05,
			P: [16]float32{0.04, 0, 0, 0, 0, 0.04, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}
		tr.ObservationCount = 5
		tr.HeadingSource = source
		tr.HeadingJitterCount = 20
		tr.HeadingJitterSumSq = 0.02
		for i := 0; i < 5; i++ {
			tr.lengthBelief.Observe(4.4)
			tr.widthBelief.Observe(1.8)
		}

		e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
			DefaultConvergenceBounds())
		e.Estimation = EstimationEstablished

		if !e.Orientation.IsResolved() {
			t.Errorf("%v heading was not treated as resolved", source)
		}
		if _, ok := e.ProjectSurface(SurfaceFront); !ok {
			t.Errorf("%v heading could not project a front surface", source)
		}
	}
}

func TestHeadingVarianceIsWideWithoutJitterHistory(t *testing.T) {
	// One frame's heading is unconstrained, not perfectly known. Reporting
	// zero variance there would be the confident-seed error in another place.
	tr := &TrackedObject{X: 1, Y: 1, OBBHeadingRad: 0.3}
	tr.ObservationCount = 1
	tr.HeadingSource = HeadingSourceVelocity

	e := SolidBodyFromTrack(tr, MotionClassBelief{Class: MotionRigidVehicle, Posterior: 0.9},
		DefaultConvergenceBounds())

	if e.Orientation.VarianceRad2 <= DefaultConvergenceBounds().MaxOrientationVarianceRad2 {
		t.Errorf("orientation variance %v with no jitter history is inside the convergence bound",
			e.Orientation.VarianceRad2)
	}
}

func TestExtentBeliefSigmaNarrowsWithSupportAndWidensOnConflict(t *testing.T) {
	const priorSigma float32 = 1.5

	var thin extentBelief
	thin.Observe(4.4)
	var thick extentBelief
	for i := 0; i < 9; i++ {
		thick.Observe(4.4)
	}

	if extentBeliefSigma(thick, priorSigma) >= extentBeliefSigma(thin, priorSigma) {
		t.Errorf("sigma did not narrow with corroboration: %v at 9 against %v at 1",
			extentBeliefSigma(thick, priorSigma), extentBeliefSigma(thin, priorSigma))
	}

	// A conflict is evidence disagreeing with the model, which is a reason to
	// be less sure rather than to pick a winner.
	conflicted := thick
	conflicted.Observe(extentBeliefMaxMetres + 1)
	if conflicted.Conflicts == 0 {
		t.Fatal("an oversized span was not recorded as a conflict")
	}
	if extentBeliefSigma(conflicted, priorSigma) <= extentBeliefSigma(thick, priorSigma) {
		t.Errorf("a conflict did not widen the sigma: %v against %v",
			extentBeliefSigma(conflicted, priorSigma), extentBeliefSigma(thick, priorSigma))
	}

	// Never wider than the prior it started from, and never below the
	// quantisation that produced it.
	if got := extentBeliefSigma(conflicted, priorSigma); got > priorSigma {
		t.Errorf("sigma %v exceeded the prior %v", got, priorSigma)
	}
	var huge extentBelief
	for i := 0; i < 10_000; i++ {
		huge.Observe(4.4)
	}
	if got := extentBeliefSigma(huge, priorSigma); got < float32(extentBeliefBinMetres) {
		t.Errorf("sigma %v fell below the bin width %v", got, extentBeliefBinMetres)
	}
}
