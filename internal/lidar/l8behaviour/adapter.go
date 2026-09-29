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
//   - Faces. SampleFromSolidBody claims no face, so every endpoint is at best
//     temporally inferred. SampleFromSolidBodyReading claims the end faces a
//     near-edge fix used, and only on an observed instant with a resolved
//     heading; a face found but left out of the fix is not claimed, which
//     errs toward inferred.
//   - Support. A coasted frame keeps the tracker's explanation of its
//     absence when it made one (occluded_inferred, missed_unknown or
//     out_of_fov, under OcclusionContinuityConfig.ExplainAbsence) and is
//     coasted otherwise. A fragmented observed frame is cluster_split. The
//     tracker never claims cluster_merged, so neither does this.
//   - Reference. Named for what the position is: the body centre, the
//     cluster medoid or the centre of the visible box. Only the first is a
//     place on the body. A near-face-centre reference needs a declared offset
//     to the body centre, which the solid-body contract does not carry yet,
//     so it is refused rather than treated as the centre.
//   - Acquisition time. The measurement's own acquisition time, which the
//     solid body carries as LastObservedUnixNanos, is kept as the sample's
//     AcquisitionUnixNanos; LastObservedUnixNanos becomes the capture time on
//     an observed instant.
//
// Nothing here changes l5tracks behaviour; it only reads its types.
//
// A persisted estimate (a lidar_track_estimates row) says less again: pose,
// velocity and covariance, and nothing of the solid body held beside them.
// TrajectoriesFromEstimates maps what a row carries and claims nothing it
// does not; see PersistedEstimate. A persisted solid body (a
// lidar_track_solid_bodies row) carries the reading itself, and
// TrajectoriesFromSolidBodies maps it as a live one; see PersistedSolidBody.

import (
	"fmt"
	"math"
	"sort"
	"strings"

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

// SupportStateFromL5 maps the tracker's frame support. Whether the frame was
// observed is the coasted count's to say; the support token only explains an
// absence, and is mapped by token, never by number. Truncation keeps the
// frame observed: the detection is present, only its extent is not admissible
// as dimension evidence, which the extent belief already accounts for.
func SupportStateFromL5(s l5tracks.SupportState) SupportState {
	if !s.IsObserved() {
		switch explained, _ := ParseSupportState(s.Instant.String()); explained {
		case SupportOccludedInferred, SupportMissedUnknown, SupportOutOfFOV:
			return explained
		}
		return SupportCoasted
	}
	if s.Fragmented {
		return SupportClusterSplit
	}
	return SupportObserved
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
	case l5tracks.ReferenceVisibleOBBCentre:
		reference = ReferenceVisibleOBBCentre
	default:
		return TrajectorySample{}, fmt.Errorf("solid body reference %s has no declared offset to the body centre", body.Reference)
	}
	estimation, err := EstimationStateFromL5(body.Estimation)
	if err != nil {
		return TrajectorySample{}, err
	}

	// The solid body stamps the measurement's own acquisition time.
	acquired := body.LastObservedUnixNanos
	lastObserved := acquired
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
		AcquisitionUnixNanos:  acquired,
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

// SampleFromSolidBodyReading builds a sample from a solid-body reading: live
// from TrackedObject.SolidBody, or read back from lidar_track_solid_bodies.
// The reading carries its own dynamic state, so its covariance agrees with the
// estimate by construction rather than by two reads landing on one frame. It
// also names the faces its near-edge fix used, which is what lets an end face
// be claimed as observed.
//
// The shadow filter updates its covariance in float32 without
// re-symmetrising it, as the tracked filter does, so its off-diagonal pairs
// differ by round-off; they are averaged as for a persisted point estimate
// (see PersistedEstimate), and a reading asymmetric beyond round-off is
// refused.
func SampleFromSolidBodyReading(
	r l5tracks.SolidBodyReading,
	captureUnixNanos int64,
	bounds l5tracks.ConvergenceBounds,
) (TrajectorySample, error) {
	block := [4]float32{r.Covariance[0], r.Covariance[1], r.Covariance[4], r.Covariance[5]}
	if block != r.Estimate.PositionCovariance {
		return TrajectorySample{}, fmt.Errorf("covariance position block %v disagrees with the solid body's %v", block, r.Estimate.PositionCovariance)
	}
	p, err := symmetricCovariance(r.Covariance)
	if err != nil {
		return TrajectorySample{}, err
	}
	body := r.Estimate
	body.PositionCovariance = [4]float32{p[0], p[1], p[4], p[5]}
	s, err := SampleFromSolidBody(body, DynamicState{VX: r.VX, VY: r.VY, Covariance: p}, captureUnixNanos, bounds)
	if err != nil {
		return TrajectorySample{}, err
	}
	// Front and rear are named along the heading the fix used, so they mean
	// the leading and trailing faces only once the heading is resolved.
	if r.Measurement.Source == l5tracks.MeasurementNearEdgeCandidateV1 &&
		s.Support == SupportObserved && s.Heading.Resolved() {
		s.Faces = FaceVisibility{
			FrontObserved: r.Measurement.Faces.Has(l5tracks.FaceFront),
			RearObserved:  r.Measurement.Faces.Has(l5tracks.FaceRear),
		}
		if err := s.Validate(); err != nil {
			return TrajectorySample{}, fmt.Errorf("solid-body reading does not form a valid sample: %w", err)
		}
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

// PersistedEstimate is one lidar_track_estimates row as the adapter reads it,
// with the sensor named by the immutable observation the row links to.
//
// The tracking pipeline writes a row only for a confirmed track at a frame
// with an accepted association (Misses == 0 and a valid residual), and a
// refined stage writes exactly the online rows' population. What a row says,
// and so what its sample says:
//
//   - State model. The filter is the estimator id up to its first "+"; what
//     follows names a smoother over that filter, which revises the state and
//     keeps its layout (pipeline.RefinedEstimatorID). Only filters with a
//     known layout are read: cv_kf_v1 is cv_cartesian_v1.
//   - Stage. The row's own: online, fixed_lag or final. This is the one place
//     final is read rather than inferred.
//   - Reference. From the geometry that entered the filter at that frame:
//     an OBB centre is the centre of the visible box and a medoid, including
//     the OBB-centre model's medoid fallback, is a cluster medoid, as
//     l5tracks.SolidBodyFromTrack names them. Neither is a place on the
//     body, whatever the row's stage, so no endpoint is projected from a row.
//   - Acquisition time. The row's measurement time, when it recorded one.
//   - Support. Observed: the row exists because a measurement was
//     associated. A frame with no row is not invented; it is a gap between
//     samples, which the encounter method holds or counts as a record gap.
//   - Estimation. l5tracks.NextEstimationState on what the row proves: a
//     sustained association (the track is confirmed) at the row's speed,
//     against no persisted geometry. Established is unreachable, so the state
//     is geometry_converging at or above the heading-observability speed and
//     initialising below it.
//   - Heading, length and width. None: no orientation or dimension belief is
//     persisted, so no endpoint can be projected from a row.
//   - Class. Unknown: the classifier's label is not persisted.
//   - Covariance. The row's, made exactly symmetric. The online filter
//     updates P in float32 and does not re-symmetrise it, so its
//     off-diagonal pairs differ by round-off (5e-8 of the largest variance on
//     kirk0); each pair is averaged when it agrees within
//     persistedAsymmetryTolerance, and the row is refused otherwise.
type PersistedEstimate struct {
	TrackID           string
	SensorID          string
	FrameUnixNanos    int64
	EstimatorID       string
	ObsModelID        string
	ParamHash         string
	Stage             string
	MeasurementSource string
	// MeasurementUnixNanos is the acquisition time of the geometry that
	// entered the filter; zero when the row did not record it.
	MeasurementUnixNanos int64
	X, Y, VX, VY         float32
	Covariance           [16]float32
}

// persistedStateModels maps a filter estimator id to its state layout.
var persistedStateModels = map[string]string{"cv_kf_v1": StateModelCVCartesianV1}

// persistedReferences maps the geometry that entered the filter to the point
// the filtered pose refers to. Near-edge faces enter only a tracked filter
// that near_edge_track has re-referenced to the body centre, and a faceless
// frame there keeps the last source, so every such row is on the centre.
var persistedReferences = map[string]l5tracks.ReferencePoint{
	string(l5tracks.MeasurementOBBCentreV1):         l5tracks.ReferenceVisibleOBBCentre,
	string(l5tracks.MeasurementMedoidV0):            l5tracks.ReferenceClusterMedoid,
	string(l5tracks.MeasurementMedoidFallbackV1):    l5tracks.ReferenceClusterMedoid,
	string(l5tracks.MeasurementNearEdgeCandidateV1): l5tracks.ReferenceBodyCentre,
}

// PersistedStateModel is the dynamic state layout of an estimator's rows, or
// an error when the filter's layout is not known.
func PersistedStateModel(estimatorID string) (string, error) {
	filter, _, _ := strings.Cut(estimatorID, "+")
	if m, ok := persistedStateModels[filter]; ok {
		return m, nil
	}
	return "", fmt.Errorf("estimator %q: filter %q has no known state layout", estimatorID, filter)
}

// TrajectoriesFromEstimates builds one trajectory per track from one version
// of persisted estimates, sorted by track id, each track's samples in frame
// order. Every row must carry the same estimator, observation model,
// parameter hash and stage; a row that cannot be read as documented on
// PersistedEstimate is an error, never a guess.
func TrajectoriesFromEstimates(rows []PersistedEstimate, bounds l5tracks.ConvergenceBounds) ([]Trajectory, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	first := rows[0]
	identity := EstimateIdentity{EstimatorID: first.EstimatorID, ObsModelID: first.ObsModelID, ParamHash: first.ParamHash}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	if identity == FixtureEstimate() {
		return nil, fmt.Errorf("a persisted estimate cannot carry the analytic fixture estimator's identity")
	}
	stateModel, err := PersistedStateModel(first.EstimatorID)
	if err != nil {
		return nil, err
	}
	stage, err := ParseEstimateStage(first.Stage)
	if err != nil {
		return nil, err
	}

	byTrack := map[string][]PersistedEstimate{}
	for _, r := range rows {
		if r.EstimatorID != first.EstimatorID || r.ObsModelID != first.ObsModelID || r.ParamHash != first.ParamHash ||
			r.Stage != first.Stage {
			return nil, fmt.Errorf("estimate rows mix versions: %s/%s/%s/%s and %s/%s/%s/%s",
				first.EstimatorID, first.ObsModelID, first.ParamHash, first.Stage,
				r.EstimatorID, r.ObsModelID, r.ParamHash, r.Stage)
		}
		byTrack[r.TrackID] = append(byTrack[r.TrackID], r)
	}
	ids := make([]string, 0, len(byTrack))
	for id := range byTrack {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Trajectory, 0, len(ids))
	for _, id := range ids {
		track := byTrack[id]
		sort.Slice(track, func(i, j int) bool { return track[i].FrameUnixNanos < track[j].FrameUnixNanos })
		t := Trajectory{
			Passage:  Passage{TrackID: id, SensorID: track[0].SensorID, MotionClass: MotionUnknown},
			Estimate: identity,
		}
		estimation := l5tracks.EstimationInitialising
		for _, r := range track {
			if r.SensorID != t.Passage.SensorID {
				return nil, fmt.Errorf("track %s is observed by sensors %q and %q", id, t.Passage.SensorID, r.SensorID)
			}
			s, next, err := sampleFromPersisted(r, stateModel, stage, estimation, bounds)
			if err != nil {
				return nil, fmt.Errorf("track %s at %d: %w", id, r.FrameUnixNanos, err)
			}
			estimation = next
			t.Samples = append(t.Samples, s)
		}
		if err := t.Validate(); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func sampleFromPersisted(r PersistedEstimate, stateModel string, stage EstimateStage, current l5tracks.EstimationState,
	bounds l5tracks.ConvergenceBounds) (TrajectorySample, l5tracks.EstimationState, error) {
	reference, ok := persistedReferences[r.MeasurementSource]
	if !ok {
		return TrajectorySample{}, current, fmt.Errorf("measurement source %q names no reference point", r.MeasurementSource)
	}
	p, err := symmetricCovariance(r.Covariance)
	if err != nil {
		return TrajectorySample{}, current, err
	}
	body := l5tracks.SolidBodyEstimate{
		StateModel: stateModel, Reference: reference, X: r.X, Y: r.Y,
		PositionCovariance: [4]float32{p[0], p[1], p[4], p[5]},
		// A row exists only at an associated frame, so it is observed at
		// its own frame.
		LastObservedUnixNanos: r.FrameUnixNanos,
	}
	body.Estimation = l5tracks.NextEstimationState(current, body, l5tracks.EstimationEvidence{
		SustainedAssociation: true,
		SpeedMps:             float32(math.Hypot(float64(r.VX), float64(r.VY))),
	}, bounds)
	s, err := SampleFromSolidBody(body, DynamicState{VX: r.VX, VY: r.VY, Covariance: p}, r.FrameUnixNanos, bounds)
	if err != nil {
		return TrajectorySample{}, current, err
	}
	s.Stage = stage
	s.AcquisitionUnixNanos = r.MeasurementUnixNanos
	if err := s.Validate(); err != nil {
		return TrajectorySample{}, current, err
	}
	return s, body.Estimation, nil
}

// PersistedSolidBody is one lidar_track_solid_bodies row as the adapter reads
// it, with the sensor named by the immutable observation the row links to.
//
// Unlike a point-estimate row, it carries the body, so its sample says what
// SampleFromSolidBodyReading says of the reading: the reference the body
// named (the centre only after a near-edge fix, the medoid otherwise), the
// heading and extents with their provenance, the end faces the fix used, the
// support token and flags, and the measurement's acquisition time. What
// remains:
//
//   - Stage. The row's, which must be the reading's own: a row cannot
//     acquire a stage its reading does not state, and no stage turns a
//     medoid into a body centre.
//   - Class. Each row holds the classifier's decision at its frame; a
//     passage has one class, so a trajectory takes its latest row's belief,
//     the decision made with the most evidence, at its effective class.
//   - Gaps. A row is written only at an associated frame, so a missing frame
//     is a gap between samples, as for point estimates.
type PersistedSolidBody struct {
	TrackID        string
	SensorID       string
	FrameUnixNanos int64
	EstimatorID    string
	ObsModelID     string
	ParamHash      string
	Stage          string
	Reading        l5tracks.SolidBodyReading
}

// TrajectoriesFromSolidBodies builds one trajectory per track from one
// version of persisted solid bodies, sorted by track id, each track's samples
// in frame order. As for TrajectoriesFromEstimates, every row must carry the
// same version, and a row that cannot be read as documented on
// PersistedSolidBody is an error, never a guess.
func TrajectoriesFromSolidBodies(rows []PersistedSolidBody, bounds l5tracks.ConvergenceBounds) ([]Trajectory, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	first := rows[0]
	identity := EstimateIdentity{EstimatorID: first.EstimatorID, ObsModelID: first.ObsModelID, ParamHash: first.ParamHash}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	if identity == FixtureEstimate() {
		return nil, fmt.Errorf("a persisted solid body cannot carry the analytic fixture estimator's identity")
	}
	stage, err := ParseEstimateStage(first.Stage)
	if err != nil {
		return nil, err
	}

	byTrack := map[string][]PersistedSolidBody{}
	for _, r := range rows {
		if r.EstimatorID != first.EstimatorID || r.ObsModelID != first.ObsModelID || r.ParamHash != first.ParamHash ||
			r.Stage != first.Stage {
			return nil, fmt.Errorf("solid-body rows mix versions: %s/%s/%s/%s and %s/%s/%s/%s",
				first.EstimatorID, first.ObsModelID, first.ParamHash, first.Stage,
				r.EstimatorID, r.ObsModelID, r.ParamHash, r.Stage)
		}
		byTrack[r.TrackID] = append(byTrack[r.TrackID], r)
	}
	ids := make([]string, 0, len(byTrack))
	for id := range byTrack {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Trajectory, 0, len(ids))
	for _, id := range ids {
		track := byTrack[id]
		sort.Slice(track, func(i, j int) bool { return track[i].FrameUnixNanos < track[j].FrameUnixNanos })
		latest := track[len(track)-1]
		t := Trajectory{
			Passage:  PassageFromL5(id, "", latest.SensorID, latest.Reading.Estimate.Motion, ""),
			Estimate: identity,
		}
		for _, r := range track {
			if r.SensorID != t.Passage.SensorID {
				return nil, fmt.Errorf("track %s is observed by sensors %q and %q", id, t.Passage.SensorID, r.SensorID)
			}
			s, err := SampleFromSolidBodyReading(r.Reading, r.FrameUnixNanos, bounds)
			if err != nil {
				return nil, fmt.Errorf("track %s at %d: %w", id, r.FrameUnixNanos, err)
			}
			if s.Stage != stage {
				return nil, fmt.Errorf("track %s at %d: a %s reading is filed under stage %s", id, r.FrameUnixNanos, s.Stage, stage)
			}
			t.Samples = append(t.Samples, s)
		}
		if err := t.Validate(); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// persistedAsymmetryTolerance bounds how far apart a persisted covariance's
// off-diagonal pairs may be, relative to its largest diagonal term (at least
// one), and still be read as round-off: about eight float32 units in the
// last place. Beyond it the matrix is not a covariance with round-off.
const persistedAsymmetryTolerance = 1e-6

// symmetricCovariance averages each off-diagonal pair of a float32
// covariance that is symmetric to within round-off, and refuses one that is
// not.
func symmetricCovariance(p [16]float32) ([16]float32, error) {
	scale := 1.0
	for i := 0; i < 4; i++ {
		scale = math.Max(scale, math.Abs(float64(p[i*4+i])))
	}
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			a, b := float64(p[i*4+j]), float64(p[j*4+i])
			if !(math.Abs(a-b) <= persistedAsymmetryTolerance*scale) {
				return p, fmt.Errorf("covariance terms (%d,%d) %g and (%d,%d) %g differ by more than round-off", i, j, a, j, i, b)
			}
			m := float32((a + b) / 2)
			p[i*4+j], p[j*4+i] = m, m
		}
	}
	return p, nil
}
