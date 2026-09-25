package l5tracks

// Occlusion continuity: an object's existence is a hypothesis held separately
// from whether it was observed.
//
// The tracker has always coasted an unmatched track on its prediction, and it
// has always recorded the coasted position in the trail exactly as it records
// an observed one. That conflates two statements. "Something was measured
// here" is evidence; "we believe the object is still here" is a hypothesis,
// and a hypothesis needs to say what it rests on and when it lapses.
// Sprint 0.5.2.2 of docs/plans/lidar-state-estimation-plan.md asks for four
// things, and this file holds the state and the rules for them:
//
//   - A support token on every instant (ObservationSupport), recorded on each
//     history point, so an observed point and a coasted one are never
//     interchangeable downstream. Always on; it changes no estimate.
//   - A per-track existence state (ExistenceState), distinct from the track
//     lifecycle (should this track exist?) and from the estimation lifecycle
//     of Section 5.6 (do we believe the physical state?). It answers a third
//     question: is the object currently seen, and if not, is its absence
//     explained?
//   - An expiry reason on every deleted track, and window counts of each.
//   - Default-off options, in OcclusionContinuityConfig, that change what the
//     tracker does: classify an absence, grow uncertainty per coast second
//     rather than per missed frame, bound coast age per class with a longer
//     allowance only for an explained absence, and refuse a reacquisition
//     whose geometry does not fit the body the track believes in.
//
// Predicted existence is a hypothesis, not proof that the object remains. The
// options are written so that the default direction of every doubt is to let
// a hypothesis lapse, and to start a new identity, rather than to extend one.

import (
	"math"
	"sort"
)

// ObservationSupport is what one instant of a track rests on: whether a
// detection was associated, and when none was, why not.
//
// The tokens are the closed vocabulary of Section 7.3 of
// docs/plans/lidar-behaviour-analytics-plan.md and are identical to
// l8behaviour.SupportState's. l5tracks cannot import that package (L5 never
// depends on L8), so a consumer maps between the two by token, String here
// and ParseSupportState there, never by numeric value. The declaration order
// happens to match; nothing should rely on it.
//
// l5tracks emits observed on every associated instant and coasted on every
// unobserved one. With OcclusionContinuityConfig.ExplainAbsence (or the class
// coast bounds, which need it) an unobserved instant is classified instead as
// occluded_inferred, missed_unknown or out_of_fov. cluster_merged and
// cluster_split are declared so the vocabulary is complete, but the tracker
// does not claim them: its merge and split flags are advisory size ratios,
// not evidence of which object a detection belongs to.
type ObservationSupport uint8

const (
	// SupportUnrecorded is the zero value: an instant with no support record,
	// such as a history point restored from storage. It is never a token.
	SupportUnrecorded ObservationSupport = iota
	// SupportObserved: a detection was associated at this instant.
	SupportObserved
	// SupportCoasted: the estimator propagated without a measurement, and the
	// absence was not classified.
	SupportCoasted
	// SupportOccludedInferred: missing, and a detection nearer the sensor
	// covers the predicted footprint.
	SupportOccludedInferred
	// SupportMissedUnknown: missing with no explanation. A detector defect
	// signal, or an object that has gone.
	SupportMissedUnknown
	// SupportClusterMerged and SupportClusterSplit are declared for the
	// vocabulary only; see the type comment.
	SupportClusterMerged
	SupportClusterSplit
	// SupportOutOfFOV: the prediction lies outside the configured coverage.
	SupportOutOfFOV
	supportTokenCount
)

var supportTokens = [supportTokenCount]string{
	"", "observed", "coasted", "occluded_inferred", "missed_unknown",
	"cluster_merged", "cluster_split", "out_of_fov",
}

// String returns the support token, or "" for SupportUnrecorded.
func (s ObservationSupport) String() string {
	if s >= supportTokenCount {
		return ""
	}
	return supportTokens[s]
}

// ParseObservationSupport parses a support token. The empty string and
// unknown tokens are refused.
func ParseObservationSupport(token string) (ObservationSupport, bool) {
	for i, name := range supportTokens {
		if i > 0 && name == token {
			return ObservationSupport(i), true
		}
	}
	return SupportUnrecorded, false
}

// IsObserved reports whether a detection was associated at this instant.
// Coasting is not observation (behaviour plan Section 9.2): every other
// token, including an explained absence, is false.
func (s ObservationSupport) IsObserved() bool { return s == SupportObserved }

// isAbsence reports an unobserved instant, classified or not.
func (s ObservationSupport) isAbsence() bool {
	switch s {
	case SupportCoasted, SupportOccludedInferred, SupportMissedUnknown, SupportOutOfFOV:
		return true
	}
	return false
}

// ExistenceState is the continuity hypothesis a track currently holds.
//
// It is the third of three lifecycles, and none substitutes for another.
// TrackState says whether the track should exist at all; EstimationState says
// whether its physical pose is believed; this says whether the object is being
// seen and, when it is not, whether the absence is explained. A confirmed,
// established track can be coasting unexplained, and that combination is
// exactly the one a consumer must not read as observed.
type ExistenceState uint8

const (
	// ExistenceUnknown is the zero value, for a track built by hand.
	ExistenceUnknown ExistenceState = iota
	// ExistenceObserved: associated with a detection this frame.
	ExistenceObserved
	// ExistenceCoasting: unobserved, and the absence was not classified
	// because absence explanation is off.
	ExistenceCoasting
	// ExistenceCoastingExplained: unobserved, and the geometry explains it:
	// occluded by a nearer detection, or outside the sensor's coverage.
	ExistenceCoastingExplained
	// ExistenceCoastingUnexplained: unobserved with no explanation.
	ExistenceCoastingUnexplained
	// ExistenceExpired: the hypothesis lapsed; ExpiryReason says why.
	ExistenceExpired
)

var existenceTokens = [...]string{"", "observed", "coasting", "coasting_explained", "coasting_unexplained", "expired"}

// String returns the existence token, or "" for ExistenceUnknown.
func (e ExistenceState) String() string {
	if int(e) >= len(existenceTokens) {
		return ""
	}
	return existenceTokens[e]
}

// existenceFor is the existence state an instant's support implies for a
// live track.
func existenceFor(s ObservationSupport) ExistenceState {
	switch s {
	case SupportObserved:
		return ExistenceObserved
	case SupportOccludedInferred, SupportOutOfFOV:
		return ExistenceCoastingExplained
	case SupportMissedUnknown:
		return ExistenceCoastingUnexplained
	default:
		return ExistenceCoasting
	}
}

// ExpiryReason records why a track was deleted.
type ExpiryReason uint8

const (
	// ExpiryNone: the track has not been deleted.
	ExpiryNone ExpiryReason = iota
	// ExpiryMisses: the frame-count rule, MaxMisses or MaxMissesConfirmed.
	ExpiryMisses
	// ExpiryCoastAge: the simple capture-time bound, MaxCoastSecs*.
	ExpiryCoastAge
	// ExpiryMissedUnknown: the class coast bound for an unexplained absence.
	ExpiryMissedUnknown
	// ExpiryOccludedInferred: the longer class coast bound for an absence a
	// nearer detection explained.
	ExpiryOccludedInferred
	// ExpiryOutOfFOV: the class coast bound while the prediction lay outside
	// the sensor's coverage. The absence is explained, but explains a
	// departure, so the allowance is the short one.
	ExpiryOutOfFOV
	// ExpiryNonFinite: predict or update produced a non-finite state.
	ExpiryNonFinite
)

var expiryTokens = [...]string{"", "misses", "coast_age", "missed_unknown", "occluded_inferred", "out_of_fov", "non_finite"}

// String returns the expiry token, or "" for ExpiryNone.
func (r ExpiryReason) String() string {
	if int(r) >= len(expiryTokens) {
		return ""
	}
	return expiryTokens[r]
}

// expiryReasonFor names a class-bound expiry after the absence that ended it.
func expiryReasonFor(s ObservationSupport) ExpiryReason {
	switch s {
	case SupportOccludedInferred:
		return ExpiryOccludedInferred
	case SupportOutOfFOV:
		return ExpiryOutOfFOV
	default:
		return ExpiryMissedUnknown
	}
}

// SensorCoverage is the region a sensor can observe, in the tracker's world
// frame about OcclusionContinuityConfig's sensor origin. A prediction outside
// it is out_of_fov rather than missed. The zero value covers everything.
type SensorCoverage struct {
	// MinRangeMetres and MaxRangeMetres bound the horizontal range. Zero
	// disables each bound.
	MinRangeMetres, MaxRangeMetres float32
	// AzimuthCentreDeg and AzimuthHalfWidthDeg describe a sector,
	// anticlockwise from +X. A half-width of zero, or of 180 or more, is the
	// full circle.
	AzimuthCentreDeg, AzimuthHalfWidthDeg float32
}

// contains reports whether a point at the given range and azimuth lies
// inside the coverage.
func (c SensorCoverage) contains(rangeMetres, azimuthRad float64) bool {
	if c.MinRangeMetres > 0 && rangeMetres < float64(c.MinRangeMetres) {
		return false
	}
	if c.MaxRangeMetres > 0 && rangeMetres > float64(c.MaxRangeMetres) {
		return false
	}
	if c.AzimuthHalfWidthDeg > 0 && c.AzimuthHalfWidthDeg < 180 {
		off := math.Abs(float64(wrapToPi(float32(azimuthRad) - float32(float64(c.AzimuthCentreDeg)*math.Pi/180))))
		if off > float64(c.AzimuthHalfWidthDeg)*math.Pi/180 {
			return false
		}
	}
	return true
}

// CoastPolicy is one motion class's allowance for an unobserved interval.
type CoastPolicy struct {
	// UnexplainedSecs bounds capture-time coast age while the absence is
	// missed_unknown or out_of_fov. ExplainedSecs is the longer bound while it
	// is occluded_inferred. Both are needed when ClassCoastBounds is on; a
	// zero bound expires a confirmed track on its first unobserved instant.
	UnexplainedSecs, ExplainedSecs float32
	// InflationPerSec is the position variance, in m² per axis, added per
	// second of unobserved capture time when CaptureTimeInflation is on. Zero
	// leaves only the filter's own process noise: pure CV growth.
	InflationPerSec float32
}

// OcclusionContinuityConfig holds the continuity options. Every switch is
// default-off, and like the other estimator options in TrackerConfig these
// are Go-level fields, deliberately not tuning keys, so the tuning
// fingerprint and the committed perf baselines do not move. The zero value is
// the shipped tracker exactly.
type OcclusionContinuityConfig struct {
	// ExplainAbsence classifies every unobserved instant as out_of_fov,
	// occluded_inferred or missed_unknown instead of coasted. On its own it
	// changes no estimate and no lifecycle decision: it is diagnostic, and
	// replayeval's coast_support experiment exists to prove that on a replay.
	ExplainAbsence bool

	// CaptureTimeInflation replaces OcclusionCovInflation, which adds a fixed
	// amount per missed frame, with the track's class InflationPerSec times
	// the capture time the frame added to its coast age. The shipped form
	// grows uncertainty with the number of frames that happened to reach the
	// tracker; this grows it with the time that passed.
	CaptureTimeInflation bool

	// ClassCoastBounds replaces the frame-count expiry rule with capture-time
	// coast bounds: TentativeCoastSecs for a tentative track, and for a
	// confirmed one its class's UnexplainedSecs, or ExplainedSecs while the
	// absence is occluded_inferred. It implies absence explanation. The bound
	// is checked before association against the last instant's explanation,
	// and after association against this frame's, so an explanation that
	// disappears (the occluder moved on and the object is not behind it)
	// lapses the hypothesis at once. MaxCoastSecs* still apply alongside.
	ClassCoastBounds bool
	// TentativeCoastSecs is the tentative track's bound under
	// ClassCoastBounds, whatever its class or explanation: a hypothesis that
	// has not earned confirmation does not earn a longer absence either.
	TentativeCoastSecs float32

	// ReacquisitionGuard refuses to let a coasting track reacquire a cluster
	// whose extent does not fit the body the track believes in, and refuses
	// an ambiguous reacquisition outright, rather than guessing. See
	// guardReacquisition.
	ReacquisitionGuard bool

	// SensorX and SensorY are the sensor origin in the tracker's world frame,
	// for occlusion geometry and coverage. Zero is correct while the pipeline
	// runs in sensor coordinates with a nil pose, as it does today; a site
	// pose must set them.
	SensorX, SensorY float32
	// Coverage is where the sensor can observe, for out_of_fov.
	Coverage SensorCoverage

	// Policies holds each motion class's coast policy, indexed by
	// MotionClass. See Tracker.coastPolicy for how a track's class is read.
	Policies [motionClassCount]CoastPolicy
}

// motionClassCount sizes per-class tables indexed by MotionClass.
const motionClassCount = int(MotionPedestrian) + 1

// explainsAbsence reports whether unobserved instants are to be classified.
func (c *OcclusionContinuityConfig) explainsAbsence() bool {
	return c.ExplainAbsence || c.ClassCoastBounds
}

// DefaultOcclusionContinuity is every continuity option switched on with
// starting values. They are stated priors, not values chosen against
// held-out occlusion scenes, which the state-estimation plan requires before
// any of them ships; the synthetic scenario tests record what each implies.
//
// Rationale, per class:
//
//   - Unexplained, 1.5 s for a classified track: the shipped confirmed miss
//     budget (15 frames) expressed in capture time at 10 Hz, so replacing the
//     frame count does not by itself change a busy scene. Unknown class,
//     1.0 s: an unclassified hypothesis should lapse sooner.
//   - Explained: twice the unexplained allowance for vehicles and
//     two-wheelers, which is roughly where a moderate hidden stop takes a CV
//     prediction beyond the 5 m jump guard, so reacquisition could not
//     succeed anyway; longer for pedestrians, whose prediction error grows
//     with walking speed rather than with speed squared.
//   - Inflation: a stop or turn hidden from the filter needs variance that
//     grows with time. Every rate here is below the 5 m²/s the shipped
//     per-frame inflation adds at 10 Hz, so at the nominal frame rate a
//     coasting track's association discount (gap analysis S3) is never
//     larger than it is today. Pedestrians are slower, so their rate is
//     lower; unknown takes the vehicle rate, broad rather than confident.
//   - Tentative, 0.3 s: the shipped three-miss tentative budget at 10 Hz.
func DefaultOcclusionContinuity() OcclusionContinuityConfig {
	var policies [motionClassCount]CoastPolicy
	policies[MotionUnknown] = CoastPolicy{UnexplainedSecs: 1.0, ExplainedSecs: 2.0, InflationPerSec: 3.0}
	policies[MotionRigidVehicle] = CoastPolicy{UnexplainedSecs: 1.5, ExplainedSecs: 3.0, InflationPerSec: 3.0}
	policies[MotionTwoWheeler] = CoastPolicy{UnexplainedSecs: 1.5, ExplainedSecs: 3.0, InflationPerSec: 2.0}
	policies[MotionPedestrian] = CoastPolicy{UnexplainedSecs: 1.5, ExplainedSecs: 4.0, InflationPerSec: 1.0}
	return OcclusionContinuityConfig{
		ExplainAbsence:       true,
		CaptureTimeInflation: true,
		ClassCoastBounds:     true,
		TentativeCoastSecs:   0.3,
		ReacquisitionGuard:   true,
		Policies:             policies,
	}
}

// trackMotionClass is the class signal the tracker has for a track: the L6
// label the pipeline writes back through UpdateClassification, with its
// confidence as the posterior. Before a track is classified, and for any
// label that is not a road user, it is unknown. There is no runner-up on the
// track, so Section 5.5's split-posterior rule cannot apply here; only the
// strength rule does.
func trackMotionClass(track *TrackedObject) (MotionClass, float32) {
	return MotionClassBelief{
		Class:     MotionClassForLabel(track.ObjectClass),
		Posterior: track.ObjectConfidence,
	}.Effective()
}

// coastPolicy is a track's coast policy. The class's row is used at the
// strength of the class belief and relaxes linearly toward unknown's as the
// confidence falls, which is Section 5.5's first rule: classification
// uncertainty must not silently become motion certainty.
func (t *Tracker) coastPolicy(track *TrackedObject) CoastPolicy {
	policies := &t.Config.OcclusionContinuity.Policies
	unknown := policies[MotionUnknown]
	class, strength := trackMotionClass(track)
	if class == MotionUnknown || strength <= 0 {
		return unknown
	}
	c := policies[class]
	lerp := func(u, v float32) float32 { return u + strength*(v-u) }
	return CoastPolicy{
		UnexplainedSecs: lerp(unknown.UnexplainedSecs, c.UnexplainedSecs),
		ExplainedSecs:   lerp(unknown.ExplainedSecs, c.ExplainedSecs),
		InflationPerSec: lerp(unknown.InflationPerSec, c.InflationPerSec),
	}
}

// classCoastBound is the capture-time allowance for a track whose current
// absence is support, and the reason recorded if it lapses.
func (t *Tracker) classCoastBound(track *TrackedObject, support ObservationSupport) (float32, ExpiryReason) {
	reason := expiryReasonFor(support)
	if track.TrackState != TrackConfirmed {
		return t.Config.OcclusionContinuity.TentativeCoastSecs, reason
	}
	policy := t.coastPolicy(track)
	if support == SupportOccludedInferred {
		return policy.ExplainedSecs, reason
	}
	return policy.UnexplainedSecs, reason
}

// classCoastExpired applies ClassCoastBounds to a track whose coast age is
// current. support is the absence to judge it against; before association,
// when this frame's absence is not yet known, the caller passes the last
// instant's, and an observed last instant counts as unexplained: nothing
// supported the interval since.
func (t *Tracker) classCoastExpired(track *TrackedObject, support ObservationSupport) (ExpiryReason, bool) {
	if !t.Config.OcclusionContinuity.ClassCoastBounds {
		return ExpiryNone, false
	}
	if !support.isAbsence() {
		support = SupportMissedUnknown
	}
	bound, reason := t.classCoastBound(track, support)
	return reason, track.CoastAgeSecs >= bound
}

// inflateCoastCovariance widens an unmatched track's position covariance for
// the frame it was not observed in.
//
// The shipped form adds OcclusionCovInflation per missed frame, whatever the
// frame interval. Under CaptureTimeInflation the track's class rate is
// charged for the capture time this frame added to its coast age, so the
// same unobserved second costs the same whether one frame reached the tracker
// in it or ten. Both are capped at MaxCovarianceDiag.
func (t *Tracker) inflateCoastCovariance(track *TrackedObject) {
	var add float32
	if t.Config.OcclusionContinuity.CaptureTimeInflation {
		elapsed := track.CoastAgeSecs - track.inflatedCoastSecs
		track.inflatedCoastSecs = track.CoastAgeSecs
		if elapsed > 0 {
			add = t.coastPolicy(track).InflationPerSec * elapsed
		}
	} else {
		add = t.Config.OcclusionCovInflation
	}
	// Capped at MaxCovarianceDiag to prevent unbounded growth over long
	// coasting periods (e.g. 15 frames × 0.5 = +7.5).
	if add > 0 {
		track.P[0*4+0] += add
		track.P[1*4+1] += add
		if track.P[0*4+0] > t.Config.MaxCovarianceDiag {
			track.P[0*4+0] = t.Config.MaxCovarianceDiag
		}
		if track.P[1*4+1] > t.Config.MaxCovarianceDiag {
			track.P[1*4+1] = t.Config.MaxCovarianceDiag
		}
	}
}

// Absence-explanation geometry. Heuristics, stated as such.
const (
	// minFootprintRadiusMetres floors a predicted footprint, so a track with
	// no extent belief still has an angular width to be covered.
	minFootprintRadiusMetres = 0.25
	// occlusionMinCoveredFraction is how much of a footprint's angular
	// interval nearer detections must cover before the absence is called an
	// occlusion. Half: once half the body is hidden, what remains of a sparse
	// object can fall below the cluster minimum.
	occlusionMinCoveredFraction = 0.5
)

// explainAbsence classifies an unobserved instant from this frame's geometry
// alone, so the answer is deterministic and needs no history.
//
// out_of_fov takes precedence: a prediction outside the coverage cannot be
// seen whatever stands in front of it. Otherwise the absence is
// occluded_inferred when this frame's clusters that lie wholly nearer the
// sensor than the predicted footprint's near edge cover at least half of its
// angular interval, and missed_unknown when they do not.
//
// Both geometric choices err toward "unexplained". The footprint uses the
// track's believed long extent as its radius, the widest it can appear; an
// occluder uses its OBB's projected half-width across the line of sight, not
// its long extent. An unexplained absence gets the shorter allowance, so an
// error here shortens a hypothesis rather than extending one.
//
// Only foreground detections are occluders. A parked vehicle absorbed into
// the background, a building or a pole produces no cluster, so an object
// hidden behind one reads as missed_unknown until the background's range
// image is consulted as well; that is recorded as what remains.
func (t *Tracker) explainAbsence(track *TrackedObject, clusters []WorldCluster) ObservationSupport {
	cfg := &t.Config.OcclusionContinuity
	dx, dy := float64(track.X-cfg.SensorX), float64(track.Y-cfg.SensorY)
	rangeM := math.Hypot(dx, dy)
	azimuth := math.Atan2(dy, dx)
	if !cfg.Coverage.contains(rangeM, azimuth) {
		return SupportOutOfFOV
	}

	radius := float64(track.believedLongExtent()) / 2
	if radius < minFootprintRadiusMetres {
		radius = minFootprintRadiusMetres
	}
	half := apparentHalfAngle(radius, rangeM)
	nearEdge := rangeM - radius

	var covered [][2]float64
	for i := range clusters {
		cx, cy := clusterCentre(&clusters[i])
		cdx, cdy := float64(cx-cfg.SensorX), float64(cy-cfg.SensorY)
		cRange := math.Hypot(cdx, cdy)
		if cRange >= nearEdge {
			continue
		}
		cAz := math.Atan2(cdy, cdx)
		cHalf := apparentHalfAngle(projectedHalfWidth(&clusters[i], cAz), cRange)
		delta := float64(wrapToPi(float32(cAz - azimuth)))
		lo, hi := math.Max(delta-cHalf, -half), math.Min(delta+cHalf, half)
		if lo < hi {
			covered = append(covered, [2]float64{lo, hi})
		}
	}
	if half > 0 && unionLength(covered) >= occlusionMinCoveredFraction*2*half {
		return SupportOccludedInferred
	}
	return SupportMissedUnknown
}

// apparentHalfAngle is the half-angle a disc of the given radius subtends
// from the given range. A disc that reaches the sensor subtends a half-turn.
func apparentHalfAngle(radius, rangeM float64) float64 {
	if radius <= 0 {
		return 0
	}
	if rangeM <= radius {
		return math.Pi / 2
	}
	return math.Asin(radius / rangeM)
}

// clusterCentre is the cluster's OBB centre when it has a finite one, and its
// centroid otherwise.
func clusterCentre(c *WorldCluster) (float32, float32) {
	if obb := c.OBB; obb != nil && finiteMeasurementCoordinate(obb.CenterX) && finiteMeasurementCoordinate(obb.CenterY) {
		return obb.CenterX, obb.CenterY
	}
	return c.CentroidX, c.CentroidY
}

// projectedHalfWidth is a cluster's half-extent across the line of sight at
// the given bearing: its OBB projected perpendicular to the ray. Without an
// OBB the shorter bounding-box extent is used, which can only under-state how
// much it hides.
func projectedHalfWidth(c *WorldCluster, bearingRad float64) float64 {
	if obb := c.OBB; obb != nil && obb.Length > 0 && obb.Width > 0 && finiteMeasurementCoordinate(obb.HeadingRad) {
		rel := float64(obb.HeadingRad) - bearingRad
		return math.Abs(float64(obb.Length)/2*math.Sin(rel)) + math.Abs(float64(obb.Width)/2*math.Cos(rel))
	}
	short := c.BoundingBoxLength
	if c.BoundingBoxWidth < short {
		short = c.BoundingBoxWidth
	}
	return float64(short) / 2
}

// unionLength is the total length of a set of intervals, overlaps counted
// once.
func unionLength(intervals [][2]float64) float64 {
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
	total := 0.0
	lo, hi := intervals[0][0], intervals[0][1]
	for _, iv := range intervals[1:] {
		if iv[0] > hi {
			total += hi - lo
			lo, hi = iv[0], iv[1]
			continue
		}
		if iv[1] > hi {
			hi = iv[1]
		}
	}
	return total + hi - lo
}

// Reacquisition-guard thresholds. Bounded heuristics, not measured noise.
const (
	// reacquisitionMinObservations is how many observations a track needs
	// before its believed extent is trusted to refuse anything.
	reacquisitionMinObservations = 3
	// A returning cluster may be smaller than the believed body, since
	// occlusion and re-entry show part of it, but not by more than this
	// factor: a car-sized belief does not reacquire a pedestrian-sized
	// cluster.
	reacquisitionMinExtentRatio = 0.3
	// Nor may it be much larger. Evidence is censored, so the belief is a
	// lower bound and some growth is legitimate; a cluster beyond
	// belief × ratio + slack is a merge or a different, larger object.
	reacquisitionMaxExtentRatio   = 1.5
	reacquisitionExtentSlackMetre = 0.5
	// reacquisitionAmbiguityMargin is the cost difference, in the solver's d²
	// units, below which two candidates are not told apart. Two is a
	// likelihood ratio of e.
	reacquisitionAmbiguityMargin = 2.0
)

// reacquisitionPlausibilityDt is the interval the implied-speed check divides
// a pairing's distance from the prediction by.
//
// By default it is the frame interval for every track, which is right for a
// track observed last frame: its prediction was corrected one frame ago, so
// a discrepancy is a velocity error accrued over one frame. For a coasting
// track it is not. Its discrepancy accrued over the whole unobserved
// interval, and dividing it by one frame makes any reacquisition further
// than MaxReasonableSpeedMps × dt from the prediction (3 m at 10 Hz and
// 30 m/s) implausible, however long the coast. That, not the Mahalanobis
// gate or the 5 m jump guard, is the constraint that binds first on a
// reacquisition, and it makes growing uncertainty pointless. Under
// ReacquisitionGuard a reacquiring track is judged over its capture-time
// coast age instead; MaxPositionJumpMetres still bounds the distance
// outright, and the guard's extent and ambiguity checks decide the pairing.
func reacquisitionPlausibilityDt(guard bool, track *TrackedObject, dt float32) float32 {
	if guard && track.Misses > 0 && track.CoastAgeSecs > dt {
		return track.CoastAgeSecs
	}
	return dt
}

// clusterLongExtent is a cluster's longer horizontal extent.
func clusterLongExtent(c *WorldCluster) float32 {
	if c.BoundingBoxWidth > c.BoundingBoxLength {
		return c.BoundingBoxWidth
	}
	return c.BoundingBoxLength
}

// observeReacquisitionExtent admits an accepted observation's long span to
// the guard's extent belief. Only under ReacquisitionGuard, and not for a
// cluster already flagged as a probable merge, which is the axis path's own
// admission rule (heading_axis.go); the flag lags a frame, which a merge
// cannot corroborate itself within.
func (t *Tracker) observeReacquisitionExtent(track *TrackedObject, c *WorldCluster) {
	if t.Config.OcclusionContinuity.ReacquisitionGuard && !track.MergeCandidate {
		track.reacquisitionExtent.Observe(clusterLongExtent(c))
	}
}

// reacquisitionBelief is the long extent the guard judges a returning cluster
// against: the larger of the track's believed long extent and the guard's own
// corroborated maximum. With the axis path off, as it is by default,
// believedLongExtent is the running mean of spans, which every partial view
// drags down (heading_extent.go explains why), so a whole view of a car can
// look like a merge against it. A corroborated maximum of lower bounds does
// not shrink that way.
func reacquisitionBelief(track *TrackedObject) float32 {
	belief := track.believedLongExtent()
	if e := track.reacquisitionExtent.Estimate(); e > belief {
		belief = e
	}
	return belief
}

// Reacquisition verdicts.
const (
	reacquisitionFits = iota
	reacquisitionTooLarge
	reacquisitionTooSmall
)

// reacquisitionVerdict says whether a cluster could be an observation of the
// body a coasting track believes in, and if not, which way it misfits. A
// track without a trusted belief, or a cluster reporting no extent, carries
// no evidence either way.
func reacquisitionVerdict(track *TrackedObject, c *WorldCluster) int {
	if track.ObservationCount < reacquisitionMinObservations {
		return reacquisitionFits
	}
	belief, extent := reacquisitionBelief(track), clusterLongExtent(c)
	switch {
	case belief <= 0 || extent <= 0:
		return reacquisitionFits
	case extent > belief*reacquisitionMaxExtentRatio+reacquisitionExtentSlackMetre:
		return reacquisitionTooLarge
	case extent < belief*reacquisitionMinExtentRatio:
		return reacquisitionTooSmall
	}
	return reacquisitionFits
}

// guardReacquisition applies ReacquisitionGuard to one assignment stage's
// cost matrix, forbidding pairings in place. A track is reacquiring when it
// missed at least the previous frame.
//
// Geometry first: a pairing whose cluster does not fit the track's believed
// body is forbidden, so reacquisition is not simply the nearest box.
//
// Then ambiguity, in both directions, among the pairings that remain. A
// reacquiring track with two admissible clusters costed within the margin of
// each other, too far apart to be fragments of one body, is given neither.
// A cluster admissible for two reacquiring tracks within the margin is given
// to neither of them. Either way the track goes on coasting and the cluster
// is free to go to a track that saw its object last frame, or to seed a new
// one: an identity left unresolved, never a guessed one. This is deliberately
// the continuity side only. It never makes a pairing cheaper, so it cannot
// deepen the coasting-track discount of gap S3, and it does not attempt S3's
// remedy.
func (t *Tracker) guardReacquisition(costMatrix [][]float32, clusters []WorldCluster, clusterIdx []int, trackIDs []string) {
	forbidden := float32(hungarianlnf)
	reacquiring := make([]bool, len(trackIDs))
	for tj, id := range trackIDs {
		track := t.Tracks[id]
		if track.Misses == 0 {
			continue
		}
		reacquiring[tj] = true
		for row, ci := range clusterIdx {
			if costMatrix[row][tj] >= forbidden {
				continue
			}
			switch reacquisitionVerdict(track, &clusters[ci]) {
			case reacquisitionTooLarge:
				t.continuity.ReacquisitionsRefusedLarger++
			case reacquisitionTooSmall:
				t.continuity.ReacquisitionsRefusedSmaller++
			default:
				continue
			}
			costMatrix[row][tj] = forbidden
		}
	}

	// Decide both directions against the same matrix, then forbid, so the
	// outcome does not depend on which direction is checked first.
	type pairing struct{ row, col int }
	var refuse []pairing
	for tj := range trackIDs {
		if !reacquiring[tj] {
			continue
		}
		best, second := -1, -1
		for row := range clusterIdx {
			c := costMatrix[row][tj]
			if c >= forbidden {
				continue
			}
			switch {
			case best < 0 || c < costMatrix[best][tj]:
				best, second = row, best
			case second < 0 || c < costMatrix[second][tj]:
				second = row
			}
		}
		if second < 0 || costMatrix[second][tj]-costMatrix[best][tj] >= reacquisitionAmbiguityMargin {
			continue
		}
		// Two fragments of one body are a split, not a question of identity.
		bx, by := clusterCentre(&clusters[clusterIdx[best]])
		sx, sy := clusterCentre(&clusters[clusterIdx[second]])
		if float32(math.Hypot(float64(bx-sx), float64(by-sy))) <= reacquisitionBelief(t.Tracks[trackIDs[tj]]) {
			continue
		}
		t.continuity.ReacquisitionsAmbiguous++
		for row := range clusterIdx {
			refuse = append(refuse, pairing{row, tj})
		}
	}
	for row := range clusterIdx {
		var cols []int
		for tj := range trackIDs {
			if reacquiring[tj] && costMatrix[row][tj] < forbidden {
				cols = append(cols, tj)
			}
		}
		if len(cols) < 2 {
			continue
		}
		sort.SliceStable(cols, func(i, j int) bool { return costMatrix[row][cols[i]] < costMatrix[row][cols[j]] })
		if costMatrix[row][cols[1]]-costMatrix[row][cols[0]] >= reacquisitionAmbiguityMargin {
			continue
		}
		t.continuity.ReacquisitionsAmbiguous++
		for _, tj := range cols {
			refuse = append(refuse, pairing{row, tj})
		}
	}
	for _, p := range refuse {
		costMatrix[p.row][p.col] = forbidden
	}
}

// coastAgeEdgesSecs are the upper edges of the coast-age histogram bins; a
// final bin holds everything at or beyond the last edge.
var coastAgeEdgesSecs = [...]float32{0.2, 0.5, 1, 1.5, 2, 3, 5}

// CoastAgeHistogram counts capture-time coast ages.
type CoastAgeHistogram struct {
	// UpperEdgesSecs are the bins' upper edges; Counts has one more bin, for
	// ages at or beyond the last edge.
	UpperEdgesSecs [len(coastAgeEdgesSecs)]float32   `json:"upper_edges_secs"`
	Counts         [len(coastAgeEdgesSecs) + 1]int64 `json:"counts"`
	MaxSecs        float32                           `json:"max_secs"`
}

func (h *CoastAgeHistogram) observe(ageSecs float32) {
	bin := len(coastAgeEdgesSecs)
	for i, edge := range coastAgeEdgesSecs {
		if ageSecs < edge {
			bin = i
			break
		}
	}
	h.Counts[bin]++
	if ageSecs > h.MaxSecs {
		h.MaxSecs = ageSecs
	}
}

// SupportCounts counts track-instants by support token: one per live track
// per Update. With absence explanation off every unobserved instant is
// coasted; with it on, none is.
type SupportCounts struct {
	Observed         int64 `json:"observed"`
	Coasted          int64 `json:"coasted"`
	OccludedInferred int64 `json:"occluded_inferred"`
	MissedUnknown    int64 `json:"missed_unknown"`
	OutOfFOV         int64 `json:"out_of_fov"`
}

func (c *SupportCounts) observe(s ObservationSupport) {
	switch s {
	case SupportObserved:
		c.Observed++
	case SupportCoasted:
		c.Coasted++
	case SupportOccludedInferred:
		c.OccludedInferred++
	case SupportMissedUnknown:
		c.MissedUnknown++
	case SupportOutOfFOV:
		c.OutOfFOV++
	}
}

// ExpiryCounts counts deletions by ExpiryReason.
type ExpiryCounts struct {
	Misses           int64 `json:"misses"`
	CoastAge         int64 `json:"coast_age"`
	MissedUnknown    int64 `json:"missed_unknown"`
	OccludedInferred int64 `json:"occluded_inferred"`
	OutOfFOV         int64 `json:"out_of_fov"`
	NonFinite        int64 `json:"non_finite"`
}

func (c *ExpiryCounts) observe(r ExpiryReason) {
	switch r {
	case ExpiryMisses:
		c.Misses++
	case ExpiryCoastAge:
		c.CoastAge++
	case ExpiryMissedUnknown:
		c.MissedUnknown++
	case ExpiryOccludedInferred:
		c.OccludedInferred++
	case ExpiryOutOfFOV:
		c.OutOfFOV++
	case ExpiryNonFinite:
		c.NonFinite++
	}
}

// ContinuityStats describes, for a window, what the tracker's hypotheses
// rested on and how they ended. It is diagnostic only: nothing in the
// estimator reads it back. Label-free by construction, so it can be compared
// between a default replay and an option on any capture.
//
// The window starts at NewTracker, Reset or BeginTrackingBaseline, so a
// replay's figures cover its scoring window.
type ContinuityStats struct {
	// TracksBorn counts tracks created in the window and TracksConfirmed
	// those of them promoted, so one minus their ratio is the fragmentation
	// proxy: the share of hypotheses born in the window that never earned
	// confirmation. A track born before the window is in neither.
	TracksBorn      int64 `json:"tracks_born"`
	TracksConfirmed int64 `json:"tracks_confirmed"`

	SupportInstants SupportCounts `json:"support_instants"`
	ExpiredByReason ExpiryCounts  `json:"expired_by_reason"`
	// CoastAgeAtExpiry is each deleted track's capture-time coast age when it
	// was deleted.
	CoastAgeAtExpiry CoastAgeHistogram `json:"coast_age_at_expiry"`

	// Reacquisitions counts associations that ended an unobserved interval,
	// and ReacquiredCoastAge the length of each interval they closed.
	Reacquisitions     int64             `json:"reacquisitions"`
	ReacquiredCoastAge CoastAgeHistogram `json:"reacquired_coast_age"`
	// ReacquisitionsRefusedLarger and ReacquisitionsRefusedSmaller count
	// pairings the guard forbade because the cluster was too large (a merge,
	// or a larger object) or too small (a fragment, or a smaller object) for
	// the believed body; ReacquisitionsAmbiguous counts ambiguous decisions it
	// left unresolved. All zero with the guard off.
	ReacquisitionsRefusedLarger  int64 `json:"reacquisitions_refused_larger"`
	ReacquisitionsRefusedSmaller int64 `json:"reacquisitions_refused_smaller"`
	ReacquisitionsAmbiguous      int64 `json:"reacquisitions_ambiguous"`
}

// ContinuityStats returns a snapshot of the continuity diagnostics.
func (t *Tracker) ContinuityStats() ContinuityStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s := t.continuity
	s.CoastAgeAtExpiry.UpperEdgesSecs = coastAgeEdgesSecs
	s.ReacquiredCoastAge.UpperEdgesSecs = coastAgeEdgesSecs
	return s
}

// recordSupport sets a live track's support and existence for this instant
// and counts it.
func (t *Tracker) recordSupport(track *TrackedObject, s ObservationSupport) {
	track.LastSupport = s
	track.Existence = existenceFor(s)
	t.continuity.SupportInstants.observe(s)
}

// supportForAbsence is an unmatched track's support this frame: coasted, or
// its classification when absence explanation is on.
func (t *Tracker) supportForAbsence(track *TrackedObject, clusters []WorldCluster) ObservationSupport {
	if !t.Config.OcclusionContinuity.explainsAbsence() {
		return SupportCoasted
	}
	return t.explainAbsence(track, clusters)
}
