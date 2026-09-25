package l5tracks

// The solid-body estimate: the contract headway consumes, per Section 5.4 of
// docs/plans/lidar-state-estimation-plan.md.
//
// The distinction this type exists to enforce is that a partial point envelope
// constrains the body but is not the body. A cluster is what the sensor saw
// this frame; the solid body persists across changing visible faces and a
// bounded occlusion. Neither a classifier label, the current OBB, nor a
// visually smooth trail can substitute for it, because none of them carries the
// uncertainty and provenance a projected bumper has to be bounded with.
//
// Every believed quantity here records where it came from, so that "we measured
// this" is never silently interchangeable with "the class prior says this".

import (
	"fmt"
	"math"
)

// StateModelCVCartesianV1 names the dynamic state layout this estimate was
// produced with: four elements [x, y, vx, vy] and a 4x4 covariance.
//
// It is persisted alongside the covariance blob so that a reader never infers
// dimensionality from the array length. Section 5.4 is explicit about this: if
// Option B is adopted the state becomes six-element with a 6x6 covariance, and
// a reader that guessed from length would silently misread every historical
// row at the migration.
const StateModelCVCartesianV1 = "cv_cartesian_v1"

// Provenance records where a believed value came from. The zero value means
// there is no value yet, which is deliberately distinct from a value of zero.
type Provenance uint8

const (
	// ProvenanceNone means nothing has supplied this field.
	ProvenanceNone Provenance = iota
	// ProvenanceClassPrior means the value is the motion class's prior, not
	// evidence about this object. Section 5.6: dimensions must be marked as
	// the prior rather than as a measurement until admissibility accepts a
	// frame.
	ProvenanceClassPrior
	// ProvenanceAccumulated means the value was built from evidence across
	// frames, typically lower-bound spans, rather than measured outright.
	ProvenanceAccumulated
	// ProvenanceObserved means the value was measured from a frame admitted
	// as a two-sided measurement.
	ProvenanceObserved
)

// String names a provenance for diagnostics and persisted rows.
func (p Provenance) String() string {
	switch p {
	case ProvenanceClassPrior:
		return "class_prior"
	case ProvenanceAccumulated:
		return "accumulated"
	case ProvenanceObserved:
		return "observed"
	default:
		return "none"
	}
}

// IsEvidence reports whether the value rests on observation of this object at
// all, as opposed to a class prior or nothing.
func (p Provenance) IsEvidence() bool {
	return p == ProvenanceAccumulated || p == ProvenanceObserved
}

// EstimationState is the estimation lifecycle of Section 5.6, which answers
// "do we believe the physical state yet?".
//
// This is a different question from the track lifecycle's "should this track
// exist?", and conflating them is what lets a biased initialisation seed become
// a confident physical prior.
type EstimationState uint8

const (
	// EstimationInitialising means a track exists but the physical pose is not
	// believed. Consumable by association only.
	EstimationInitialising EstimationState = iota
	// EstimationGeometryConverging means motion is credible while dimensions
	// and orientation are still moving. Motion-only metrics, flagged
	// provisional.
	EstimationGeometryConverging
	// EstimationEstablished means pose, orientation and dimensions are all
	// within convergence bounds.
	EstimationEstablished
	// EstimationTemporarilyDegraded means it was established and the evidence
	// has become inadequate: occlusion, fragmentation, sustained high NIS.
	EstimationTemporarilyDegraded
	// EstimationModelInvalid means the object model may no longer describe
	// this object. Evidence only, no pose.
	EstimationModelInvalid
)

// String names an estimation state for diagnostics and persisted rows.
func (s EstimationState) String() string {
	switch s {
	case EstimationInitialising:
		return "initialising"
	case EstimationGeometryConverging:
		return "geometry_converging"
	case EstimationEstablished:
		return "established"
	case EstimationTemporarilyDegraded:
		return "temporarily_degraded"
	case EstimationModelInvalid:
		return "model_invalid"
	default:
		return "unknown"
	}
}

// AllowsReportedPose reports whether a consumer may publish this estimate's
// pose. Two states forbid it outright, and for opposite reasons: initialising
// has not earned belief yet, and model_invalid has lost it.
func (s EstimationState) AllowsReportedPose() bool {
	return s == EstimationGeometryConverging ||
		s == EstimationEstablished ||
		s == EstimationTemporarilyDegraded
}

// AllowsBehaviourMetric reports whether a behaviour metric may be derived.
// geometry_converging permits motion-only metrics, which MetricsAreProvisional
// requires be flagged as such.
func (s EstimationState) AllowsBehaviourMetric() bool {
	return s == EstimationGeometryConverging || s == EstimationEstablished
}

// MetricsAreProvisional reports whether anything derived must be labelled as
// not yet dependable. A coasted state is marked the same way.
func (s EstimationState) MetricsAreProvisional() bool {
	return s == EstimationGeometryConverging || s == EstimationTemporarilyDegraded
}

// MotionClass is the motion-model taxonomy of Section 5.5. The estimation
// framework and data semantics are common across classes; only the priors vary.
//
// The mapping from classifier labels is deliberately coarse: the classifier
// cannot reliably separate car from truck, and the motion priors for the two
// barely differ. Four classes is the granularity the evidence supports.
type MotionClass uint8

const (
	// MotionUnknown carries weak priors and broad bounds, and is deliberately
	// worse at producing clean paths than a committed class would be.
	MotionUnknown MotionClass = iota
	// MotionRigidVehicle is car, truck, bus: rigid, predominantly forward,
	// lateral velocity strongly penalised, heading strongly coupled to
	// velocity direction.
	MotionRigidVehicle
	// MotionTwoWheeler is cyclist and motorcyclist: forward-biased but more
	// weakly, with more lateral freedom and expected low-speed instability.
	MotionTwoWheeler
	// MotionPedestrian is approximately holonomic: may stop abruptly, turn in
	// place, step sideways or reverse.
	MotionPedestrian
)

// String names a motion class for diagnostics and persisted rows.
func (m MotionClass) String() string {
	switch m {
	case MotionRigidVehicle:
		return "rigid_vehicle"
	case MotionTwoWheeler:
		return "two_wheeler"
	case MotionPedestrian:
		return "pedestrian"
	default:
		return "unknown"
	}
}

// HeadingFollowsVelocity reports whether velocity direction may be used to
// resolve the heading's 180-degree ambiguity.
//
// False for pedestrians, and that is the whole point of the class existing: a
// pedestrian's heading is not equivalent to their velocity direction, and
// inferring one from the other makes ordinary sideways stepping look like a
// tracking failure.
func (m MotionClass) HeadingFollowsVelocity() bool {
	return m == MotionRigidVehicle || m == MotionTwoWheeler
}

// MotionClassForLabel maps a classifier label to a motion class.
//
// The argument is the label string rather than l6objects.ObjectClass because
// l6objects depends on this package; taking the typed class would invert that.
// An unrecognised label is MotionUnknown, which is the safe direction: weak
// priors rather than confidently wrong ones.
func MotionClassForLabel(label string) MotionClass {
	switch label {
	case "car", "truck", "bus":
		return MotionRigidVehicle
	case "cyclist", "motorcyclist":
		return MotionTwoWheeler
	case "pedestrian":
		return MotionPedestrian
	default:
		// "dynamic", "bird", "noise", "" and anything unrecognised.
		return MotionUnknown
	}
}

// MotionClassBelief is the class posterior, kept as a belief rather than a
// label so that classification uncertainty cannot silently become motion
// certainty. Section 5.5 enforces that with two rules, both applied by
// Effective.
type MotionClassBelief struct {
	// Class is the most probable motion class and Posterior its probability
	// in [0, 1].
	Class     MotionClass
	Posterior float32
	// Runner is the next most probable class, needed because a posterior split
	// between classes with different motion models is not the same situation as
	// a confident one, even at the same argmax.
	Runner          MotionClass
	RunnerPosterior float32
}

// minPriorSeparation is how far ahead the leading class must be before its
// priors are used at all. Below it the posterior is genuinely split and the
// weaker model is used instead.
const minPriorSeparation = 0.15

// Effective returns the motion class to run with and the strength its priors
// should be applied at, in [0, 1].
//
// Rule 1: prior strength scales with the posterior, so at low confidence the
// priors relax toward unknown rather than snapping to the argmax class.
//
// Rule 2: when the posterior is genuinely split between classes with different
// motion models — cyclist against pedestrian at walking pace is the case that
// motivates it — the track runs with the weaker of the two rather than
// picking a winner on a small margin.
func (b MotionClassBelief) Effective() (MotionClass, float32) {
	strength := b.Posterior
	if strength < 0 {
		strength = 0
	}
	if strength > 1 {
		strength = 1
	}

	class := b.Class
	if b.Runner != b.Class &&
		b.RunnerPosterior > 0 &&
		b.Posterior-b.RunnerPosterior < minPriorSeparation {
		class = weakerMotionClass(b.Class, b.Runner)
	}
	if class == MotionUnknown {
		// Unknown has no priors to strengthen.
		return MotionUnknown, 0
	}
	return class, strength
}

// weakerMotionClass returns whichever of two classes constrains motion less,
// so that a split posterior errs toward admitting motion rather than refusing
// it. Ordering: unknown is weakest, then pedestrian (holonomic), then
// two_wheeler, then rigid_vehicle.
func weakerMotionClass(a, b MotionClass) MotionClass {
	rank := func(m MotionClass) int {
		switch m {
		case MotionUnknown:
			return 0
		case MotionPedestrian:
			return 1
		case MotionTwoWheeler:
			return 2
		case MotionRigidVehicle:
			return 3
		}
		return 0
	}
	if rank(a) <= rank(b) {
		return a
	}
	return b
}

// OrientationBelief is body orientation, held outside the dynamic state so the
// motion filter stays linear and orientation keeps its own dynamics and its own
// failure mode at low speed.
//
// The 180-degree ambiguity is represented explicitly as a bimodal belief rather
// than resolved by a guard. It collapses when velocity direction or an
// asymmetric geometric cue resolves it; for a pedestrian it may never collapse,
// and that is a correct outcome rather than a missing feature.
type OrientationBelief struct {
	PsiRad       float32
	VarianceRad2 float32
	// AmbiguousModeWeight is the weight on the psi+pi mode, in [0, 0.5]. Zero
	// means resolved: one mode carries everything.
	AmbiguousModeWeight float32
	Provenance          Provenance
}

// IsResolved reports whether the direction ambiguity has collapsed.
func (o OrientationBelief) IsResolved() bool {
	return o.Provenance != ProvenanceNone && o.AmbiguousModeWeight == 0
}

// Modes returns the orientation's candidate headings: one when resolved, two
// half a turn apart when not. A consumer that renders or projects must handle
// both rather than silently taking the first.
func (o OrientationBelief) Modes() []float32 {
	if o.Provenance == ProvenanceNone {
		return nil
	}
	if o.AmbiguousModeWeight == 0 {
		return []float32{o.PsiRad}
	}
	return []float32{o.PsiRad, wrapToPi(o.PsiRad + math.Pi)}
}

// ResolveWith collapses the ambiguity using a reference direction, which must
// come from a cue the motion class admits: velocity direction for a vehicle or
// two-wheeler, an asymmetric geometric cue otherwise. The mode nearer the
// reference wins.
//
// It is a no-op on a belief with no value, so a caller cannot manufacture an
// orientation out of a reference alone.
func (o *OrientationBelief) ResolveWith(referenceRad float32) {
	if o.Provenance == ProvenanceNone {
		return
	}
	if math.Abs(float64(angleDifference(o.PsiRad, referenceRad))) > math.Pi/2 {
		o.PsiRad = wrapToPi(o.PsiRad + math.Pi)
	}
	o.AmbiguousModeWeight = 0
}

// DimensionBelief is one body dimension with its uncertainty and the count of
// frames admitted as evidence for it.
//
// Section 9.2.1 admits evidence per dimension rather than per frame: a frame
// may be admissible for width and inadmissible for length in the same instant,
// so each dimension carries its own count.
type DimensionBelief struct {
	Metres           float32
	SigmaMetres      float32
	AdmissibleFrames int
	Provenance       Provenance
}

// IsConverged reports whether the dimension is believed well enough for the
// established state, per the entry evidence in Section 5.6.
//
// A class prior never counts as converged however tight its sigma: the bound is
// about evidence concerning this object, and a prior is evidence about its
// class.
func (d DimensionBelief) IsConverged(maxSigmaMetres float32, minFrames int) bool {
	if !d.Provenance.IsEvidence() || d.Metres <= 0 {
		return false
	}
	return d.SigmaMetres <= maxSigmaMetres && d.AdmissibleFrames >= minFrames
}

// ReferencePoint names the physical point an estimate's planar position refers
// to. It is part of the contract because a pose is meaningless without it: the
// same object reported at its body centre and at its near face differ by half
// its width, which is the size of the defect this plan exists to fix.
type ReferencePoint uint8

const (
	// ReferenceUnknown means the pose has no named physical meaning and must
	// not be consumed as one.
	ReferenceUnknown ReferencePoint = iota
	// ReferenceBodyCentre is the centre of the believed body.
	ReferenceBodyCentre
	// ReferenceNearFaceCentre is the centre of the nearest observed face,
	// which is what an edge measurement directly constrains.
	ReferenceNearFaceCentre
	// ReferenceClusterMedoid is the initialisation seed: a point in the
	// observed cluster, carrying the medoid's known bias toward the sensor.
	ReferenceClusterMedoid
)

// String names a reference point for diagnostics and persisted rows.
func (r ReferencePoint) String() string {
	switch r {
	case ReferenceBodyCentre:
		return "body_centre"
	case ReferenceNearFaceCentre:
		return "near_face_centre"
	case ReferenceClusterMedoid:
		return "cluster_medoid"
	default:
		return "unknown"
	}
}

// IsPhysical reports whether the reference point denotes a place on the body
// rather than an artefact of what the sensor happened to see.
func (r ReferencePoint) IsPhysical() bool {
	return r == ReferenceBodyCentre || r == ReferenceNearFaceCentre
}

// SupportState records what the current frame's evidence actually consisted of,
// so a consumer can tell a well-observed estimate from a coasted one without
// inferring it from smoothness.
type SupportState struct {
	// PointCount is the number of returns supporting this frame's update.
	PointCount int
	// CoastedFrames is how many consecutive frames have had no accepted
	// measurement. Zero means this frame was observed.
	CoastedFrames int
	// Instant is the tracker's support token for this instant, the behaviour
	// plan's Section 7.3 vocabulary (see ObservationSupport). It carries the
	// explanation of an absence, which CoastedFrames cannot; zero when the
	// estimate was not read off a live track.
	Instant ObservationSupport
	// Fragmented and Truncated mark evidence that Section 9.2.1 refuses as
	// dimension evidence: a fragment's extent is meaningless as a dimension,
	// and a cluster cut off at the field-of-view boundary has an extent that
	// is an artefact of the sensor.
	Fragmented bool
	Truncated  bool
}

// IsObserved reports whether this frame had its own measurement.
func (s SupportState) IsObserved() bool { return s.CoastedFrames == 0 }

// EstimateStage distinguishes the live estimate from a revised one, so that a
// report and a live view disagreeing is explainable rather than a mystery.
type EstimateStage uint8

const (
	// StageLive is the filtered estimate as of the last frame.
	StageLive EstimateStage = iota
	// StageSmoothed is a retrospective estimate that used later frames.
	StageSmoothed
)

// String names an estimate stage for diagnostics and persisted rows.
func (s EstimateStage) String() string {
	if s == StageSmoothed {
		return "smoothed"
	}
	return "live"
}

// SolidBodyEstimate is the contract of Section 5.4: dynamic position together
// with the beliefs held outside the dynamic state, each with its own
// uncertainty and provenance.
type SolidBodyEstimate struct {
	// StateModel is the dynamic state layout discriminator. See
	// StateModelCVCartesianV1.
	StateModel string

	// Reference names what X and Y refer to on the body.
	Reference ReferencePoint
	X, Y      float32
	// PositionCovariance is the planar 2x2 position block, row-major:
	// [xx, xy, yx, yy]. Extracted from the dynamic covariance rather than
	// re-derived, so it cannot drift from the filter's own belief.
	PositionCovariance [4]float32

	Orientation OrientationBelief

	Length, Width, Height DimensionBelief

	// GroundZ is the believed vertical position of the object's ground
	// contact, and GroundSurfaceModel names the surface it was resolved
	// against. Vertical motion here is road grade, not object dynamics.
	GroundZ            float32
	GroundSurfaceModel string

	Motion     MotionClassBelief
	Estimation EstimationState
	Stage      EstimateStage

	LastObservedUnixNanos int64
	Support               SupportState
}

// BodySurface names a face of the body that a consumer may want projected.
type BodySurface uint8

const (
	// SurfaceFront is the leading face along the body's own orientation.
	SurfaceFront BodySurface = iota
	// SurfaceRear is the trailing face.
	SurfaceRear
)

// String names a body surface for diagnostics.
func (s BodySurface) String() string {
	if s == SurfaceRear {
		return "rear"
	}
	return "front"
}

// SurfaceProjection is a projected body surface: where it is believed to be,
// and how tightly that is bounded.
//
// The bound is the point of the type. Projecting a bumper means combining pose,
// orientation and a dimension, each uncertain, so a projection without a bound
// would be a confident-looking number with no basis. It is never produced
// without one.
type SurfaceProjection struct {
	Surface BodySurface
	X, Y    float32
	// SigmaMetres is the one-sigma bound along the projection direction,
	// combining position, orientation and dimension uncertainty.
	SigmaMetres float32
	// FromClassPrior is true when the dimension used was the class prior
	// rather than evidence about this object. The projection is still bounded,
	// but a consumer should know the extent was assumed.
	FromClassPrior bool
}

// ProjectSurface projects a named surface of the body onto its own orientation.
//
// The second return is false when the joint bound is unavailable, and a caller
// must then suppress rather than substitute: Section 5.4 requires that headway
// be suppressed when pose, heading and dimensions cannot be bounded together.
// Unavailable means any of:
//
//   - the estimation state forbids a reported pose;
//   - the reference point is not a physical place on the body;
//   - the orientation is unresolved, so front and rear are interchangeable;
//   - the length has no value at all, not even a prior;
//   - the position covariance is absent or not positive.
//
// A class-prior length does not make it unavailable. It makes the projection
// wider and flags it, which is a weaker claim rather than no claim.
func (e SolidBodyEstimate) ProjectSurface(s BodySurface) (SurfaceProjection, bool) {
	if !e.Estimation.AllowsReportedPose() {
		return SurfaceProjection{}, false
	}
	if !e.Reference.IsPhysical() {
		return SurfaceProjection{}, false
	}
	if !e.Orientation.IsResolved() {
		return SurfaceProjection{}, false
	}
	if e.Length.Provenance == ProvenanceNone || e.Length.Metres <= 0 {
		return SurfaceProjection{}, false
	}
	posVar := e.positionVarianceAlong(e.Orientation.PsiRad)
	if posVar <= 0 || math.IsNaN(float64(posVar)) || math.IsInf(float64(posVar), 0) {
		return SurfaceProjection{}, false
	}

	half := e.Length.Metres / 2
	if s == SurfaceRear {
		half = -half
	}
	cos := float32(math.Cos(float64(e.Orientation.PsiRad)))
	sin := float32(math.Sin(float64(e.Orientation.PsiRad)))

	// Half the length's sigma, since it is the half-length that is projected.
	dimVar := (e.Length.SigmaMetres / 2) * (e.Length.SigmaMetres / 2)
	// An orientation error of sigma swings the projected point by roughly
	// half-length times that angle, perpendicular to the body axis. Taken
	// along the projection direction it is a second-order term, but it is the
	// term that grows with the body, so it is retained rather than dropped.
	angVar := (half * half) * e.Orientation.VarianceRad2

	return SurfaceProjection{
		Surface:        s,
		X:              e.X + half*cos,
		Y:              e.Y + half*sin,
		SigmaMetres:    float32(math.Sqrt(float64(posVar + dimVar + angVar))),
		FromClassPrior: e.Length.Provenance == ProvenanceClassPrior,
	}, true
}

// positionVarianceAlong is the position variance resolved along a heading,
// u'Pu for the unit vector u at that angle. Using the full 2x2 block rather
// than a diagonal element keeps the cross term, which is exactly the
// correlation a projected bumper's bound depends on.
func (e SolidBodyEstimate) positionVarianceAlong(psiRad float32) float32 {
	c := math.Cos(float64(psiRad))
	s := math.Sin(float64(psiRad))
	xx := float64(e.PositionCovariance[0])
	xy := float64(e.PositionCovariance[1])
	yx := float64(e.PositionCovariance[2])
	yy := float64(e.PositionCovariance[3])
	return float32(c*c*xx + c*s*(xy+yx) + s*s*yy)
}

// String renders the estimate for diagnostics, leading with what a reader needs
// in order to know whether to trust it.
func (e SolidBodyEstimate) String() string {
	return fmt.Sprintf("%s %s pose=(%.2f,%.2f)@%s psi=%.3f[%s] L=%.2f+-%.2f[%s] class=%s stage=%s",
		e.StateModel, e.Estimation, e.X, e.Y, e.Reference,
		e.Orientation.PsiRad, e.Orientation.Provenance,
		e.Length.Metres, e.Length.SigmaMetres, e.Length.Provenance,
		e.Motion.Class, e.Stage)
}

// wrapToPi folds an angle into [-pi, pi].
func wrapToPi(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// angleDifference is the shortest signed angle from b to a, in [-pi, pi].
func angleDifference(a, b float32) float32 {
	return wrapToPi(a - b)
}
