package l5tracks

// Seeding and lifecycle for the solid-body estimate, per Sections 5.6 and 9.2
// of docs/plans/lidar-state-estimation-plan.md.
//
// The load-bearing idea here is that a biased seed with an honest covariance is
// recoverable and a biased seed with a confident covariance is not. Track birth
// is where that bites: position comes from a medoid biased toward the sensor,
// heading is unobservable, dimensions are a single partial view, and
// association is at its weakest. Everything below exists to stop that moment
// producing a confident physical prior.

import "math"

// classDimensionPrior is the assumed body geometry for a motion class, used
// before any frame has been admitted as dimension evidence.
//
// These are priors, not tuning keys, and deliberately so: TuningConfig's
// fingerprint hashes the whole resolved config, so exposing them would
// invalidate the committed perf baselines before anyone had measured whether
// they help. They are also marked ProvenanceClassPrior wherever they are used,
// so a consumer can never mistake one for a measurement of this object.
type classDimensionPrior struct {
	lengthMetres float32
	widthMetres  float32
	heightMetres float32
	// sigmaMetres is the spread within the class, not a measurement precision.
	// It is wide on purpose.
	sigmaMetres float32
}

func dimensionPriorFor(class MotionClass) classDimensionPrior {
	switch class {
	case MotionRigidVehicle:
		// Spans a small hatchback to a bus, which is why the sigma is large.
		return classDimensionPrior{lengthMetres: 4.5, widthMetres: 1.9, heightMetres: 1.6, sigmaMetres: 1.5}
	case MotionTwoWheeler:
		return classDimensionPrior{lengthMetres: 1.8, widthMetres: 0.7, heightMetres: 1.7, sigmaMetres: 0.4}
	case MotionPedestrian:
		return classDimensionPrior{lengthMetres: 0.6, widthMetres: 0.6, heightMetres: 1.7, sigmaMetres: 0.3}
	default:
		// Unknown: broad bounds, deliberately unhelpful rather than
		// confidently wrong.
		return classDimensionPrior{lengthMetres: 4.5, widthMetres: 2.0, heightMetres: 1.7, sigmaMetres: 2.5}
	}
}

// SeedSolidBody builds the estimate for a track that has just been created.
//
// The position is the medoid, because nothing better exists yet, and the
// position covariance reflects the medoid's known bias rather than its
// precision. That bias is up to half the body's width — the medoid sits on the
// sensor-facing surface, not the centre — so the seeded sigma is of that order.
// Using the measurement noise here instead would state roughly 0.22 m of
// uncertainty about a position that may be a metre out.
//
// Dimensions come from the class prior, marked as the prior. Orientation is
// seeded with no value at all rather than a guess, because at birth it is
// unobservable and a bimodal belief over a value nobody has measured would be
// two wrong answers rather than one.
func SeedSolidBody(class MotionClassBelief, medoidX, medoidY float32, nowNanos int64) SolidBodyEstimate {
	effective, _ := class.Effective()
	prior := dimensionPriorFor(effective)

	// The seed's one-sigma position uncertainty: half a body width, which is
	// the largest the medoid offset can be along the sensor direction.
	sigma := prior.widthMetres / 2
	variance := sigma * sigma

	return SolidBodyEstimate{
		StateModel: StateModelCVCartesianV1,
		Reference:  ReferenceClusterMedoid,
		X:          medoidX,
		Y:          medoidY,
		// Isotropic: at birth there is no orientation, so there is no axis
		// along which to prefer one direction's uncertainty over another's.
		PositionCovariance: [4]float32{variance, 0, 0, variance},
		Orientation:        OrientationBelief{Provenance: ProvenanceNone},
		Length: DimensionBelief{
			Metres: prior.lengthMetres, SigmaMetres: prior.sigmaMetres,
			Provenance: ProvenanceClassPrior,
		},
		Width: DimensionBelief{
			Metres: prior.widthMetres, SigmaMetres: prior.sigmaMetres,
			Provenance: ProvenanceClassPrior,
		},
		Height: DimensionBelief{
			Metres: prior.heightMetres, SigmaMetres: prior.sigmaMetres,
			Provenance: ProvenanceClassPrior,
		},
		Motion:                class,
		Estimation:            EstimationInitialising,
		Stage:                 StageLive,
		LastObservedUnixNanos: nowNanos,
		Support:               SupportState{},
	}
}

// ConvergenceBounds are the thresholds the established state requires. They are
// arguments rather than constants so a caller can tighten them for a gate run
// without moving the tuning fingerprint.
type ConvergenceBounds struct {
	// MaxDimensionSigmaMetres is the per-dimension sigma below which a
	// dimension counts as believed.
	MaxDimensionSigmaMetres float32
	// MinAdmissibleFrames is how many frames must have been admitted as
	// evidence for that dimension.
	MinAdmissibleFrames int
	// MaxOrientationVarianceRad2 is the orientation variance below which the
	// body's axis counts as believed.
	MaxOrientationVarianceRad2 float32
	// MinSpeedForHeadingMps is the heading-observability floor: below this
	// speed, velocity direction carries no usable orientation information.
	MinSpeedForHeadingMps float32
}

// DefaultConvergenceBounds are starting values, not gate-validated ones.
//
// The orientation bound is (10 degrees)^2: a 10-degree axis error moves a
// 4.5 m vehicle's projected bumper by roughly 0.4 m, which is the order of the
// defect this plan exists to fix, so believing the axis any less tightly than
// that would not support a headway claim.
func DefaultConvergenceBounds() ConvergenceBounds {
	const tenDegrees = 10 * math.Pi / 180
	return ConvergenceBounds{
		MaxDimensionSigmaMetres:    0.5,
		MinAdmissibleFrames:        3,
		MaxOrientationVarianceRad2: float32(tenDegrees * tenDegrees),
		MinSpeedForHeadingMps:      0.5,
	}
}

// EstimationEvidence is what the lifecycle decision is made from. It is passed
// explicitly rather than read off a track so that the rules can be tested
// against each condition in isolation.
type EstimationEvidence struct {
	// SustainedAssociation reports whether the track has held association
	// long enough to be credible. It is the caller's HitsToConfirm-style
	// judgement, not a frame count this function applies itself.
	SustainedAssociation bool
	// SpeedMps is the current speed, compared against the
	// heading-observability floor.
	SpeedMps float32
	// ModelInvalid marks the object model as no longer describing this object.
	ModelInvalid bool
	// EvidenceInadequate marks occlusion, fragmentation or sustained high NIS.
	EvidenceInadequate bool
}

// NextEstimationState decides the estimation state from the current one, the
// estimate's beliefs and this frame's evidence.
//
// A track may not leave initialising on frame count alone: it leaves when the
// listed evidence exists. That is why SustainedAssociation is a judgement the
// caller supplies rather than a counter this function keeps.
//
// The degraded state is reachable only from a state that had earned belief;
// a track that never got there goes on waiting rather than being described as
// having lost something it never had.
func NextEstimationState(
	current EstimationState,
	e SolidBodyEstimate,
	ev EstimationEvidence,
	bounds ConvergenceBounds,
) EstimationState {
	// Model invalidity overrides everything: the object being described may
	// not be the object that is there.
	if ev.ModelInvalid {
		return EstimationModelInvalid
	}
	// Once invalid, nothing here re-establishes belief. Recovery is Section
	// 12's business, not a threshold comparison.
	if current == EstimationModelInvalid {
		return EstimationModelInvalid
	}

	if ev.EvidenceInadequate {
		switch current {
		case EstimationEstablished, EstimationTemporarilyDegraded:
			return EstimationTemporarilyDegraded
		default:
			// Never established, so there is nothing to degrade from.
			return current
		}
	}

	established := e.Length.IsConverged(bounds.MaxDimensionSigmaMetres, bounds.MinAdmissibleFrames) &&
		e.Width.IsConverged(bounds.MaxDimensionSigmaMetres, bounds.MinAdmissibleFrames) &&
		e.Orientation.IsResolved() &&
		e.Orientation.VarianceRad2 <= bounds.MaxOrientationVarianceRad2 &&
		e.Reference.IsPhysical()

	if established {
		return EstimationEstablished
	}

	// Motion is credible but the geometry is still moving. The speed floor is
	// what makes heading observable at all, so below it a track stays
	// initialising however long it has been associated.
	if ev.SustainedAssociation && ev.SpeedMps >= bounds.MinSpeedForHeadingMps {
		return EstimationGeometryConverging
	}

	// A track that was established and is merely slow has not become
	// un-estimated; it keeps what it earned.
	if current == EstimationEstablished || current == EstimationTemporarilyDegraded {
		return current
	}
	return EstimationInitialising
}

// SolidBodyFromTrack reads the current solid-body estimate off a track.
//
// The dimension beliefs come from the track's extentBelief accumulators, which
// hold lower-bound spans from confidently assigned observations, so they are
// marked accumulated rather than observed: a partial visible extent constrains
// the dimension without measuring it. Where an accumulator has no support the
// class prior fills in and is marked as such.
//
// The position covariance is sliced out of the filter's own 4x4 rather than
// recomputed, so it cannot disagree with the covariance the filter is gating
// on.
func SolidBodyFromTrack(t *TrackedObject, class MotionClassBelief, bounds ConvergenceBounds) SolidBodyEstimate {
	effective, _ := class.Effective()
	prior := dimensionPriorFor(effective)

	e := SolidBodyEstimate{
		StateModel: StateModelCVCartesianV1,
		X:          t.X,
		Y:          t.Y,
		PositionCovariance: [4]float32{
			t.P[0*4+0], t.P[0*4+1],
			t.P[1*4+0], t.P[1*4+1],
		},
		GroundZ:               t.LatestZ,
		Motion:                class,
		Stage:                 StageLive,
		LastObservedUnixNanos: t.LastMeasurementUnixNanos,
		Support: SupportState{
			CoastedFrames: t.Misses,
		},
	}

	// The current measurement is an OBB centre, which is a place on the body
	// rather than a point in the cluster, so the reference is the body centre
	// once anything has been measured at all.
	if t.ObservationCount > 0 {
		e.Reference = ReferenceBodyCentre
	} else {
		e.Reference = ReferenceClusterMedoid
	}

	e.Length = dimensionFromBelief(t.lengthBelief, prior.lengthMetres, prior.sigmaMetres)
	e.Width = dimensionFromBelief(t.widthBelief, prior.widthMetres, prior.sigmaMetres)
	// Height has no accumulator: the OBB's vertical extent is corrupted by the
	// P11 grade artefact on any slope, so it is the class prior until a
	// GroundClipped-aware admissibility rule exists to admit it.
	e.Height = DimensionBelief{
		Metres: prior.heightMetres, SigmaMetres: prior.sigmaMetres,
		Provenance: ProvenanceClassPrior,
	}

	// Orientation: the track carries a smoothed heading and the source that
	// produced it. A held heading is not evidence about this frame, and a
	// heading that was never resolved against a direction cue stays bimodal.
	if t.ObservationCount > 0 && !t.HeadingSource.IsLocked() {
		e.Orientation = OrientationBelief{
			PsiRad:       t.OBBHeadingRad,
			VarianceRad2: headingVarianceFromJitter(t),
			Provenance:   ProvenanceObserved,
		}
		switch t.HeadingSource {
		case HeadingSourceVelocity, HeadingSourceDisplacement:
			// Resolved against a direction cue, so the ambiguity collapsed.
			e.Orientation.AmbiguousModeWeight = 0
		default:
			// PCA and the axis path recover an axis, not a direction.
			e.Orientation.AmbiguousModeWeight = 0.5
		}
	}

	return e
}

// dimensionFromBelief converts an accumulated extent belief into a dimension
// belief, falling back to the class prior where there is no support.
func dimensionFromBelief(b extentBelief, priorMetres, priorSigma float32) DimensionBelief {
	estimate := b.Estimate()
	if b.Support == 0 || estimate <= 0 {
		return DimensionBelief{
			Metres: priorMetres, SigmaMetres: priorSigma,
			Provenance: ProvenanceClassPrior,
		}
	}
	return DimensionBelief{
		Metres:           estimate,
		SigmaMetres:      extentBeliefSigma(b, priorSigma),
		AdmissibleFrames: b.Support,
		Provenance:       ProvenanceAccumulated,
	}
}

// extentBeliefSigma is the uncertainty to attach to an accumulated extent.
//
// The accumulator is a corroborated maximum over lower bounds, so its
// uncertainty falls with corroboration but never below the bin width that
// quantised it, and a conflict — a span too large to be a road user — widens it
// again rather than being discarded silently.
func extentBeliefSigma(b extentBelief, priorSigma float32) float32 {
	if b.Support <= 0 {
		return priorSigma
	}
	// Shrink from the prior toward the quantisation floor as support builds.
	sigma := priorSigma / float32(math.Sqrt(float64(b.Support)))
	if floor := float32(extentBeliefBinMetres); sigma < floor {
		sigma = floor
	}
	if b.Conflicts > 0 {
		// Evidence and model disagree about this object's size. That is a
		// reason to be less sure, not to pick a winner.
		sigma *= 1 + float32(b.Conflicts)
	}
	if sigma > priorSigma {
		sigma = priorSigma
	}
	return sigma
}

// headingVarianceFromJitter turns the track's accumulated frame-to-frame
// heading jitter into an orientation variance.
//
// This is an observed dispersion rather than a filter covariance, which is the
// honest thing available: orientation is smoothed by an EMA, not estimated by a
// filter that maintains its own uncertainty. It is marked as such by being
// derived here rather than stored on the track as if it were a covariance.
func headingVarianceFromJitter(t *TrackedObject) float32 {
	if t.HeadingJitterCount <= 0 {
		// No jitter history: a single frame's heading, which is unconstrained
		// rather than perfectly known.
		return float32(math.Pi * math.Pi / 4) // (pi/2)^2
	}
	return float32(t.HeadingJitterSumSq / float64(t.HeadingJitterCount))
}
