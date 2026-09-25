package l8behaviour

// The mapping from the tracker's solid-body estimate to the trajectory
// contract. It is deliberately thin and deliberately conservative: where
// l5tracks cannot say something, the sample says less rather than guessing.
//
//   - Stage. l5tracks knows "live" and "smoothed". Live is the online stage.
//     Smoothed may be fixed-lag or final, and is mapped to fixed_lag: final is
//     never inferred from memory, only read from a persisted final row, so a
//     tracker estimate can never pass the production-emission guard by
//     accident.
//   - Faces. l5tracks does not yet report which end faces had returns, so no
//     face is claimed as observed and every endpoint is at best temporally
//     inferred.
//   - Support. A coasted frame is coasted and a fragmented one is
//     cluster_split. Occlusion, unexplained misses and field-of-view exits
//     need scene reasoning the tracker does not do, so they are not claimed.
//   - Reference. A near-face-centre reference needs a declared offset to the
//     body centre, which the solid-body contract does not carry yet, so it is
//     refused rather than treated as the centre.
//
// Nothing here changes l5tracks behaviour; it only reads its types.

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// DynamicState is the part of the tracker state the solid-body estimate does
// not carry: velocity and the full 4x4 covariance over [x, y, vx, vy].
type DynamicState struct {
	VX, VY     float32
	Covariance [16]float32
}

// MotionClassFromL5 maps the tracker's motion class to the contract's. The
// mapping is total: l5tracks' zero value is unknown, a real class.
func MotionClassFromL5(m l5tracks.MotionClass) MotionClass {
	switch m {
	case l5tracks.MotionRigidVehicle:
		return MotionRigidVehicle
	case l5tracks.MotionTwoWheeler:
		return MotionTwoWheeler
	case l5tracks.MotionPedestrian:
		return MotionPedestrian
	default:
		return MotionUnknown
	}
}

// EstimationStateFromL5 maps the estimation lifecycle one to one.
func EstimationStateFromL5(s l5tracks.EstimationState) (EstimationState, error) {
	switch s {
	case l5tracks.EstimationInitialising:
		return EstimationInitialising, nil
	case l5tracks.EstimationGeometryConverging:
		return EstimationGeometryConverging, nil
	case l5tracks.EstimationEstablished:
		return EstimationEstablished, nil
	case l5tracks.EstimationTemporarilyDegraded:
		return EstimationTemporarilyDegraded, nil
	case l5tracks.EstimationModelInvalid:
		return EstimationModelInvalid, nil
	}
	return EstimationUnspecified, fmt.Errorf("unknown l5tracks estimation state %d", s)
}

// EstimateStageFromL5 maps an in-memory stage, never to final.
func EstimateStageFromL5(s l5tracks.EstimateStage) EstimateStage {
	if s == l5tracks.StageSmoothed {
		return StageFixedLag
	}
	return StageOnline
}

// BeliefProvenanceFromL5 maps a provenance; l5tracks' "none" is unspecified.
func BeliefProvenanceFromL5(p l5tracks.Provenance) BeliefProvenance {
	switch p {
	case l5tracks.ProvenanceClassPrior:
		return ProvenanceClassPrior
	case l5tracks.ProvenanceAccumulated:
		return ProvenanceAccumulated
	case l5tracks.ProvenanceObserved:
		return ProvenanceObserved
	default:
		return ProvenanceUnspecified
	}
}

// SupportStateFromL5 maps the tracker's frame support. Truncation keeps the
// frame observed: the detection is present, only its extent is not admissible
// as dimension evidence, which the extent belief already accounts for.
func SupportStateFromL5(s l5tracks.SupportState) SupportState {
	switch {
	case !s.IsObserved():
		return SupportCoasted
	case s.Fragmented:
		return SupportClusterSplit
	default:
		return SupportObserved
	}
}

// PassageFromL5 builds a passage's identity and class from the tracker's class
// belief. The class is the belief's effective class, so a posterior split
// between classes with different motion models takes the weaker one rather
// than the argmax (state-estimation plan, Section 5.5).
func PassageFromL5(trackID, siteID, sensorID string, belief l5tracks.MotionClassBelief, classLabel string) Passage {
	class, _ := belief.Effective()
	p := Passage{
		TrackID: trackID, SiteID: siteID, SensorID: sensorID,
		MotionClass: MotionClassFromL5(class),
		ClassLabel:  classLabel,
	}
	if belief.Posterior >= 0 && belief.Posterior <= 1 {
		p.ClassConfidence = ptr(float64(belief.Posterior))
	}
	return p
}

// SampleFromSolidBody builds a trajectory sample from a solid-body estimate,
// the dynamic state it was read with, and the convergence bounds that decide
// each extent's Converged flag.
//
// The covariance's position block must equal the estimate's own
// PositionCovariance: they are sliced from one filter state, so a mismatch
// means the two were read at different frames and the sample would describe
// neither.
func SampleFromSolidBody(
	body l5tracks.SolidBodyEstimate,
	dyn DynamicState,
	captureUnixNanos int64,
	bounds l5tracks.ConvergenceBounds,
) (TrajectorySample, error) {
	if body.StateModel != l5tracks.StateModelCVCartesianV1 {
		return TrajectorySample{}, fmt.Errorf("solid body state model %q is not %q", body.StateModel, l5tracks.StateModelCVCartesianV1)
	}
	block := [4]float32{dyn.Covariance[0], dyn.Covariance[1], dyn.Covariance[4], dyn.Covariance[5]}
	if block != body.PositionCovariance {
		return TrajectorySample{}, fmt.Errorf("covariance position block %v disagrees with the solid body's %v", block, body.PositionCovariance)
	}

	var reference ReferencePoint
	switch body.Reference {
	case l5tracks.ReferenceBodyCentre:
		reference = ReferenceBodyCentre
	case l5tracks.ReferenceClusterMedoid:
		reference = ReferenceClusterMedoid
	default:
		return TrajectorySample{}, fmt.Errorf("solid body reference %s has no declared offset to the body centre", body.Reference)
	}
	estimation, err := EstimationStateFromL5(body.Estimation)
	if err != nil {
		return TrajectorySample{}, err
	}

	lastObserved := body.LastObservedUnixNanos
	support := SupportStateFromL5(body.Support)
	if support == SupportObserved {
		// The tracker stamps the measurement's own acquisition time, which
		// may precede the frame's capture time by the cluster's acquisition
		// offset; an observed instant is observed at its capture time.
		lastObserved = captureUnixNanos
	}

	var cov [16]float64
	for i, v := range dyn.Covariance {
		cov[i] = float64(v)
	}
	s := TrajectorySample{
		CaptureUnixNanos:      captureUnixNanos,
		StateModel:            StateModelCVCartesianV1,
		Reference:             reference,
		X:                     float64(body.X),
		Y:                     float64(body.Y),
		VX:                    float64(dyn.VX),
		VY:                    float64(dyn.VY),
		Covariance:            cov,
		Length:                extentFromL5(body.Length, bounds),
		Width:                 extentFromL5(body.Width, bounds),
		Support:               support,
		Stage:                 EstimateStageFromL5(body.Stage),
		Estimation:            estimation,
		LastObservedUnixNanos: lastObserved,
	}
	if body.Orientation.Provenance != l5tracks.ProvenanceNone {
		s.Heading = HeadingBelief{
			Rad:                 float64(body.Orientation.PsiRad),
			VarianceRad2:        float64(body.Orientation.VarianceRad2),
			AmbiguousModeWeight: float64(body.Orientation.AmbiguousModeWeight),
			Provenance:          BeliefProvenanceFromL5(body.Orientation.Provenance),
		}
	}
	// An estimate l5tracks calls established whose extents fail these bounds
	// was judged against different bounds; Validate reports the mismatch
	// rather than this function quietly downgrading the state.
	if err := s.Validate(); err != nil {
		return TrajectorySample{}, fmt.Errorf("solid body does not form a valid sample: %w", err)
	}
	return s, nil
}

// SampleFromTrack reads the solid-body estimate off a live track, applies the
// caller's estimation lifecycle state, and builds a sample from it. The
// lifecycle is an argument because l5tracks decides it with
// NextEstimationState over evidence this package does not see.
func SampleFromTrack(
	t *l5tracks.TrackedObject,
	class l5tracks.MotionClassBelief,
	estimation l5tracks.EstimationState,
	captureUnixNanos int64,
	bounds l5tracks.ConvergenceBounds,
) (TrajectorySample, error) {
	if t == nil {
		return TrajectorySample{}, fmt.Errorf("sample requires a track")
	}
	body := l5tracks.SolidBodyFromTrack(t, class, bounds)
	body.Estimation = estimation
	return SampleFromSolidBody(body, DynamicState{VX: t.VX, VY: t.VY, Covariance: t.P}, captureUnixNanos, bounds)
}

func extentFromL5(d l5tracks.DimensionBelief, bounds l5tracks.ConvergenceBounds) ExtentBelief {
	provenance := BeliefProvenanceFromL5(d.Provenance)
	if provenance == ProvenanceUnspecified {
		return ExtentBelief{}
	}
	return ExtentBelief{
		Metres:           float64(d.Metres),
		SigmaMetres:      float64(d.SigmaMetres),
		AdmissibleFrames: d.AdmissibleFrames,
		Provenance:       provenance,
		Converged:        d.IsConverged(bounds.MaxDimensionSigmaMetres, bounds.MinAdmissibleFrames),
	}
}
