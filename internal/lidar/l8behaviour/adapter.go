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
// velocity and covariance with the reference point and support token the
// writer stated, and nothing of the solid body held beside them.
// TrajectoriesFromEstimates maps what a row carries and claims nothing it
// does not; see PersistedEstimate. A persisted solid body (a
// lidar_track_solid_bodies row) carries the reading itself, and
// TrajectoriesFromSolidBodies maps it as a live one; see PersistedSolidBody.
// A persisted published box (a lidar_track_observations row's box, filed
// beside an online estimate) is read as the body by TrajectoriesFromBoxes,
// with the estimate supplying velocity and covariance; see PersistedBox.

import (
	"errors"
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
		return TrajectorySample{}, &InvalidSampleError{Subject: "solid body", Reason: err}
	}
	return s, nil
}

// InvalidSampleError is a solid body that was read, and whose beliefs
// contradict the trajectory contract: Reason is what TrajectorySample.Validate
// refused. It is told apart from a reading the adapter cannot interpret (an
// unknown reference, a covariance that disagrees with its estimate), which
// stays a plain error, because a reader may hold the first as a gap and must
// never guess at the second.
type InvalidSampleError struct {
	// Subject names what was read: a solid body, or a solid-body reading
	// once the faces its fix used are applied.
	Subject string
	Reason  error
}

func (e *InvalidSampleError) Error() string {
	return e.Subject + " does not form a valid sample: " + e.Reason.Error()
}

func (e *InvalidSampleError) Unwrap() error { return e.Reason }

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
			return TrajectorySample{}, &InvalidSampleError{Subject: "solid-body reading", Reason: err}
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
//   - Reference. The row's reference_point, as the tracker or smoother
//     stated it when the row was written; never inferred from the
//     measurement source, which names the geometry that entered the filter
//     and not the point the state refers to. Today's writers state the
//     centre of the visible box or the cluster medoid, as
//     l5tracks.TrackedObject.PositionReference names them; neither is a
//     place on the body, whatever the row's stage. A near-face or unknown
//     reference is refused, as for a solid body.
//   - Acquisition time. The row's measurement time, when it recorded one.
//   - Support. The row's support_instant, which must be observed: every
//     writer writes a row only at an associated frame, and a row records no
//     coasted count or last observed time, so an unobserved row is refused
//     rather than read with those guessed. A frame with no row is not
//     invented; it is a gap between samples, which the encounter method
//     holds or counts as a record gap.
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
	TrackID        string
	SensorID       string
	FrameUnixNanos int64
	EstimatorID    string
	ObsModelID     string
	ParamHash      string
	Stage          string
	// Reference and Support are the row's reference_point and
	// support_instant, read by the l5tracks parsers. There is deliberately no
	// measurement source here: nothing is inferred from it.
	Reference l5tracks.ReferencePoint
	Support   l5tracks.ObservationSupport
	// MeasurementUnixNanos is the acquisition time of the geometry that
	// entered the filter; zero when the row did not record it.
	MeasurementUnixNanos int64
	X, Y, VX, VY         float32
	Covariance           [16]float32
}

// persistedStateModels maps a filter estimator id to its state layout.
var persistedStateModels = map[string]string{"cv_kf_v1": StateModelCVCartesianV1}

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
	if !r.Support.IsObserved() {
		return TrajectorySample{}, current, fmt.Errorf("support %q: a point-estimate row records no last observed time, so only an observed row is read", r.Support)
	}
	p, err := symmetricCovariance(r.Covariance)
	if err != nil {
		return TrajectorySample{}, current, err
	}
	body := l5tracks.SolidBodyEstimate{
		StateModel: stateModel, Reference: r.Reference, X: r.X, Y: r.Y,
		PositionCovariance: [4]float32{p[0], p[1], p[4], p[5]},
		// An observed row is observed at its own frame.
		LastObservedUnixNanos: r.FrameUnixNanos,
		Support:               l5tracks.SupportState{Instant: r.Support},
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
//     medoid into a body centre. A smoothed reading is either refined stage,
//     so a final row of one is final; a live reading is online only.
//   - Class. Each row holds the classifier's decision at its frame; a
//     passage has one class, so a trajectory takes its latest row's belief,
//     the decision made with the most evidence, at its effective class.
//   - Gaps. A row is written only at an associated frame, so a missing frame
//     is a gap between samples, as for point estimates. A row whose reading
//     contradicts the trajectory contract (an InvalidSampleError, such as an
//     established state without converged extents, a resolved heading and a
//     physical reference) is a gap too, as a frame with no published box is
//     for PersistedBox: the writer's claim is not read, and no weaker one is
//     guessed in its place. The reader counts such rows by reason.
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

// SolidBodyGaps counts the persisted solid bodies that
// TrajectoriesFromSolidBodiesWithGaps held as gaps rather than samples, by
// the contract condition each refused (InvalidSampleError's Reason).
type SolidBodyGaps struct {
	Rows     int
	ByReason map[string]int
}

func (g *SolidBodyGaps) add(reason string) {
	if g.ByReason == nil {
		g.ByReason = map[string]int{}
	}
	g.Rows++
	g.ByReason[reason]++
}

// TrajectoriesFromSolidBodies builds one trajectory per track from one
// version of persisted solid bodies, sorted by track id, each track's samples
// in frame order. As for TrajectoriesFromEstimates, every row must carry the
// same version, and a row that cannot be read as documented on
// PersistedSolidBody is an error, never a guess. So is a row whose reading
// is not a valid sample; TrajectoriesFromSolidBodiesWithGaps holds those as
// gaps instead.
func TrajectoriesFromSolidBodies(rows []PersistedSolidBody, bounds l5tracks.ConvergenceBounds) ([]Trajectory, error) {
	out, _, err := trajectoriesFromSolidBodies(rows, bounds, false)
	return out, err
}

// TrajectoriesFromSolidBodiesWithGaps is TrajectoriesFromSolidBodies with a
// row whose reading is not a valid sample (an InvalidSampleError) held as a
// gap and counted by reason, as PersistedSolidBody describes. A track left
// with no valid row has no trajectory, and a version with no valid row at
// all is refused.
func TrajectoriesFromSolidBodiesWithGaps(rows []PersistedSolidBody, bounds l5tracks.ConvergenceBounds) ([]Trajectory, SolidBodyGaps, error) {
	return trajectoriesFromSolidBodies(rows, bounds, true)
}

func trajectoriesFromSolidBodies(rows []PersistedSolidBody, bounds l5tracks.ConvergenceBounds, holdInvalid bool) ([]Trajectory, SolidBodyGaps, error) {
	var gaps SolidBodyGaps
	if len(rows) == 0 {
		return nil, gaps, nil
	}
	first := rows[0]
	identity := EstimateIdentity{EstimatorID: first.EstimatorID, ObsModelID: first.ObsModelID, ParamHash: first.ParamHash}
	if err := identity.Validate(); err != nil {
		return nil, gaps, err
	}
	if identity == FixtureEstimate() {
		return nil, gaps, fmt.Errorf("a persisted solid body cannot carry the analytic fixture estimator's identity")
	}
	stage, err := ParseEstimateStage(first.Stage)
	if err != nil {
		return nil, gaps, err
	}

	byTrack := map[string][]PersistedSolidBody{}
	for _, r := range rows {
		if r.EstimatorID != first.EstimatorID || r.ObsModelID != first.ObsModelID || r.ParamHash != first.ParamHash ||
			r.Stage != first.Stage {
			return nil, gaps, fmt.Errorf("solid-body rows mix versions: %s/%s/%s/%s and %s/%s/%s/%s",
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
				return nil, gaps, fmt.Errorf("track %s is observed by sensors %q and %q", id, t.Passage.SensorID, r.SensorID)
			}
			s, err := SampleFromSolidBodyReading(r.Reading, r.FrameUnixNanos, bounds)
			var invalid *InvalidSampleError
			if holdInvalid && errors.As(err, &invalid) {
				gaps.add(invalid.Reason.Error())
				continue
			}
			if err != nil {
				return nil, gaps, fmt.Errorf("track %s at %d: %w", id, r.FrameUnixNanos, err)
			}
			if s.Stage == StageFixedLag && stage == StageFinal {
				// A smoothed reading cannot say which refined stage it is;
				// a persisted final row can, and is the one place final is
				// read (see the Stage note at the top of this file).
				s.Stage = StageFinal
			}
			if s.Stage != stage {
				return nil, gaps, fmt.Errorf("track %s at %d: a %s reading is filed under stage %s", id, r.FrameUnixNanos, s.Stage, stage)
			}
			t.Samples = append(t.Samples, s)
		}
		if len(t.Samples) == 0 {
			continue
		}
		if err := t.Validate(); err != nil {
			return nil, gaps, err
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, gaps, fmt.Errorf("none of the version's %d solid bodies forms a valid sample (%s)", len(rows), gaps.describe())
	}
	return out, gaps, nil
}

// Reasons lists the gaps' reasons, most frequent first, ties by name.
func (g SolidBodyGaps) Reasons() []string {
	reasons := make([]string, 0, len(g.ByReason))
	for r := range g.ByReason {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if g.ByReason[reasons[i]] != g.ByReason[reasons[j]] {
			return g.ByReason[reasons[i]] > g.ByReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	return reasons
}

func (g SolidBodyGaps) describe() string {
	parts := make([]string, 0, len(g.ByReason))
	for _, r := range g.Reasons() {
		parts = append(parts, fmt.Sprintf("%s: %d", r, g.ByReason[r]))
	}
	return strings.Join(parts, "; ")
}

// PublishedBoxSource names the tracker's published box as a trajectory
// source: the rectangle the box model draws for a track (l5tracks'
// BoxModelConfig), recorded with every observation since migration 000062.
const PublishedBoxSource = "published_box"

// BoxObservationModelID is the observation model a box trajectory carries in
// its estimate identity: the published box over the filter's own observation
// model. It keeps a box run's stored encounters apart from a run over the
// same estimates read as points, which an identical identity would collide
// with.
func BoxObservationModelID(filterObsModelID string) string {
	return PublishedBoxSource + "/" + filterObsModelID
}

// boxCourseAgreementRad is how far the box's axis may lie from the course and
// still be read as the length along it. Beyond it, and short of the same
// distance from the reverse course, the box puts its length across the
// direction of travel, and no front can be named from it.
const boxCourseAgreementRad = math.Pi / 4

// PersistedBox is one published box as an evidence database files it: the box
// columns of a lidar_track_observations row (migration 000062), the
// lidar_track_estimates row written for the same track at the same frame, the
// track's label from lidar_tracks and the sensor of the observation the
// estimate names. The box is the body; the estimate supplies what the box
// does not record. What a sample built from it says:
//
//   - Reference. body_centre, at the box's centre: the box model places it so
//     the box holds every return of the frame, with the unseen remainder on
//     the side away from the sensor. That is a believed body, not a measured
//     one, and the endpoints say so (see Faces).
//   - Heading. The box's, as an accumulated belief: the box model's heading
//     filter, observed by the rectangle fit's axis and, at speed, the course.
//     A row does not record whether the box model had labelled the length
//     and the front, so the front is named as the box model names it, from
//     the estimate's velocity. At or above the heading-observability speed
//     (ConvergenceBounds.MinSpeedForHeadingMps), a box axis within 45
//     degrees of the course is the heading and one within 45 degrees of the
//     reverse course is turned by half a turn. Below it, where the box model
//     keeps its label and cannot spin, the track's last named front is held
//     the same way. A box across the course, or across its held front, or
//     slow with no front named yet, keeps its axis with the front unnamed
//     (ambiguous-mode weight 0.5), and no endpoint is projected from it.
//   - Length and width. The box's, as accumulated beliefs, so an endpoint is
//     temporally inferred, never prior-dominated. A row cannot say when the
//     box model drew a labelled vehicle-width box to its vehicle length floor
//     rather than to returns, so a floored length is read as evidence too.
//   - Faces. None: the box records no face, so every endpoint is temporally
//     inferred, even on an observed instant.
//   - Uncertainty. The box records no spread of its own. Heading variance and
//     extent sigmas are zero, and the position covariance is the estimate's,
//     stated for the filter's reference point rather than the box centre. An
//     endpoint's sigma is therefore the filter's position sigma alone, a
//     lower bound.
//   - Admissible frames. The box's own corroboration window is not recorded;
//     each extent counts the track's boxes read so far, this one included, so
//     an extent converges on its MinAdmissibleFrames-th box.
//   - Estimation. l5tracks.NextEstimationState on the beliefs above, with a
//     sustained association (the track is confirmed) at the estimate's
//     speed, as for a point estimate: established once both extents have
//     converged and the front is named, geometry_converging before that at
//     speed, initialising below it. An established track that is slow keeps
//     its state there, as the tracker's does, but only while its front is
//     held; a slow box that turns across its held front is
//     temporarily_degraded, not established without a front.
//   - Velocity and covariance. The estimate's, the covariance made exactly
//     symmetric as for PersistedEstimate.
//   - Support and acquisition time. The estimate's support_instant, which
//     must be observed, as for a point estimate; the estimate's measurement
//     time.
//   - Stage. Online only: the box is the online tracker's, and no refined
//     stage revises it.
//   - Class. The track's label, the classifier's latest for the track,
//     mapped by l5tracks.MotionClassForLabel with its confidence as the
//     posterior, as the tracker reads it for the solid body. No label is
//     unknown, with no confidence.
//   - Gaps. A frame with no box filed (the box model off or not yet seeded,
//     or no observation row: see sqlite.BoxWithEstimate.Filed) is not a
//     sample; like a frame with no row, it is a gap the encounter method
//     holds or counts as a record gap. The caller counts such frames.
type PersistedBox struct {
	TrackID        string
	SensorID       string
	FrameUnixNanos int64
	// EstimatorID, ObsModelID, ParamHash and Stage are the estimate row's.
	EstimatorID string
	ObsModelID  string
	ParamHash   string
	Stage       string
	Support     l5tracks.ObservationSupport
	// MeasurementUnixNanos is the estimate's measurement time; zero when the
	// row did not record it.
	MeasurementUnixNanos int64
	VX, VY               float32
	Covariance           [16]float32
	// HasBox is false when the observation records no published box.
	HasBox                                         bool
	BoxX, BoxY, BoxHeadingRad, BoxLength, BoxWidth float32
	// ClassLabel is the track's label, empty when it has none, and
	// ClassConfidence its confidence, nil without one.
	ClassLabel      string
	ClassConfidence *float32
}

// TrajectoriesFromBoxes builds one trajectory per track with at least one
// published box from one online version of persisted estimates and the boxes
// filed beside them, sorted by track id, each track's samples in frame order.
// As for TrajectoriesFromEstimates, every row must carry the same version,
// and a row that cannot be read as documented on PersistedBox is an error,
// never a guess.
func TrajectoriesFromBoxes(rows []PersistedBox, bounds l5tracks.ConvergenceBounds) ([]Trajectory, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	first := rows[0]
	filter := EstimateIdentity{EstimatorID: first.EstimatorID, ObsModelID: first.ObsModelID, ParamHash: first.ParamHash}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	if filter == FixtureEstimate() {
		return nil, fmt.Errorf("a persisted box cannot carry the analytic fixture estimator's identity")
	}
	stateModel, err := PersistedStateModel(first.EstimatorID)
	if err != nil {
		return nil, err
	}
	if stage, err := ParseEstimateStage(first.Stage); err != nil {
		return nil, err
	} else if stage != StageOnline {
		return nil, fmt.Errorf("a published box is the online tracker's and is filed beside online estimates, not %s ones", stage)
	}
	identity := filter
	identity.ObsModelID = BoxObservationModelID(filter.ObsModelID)

	byTrack := map[string][]PersistedBox{}
	for _, r := range rows {
		if r.EstimatorID != first.EstimatorID || r.ObsModelID != first.ObsModelID || r.ParamHash != first.ParamHash ||
			r.Stage != first.Stage {
			return nil, fmt.Errorf("box rows mix versions: %s/%s/%s/%s and %s/%s/%s/%s",
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
		passage, err := boxPassage(id, track)
		if err != nil {
			return nil, err
		}
		t := Trajectory{Passage: passage, Estimate: identity}
		estimation := l5tracks.EstimationInitialising
		boxes := 0
		var front *float32
		for _, r := range track {
			if r.SensorID != t.Passage.SensorID {
				return nil, fmt.Errorf("track %s is observed by sensors %q and %q", id, t.Passage.SensorID, r.SensorID)
			}
			if !r.HasBox {
				continue
			}
			boxes++
			s, next, err := sampleFromBox(r, stateModel, boxes, front, estimation, bounds)
			if err != nil {
				return nil, fmt.Errorf("track %s at %d: %w", id, r.FrameUnixNanos, err)
			}
			if s.Heading.Resolved() {
				psi := float32(s.Heading.Rad)
				front = &psi
			}
			estimation = next
			t.Samples = append(t.Samples, s)
		}
		if boxes == 0 {
			continue
		}
		if err := t.Validate(); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// boxPassage is a track's identity and class. Every row joins the same
// lidar_tracks row, so a track whose rows disagree on the label was not read
// from one database.
func boxPassage(id string, track []PersistedBox) (Passage, error) {
	first := track[0]
	for _, r := range track[1:] {
		if r.ClassLabel != first.ClassLabel || (r.ClassConfidence == nil) != (first.ClassConfidence == nil) ||
			(r.ClassConfidence != nil && *r.ClassConfidence != *first.ClassConfidence) {
			return Passage{}, fmt.Errorf("track %s carries two labels", id)
		}
	}
	if first.ClassLabel == "" {
		return Passage{TrackID: id, SensorID: first.SensorID, MotionClass: MotionUnknown}, nil
	}
	belief := l5tracks.MotionClassBelief{Class: l5tracks.MotionClassForLabel(first.ClassLabel)}
	if first.ClassConfidence != nil {
		belief.Posterior = *first.ClassConfidence
	}
	p := PassageFromL5(id, "", first.SensorID, belief, first.ClassLabel)
	if first.ClassConfidence == nil {
		p.ClassConfidence = nil
	}
	return p, nil
}

// boxHeading is the box's orientation belief, with the front named from the
// course, or below the heading-observability speed held from the track's last
// named front, as PersistedBox describes.
func boxHeading(r PersistedBox, front *float32, bounds l5tracks.ConvergenceBounds) l5tracks.OrientationBelief {
	o := l5tracks.OrientationBelief{
		PsiRad: r.BoxHeadingRad, AmbiguousModeWeight: 0.5, Provenance: l5tracks.ProvenanceAccumulated,
	}
	vx, vy := float64(r.VX), float64(r.VY)
	var reference float64
	switch {
	case math.Hypot(vx, vy) >= float64(bounds.MinSpeedForHeadingMps):
		reference = math.Atan2(vy, vx)
	case front != nil:
		reference = float64(*front)
	default:
		return o
	}
	off := math.Abs(math.Remainder(float64(r.BoxHeadingRad)-reference, 2*math.Pi))
	switch {
	case off <= boxCourseAgreementRad:
		o.AmbiguousModeWeight = 0
	case off >= math.Pi-boxCourseAgreementRad:
		o.PsiRad = float32(math.Remainder(float64(r.BoxHeadingRad)+math.Pi, 2*math.Pi))
		o.AmbiguousModeWeight = 0
	}
	return o
}

// sampleFromBox builds the sample of one box, the n-th of its track, given
// the track's last named front (nil before one), and the estimation state it
// leaves.
func sampleFromBox(r PersistedBox, stateModel string, n int, front *float32, current l5tracks.EstimationState,
	bounds l5tracks.ConvergenceBounds) (TrajectorySample, l5tracks.EstimationState, error) {
	if !r.Support.IsObserved() {
		return TrajectorySample{}, current, fmt.Errorf("support %q: a box is filed only at an associated frame, so only an observed row is read", r.Support)
	}
	if !(r.BoxLength > 0 && r.BoxWidth > 0) {
		return TrajectorySample{}, current, fmt.Errorf("published box %g by %g m is not a body", r.BoxLength, r.BoxWidth)
	}
	p, err := symmetricCovariance(r.Covariance)
	if err != nil {
		return TrajectorySample{}, current, err
	}
	extent := func(m float32) l5tracks.DimensionBelief {
		return l5tracks.DimensionBelief{Metres: m, AdmissibleFrames: n, Provenance: l5tracks.ProvenanceAccumulated}
	}
	body := l5tracks.SolidBodyEstimate{
		StateModel: stateModel, Reference: l5tracks.ReferenceBodyCentre, X: r.BoxX, Y: r.BoxY,
		PositionCovariance: [4]float32{p[0], p[1], p[4], p[5]},
		Orientation:        boxHeading(r, front, bounds),
		Length:             extent(r.BoxLength), Width: extent(r.BoxWidth),
		// An observed row is observed at its own frame.
		LastObservedUnixNanos: r.FrameUnixNanos,
		Support:               l5tracks.SupportState{Instant: r.Support},
	}
	body.Estimation = l5tracks.NextEstimationState(current, body, l5tracks.EstimationEvidence{
		SustainedAssociation: true,
		SpeedMps:             float32(math.Hypot(float64(r.VX), float64(r.VY))),
	}, bounds)
	s, err := SampleFromSolidBody(body, DynamicState{VX: r.VX, VY: r.VY, Covariance: p}, r.FrameUnixNanos, bounds)
	if err != nil {
		return TrajectorySample{}, current, err
	}
	s.AcquisitionUnixNanos = r.MeasurementUnixNanos
	if err := s.Validate(); err != nil {
		return TrajectorySample{}, current, err
	}
	return s, body.Estimation, nil
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
