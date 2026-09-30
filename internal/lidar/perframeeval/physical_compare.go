package perframeeval

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// CentreComparison is a prediction's point against the reference body
// centre. Along and across the reference axis the signs are meaningful only
// when the axis is resolved; under an ambiguous axis only magnitudes are.
type CentreComparison struct {
	ErrorM              float64     `json:"error_m"`
	DXM                 float64     `json:"dx_m"`
	DYM                 float64     `json:"dy_m"`
	LongitudinalM       *float64    `json:"longitudinal_m,omitempty"`
	LateralM            *float64    `json:"lateral_m,omitempty"`
	AlongAxisAbsM       *float64    `json:"along_axis_abs_m,omitempty"`
	AcrossAxisAbsM      *float64    `json:"across_axis_abs_m,omitempty"`
	ReferenceBoundM     float64     `json:"reference_bound_m"`
	PredictionSigmaM    float64     `json:"prediction_sigma_m"`
	PredictionReference string      `json:"prediction_reference"`
	Step                *CentreStep `json:"step,omitempty"`
}

// CentreStep compares how far the prediction and the reference moved since
// the object's previous scored instant. True motion moves both; an apparent
// step at a face transition moves only the prediction.
type CentreStep struct {
	FromSampleID    int     `json:"from_sample_id"`
	ReferenceMoveM  float64 `json:"reference_move_m"`
	PredictionMoveM float64 `json:"prediction_move_m"`
	ErrorM          float64 `json:"error_m"`
	SameTrack       bool    `json:"same_track"`
}

// YawComparison is the heading error. With either axis unresolved it is an
// axis error, modulo half a turn, and says so.
type YawComparison struct {
	ErrorRad           float64 `json:"error_rad"`
	AxisOnly           bool    `json:"axis_only"`
	ReferenceBoundRad  float64 `json:"reference_bound_rad"`
	PredictionSigmaRad float64 `json:"prediction_sigma_rad"`
}

// DimensionComparison is one body dimension against its reference interval.
type DimensionComparison struct {
	PredictedM          float64 `json:"predicted_m"`
	PredictedSigmaM     float64 `json:"predicted_sigma_m"`
	PredictedProvenance string  `json:"predicted_provenance"`
	PredictedEvidence   bool    `json:"predicted_evidence"`
	ReferenceLowerM     float64 `json:"reference_lower_m"`
	ReferenceUpperM     float64 `json:"reference_upper_m"`
	ReferenceValueM     float64 `json:"reference_value_m"`
	ReferenceHalfWidthM float64 `json:"reference_half_width_m"`
	// ErrorM is predicted minus the reference value; OutsideBoundM is how
	// far beyond the interval the prediction lies, signed, zero inside it.
	ErrorM        float64 `json:"error_m"`
	OutsideBoundM float64 `json:"outside_bound_m"`
}

// LowerBoundCheck reports a prediction against a partial span's lower bound.
// It is not a dimension error: the rest of the body was not seen.
type LowerBoundCheck struct {
	LowerM     float64 `json:"lower_m"`
	PredictedM float64 `json:"predicted_m"`
	ShortfallM float64 `json:"shortfall_m"`
}

// EndpointComparison is a predicted bumper against the reference bumper,
// along and across the reference axis.
type EndpointComparison struct {
	LongitudinalErrorM float64 `json:"longitudinal_error_m"`
	LateralErrorM      float64 `json:"lateral_error_m"`
	DistanceM          float64 `json:"distance_m"`
	ReferenceBoundM    float64 `json:"reference_bound_m"`
}

// EndsComparison pairs the two predicted ends with the two reference ends in
// whichever order fits better, and reports the mean distance. It has no sign.
type EndsComparison struct {
	MeanDistanceM   float64 `json:"mean_distance_m"`
	ReferenceBoundM float64 `json:"reference_bound_m"`
}

// BoxComparison is the footprint overlap of two complete boxes.
type BoxComparison struct {
	IoU float64 `json:"iou"`
}

// PhysicalComparison holds every comparison made at one instant.
type PhysicalComparison struct {
	Centre      *CentreComparison                     `json:"centre,omitempty"`
	Yaw         *YawComparison                        `json:"yaw,omitempty"`
	Length      *DimensionComparison                  `json:"length,omitempty"`
	Width       *DimensionComparison                  `json:"width,omitempty"`
	Height      *DimensionComparison                  `json:"height,omitempty"`
	LowerBounds map[PhysicalComponent]LowerBoundCheck `json:"lower_bounds,omitempty"`
	Front       *EndpointComparison                   `json:"front,omitempty"`
	Rear        *EndpointComparison                   `json:"rear,omitempty"`
	Ends        *EndsComparison                       `json:"ends_unsigned,omitempty"`
	Box         *BoxComparison                        `json:"box,omitempty"`
}

// PhysicalInstant is one expected object at one sample: the reference layer,
// the prediction layer and their comparison, kept apart, with the reference
// revision and estimate version they came from, so a record read on its own
// cannot be mistaken for another instant's or another version's.
type PhysicalInstant struct {
	EpisodeID         string                        `json:"episode_id"`
	ObjectID          string                        `json:"object_id"`
	SampleID          int                           `json:"sample_id"`
	TimestampNs       int64                         `json:"timestamp_ns"`
	ReferenceRevision int                           `json:"reference_revision"`
	Estimate          string                        `json:"estimate"`
	Reference         *annotation.PhysicalGeometry  `json:"reference,omitempty"`
	Prediction        *PredictedBody                `json:"prediction,omitempty"`
	Match             *PhysicalMatch                `json:"match,omitempty"`
	Comparison        PhysicalComparison            `json:"comparison"`
	Outcomes          map[PhysicalComponent]Outcome `json:"outcomes"`
}

// PredictedGap is the gap between two matched predictions along the chosen
// axis, from the behaviour layer's projected footprint extremes.
type PredictedGap struct {
	ValueM         float64 `json:"value_m"`
	SigmaM         float64 `json:"sigma_m"`
	FollowerTrack  string  `json:"follower_track"`
	LeaderTrack    string  `json:"leader_track"`
	Axis           string  `json:"axis"`
	AxisRad        float64 `json:"axis_rad"`
	FollowerSource string  `json:"follower_source"`
	LeaderSource   string  `json:"leader_source"`
	// PredictionReason is the behaviour layer's own suppression of this gap,
	// if any: the gap is still compared, and the reason shown beside it.
	PredictionReason string `json:"prediction_reason,omitempty"`
}

// PhysicalFollowingInstant is one sample of one following reference.
type PhysicalFollowingInstant struct {
	EpisodeID         string                       `json:"episode_id"`
	FollowingID       string                       `json:"following_id"`
	FollowerObjectID  string                       `json:"follower_object_id"`
	LeaderObjectID    string                       `json:"leader_object_id,omitempty"`
	Decision          annotation.FollowingDecision `json:"decision"`
	SampleID          int                          `json:"sample_id"`
	TimestampNs       int64                        `json:"timestamp_ns"`
	ReferenceRevision int                          `json:"reference_revision"`
	Estimate          string                       `json:"estimate"`
	ReferenceGap      *annotation.FollowingGap     `json:"reference_gap,omitempty"`
	PredictedGap      *PredictedGap                `json:"predicted_gap,omitempty"`
	ErrorM            *float64                     `json:"error_m,omitempty"`
	OutsideBoundM     *float64                     `json:"outside_bound_m,omitempty"`
	Outcome           Outcome                      `json:"outcome"`
}

// ComponentAccounting is where every expected instant of one component went.
type ComponentAccounting struct {
	Expected int `json:"expected"`
	Scored   int `json:"scored"`
	// Unscored is category, then reason, then count.
	Unscored map[string]map[string]int `json:"unscored"`
}

func (a *ComponentAccounting) add(o Outcome) {
	a.Expected++
	if o.Category == OutcomeScored {
		a.Scored++
		return
	}
	if a.Unscored == nil {
		a.Unscored = map[string]map[string]int{}
	}
	if a.Unscored[o.Category] == nil {
		a.Unscored[o.Category] = map[string]int{}
	}
	a.Unscored[o.Category][o.Reason]++
}

// Complete reports whether every expected instant is scored or counted
// under a named category.
func (a ComponentAccounting) Complete() bool {
	if _, unnamed := a.Unscored[""]; unnamed {
		return false
	}
	n := a.Scored
	for _, reasons := range a.Unscored {
		for _, c := range reasons {
			n += c
		}
	}
	return n == a.Expected
}

// PhysicalAccounting is the complete accounting of an arm's physical score.
type PhysicalAccounting struct {
	Components map[PhysicalComponent]ComponentAccounting `json:"components"`
	Following  ComponentAccounting                       `json:"following"`
}

// ComponentSummary pools one component's scored errors: metres, or radians
// for yaw. Within counts errors no larger than the reference's own bound,
// which is a statement about this reference, not a calibrated coverage.
type ComponentSummary struct {
	Scored               int     `json:"scored"`
	MeanAbsError         float64 `json:"mean_abs_error"`
	RMSError             float64 `json:"rms_error"`
	MaxAbsError          float64 `json:"max_abs_error"`
	MeanReferenceBound   float64 `json:"mean_reference_bound"`
	WithinReferenceBound int     `json:"within_reference_bound"`
}

// PhysicalSummary pools an arm's scored comparisons.
type PhysicalSummary struct {
	Components map[PhysicalComponent]ComponentSummary `json:"components"`
	// CentreByPredictionReference splits centre errors by what the
	// prediction's point was, since a medoid is not a body centre.
	CentreByPredictionReference map[string]ComponentSummary `json:"centre_by_prediction_reference"`
	BoxScored                   int                         `json:"box_scored"`
	MeanBoxIoU                  float64                     `json:"mean_box_iou"`
	MinBoxIoU                   float64                     `json:"min_box_iou"`
	Following                   ComponentSummary            `json:"following_gap"`
}

// PhysicalResult is one estimate version scored against one physical
// reference.
type PhysicalResult struct {
	Schema        string                    `json:"schema"`
	SchemaVersion int                       `json:"schema_version"`
	Reference     PhysicalReferenceIdentity `json:"reference"`
	Arm           ArmIdentity               `json:"arm"`
	// ArmCalibrations are the calibrations the arm's rows were made under:
	// one, since an arm that mixes them is refused.
	ArmCalibrations []string                   `json:"arm_calibration_ids"`
	Instants        []PhysicalInstant          `json:"instants"`
	Following       []PhysicalFollowingInstant `json:"following"`
	Accounting      PhysicalAccounting         `json:"accounting"`
	Summary         PhysicalSummary            `json:"summary"`
	Caveats         []string                   `json:"caveats"`
}

// Instant returns the record for one object at one sample of one episode.
func (r PhysicalResult) Instant(episodeID, objectID string, sampleID int) (PhysicalInstant, bool) {
	for _, in := range r.Instants {
		if in.EpisodeID == episodeID && in.ObjectID == objectID && in.SampleID == sampleID {
			return in, true
		}
	}
	return PhysicalInstant{}, false
}

// ScorePhysical scores one estimate version against the physical reference.
// An arm whose rows name another sensor, or another calibration than the
// reference states, is refused.
func ScorePhysical(pr *PhysicalReference, arm PhysicalArm) (PhysicalResult, error) {
	if err := checkSource(pr.Source, arm); err != nil {
		return PhysicalResult{}, err
	}
	s := physicalScorer{
		pr: pr, aligned: pr.alignPredictions(arm.Bodies), matches: map[int]sampleMatches{},
		estimate: fmt.Sprintf("%s/%s/%s/%s/%s", arm.Identity.SourceID, arm.Identity.EstimatorID,
			arm.Identity.ObservationModelID, arm.Identity.ParamHash, arm.Identity.Stage),
	}
	res := PhysicalResult{
		Schema: PhysicalScoreSchema, SchemaVersion: PhysicalScoreSchemaVersion, Reference: pr.Identity, Arm: arm.Identity,
		ArmCalibrations: arm.CalibrationIDs,
		Accounting:      PhysicalAccounting{Components: map[PhysicalComponent]ComponentAccounting{}},
	}
	for _, e := range pr.instants {
		res.Instants = append(res.Instants, s.instant(e))
	}
	addSteps(res.Instants)
	for _, e := range pr.following {
		f, err := s.following(e)
		if err != nil {
			return PhysicalResult{}, err
		}
		res.Following = append(res.Following, f)
	}
	for _, in := range res.Instants {
		for _, c := range PhysicalComponents() {
			a := res.Accounting.Components[c]
			a.add(in.Outcomes[c])
			res.Accounting.Components[c] = a
		}
	}
	for _, f := range res.Following {
		res.Accounting.Following.add(f.Outcome)
	}
	res.Summary = summarisePhysical(res)
	res.Caveats = physicalCaveats(res)
	if n := len(s.pr.drifted); n > 0 {
		keys := make([]string, 0, n)
		for k := range s.pr.drifted {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d reviewed record(s) were reviewed against other membership than revision %d "+
			"scores, in frames they rest on, and are not scored: %s. Review them again against the scored revision.",
			n, res.Reference.SidecarRevision, strings.Join(keys, ", ")))
	}
	if s.pr.unpinnedReviews > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d reviewed record(s) carry no membership pin (reviewed before pins were "+
			"recorded, or imported): a membership change since their review cannot be detected.", s.pr.unpinnedReviews))
	}
	if s.pr.FrozenWithoutPhysicalPin {
		res.Caveats = append(res.Caveats, fmt.Sprintf("The frozen split pins no physical revision for this pack: revision %d was the pack's current references when scored, and a later save would change this score; freeze a new split revision to pin it.", res.Reference.PhysicalRevision))
	}
	return res, nil
}

type physicalScorer struct {
	pr       *PhysicalReference
	aligned  map[int][]alignedPrediction
	matches  map[int]sampleMatches
	estimate string
}

func (s *physicalScorer) match(sample int) sampleMatches {
	m, ok := s.matches[sample]
	if !ok {
		m = s.pr.matchSample(sample, s.aligned[sample])
		s.matches[sample] = m
	}
	return m
}

// referenceReason is why a keyframe cannot be truth, or empty.
func referenceReason(g annotation.PhysicalGeometry, keyed bool, drifted map[string]bool) string {
	switch {
	case !keyed:
		return ReasonNoKeyframe
	case drifted["keyframe/"+g.KeyframeID]:
		return ReasonReferenceMembershipDrift
	case g.Origin != annotation.OriginIndependent:
		return ReasonReferenceTrackerAssisted
	case g.ReviewStatus != annotation.StatusReviewed:
		return ReasonReferenceUnreviewed
	}
	return ""
}

func (s *physicalScorer) instant(e expectedInstant) PhysicalInstant {
	in := PhysicalInstant{
		EpisodeID: e.episode, ObjectID: e.object, SampleID: e.sample, TimestampNs: s.pr.samples[e.sample].TimestampNs,
		ReferenceRevision: s.pr.Identity.PhysicalRevision, Estimate: s.estimate,
		Outcomes: map[PhysicalComponent]Outcome{},
	}
	g, keyed := s.pr.geometry[e.object][e.sample]
	if keyed {
		// Shown whatever its review: the reference layer displays
		// proposals too, and the outcomes say they were not scored.
		in.Reference = &g
	}
	if reason := referenceReason(g, keyed, s.pr.drifted); reason != "" {
		for _, c := range PhysicalComponents() {
			in.Outcomes[c] = Outcome{Category: OutcomeUnknownGeometry, Reason: reason}
		}
		return in
	}

	m := s.match(e.sample)
	idx, matched := m.matched[e.object]
	var pending Outcome
	_, _, positioned := referencePosition(g)
	switch {
	case !positioned:
		pending = Outcome{Category: OutcomeUnmatched, Reason: ReasonNoReferencePosition}
	case len(m.predictions) == 0:
		pending = Outcome{Category: OutcomeMissingPrediction, Reason: ReasonNoPredictionAtInstant}
	case !matched && m.candidates[e.object] == 0:
		pending = Outcome{Category: OutcomeUnmatched, Reason: ReasonNoPredictionWithinGate}
	case !matched:
		pending = Outcome{Category: OutcomeUnmatched, Reason: ReasonPredictionTaken}
	default:
		a := m.predictions[idx]
		in.Prediction = &a.body
		px, py := predictionPosition(a.body)
		rx, ry, _ := referencePosition(g)
		in.Match = &PhysicalMatch{TrackKey: a.body.TrackKey, DistanceM: math.Hypot(px-rx, py-ry),
			OffsetNanos: a.offset, Candidates: m.candidates[e.object]}
	}

	unavailable := map[PhysicalComponent]string{
		ComponentCentre: g.CentreUnavailable, ComponentYaw: g.YawUnavailable,
		ComponentLength: g.LengthUnavailable, ComponentWidth: g.WidthUnavailable, ComponentHeight: g.HeightUnavailable,
		ComponentFront: signedEndUnavailable(g, g.FrontUnavailable), ComponentRear: signedEndUnavailable(g, g.RearUnavailable),
		ComponentEnds: endsUnavailable(g),
		ComponentBox:  g.BoxUnavailable,
	}
	for _, c := range PhysicalComponents() {
		switch {
		case unavailable[c] != "":
			in.Outcomes[c] = Outcome{Category: OutcomeUnknownGeometry, Reason: unavailable[c]}
			if unavailable[c] == annotation.UnavailableLowerBoundOnly && in.Prediction != nil {
				s.lowerBound(&in, c)
			}
		case pending.Category != "":
			in.Outcomes[c] = pending
		default:
			if reason := compare(&in, g, c); reason != "" {
				in.Outcomes[c] = Outcome{Category: OutcomeMissingPrediction, Reason: reason}
			} else {
				in.Outcomes[c] = Outcome{Category: OutcomeScored}
			}
		}
	}
	return in
}

// signedEndUnavailable is why a signed bumper cannot be scored: the
// geometry's own reason, or a missing yaw, since the bumper's error is taken
// along the reference axis.
func signedEndUnavailable(g annotation.PhysicalGeometry, reason string) string {
	if reason == "" && g.Yaw == nil {
		return annotation.UnavailableYaw
	}
	return reason
}

func endsUnavailable(g annotation.PhysicalGeometry) string {
	switch {
	case len(g.Ends) > 0:
		return ""
	case g.CentreUnavailable != "":
		return g.CentreUnavailable
	case g.YawUnavailable != "":
		return g.YawUnavailable
	}
	return g.LengthUnavailable
}

// lowerBound checks a prediction against a partial span without scoring it.
func (s *physicalScorer) lowerBound(in *PhysicalInstant, c PhysicalComponent) {
	body := s.pr.bodies[in.ObjectID]
	var d annotation.DimensionBound
	var p *PredictedExtent
	switch c {
	case ComponentLength:
		d, p = body.Length, in.Prediction.Length
	case ComponentWidth:
		d, p = body.Width, in.Prediction.Width
	default:
		d, p = body.Height, in.Prediction.Height
	}
	if p == nil {
		return
	}
	if in.Comparison.LowerBounds == nil {
		in.Comparison.LowerBounds = map[PhysicalComponent]LowerBoundCheck{}
	}
	in.Comparison.LowerBounds[c] = LowerBoundCheck{LowerM: *d.LowerM, PredictedM: p.Metres, ShortfallM: math.Max(*d.LowerM-p.Metres, 0)}
}

// compare fills one component's comparison, or says what the prediction
// lacks.
func compare(in *PhysicalInstant, g annotation.PhysicalGeometry, c PhysicalComponent) string {
	p := in.Prediction
	cmp := &in.Comparison
	switch c {
	case ComponentCentre:
		// The distance is kept either way: for a point that is not on the
		// body it says where that point sits, and is reported only by
		// prediction reference, never as a body-centre error.
		cmp.Centre = compareCentre(p, g)
		if !p.Physical {
			return ReasonPredictionNotOnBody
		}
	case ComponentYaw:
		if p.Heading == nil {
			return ReasonPredictionNoHeading
		}
		// A signed heading error needs both ends named on both sides.
		diff, axisOnly := p.Heading.Rad-g.Yaw.Rad, !(p.Heading.Resolved && g.Yaw.Axis == annotation.AxisResolved)
		if axisOnly {
			diff = wrapHalfTurn(diff)
		} else {
			diff = math.Remainder(diff, 2*math.Pi)
		}
		cmp.Yaw = &YawComparison{ErrorRad: diff, AxisOnly: axisOnly, ReferenceBoundRad: g.Yaw.BoundRad, PredictionSigmaRad: p.Heading.SigmaRad}
	case ComponentLength, ComponentWidth, ComponentHeight:
		ref, pred, into := g.Length, p.Length, &cmp.Length
		if c == ComponentWidth {
			ref, pred, into = g.Width, p.Width, &cmp.Width
		} else if c == ComponentHeight {
			ref, pred, into = g.Height, p.Height, &cmp.Height
		}
		if pred == nil {
			return ReasonPredictionNoExtent
		}
		*into = compareDimension(*pred, *ref)
	case ComponentFront, ComponentRear:
		sign, ref, into := 1.0, g.Front, &cmp.Front
		if c == ComponentRear {
			sign, ref, into = -1, g.Rear, &cmp.Rear
		}
		if reason := predictionEndsReason(p, true); reason != "" {
			return reason
		}
		*into = compareEndpoint(predictedEnd(p, sign), *ref, g.Yaw.Rad)
	case ComponentEnds:
		if reason := predictionEndsReason(p, false); reason != "" {
			return reason
		}
		cmp.Ends = compareEnds(p, g.Ends)
	case ComponentBox:
		if !p.Physical || p.Heading == nil || p.Length == nil || p.Width == nil {
			return ReasonPredictionIncompleteBox
		}
		cmp.Box = &BoxComparison{IoU: boxIoU(*g.Box, annotation.BoxBound{
			CentreXM: p.CentreXM, CentreYM: p.CentreYM, YawRad: p.Heading.Rad, LengthM: p.Length.Metres, WidthM: p.Width.Metres,
		})}
	}
	return ""
}

func compareCentre(p *PredictedBody, g annotation.PhysicalGeometry) *CentreComparison {
	px, py := predictionPosition(*p)
	dx, dy := px-g.Centre.XM, py-g.Centre.YM
	c := &CentreComparison{
		ErrorM: math.Hypot(dx, dy), DXM: dx, DYM: dy, ReferenceBoundM: g.Centre.BoundM,
		PredictionSigmaM: p.PositionSigmaM, PredictionReference: p.Reference,
	}
	if g.Yaw != nil {
		ux, uy := math.Cos(g.Yaw.Rad), math.Sin(g.Yaw.Rad)
		along, across := dx*ux+dy*uy, -dx*uy+dy*ux
		absAlong, absAcross := math.Abs(along), math.Abs(across)
		c.AlongAxisAbsM, c.AcrossAxisAbsM = &absAlong, &absAcross
		if g.Yaw.Axis == annotation.AxisResolved {
			c.LongitudinalM, c.LateralM = &along, &across
		}
	}
	return c
}

func compareDimension(p PredictedExtent, ref annotation.LinearBound) *DimensionComparison {
	d := &DimensionComparison{
		PredictedM: p.Metres, PredictedSigmaM: p.SigmaMetres, PredictedProvenance: p.Provenance, PredictedEvidence: p.Evidence,
		ReferenceLowerM: ref.LowerM, ReferenceUpperM: ref.UpperM, ReferenceValueM: ref.ValueM, ReferenceHalfWidthM: ref.HalfWidthM,
		ErrorM: p.Metres - ref.ValueM,
	}
	switch {
	case p.Metres > ref.UpperM:
		d.OutsideBoundM = p.Metres - ref.UpperM
	case p.Metres < ref.LowerM:
		d.OutsideBoundM = p.Metres - ref.LowerM
	}
	return d
}

// predictionEndsReason says why a prediction cannot place its bumpers: it
// needs a point on the body, a heading, a length and, for a signed bumper, a
// resolved heading.
func predictionEndsReason(p *PredictedBody, signed bool) string {
	switch {
	case !p.Physical:
		return ReasonPredictionNotOnBody
	case p.Heading == nil:
		return ReasonPredictionNoHeading
	case signed && !p.Heading.Resolved:
		return ReasonPredictionAxisAmbiguous
	case p.Length == nil:
		return ReasonPredictionNoExtent
	}
	return ""
}

// predictedEnd is half the predicted length from its centre, forward for +1.
func predictedEnd(p *PredictedBody, sign float64) [2]float64 {
	half := p.Length.Metres / 2
	return [2]float64{p.CentreXM + sign*half*math.Cos(p.Heading.Rad), p.CentreYM + sign*half*math.Sin(p.Heading.Rad)}
}

func compareEndpoint(pred [2]float64, ref annotation.PlanarBound, yaw float64) *EndpointComparison {
	dx, dy := pred[0]-ref.XM, pred[1]-ref.YM
	ux, uy := math.Cos(yaw), math.Sin(yaw)
	return &EndpointComparison{
		LongitudinalErrorM: dx*ux + dy*uy, LateralErrorM: -dx*uy + dy*ux,
		DistanceM: math.Hypot(dx, dy), ReferenceBoundM: ref.BoundM,
	}
}

func compareEnds(p *PredictedBody, ref []annotation.PlanarBound) *EndsComparison {
	a, b := predictedEnd(p, 1), predictedEnd(p, -1)
	dist := func(p [2]float64, r annotation.PlanarBound) float64 { return math.Hypot(p[0]-r.XM, p[1]-r.YM) }
	straight := (dist(a, ref[0]) + dist(b, ref[1])) / 2
	crossed := (dist(a, ref[1]) + dist(b, ref[0])) / 2
	return &EndsComparison{MeanDistanceM: math.Min(straight, crossed), ReferenceBoundM: math.Max(ref[0].BoundM, ref[1].BoundM)}
}

// wrapHalfTurn folds an angle difference into [-pi/2, pi/2]: the error
// between two axes, whichever way each points.
func wrapHalfTurn(a float64) float64 { return math.Remainder(a, math.Pi) }

// addSteps attaches to each scored centre the move since the object's
// previous scored centre in the same episode.
func addSteps(instants []PhysicalInstant) {
	type key struct{ episode, object string }
	last := map[key]int{}
	for i := range instants {
		in := &instants[i]
		if in.Comparison.Centre == nil {
			continue
		}
		k := key{in.EpisodeID, in.ObjectID}
		if j, ok := last[k]; ok {
			prev := instants[j]
			ref := math.Hypot(in.Reference.Centre.XM-prev.Reference.Centre.XM, in.Reference.Centre.YM-prev.Reference.Centre.YM)
			px, py := predictionPosition(*in.Prediction)
			qx, qy := predictionPosition(*prev.Prediction)
			in.Comparison.Centre.Step = &CentreStep{
				FromSampleID: prev.SampleID, ReferenceMoveM: ref, PredictionMoveM: math.Hypot(px-qx, py-qy),
				ErrorM: math.Hypot((px-qx)-(in.Reference.Centre.XM-prev.Reference.Centre.XM),
					(py-qy)-(in.Reference.Centre.YM-prev.Reference.Centre.YM)),
				SameTrack: in.Match.TrackKey == prev.Match.TrackKey,
			}
		}
		last[k] = i
	}
}

// following scores one sample of one following reference. The gap is taken
// along the reference follower's axis, through the behaviour layer's own
// projection, so a predicted gap here is the one it would report. A gap whose
// follower has no truth keyframe with a resolved axis at that sample has no
// axis to be measured along: the reference validation already refuses a named
// follower front there, and anything else is counted, not measured along the
// prediction's own heading.
func (s *physicalScorer) following(e expectedFollowing) (PhysicalFollowingInstant, error) {
	f := e.ref
	out := PhysicalFollowingInstant{
		EpisodeID: e.episode, FollowingID: f.FollowingID, FollowerObjectID: f.FollowerObjectID, LeaderObjectID: f.LeaderObjectID,
		Decision: f.Decision, SampleID: e.sample, TimestampNs: s.pr.samples[e.sample].TimestampNs,
		ReferenceRevision: s.pr.Identity.PhysicalRevision, Estimate: s.estimate,
	}
	set := func(category, reason string) (PhysicalFollowingInstant, error) {
		out.Outcome = Outcome{Category: category, Reason: reason}
		return out, nil
	}
	switch {
	case f.Review.Origin != annotation.OriginIndependent:
		return set(OutcomeUnknownGeometry, ReasonReferenceTrackerAssisted)
	case f.Review.Status != annotation.StatusReviewed:
		return set(OutcomeUnknownGeometry, ReasonReferenceUnreviewed)
	case f.Decision != annotation.FollowingLeader:
		return set(OutcomeNotFollowing, string(f.Decision))
	}
	for i := range f.Gaps {
		if f.Gaps[i].SampleID == e.sample {
			out.ReferenceGap = &f.Gaps[i]
		}
	}
	axis, hasAxis := s.pr.geometry[f.FollowerObjectID][e.sample]
	switch {
	case out.ReferenceGap == nil:
		return set(OutcomeUnknownGeometry, ReasonNoGapReference)
	case !out.ReferenceGap.Status.Scorable():
		return set(OutcomeUnknownGeometry, string(out.ReferenceGap.Status))
	case !hasAxis || !axis.Truth || axis.Yaw == nil || axis.Yaw.Axis != annotation.AxisResolved:
		return set(OutcomeUnknownGeometry, ReasonFollowerAxisUnavailable)
	case !e.leaderInEpisode:
		// The leader is not an object this episode scores, perhaps one in
		// another split: its reference is not used here.
		return set(OutcomeUnknownGeometry, ReasonLeaderOutsideEpisode)
	}
	if lg, ok := s.pr.geometry[f.LeaderObjectID][e.sample]; !ok || !lg.Truth {
		return set(OutcomeUnknownGeometry, ReasonNoLeaderKeyframe)
	}
	m := s.match(e.sample)
	fi, followerOK := m.matched[f.FollowerObjectID]
	li, leaderOK := m.matched[f.LeaderObjectID]
	switch {
	case len(m.predictions) == 0:
		return set(OutcomeMissingPrediction, ReasonNoPredictionAtInstant)
	case !followerOK:
		return set(OutcomeUnmatched, ReasonFollowerUnmatched)
	case !leaderOK:
		return set(OutcomeUnmatched, ReasonLeaderUnmatched)
	}
	fb, lb := m.predictions[fi].body, m.predictions[li].body
	gap := &PredictedGap{FollowerTrack: fb.TrackKey, LeaderTrack: lb.TrackKey,
		Axis: "reference_follower_axis", AxisRad: axis.Yaw.Rad}
	if fb.TimestampNs != lb.TimestampNs {
		return set(OutcomeMissingPrediction, ReasonPredictionInstantsDiffer)
	}
	fx, fy := predictionPosition(fb)
	path := l8behaviour.StraightPath{
		ID:      fmt.Sprintf("physical-gap/%s/%d", f.FollowingID, e.sample),
		OriginX: fx - 1000*math.Cos(gap.AxisRad), OriginY: fy - 1000*math.Sin(gap.AxisRad),
		HeadingRad: gap.AxisRad, LengthM: 2000,
	}
	follower, reason, err := l8behaviour.ProjectBody(path, fb.TrackKey, fb.sample)
	if err != nil {
		return out, fmt.Errorf("follower %s at sample %d: %w", fb.TrackKey, e.sample, err)
	}
	if reason != l8behaviour.ReasonUnspecified {
		return set(OutcomeMissingPrediction, "follower_"+reason.String())
	}
	leader, reason, err := l8behaviour.ProjectBody(path, lb.TrackKey, lb.sample)
	if err != nil {
		return out, fmt.Errorf("leader %s at sample %d: %w", lb.TrackKey, e.sample, err)
	}
	if reason != l8behaviour.ReasonUnspecified {
		return set(OutcomeMissingPrediction, "leader_"+reason.String())
	}
	est, err := l8behaviour.SpatialGap(leader.Trailing, follower.Leading)
	if err != nil {
		return out, fmt.Errorf("gap at sample %d: %w", e.sample, err)
	}
	gap.ValueM, gap.SigmaM = est.ValueM, est.SigmaM
	gap.FollowerSource, gap.LeaderSource = est.FollowerSource.String(), est.LeaderSource.String()
	if est.Reason != l8behaviour.ReasonUnspecified {
		gap.PredictionReason = est.Reason.String()
	}
	out.PredictedGap = gap
	ref := out.ReferenceGap
	errM, outside := gap.ValueM-referenceGapValue(*ref), 0.0
	switch {
	case gap.ValueM > *ref.UpperM:
		outside = gap.ValueM - *ref.UpperM
	case gap.ValueM < *ref.LowerM:
		outside = gap.ValueM - *ref.LowerM
	}
	out.ErrorM, out.OutsideBoundM = &errM, &outside
	return set(OutcomeScored, "")
}

// referenceGapValue is the gap a reference quotes: its value, or the middle
// of its interval.
func referenceGapValue(ref annotation.FollowingGap) float64 {
	if ref.ValueM != nil {
		return *ref.ValueM
	}
	return (*ref.LowerM + *ref.UpperM) / 2
}

// boxIoU is the intersection over union of two rotated rectangles.
func boxIoU(a, b annotation.BoxBound) float64 {
	pa, pb := boxCorners(a), boxCorners(b)
	inter := polygonArea(clipConvex(pa, pb))
	union := polygonArea(pa) + polygonArea(pb) - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

// boxCorners lists a box's corners anticlockwise.
func boxCorners(b annotation.BoxBound) [][2]float64 {
	c, s := math.Cos(b.YawRad), math.Sin(b.YawRad)
	hl, hw := b.LengthM/2, b.WidthM/2
	out := make([][2]float64, 0, 4)
	for _, k := range [][2]float64{{hl, -hw}, {hl, hw}, {-hl, hw}, {-hl, -hw}} {
		out = append(out, [2]float64{b.CentreXM + k[0]*c - k[1]*s, b.CentreYM + k[0]*s + k[1]*c})
	}
	return out
}

// clipConvex clips one convex polygon by another, both anticlockwise
// (Sutherland-Hodgman).
func clipConvex(subject, clip [][2]float64) [][2]float64 {
	out := subject
	for i := range clip {
		a, b := clip[i], clip[(i+1)%len(clip)]
		inside := func(p [2]float64) bool { return (b[0]-a[0])*(p[1]-a[1])-(b[1]-a[1])*(p[0]-a[0]) >= 0 }
		in := out
		out = nil
		for j := range in {
			p, q := in[j], in[(j+1)%len(in)]
			pIn, qIn := inside(p), inside(q)
			if pIn {
				out = append(out, p)
			}
			if pIn != qIn {
				// The edge p-q crosses the clip line a-b.
				dx, dy := q[0]-p[0], q[1]-p[1]
				ex, ey := b[0]-a[0], b[1]-a[1]
				t := (ex*(p[1]-a[1]) - ey*(p[0]-a[0])) / (ey*dx - ex*dy)
				out = append(out, [2]float64{p[0] + t*dx, p[1] + t*dy})
			}
		}
		if len(out) == 0 {
			return nil
		}
	}
	return out
}

func polygonArea(p [][2]float64) float64 {
	area := 0.0
	for i := range p {
		q := p[(i+1)%len(p)]
		area += p[i][0]*q[1] - q[0]*p[i][1]
	}
	return math.Abs(area) / 2
}

type errorPool struct {
	errs, bounds []float64
	within       int
}

func (p *errorPool) add(err, bound float64, within bool) {
	p.errs, p.bounds = append(p.errs, math.Abs(err)), append(p.bounds, bound)
	if within {
		p.within++
	}
}

func (p errorPool) summary() ComponentSummary {
	s := ComponentSummary{Scored: len(p.errs), WithinReferenceBound: p.within}
	if s.Scored == 0 {
		return s
	}
	var sum, sq, bounds float64
	for i, e := range p.errs {
		sum, sq, bounds = sum+e, sq+e*e, bounds+p.bounds[i]
		s.MaxAbsError = math.Max(s.MaxAbsError, e)
	}
	n := float64(s.Scored)
	s.MeanAbsError, s.RMSError, s.MeanReferenceBound = sum/n, math.Sqrt(sq/n), bounds/n
	return s
}

func summarisePhysical(res PhysicalResult) PhysicalSummary {
	pools := map[PhysicalComponent]*errorPool{}
	byRef := map[string]*errorPool{}
	pool := func(c PhysicalComponent) *errorPool {
		if pools[c] == nil {
			pools[c] = &errorPool{}
		}
		return pools[c]
	}
	sum := PhysicalSummary{MinBoxIoU: 1}
	var ious float64
	for _, in := range res.Instants {
		c := in.Comparison
		if c.Centre != nil {
			if in.Outcomes[ComponentCentre].Category == OutcomeScored {
				pool(ComponentCentre).add(c.Centre.ErrorM, c.Centre.ReferenceBoundM, c.Centre.ErrorM <= c.Centre.ReferenceBoundM)
			}
			if byRef[c.Centre.PredictionReference] == nil {
				byRef[c.Centre.PredictionReference] = &errorPool{}
			}
			byRef[c.Centre.PredictionReference].add(c.Centre.ErrorM, c.Centre.ReferenceBoundM, c.Centre.ErrorM <= c.Centre.ReferenceBoundM)
		}
		if c.Yaw != nil {
			pool(ComponentYaw).add(c.Yaw.ErrorRad, c.Yaw.ReferenceBoundRad, math.Abs(c.Yaw.ErrorRad) <= c.Yaw.ReferenceBoundRad)
		}
		for comp, d := range map[PhysicalComponent]*DimensionComparison{ComponentLength: c.Length, ComponentWidth: c.Width, ComponentHeight: c.Height} {
			if d != nil {
				pool(comp).add(d.ErrorM, d.ReferenceHalfWidthM, d.OutsideBoundM == 0)
			}
		}
		for comp, e := range map[PhysicalComponent]*EndpointComparison{ComponentFront: c.Front, ComponentRear: c.Rear} {
			if e != nil {
				pool(comp).add(e.DistanceM, e.ReferenceBoundM, e.DistanceM <= e.ReferenceBoundM)
			}
		}
		if c.Ends != nil {
			pool(ComponentEnds).add(c.Ends.MeanDistanceM, c.Ends.ReferenceBoundM, c.Ends.MeanDistanceM <= c.Ends.ReferenceBoundM)
		}
		if c.Box != nil {
			sum.BoxScored++
			ious += c.Box.IoU
			sum.MinBoxIoU = math.Min(sum.MinBoxIoU, c.Box.IoU)
		}
	}
	if sum.BoxScored > 0 {
		sum.MeanBoxIoU = ious / float64(sum.BoxScored)
	} else {
		sum.MinBoxIoU = 0
	}
	sum.Components = map[PhysicalComponent]ComponentSummary{}
	for _, c := range PhysicalComponents() {
		if c == ComponentBox {
			continue
		}
		sum.Components[c] = pool(c).summary()
	}
	sum.CentreByPredictionReference = map[string]ComponentSummary{}
	for ref, p := range byRef {
		sum.CentreByPredictionReference[ref] = p.summary()
	}
	var gaps errorPool
	for _, f := range res.Following {
		if f.ErrorM != nil {
			// The quoted value need not be the midpoint: the bound is its
			// farther distance to either end, as for a dimension.
			ref := f.ReferenceGap
			value := referenceGapValue(*ref)
			gaps.add(*f.ErrorM, math.Max(value-*ref.LowerM, *ref.UpperM-value), *f.OutsideBoundM == 0)
		}
	}
	sum.Following = gaps.summary()
	return sum
}

func physicalCaveats(res PhysicalResult) []string {
	out := []string{fmt.Sprintf("Split %q has role %q: physical scores are tuning evidence, not a held-out result.",
		res.Reference.Split, res.Reference.SplitRole)}
	if res.Arm.Stage != StageFinal {
		out = append(out, fmt.Sprintf("Arm %s scores %s-stage estimates as a declared baseline.", res.Arm.Label, res.Arm.Stage))
	}
	var offBody []string
	for ref, s := range res.Summary.CentreByPredictionReference {
		if ref != "body_centre" && ref != "near_face_centre" {
			offBody = append(offBody, fmt.Sprintf("%d at %s", s.Scored, ref))
		}
	}
	if len(offBody) > 0 {
		sort.Strings(offBody)
		out = append(out, fmt.Sprintf("Arm %s reports points that are not body centres (%s): their centres are counted as "+
			"missing predictions, and how far each point sits from the body centre is reported only by prediction reference.",
			res.Arm.Label, strings.Join(offBody, ", ")))
	}
	out = append(out,
		"Following gaps are chords along the follower's axis between projected footprint extremes, not the along-path "+
			"headway arc; leader choice is not scored.",
		"Within-bound counts compare errors with each reference's own stated bound; they are not a calibrated coverage.")
	return out
}
