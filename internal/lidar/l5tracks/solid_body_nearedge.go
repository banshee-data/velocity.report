package l5tracks

// Populating the solid-body estimate from the near-edge measurement model, per
// Sections 5.3 to 5.6 and 9 of docs/plans/lidar-state-estimation-plan.md.
//
// This is a shadow estimator, and deliberately so. Gate G-GEO-1 asks whether
// the near-edge measurement fixes the medoid's lateral bias with the motion
// estimator held fixed (Section 5.3, Option A). So each track carries a second
// four-state constant-velocity filter with exactly the tracked filter's
// transition, process noise, clamps and caps, fed by the near-edge measurement
// instead of the medoid. It never feeds back: association, the tracked state
// and every recorded output are the same with it on or off, which is what makes
// its result attributable to the observation model alone.
//
// It is opt-in through TrackerConfig.SolidBody and off by default. With it off
// nothing here runs, and a track's solid body stays at its zero value.
//
// What is observed, and what is not:
//
//   - Anchor. The body centre, reconstructed from each visible face and the
//     believed half-extent behind it. One face constrains its own normal only;
//     the filter is updated once per face along that normal, so a rank-one
//     frame moves nothing across it (Section 9.1). A face whose half-extent
//     is only the class prior is excluded, not downweighted (Section 8.1).
//   - Extents. A face never measures the far side, so a frame's span along a
//     body axis is admitted only as lower-bound evidence, and only on an axis
//     whose near face was found (Section 9.2.1, "one face only"). The span is
//     the smallest within the orientation bound of the believed axis, so a
//     lagging heading cannot inflate it, and none is admitted while a moving
//     body's axis is further than that bound from its course.
//   - Heading. The tracked heading decision, with its own variance and
//     direction ambiguity; the near-edge model needs an axis, not a direction.
//   - Initialisation. The first HitsToConfirm observations update the solid
//     body from the medoid and keep it referenced to the medoid, per Phase 2's
//     mitigation for the heading/face circularity. The first near-edge fix
//     re-references it to the body centre and widens its position covariance
//     by the medoid's known bias before applying the fix.
//   - Lapse. A body-centre body that goes MaxMisses consecutive frames
//     without a usable face is re-seeded at the medoid and referenced to it
//     again, rather than coasting away from evidence that is still arriving.
//     On kirk0 a sparse object at 55 m went three seconds faceless while
//     observed every frame, drifted six metres and then snapped back.
//
// Named limitations. The position covariance does not carry the half-extent's
// uncertainty: an error in the believed width moves the anchor one-for-one and
// consistently from frame to frame, which is a bias the filter cannot see.
// ProjectSurface adds the dimension's own variance when it bounds a bumper, but
// the correlation between anchor and dimension is not represented; carrying it
// needs the dimension in the state or a consider-parameter treatment. And the
// heading that selects a face is the tracked one, which lags a real turn by its
// smoothing: a heading error tilts the face normal and biases the plane, so the
// heading is the first thing to stratify by when the anchor disagrees with
// truth (the plan's invalidating condition b).

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// SolidBodyOptions opts a tracker into populating a solid-body estimate per
// track from the near-edge measurement model.
//
// It is a Go-level option and deliberately not a tuning key: the tuning
// fingerprint hashes the whole resolved config, so a key would make the
// committed perf baselines refuse to compare before anyone had measured
// whether the estimate helps. The offline replay reaches it by name.
type SolidBodyOptions struct {
	// Enabled switches the solid-body estimator on. Default false.
	Enabled bool
	// SensorX and SensorY are the calibrated sensor origin in the tracker's
	// own frame. The near-edge model cannot tell the near face from the far
	// one without it. The pipeline tracks in the sensor frame today
	// (l4perception.TransformToWorld with no pose), where the origin is
	// (0, 0); a caller that tracks in a posed site frame must set it.
	SensorX, SensorY float32
}

// GroundSurfaceClusterMinZ names how GroundZ is resolved here: the associated
// cluster's lowest return, which is what the OBB's CenterZ records. It is not
// a road-surface model, and the name says so.
const GroundSurfaceClusterMinZ = "cluster_min_z"

// VisibleFaces is the set of body faces a frame's measurement used.
type VisibleFaces uint8

// faceBit is the set containing one face.
func faceBit(f BodyFace) VisibleFaces { return 1 << f }

// Has reports whether the set contains a face.
func (v VisibleFaces) Has(f BodyFace) bool { return v&faceBit(f) != 0 }

// allBodyFaces is the fixed order faces are named in.
var allBodyFaces = [...]BodyFace{FaceFront, FaceRear, FaceLeft, FaceRight}

// String names the faces in fixed order, comma-separated, for diagnostics and
// persisted rows. The empty set is the empty string.
func (v VisibleFaces) String() string {
	names := make([]string, 0, len(allBodyFaces))
	for _, f := range allBodyFaces {
		if v.Has(f) {
			names = append(names, f.String())
		}
	}
	return strings.Join(names, ",")
}

// ParseVisibleFaces reads a set written by String. An unknown face name is an
// error, never ignored.
func ParseVisibleFaces(s string) (VisibleFaces, error) {
	var v VisibleFaces
	if s == "" {
		return v, nil
	}
	for _, name := range strings.Split(s, ",") {
		found := false
		for _, f := range allBodyFaces {
			if f.String() == name {
				v |= faceBit(f)
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("unknown body face %q", name)
		}
	}
	return v, nil
}

// SolidBodyMeasurement records what one instant's update of the solid body
// consisted of, so a consumer can stratify by it rather than infer it.
type SolidBodyMeasurement struct {
	// Source is the geometry that updated the solid body at this instant:
	// MeasurementNearEdgeCandidateV1, MeasurementMedoidV0 while the body is
	// still referenced to its seed, or empty when nothing updated it.
	Source MeasurementSource
	// Rank is the number of independent directions the update constrained:
	// the number of faces for a near-edge fix, two for a medoid, zero for
	// none.
	Rank int
	// Faces are the faces a near-edge fix used.
	Faces VisibleFaces
	// InferredExtent is true when a face the model found was left out because
	// its half-extent was only the class prior, which Section 8.1 excludes
	// rather than downweights.
	InferredExtent bool
	// AspectRad is the angle between the body axis and the sensor's line of
	// sight to the body, folded into [0, pi/2]: zero is end-on, pi/2
	// broadside. It is meaningful only when AspectKnown.
	AspectRad   float32
	AspectKnown bool
	// NIS is the normalised innovation squared of the update, with Rank
	// degrees of freedom. The faces are applied sequentially with
	// independent noise, so the sum of the per-face terms is the joint NIS.
	NIS float32
	// FallbackReason says why no near-edge fix was made, when none was.
	FallbackReason string
}

// SolidBodyReading is a track's solid body at its latest update: the estimate,
// and the dynamic state it was read with. Covariance is the shadow filter's
// full 4x4 over [x, y, vx, vy], row-major; its position block is the
// estimate's PositionCovariance, so the two cannot describe different frames.
type SolidBodyReading struct {
	Estimate    SolidBodyEstimate
	VX, VY      float32
	Covariance  [16]float32
	Measurement SolidBodyMeasurement
}

// solidBodyTrack is one track's solid-body estimator state. It is held by
// value on TrackedObject so that a snapshot copy is an independent copy.
type solidBodyTrack struct {
	seeded bool
	// state is [x, y, vx, vy] and p its covariance, row-major.
	state [4]float32
	p     [16]float32

	reference   ReferencePoint
	orientation OrientationBelief
	estimation  EstimationState

	// lengthBelief and widthBelief are lower-bound extent evidence admitted
	// from near-edge frames, with the same corroborated-maximum semantics as
	// the tracked extent beliefs.
	lengthBelief, widthBelief extentBelief

	lastObservedNanos int64
	support           SupportState

	// reading is assembled when the state changes, so the estimate, its
	// lifecycle decision and its class prior all describe the same instant.
	reading SolidBodyReading
}

// SolidBody returns the track's solid-body estimate at its latest update, and
// false when the tracker is not populating one.
func (t *TrackedObject) SolidBody() (SolidBodyReading, bool) {
	if !t.solidBody.seeded {
		return SolidBodyReading{}, false
	}
	return t.solidBody.reading, true
}

// solidBodyClass reads the classifier's decision as a motion-class belief.
// The label's confidence is the posterior; there is no runner-up, so a split
// posterior cannot be represented here and the belief is taken as stated.
func solidBodyClass(t *TrackedObject) MotionClassBelief {
	return MotionClassBelief{Class: MotionClassForLabel(t.ObjectClass), Posterior: t.ObjectConfidence}
}

// seedSolidBody starts a track's solid body from its first cluster, per
// SeedSolidBody: at the medoid, with a covariance stating the medoid's bias.
func (t *Tracker) seedSolidBody(track *TrackedObject, cluster WorldCluster) {
	class := solidBodyClass(track)
	seed := SeedSolidBody(class, cluster.CentroidX, cluster.CentroidY, track.LastMeasurementUnixNanos)
	sb := &track.solidBody
	*sb = solidBodyTrack{
		seeded:    true,
		state:     [4]float32{seed.X, seed.Y, 0, 0},
		reference: seed.Reference,
		// Velocity starts where the tracked filter's does: zero, with unit
		// variance. Only position has a known seed bias to state.
		p: [16]float32{
			seed.PositionCovariance[0], seed.PositionCovariance[1], 0, 0,
			seed.PositionCovariance[2], seed.PositionCovariance[3], 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		estimation:        EstimationInitialising,
		lastObservedNanos: track.LastMeasurementUnixNanos,
		support:           SupportState{PointCount: cluster.PointsCount, Instant: SupportObserved},
	}
	sb.orientation = orientationFromTrack(track, OrientationBelief{})
	t.publishSolidBody(track, class, SolidBodyMeasurement{
		Source:         MeasurementMedoidV0,
		Rank:           2,
		FallbackReason: "initialisation_window",
	})
}

// predictSolidBody carries the solid body across dt exactly as predictSpan
// carries the tracked state: clamped to MaxPredictDt, or sub-stepped across a
// capture gap under CaptureGapPrediction.
func (t *Tracker) predictSolidBody(track *TrackedObject, dt float32) {
	sb := &track.solidBody
	if !sb.seeded {
		return
	}
	step := t.Config.MaxPredictDt
	if !t.Config.CaptureGapPrediction || dt <= step || step <= 0 {
		t.predictSolidBodyStep(track, dt)
		return
	}
	remaining := dt
	for n := 0; remaining > 0 && sb.seeded && n < maxGapPredictionSteps; n++ {
		s := step
		if remaining < s {
			s = remaining
		}
		t.predictSolidBodyStep(track, s)
		remaining -= s
	}
}

// predictSolidBodyStep is one constant-velocity prediction with the tracked
// filter's own clamp, process noise, covariance cap and speed limit.
func (t *Tracker) predictSolidBodyStep(track *TrackedObject, dt float32) {
	sb := &track.solidBody
	if dt > t.Config.MaxPredictDt {
		dt = t.Config.MaxPredictDt
	}
	sb.state[0] += sb.state[2] * dt
	sb.state[1] += sb.state[3] * dt
	sb.p = propagateConstantVelocity(sb.p, dt)
	addProcessNoise(&sb.p, t.Config.ProcessNoisePos, t.Config.ProcessNoiseVel, dt, t.Config.CoupledProcessNoise)
	for i := 0; i < 4; i++ {
		if sb.p[i*4+i] > t.Config.MaxCovarianceDiag {
			sb.p[i*4+i] = t.Config.MaxCovarianceDiag
		}
	}
	t.finishSolidBodyState(track, "predict")
}

// propagateConstantVelocity is F P Fᵀ for the constant-velocity transition.
func propagateConstantVelocity(p [16]float32, dt float32) [16]float32 {
	var fp [16]float32
	for j := 0; j < 4; j++ {
		fp[0*4+j] = p[0*4+j] + dt*p[2*4+j]
		fp[1*4+j] = p[1*4+j] + dt*p[3*4+j]
		fp[2*4+j] = p[2*4+j]
		fp[3*4+j] = p[3*4+j]
	}
	var out [16]float32
	for i := 0; i < 4; i++ {
		out[i*4+0] = fp[i*4+0] + dt*fp[i*4+2]
		out[i*4+1] = fp[i*4+1] + dt*fp[i*4+3]
		out[i*4+2] = fp[i*4+2]
		out[i*4+3] = fp[i*4+3]
	}
	return out
}

// finishSolidBodyState applies the tracked filter's speed clamp and refuses a
// non-finite state. A non-finite solid body is dropped, visibly, rather than
// reset to the origin: the tracked state is unaffected, and a reset would
// persist a confident-looking position with no basis.
func (t *Tracker) finishSolidBodyState(track *TrackedObject, step string) {
	sb := &track.solidBody
	for _, v := range sb.state {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			opsf("Solid body %s produced non-finite state: track_id=%s dropping solid body", step, track.TrackID)
			*sb = solidBodyTrack{}
			return
		}
	}
	for i := 0; i < 4; i++ {
		v := float64(sb.p[i*4+i])
		if math.IsNaN(v) || math.IsInf(v, 0) {
			opsf("Solid body %s produced non-finite covariance: track_id=%s dropping solid body", step, track.TrackID)
			*sb = solidBodyTrack{}
			return
		}
	}
	speed := float32(math.Hypot(float64(sb.state[2]), float64(sb.state[3])))
	if speed > t.Config.MaxReasonableSpeedMps && speed > 0 {
		scale := t.Config.MaxReasonableSpeedMps / speed
		sb.state[2] *= scale
		sb.state[3] *= scale
	}
}

// updateSolidBody updates an associated track's solid body from its cluster.
// It runs after the tracked update, so it reads this frame's heading decision.
func (t *Tracker) updateSolidBody(track *TrackedObject, cluster WorldCluster) {
	if track.TrackState == TrackDeleted {
		return
	}
	sb := &track.solidBody
	if !sb.seeded {
		// Switched on after this track was born: start it here.
		t.seedSolidBody(track, cluster)
		return
	}
	class := solidBodyClass(track)
	effective, _ := class.Effective()
	prior := dimensionPriorFor(effective)
	sb.orientation = orientationFromTrack(track, sb.orientation)

	var m SolidBodyMeasurement
	var edges EdgeMeasurementSet
	measured := false

	// Phase 2's mitigation: until HitsToConfirm observations have passed, the
	// heading that selects a face is itself unconverged, so the medoid is used
	// and the body stays referenced to it.
	switch {
	case track.ObservationCount <= t.Config.HitsToConfirm:
		m.FallbackReason = "initialisation_window"
	case sb.orientation.Provenance == ProvenanceNone:
		m.FallbackReason = "missing_heading"
	default:
		length := dimensionFromBelief(sb.lengthBelief, prior.lengthMetres, prior.sigmaMetres)
		width := dimensionFromBelief(sb.widthBelief, prior.widthMetres, prior.sigmaMetres)
		edges = MeasureNearEdge(NearEdgeInput{
			Cluster:          cluster,
			Points:           cluster.RetainedPoints,
			SensorX:          t.Config.SolidBody.SensorX,
			SensorY:          t.Config.SolidBody.SensorY,
			HeadingRad:       sb.orientation.PsiRad,
			HalfLength:       length.Metres / 2,
			HalfWidth:        width.Metres / 2,
			LengthProvenance: length.Provenance,
			WidthProvenance:  width.Provenance,
			PredictedX:       sb.state[0],
			PredictedY:       sb.state[1],
		})
		if edges.Rank == 0 {
			m.FallbackReason = edges.FallbackReason
			break
		}
		// Section 8.1: a face whose half-extent is only the class prior
		// cannot have its dimension-derived bias corrected, so it is excluded
		// rather than downweighted. It still counts as a seen face for the
		// extent evidence below, which is how the prior stops being one.
		usable := evidenceBackedEdges(edges.Edges)
		if len(usable) == 0 {
			m.FallbackReason = "class_prior_extent"
			m.InferredExtent = true
			break
		}
		startP := sb.p
		if sb.reference != ReferenceBodyCentre {
			// The state described a point on the sensor-facing surface. From
			// this fix on it describes the body centre, and before the fix its
			// position is uncertain by as much as the medoid was biased.
			bias := (prior.widthMetres / 2) * (prior.widthMetres / 2)
			startP[0*4+0] += bias
			startP[1*4+1] += bias
		}
		state, p, nis, faces, ok := t.applyEdgeMeasurements(sb.state, startP, usable)
		if !ok {
			m.FallbackReason = "singular_innovation"
			break
		}
		sb.state, sb.p, sb.reference = state, p, ReferenceBodyCentre
		m.Source = MeasurementNearEdgeCandidateV1
		m.Rank = len(usable)
		m.Faces = faces
		m.InferredExtent = len(usable) < len(edges.Edges)
		m.NIS = nis
		measured = true
	}

	if !measured && sb.reference == ReferenceBodyCentre && sb.support.CoastedFrames+1 >= t.solidBodyFacelessLimit() {
		// The object is being observed, but not in any way this measurement
		// model can use, and has been for as long as the tracker lets an
		// unconfirmed hypothesis go unmeasured. Coasting further would let the
		// body drift from evidence that is arriving every frame, so the
		// body-centre claim lapses and the body is re-seeded where the evidence
		// is, stating the medoid's bias. The next usable face re-references it.
		t.lapseSolidBodyToMedoid(sb, cluster, prior)
		m.Source, m.Rank, m.FallbackReason = MeasurementMedoidV0, 2, "body_centre_lapsed"
		measured = true
	}
	if !measured && sb.reference == ReferenceClusterMedoid {
		// Never re-referenced, so the medoid is still what the state
		// describes, and the tracked filter's own noise applies to it.
		nis, ok := t.applyMedoidMeasurement(sb, cluster)
		if ok {
			m.Source = MeasurementMedoidV0
			m.Rank = 2
			m.NIS = nis
			measured = true
		}
	}
	t.finishSolidBodyState(track, "update")
	if !sb.seeded {
		return
	}

	// Extent evidence comes from every face the model found, used or not.
	t.admitSolidBodyExtents(track, cluster, edges)
	if measured {
		sb.lastObservedNanos = track.LastMeasurementUnixNanos
		sb.support = SupportState{PointCount: cluster.PointsCount, Instant: SupportObserved}
	} else {
		// The track was observed, but nothing its measurement model could use
		// was, so for this estimate the instant is a coast.
		sb.support.CoastedFrames++
		sb.support.Instant = SupportCoasted
	}
	t.advanceSolidBodyLifecycle(track, class, !measured)
	t.publishSolidBody(track, class, m)
}

// solidBodyFacelessLimit is how many consecutive frames a body-centre solid
// body may go without a usable face before its claim lapses: the tracker's
// own miss budget for a tentative track, and never less than one.
func (t *Tracker) solidBodyFacelessLimit() int {
	return max(1, t.Config.MaxMisses)
}

// lapseSolidBodyToMedoid re-seeds the solid body's position at the cluster
// medoid, with the seed's covariance stating the medoid's bias and no
// position-velocity correlation, and references it to the medoid again.
// Velocity and every belief outside the dynamic state are kept: they describe
// the body, not where on it the position refers to.
func (t *Tracker) lapseSolidBodyToMedoid(sb *solidBodyTrack, cluster WorldCluster, prior classDimensionPrior) {
	bias := (prior.widthMetres / 2) * (prior.widthMetres / 2)
	sb.state[0], sb.state[1] = cluster.CentroidX, cluster.CentroidY
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if i < 2 || j < 2 {
				sb.p[i*4+j] = 0
			}
		}
	}
	sb.p[0*4+0], sb.p[1*4+1] = bias, bias
	sb.reference = ReferenceClusterMedoid
}

// evidenceBackedEdges keeps the faces whose half-extent rests on evidence
// about this object.
func evidenceBackedEdges(edges []EdgeMeasurement) []EdgeMeasurement {
	out := make([]EdgeMeasurement, 0, len(edges))
	for _, e := range edges {
		if e.HalfExtentProvenance.IsEvidence() {
			out = append(out, e)
		}
	}
	return out
}

// applyEdgeMeasurements updates a solid-body state once per observed face
// along that face's normal, returning the updated state and covariance. The
// faces' noises are independent, so sequential scalar updates are exactly the
// joint update, and a face says nothing about the direction across its own
// normal. Nothing is returned as updated unless every face applied.
//
// Each face carries the tracked filter's own measurement noise, so the two
// filters differ in what they measure and not in how much they trust it. The
// half-extent's error is a bias, not noise: Section 8.1 forbids widening the
// measurement variance to cover it, and a variance that did would also turn
// the filter into a smoother that lags a real manoeuvre.
func (t *Tracker) applyEdgeMeasurements(state [4]float32, p [16]float32, edges []EdgeMeasurement) ([4]float32, [16]float32, float32, VisibleFaces, bool) {
	var nis float64
	var faces VisibleFaces
	next := state
	nextP := p
	r := float64(t.Config.MeasurementNoise)
	for _, e := range edges {
		term, ok := scalarPositionUpdate(&next, &nextP,
			float64(e.NormalX), float64(e.NormalY), float64(e.ImpliedCentreOffset()), r)
		if !ok {
			return state, p, 0, 0, false
		}
		nis += term
		faces |= faceBit(e.Face)
	}
	return next, nextP, float32(nis), faces, true
}

// applyMedoidMeasurement updates a medoid-referenced solid body from the
// cluster medoid, as two independent scalar updates with the tracked filter's
// isotropic noise: exactly its two-dimensional update.
func (t *Tracker) applyMedoidMeasurement(sb *solidBodyTrack, cluster WorldCluster) (float32, bool) {
	r := float64(t.Config.MeasurementNoise)
	next := sb.state
	nextP := sb.p
	nx, okX := scalarPositionUpdate(&next, &nextP, 1, 0, float64(cluster.CentroidX), r)
	ny, okY := scalarPositionUpdate(&next, &nextP, 0, 1, float64(cluster.CentroidY), r)
	if !okX || !okY {
		return 0, false
	}
	sb.state, sb.p = next, nextP
	return float32(nx + ny), true
}

// scalarPositionUpdate applies one scalar measurement z = h·(x, y) + v, with
// v ~ N(0, r) and h a unit direction, to a four-state CV filter, using the
// Joseph form so the covariance stays symmetric. It returns the update's NIS,
// and false without changing anything when the innovation variance is not
// positive.
func scalarPositionUpdate(x *[4]float32, p *[16]float32, hx, hy, z, r float64) (float64, bool) {
	var pht [4]float64
	for i := 0; i < 4; i++ {
		pht[i] = float64(p[i*4+0])*hx + float64(p[i*4+1])*hy
	}
	s := hx*pht[0] + hy*pht[1] + r
	if !(s > 0) || math.IsInf(s, 0) {
		return 0, false
	}
	innovation := z - (hx*float64(x[0]) + hy*float64(x[1]))
	var k [4]float64
	for i := range k {
		k[i] = pht[i] / s
	}
	for i := range x {
		x[i] += float32(k[i] * innovation)
	}

	// P' = (I - k h) P (I - k h)ᵀ + r k kᵀ, with h = [hx, hy, 0, 0].
	var a [16]float64
	for i := 0; i < 4; i++ {
		a[i*4+i] = 1
		a[i*4+0] -= k[i] * hx
		a[i*4+1] -= k[i] * hy
	}
	var ap [16]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var sum float64
			for m := 0; m < 4; m++ {
				sum += a[i*4+m] * float64(p[m*4+j])
			}
			ap[i*4+j] = sum
		}
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var sum float64
			for m := 0; m < 4; m++ {
				sum += ap[i*4+m] * a[j*4+m]
			}
			p[i*4+j] = float32(sum + r*k[i]*k[j])
		}
	}
	return innovation * innovation / s, true
}

// admitSolidBodyExtents admits this frame's spans as lower-bound dimension
// evidence, per dimension, under Section 9.2.1's rules as far as the tracker
// can apply them: only on an axis whose near face was measured, never while
// the estimation lifecycle is initialising, and never on a suspected merge.
//
// Truncation at the field of view and fragmentation are not detected by the
// tracker, so those two rules are not applied here; they are recorded as
// false in the support state rather than claimed.
func (t *Tracker) admitSolidBodyExtents(track *TrackedObject, cluster WorldCluster, set EdgeMeasurementSet) {
	sb := &track.solidBody
	if len(set.Edges) == 0 || sb.estimation == EstimationInitialising || track.MergeCandidate {
		return
	}
	// Section 9.2: a span is lower-bound evidence only along believed axes.
	// Where the body is moving fast enough for its course to mean something,
	// an axis further from the course than the span search covers is not
	// believed: an axis swap reads the length as the width, and the
	// corroborated maximum would keep it for the rest of the track. On kirk0
	// the tracked heading was over ten degrees off the course on 59% of
	// moving near-edge frames. The axis is undirected, so the comparison is
	// folded; for a pedestrian stepping sideways this refuses evidence
	// rather than admitting it, which is the safe direction.
	if speed := math.Hypot(float64(sb.state[2]), float64(sb.state[3])); speed >= CourseAlignmentMinSpeedMps {
		course := math.Atan2(float64(sb.state[3]), float64(sb.state[2]))
		if FoldAxisAngleDeg(float64(sb.orientation.PsiRad)-course) > spanSearchHalfWindowDeg {
			return
		}
	}
	for _, e := range set.Edges {
		if e.Face.IsLongitudinal() {
			if span, ok := minimumAxisSpan(cluster.RetainedPoints, sb.orientation.PsiRad); ok {
				sb.lengthBelief.Observe(span)
			}
		} else if span, ok := minimumAxisSpan(cluster.RetainedPoints, sb.orientation.PsiRad+math.Pi/2); ok {
			sb.widthBelief.Observe(span)
		}
	}
}

// spanTrimPercent trims each end of a span measurement. It is smaller than the
// near-edge percentile on purpose: a span is lower-bound evidence, and
// trimming five per cent from each end of a uniformly sampled side would
// understate it by a tenth. One per cent still keeps a stray return from
// defining it.
const spanTrimPercent = 1.0

// spanSearchHalfWindowDeg and spanSearchStepDeg bound the axis search a span
// takes its minimum over: the orientation convergence bound either side of the
// believed axis, in whole degrees.
const (
	spanSearchHalfWindowDeg = 10
	spanSearchStepDeg       = 1
)

// minimumAxisSpan is the smallest trimmed extent of the points along any axis
// within spanSearchHalfWindowDeg of axisRad.
//
// Taking the minimum is what keeps a span a lower bound when the believed axis
// is wrong. Across a side of length L, an axis error of theta adds up to
// L·sin(theta) to the width: seven degrees of heading lag on a 4.5 m car is
// over half a metre, which a corroborated maximum would then hold for the rest
// of the track. The span along the true axis is at most the true dimension, and
// the minimum over a window containing it is at most that span, so the result
// can understate the dimension but cannot overstate it.
func minimumAxisSpan(points []l4perception.WorldPoint, axisRad float32) (float32, bool) {
	if len(points) < DefaultMinFaceSupport {
		return 0, false
	}
	projections := make([]float64, len(points))
	best := math.Inf(1)
	for deg := -spanSearchHalfWindowDeg; deg <= spanSearchHalfWindowDeg; deg += spanSearchStepDeg {
		angle := float64(axisRad) + float64(deg)*math.Pi/180
		span, ok := trimmedSpan(points, projections, math.Cos(angle), math.Sin(angle))
		if ok && span < best {
			best = span
		}
	}
	if math.IsInf(best, 0) {
		return 0, false
	}
	return float32(best), true
}

// trimmedSpan is the extent of the points along a unit direction between the
// spanTrimPercent ends, using scratch for the projections.
func trimmedSpan(points []l4perception.WorldPoint, scratch []float64, dirX, dirY float64) (float64, bool) {
	for i, p := range points {
		scratch[i] = p.X*dirX + p.Y*dirY
	}
	sort.Float64s(scratch)
	last := len(scratch) - 1
	lo := int(spanTrimPercent / 100 * float64(last))
	span := scratch[last-lo] - scratch[lo]
	if !(span > 0) || math.IsInf(span, 0) {
		return 0, false
	}
	return span, true
}

// coastSolidBody records a frame on which the track had no association. The
// prediction already happened with the tracked state's; this adds the same
// coast inflation the tracked covariance received, so the two filters stay
// comparable through a gap, and carries the tracked instant's support token,
// which is where an explained absence is recorded.
func (t *Tracker) coastSolidBody(track *TrackedObject, inflation float32) {
	sb := &track.solidBody
	if !sb.seeded {
		return
	}
	if inflation > 0 {
		sb.p[0*4+0] += inflation
		sb.p[1*4+1] += inflation
		if sb.p[0*4+0] > t.Config.MaxCovarianceDiag {
			sb.p[0*4+0] = t.Config.MaxCovarianceDiag
		}
		if sb.p[1*4+1] > t.Config.MaxCovarianceDiag {
			sb.p[1*4+1] = t.Config.MaxCovarianceDiag
		}
	}
	sb.support.CoastedFrames++
	sb.support.Instant = track.LastSupport
	class := solidBodyClass(track)
	t.advanceSolidBodyLifecycle(track, class, true)
	t.publishSolidBody(track, class, SolidBodyMeasurement{FallbackReason: "no_association"})
}

// advanceSolidBodyLifecycle applies NextEstimationState with the evidence the
// tracker has: association is sustained once the track is confirmed, speed is
// the solid body's own, and evidence is inadequate on a frame that did not
// update it. Model invalidity is Section 12's, and is never raised here.
func (t *Tracker) advanceSolidBodyLifecycle(track *TrackedObject, class MotionClassBelief, inadequate bool) {
	sb := &track.solidBody
	e := t.assembleSolidBody(track, class)
	sb.estimation = NextEstimationState(sb.estimation, e, EstimationEvidence{
		SustainedAssociation: track.TrackState == TrackConfirmed,
		SpeedMps:             float32(math.Hypot(float64(sb.state[2]), float64(sb.state[3]))),
		EvidenceInadequate:   inadequate,
	}, DefaultConvergenceBounds())
}

// assembleSolidBody builds the estimate from the solid body's current state.
func (t *Tracker) assembleSolidBody(track *TrackedObject, class MotionClassBelief) SolidBodyEstimate {
	sb := &track.solidBody
	effective, _ := class.Effective()
	prior := dimensionPriorFor(effective)
	return SolidBodyEstimate{
		StateModel: StateModelCVCartesianV1,
		Reference:  sb.reference,
		X:          sb.state[0],
		Y:          sb.state[1],
		PositionCovariance: [4]float32{
			sb.p[0*4+0], sb.p[0*4+1],
			sb.p[1*4+0], sb.p[1*4+1],
		},
		Orientation: sb.orientation,
		Length:      dimensionFromBelief(sb.lengthBelief, prior.lengthMetres, prior.sigmaMetres),
		Width:       dimensionFromBelief(sb.widthBelief, prior.widthMetres, prior.sigmaMetres),
		// Height stays the class prior, as in SolidBodyFromTrack: vertical
		// extent is corrupted by the P11 grade artefact on any slope.
		Height: DimensionBelief{
			Metres: prior.heightMetres, SigmaMetres: prior.sigmaMetres,
			Provenance: ProvenanceClassPrior,
		},
		GroundZ:               track.LatestZ,
		GroundSurfaceModel:    GroundSurfaceClusterMinZ,
		Motion:                class,
		Estimation:            sb.estimation,
		Stage:                 StageLive,
		LastObservedUnixNanos: sb.lastObservedNanos,
		Support:               sb.support,
	}
}

// publishSolidBody stores the reading for this instant.
func (t *Tracker) publishSolidBody(track *TrackedObject, class MotionClassBelief, m SolidBodyMeasurement) {
	sb := &track.solidBody
	e := t.assembleSolidBody(track, class)
	if sb.orientation.Provenance != ProvenanceNone {
		los := math.Atan2(float64(sb.state[1]-t.Config.SolidBody.SensorY), float64(sb.state[0]-t.Config.SolidBody.SensorX))
		m.AspectRad = float32(FoldAxisAngleDeg(float64(sb.orientation.PsiRad)-los) * math.Pi / 180)
		m.AspectKnown = true
	}
	sb.reading = SolidBodyReading{
		Estimate:    e,
		VX:          sb.state[2],
		VY:          sb.state[3],
		Covariance:  sb.p,
		Measurement: m,
	}
}

// orientationFromTrack reads this frame's heading decision as an orientation
// belief, by the rule SolidBodyFromTrack applies. When the tracked heading was
// held this frame the previous belief is kept: a held heading is not evidence
// about this frame, but the body did not stop having an orientation either.
func orientationFromTrack(t *TrackedObject, previous OrientationBelief) OrientationBelief {
	if o, ok := trackOrientation(t); ok {
		return o
	}
	return previous
}
