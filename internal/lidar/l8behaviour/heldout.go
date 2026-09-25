package l8behaviour

// The held-out scoring harness of Section 8.3 of
// docs/plans/lidar-behaviour-analytics-plan.md: "validate endpoint/gap error
// and interval coverage on independent held-out physical references,
// stratified by class, aspect, range and support. Pin acceptable error and
// suppression bounds before scoring; a deterministic PCAP comparison does not
// establish bumper accuracy."
//
// This file is the harness, not the validation. The validation needs
// annotated references this repository does not hold; until they exist the
// harness is exercised on the analytic fixtures with known perturbations.
//
// Method following_heldout_scoring_v1:
//
//   - References. A reference is one body at one instant from a source
//     independent of the estimator: its true centre, heading, length and width,
//     its motion class, and the estimated track it annotates. Its endpoints are
//     found by projecting its four footprint corners onto the path the estimate
//     was measured along and taking the extreme arcs, which is independent of
//     the estimator's closed form and so also scores that form's local-tangent
//     approximation on a curve. A corner off the path beyond the far end
//     cannot be a trailing extreme, nor one before the start a leading
//     extreme; a reference whose needed extreme may lie off the path is
//     unscorable and counted.
//   - Cases. Each evaluated pair instant yields up to three cases: the leader's
//     trailing endpoint, the follower's leading endpoint, and the gap. A case
//     whose estimate exists is scored: error = estimate - reference, covered
//     when |error| <= z sigma with z the two-sided normal quantile of the
//     nominal coverage. An endpoint with no projected body, or a gap that is
//     not supported (apart from publication stage and, for the gap, the speed
//     floor that concerns only the time gap), is counted as suppressed under
//     the pair's first such reason. References no estimate matched are counted
//     as unmatched: a miss is reported, never scored as an error.
//   - Strata. An endpoint case is stratified by its reference's class, range
//     and face aspect, and its own party's support at the instant; a gap case
//     by the follower's reference and leading face, and the worse of the two
//     parties' support. Range is the reference centre's distance from the
//     sensor. Face aspect is the angle between the outward normal of the face
//     carrying the endpoint (along the path tangent for a leading endpoint,
//     against it for a trailing one) and the direction from the reference
//     centre to the sensor: 0 when the face is turned to the sensor, a half
//     turn when it is turned away. It is per face because a leader's rear and
//     a follower's front on the same approach are seen very differently.
//   - Scores. Per stratum: case count, bias (mean error), RMS error, the 95th
//     percentile of |error| (nearest rank), empirical coverage, suppressions by
//     reason and the suppression rate over cases and suppressions.
//   - Verdict. A stratum with fewer than MinCasesPerStratum scored cases is
//     insufficient_cases. Otherwise it fails when its 95th percentile error
//     exceeds the bound for its kind, its coverage falls outside the accepted
//     range (too low is overconfidence; too high is an interval too wide to
//     inform), or its suppression rate exceeds the bound. The report fails if
//     any stratum fails, is insufficient if any is, and passes otherwise.
//   - Pinning. The plan (bins, nominal coverage, bounds and the reference set
//     it was written for) is hashed, and scoring refuses a plan whose hash is
//     not the one pinned beforehand, or a reference set it was not written
//     for. Commit the plan and its hash before the references are scored; the
//     report repeats the hash so a later bound change is visible.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// HeldOutScoringMethodID versions the harness: case construction, strata,
// statistics and verdict rules.
const HeldOutScoringMethodID = "following_heldout_scoring_v1"

// Stratum kinds and verdicts. They label a validation report, not a
// behaviour result, and are fixed by this method id.
const (
	CaseKindEndpoint = "endpoint"
	CaseKindGap      = "gap"

	VerdictPass              = "pass"
	VerdictFail              = "fail"
	VerdictInsufficientCases = "insufficient_cases"
)

// ReferenceBody is one independently annotated body at one instant.
type ReferenceBody struct {
	// TrackID is the estimated track the reference annotates.
	TrackID          string      `json:"track_id"`
	CaptureUnixNanos int64       `json:"capture_unix_nanos"`
	MotionClass      MotionClass `json:"motion_class"`
	CentreX          float64     `json:"centre_x"`
	CentreY          float64     `json:"centre_y"`
	HeadingRad       float64     `json:"heading_rad"`
	LengthM          float64     `json:"length_m"`
	WidthM           float64     `json:"width_m"`
}

func (r ReferenceBody) validate() error {
	if r.TrackID == "" || r.CaptureUnixNanos <= 0 || !r.MotionClass.Valid() {
		return fmt.Errorf("reference requires a track id, a capture time and a motion class")
	}
	for _, v := range []float64{r.CentreX, r.CentreY, r.HeadingRad} {
		if !finite(v) {
			return fmt.Errorf("reference %s pose must be finite", r.TrackID)
		}
	}
	if !(r.LengthM > 0) || !(r.WidthM > 0) || !finite(r.LengthM) || !finite(r.WidthM) {
		return fmt.Errorf("reference %s extent must be positive and finite", r.TrackID)
	}
	return nil
}

// HeldOutSet is a named set of references and the sensor position their range
// and aspect are measured from.
type HeldOutSet struct {
	ReferenceSetID string          `json:"reference_set_id"`
	SensorX        float64         `json:"sensor_x"`
	SensorY        float64         `json:"sensor_y"`
	References     []ReferenceBody `json:"references"`
}

// AcceptanceBounds are the pinned acceptance criteria.
type AcceptanceBounds struct {
	MaxEndpointP95AbsErrorM float64 `json:"max_endpoint_p95_abs_error_m"`
	MaxGapP95AbsErrorM      float64 `json:"max_gap_p95_abs_error_m"`
	// MinCoverage and MaxCoverage bound the empirical coverage of the nominal
	// interval.
	MinCoverage        float64 `json:"min_coverage"`
	MaxCoverage        float64 `json:"max_coverage"`
	MaxSuppressionRate float64 `json:"max_suppression_rate"`
	MinCasesPerStratum int     `json:"min_cases_per_stratum"`
}

// ScoringPlan is everything fixed before scoring: the reference set it is for,
// the nominal coverage, the strata and the bounds.
type ScoringPlan struct {
	ReferenceSetID  string  `json:"reference_set_id"`
	NominalCoverage float64 `json:"nominal_coverage"`
	// RangeEdgesM split range into [0, e0), [e0, e1), ..., [en, inf).
	RangeEdgesM []float64 `json:"range_edges_m"`
	// AspectEdgesRad split aspect into [0, e0), ..., [en, pi].
	AspectEdgesRad []float64        `json:"aspect_edges_rad"`
	Bounds         AcceptanceBounds `json:"bounds"`
}

// Validate requires an identity, a coverage and bounds in range, and edges
// that strictly ascend inside their domain.
func (p ScoringPlan) Validate() error {
	if p.ReferenceSetID == "" {
		return fmt.Errorf("scoring plan requires the reference set it was written for")
	}
	if !(p.NominalCoverage > 0 && p.NominalCoverage < 1) {
		return fmt.Errorf("scoring plan nominal coverage must be in (0, 1)")
	}
	edges := func(name string, e []float64, hi float64) error {
		for i, v := range e {
			if !(v > 0 && v < hi) || (i > 0 && !(v > e[i-1])) {
				return fmt.Errorf("scoring plan %s edges must ascend strictly inside (0, %g)", name, hi)
			}
		}
		return nil
	}
	if err := edges("range", p.RangeEdgesM, math.Inf(1)); err != nil {
		return err
	}
	if err := edges("aspect", p.AspectEdgesRad, math.Pi); err != nil {
		return err
	}
	b := p.Bounds
	if !(b.MaxEndpointP95AbsErrorM > 0) || !(b.MaxGapP95AbsErrorM > 0) ||
		!finite(b.MaxEndpointP95AbsErrorM) || !finite(b.MaxGapP95AbsErrorM) {
		return fmt.Errorf("scoring plan error bounds must be positive and finite")
	}
	if !(b.MinCoverage >= 0 && b.MinCoverage <= b.MaxCoverage && b.MaxCoverage <= 1) {
		return fmt.Errorf("scoring plan coverage range must satisfy 0 <= min <= max <= 1")
	}
	if !(b.MaxSuppressionRate >= 0 && b.MaxSuppressionRate <= 1) || b.MinCasesPerStratum < 1 {
		return fmt.Errorf("scoring plan needs a suppression rate in [0, 1] and at least one case per stratum")
	}
	return nil
}

// Hash is the plan's identity: the first 16 hex digits of the SHA-256 of its
// JSON encoding. Pin it before scoring.
func (p ScoringPlan) Hash() string {
	raw, _ := json.Marshal(p) // numbers and strings always encode
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}

// EstimatedPair is one evaluated pair instant to score: the path it was
// measured along, the evaluation, and each party's support at the instant.
type EstimatedPair struct {
	Path            PathFrame      `json:"-"`
	Point           FollowingPoint `json:"point"`
	LeaderSupport   SupportState   `json:"leader_support"`
	FollowerSupport SupportState   `json:"follower_support"`
}

// EstimatedPairsFromAnalysis collects every evaluated instant of an analysis,
// in encounter and instant order, with each party's support read from the
// trajectories the analysis was run on.
func EstimatedPairsFromAnalysis(a FollowingAnalysis, trajectories []Trajectory) ([]EstimatedPair, error) {
	byID := make(map[string]Trajectory, len(trajectories))
	for _, t := range trajectories {
		byID[t.Passage.TrackID] = t
	}
	var out []EstimatedPair
	for _, e := range a.Encounters {
		res, ok := a.Path(e.FollowerTrackID)
		if !ok || res.Path == nil {
			return nil, fmt.Errorf("encounter %s -> %s has no path", e.LeaderTrackID, e.FollowerTrackID)
		}
		for _, inst := range e.Instants {
			if inst.Point == nil {
				continue
			}
			ls, lok := byID[e.LeaderTrackID].SampleAt(inst.CaptureUnixNanos)
			fs, fok := byID[e.FollowerTrackID].SampleAt(inst.CaptureUnixNanos)
			if !lok || !fok {
				return nil, fmt.Errorf("encounter %s -> %s at %d: trajectories do not match the analysis",
					e.LeaderTrackID, e.FollowerTrackID, inst.CaptureUnixNanos)
			}
			out = append(out, EstimatedPair{
				Path: res.Path, Point: *inst.Point, LeaderSupport: ls.Support, FollowerSupport: fs.Support,
			})
		}
	}
	return out, nil
}

// Stratum is one cell of the stratification.
type Stratum struct {
	Kind    string       `json:"kind"`
	Class   MotionClass  `json:"class"`
	Range   string       `json:"range"`
	Aspect  string       `json:"aspect"`
	Support SupportState `json:"support"`
}

func (s Stratum) less(o Stratum) bool {
	switch {
	case s.Kind != o.Kind:
		return s.Kind < o.Kind // endpoint before gap
	case s.Class != o.Class:
		return s.Class < o.Class
	case s.Range != o.Range:
		return s.Range < o.Range
	case s.Aspect != o.Aspect:
		return s.Aspect < o.Aspect
	}
	return s.Support < o.Support
}

// ReasonCount counts suppressed cases for one reason.
type ReasonCount struct {
	Reason SuppressionReason `json:"reason"`
	Count  int               `json:"count"`
}

// StratumScore is one stratum's scores and verdict.
type StratumScore struct {
	Stratum
	Cases           int           `json:"cases"`
	BiasM           float64       `json:"bias_m"`
	RMSErrorM       float64       `json:"rms_error_m"`
	P95AbsErrorM    float64       `json:"p95_abs_error_m"`
	Coverage        float64       `json:"coverage"`
	Suppressed      []ReasonCount `json:"suppressed,omitempty"`
	SuppressionRate float64       `json:"suppression_rate"`
	Verdict         string        `json:"verdict"`
	// Failed names the bounds a failing stratum broke.
	Failed []string `json:"failed,omitempty"`
}

// HeldOutReport is the scored result.
type HeldOutReport struct {
	MethodID       string `json:"method_id"`
	ReferenceSetID string `json:"reference_set_id"`
	PlanHash       string `json:"plan_hash"`
	// Z is the interval half-width in sigmas for the nominal coverage.
	Z      float64        `json:"z"`
	Strata []StratumScore `json:"strata"`
	// UnmatchedReferences had no evaluated instant; UnscorableReferences
	// matched one but their footprint left the path.
	UnmatchedReferences  int    `json:"unmatched_references"`
	UnscorableReferences int    `json:"unscorable_references"`
	Verdict              string `json:"verdict"`
}

type referenceKey struct {
	trackID string
	capture int64
}

type stratumAccumulator struct {
	errs       []float64
	covered    int
	suppressed map[SuppressionReason]int
}

// ScoreHeldOut scores estimated pair instants against a held-out reference
// set under a plan pinned by its hash.
func ScoreHeldOut(pinnedPlanHash string, plan ScoringPlan, set HeldOutSet, estimates []EstimatedPair) (HeldOutReport, error) {
	if err := plan.Validate(); err != nil {
		return HeldOutReport{}, err
	}
	if h := plan.Hash(); h != pinnedPlanHash {
		return HeldOutReport{}, fmt.Errorf("scoring plan hash %s is not the pinned %s: bounds are fixed before scoring", h, pinnedPlanHash)
	}
	if set.ReferenceSetID != plan.ReferenceSetID {
		return HeldOutReport{}, fmt.Errorf("plan was pinned for reference set %q, not %q", plan.ReferenceSetID, set.ReferenceSetID)
	}
	if !finite(set.SensorX) || !finite(set.SensorY) {
		return HeldOutReport{}, fmt.Errorf("reference set sensor position must be finite")
	}
	refs := make(map[referenceKey]ReferenceBody, len(set.References))
	for _, r := range set.References {
		if err := r.validate(); err != nil {
			return HeldOutReport{}, err
		}
		k := referenceKey{r.TrackID, r.CaptureUnixNanos}
		if _, dup := refs[k]; dup {
			return HeldOutReport{}, fmt.Errorf("reference %s at %d appears twice", r.TrackID, r.CaptureUnixNanos)
		}
		refs[k] = r
	}

	report := HeldOutReport{
		MethodID: HeldOutScoringMethodID, ReferenceSetID: set.ReferenceSetID, PlanHash: pinnedPlanHash,
		Z: math.Sqrt2 * math.Erfinv(plan.NominalCoverage),
	}
	acc := map[Stratum]*stratumAccumulator{}
	add := func(s Stratum) *stratumAccumulator {
		a := acc[s]
		if a == nil {
			a = &stratumAccumulator{suppressed: map[SuppressionReason]int{}}
			acc[s] = a
		}
		return a
	}
	matched := map[referenceKey]bool{}
	unscorable := map[referenceKey]bool{}
	// strat places a case; faceNormal is the outward normal of the face that
	// carries the endpoint.
	strat := func(kind string, r ReferenceBody, faceNormal float64, support SupportState) Stratum {
		dx, dy := set.SensorX-r.CentreX, set.SensorY-r.CentreY
		aspect := math.Abs(math.Remainder(faceNormal-math.Atan2(dy, dx), 2*math.Pi))
		return Stratum{
			Kind: kind, Class: r.MotionClass, Support: support,
			Range:  binLabel(math.Hypot(dx, dy), plan.RangeEdgesM, "inf"),
			Aspect: binLabel(aspect, plan.AspectEdgesRad, "pi"),
		}
	}

	for _, ep := range estimates {
		pt := ep.Point
		if ep.Path == nil {
			return HeldOutReport{}, fmt.Errorf("estimate %s -> %s at %d has no path", pt.LeaderTrackID, pt.FollowerTrackID, pt.CaptureUnixNanos)
		}
		if g := pt.SpatialGap.Provenance.Version.GeometryID; g != ep.Path.GeometryID() {
			return HeldOutReport{}, fmt.Errorf("estimate was measured along %s, not the supplied %s", g, ep.Path.GeometryID())
		}
		if !ep.LeaderSupport.Valid() || !ep.FollowerSupport.Valid() {
			return HeldOutReport{}, fmt.Errorf("estimate %s -> %s at %d needs both parties' support", pt.LeaderTrackID, pt.FollowerTrackID, pt.CaptureUnixNanos)
		}
		lk := referenceKey{pt.LeaderTrackID, pt.CaptureUnixNanos}
		fk := referenceKey{pt.FollowerTrackID, pt.CaptureUnixNanos}
		lr, lok := refs[lk]
		fr, fok := refs[fk]
		firstReason := firstPhysicalReason(pt.Reasons)

		var lTrue, fTrue, lTangent, fTangent float64
		var lScorable, fScorable bool
		if lok {
			matched[lk] = true
			if lTrue, lTangent, lScorable = footprintExtreme(ep.Path, lr, ExtremityTrailing); !lScorable {
				unscorable[lk] = true
			}
		}
		if fok {
			matched[fk] = true
			if fTrue, fTangent, fScorable = footprintExtreme(ep.Path, fr, ExtremityLeading); !fScorable {
				unscorable[fk] = true
			}
		}
		endpoint := func(r ReferenceBody, normal float64, support SupportState, body *BodyOnPath, est func(*BodyOnPath) Endpoint, truth float64) {
			a := add(strat(CaseKindEndpoint, r, normal, support))
			if body == nil {
				a.suppressed[firstReason]++
				return
			}
			e := est(body)
			err := e.ArcM - truth
			a.errs = append(a.errs, err)
			if math.Abs(err) <= report.Z*e.SigmaM {
				a.covered++
			}
		}
		if lScorable {
			endpoint(lr, lTangent+math.Pi, ep.LeaderSupport, pt.Leader, func(b *BodyOnPath) Endpoint { return b.Trailing }, lTrue)
		}
		if fScorable {
			endpoint(fr, fTangent, ep.FollowerSupport, pt.Follower, func(b *BodyOnPath) Endpoint { return b.Leading }, fTrue)
		}
		if lScorable && fScorable {
			support := ep.FollowerSupport
			if support == SupportObserved {
				support = ep.LeaderSupport
			}
			a := add(strat(CaseKindGap, fr, fTangent, support))
			if spatialGapSupported(&pt) {
				err := pt.Gap.ValueM - (lTrue - fTrue)
				a.errs = append(a.errs, err)
				if math.Abs(err) <= report.Z*pt.Gap.SigmaM {
					a.covered++
				}
			} else {
				a.suppressed[firstGapReason(pt.Reasons)]++
			}
		}
	}
	for k := range refs {
		if !matched[k] {
			report.UnmatchedReferences++
		}
	}
	report.UnscorableReferences = len(unscorable)

	strata := make([]Stratum, 0, len(acc))
	for s := range acc {
		strata = append(strata, s)
	}
	sort.Slice(strata, func(i, j int) bool { return strata[i].less(strata[j]) })
	anyFail, anyInsufficient := false, len(strata) == 0
	for _, s := range strata {
		score := scoreStratum(s, acc[s], plan.Bounds)
		anyFail = anyFail || score.Verdict == VerdictFail
		anyInsufficient = anyInsufficient || score.Verdict == VerdictInsufficientCases
		report.Strata = append(report.Strata, score)
	}
	switch {
	case anyFail:
		report.Verdict = VerdictFail
	case anyInsufficient:
		report.Verdict = VerdictInsufficientCases
	default:
		report.Verdict = VerdictPass
	}
	return report, nil
}

// firstGapReason is why a gap was not scored: its first reason that is
// neither publication stage nor the speed floor, which concerns only the time
// gap.
func firstGapReason(reasons []SuppressionReason) SuppressionReason {
	for _, r := range reasons {
		if r != ReasonEstimateNotFinal && r != ReasonBelowSpeedFloor {
			return r
		}
	}
	return ReasonUnspecified
}

// footprintExtreme projects a reference footprint's four corners onto the
// path and returns one extreme: the greatest arc for the leading extreme, the
// least for the trailing one, with the path tangent at the reference centre.
//
// A body near a path end may have corners off it. A corner off the path ahead
// of the centre (along the tangent) lies beyond the far end and cannot be the
// least arc, and one behind cannot be the greatest, so the extreme is still
// scorable from the corners that project; a corner off the path on the side
// the extreme lies is not, and the reference is unscorable for that extreme.
func footprintExtreme(path PathFrame, r ReferenceBody, extremity PathExtremity) (arc, tangent float64, ok bool) {
	centre, located := path.Locate(r.CentreX, r.CentreY)
	if !located {
		return 0, 0, false
	}
	tx, ty := math.Cos(centre.TangentRad), math.Sin(centre.TangentRad)
	c, s := math.Cos(r.HeadingRad), math.Sin(r.HeadingRad)
	leading := extremity == ExtremityLeading
	arc = math.Inf(1)
	if leading {
		arc = math.Inf(-1)
	}
	for _, a := range []float64{r.LengthM / 2, -r.LengthM / 2} {
		for _, b := range []float64{r.WidthM / 2, -r.WidthM / 2} {
			ox, oy := a*c-b*s, a*s+b*c
			loc, located := path.Locate(r.CentreX+ox, r.CentreY+oy)
			if !located {
				if ahead := ox*tx+oy*ty > 0; ahead == leading {
					return 0, 0, false
				}
				continue
			}
			if leading {
				arc = math.Max(arc, loc.ArcM)
			} else {
				arc = math.Min(arc, loc.ArcM)
			}
		}
	}
	return arc, centre.TangentRad, !math.IsInf(arc, 0)
}

// binLabel names the half-open bin a value falls in.
func binLabel(v float64, edges []float64, top string) string {
	lo := 0.0
	for _, e := range edges {
		if v < e {
			return fmt.Sprintf("[%.4g,%.4g)", lo, e)
		}
		lo = e
	}
	return fmt.Sprintf("[%.4g,%s]", lo, top)
}

func scoreStratum(s Stratum, a *stratumAccumulator, b AcceptanceBounds) StratumScore {
	score := StratumScore{Stratum: s, Cases: len(a.errs)}
	suppressed := 0
	for _, r := range SuppressionReasons() {
		if n := a.suppressed[r]; n > 0 {
			score.Suppressed = append(score.Suppressed, ReasonCount{r, n})
			suppressed += n
		}
	}
	score.SuppressionRate = float64(suppressed) / float64(suppressed+score.Cases)
	if score.Cases > 0 {
		abs := make([]float64, score.Cases)
		var sum, sumSq float64
		for i, e := range a.errs {
			sum += e
			sumSq += e * e
			abs[i] = math.Abs(e)
		}
		n := float64(score.Cases)
		score.BiasM, score.RMSErrorM = sum/n, math.Sqrt(sumSq/n)
		sort.Float64s(abs)
		score.P95AbsErrorM = abs[min(max(int(math.Ceil(0.95*n))-1, 0), score.Cases-1)]
		score.Coverage = float64(a.covered) / n
	}
	if score.Cases < b.MinCasesPerStratum {
		score.Verdict = VerdictInsufficientCases
		return score
	}
	maxErr := b.MaxEndpointP95AbsErrorM
	if s.Kind == CaseKindGap {
		maxErr = b.MaxGapP95AbsErrorM
	}
	if score.P95AbsErrorM > maxErr {
		score.Failed = append(score.Failed, "p95_abs_error")
	}
	if score.Coverage < b.MinCoverage {
		score.Failed = append(score.Failed, "coverage_low")
	}
	if score.Coverage > b.MaxCoverage {
		score.Failed = append(score.Failed, "coverage_high")
	}
	if score.SuppressionRate > b.MaxSuppressionRate {
		score.Failed = append(score.Failed, "suppression_rate")
	}
	score.Verdict = VerdictPass
	if len(score.Failed) > 0 {
		score.Verdict = VerdictFail
	}
	return score
}
