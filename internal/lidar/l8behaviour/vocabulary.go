package l8behaviour

// Closed vocabularies for behaviour results, per Sections 7.2, 7.3, 8.3, 9.3
// and 10.2 of docs/plans/lidar-behaviour-analytics-plan.md.
//
// Every vocabulary here shares one shape: a uint8 whose zero value means
// "unspecified", a names table that is the single source of truth for the wire
// token of each value, and text marshalling that refuses to write an
// unspecified or out-of-range value. That last rule is the point. A field a
// caller forgot to set must fail to serialise rather than reach storage as a
// plausible-looking default, which is how a missing support state would
// otherwise become "observed" and a missing stage become "final".
//
// Numeric values are not a wire format. Only the snake-case tokens are
// persisted or sent, so a value may be inserted mid-list without migrating
// anything. SuppressionReason relies on that: its declaration order is its
// reporting precedence.
//
// Every token is registered in docs/lidar/architecture/label-vocabulary.md
// (Visibility in docs/platform/architecture/metrics-registry.md), and a test
// fails when a token is missing from its registry.

import "fmt"

// vocabulary is the names table behind one closed vocabulary. names[0] is the
// unspecified slot and is never a valid token.
type vocabulary[T ~uint8] struct {
	kind  string
	names []string
}

func (v vocabulary[T]) valid(x T) bool {
	return x != 0 && int(x) < len(v.names)
}

func (v vocabulary[T]) name(x T) string {
	if x == 0 {
		return "unspecified"
	}
	if int(x) < len(v.names) {
		return v.names[x]
	}
	return fmt.Sprintf("%s(%d)", v.kind, uint8(x))
}

func (v vocabulary[T]) parse(s string) (T, error) {
	for i := 1; i < len(v.names); i++ {
		if v.names[i] == s {
			return T(i), nil
		}
	}
	return 0, fmt.Errorf("unknown %s %q", v.kind, s)
}

func (v vocabulary[T]) marshal(x T) ([]byte, error) {
	if !v.valid(x) {
		return nil, fmt.Errorf("cannot serialise %s %s", v.kind, v.name(x))
	}
	return []byte(v.names[x]), nil
}

func (v vocabulary[T]) unmarshal(dst *T, b []byte) error {
	x, err := v.parse(string(b))
	if err != nil {
		return err
	}
	*dst = x
	return nil
}

func (v vocabulary[T]) values() []T {
	out := make([]T, 0, len(v.names)-1)
	for i := 1; i < len(v.names); i++ {
		out = append(out, T(i))
	}
	return out
}

// --- Suppression reasons ---------------------------------------------------

// SuppressionReason says why a metric carries no publishable value. Section
// 7.2: a suppressed metric is a first-class stored result, and the per-site
// rate of each reason is a diagnostic in its own right.
//
// The declaration order is the reporting precedence: when several reasons
// apply, the earliest is the one a Measurement carries. The order runs from
// "the metric is undefined here", through "the estimator disowns the state"
// and "nothing was seen", to geometry, uncertainty and value conditions, and
// ends with publication stage. Stage is last on purpose: a provisional run over
// online estimates still shows the physical suppression mix underneath, rather
// than reporting estimate_not_final for every instant and hiding it.
type SuppressionReason uint8

const (
	// ReasonUnspecified is the zero value and never a valid reason.
	ReasonUnspecified SuppressionReason = iota
	// ReasonClassNotSupported: the metric is not defined for this motion
	// class. Section 7.2.
	ReasonClassNotSupported
	// ReasonMetricNotObservable: structurally unobservable at this sample
	// rate, for example jerk on a short passage. Section 7.2.
	ReasonMetricNotObservable
	// ReasonInteractionTypeUncertain: interaction classification confidence
	// is below the metric's bound. Sections 7.2 and 7.5.
	ReasonInteractionTypeUncertain
	// ReasonModelDegraded: the estimator reported model_invalid or
	// temporarily_degraded for a contributing track. Section 7.2.
	ReasonModelDegraded
	// ReasonInsufficientObservation: too few observed frames, the passage is
	// shorter than the metric's minimum support, or the pose is not yet
	// believed (initialising, or a non-physical reference point). Section 7.2.
	ReasonInsufficientObservation
	// ReasonNotObserved: a contributing track, or a surface the metric
	// requires, was not directly observed at this instant. Coasting is not
	// observation (Sections 9.1 and 9.2); the instant is review-only and never
	// enters an exposure denominator.
	ReasonNotObserved
	// ReasonNoCommonPath: the pair does not share the relation the metric
	// assumes. Section 7.2.
	ReasonNoCommonPath
	// ReasonAmbiguousLeader: the follower has more than one credible leader,
	// or leader/follower order is not stable. Section 8.3 suppresses ordering
	// ambiguity rather than choosing the argmax.
	ReasonAmbiguousLeader
	// ReasonRoadGeometryUnavailable: no road-surface model for the traversed
	// region. Section 7.2.
	ReasonRoadGeometryUnavailable
	// ReasonLaneGeometryUnavailable: no lane centreline or edges. Section 7.2.
	ReasonLaneGeometryUnavailable
	// ReasonPlanarFallbackInsufficient: computed under a planar assumption on
	// a graded site, where the grade error dominates. Section 7.2.
	ReasonPlanarFallbackInsufficient
	// ReasonOrientationUnresolved: the body's front/rear direction is not
	// resolved, so a physical endpoint cannot be named. Section 8.3.
	ReasonOrientationUnresolved
	// ReasonExtentNotConverged: a required dimension belief is absent, has
	// not met its admissibility count, or is a class prior. Sections 7.2 and
	// 9.1; ProjectBody reports it for a missing length or width belief.
	ReasonExtentNotConverged
	// ReasonTrajectoryUncertaintyTooHigh: propagated uncertainty exceeds the
	// metric's usable bound. Section 7.2.
	ReasonTrajectoryUncertaintyTooHigh
	// ReasonNonPositiveGap: the endpoint gap is zero or negative. Section 8.3
	// requires an overlap/geometry review, never an automatic zero-headway or
	// collision claim.
	ReasonNonPositiveGap
	// ReasonBelowSpeedFloor: the follower's along-path speed is below the
	// calibrated floor, where net time gap is undefined or unstable. Sections
	// 8.3 and 9.1: time gap is undefined at rest, not infinite.
	ReasonBelowSpeedFloor
	// ReasonEstimateNotFinal: derived from an online or fixed-lag estimate.
	// Production results read final estimates only (Section 2, G-SMO-1); the
	// value is review-only.
	ReasonEstimateNotFinal
	suppressionReasonEnd
)

var suppressionReasons = vocabulary[SuppressionReason]{
	kind: "suppression reason",
	names: []string{
		"",
		"class_not_supported",
		"metric_not_observable",
		"interaction_type_uncertain",
		"model_degraded",
		"insufficient_observation",
		"not_observed",
		"no_common_path",
		"ambiguous_leader",
		"road_geometry_unavailable",
		"lane_geometry_unavailable",
		"planar_fallback_insufficient",
		"orientation_unresolved",
		"extent_not_converged",
		"trajectory_uncertainty_too_high",
		"non_positive_gap",
		"below_speed_floor",
		"estimate_not_final",
	},
}

func (r SuppressionReason) String() string { return suppressionReasons.name(r) }

// Valid reports whether r is a registered reason.
func (r SuppressionReason) Valid() bool { return suppressionReasons.valid(r) }

// MarshalText writes the registered token and refuses an unspecified reason.
func (r SuppressionReason) MarshalText() ([]byte, error) { return suppressionReasons.marshal(r) }

// UnmarshalText accepts registered tokens only.
func (r *SuppressionReason) UnmarshalText(b []byte) error {
	return suppressionReasons.unmarshal(r, b)
}

// ParseSuppressionReason parses a registered token.
func ParseSuppressionReason(s string) (SuppressionReason, error) { return suppressionReasons.parse(s) }

// SuppressionReasons lists every reason in precedence order.
func SuppressionReasons() []SuppressionReason { return suppressionReasons.values() }

// --- Observation support ---------------------------------------------------

// SupportState is what one sampled instant of a track rests on, per Section
// 7.3. Not every missing frame is an occlusion, and conflating them corrupts
// exposure denominators: occluded_inferred is a property of the scene and is
// expected, missed_unknown is a property of the pipeline and is a defect signal.
type SupportState uint8

const (
	// SupportUnspecified is the zero value and never valid.
	SupportUnspecified SupportState = iota
	// SupportObserved: a detection was associated at this instant.
	SupportObserved
	// SupportCoasted: the estimator propagated without a measurement.
	SupportCoasted
	// SupportOccludedInferred: missing, and another object's geometry explains
	// the absence.
	SupportOccludedInferred
	// SupportMissedUnknown: missing with no explanation.
	SupportMissedUnknown
	// SupportClusterMerged: present but merged with another object.
	SupportClusterMerged
	// SupportClusterSplit: present but fragmented across clusters.
	SupportClusterSplit
	// SupportOutOfFOV: geometrically outside the sensor's coverage.
	SupportOutOfFOV
	supportStateEnd
)

var supportStates = vocabulary[SupportState]{
	kind: "support state",
	names: []string{
		"",
		"observed",
		"coasted",
		"occluded_inferred",
		"missed_unknown",
		"cluster_merged",
		"cluster_split",
		"out_of_fov",
	},
}

func (s SupportState) String() string { return supportStates.name(s) }

// Valid reports whether s is a registered support state.
func (s SupportState) Valid() bool { return supportStates.valid(s) }

// MarshalText writes the registered token and refuses an unspecified state.
func (s SupportState) MarshalText() ([]byte, error) { return supportStates.marshal(s) }

// UnmarshalText accepts registered tokens only.
func (s *SupportState) UnmarshalText(b []byte) error { return supportStates.unmarshal(s, b) }

// ParseSupportState parses a registered token.
func ParseSupportState(s string) (SupportState, error) { return supportStates.parse(s) }

// SupportStates lists every support state.
func SupportStates() []SupportState { return supportStates.values() }

// CountsTowardExposure reports whether time in this state may enter an
// exposure or opportunity denominator. Only observed time does: a vehicle
// hidden behind a van for six frames must not contribute six frames of
// fictitious following exposure (Section 9.2).
func (s SupportState) CountsTowardExposure() bool { return s == SupportObserved }

// IsDefectSignal reports whether this state is evidence of a pipeline defect
// rather than a property of the scene. Section 7.3 flags missed_unknown for
// exactly this reason; out_of_fov is explicitly not a failure.
func (s SupportState) IsDefectSignal() bool { return s == SupportMissedUnknown }

// supportSeverity ranks support states for WorseSupport: less direct evidence
// about the body is worse, and among absences an unexplained one is worse
// than an explained one. A present but fragmented body still has returns; a
// merged one has returns contaminated by another object; an occluded or
// out-of-view body has none, for a stated reason; a coasted one has none and
// no recorded reason; a missed one is a defect signal. Declaration order is
// not this order, and must not be read as it.
var supportSeverity = [supportStateEnd]int{
	SupportObserved:         1,
	SupportClusterSplit:     2,
	SupportClusterMerged:    3,
	SupportOccludedInferred: 4,
	SupportOutOfFOV:         5,
	SupportCoasted:          6,
	SupportMissedUnknown:    7,
}

// WorseSupport returns whichever of two support states rests on weaker
// evidence, per supportSeverity; an unspecified state never wins. An
// interaction records the worst support of either party over its interval
// (Section 10.2) with it.
func WorseSupport(a, b SupportState) SupportState {
	if !b.Valid() {
		return a
	}
	if !a.Valid() || supportSeverity[b] > supportSeverity[a] {
		return b
	}
	return a
}

// --- Estimate stage --------------------------------------------------------

// EstimateStage names which estimator output a state came from, per Section
// 10.1 of docs/plans/lidar-state-estimation-plan.md. The tokens match the
// persisted lidar_track_estimates.stage column, which the online sink writes
// as "online".
type EstimateStage uint8

const (
	// StageUnspecified is the zero value and never valid.
	StageUnspecified EstimateStage = iota
	// StageOnline is the filtered estimate as of its own frame: association
	// and the live view.
	StageOnline
	// StageFixedLag is the persisted per-frame record after a bounded lag:
	// comparison and provisional inspection.
	StageFixedLag
	// StageFinal is the full-track estimate at track close: production
	// behaviour, reports and public analysis.
	StageFinal
	estimateStageEnd
)

var estimateStages = vocabulary[EstimateStage]{
	kind:  "estimate stage",
	names: []string{"", "online", "fixed_lag", "final"},
}

func (s EstimateStage) String() string { return estimateStages.name(s) }

// Valid reports whether s is a registered stage.
func (s EstimateStage) Valid() bool { return estimateStages.valid(s) }

// MarshalText writes the registered token and refuses an unspecified stage.
func (s EstimateStage) MarshalText() ([]byte, error) { return estimateStages.marshal(s) }

// UnmarshalText accepts registered tokens only.
func (s *EstimateStage) UnmarshalText(b []byte) error { return estimateStages.unmarshal(s, b) }

// ParseEstimateStage parses a registered token, including the persisted
// lidar_track_estimates.stage value.
func ParseEstimateStage(s string) (EstimateStage, error) { return estimateStages.parse(s) }

// EstimateStages lists every stage, least final first.
func EstimateStages() []EstimateStage { return estimateStages.values() }

// --- Estimation lifecycle --------------------------------------------------

// EstimationState is the estimation lifecycle of Section 5.6 of the
// state-estimation plan: "do we believe the physical state yet?". The tokens
// are l5tracks.EstimationState's; this type exists so they serialise as tokens.
type EstimationState uint8

const (
	// EstimationUnspecified is the zero value and never valid.
	EstimationUnspecified EstimationState = iota
	// EstimationInitialising: the physical pose is not yet believed.
	EstimationInitialising
	// EstimationGeometryConverging: motion is credible; dimensions and
	// orientation are still moving.
	EstimationGeometryConverging
	// EstimationEstablished: pose, orientation and dimensions are all within
	// convergence bounds.
	EstimationEstablished
	// EstimationTemporarilyDegraded: was established; evidence has become
	// inadequate.
	EstimationTemporarilyDegraded
	// EstimationModelInvalid: the object model may no longer describe this
	// object. Evidence only, no pose.
	EstimationModelInvalid
	estimationStateEnd
)

var estimationStates = vocabulary[EstimationState]{
	kind: "estimation state",
	names: []string{
		"",
		"initialising",
		"geometry_converging",
		"established",
		"temporarily_degraded",
		"model_invalid",
	},
}

func (s EstimationState) String() string { return estimationStates.name(s) }

// Valid reports whether s is a registered estimation state.
func (s EstimationState) Valid() bool { return estimationStates.valid(s) }

// MarshalText writes the registered token and refuses an unspecified state.
func (s EstimationState) MarshalText() ([]byte, error) { return estimationStates.marshal(s) }

// UnmarshalText accepts registered tokens only.
func (s *EstimationState) UnmarshalText(b []byte) error {
	return estimationStates.unmarshal(s, b)
}

// ParseEstimationState parses a registered token.
func ParseEstimationState(s string) (EstimationState, error) { return estimationStates.parse(s) }

// EstimationStates lists every estimation state.
func EstimationStates() []EstimationState { return estimationStates.values() }

// AllowsReportedPose mirrors l5tracks: initialising has not earned belief and
// model_invalid has lost it, so neither may have its pose shown, even for
// review.
func (s EstimationState) AllowsReportedPose() bool {
	return s == EstimationGeometryConverging ||
		s == EstimationEstablished ||
		s == EstimationTemporarilyDegraded
}

// --- Endpoint source -------------------------------------------------------

// EndpointSource records the evidence behind a projected body endpoint, per
// Section 8.3. A body model may infer an unseen bumper with stated
// uncertainty; it must not relabel that inference as a measured return.
type EndpointSource uint8

const (
	// EndpointSourceUnspecified is the zero value and never valid.
	EndpointSourceUnspecified EndpointSource = iota
	// EndpointDirectlyObserved: returns from the face carrying this endpoint
	// were associated at this instant.
	EndpointDirectlyObserved
	// EndpointTemporallyInferred: the face was not seen at this instant; its
	// position comes from extent evidence about this object accumulated over
	// other frames.
	EndpointTemporallyInferred
	// EndpointPriorDominated: the face was not seen and the extent carrying it
	// is a class prior, not evidence about this object.
	EndpointPriorDominated
	endpointSourceEnd
)

var endpointSources = vocabulary[EndpointSource]{
	kind:  "endpoint source",
	names: []string{"", "directly_observed", "temporally_inferred", "prior_dominated"},
}

func (s EndpointSource) String() string { return endpointSources.name(s) }

// Valid reports whether s is a registered endpoint source.
func (s EndpointSource) Valid() bool { return endpointSources.valid(s) }

// MarshalText writes the registered token and refuses an unspecified source.
func (s EndpointSource) MarshalText() ([]byte, error) { return endpointSources.marshal(s) }

// UnmarshalText accepts registered tokens only.
func (s *EndpointSource) UnmarshalText(b []byte) error { return endpointSources.unmarshal(s, b) }

// ParseEndpointSource parses a registered token.
func ParseEndpointSource(s string) (EndpointSource, error) { return endpointSources.parse(s) }

// EndpointSources lists every endpoint source, strongest evidence first.
func EndpointSources() []EndpointSource { return endpointSources.values() }

// WeakerEndpointSource returns whichever of two sources rests on weaker
// evidence, so a pair result is labelled by its weakest endpoint rather than
// its best one.
func WeakerEndpointSource(a, b EndpointSource) EndpointSource {
	if a > b {
		return a
	}
	return b
}

// --- Motion class ----------------------------------------------------------

// MotionClass is the motion-model taxonomy of Section 5.5 of the
// state-estimation plan. The tokens are l5tracks.MotionClass's. Unlike
// l5tracks, the zero value here is unspecified rather than unknown: unknown is
// a real class (weak priors, broad bounds), and a caller who forgot to set the
// class must not be read as having chosen it.
type MotionClass uint8

const (
	// MotionClassUnspecified is the zero value and never valid.
	MotionClassUnspecified MotionClass = iota
	// MotionUnknown: dynamic or unclassified; weak priors.
	MotionUnknown
	// MotionRigidVehicle: car, truck, bus.
	MotionRigidVehicle
	// MotionTwoWheeler: cyclist, motorcyclist.
	MotionTwoWheeler
	// MotionPedestrian: approximately holonomic.
	MotionPedestrian
	motionClassEnd
)

var motionClasses = vocabulary[MotionClass]{
	kind:  "motion class",
	names: []string{"", "unknown", "rigid_vehicle", "two_wheeler", "pedestrian"},
}

func (m MotionClass) String() string { return motionClasses.name(m) }

// Valid reports whether m is a registered motion class.
func (m MotionClass) Valid() bool { return motionClasses.valid(m) }

// MarshalText writes the registered token and refuses an unspecified class.
func (m MotionClass) MarshalText() ([]byte, error) { return motionClasses.marshal(m) }

// UnmarshalText accepts registered tokens only.
func (m *MotionClass) UnmarshalText(b []byte) error { return motionClasses.unmarshal(m, b) }

// ParseMotionClass parses a registered token.
func ParseMotionClass(s string) (MotionClass, error) { return motionClasses.parse(s) }

// MotionClasses lists every motion class.
func MotionClasses() []MotionClass { return motionClasses.values() }

// --- Reference point -------------------------------------------------------

// ReferencePoint names the physical point a sample's planar position refers
// to. The tokens are l5tracks.ReferencePoint's. A pose is meaningless without
// it: the same object reported at its body centre and at its near face differ
// by half its width, which is the size of the defect the estimation plan
// exists to fix.
type ReferencePoint uint8

const (
	// ReferenceUnspecified is the zero value and never valid.
	ReferenceUnspecified ReferencePoint = iota
	// ReferenceBodyCentre is the centre of the believed body.
	ReferenceBodyCentre
	// ReferenceNearFaceCentre is the centre of the nearest observed face.
	ReferenceNearFaceCentre
	// ReferenceClusterMedoid is the initialisation seed, a point in the
	// observed cluster. It is not a place on the body and no endpoint may be
	// projected from it.
	ReferenceClusterMedoid
	referencePointEnd
)

var referencePoints = vocabulary[ReferencePoint]{
	kind:  "reference point",
	names: []string{"", "body_centre", "near_face_centre", "cluster_medoid"},
}

func (r ReferencePoint) String() string { return referencePoints.name(r) }

// Valid reports whether r is a registered reference point.
func (r ReferencePoint) Valid() bool { return referencePoints.valid(r) }

// MarshalText writes the registered token and refuses an unspecified point.
func (r ReferencePoint) MarshalText() ([]byte, error) { return referencePoints.marshal(r) }

// UnmarshalText accepts registered tokens only.
func (r *ReferencePoint) UnmarshalText(b []byte) error { return referencePoints.unmarshal(r, b) }

// ParseReferencePoint parses a registered token.
func ParseReferencePoint(s string) (ReferencePoint, error) { return referencePoints.parse(s) }

// ReferencePoints lists every reference point.
func ReferencePoints() []ReferencePoint { return referencePoints.values() }

// IsPhysical reports whether the reference denotes a place on the body.
func (r ReferencePoint) IsPhysical() bool {
	return r == ReferenceBodyCentre || r == ReferenceNearFaceCentre
}

// --- Belief provenance -----------------------------------------------------

// BeliefProvenance records where a believed extent or heading came from. The
// tokens are l5tracks.Provenance's; l5tracks' "none" is the unspecified zero
// value here, meaning there is no belief at all.
type BeliefProvenance uint8

const (
	// ProvenanceUnspecified is the zero value: nothing has supplied a belief.
	ProvenanceUnspecified BeliefProvenance = iota
	// ProvenanceClassPrior: the motion class's prior, not evidence about this
	// object.
	ProvenanceClassPrior
	// ProvenanceAccumulated: built from evidence across frames, typically
	// lower-bound spans.
	ProvenanceAccumulated
	// ProvenanceObserved: measured from a frame admitted as a two-sided
	// measurement.
	ProvenanceObserved
	beliefProvenanceEnd
)

var beliefProvenances = vocabulary[BeliefProvenance]{
	kind:  "belief provenance",
	names: []string{"", "class_prior", "accumulated", "observed"},
}

func (p BeliefProvenance) String() string { return beliefProvenances.name(p) }

// Valid reports whether p is a registered provenance.
func (p BeliefProvenance) Valid() bool { return beliefProvenances.valid(p) }

// MarshalText writes the registered token and refuses an unspecified value.
func (p BeliefProvenance) MarshalText() ([]byte, error) { return beliefProvenances.marshal(p) }

// UnmarshalText accepts registered tokens only.
func (p *BeliefProvenance) UnmarshalText(b []byte) error {
	return beliefProvenances.unmarshal(p, b)
}

// ParseBeliefProvenance parses a registered token.
func ParseBeliefProvenance(s string) (BeliefProvenance, error) { return beliefProvenances.parse(s) }

// BeliefProvenances lists every provenance.
func BeliefProvenances() []BeliefProvenance { return beliefProvenances.values() }

// IsEvidence reports whether the belief rests on observation of this object,
// as opposed to a class prior or nothing.
func (p BeliefProvenance) IsEvidence() bool {
	return p == ProvenanceAccumulated || p == ProvenanceObserved
}

// --- Path extremity --------------------------------------------------------

// PathExtremity names which end of a body's footprint, measured along the
// shared path's direction, an endpoint is. It is deliberately path-relative
// rather than body-relative: the gap is between the leader's trailing extreme
// and the follower's leading extreme whichever way either body faces.
type PathExtremity uint8

const (
	// ExtremityUnspecified is the zero value and never valid.
	ExtremityUnspecified PathExtremity = iota
	// ExtremityLeading is the footprint's foremost point along the path.
	ExtremityLeading
	// ExtremityTrailing is the footprint's rearmost point along the path.
	ExtremityTrailing
	pathExtremityEnd
)

var pathExtremities = vocabulary[PathExtremity]{
	kind:  "path extremity",
	names: []string{"", "leading", "trailing"},
}

func (e PathExtremity) String() string { return pathExtremities.name(e) }

// Valid reports whether e is a registered extremity.
func (e PathExtremity) Valid() bool { return pathExtremities.valid(e) }

// MarshalText writes the registered token and refuses an unspecified value.
func (e PathExtremity) MarshalText() ([]byte, error) { return pathExtremities.marshal(e) }

// UnmarshalText accepts registered tokens only.
func (e *PathExtremity) UnmarshalText(b []byte) error { return pathExtremities.unmarshal(e, b) }

// ParsePathExtremity parses a registered token.
func ParsePathExtremity(s string) (PathExtremity, error) { return pathExtremities.parse(s) }

// PathExtremities lists every extremity.
func PathExtremities() []PathExtremity { return pathExtremities.values() }

// --- Path condition --------------------------------------------------------

// PathCondition says why no common path could be established: the detail
// behind a no_common_path suppression. The first five are why a local path
// was refused, or why a track is not on one; the last two are why one
// follower instant could not be ordered along a path that was built. The
// declaration order is the reporting precedence when a refusal has several.
type PathCondition uint8

const (
	// PathConditionUnspecified is the zero value and never valid.
	PathConditionUnspecified PathCondition = iota
	// PathWeakSupport: too little observed, moving evidence to fit a path,
	// or to place a track on one.
	PathWeakSupport
	// PathDirectionReversal: motion in both directions along the corridor,
	// from a member that reverses or a body moving against the path.
	PathDirectionReversal
	// PathCrossing: a body crosses the corridor at an angle no following
	// relation allows.
	PathCrossing
	// PathForkOrMerge: tracks share part of the corridor and diverge
	// elsewhere, so the path branches.
	PathForkOrMerge
	// PathLateralIncompatible: tracks that overlap along the path are
	// laterally apart, so the corridor spans more than one path.
	PathLateralIncompatible
	// PathOutsideExtent: the follower lies outside the path's supported
	// extent at this instant.
	PathOutsideExtent
	// PathUnestablishedBody: the nearest body ahead in the corridor is not
	// established on the path, so it can be neither chosen nor ruled out.
	PathUnestablishedBody
	pathConditionEnd
)

var pathConditions = vocabulary[PathCondition]{
	kind: "path condition",
	names: []string{
		"",
		"weak_support",
		"direction_reversal",
		"crossing",
		"fork_or_merge",
		"lateral_incompatible",
		"outside_extent",
		"unestablished_body",
	},
}

func (c PathCondition) String() string { return pathConditions.name(c) }

// Valid reports whether c is a registered condition.
func (c PathCondition) Valid() bool { return pathConditions.valid(c) }

// MarshalText writes the registered token and refuses an unspecified value.
func (c PathCondition) MarshalText() ([]byte, error) { return pathConditions.marshal(c) }

// UnmarshalText accepts registered tokens only.
func (c *PathCondition) UnmarshalText(b []byte) error { return pathConditions.unmarshal(c, b) }

// ParsePathCondition parses a registered token.
func ParsePathCondition(s string) (PathCondition, error) { return pathConditions.parse(s) }

// PathConditions lists every condition in precedence order.
func PathConditions() []PathCondition { return pathConditions.values() }

// --- Candidate disposition -------------------------------------------------

// CandidateDisposition is what the pairing step decided about one other body
// at one follower instant, so a chosen leader, or a suppression, can be
// explained from the record rather than re-derived.
type CandidateDisposition uint8

const (
	// DispositionUnspecified is the zero value and never valid.
	DispositionUnspecified CandidateDisposition = iota
	// DispositionLeader: the nearest credible leader, chosen.
	DispositionLeader
	// DispositionCompeting: ahead in the corridor and not separable from the
	// nearest, so neither can be chosen.
	DispositionCompeting
	// DispositionUnestablished: the nearest body ahead in the corridor, but
	// not established on the follower's path.
	DispositionUnestablished
	// DispositionBlocked: ahead in the corridor, separably beyond the nearest.
	DispositionBlocked
	// DispositionBeyondRange: ahead in the corridor, beyond the search range.
	DispositionBeyondRange
	// DispositionBehind: in the corridor, not ahead of the follower.
	DispositionBehind
	// DispositionOutsideCorridor: on the path, laterally outside the corridor.
	DispositionOutsideCorridor
	// DispositionOffPath: does not project onto the path's supported extent.
	DispositionOffPath
	candidateDispositionEnd
)

var candidateDispositions = vocabulary[CandidateDisposition]{
	kind: "candidate disposition",
	names: []string{
		"",
		"leader",
		"competing",
		"unestablished",
		"blocked",
		"beyond_range",
		"behind",
		"outside_corridor",
		"off_path",
	},
}

func (d CandidateDisposition) String() string { return candidateDispositions.name(d) }

// Valid reports whether d is a registered disposition.
func (d CandidateDisposition) Valid() bool { return candidateDispositions.valid(d) }

// MarshalText writes the registered token and refuses an unspecified value.
func (d CandidateDisposition) MarshalText() ([]byte, error) {
	return candidateDispositions.marshal(d)
}

// UnmarshalText accepts registered tokens only.
func (d *CandidateDisposition) UnmarshalText(b []byte) error {
	return candidateDispositions.unmarshal(d, b)
}

// ParseCandidateDisposition parses a registered token.
func ParseCandidateDisposition(s string) (CandidateDisposition, error) {
	return candidateDispositions.parse(s)
}

// CandidateDispositions lists every disposition.
func CandidateDispositions() []CandidateDisposition { return candidateDispositions.values() }

// --- Interaction type ------------------------------------------------------

// InteractionType is the kind of pairwise encounter a persisted interaction
// records (Section 10.2). Each type fixes what its primary and secondary
// tracks are, and those are geometric roles, never fault.
//
// Only the types a method produces are registered. The plan's crossing,
// merging, overtaking and opposing are reserved until one does, together with
// the classification posterior Section 7.5 requires before a metric may be
// computed for them; a token may be added mid-list without migrating stored
// rows, because only the token is persisted.
type InteractionType uint8

const (
	// InteractionUnspecified is the zero value and never valid.
	InteractionUnspecified InteractionType = iota
	// InteractionFollowing: a leader/follower encounter on a shared directed
	// path. The primary track is the follower, the party whose gap to the body
	// ahead is measured; the secondary is the leader.
	InteractionFollowing
	interactionTypeEnd
)

var interactionTypes = vocabulary[InteractionType]{
	kind:  "interaction type",
	names: []string{"", "following"},
}

func (t InteractionType) String() string { return interactionTypes.name(t) }

// Valid reports whether t is a registered type.
func (t InteractionType) Valid() bool { return interactionTypes.valid(t) }

// MarshalText writes the registered token and refuses an unspecified type.
func (t InteractionType) MarshalText() ([]byte, error) { return interactionTypes.marshal(t) }

// UnmarshalText accepts registered tokens only.
func (t *InteractionType) UnmarshalText(b []byte) error { return interactionTypes.unmarshal(t, b) }

// ParseInteractionType parses a registered token.
func ParseInteractionType(s string) (InteractionType, error) { return interactionTypes.parse(s) }

// InteractionTypes lists every registered type.
func InteractionTypes() []InteractionType { return interactionTypes.values() }

// --- Exposure kind and observation basis -----------------------------------

// ExposureKind names the opportunity an exposure window would count toward
// (Sections 6 and 10.2). As with InteractionType, only produced kinds are
// registered: the plan's free_flow, yielding_opportunity and overtaking are
// reserved until a method produces them.
type ExposureKind uint8

const (
	// ExposureKindUnspecified is the zero value and never valid.
	ExposureKindUnspecified ExposureKind = iota
	// ExposureValidFollowing: time a follower spent behind a leader on a
	// shared path, the denominator of every following rate.
	ExposureValidFollowing
	exposureKindEnd
)

var exposureKinds = vocabulary[ExposureKind]{
	kind:  "exposure kind",
	names: []string{"", "valid_following"},
}

func (k ExposureKind) String() string { return exposureKinds.name(k) }

// Valid reports whether k is a registered kind.
func (k ExposureKind) Valid() bool { return exposureKinds.valid(k) }

// MarshalText writes the registered token and refuses an unspecified kind.
func (k ExposureKind) MarshalText() ([]byte, error) { return exposureKinds.marshal(k) }

// UnmarshalText accepts registered tokens only.
func (k *ExposureKind) UnmarshalText(b []byte) error { return exposureKinds.unmarshal(k, b) }

// ParseExposureKind parses a registered token.
func ParseExposureKind(s string) (ExposureKind, error) { return exposureKinds.parse(s) }

// ExposureKinds lists every registered kind.
func ExposureKinds() []ExposureKind { return exposureKinds.values() }

// ObservationBasis says what an interval of a pairwise encounter rests on:
// observation of both parties, or prediction for at least one. It is what
// keeps predicted-only time distinct from observed opportunity in storage
// (Sections 9.2 and 10.4): an exposure window or an encounter instant carries
// one, and only observed time may enter a denominator, whatever its kind.
type ObservationBasis uint8

const (
	// BasisUnspecified is the zero value and never valid.
	BasisUnspecified ObservationBasis = iota
	// BasisObserved: both parties were observed at the instant.
	BasisObserved
	// BasisPredictedOnly: at least one party was coasted, occluded, missing
	// or otherwise not observed, so any value there is a prediction and is
	// review-only.
	BasisPredictedOnly
	observationBasisEnd
)

var observationBases = vocabulary[ObservationBasis]{
	kind:  "observation basis",
	names: []string{"", "observed", "predicted_only"},
}

func (b ObservationBasis) String() string { return observationBases.name(b) }

// Valid reports whether b is a registered basis.
func (b ObservationBasis) Valid() bool { return observationBases.valid(b) }

// MarshalText writes the registered token and refuses an unspecified basis.
func (b ObservationBasis) MarshalText() ([]byte, error) { return observationBases.marshal(b) }

// UnmarshalText accepts registered tokens only.
func (b *ObservationBasis) UnmarshalText(v []byte) error { return observationBases.unmarshal(b, v) }

// ParseObservationBasis parses a registered token.
func ParseObservationBasis(s string) (ObservationBasis, error) { return observationBases.parse(s) }

// ObservationBases lists every basis.
func ObservationBases() []ObservationBasis { return observationBases.values() }

// CountsTowardExposure reports whether time on this basis may enter an
// exposure or opportunity denominator: observed time only.
func (b ObservationBasis) CountsTowardExposure() bool { return b == BasisObserved }

// basisOf is the pair basis of two parties' support at one instant.
func basisOf(follower, leader SupportState) ObservationBasis {
	if follower == SupportObserved && leader == SupportObserved {
		return BasisObserved
	}
	return BasisPredictedOnly
}

// --- Uncertainty kind and propagation method -------------------------------

// UncertaintyKind is the representation an uncertainty uses, per Section 9.3:
// a scalar sigma misreports ratios and extrema, so the kind is chosen per
// metric rather than imposed globally.
type UncertaintyKind uint8

const (
	// UncertaintyUnspecified is the zero value and never valid.
	UncertaintyUnspecified UncertaintyKind = iota
	// UncertaintyNone states explicitly that no uncertainty is available.
	UncertaintyNone
	// UncertaintySigma is a symmetric, approximately Gaussian one-sigma.
	UncertaintySigma
	// UncertaintyInterval is a coverage interval, for asymmetric or
	// heavy-tailed quantities.
	UncertaintyInterval
	// UncertaintyBounds is a hard lower and/or upper bound.
	UncertaintyBounds
	uncertaintyKindEnd
)

var uncertaintyKinds = vocabulary[UncertaintyKind]{
	kind:  "uncertainty kind",
	names: []string{"", "none", "sigma", "interval", "bounds"},
}

func (k UncertaintyKind) String() string { return uncertaintyKinds.name(k) }

// Valid reports whether k is a registered kind.
func (k UncertaintyKind) Valid() bool { return uncertaintyKinds.valid(k) }

// MarshalText writes the registered token and refuses an unspecified kind.
func (k UncertaintyKind) MarshalText() ([]byte, error) { return uncertaintyKinds.marshal(k) }

// UnmarshalText accepts registered tokens only.
func (k *UncertaintyKind) UnmarshalText(b []byte) error { return uncertaintyKinds.unmarshal(k, b) }

// ParseUncertaintyKind parses a registered token.
func ParseUncertaintyKind(s string) (UncertaintyKind, error) { return uncertaintyKinds.parse(s) }

// UncertaintyKinds lists every kind.
func UncertaintyKinds() []UncertaintyKind { return uncertaintyKinds.values() }

// PropagationMethod is how an uncertainty was propagated, per Section 9.3.
type PropagationMethod uint8

const (
	// MethodUnspecified is the zero value and never valid.
	MethodUnspecified PropagationMethod = iota
	// MethodAnalytic is an exact closed form.
	MethodAnalytic
	// MethodLinearised is first-order propagation through the Jacobian.
	MethodLinearised
	// MethodSigmaPoint is deterministic sigma-point propagation.
	MethodSigmaPoint
	// MethodMonteCarlo is stochastic sampling; the sample count is recorded.
	MethodMonteCarlo
	propagationMethodEnd
)

var propagationMethods = vocabulary[PropagationMethod]{
	kind:  "propagation method",
	names: []string{"", "analytic", "linearised", "sigma_point", "monte_carlo"},
}

func (m PropagationMethod) String() string { return propagationMethods.name(m) }

// Valid reports whether m is a registered method.
func (m PropagationMethod) Valid() bool { return propagationMethods.valid(m) }

// MarshalText writes the registered token and refuses an unspecified method.
func (m PropagationMethod) MarshalText() ([]byte, error) { return propagationMethods.marshal(m) }

// UnmarshalText accepts registered tokens only.
func (m *PropagationMethod) UnmarshalText(b []byte) error {
	return propagationMethods.unmarshal(m, b)
}

// ParsePropagationMethod parses a registered token.
func ParsePropagationMethod(s string) (PropagationMethod, error) {
	return propagationMethods.parse(s)
}

// PropagationMethods lists every method.
func PropagationMethods() []PropagationMethod { return propagationMethods.values() }

// --- Benchmark kind --------------------------------------------------------

// BenchmarkKind is what a metric is compared against, per Section 5. It is a
// required declaration, not documentation: a metric with no defensible
// universal threshold is reported as a value and a percentile, never converted
// into pass or fail.
type BenchmarkKind uint8

const (
	// BenchmarkUnspecified is the zero value. On a metric definition it means
	// no benchmark applies ("n/a" in the plan's tables).
	BenchmarkUnspecified BenchmarkKind = iota
	// BenchmarkLegal: conformance with a jurisdictional rule.
	BenchmarkLegal
	// BenchmarkResearchThreshold: conflict severity under a published
	// methodology.
	BenchmarkResearchThreshold
	// BenchmarkExternalDistribution: position against a published population.
	BenchmarkExternalDistribution
	// BenchmarkLocalDistribution: position against comparable local traffic.
	BenchmarkLocalDistribution
	// BenchmarkNoEstablishedThreshold: none defensible, stated explicitly.
	// Time-headway bands are this, never research_threshold.
	BenchmarkNoEstablishedThreshold
	benchmarkKindEnd
)

var benchmarkKinds = vocabulary[BenchmarkKind]{
	kind: "benchmark kind",
	names: []string{
		"",
		"legal",
		"research_threshold",
		"external_distribution",
		"local_distribution",
		"no_established_threshold",
	},
}

func (k BenchmarkKind) String() string { return benchmarkKinds.name(k) }

// Valid reports whether k is a registered kind.
func (k BenchmarkKind) Valid() bool { return benchmarkKinds.valid(k) }

// MarshalText writes the registered token and refuses an unspecified kind.
func (k BenchmarkKind) MarshalText() ([]byte, error) { return benchmarkKinds.marshal(k) }

// UnmarshalText accepts registered tokens only.
func (k *BenchmarkKind) UnmarshalText(b []byte) error { return benchmarkKinds.unmarshal(k, b) }

// ParseBenchmarkKind parses a registered token.
func ParseBenchmarkKind(s string) (BenchmarkKind, error) { return benchmarkKinds.parse(s) }

// BenchmarkKinds lists every kind.
func BenchmarkKinds() []BenchmarkKind { return benchmarkKinds.values() }

// --- Visibility ------------------------------------------------------------

// Visibility is a metric's surface expectation in the metrics registry
// (docs/platform/architecture/metrics-registry.md). review_only is the token
// this package adds: a value shown on review surfaces, clearly separated, that
// never enters a published distribution or an exposure denominator.
type Visibility uint8

const (
	// VisibilityUnspecified is the zero value and never valid.
	VisibilityUnspecified Visibility = iota
	// VisibilityPublic may reach public surfaces.
	VisibilityPublic
	// VisibilityInternal stays inside the system.
	VisibilityInternal
	// VisibilityReviewOnly is shown for review only.
	VisibilityReviewOnly
	// VisibilityFutureStub is reserved, not yet produced.
	VisibilityFutureStub
	// VisibilityDeprecated is retained for migration only.
	VisibilityDeprecated
	visibilityEnd
)

var visibilities = vocabulary[Visibility]{
	kind:  "visibility",
	names: []string{"", "public", "internal", "review_only", "future_stub", "deprecated"},
}

func (v Visibility) String() string { return visibilities.name(v) }

// Valid reports whether v is a registered visibility.
func (v Visibility) Valid() bool { return visibilities.valid(v) }

// MarshalText writes the registered token and refuses an unspecified value.
func (v Visibility) MarshalText() ([]byte, error) { return visibilities.marshal(v) }

// UnmarshalText accepts registered tokens only.
func (v *Visibility) UnmarshalText(b []byte) error { return visibilities.unmarshal(v, b) }

// ParseVisibility parses a registered token.
func ParseVisibility(s string) (Visibility, error) { return visibilities.parse(s) }

// Visibilities lists every visibility.
func Visibilities() []Visibility { return visibilities.values() }

// EntersDistribution reports whether values of a metric with this visibility
// may enter a published distribution or exposure aggregate.
func (v Visibility) EntersDistribution() bool { return v == VisibilityPublic }
