package l8behaviour

// Pointwise following metrics, per Sections 8.3, 9.1 and 9.2 of
// docs/plans/lidar-behaviour-analytics-plan.md.
//
// For follower F and leader L on one directed path, at one synchronised
// instant:
//
//	gap = s(L trailing extreme) - s(F leading extreme)        bumper to bumper
//	THW = gap / v_F                                          net time gap
//
// where s is arc length along the path and v_F the follower's along-path
// speed. Each extreme is the body footprint's projection onto the path, never
// a cluster extremum, a medoid or an unqualified box centre. With the body
// centre at s_c, relative heading theta, length L and width W:
//
//	h        = (L/2)|cos theta| + (W/2)|sin theta|
//	leading  = s_c + h
//	trailing = s_c - h
//
// which reduces to the plan's centred half-length form at theta = 0.
//
// Uncertainty is first-order (linearised) and treats position, length, width
// and heading as independent. That is honest about what the contract carries:
// the estimate does not yet expose pose/extent cross-covariance. For a face
// that was directly observed the true pose/extent correlation is negative, so
// independence overstates that endpoint's sigma rather than understating it.
//
// This file stops at one instant. Building the path, choosing the nearest
// credible leader among candidates and aggregating exposure over an encounter
// are the next increment, and plug in through PathFrame, EvaluateFollowing and
// FollowingPoint.SupportedOpportunity.

import (
	"errors"
	"fmt"
	"math"
)

// FollowingMethodID versions the pointwise following equations and their
// suppression precedence, and is the method_id EvaluateFollowing records.
// Change it when either changes.
const FollowingMethodID = "following_pointwise_v1"

// ObservedSurfaceMethodID is the method_id a caller records for the
// supplemental observed-surface gap.
const ObservedSurfaceMethodID = "following_observed_surface_v1"

// Endpoint is one extreme of a body's footprint projected onto the shared
// path, with its bound and the evidence behind it.
type Endpoint struct {
	TrackID          string         `json:"track_id"`
	CaptureUnixNanos int64          `json:"capture_unix_nanos"`
	Extremity        PathExtremity  `json:"extremity"`
	ArcM             float64        `json:"arc_m"`
	SigmaM           float64        `json:"sigma_m"`
	Source           EndpointSource `json:"source"`
	// Support is the owning track's support at this instant.
	Support SupportState `json:"support"`
	// ExtentConverged is true when both extents carrying the endpoint met
	// their convergence bound. A prior-dominated endpoint never has.
	ExtentConverged bool `json:"extent_converged"`
}

func (e Endpoint) validate(want PathExtremity) error {
	if e.TrackID == "" {
		return fmt.Errorf("endpoint requires a track id")
	}
	if e.Extremity != want {
		return fmt.Errorf("endpoint of %s is %s, want %s", e.TrackID, e.Extremity, want)
	}
	if !finite(e.ArcM) || !finiteNonNegative(e.SigmaM) {
		return fmt.Errorf("endpoint of %s must have a finite arc and a non-negative sigma", e.TrackID)
	}
	if !e.Source.Valid() || !e.Support.Valid() {
		return fmt.Errorf("endpoint of %s requires a source and a support state", e.TrackID)
	}
	if e.Source == EndpointPriorDominated && e.ExtentConverged {
		return fmt.Errorf("endpoint of %s is prior-dominated and cannot be converged", e.TrackID)
	}
	if e.Source == EndpointDirectlyObserved && e.Support != SupportObserved {
		return fmt.Errorf("endpoint of %s cannot be directly observed on a %s instant", e.TrackID, e.Support)
	}
	return nil
}

// BodyOnPath is one sample's body expressed in the shared path's frame.
type BodyOnPath struct {
	TrackID            string   `json:"track_id"`
	CaptureUnixNanos   int64    `json:"capture_unix_nanos"`
	CentreArcM         float64  `json:"centre_arc_m"`
	LateralM           float64  `json:"lateral_m"`
	RelativeHeadingRad float64  `json:"relative_heading_rad"`
	AlongSpeedMps      float64  `json:"along_speed_mps"`
	AlongSpeedSigmaMps float64  `json:"along_speed_sigma_mps"`
	Leading            Endpoint `json:"leading"`
	Trailing           Endpoint `json:"trailing"`
}

// ProjectBody projects a sample's physical footprint onto the path.
//
// It returns a suppression reason, and no body, when an endpoint cannot be
// named at all: a reference point that is not on the body, an unresolved
// front/rear orientation (Section 8.3), an extent with no belief, or a body
// that does not project onto the path. An unconverged or prior extent does not
// stop the projection; it is recorded on the endpoints, so review can still
// show the bounded, wider inference.
func ProjectBody(path PathFrame, trackID string, s TrajectorySample) (BodyOnPath, SuppressionReason, error) {
	if path == nil {
		return BodyOnPath{}, ReasonUnspecified, fmt.Errorf("projection requires a path")
	}
	if trackID == "" {
		return BodyOnPath{}, ReasonUnspecified, fmt.Errorf("projection requires a track id")
	}
	if err := s.Validate(); err != nil {
		return BodyOnPath{}, ReasonUnspecified, fmt.Errorf("track %s: %w", trackID, err)
	}
	if !s.Reference.IsPhysical() {
		return BodyOnPath{}, ReasonInsufficientObservation, nil
	}
	if !s.Heading.Resolved() {
		return BodyOnPath{}, ReasonOrientationUnresolved, nil
	}
	if !s.Length.Present() || !s.Width.Present() {
		return BodyOnPath{}, ReasonExtentNotConverged, nil
	}

	psi := s.Heading.Rad
	cosPsi, sinPsi := math.Cos(psi), math.Sin(psi)
	ox, oy := s.AnchorToCentre.LongitudinalM, s.AnchorToCentre.LateralM
	cx := s.X + ox*cosPsi - oy*sinPsi
	cy := s.Y + ox*sinPsi + oy*cosPsi
	loc, ok := path.Locate(cx, cy)
	if !ok {
		return BodyOnPath{}, ReasonNoCommonPath, nil
	}

	theta := math.Remainder(psi-loc.TangentRad, 2*math.Pi)
	cosT, sinT := math.Cos(theta), math.Sin(theta)
	tx, ty := math.Cos(loc.TangentRad), math.Sin(loc.TangentRad)
	p := s.Covariance
	posVar := tx*tx*p[0] + tx*ty*(p[1]+p[4]) + ty*ty*p[5]
	speedVar := tx*tx*p[10] + tx*ty*(p[11]+p[14]) + ty*ty*p[15]

	halfL, halfW := s.Length.Metres/2, s.Width.Metres/2
	h := halfL*math.Abs(cosT) + halfW*math.Abs(sinT)
	// dh/dtheta. |cos| and |sin| have kinks at the axes; there the one-sided
	// slope's magnitude is used rather than zero, which would claim that
	// orientation error cannot move a corner forward. At exact alignment the
	// true effect is one-sided (the corner only ever moves outward), so a
	// symmetric linearised term overstates the spread and ignores the bias. A
	// sigma-point method may replace it once a metric needs the asymmetry.
	dh := -halfL*sinT*kinkSign(cosT) + halfW*cosT*kinkSign(sinT)
	// ds_c/dpsi: an anchor offset swings the centre as the body turns.
	dc := -ox*sinT - oy*cosT
	extentVar := sq(math.Abs(cosT)*s.Length.SigmaMetres/2) + sq(math.Abs(sinT)*s.Width.SigmaMetres/2)

	endpoint := func(extremity PathExtremity, arc, dArcDPsi float64, faceSeen bool) Endpoint {
		return Endpoint{
			TrackID: trackID, CaptureUnixNanos: s.CaptureUnixNanos, Extremity: extremity,
			ArcM:            arc,
			SigmaM:          math.Sqrt(posVar + extentVar + sq(dArcDPsi)*s.Heading.VarianceRad2),
			Source:          endpointSource(faceSeen, s.Length, s.Width),
			Support:         s.Support,
			ExtentConverged: s.Length.Converged && s.Width.Converged,
		}
	}
	// Which body face leads along the path depends on which way the body
	// faces: a body facing against the path direction leads with its rear.
	frontLeads := cosT >= 0
	leadingSeen, trailingSeen := s.Faces.FrontObserved, s.Faces.RearObserved
	if !frontLeads {
		leadingSeen, trailingSeen = trailingSeen, leadingSeen
	}
	return BodyOnPath{
		TrackID: trackID, CaptureUnixNanos: s.CaptureUnixNanos,
		CentreArcM: loc.ArcM, LateralM: loc.LateralM, RelativeHeadingRad: theta,
		AlongSpeedMps:      s.VX*tx + s.VY*ty,
		AlongSpeedSigmaMps: math.Sqrt(math.Max(speedVar, 0)),
		Leading:            endpoint(ExtremityLeading, loc.ArcM+h, dc+dh, leadingSeen),
		Trailing:           endpoint(ExtremityTrailing, loc.ArcM-h, dc-dh, trailingSeen),
	}, ReasonUnspecified, nil
}

// endpointSource derives the evidence label rather than accepting one, so a
// label cannot claim more than the sample supports. A face seen at this
// instant is directly observed; otherwise the endpoint is temporally inferred
// when both extents rest on evidence about this object, and prior-dominated
// when either is a class prior.
func endpointSource(faceSeen bool, length, width ExtentBelief) EndpointSource {
	switch {
	case faceSeen:
		return EndpointDirectlyObserved
	case length.Provenance.IsEvidence() && width.Provenance.IsEvidence():
		return EndpointTemporallyInferred
	default:
		return EndpointPriorDominated
	}
}

// GapEstimate is the arithmetic of one gap and the equation-level reason, if
// any, that it may not be published. The value is retained even when a reason
// is set, because review needs it: a non-positive gap is a geometry-review
// item and a coasted gap is the predicted gap. It becomes a Measurement only
// through a builder that drops the value whenever a reason applies.
type GapEstimate struct {
	CaptureUnixNanos int64             `json:"capture_unix_nanos"`
	ValueM           float64           `json:"value_m"`
	SigmaM           float64           `json:"sigma_m"`
	LeaderSource     EndpointSource    `json:"leader_source"`
	FollowerSource   EndpointSource    `json:"follower_source"`
	Reason           SuppressionReason `json:"reason,omitempty"`
}

// Source is the weaker of the two endpoint sources: a gap is only as well
// evidenced as its worse end.
func (g GapEstimate) Source() EndpointSource {
	return WeakerEndpointSource(g.LeaderSource, g.FollowerSource)
}

// Measurement turns the estimate into a registered measurement: its value and
// linearised sigma when no reason applies, a suppression otherwise.
func (g GapEstimate) Measurement(id MetricID, prov Provenance) (Measurement, error) {
	if g.Reason != ReasonUnspecified {
		return NewSuppressedMeasurement(id, g.Reason, prov)
	}
	return NewMeasurement(id, g.ValueM, SigmaUncertainty(g.SigmaM, MethodLinearised), prov)
}

// SpatialGap is the physical bumper-to-bumper gap between the leader's
// trailing extreme and the follower's leading extreme.
//
//	gap     = s_L,trailing - s_F,leading
//	sigma^2 = sigma_L,trailing^2 + sigma_F,leading^2
//
// With centred, aligned bodies each endpoint sigma is sqrt(sigma_s^2 +
// (sigma_len/2)^2), so this is Section 9.1's sigma_gap^2 = sigma_sL^2 +
// sigma_sF^2 + (sigma_LL/2)^2 + (sigma_LF/2)^2.
//
// Equation-level reasons, in precedence order: an endpoint not observed at
// this instant (the value becomes the review-only predicted gap), an extent
// not converged, and a non-positive gap, which requires overlap/geometry
// review rather than a zero-headway or collision claim.
func SpatialGap(leaderTrailing, followerLeading Endpoint) (GapEstimate, error) {
	if err := leaderTrailing.validate(ExtremityTrailing); err != nil {
		return GapEstimate{}, err
	}
	if err := followerLeading.validate(ExtremityLeading); err != nil {
		return GapEstimate{}, err
	}
	if leaderTrailing.TrackID == followerLeading.TrackID {
		return GapEstimate{}, fmt.Errorf("leader and follower are the same track %s", leaderTrailing.TrackID)
	}
	if leaderTrailing.CaptureUnixNanos != followerLeading.CaptureUnixNanos {
		return GapEstimate{}, fmt.Errorf("endpoints are not synchronised: %d and %d",
			leaderTrailing.CaptureUnixNanos, followerLeading.CaptureUnixNanos)
	}
	g := GapEstimate{
		CaptureUnixNanos: leaderTrailing.CaptureUnixNanos,
		ValueM:           leaderTrailing.ArcM - followerLeading.ArcM,
		SigmaM:           math.Hypot(leaderTrailing.SigmaM, followerLeading.SigmaM),
		LeaderSource:     leaderTrailing.Source,
		FollowerSource:   followerLeading.Source,
	}
	g.Reason = gapReasons(leaderTrailing, followerLeading, g.ValueM).first()
	return g, nil
}

// gapReasons is every equation-level reason for one endpoint pair. SpatialGap
// reports the first; the pointwise evaluator needs all of them, because a
// coasted pair that also overlaps must not have its overlap hidden behind
// not_observed and then shown as a predicted gap.
func gapReasons(leaderTrailing, followerLeading Endpoint, valueM float64) reasonSet {
	var r reasonSet
	if leaderTrailing.Support != SupportObserved || followerLeading.Support != SupportObserved {
		r.add(ReasonNotObserved)
	}
	if !leaderTrailing.ExtentConverged || !followerLeading.ExtentConverged {
		r.add(ReasonExtentNotConverged)
	}
	if !(valueM > 0) {
		r.add(ReasonNonPositiveGap)
	}
	return r
}

// TimeGapEstimate is a net time gap: a value and sigma, or a reason and no
// value. The pointers are the contract: a suppressed time gap has nothing to
// read, so it cannot be read as zero.
type TimeGapEstimate struct {
	ValueS *float64          `json:"value_s,omitempty"`
	SigmaS *float64          `json:"sigma_s,omitempty"`
	Reason SuppressionReason `json:"reason,omitempty"`
}

// NetTimeGap is gap / v_F, defined only for a publishable gap and a follower
// at or above the speed floor. Below the floor it is undefined or unstable,
// not infinite, and is suppressed (Sections 8.3 and 9.1); a spatial gap at
// standstill may still be valid.
//
//	sigma_THW = THW * sqrt((sigma_gap/gap)^2 + (sigma_v/v)^2)
//
// This is a net time gap, not front-to-front passage headway at a fixed
// detector. A gap already carrying a reason passes that reason through and
// yields no value.
func NetTimeGap(gap GapEstimate, speedMps, speedSigmaMps, speedFloorMps float64) (TimeGapEstimate, error) {
	if !(speedFloorMps > 0) || !finite(speedFloorMps) {
		return TimeGapEstimate{}, fmt.Errorf("speed floor must be positive and finite")
	}
	if !finite(speedMps) || !finiteNonNegative(speedSigmaMps) {
		return TimeGapEstimate{}, fmt.Errorf("follower speed must be finite with a non-negative sigma")
	}
	if !finite(gap.ValueM) || !finiteNonNegative(gap.SigmaM) {
		return TimeGapEstimate{}, fmt.Errorf("gap must be finite with a non-negative sigma")
	}
	if gap.Reason != ReasonUnspecified {
		return TimeGapEstimate{Reason: gap.Reason}, nil
	}
	if !(gap.ValueM > 0) {
		return TimeGapEstimate{}, fmt.Errorf("an unsuppressed gap must be positive")
	}
	if speedMps < speedFloorMps {
		return TimeGapEstimate{Reason: ReasonBelowSpeedFloor}, nil
	}
	thw := gap.ValueM / speedMps
	sigma := thw * math.Hypot(gap.SigmaM/gap.ValueM, speedSigmaMps/speedMps)
	return TimeGapEstimate{ValueS: ptr(thw), SigmaS: ptr(sigma)}, nil
}

// ObservedSurface is the along-path position of a face's observed returns:
// what the sensor saw of a surface, not where the body model puts it. It is a
// distinct type so that a body-model endpoint cannot be passed off as an
// observed surface by relabelling it (Section 8.3). A face with no returns is
// not an ObservedSurface; the caller suppresses with not_observed instead.
type ObservedSurface struct {
	TrackID          string        `json:"track_id"`
	CaptureUnixNanos int64         `json:"capture_unix_nanos"`
	Extremity        PathExtremity `json:"extremity"`
	ArcM             float64       `json:"arc_m"`
	SigmaM           float64       `json:"sigma_m"`
	PointCount       int           `json:"point_count"`
}

// ObservedSurfaceGap is the supplemental observed_surface_gap: the separation
// of two directly observed surfaces. It is sparse, has its own coverage and
// uncertainty, and never enters the published following distribution; its
// registry visibility is review_only.
func ObservedSurfaceGap(leader, follower ObservedSurface) (GapEstimate, error) {
	for _, c := range []struct {
		s    ObservedSurface
		want PathExtremity
	}{{leader, ExtremityTrailing}, {follower, ExtremityLeading}} {
		if c.s.TrackID == "" || c.s.Extremity != c.want {
			return GapEstimate{}, fmt.Errorf("observed surface of %q is %s, want %s", c.s.TrackID, c.s.Extremity, c.want)
		}
		if !finite(c.s.ArcM) || !finiteNonNegative(c.s.SigmaM) || c.s.PointCount <= 0 {
			return GapEstimate{}, fmt.Errorf("observed surface of %s needs a finite arc, a sigma and supporting returns", c.s.TrackID)
		}
	}
	if leader.TrackID == follower.TrackID || leader.CaptureUnixNanos != follower.CaptureUnixNanos {
		return GapEstimate{}, fmt.Errorf("observed surfaces must be two tracks at one instant")
	}
	g := GapEstimate{
		CaptureUnixNanos: leader.CaptureUnixNanos,
		ValueM:           leader.ArcM - follower.ArcM,
		SigmaM:           math.Hypot(leader.SigmaM, follower.SigmaM),
		LeaderSource:     EndpointDirectlyObserved,
		FollowerSource:   EndpointDirectlyObserved,
	}
	if !(g.ValueM > 0) {
		g.Reason = ReasonNonPositiveGap
	}
	return g, nil
}

// FollowingParams are the bounds the pointwise evaluation applies. None has a
// default: the speed floor is to be calibrated (Section 8.11 puts the noise
// floor near 0.3 m/s at 5 Hz) and the corridor is a site property, so a caller
// states them and they belong in the parameter hash.
type FollowingParams struct {
	// SpeedFloorMps is the follower speed below which net time gap is
	// suppressed.
	SpeedFloorMps float64 `json:"speed_floor_mps"`
	// CorridorHalfWidthM bounds each body centre's lateral offset from the
	// shared path.
	CorridorHalfWidthM float64 `json:"corridor_half_width_m"`
	// MaxRelativeHeadingRad bounds each body's heading against the path
	// tangent. It must be under a quarter turn, which is what "same direction"
	// means; opposing traffic is never a leader.
	MaxRelativeHeadingRad float64 `json:"max_relative_heading_rad"`
}

// Validate requires every bound, positive and finite.
func (p FollowingParams) Validate() error {
	if !(p.SpeedFloorMps > 0) || !(p.CorridorHalfWidthM > 0) || !(p.MaxRelativeHeadingRad > 0) ||
		!finite(p.SpeedFloorMps) || !finite(p.CorridorHalfWidthM) {
		return fmt.Errorf("following parameters must all be positive and finite")
	}
	if !(p.MaxRelativeHeadingRad < math.Pi/2) {
		return fmt.Errorf("maximum relative heading must be under a quarter turn")
	}
	return nil
}

// Party is one road user at the instant being evaluated.
type Party struct {
	Passage  Passage          `json:"passage"`
	Estimate EstimateIdentity `json:"estimate"`
	Sample   TrajectorySample `json:"sample"`
}

// PartyAt takes a trajectory's sample at an exact capture time.
func PartyAt(t Trajectory, captureUnixNanos int64) (Party, bool) {
	s, ok := t.SampleAt(captureUnixNanos)
	if !ok {
		return Party{}, false
	}
	return Party{Passage: t.Passage, Estimate: t.Estimate, Sample: s}, true
}

// FollowingPoint is one instant of one leader/follower pair: the projected
// bodies, the gap arithmetic, every reason that applied, and the measurements
// built from them.
type FollowingPoint struct {
	CaptureUnixNanos int64       `json:"capture_unix_nanos"`
	LeaderTrackID    string      `json:"leader_track_id"`
	FollowerTrackID  string      `json:"follower_track_id"`
	Leader           *BodyOnPath `json:"leader,omitempty"`
	Follower         *BodyOnPath `json:"follower,omitempty"`
	// Gap is present whenever both bodies projected; its value is review
	// material and is published only through SpatialGap.
	Gap *GapEstimate `json:"gap,omitempty"`
	// Reasons is every reason that applied, in precedence order. SpatialGap
	// carries the first that concerns it, NetTimeGap the first overall.
	Reasons    []SuppressionReason `json:"reasons,omitempty"`
	SpatialGap Measurement         `json:"spatial_gap"`
	NetTimeGap Measurement         `json:"net_time_gap"`
	// PredictedGap is present only when a party was not observed at this
	// instant. It is review-only: it never enters valid following time,
	// threshold exposure or an observed-gap series, and reacquisition does not
	// turn it into observation (Section 9.2).
	PredictedGap *Measurement `json:"predicted_gap,omitempty"`
	// CoastAgeNanos is the longest time since either party was last observed.
	CoastAgeNanos int64 `json:"coast_age_nanos,omitempty"`
	// SupportedOpportunity reports whether this instant may count toward
	// valid following time: net time gap is supported here, apart from
	// publication stage. Standstill is constrained time (Section 6, rule 2):
	// the spatial gap stays valid there, but the instant is not following
	// opportunity because the net time gap is undefined.
	SupportedOpportunity bool `json:"supported_opportunity"`
}

// predictedGapTolerable are the reasons a predicted gap may carry a review
// value through: the party was not observed (which is what a prediction is),
// the model is degraded but still reports a pose, the extent has not
// converged (the sigma says so), or the stage is not final.
var predictedGapTolerable = map[SuppressionReason]bool{
	ReasonNotObserved:        true,
	ReasonModelDegraded:      true,
	ReasonExtentNotConverged: true,
	ReasonEstimateNotFinal:   true,
}

// predictionWanted are the reasons that describe why a prediction is shown
// rather than why it cannot be.
var predictionWanted = map[SuppressionReason]bool{
	ReasonNotObserved:      true,
	ReasonEstimateNotFinal: true,
}

// EvaluateFollowing evaluates one leader/follower pair at one instant along a
// shared path. It does not choose the leader: which candidate is the nearest
// credible leader, and whether that choice is ambiguous, is the pairing step's
// decision, made before this is called.
//
// Every reason that applies is collected and reported in the vocabulary's
// precedence order, so the stored reason is the most fundamental one and a
// provisional run over non-final estimates still shows the physical reasons
// beneath estimate_not_final. Inputs that contradict themselves, or a pair
// from two estimator versions, are errors.
func EvaluateFollowing(path PathFrame, leader, follower Party, params FollowingParams) (FollowingPoint, error) {
	if err := params.Validate(); err != nil {
		return FollowingPoint{}, err
	}
	for _, p := range []Party{leader, follower} {
		if err := p.Passage.Validate(); err != nil {
			return FollowingPoint{}, err
		}
		if err := p.Estimate.Validate(); err != nil {
			return FollowingPoint{}, fmt.Errorf("track %s: %w", p.Passage.TrackID, err)
		}
	}
	if leader.Passage.TrackID == follower.Passage.TrackID {
		return FollowingPoint{}, fmt.Errorf("leader and follower are the same track %s", leader.Passage.TrackID)
	}
	if leader.Estimate != follower.Estimate {
		return FollowingPoint{}, fmt.Errorf("pair mixes estimator versions: %+v and %+v", leader.Estimate, follower.Estimate)
	}
	capture := follower.Sample.CaptureUnixNanos
	if leader.Sample.CaptureUnixNanos != capture {
		return FollowingPoint{}, fmt.Errorf("pair is not synchronised: %d and %d", leader.Sample.CaptureUnixNanos, capture)
	}

	pt := FollowingPoint{
		CaptureUnixNanos: capture,
		LeaderTrackID:    leader.Passage.TrackID,
		FollowerTrackID:  follower.Passage.TrackID,
	}
	var pairReasons reasonSet
	classReason, err := ClassApplicability(MetricFollowingSpatialGap, follower.Passage.MotionClass, leader.Passage.MotionClass)
	if err != nil {
		return FollowingPoint{}, err
	}
	pairReasons.add(classReason)

	var bodies [2]*BodyOnPath
	anyUnobserved := false
	for i, p := range []Party{leader, follower} {
		pairReasons.add(sampleReasons(p.Sample)...)
		if p.Sample.Support != SupportObserved {
			anyUnobserved = true
			pt.CoastAgeNanos = max(pt.CoastAgeNanos, capture-p.Sample.LastObservedUnixNanos)
		}
		body, reason, err := ProjectBody(path, p.Passage.TrackID, p.Sample)
		if err != nil {
			return FollowingPoint{}, err
		}
		pairReasons.add(reason)
		if reason != ReasonUnspecified {
			continue
		}
		if math.Abs(body.LateralM) > params.CorridorHalfWidthM ||
			math.Abs(body.RelativeHeadingRad) > params.MaxRelativeHeadingRad {
			pairReasons.add(ReasonNoCommonPath)
		}
		bodies[i] = &body
	}
	pt.Leader, pt.Follower = bodies[0], bodies[1]

	// Both reason sets start from everything that concerns the gap; the net
	// time gap adds the speed floor, which does not concern the gap at all.
	var thw TimeGapEstimate
	if pt.Leader != nil && pt.Follower != nil {
		gap, err := SpatialGap(pt.Leader.Trailing, pt.Follower.Leading)
		if err != nil {
			return FollowingPoint{}, err
		}
		pt.Gap = &gap
		pairReasons |= gapReasons(pt.Leader.Trailing, pt.Follower.Leading, gap.ValueM)
		thw, err = NetTimeGap(gap, pt.Follower.AlongSpeedMps, pt.Follower.AlongSpeedSigmaMps, params.SpeedFloorMps)
		if err != nil {
			return FollowingPoint{}, err
		}
	}
	thwReasons := pairReasons
	thwReasons.add(thw.Reason)
	pt.Reasons = thwReasons.ordered()

	prov := Provenance{
		Version: VersionProvenance{
			// A pair is only as final as its less final party.
			EstimateStage: min(leader.Sample.Stage, follower.Sample.Stage),
			EstimatorID:   follower.Estimate.EstimatorID,
			ObsModelID:    follower.Estimate.ObsModelID,
			MethodID:      FollowingMethodID,
			GeometryID:    path.GeometryID(),
			ParamHash:     follower.Estimate.ParamHash,
		},
		Input: InputProvenance{
			ContributingTrackIDs: []string{leader.Passage.TrackID, follower.Passage.TrackID},
			FirstUnixNanos:       capture,
			LastUnixNanos:        capture,
			PlanarFallback:       true,
		},
	}
	for _, p := range []Party{leader, follower} {
		if p.Sample.Support == SupportObserved {
			prov.Input.ObservedFrames++
		} else {
			prov.Input.CoastedFrames++
		}
	}

	// A reason-free gap set implies both bodies projected, so pt.Gap exists;
	// a reason-free time-gap set implies thw carries its value. Construction
	// errors therefore mean a broken invariant, and all of them are reported
	// together rather than the first alone.
	var errSpatial, errTime, errPredicted error
	pt.SpatialGap, errSpatial = measureOrSuppress(MetricFollowingSpatialGap, pairReasons.first(), prov, pt.Gap, nil)
	pt.NetTimeGap, errTime = measureOrSuppress(MetricFollowingNetTimeGap, thwReasons.first(), prov, nil, &thw)
	pt.SupportedOpportunity = thwReasons.onlyWithin(map[SuppressionReason]bool{ReasonEstimateNotFinal: true})

	if anyUnobserved {
		reason := pairReasons.firstOutside(predictedGapTolerable)
		if reason == ReasonUnspecified && pt.Gap == nil {
			// Every body-level reason here is tolerable (an absent extent),
			// but without two bodies there is no gap to predict. not_observed
			// is what makes a prediction wanted, so it cannot be why there is
			// none: report what stopped a body projecting.
			reason = pairReasons.firstOutside(predictionWanted)
		}
		if reason == ReasonUnspecified && !(leader.Sample.reviewPoseAllowed() && follower.Sample.reviewPoseAllowed()) {
			// model_invalid is tolerated as a reason but has no pose to show.
			reason = ReasonModelDegraded
		}
		var predicted Measurement
		predicted, errPredicted = measureOrSuppress(MetricFollowingPredictedGap, reason, prov, pt.Gap, nil)
		pt.PredictedGap = &predicted
	}
	if err := errors.Join(errSpatial, errTime, errPredicted); err != nil {
		return FollowingPoint{}, err
	}
	return pt, nil
}

// measureOrSuppress builds a suppression when a reason applies, and otherwise
// a measurement from whichever estimate was supplied. A reason-free call with
// no value to read is a programming error and is reported as one rather than
// dereferenced.
func measureOrSuppress(id MetricID, reason SuppressionReason, prov Provenance, gap *GapEstimate, thw *TimeGapEstimate) (Measurement, error) {
	if reason != ReasonUnspecified {
		return NewSuppressedMeasurement(id, reason, prov)
	}
	switch {
	case gap != nil:
		return NewMeasurement(id, gap.ValueM, SigmaUncertainty(gap.SigmaM, MethodLinearised), prov)
	case thw != nil && thw.ValueS != nil && thw.SigmaS != nil:
		return NewMeasurement(id, *thw.ValueS, SigmaUncertainty(*thw.SigmaS, MethodLinearised), prov)
	}
	return Measurement{}, fmt.Errorf("metric %s has no reason and no value", id)
}

// reasonSet collects suppression reasons and reports them in precedence
// order, which is their declaration order.
type reasonSet uint32

func (r *reasonSet) add(reasons ...SuppressionReason) {
	for _, reason := range reasons {
		if reason != ReasonUnspecified {
			*r |= 1 << reason
		}
	}
}

func (r reasonSet) ordered() []SuppressionReason {
	var out []SuppressionReason
	for _, reason := range SuppressionReasons() {
		if r&(1<<reason) != 0 {
			out = append(out, reason)
		}
	}
	return out
}

func (r reasonSet) first() SuppressionReason {
	return r.firstOutside(nil)
}

func (r reasonSet) firstOutside(tolerated map[SuppressionReason]bool) SuppressionReason {
	for _, reason := range r.ordered() {
		if !tolerated[reason] {
			return reason
		}
	}
	return ReasonUnspecified
}

func (r reasonSet) onlyWithin(allowed map[SuppressionReason]bool) bool {
	return r.firstOutside(allowed) == ReasonUnspecified
}

func kinkSign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func sq(v float64) float64 { return v * v }
