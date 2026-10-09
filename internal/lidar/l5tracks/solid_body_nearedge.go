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
//     frame moves nothing across it (Section 9.1), unless remedy T5 is on
//     (SolidBodyOptions.RankOneMedoidScale). A face whose half-extent
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
	// OriginSource says where SensorX and SensorY came from: a sensor
	// geometry declaration's id, or the tracking transform the frame was
	// built with. Empty means the origin was never declared, and the solid
	// body then makes no near-edge fix and says so on every row: (0, 0) is a
	// value, not a declaration (the near-edge plan's invariant 5).
	OriginSource string
	// FaceHysteresis is remedy T1 of the near-edge tracked-state plan: a face
	// enters the fix only once it has been usable on faceHysteresisFrames
	// consecutive frames, and leaves only once it has been unusable for as
	// many, so a face at the support threshold stops flickering in and out.
	// Until then the frame is not a fix. Default false.
	FaceHysteresis bool
	// FaceEntryConsider is remedy T2: the first update from a face that was
	// not in the previous fix adds the variance of the half-extent behind it
	// to that face's measurement noise, once, as a consider term. Later
	// updates from the same face carry the measurement noise alone, as
	// Section 8.1 requires. Default false.
	FaceEntryConsider bool
	// CourseAlignedFaces is remedy T3: while the solid body moves at
	// CourseAlignmentMinSpeedMps or more, the body axis that picks and
	// orients the near faces, and along which extent spans are taken, is its
	// own course rather than the tracked heading, which lags a turn by its
	// smoothing. Below that speed the tracked heading is used. The reported
	// orientation is the tracked heading either way. It assumes a body moves
	// along its length, which holds for vehicles and cyclists and not for a
	// pedestrian stepping sideways. Default false.
	CourseAlignedFaces bool
	// ReferenceTranslation makes the first fix a translation before it is an
	// update, per the near-edge plan's invariant 3: the medoid-referenced
	// position moves along each fixing face's normal by the offset between
	// this frame's reconstructed centre and its medoid, the position
	// covariance widens by the medoid's bias, and only then do the faces
	// update the filter, so their innovation is the prediction's error and
	// not half a body. Velocity is untouched by the translation. Without it
	// the faces absorb the offset as an innovation, which the shadow has
	// always done; NearEdgeTracking always translates, because there the
	// velocity the innovation would move is the tracked one. Default false.
	ReferenceTranslation bool
	// RankOneMedoidScale is remedy T5: at a fix by a front or rear face
	// alone, the width direction that face leaves open is also updated from
	// the cluster medoid projected onto it, with the tracked noise R plus
	// this scale times the square of the believed half-width. At scale one
	// that is A2's loose term, used by the update as well as the gate.
	// Without it the lateral position rides on the prediction until a side
	// face arrives and corrects it in one step: the rank-one drift the plan
	// names as the likelier cause of the lateral-face entry tail F5 and F6
	// measured. It is the declared exception to the plan's invariant 2 (a
	// rank-one frame moves nothing across its normal).
	//
	// It does not apply when any side face was found on the frame, even one
	// that face hysteresis is still withholding or whose half-extent is only
	// the class prior: a visible side pulls the medoid toward it, so the
	// medoid is most biased across the body exactly then. Nor does it apply
	// at a fix by a side face alone, where the open direction is the length:
	// the medoid slides along a passing body as its aspect changes, and the
	// position-velocity covariance would carry that slide into the speed.
	// The medoid's offset is a bias toward the sensor, not noise, which the
	// five-point residual cannot see; physical references are the check.
	// Zero, the default, leaves the open direction to the prediction.
	RankOneMedoidScale float32
	// CourseHeading makes the solid body's orientation its own course while
	// it moves at CourseAlignmentMinSpeedMps or more, resolved by the
	// direction of travel, with the course's variance from the velocity's
	// uncertainty across it. Below that speed the tracked heading is used.
	// The tracked heading is a PCA axis seeded at birth, and for a cluster
	// that is one end face, a strip as wide as the vehicle and a few
	// decimetres deep, that axis runs across the vehicle; nothing corrects a
	// heading that started wrong, and velocity only flips it by pi. It
	// assumes a body moves along its length, as CourseAlignedFaces does.
	// Default false.
	CourseHeading bool
	// ExtentPriorFloor holds a dimension at the class prior while what has
	// accumulated is shorter: every span is a lower bound, and one shorter
	// than the prior says nothing the prior does not. Without it the first
	// span replaces the prior, so an approaching truck's front face makes it
	// 0.6 m long. Default false.
	ExtentPriorFloor bool
	// FacePlaneSpans admits, for each face found, the span along that face's
	// own plane: an end face's span across the axis as the width, a side
	// face's span along the axis as the length. Without it each face admits
	// the span along its normal, which a single face sees only as deep as
	// the returns behind it reach: the face depth, for a vehicle seen end
	// on. In a corner view the two rules admit the same two spans, since
	// each face's normal is the other face's plane. Default false.
	FacePlaneSpans bool
	// VehicleExtentFloor holds a rigid vehicle's accumulated length and width
	// at no less than the smallest road car's, 2.4 by 1.4 m. Every span is a
	// lower bound, and one below that is a partial view, not a small vehicle;
	// without the floor it replaces the prior, so a truck seen across its
	// rear is a metre wide. Unlike ExtentPriorFloor it keeps the dimension
	// accumulated, so the faces it places stay in the fix, and it does not
	// lengthen a small car seen whole. A body of unknown class already seen
	// as long as that car is held at its width (see widthOf); other classes,
	// whose smallest size is not known, are unchanged. Default false.
	VehicleExtentFloor bool
	// EndFaceCentring is a rank-one remedy like T5, from the end face itself
	// rather than the medoid: at a fix by a front or rear face alone, the
	// width direction that face leaves open is also updated from the
	// midpoint of the returns on the face, which it sees across its whole
	// width, with the tracked noise R plus the square of half of however much
	// narrower than the believed width the face was seen. Unlike T5 it
	// applies when a side face is found but not in the fix: a side pulls the
	// medoid toward it, but its returns lie behind the end face's plane, not
	// on it. Without it the position across the body rides on the prediction
	// until a side face corrects it in one step. A face seen less than half
	// as wide as the believed width is taken as cut off and gives nothing.
	// Default false.
	EndFaceCentring bool
	// EndFaceCentringOpenPrior, with EndFaceCentring, refuses a narrow face
	// only against a width measured on this body. Reading a narrow face as
	// cut off needs a width for it to be cut off from, and a class prior is
	// the class's, not this body's; against one the face is used, at its
	// shortfall's variance. Without it a body still at the prior width has
	// nothing across it at an end-face fix until a width is measured, and
	// then the term arrives in one step. Default false.
	EndFaceCentringOpenPrior bool
	// Containment holds the reported box on the frame's points, the geometry
	// convergence plan's W2, by two rules kept apart. The reported length and
	// width are never below the frame's window-minimum observed spans along
	// and across the orientation: that is the persisted and drawn box, and
	// the extent beliefs never read the floor, since a floor that fed the
	// belief would corroborate a merge. And at a fix, or on a faceless frame
	// that would otherwise lapse, the position along each body axis is held
	// to the interval the points allow given the belief's upper tail, by
	// truncating the position's marginal to that interval (Simon 2010, the
	// density-truncation form): no gain to tune, a move bounded by the
	// state's own uncertainty, a no-op well inside the interval, applied to
	// the position block alone. A belief shorter than the span gives no
	// interval, and that frame is extent evidence rather than a position
	// measurement; a merge candidate's frame is left alone. Default false.
	Containment bool
	// RectangleFit fits the rectangle orientation to each associated
	// frame's points (l4perception.FitRectangle) and records it on the row
	// as a diagnostic: the geometry convergence plan's W1a. Nothing reads it
	// yet; W1b makes it the heading observation. Default false.
	RectangleFit bool
	// ExtentGrowthAdmission keeps length evidence flowing while the track is
	// a merge candidate, if the cluster is no wider across the body than the
	// believed width plus extentGrowthLateralMarginMetres. Width is still
	// refused then. The merge test
	// compares this frame's box with the mean of every earlier one, so a
	// vehicle whose side comes into view after a run of end-on frames is a
	// "merge" for as long as that mean takes to catch up, which refuses its
	// length exactly when it is first visible. Two road users side by side
	// widen the cluster across the body; one seen more fully lengthens it
	// along the body. Default false.
	ExtentGrowthAdmission bool
}

// extentGrowthLateralMarginMetres is how much wider than the believed width
// a merge-candidate cluster may be and still be one body's growing view. A
// second road user beside it adds at least a pedestrian's width, 0.6 m, and
// the gap between them; a metre is the narrowest such pair plus a gap, so a
// wider cluster is still refused.
const extentGrowthLateralMarginMetres = 1.0

// ReferenceChange names a change of the point a solid body's position refers
// to, recorded on the instant it happened.
type ReferenceChange uint8

const (
	// ReferenceUnchanged is every instant on which the reference stayed put.
	ReferenceUnchanged ReferenceChange = iota
	// ReferenceToBodyCentre is the first fix after the body was referenced to
	// the medoid: its seed, or a lapse.
	ReferenceToBodyCentre
	// ReferenceToMedoid is a lapse: a body-centre body that went too long
	// without a usable face, re-seeded where the evidence is.
	ReferenceToMedoid
)

// String names the change for diagnostics and summaries; no change is the
// empty string.
func (c ReferenceChange) String() string {
	switch c {
	case ReferenceToBodyCentre:
		return "medoid_to_body_centre"
	case ReferenceToMedoid:
		return "body_centre_to_medoid"
	default:
		return ""
	}
}

// SolidBodyReferenceChanges counts the solid bodies' reference changes over a
// tracker's life, so a replay can report them beside its fixes and lapses.
type SolidBodyReferenceChanges struct {
	ToBodyCentre int64
	ToMedoid     int64
}

// faceHysteresisFrames is how many consecutive frames a face must be usable
// before FaceHysteresis admits it, and unusable before it is dropped.
const faceHysteresisFrames = 2

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
	// ReferenceChange is the reference change this instant made, if any.
	ReferenceChange ReferenceChange
	// ContainmentShare is the share of the frame's cluster points inside the
	// box the estimate reports (its position, orientation and dimensions,
	// widened by DefaultFaceToleranceMetres), and ContainedPoints is how many
	// points were counted. It measures the box against the points it was
	// measured from, since the body cannot be where its returns are not. It
	// is meaningful only when ContainmentKnown: a seed, a coast or a frame
	// with no orientation carries none.
	ContainmentShare float32
	ContainedPoints  int
	ContainmentKnown bool
	// ObservedSpanAlongMetres and ObservedSpanAcrossMetres are the cluster's
	// trimmed spans along and across the reported orientation, zero when
	// unknown. A span is a lower bound on the dimension (state plan 9.2.1),
	// and along a wrong axis it overstates, so this is the reported extent
	// measured against the frame, not a floor on it.
	ObservedSpanAlongMetres  float32
	ObservedSpanAcrossMetres float32
	// ExtentFloor names the reported dimensions Containment floored at the
	// frame's window-minimum span this instant: "", "length", "width" or
	// "length,width". The beliefs are untouched.
	ExtentFloor string
	// ContainmentShiftAlongMetres and ContainmentShiftAcrossMetres are the
	// moves Containment's truncation made along the body axis and along its
	// left normal this instant, signed; zero when it did not act.
	ContainmentShiftAlongMetres  float32
	ContainmentShiftAcrossMetres float32
	// Rectangle is the frame's rectangle orientation fit under RectangleFit:
	// the axis modulo 90 degrees with its sigma and plateau, the spans along
	// and across it, and why it abstained, if it did. RectangleKnown is
	// false when no fit was made.
	RectangleKnown      bool
	RectangleAxisRad    float32
	RectangleSigmaRad   float32
	RectanglePlateauRad float32
	RectangleSpan1      float32
	RectangleSpan2      float32
	RectangleAbstain    string
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
	// floorAlong and floorAcross are this frame's window-minimum observed
	// spans under Containment, the floor assembleSolidBody reports the
	// dimensions at; zero on a frame without points.
	floorAlong, floorAcross float32

	lastObservedNanos int64
	support           SupportState

	// faceRun counts, per face, the consecutive frames it has been usable
	// (positive) or unusable (negative), capped at faceHysteresisFrames either
	// way, and admitted is the set FaceHysteresis currently lets into the
	// fix. Both are maintained only when FaceHysteresis is on.
	faceRun  [len(allBodyFaces)]int8
	admitted VisibleFaces
	// lastFixFaces is the face set of the most recent near-edge fix since the
	// body was last referenced to the medoid; FaceEntryConsider treats any
	// other face as entering.
	lastFixFaces VisibleFaces

	// tracked says the body has no filter of its own: under NearEdgeTracking
	// state and p are copies of the tracked filter's, taken whenever the
	// state machine or a reading needs them, and the step's result is
	// written back to it.
	tracked bool
	// pending is this frame's step under NearEdgeTracking, taken during the
	// tracked update and completed once the frame's merge flags are known.
	pending nearEdgePending
	// hasPair and pairClusterIdx name the cluster association gave a tracked
	// body this frame, whose measurement the gate kept.
	hasPair        bool
	pairClusterIdx int

	// reading is assembled when the state changes, so the estimate, its
	// lifecycle decision and its class prior all describe the same instant.
	reading SolidBodyReading
}

// nearEdgeFrame is the near-edge measurement one associated frame offers the
// state machine, made before the machine consumes it: by the shadow just
// before its update, and under NearEdgeTracking once per candidate pair during
// association, where the same result is kept for the update.
type nearEdgeFrame struct {
	// fallback says why no measurement was attempted; empty when one was.
	fallback string
	edges    EdgeMeasurementSet
	// axis is the body axis the faces and spans are taken along, and
	// axisIsCourse says it is the solid body's course (faceAxis).
	axis         float32
	axisIsCourse bool
	// length and width are the beliefs the half-extents came from.
	length, width DimensionBelief
	// mergeCandidate is the track's merge flag as the step knows it: this
	// frame's for the shadow, the previous frame's under NearEdgeTracking,
	// where the flags follow the update.
	mergeCandidate bool
}

// nearEdgeOutcome is what one step of the state machine did to the dynamic
// state.
type nearEdgeOutcome uint8

const (
	// nearEdgeCoast: nothing updated the state. For the body the instant is
	// a coast although the track was associated.
	nearEdgeCoast nearEdgeOutcome = iota
	// nearEdgeFix: the faces updated a body-centre state.
	nearEdgeFix
	// nearEdgeLapse: the body-centre claim lapsed and the state was re-seeded
	// at the medoid.
	nearEdgeLapse
	// nearEdgeMedoid: the state is referenced to the medoid, and the medoid
	// update applies: the shadow's own, or under NearEdgeTracking the
	// tracked filter's usual one.
	nearEdgeMedoid
)

// nearEdgePending carries a tracked step from the update to its completion.
type nearEdgePending struct {
	valid   bool
	frame   nearEdgeFrame
	m       SolidBodyMeasurement
	outcome nearEdgeOutcome
	applied nearEdgeApplication
}

// nearEdgeApplication is what a step did to the position, for the record the
// tracked filter keeps of it. shiftX and shiftY are a reference change's
// translation: to the implied centre before a fix, or back to the medoid on
// a lapse. startX and startY are the position the faces updated, after any
// translation, and measuredX and measuredY that position moved along each
// applied face's normal to the face's implied centre offset: the measurement
// the fix applied, at its rank, with nothing across the faces unless T5 also
// moved it to the medoid across the body (addLooseTerm).
type nearEdgeApplication struct {
	shiftX, shiftY       float32
	startX, startY       float32
	measuredX, measuredY float32
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
	if t.Config.NearEdgeTracking {
		sb.tracked = true
		sb.syncFromTrack(track)
	}
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
	if !sb.seeded || sb.tracked {
		// A tracked body is predicted with the tracked filter it copies.
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
// Under NearEdgeTracking the tracked update already took the step, with the
// heading the association measured the pair with, and this completes it.
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
	if sb.tracked {
		t.completeTrackedSolidBody(track, cluster)
		return
	}
	class := solidBodyClass(track)
	effective, _ := class.Effective()
	prior := dimensionPriorFor(effective)
	sb.orientation = t.solidBodyOrientation(track, sb)

	frame := t.measureNearEdgeFrame(sb, cluster, prior, track.ObservationCount, track.MergeCandidate)
	m, outcome, _ := t.stepNearEdge(sb, cluster, prior, frame)
	measured := outcome != nearEdgeCoast
	if outcome == nearEdgeMedoid {
		// Never re-referenced, so the medoid is still what the state
		// describes, and the tracked filter's own noise applies to it.
		nis, ok := t.applyMedoidMeasurement(sb, cluster)
		if ok {
			m.Source = MeasurementMedoidV0
			m.Rank = 2
			m.NIS = nis
		}
		measured = ok
	}
	t.finishSolidBodyState(track, "update")
	if !sb.seeded {
		return
	}
	t.completeSolidBodyFrame(track, class, cluster, frame, m, measured)
}

// measureNearEdgeFrame makes a frame's near-edge measurement with the solid
// body's beliefs, predicted at the body's state, or says why none was made.
// observations is the track's observation count including this frame.
//
// An undeclared origin comes first: it is a configuration fault, and every row
// says so rather than only those past the initialisation window. Phase 2's
// mitigation: until HitsToConfirm observations have passed, the heading that
// selects a face is itself unconverged, so the medoid is used and the body
// stays referenced to it.
func (t *Tracker) measureNearEdgeFrame(sb *solidBodyTrack, cluster WorldCluster, prior classDimensionPrior, observations int, mergeCandidate bool) nearEdgeFrame {
	f := nearEdgeFrame{axis: sb.orientation.PsiRad, mergeCandidate: mergeCandidate}
	switch {
	case t.Config.SolidBody.OriginSource == "":
		f.fallback = "missing_calibrated_sensor_origin"
	case observations <= t.Config.HitsToConfirm:
		f.fallback = "initialisation_window"
	case sb.orientation.Provenance == ProvenanceNone:
		f.fallback = "missing_heading"
	default:
		f.length = t.dimensionOf(sb.lengthBelief, prior.lengthMetres, prior.sigmaMetres, prior.minLengthMetres)
		f.width = t.widthOf(sb, prior)
		f.axis, f.axisIsCourse = t.faceAxis(sb)
		f.edges = MeasureNearEdge(NearEdgeInput{
			Cluster:          cluster,
			Points:           nearEdgePoints(cluster),
			SensorX:          t.Config.SolidBody.SensorX,
			SensorY:          t.Config.SolidBody.SensorY,
			HeadingRad:       f.axis,
			HalfLength:       f.length.Metres / 2,
			HalfWidth:        f.width.Metres / 2,
			LengthProvenance: f.length.Provenance,
			WidthProvenance:  f.width.Provenance,
			PredictedX:       sb.state[0],
			PredictedY:       sb.state[1],
		})
	}
	return f
}

// stepNearEdge is the near-edge state machine: one associated frame's
// transition of a (state, covariance, reference, support) value, shared by the
// shadow and, under NearEdgeTracking, the tracked filter. From the frame's
// measurement it makes a fix (re-referencing a medoid-referenced body to its
// centre first), lets a body-centre claim lapse after too many faceless
// frames, or leaves a medoid-referenced state for the medoid update, which is
// the caller's: the shadow applies its own, the tracked filter its usual one.
// It never touches the track, so the caller decides whose filter sb's state is.
func (t *Tracker) stepNearEdge(sb *solidBodyTrack, cluster WorldCluster, prior classDimensionPrior, f nearEdgeFrame) (SolidBodyMeasurement, nearEdgeOutcome, nearEdgeApplication) {
	var m SolidBodyMeasurement
	var applied nearEdgeApplication
	outcome := nearEdgeCoast
	// facesCounted records whether this frame's usable faces reached the
	// hysteresis counts; any frame that did not is a frame without them.
	facesCounted := false

	switch {
	case f.fallback != "":
		m.FallbackReason = f.fallback
	case f.edges.Rank == 0:
		m.FallbackReason = f.edges.FallbackReason
	default:
		// Section 8.1: a face whose half-extent is only the class prior
		// cannot have its dimension-derived bias corrected, so it is excluded
		// rather than downweighted. It still counts as a seen face for the
		// extent evidence, which is how the prior stops being one.
		usable := evidenceBackedEdges(f.edges.Edges)
		inferred := len(usable) < len(f.edges.Edges)
		if t.Config.SolidBody.FaceHysteresis {
			found := len(usable)
			usable = sb.admitFaces(usable)
			facesCounted = true
			if len(usable) == 0 && found > 0 {
				m.FallbackReason = "face_hysteresis"
				m.InferredExtent = inferred
				break
			}
		}
		if len(usable) == 0 {
			m.FallbackReason = "class_prior_extent"
			m.InferredExtent = true
			break
		}
		startState, startP := sb.state, sb.p
		change := ReferenceUnchanged
		if sb.reference != ReferenceBodyCentre {
			// The state described a point on the sensor-facing surface. From
			// this fix on it describes the body centre, and before the fix its
			// position is uncertain by as much as the medoid was biased.
			if t.translatesReference(sb) {
				startState = translateToFaces(startState, usable, cluster)
			}
			bias := (prior.widthMetres / 2) * (prior.widthMetres / 2)
			startP[0*4+0] += bias
			startP[1*4+1] += bias
			change = ReferenceToBodyCentre
		}
		var consider []float64
		if t.Config.SolidBody.FaceEntryConsider {
			consider = sb.entryConsider(usable, f.length, f.width)
		}
		state, p, nis, faces, ok := t.applyEdgeMeasurements(startState, startP, usable, consider)
		if !ok {
			m.FallbackReason = "singular_innovation"
			break
		}
		applied = applicationOf(sb.state, startState, usable)
		loose, ok := t.endFaceCentringTerm(usable, f, cluster)
		if !ok {
			loose, ok = t.rankOneMedoidTerm(usable, f, cluster)
		}
		if ok {
			// The faces' NIS stays the fix's, with its rank: the loose term
			// is weak evidence about the direction no face measured, not a
			// face; its innovation is biased toward the sensor, so its NIS
			// would not be chi-square anyway; and the per-rank NIS check is
			// about the faces' R.
			if _, ok := scalarPositionUpdate(&state, &p, loose.hx, loose.hy, loose.z, loose.variance); ok {
				applied.addLooseTerm(loose)
			}
		}
		if t.Config.SolidBody.Containment && !f.mergeCandidate {
			// Only the directions the fix left open: a face has placed the
			// box along its own normal, and a truncation there would only
			// follow the far extreme's jitter.
			alongOpen, acrossOpen := true, true
			for _, e := range usable {
				if e.Face.IsLongitudinal() {
					alongOpen = false
				} else {
					acrossOpen = false
				}
			}
			along, across := t.containmentTruncation(&state, &p, f, cluster, alongOpen, acrossOpen)
			applied.addShift(f.axis, along, across)
			m.ContainmentShiftAlongMetres, m.ContainmentShiftAcrossMetres = along, across
		}
		sb.state, sb.p, sb.reference = state, p, ReferenceBodyCentre
		sb.lastFixFaces = faces
		m.Source = MeasurementNearEdgeCandidateV1
		m.Rank = len(usable)
		m.Faces = faces
		m.InferredExtent = inferred
		m.NIS = nis
		m.ReferenceChange = change
		outcome = nearEdgeFix
	}

	if t.Config.SolidBody.FaceHysteresis && !facesCounted {
		sb.admitFaces(nil)
	}

	if outcome == nearEdgeCoast && sb.reference == ReferenceBodyCentre && sb.support.CoastedFrames+1 >= t.solidBodyFacelessLimit() &&
		t.Config.SolidBody.Containment && !f.mergeCandidate && f.fallback == "" && len(nearEdgePoints(cluster)) >= DefaultMinFaceSupport {
		// Under Containment the points can still hold the box: the claim
		// stands, held to them, and lapses only when there are no points.
		along, across := t.containmentTruncation(&sb.state, &sb.p, f, cluster, true, true)
		m.ContainmentShiftAlongMetres, m.ContainmentShiftAcrossMetres = along, across
		applied.startX, applied.startY = sb.state[0], sb.state[1]
		applied.measuredX, applied.measuredY = sb.state[0], sb.state[1]
	} else if outcome == nearEdgeCoast && sb.reference == ReferenceBodyCentre && sb.support.CoastedFrames+1 >= t.solidBodyFacelessLimit() {
		// The object is being observed, but not in any way this measurement
		// model can use, and has been for as long as the tracker lets an
		// unconfirmed hypothesis go unmeasured. Coasting further would let the
		// body drift from evidence that is arriving every frame, so the
		// body-centre claim lapses and the body is re-seeded where the evidence
		// is, stating the medoid's bias. The next usable face re-references it.
		before := sb.state
		t.lapseSolidBodyToMedoid(sb, cluster, prior)
		applied.shiftX, applied.shiftY = sb.state[0]-before[0], sb.state[1]-before[1]
		applied.startX, applied.startY = sb.state[0], sb.state[1]
		applied.measuredX, applied.measuredY = sb.state[0], sb.state[1]
		m.Source, m.Rank, m.FallbackReason = MeasurementMedoidV0, 2, "body_centre_lapsed"
		m.ReferenceChange = ReferenceToMedoid
		outcome = nearEdgeLapse
	}
	if outcome == nearEdgeCoast && sb.reference == ReferenceClusterMedoid {
		outcome = nearEdgeMedoid
	}
	return m, outcome, applied
}

// applicationOf records a fix: the translation from the state before it to
// the state the faces updated, and the measurement the faces applied there.
func applicationOf(before, start [4]float32, faces []EdgeMeasurement) nearEdgeApplication {
	a := nearEdgeApplication{
		shiftX: start[0] - before[0], shiftY: start[1] - before[1],
		startX: start[0], startY: start[1], measuredX: start[0], measuredY: start[1],
	}
	for _, e := range faces {
		along := float64(e.ImpliedCentreOffset()) - (float64(e.NormalX)*float64(start[0]) + float64(e.NormalY)*float64(start[1]))
		a.measuredX += float32(along * float64(e.NormalX))
		a.measuredY += float32(along * float64(e.NormalY))
	}
	return a
}

// looseMedoidTerm is one scalar measurement of the body centre along a
// direction no face constrained: the medoid's projection z onto the unit
// direction (hx, hy), with its variance.
type looseMedoidTerm struct{ hx, hy, z, variance float64 }

// rankOneMedoidTerm is remedy T5's measurement for a fix by a front or rear
// face alone (see SolidBodyOptions.RankOneMedoidScale): across the body, the
// medoid, with R plus the scaled square of the believed half-width, because
// the medoid can sit anywhere from the centre to the near side that way. It
// is false with the option off, for any other fix, and whenever the frame
// found a side face at all, admitted or not.
func (t *Tracker) rankOneMedoidTerm(faces []EdgeMeasurement, f nearEdgeFrame, cluster WorldCluster) (looseMedoidTerm, bool) {
	scale := float64(t.Config.SolidBody.RankOneMedoidScale)
	if !(scale > 0) || len(faces) != 1 || !faces[0].Face.IsLongitudinal() {
		return looseMedoidTerm{}, false
	}
	for _, found := range f.edges.Edges {
		if !found.Face.IsLongitudinal() {
			return looseMedoidTerm{}, false
		}
	}
	e := faces[0]
	hx, hy := -float64(e.NormalY), float64(e.NormalX)
	half := float64(f.width.Metres) / 2
	return looseMedoidTerm{
		hx: hx, hy: hy,
		z:        hx*float64(cluster.CentroidX) + hy*float64(cluster.CentroidY),
		variance: float64(t.Config.MeasurementNoise) + scale*half*half,
	}, true
}

// endFaceCentringTerm is EndFaceCentring's term at a fix by an end face
// alone: across the body, the midpoint of the face's own returns.
func (t *Tracker) endFaceCentringTerm(faces []EdgeMeasurement, f nearEdgeFrame, cluster WorldCluster) (looseMedoidTerm, bool) {
	if !t.Config.SolidBody.EndFaceCentring || len(faces) != 1 || !faces[0].Face.IsLongitudinal() {
		return looseMedoidTerm{}, false
	}
	e := faces[0]
	nx, ny := float64(e.NormalX), float64(e.NormalY)
	hx, hy := -ny, nx
	var across []float64
	for _, p := range nearEdgePoints(cluster) {
		if p.X*nx+p.Y*ny >= float64(e.PlaneOffsetMetres)-DefaultFaceToleranceMetres {
			across = append(across, p.X*hx+p.Y*hy)
		}
	}
	if len(across) < DefaultMinFaceSupport {
		return looseMedoidTerm{}, false
	}
	last := len(across) - 1
	k := int(spanTrimPercent / 100 * float64(last))
	hi := l4perception.NthFloat64(across, last-k)
	lo := l4perception.NthFloat64(across[:last-k], k)
	seen, width := hi-lo, float64(f.width.Metres)
	cutOff := seen < width/2
	if t.Config.SolidBody.EndFaceCentringOpenPrior && !f.width.Provenance.IsEvidence() {
		cutOff = false
	}
	if !(seen > 0) || cutOff {
		return looseMedoidTerm{}, false
	}
	short := math.Max(width-seen, 0) / 2
	return looseMedoidTerm{
		hx: hx, hy: hy, z: (lo + hi) / 2,
		variance: float64(t.Config.MeasurementNoise) + short*short,
	}, true
}

// addLooseTerm records a loose term in a fix's application: the measurement
// moves from the start to the medoid along the term's direction, so the
// recorded innovation that way is the medoid's residual.
func (a *nearEdgeApplication) addLooseTerm(l looseMedoidTerm) {
	along := l.z - (l.hx*float64(a.startX) + l.hy*float64(a.startY))
	a.measuredX += float32(along * l.hx)
	a.measuredY += float32(along * l.hy)
}

// addShift records Containment's moves in a fix's application, along the
// axis and its left normal, so the recorded measurement is where the
// truncation left the position.
func (a *nearEdgeApplication) addShift(axis float32, along, across float32) {
	c, s := math.Cos(float64(axis)), math.Sin(float64(axis))
	a.measuredX += float32(float64(along)*c - float64(across)*s)
	a.measuredY += float32(float64(along)*s + float64(across)*c)
}

// translatesReference says whether a re-reference is a translation before it
// is an update: always for the tracked filter, and for the shadow when asked.
func (t *Tracker) translatesReference(sb *solidBodyTrack) bool {
	return sb.tracked || t.Config.SolidBody.ReferenceTranslation
}

// translateToFaces moves a medoid-referenced position to the body centre this
// frame's faces imply, along each face's normal only: by the offset between
// the face's implied centre offset and the medoid's projection on the same
// normal. A rank-one frame moves nothing across its face, and velocity is not
// touched. The fixing faces are at most one longitudinal and one lateral, whose
// normals are perpendicular, so the per-face moves are independent.
func translateToFaces(state [4]float32, faces []EdgeMeasurement, cluster WorldCluster) [4]float32 {
	for _, e := range faces {
		medoid := float64(e.NormalX)*float64(cluster.CentroidX) + float64(e.NormalY)*float64(cluster.CentroidY)
		shift := float64(e.ImpliedCentreOffset()) - medoid
		state[0] += float32(shift * float64(e.NormalX))
		state[1] += float32(shift * float64(e.NormalY))
	}
	return state
}

// completeSolidBodyFrame finishes an associated frame for the solid body once
// its state is final: extent evidence, support, lifecycle and the reading.
func (t *Tracker) completeSolidBodyFrame(track *TrackedObject, class MotionClassBelief, cluster WorldCluster, f nearEdgeFrame, m SolidBodyMeasurement, measured bool) {
	sb := &track.solidBody
	sb.floorAlong, sb.floorAcross = 0, 0
	// Extent evidence comes from every face the model found, used or not.
	t.admitSolidBodyExtents(track, cluster, f.edges, f.axis, f.axisIsCourse)
	if measured {
		sb.lastObservedNanos = track.LastMeasurementUnixNanos
		sb.support = SupportState{PointCount: cluster.PointsCount, Instant: SupportObserved}
	} else {
		// The track was observed, but nothing its measurement model could use
		// was, so for this estimate the instant is a coast.
		sb.support.CoastedFrames++
		sb.support.Instant = SupportCoasted
	}
	switch m.ReferenceChange {
	case ReferenceToBodyCentre:
		t.solidBodyReferenceChanges.ToBodyCentre++
	case ReferenceToMedoid:
		t.solidBodyReferenceChanges.ToMedoid++
	}
	t.advanceSolidBodyLifecycle(track, class, !measured)
	t.measureContainment(track, class, cluster, &m)
	t.publishSolidBody(track, class, m)
}

// measureContainment records how the box the estimate reports sits on the
// frame's points: the share inside it and the points' spans along and across
// its orientation. It reads the state after the frame's update and extent
// admission, so the box it tests is the one publishSolidBody reports.
func (t *Tracker) measureContainment(track *TrackedObject, class MotionClassBelief, cluster WorldCluster, m *SolidBodyMeasurement) {
	points := nearEdgePoints(cluster)
	sb := &track.solidBody
	if len(points) == 0 || sb.orientation.Provenance == ProvenanceNone {
		return
	}
	if t.Config.SolidBody.Containment && len(points) >= DefaultMinFaceSupport {
		// The floor is set after the belief is read, so the two can be told
		// apart: the beliefs never see it.
		belief := t.assembleSolidBody(track, class)
		sb.floorAlong, sb.floorAcross = windowMinimumSpans(points, float64(sb.orientation.PsiRad))
		floored := t.assembleSolidBody(track, class)
		m.ExtentFloor = extentFloorName(floored.Length.Metres > belief.Length.Metres, floored.Width.Metres > belief.Width.Metres)
	}
	if t.Config.SolidBody.RectangleFit {
		fit := l4perception.FitRectangle(points)
		m.RectangleKnown = fit.Points >= l4perception.RectangleFitMinPoints
		m.RectangleAxisRad, m.RectangleSigmaRad, m.RectanglePlateauRad = float32(fit.AxisRad), float32(fit.SigmaRad), float32(fit.PlateauRad)
		m.RectangleSpan1, m.RectangleSpan2, m.RectangleAbstain = float32(fit.Span1), float32(fit.Span2), fit.Abstain
	}
	e := t.assembleSolidBody(track, class)
	inside := boxContainment(points, float64(e.X), float64(e.Y), float64(e.Orientation.PsiRad),
		float64(e.Length.Metres)/2+DefaultFaceToleranceMetres, float64(e.Width.Metres)/2+DefaultFaceToleranceMetres)
	m.ContainmentShare, m.ContainedPoints, m.ContainmentKnown = float32(inside)/float32(len(points)), len(points), true
	m.ObservedSpanAlongMetres, m.ObservedSpanAcrossMetres = observedSpans(points, float64(e.Orientation.PsiRad))
}

// boxContainment counts the points inside a box of half-extents halfL along
// the axis at psi and halfW across it, centred at (x, y).
func boxContainment(points []l4perception.WorldPoint, x, y, psi, halfL, halfW float64) int {
	c, s := math.Cos(psi), math.Sin(psi)
	inside := 0
	for _, p := range points {
		dx, dy := p.X-x, p.Y-y
		if math.Abs(dx*c+dy*s) <= halfL && math.Abs(-dx*s+dy*c) <= halfW {
			inside++
		}
	}
	return inside
}

// extentFloorName names the dimensions a frame's floor raised.
func extentFloorName(length, width bool) string {
	switch {
	case length && width:
		return "length,width"
	case length:
		return "length"
	case width:
		return "width"
	}
	return ""
}

// windowMinimumSpans is the points' window-minimum spans along the axis at
// psi and across it (minimumAxisSpan, which understates along a wrong axis
// and cannot overstate), zero for too few points.
func windowMinimumSpans(points []l4perception.WorldPoint, psi float64) (along, across float32) {
	along, _ = minimumAxisSpan(points, float32(psi))
	across, _ = minimumAxisSpan(points, float32(psi+math.Pi/2))
	return along, across
}

// Containment's bound noise: the trimmed extreme's sampling spread, and the
// Pandar40P's azimuth step as a range-proportional spacing between returns.
const (
	containmentExtremeNoiseMetres = 0.1
	containmentAngularSpacingRad  = 0.0035
	// containmentVarianceFloor is the least variance a truncation leaves
	// along its direction, (0.01 m)^2.
	containmentVarianceFloor = 1e-4
)

// containmentTruncation holds the position to the interval the frame's
// points allow along and across the body axis, on the directions asked for:
// a box with the belief's upper tail as its dimension must contain the
// points' trimmed extremes. It returns the moves it made along the axis and
// its left normal.
func (t *Tracker) containmentTruncation(state *[4]float32, p *[16]float32, f nearEdgeFrame, cluster WorldCluster, alongOpen, acrossOpen bool) (along, across float32) {
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport || f.length.Metres <= 0 || f.width.Metres <= 0 {
		return 0, 0
	}
	scratch := make([]float64, len(points))
	c, s := math.Cos(float64(f.axis)), math.Sin(float64(f.axis))
	rangeMetres := math.Hypot(float64(state[0]-t.Config.SolidBody.SensorX), float64(state[1]-t.Config.SolidBody.SensorY))
	if alongOpen {
		along = truncateAxis(state, p, points, scratch, c, s, f.length, rangeMetres)
	}
	if acrossOpen {
		across = truncateAxis(state, p, points, scratch, -s, c, f.width, rangeMetres)
	}
	return along, across
}

// truncateAxis is containmentTruncation for one unit direction (ux, uy) and
// the belief of the dimension along it. The points' trimmed extremes, taken
// relative to the position, bound the centre's offset d to
// [hi - h, lo + h] with h half the belief's upper tail; an empty interval
// means the belief cannot hold the span and the frame is extent evidence.
// The interval is widened by the bound's noise, the position's marginal
// along the direction is truncated to it, and its mean and variance replace
// the marginal's through the position block's regression on that direction
// (the conditional form, which keeps the block positive semi-definite);
// velocity rows are untouched. It returns the move along the direction.
func truncateAxis(state *[4]float32, p *[16]float32, points []l4perception.WorldPoint, scratch []float64,
	ux, uy float64, belief DimensionBelief, rangeMetres float64) float32 {
	lo, hi, ok := trimmedExtremes(points, scratch, ux, uy)
	if !ok {
		return 0
	}
	centre := ux*float64(state[0]) + uy*float64(state[1])
	lo, hi = lo-centre, hi-centre
	upper := float64(belief.Metres + belief.SigmaMetres)
	a, b := hi-upper/2, lo+upper/2
	if a > b {
		return 0
	}
	shortfall := (upper - (hi - lo)) / 2
	spacing := rangeMetres * containmentAngularSpacingRad
	boundSigma := math.Sqrt(containmentExtremeNoiseMetres*containmentExtremeNoiseMetres + spacing*spacing + shortfall*shortfall)
	a, b = a-boundSigma, b+boundSigma
	// The marginal along the direction, and the whole state's regression on
	// it: k = P u / sigma^2, so u·k = 1. The truncated marginal replaces the
	// prior one through that regression, the conditional form, which moves
	// the velocity by its covariance with the position and keeps the
	// covariance positive semi-definite; adjusting the position block alone
	// leaves the cross terms inconsistent and the next update's velocity
	// gain unbounded.
	var k [4]float64
	for i := 0; i < 4; i++ {
		k[i] = float64(p[i*4+0])*ux + float64(p[i*4+1])*uy
	}
	sigma2 := ux*k[0] + uy*k[1]
	if !(sigma2 > 0) || math.IsInf(sigma2, 0) {
		return 0
	}
	mean, variance, ok := truncatedNormalMoments(0, math.Sqrt(sigma2), a, b)
	if !ok {
		return 0
	}
	// The truncation never claims the position to better than a
	// centimetre, and in float32 a diagonal can round below zero.
	variance = math.Max(variance, containmentVarianceFloor)
	for i := range k {
		k[i] /= sigma2
		state[i] += float32(k[i] * mean)
	}
	d := variance - sigma2
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			v := float64(p[i*4+j]) + d*k[i]*k[j]
			if i == j && v < 0 {
				v = 0
			}
			p[i*4+j] = float32(v)
		}
	}
	return float32(mean)
}

// truncatedNormalMoments is the mean and variance of N(mu, sigma^2)
// truncated to [a, b]. It is false when the interval holds too little of
// the density to condition on.
func truncatedNormalMoments(mu, sigma, a, b float64) (mean, variance float64, ok bool) {
	alpha, beta := (a-mu)/sigma, (b-mu)/sigma
	phi := func(z float64) float64 { return math.Exp(-z*z/2) / math.Sqrt(2*math.Pi) }
	cdf := func(z float64) float64 { return 0.5 * (1 + math.Erf(z/math.Sqrt2)) }
	mass := cdf(beta) - cdf(alpha)
	if !(mass > 1e-12) {
		return 0, 0, false
	}
	pa, pb := phi(alpha), phi(beta)
	lambda := (pa - pb) / mass
	mean = mu + sigma*lambda
	variance = sigma * sigma * (1 + (alpha*pa-beta*pb)/mass - lambda*lambda)
	if !(variance > 0) {
		variance = 0
	}
	return mean, variance, true
}

// trimmedExtremes is the lower and upper trimmed projections of the points
// on a direction, by the selection trimmedSpan uses.
func trimmedExtremes(points []l4perception.WorldPoint, scratch []float64, dirX, dirY float64) (lo, hi float64, ok bool) {
	if len(points) == 0 {
		return 0, 0, false
	}
	for i, p := range points {
		scratch[i] = p.X*dirX + p.Y*dirY
	}
	last := len(points) - 1
	l := int(spanTrimPercent / 100 * float64(last))
	h := last - l
	upper := l4perception.NthFloat64(scratch[:len(points)], h)
	lower := upper
	if l < h {
		lower = l4perception.NthFloat64(scratch[:h], l)
	}
	if math.IsInf(upper-lower, 0) || math.IsNaN(upper-lower) {
		return 0, 0, false
	}
	return lower, upper, true
}

// observedSpans is the points' trimmed spans along the axis at psi and across
// it, zero for too few points to trim.
func observedSpans(points []l4perception.WorldPoint, psi float64) (along, across float32) {
	if len(points) < DefaultMinFaceSupport {
		return 0, 0
	}
	c, s := math.Cos(psi), math.Sin(psi)
	scratch := make([]float64, len(points))
	if span, ok := trimmedSpan(points, scratch, c, s); ok {
		along = float32(span)
	}
	if span, ok := trimmedSpan(points, scratch, -s, c); ok {
		across = float32(span)
	}
	return along, across
}

// SolidBodyReferenceChanges returns the solid bodies' reference changes since
// the tracker was built or reset.
func (t *Tracker) SolidBodyReferenceChanges() SolidBodyReferenceChanges {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.solidBodyReferenceChanges
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
	sb.lastFixFaces = 0
}

// admitFaces advances FaceHysteresis by one frame in which the given faces
// were usable, and returns those of them now admitted to the fix. A face
// unusable this frame is not returned even if admitted: there is nothing of it
// to measure. Admission is what it keeps, so a face that drops out for a
// single frame comes straight back, while one that has been gone for
// faceHysteresisFrames must qualify again.
func (sb *solidBodyTrack) admitFaces(usable []EdgeMeasurement) []EdgeMeasurement {
	var seen VisibleFaces
	for _, e := range usable {
		seen |= faceBit(e.Face)
	}
	for i, f := range allBodyFaces {
		run := sb.faceRun[i]
		if seen.Has(f) {
			run = max(run, 0) + 1
		} else {
			run = min(run, 0) - 1
		}
		run = max(-faceHysteresisFrames, min(faceHysteresisFrames, run))
		sb.faceRun[i] = run
		switch {
		case run >= faceHysteresisFrames:
			sb.admitted |= faceBit(f)
		case run <= -faceHysteresisFrames:
			sb.admitted &^= faceBit(f)
		}
	}
	out := make([]EdgeMeasurement, 0, len(usable))
	for _, e := range usable {
		if sb.admitted.Has(e.Face) {
			out = append(out, e)
		}
	}
	return out
}

// entryConsider is FaceEntryConsider's extra measurement variance per face:
// the variance of the half-extent behind a face that was not in the previous
// fix, and zero for one that was. A face constrains the centre through its
// plane less the half-extent, so the half-extent's error enters the implied
// centre one-for-one, and half a dimension has a quarter of its variance.
func (sb *solidBodyTrack) entryConsider(edges []EdgeMeasurement, length, width DimensionBelief) []float64 {
	out := make([]float64, len(edges))
	for i, e := range edges {
		if sb.lastFixFaces.Has(e.Face) {
			continue
		}
		sigma := float64(width.SigmaMetres)
		if e.Face.IsLongitudinal() {
			sigma = float64(length.SigmaMetres)
		}
		out[i] = sigma * sigma / 4
	}
	return out
}

// faceAxis is the body axis the near-edge measurement and the extent spans use
// this frame: the tracked heading, unless CourseAlignedFaces is on and the
// solid body moves fast enough for its course to mean something, when it is
// the course, and the second result says so. On columbus-broadway the solid
// body's face-stable p99 is more than twice the point estimate's while turning
// at 15 degrees per second or more, because a face chosen and oriented by a
// lagging heading carries the lag into the centre.
//
// The axis's direction does not change the geometry, since each face's normal
// points at the sensor whichever way the axis runs, but it names the faces:
// MeasureNearEdge calls the face along the axis the front. So the course is
// taken in whichever direction lies within 90 degrees of the tracked heading.
// The faces keep the names the reported orientation gives them, and do not
// swap front for rear when the body crosses the speed threshold under a
// heading that points the other way.
func (t *Tracker) faceAxis(sb *solidBodyTrack) (float32, bool) {
	psi := sb.orientation.PsiRad
	if t.Config.SolidBody.CourseAlignedFaces {
		vx, vy := float64(sb.state[2]), float64(sb.state[3])
		if math.Hypot(vx, vy) >= CourseAlignmentMinSpeedMps {
			course := math.Atan2(vy, vx)
			if math.Cos(course-float64(psi)) < 0 {
				course = math.Remainder(course+math.Pi, 2*math.Pi)
			}
			return float32(course), true
		}
	}
	return psi, false
}

// nearEdgePoints is the geometry the near-edge measurement and the extent
// spans read: the cluster's members when L4 handed them over, and otherwise
// its retained evidence sample. A sample of 122 or more points supports every
// face the members would (the face plane is the 95th percentile, so at least
// 5 % of a sample lies on or beyond it), but a small persistence cap does not,
// and the live pipeline retains no sample at all.
func nearEdgePoints(c WorldCluster) []l4perception.WorldPoint {
	if len(c.Members) > 0 {
		return c.Members
	}
	return c.RetainedPoints
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
// the filter into a smoother that lags a real manoeuvre. consider, when not
// nil, adds a one-off variance to each face's noise (FaceEntryConsider).
func (t *Tracker) applyEdgeMeasurements(state [4]float32, p [16]float32, edges []EdgeMeasurement, consider []float64) ([4]float32, [16]float32, float32, VisibleFaces, bool) {
	var nis float64
	var faces VisibleFaces
	next := state
	nextP := p
	for i, e := range edges {
		r := float64(t.Config.MeasurementNoise)
		if consider != nil {
			r += consider[i]
		}
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
// false in the support state rather than claimed. axis is the body axis the
// frame's faces were measured along, and axisIsCourse says faceAxis took it
// from the solid body's course before this frame's update.
func (t *Tracker) admitSolidBodyExtents(track *TrackedObject, cluster WorldCluster, set EdgeMeasurementSet, axis float32, axisIsCourse bool) {
	sb := &track.solidBody
	if len(set.Edges) == 0 || sb.estimation == EstimationInitialising {
		return
	}
	// A merge candidate that is only growing gives its length and nothing
	// else: a view filling in lengthens a body, while a second body beside it
	// widens it, and the corroborated maximum would keep a merged width.
	growing := track.MergeCandidate
	if growing && !t.growingNotMerging(track, cluster, axis) {
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
	//
	// An axis taken from the course is believed by construction. It is the
	// course before this frame's update, and the update can turn the
	// velocity, so comparing it with the course after the update would refuse
	// spans measured along exactly the axis the faces were.
	if speed := math.Hypot(float64(sb.state[2]), float64(sb.state[3])); !axisIsCourse && speed >= CourseAlignmentMinSpeedMps {
		course := math.Atan2(float64(sb.state[3]), float64(sb.state[2]))
		if FoldAxisAngleDeg(float64(axis)-course) > spanSearchHalfWindowDeg {
			return
		}
	}
	for _, e := range set.Edges {
		// Under FacePlaneSpans a face gives the dimension it lies across: an
		// end face the width, a side face the length.
		alongAxis := e.Face.IsLongitudinal() != t.Config.SolidBody.FacePlaneSpans
		if alongAxis {
			// On a merge candidate a span matters only if it can raise the
			// length: the span straight along the axis bounds the search's
			// minimum from above, so when it does not exceed the belief the
			// search is skipped, at a twenty-first of its cost.
			if growing && !spanCouldExceed(nearEdgePoints(cluster), axis, sb.lengthBelief.Estimate()) {
				continue
			}
			if span, ok := minimumAxisSpan(nearEdgePoints(cluster), axis); ok {
				sb.lengthBelief.Observe(span)
			}
		} else if growing {
			continue
		} else if span, ok := minimumAxisSpan(nearEdgePoints(cluster), axis+math.Pi/2); ok {
			sb.widthBelief.Observe(span)
		}
	}
}

// spanCouldExceed says whether the points' trimmed span straight along axisRad
// is longer than belief: an upper bound on what minimumAxisSpan would admit.
func spanCouldExceed(points []l4perception.WorldPoint, axisRad, belief float32) bool {
	if len(points) < DefaultMinFaceSupport {
		return false
	}
	span, ok := trimmedSpan(points, make([]float64, len(points)), math.Cos(float64(axisRad)), math.Sin(float64(axisRad)))
	return ok && span > float64(belief)
}

// growingNotMerging says whether a merge-candidate cluster may still give
// extent evidence under ExtentGrowthAdmission: when it is no wider across the
// body than the believed width, or the class prior's if wider, plus
// extentGrowthLateralMarginMetres.
func (t *Tracker) growingNotMerging(track *TrackedObject, cluster WorldCluster, axis float32) bool {
	if !t.Config.SolidBody.ExtentGrowthAdmission {
		return false
	}
	// One trimmed span straight across the axis, not the minimum over the
	// span window: this only has to tell a body from two side by side, and a
	// second search would double the cost of every merge-candidate frame.
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport {
		return false
	}
	across, ok := trimmedSpan(points, make([]float64, len(points)), -math.Sin(float64(axis)), math.Cos(float64(axis)))
	if !ok {
		return false
	}
	effective, _ := solidBodyClass(track).Effective()
	width := dimensionPriorFor(effective).widthMetres
	if w := track.solidBody.widthBelief.Estimate(); w > width {
		width = w
	}
	return across <= float64(width)+extentGrowthLateralMarginMetres
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
//
// It needs only two order statistics, so it selects them rather than sorting:
// the upper one first, which leaves every value ordered before it in
// scratch[:hi], and then the lower one within that prefix. The values are the
// ones a sort would put at those positions, so the span is the same, at linear
// rather than n log n cost. minimumAxisSpan calls this 21 times per face per
// frame, which made the sort most of a solid body's update cost.
func trimmedSpan(points []l4perception.WorldPoint, scratch []float64, dirX, dirY float64) (float64, bool) {
	for i, p := range points {
		scratch[i] = p.X*dirX + p.Y*dirY
	}
	last := len(scratch) - 1
	lo := int(spanTrimPercent / 100 * float64(last))
	hi := last - lo
	upper := l4perception.NthFloat64(scratch, hi)
	lower := upper
	if lo < hi {
		lower = l4perception.NthFloat64(scratch[:hi], lo)
	}
	span := upper - lower
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
	if sb.tracked {
		// The tracked covariance has had the same inflation.
		sb.syncFromTrack(track)
	} else if inflation > 0 {
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
	if t.Config.SolidBody.FaceHysteresis {
		// A coast is a frame on which no face was seen.
		sb.admitFaces(nil)
	}
	class := solidBodyClass(track)
	t.advanceSolidBodyLifecycle(track, class, true)
	sb.floorAlong, sb.floorAcross = 0, 0
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
	e := SolidBodyEstimate{
		StateModel: StateModelCVCartesianV1,
		Reference:  sb.reference,
		X:          sb.state[0],
		Y:          sb.state[1],
		PositionCovariance: [4]float32{
			sb.p[0*4+0], sb.p[0*4+1],
			sb.p[1*4+0], sb.p[1*4+1],
		},
		Orientation: sb.orientation,
		Length:      t.dimensionOf(sb.lengthBelief, prior.lengthMetres, prior.sigmaMetres, prior.minLengthMetres),
		Width:       t.widthOf(sb, prior),
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
	// Containment's floor: the reported box is never shorter than the points
	// this frame measured along its axes. The beliefs are read above and
	// never see it.
	if sb.floorAlong > e.Length.Metres {
		e.Length.Metres = sb.floorAlong
	}
	if sb.floorAcross > e.Width.Metres {
		e.Width.Metres = sb.floorAcross
	}
	return e
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

// solidBodyOrientation is this frame's orientation belief: the tracked
// heading's, or under CourseHeading, a blend toward the body's course.
//
// The tracked axis is kept, pointed the way the body travels, while it agrees
// with the course to within the span window (10 degrees), the window the
// extent admission already uses to decide whether an axis is believed: it does
// not lag an acceleration as the course does. Beyond the window the belief
// turns toward the course, reaching it at twice the window; and the course
// counts from CourseAlignmentMinSpeedMps, fully at twice that speed. Both are
// ramps rather than switches because the orientation names the faces and sets
// the direction the body centre sits behind them: a switch turned it by the
// whole window in one frame, and moved the centre sideways by half a length
// times that angle.
func (t *Tracker) solidBodyOrientation(track *TrackedObject, sb *solidBodyTrack) OrientationBelief {
	tracked := orientationFromTrack(track, sb.orientation)
	if !t.Config.SolidBody.CourseHeading {
		return tracked
	}
	course, ok := courseOrientation(sb.state, sb.p)
	if !ok {
		return tracked
	}
	if tracked.Provenance == ProvenanceNone {
		return course
	}
	if math.Cos(float64(tracked.PsiRad)-float64(course.PsiRad)) < 0 {
		tracked.PsiRad = float32(math.Remainder(float64(tracked.PsiRad)+math.Pi, 2*math.Pi))
	}
	tracked.AmbiguousModeWeight = 0
	speed := math.Hypot(float64(sb.state[2]), float64(sb.state[3]))
	diff := math.Remainder(float64(course.PsiRad)-float64(tracked.PsiRad), 2*math.Pi)
	w := ramp(math.Abs(diff)*180/math.Pi, spanSearchHalfWindowDeg) *
		ramp(speed, CourseAlignmentMinSpeedMps)
	if w == 0 {
		return tracked
	}
	return OrientationBelief{
		PsiRad:       float32(math.Remainder(float64(tracked.PsiRad)+w*diff, 2*math.Pi)),
		VarianceRad2: float32((1-w)*float64(tracked.VarianceRad2) + w*float64(course.VarianceRad2)),
		Provenance:   ProvenanceObserved,
	}
}

// ramp is zero up to threshold, one from twice it, and linear between.
func ramp(x, threshold float64) float64 {
	return math.Min(math.Max(x/threshold-1, 0), 1)
}

// courseOrientation is the direction of travel as an orientation belief, and
// false below CourseAlignmentMinSpeedMps. Travel resolves the direction, so
// the belief is unambiguous. Its variance is the velocity's variance across
// the course over the speed squared: the angle a one-sigma sideways error in
// the velocity turns it through, to first order.
func courseOrientation(state [4]float32, p [16]float32) (OrientationBelief, bool) {
	vx, vy := float64(state[2]), float64(state[3])
	speed := math.Hypot(vx, vy)
	if speed < CourseAlignmentMinSpeedMps {
		return OrientationBelief{}, false
	}
	nx, ny := -vy/speed, vx/speed
	across := nx*nx*float64(p[2*4+2]) + 2*nx*ny*float64(p[2*4+3]) + ny*ny*float64(p[3*4+3])
	if !(across >= 0) {
		across = 0
	}
	return OrientationBelief{
		PsiRad:       float32(math.Atan2(vy, vx)),
		VarianceRad2: float32(across / (speed * speed)),
		Provenance:   ProvenanceObserved,
	}, true
}

// dimensionOf is dimensionFromBelief under the tracker's options: with
// ExtentPriorFloor, an accumulated extent shorter than the class prior leaves
// the prior standing; with VehicleExtentFloor, one shorter than the class's
// smallest member is raised to it.
func (t *Tracker) dimensionOf(b extentBelief, priorMetres, priorSigma, minimumMetres float32) DimensionBelief {
	d := dimensionFromBelief(b, priorMetres, priorSigma)
	if d.Provenance != ProvenanceAccumulated {
		return d
	}
	if t.Config.SolidBody.ExtentPriorFloor && d.Metres < priorMetres {
		return DimensionBelief{Metres: priorMetres, SigmaMetres: priorSigma, Provenance: ProvenanceClassPrior}
	}
	if t.Config.SolidBody.VehicleExtentFloor && d.Metres < minimumMetres {
		d.Metres = minimumMetres
	}
	return d
}

// widthOf is the body's width under the tracker's options. Under
// VehicleExtentFloor a body of no known smallest size that has already been
// seen as long as the smallest road car is held at that car's width too: by
// then it is a vehicle whatever its class says, and a ten-metre truck seen
// across a sliver of its rear is not 0.4 m wide. A cyclist, at under 2 m,
// is not reached.
func (t *Tracker) widthOf(sb *solidBodyTrack, prior classDimensionPrior) DimensionBelief {
	minimum := prior.minWidthMetres
	if vehicle := dimensionPriorFor(MotionRigidVehicle); minimum == 0 &&
		t.Config.SolidBody.VehicleExtentFloor && sb.lengthBelief.Estimate() >= vehicle.minLengthMetres {
		minimum = vehicle.minWidthMetres
	}
	return t.dimensionOf(sb.widthBelief, prior.widthMetres, prior.sigmaMetres, minimum)
}
