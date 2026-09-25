package l8behaviour

// Following exposure over a leader/follower encounter, per Sections 6, 8.3,
// 9.2, 9.3 and 10.4 of docs/plans/lidar-behaviour-analytics-plan.md: spatial
// gap and net time gap series, their minimum and median with an interval, valid
// following time over observed support only, time below each named band and
// the rate over that time, and every suppression counted by reason.
//
// Method following_exposure_v1:
//
//   - Encounter. An encounter is one follower and one leader. An instant of the
//     follower belongs to it when that leader was chosen, when it competed for
//     the choice (ambiguous_leader), or when it was the nearest established
//     body behind an unestablished one (no_common_path). Free-flow instants,
//     and instants the follower's own path could not order, belong to no
//     encounter and appear only in the follower's timeline.
//   - Time. Each follower instant stands for the interval to its next sample
//     (sample and hold, as Trajectory.SupportSeconds); the last sample closes
//     the passage and stands for nothing. An interval longer than
//     MaxIntervalNanos means rows are missing: it is a record gap, counted
//     apart and attributed to nothing, so missing rows cannot inflate any time.
//   - Valid following time is the time of instants whose point is supported
//     opportunity (EvaluateFollowing), which excludes coasted, occluded,
//     standstill and otherwise suppressed time. Time at which either party was
//     not observed is also counted apart as unobserved time, and its gaps form
//     a review-only predicted series that never enters a statistic.
//   - Series and statistics. The spatial gap series holds every instant whose
//     gap is supported, including standstill; the net time gap series holds
//     the valid instants. Minimum and median are taken over instants, not
//     weighted by time; the median of an even count is the mean of the middle
//     two. Time below a band is the valid time whose net time gap is strictly
//     below it; the rate divides that by valid following time.
//   - Minimum opportunity. Below MinOpportunitySeconds of valid following
//     time, band durations and rates are suppressed with
//     insufficient_observation, never reported as zero (Section 6, rule 3). A
//     statistic of an empty series is suppressed the same way.
//   - Uncertainty. Extrema and order statistics of a correlated series are not
//     symmetric (Section 9.3), so every statistic carries an interval from a
//     seeded Monte Carlo draw. Each draw perturbs every instant's gap and time
//     gap by its own one-sigma times e = sqrt(rho) z_common + sqrt(1 - rho)
//     z_instant, one standard normal shared by the encounter and one per
//     instant, drawn in instant order and shared by the instant's gap and time
//     gap. rho is CommonModeFraction: 1 treats the error as one offset for the
//     encounter (a constant extent error), 0 as independent per instant (which
//     biases a minimum low). The interval is the nearest-rank quantiles at
//     (1 -/+ IntervalCoverage) / 2 over MonteCarloSamples draws, and the seed
//     is FNV-1a of the method, parameter hash, geometry and pair, so the same
//     inputs give the same interval. Valid time is an accounting sum and
//     declares no uncertainty.
//   - Emission. Measurements pass class applicability, then the minimum
//     opportunity rule, then the stage guard: an encounter whose path or any
//     contributing sample is not final is suppressed with estimate_not_final,
//     and its values appear only in the review-labelled Provisional block.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
)

// FollowingEncounterMethodID versions the whole encounter method: the local
// path (LocalPathMethodID), leader choice (PairingMethodID), synchronisation
// (SyncMethodID), the pointwise equations (FollowingMethodID) and the exposure
// rules above (ExposureMethodID). Change it when any part changes. Encounter
// measurements record it with the parameter hash appended, so a threshold
// change is a version change too.
const FollowingEncounterMethodID = "following_encounter_v1"

// ExposureMethodID versions the encounter accounting, statistics and interval
// method.
const ExposureMethodID = "following_exposure_v1"

// ExposureParams bound encounter aggregation. None has a default.
type ExposureParams struct {
	// MinOpportunitySeconds is the valid following time below which band
	// durations and rates are suppressed.
	MinOpportunitySeconds float64 `json:"min_opportunity_seconds"`
	// MaxIntervalNanos is the longest sample interval that is not a record
	// gap; set it a little above the frame period.
	MaxIntervalNanos int64 `json:"max_interval_nanos"`
	// IntervalCoverage is the nominal coverage of every statistic's interval.
	IntervalCoverage float64 `json:"interval_coverage"`
	// MonteCarloSamples is the number of draws behind each interval.
	MonteCarloSamples int `json:"monte_carlo_samples"`
	// CommonModeFraction is rho, the share of each instant's error variance
	// common to the whole encounter, in [0, 1].
	CommonModeFraction float64 `json:"common_mode_fraction"`
}

// minMonteCarloSamples keeps an interval's own quantile error small enough to
// report: at 100 draws a 5 % quantile rests on five of them.
const minMonteCarloSamples = 100

// Validate requires every bound in range.
func (p ExposureParams) Validate() error {
	if !(p.MinOpportunitySeconds > 0) || !finite(p.MinOpportunitySeconds) || p.MaxIntervalNanos <= 0 {
		return fmt.Errorf("exposure minimum opportunity and maximum interval must be positive")
	}
	if !(p.IntervalCoverage > 0 && p.IntervalCoverage < 1) {
		return fmt.Errorf("exposure interval coverage must be in (0, 1)")
	}
	if p.MonteCarloSamples < minMonteCarloSamples {
		return fmt.Errorf("exposure needs at least %d Monte Carlo samples", minMonteCarloSamples)
	}
	if !(p.CommonModeFraction >= 0 && p.CommonModeFraction <= 1) {
		return fmt.Errorf("exposure common-mode fraction must be in [0, 1]")
	}
	return nil
}

// FollowingAnalysisParams are every bound the encounter method applies.
type FollowingAnalysisParams struct {
	Path      LocalPathParams `json:"path"`
	Following FollowingParams `json:"following"`
	Pairing   PairingParams   `json:"pairing"`
	Exposure  ExposureParams  `json:"exposure"`
}

// Validate checks each part.
func (p FollowingAnalysisParams) Validate() error {
	if err := p.Path.Validate(); err != nil {
		return err
	}
	if err := p.Following.Validate(); err != nil {
		return err
	}
	if err := p.Pairing.Validate(); err != nil {
		return err
	}
	return p.Exposure.Validate()
}

// Hash is the parameters' identity: the first 16 hex digits of the SHA-256 of
// their JSON encoding.
func (p FollowingAnalysisParams) Hash() string {
	raw, _ := json.Marshal(p) // a struct of numbers always encodes
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}

// ReasonTally counts the instants suppressed for one reason and the time they
// stand for.
type ReasonTally struct {
	Reason   SuppressionReason `json:"reason"`
	Instants int               `json:"instants"`
	Nanos    int64             `json:"nanos"`
}

// EncounterInstant is one follower instant of an encounter.
type EncounterInstant struct {
	CaptureUnixNanos int64 `json:"capture_unix_nanos"`
	// IntervalNanos is the time to the follower's next sample, which this
	// instant stands for; zero at the follower's last sample.
	IntervalNanos int64 `json:"interval_nanos"`
	// RecordGap marks an interval over MaxIntervalNanos: it stands for
	// nothing and is counted as a record gap.
	RecordGap bool `json:"record_gap,omitempty"`
	// Role is the leader's disposition at this instant: leader, competing, or
	// blocked behind an unestablished body.
	Role CandidateDisposition `json:"role"`
	// Valid is true when the instant is supported opportunity, apart from
	// publication stage; Reason says why not otherwise.
	Valid     bool              `json:"valid"`
	Reason    SuppressionReason `json:"reason,omitempty"`
	Condition PathCondition     `json:"condition,omitempty"`
	// Unobserved is true when either party was not observed at the instant.
	Unobserved bool `json:"unobserved,omitempty"`
	// Point is the pointwise evaluation, present when this leader was chosen
	// and had a sample at the instant.
	Point *FollowingPoint `json:"point,omitempty"`
}

// SeriesPoint is one supported value of a series.
type SeriesPoint struct {
	CaptureUnixNanos int64   `json:"capture_unix_nanos"`
	Value            float64 `json:"value"`
	Sigma            float64 `json:"sigma"`
}

// PredictedPoint is one review-only predicted gap, with its coast age.
type PredictedPoint struct {
	CaptureUnixNanos int64   `json:"capture_unix_nanos"`
	ValueM           float64 `json:"value_m"`
	SigmaM           float64 `json:"sigma_m"`
	CoastAgeNanos    int64   `json:"coast_age_nanos"`
}

// EncounterAccounting is where an encounter's time went. Every interval that
// is not a record gap is either valid or suppressed for exactly one reason;
// unobserved time overlaps both.
type EncounterAccounting struct {
	ValidNanos int64 `json:"valid_nanos"`
	// BandNanos is the valid time below each of FollowingBands(), in order.
	BandNanos       []int64       `json:"band_nanos"`
	UnobservedNanos int64         `json:"unobserved_nanos"`
	RecordGapNanos  int64         `json:"record_gap_nanos"`
	Suppressions    []ReasonTally `json:"suppressions,omitempty"`
}

// Suppression returns the tally for one reason; zero when it never applied.
func (a EncounterAccounting) Suppression(r SuppressionReason) ReasonTally {
	for _, t := range a.Suppressions {
		if t.Reason == r {
			return t
		}
	}
	return ReasonTally{Reason: r}
}

// Encounter is one leader/follower encounter and its following exposure.
type Encounter struct {
	LeaderTrackID   string `json:"leader_track_id"`
	FollowerTrackID string `json:"follower_track_id"`
	GeometryID      string `json:"geometry_id"`
	MethodID        string `json:"method_id"`
	FirstUnixNanos  int64  `json:"first_unix_nanos"`
	LastUnixNanos   int64  `json:"last_unix_nanos"`
	// Stage is the least final of the path and every contributing sample.
	Stage              EstimateStage       `json:"estimate_stage"`
	Instants           []EncounterInstant  `json:"instants"`
	SpatialGapSeries   []SeriesPoint       `json:"spatial_gap_series,omitempty"`
	NetTimeGapSeries   []SeriesPoint       `json:"net_time_gap_series,omitempty"`
	PredictedGapSeries []PredictedPoint    `json:"predicted_gap_series,omitempty"`
	Accounting         EncounterAccounting `json:"accounting"`
	// Measurements is the production contract, in EncounterMetrics() order.
	Measurements []Measurement `json:"measurements"`
	// Provisional carries the same measurements with their values when the
	// encounter is not final. It is review-only.
	Provisional []Measurement `json:"provisional,omitempty"`
}

// EncounterMetrics lists the encounter measurements in emission order.
func EncounterMetrics() []MetricID {
	return []MetricID{
		MetricFollowingValidTime,
		MetricFollowingSpatialGapMin, MetricFollowingSpatialGapP50,
		MetricFollowingNetTimeGapMin, MetricFollowingNetTimeGapP50,
		MetricFollowingTimeBelow2000ms, MetricFollowingTimeBelow1500ms, MetricFollowingTimeBelow1000ms,
		MetricFollowingRateBelow2000ms, MetricFollowingRateBelow1500ms, MetricFollowingRateBelow1000ms,
	}
}

// Measurement returns the production measurement for one metric.
func (e Encounter) Measurement(id MetricID) (Measurement, bool) {
	return findMeasurement(e.Measurements, id)
}

// ProvisionalMeasurement returns the review-only measurement for one metric.
func (e Encounter) ProvisionalMeasurement(id MetricID) (Measurement, bool) {
	return findMeasurement(e.Provisional, id)
}

func findMeasurement(ms []Measurement, id MetricID) (Measurement, bool) {
	for _, m := range ms {
		if m.Name == id {
			return m, true
		}
	}
	return Measurement{}, false
}

// FollowerTimeline is every pairing decision for one follower, one per sample.
type FollowerTimeline struct {
	FollowerTrackID string `json:"follower_track_id"`
	// GeometryID is the follower's path; empty when it was refused.
	GeometryID string            `json:"geometry_id,omitempty"`
	Decisions  []PairingDecision `json:"decisions"`
}

// FollowingAnalysis is the following method's result for one capture: every
// track's local path, its pairing timeline as a follower, and every encounter.
type FollowingAnalysis struct {
	MethodID   string             `json:"method_id"`
	ParamsHash string             `json:"params_hash"`
	Paths      []LocalPathResult  `json:"paths"`
	Timelines  []FollowerTimeline `json:"timelines"`
	Encounters []Encounter        `json:"encounters"`
}

// Path returns one follower's path result.
func (a FollowingAnalysis) Path(followerTrackID string) (LocalPathResult, bool) {
	for _, p := range a.Paths {
		if p.FollowerTrackID == followerTrackID {
			return p, true
		}
	}
	return LocalPathResult{}, false
}

// Timeline returns one follower's timeline.
func (a FollowingAnalysis) Timeline(followerTrackID string) (FollowerTimeline, bool) {
	for _, t := range a.Timelines {
		if t.FollowerTrackID == followerTrackID {
			return t, true
		}
	}
	return FollowerTimeline{}, false
}

// Encounter returns one encounter.
func (a FollowingAnalysis) Encounter(leaderTrackID, followerTrackID string) (Encounter, bool) {
	for _, e := range a.Encounters {
		if e.LeaderTrackID == leaderTrackID && e.FollowerTrackID == followerTrackID {
			return e, true
		}
	}
	return Encounter{}, false
}

// AnalyseFollowing runs the following method over one capture's trajectories:
// every track is taken in turn as a follower, its local path is fitted, a
// leader is chosen at each of its instants, and each leader's instants are
// aggregated into an encounter. Output is ordered by track id, then leader id,
// whatever the input order. The trajectories must be valid, uniquely
// identified and from one estimator run.
func AnalyseFollowing(trajectories []Trajectory, params FollowingAnalysisParams) (FollowingAnalysis, error) {
	if err := params.Validate(); err != nil {
		return FollowingAnalysis{}, err
	}
	sorted, err := prepareTrajectories(trajectories)
	if err != nil {
		return FollowingAnalysis{}, err
	}
	byID := make(map[string]Trajectory, len(sorted))
	for _, t := range sorted {
		byID[t.Passage.TrackID] = t
	}
	out := FollowingAnalysis{MethodID: FollowingEncounterMethodID, ParamsHash: params.Hash()}
	for _, f := range sorted {
		res, err := buildLocalPath(f, sorted, params.Path)
		if err != nil {
			return FollowingAnalysis{}, err
		}
		timeline, encounters, err := analyseFollower(f, res, sorted, byID, params, out.ParamsHash)
		if err != nil {
			return FollowingAnalysis{}, err
		}
		out.Paths = append(out.Paths, res)
		out.Timelines = append(out.Timelines, timeline)
		out.Encounters = append(out.Encounters, encounters...)
	}
	return out, nil
}

func analyseFollower(f Trajectory, res LocalPathResult, all []Trajectory, byID map[string]Trajectory,
	params FollowingAnalysisParams, paramsHash string) (FollowerTimeline, []Encounter, error) {
	fid := f.Passage.TrackID
	timeline := FollowerTimeline{FollowerTrackID: fid}
	if res.Path != nil {
		timeline.GeometryID = res.Path.ID
	}
	instants := map[string][]EncounterInstant{}

	// Only tracks whose lifetime overlaps the follower's can be present at
	// any of its instants.
	first, last := f.Samples[0].CaptureUnixNanos, f.Samples[len(f.Samples)-1].CaptureUnixNanos
	var overlapping []Trajectory
	for _, o := range all {
		if o.Passage.TrackID != fid && o.Samples[0].CaptureUnixNanos <= last &&
			o.Samples[len(o.Samples)-1].CaptureUnixNanos >= first {
			overlapping = append(overlapping, o)
		}
	}

	for i, s := range f.Samples {
		t := s.CaptureUnixNanos
		var interval int64
		if i+1 < len(f.Samples) {
			interval = f.Samples[i+1].CaptureUnixNanos - t
		}
		var dec PairingDecision
		if res.Path == nil {
			dec = PairingDecision{FollowerTrackID: fid, CaptureUnixNanos: t, Reason: ReasonNoCommonPath, Condition: res.Conditions[0]}
		} else {
			fp, _ := PresenceAt(f, t)
			var others []Presence
			for _, o := range overlapping {
				if p, ok := PresenceAt(o, t); ok {
					others = append(others, p)
				}
			}
			var err error
			if dec, err = DecideLeader(res.Path, res, fp, others, params.Following, params.Pairing); err != nil {
				return FollowerTimeline{}, nil, fmt.Errorf("follower %s at %d: %w", fid, t, err)
			}
		}
		timeline.Decisions = append(timeline.Decisions, dec)

		for _, lid := range dec.Involved() {
			cand, _ := dec.Candidate(lid)
			inst := EncounterInstant{
				CaptureUnixNanos: t, IntervalNanos: interval,
				RecordGap: interval > params.Exposure.MaxIntervalNanos, Role: cand.Disposition,
			}
			leader := byID[lid]
			ls, synced := leader.SampleAt(t)
			inst.Unobserved = s.Support != SupportObserved || !synced || ls.Support != SupportObserved
			switch {
			case dec.LeaderTrackID != lid:
				inst.Reason, inst.Condition = dec.Reason, dec.Condition
			case !synced:
				// Present without a row at this instant: nothing to evaluate.
				inst.Reason = ReasonNotObserved
			default:
				pt, err := EvaluateFollowing(res.Path,
					Party{Passage: leader.Passage, Estimate: leader.Estimate, Sample: ls},
					Party{Passage: f.Passage, Estimate: f.Estimate, Sample: s}, params.Following)
				if err != nil {
					return FollowerTimeline{}, nil, fmt.Errorf("pair %s -> %s at %d: %w", lid, fid, t, err)
				}
				inst.Point = &pt
				if pt.SupportedOpportunity {
					inst.Valid = true
				} else {
					inst.Reason = firstPhysicalReason(pt.Reasons)
				}
			}
			instants[lid] = append(instants[lid], inst)
		}
	}

	leaders := make([]string, 0, len(instants))
	for lid := range instants {
		leaders = append(leaders, lid)
	}
	sort.Strings(leaders)
	var encounters []Encounter
	for _, lid := range leaders {
		e, err := buildEncounter(byID[lid], f, res.Path, instants[lid], params, paramsHash)
		if err != nil {
			return FollowerTimeline{}, nil, err
		}
		encounters = append(encounters, e)
	}
	return timeline, encounters, nil
}

// firstPhysicalReason is the first reason that is not publication stage. A
// point that is not supported opportunity always has one, because stage alone
// leaves opportunity intact.
func firstPhysicalReason(reasons []SuppressionReason) SuppressionReason {
	for _, r := range reasons {
		if r != ReasonEstimateNotFinal {
			return r
		}
	}
	return ReasonUnspecified
}

// spatialGapSupported reports whether a point's spatial gap is supported apart
// from stage. Standstill does not suppress it: only the time gap needs speed.
func spatialGapSupported(pt *FollowingPoint) bool {
	if pt == nil || pt.Gap == nil {
		return false
	}
	for _, r := range pt.Reasons {
		if r != ReasonBelowSpeedFloor && r != ReasonEstimateNotFinal {
			return false
		}
	}
	return true
}

func buildEncounter(leader, follower Trajectory, path *LocalPath, instants []EncounterInstant,
	params FollowingAnalysisParams, paramsHash string) (Encounter, error) {
	e := Encounter{
		LeaderTrackID: leader.Passage.TrackID, FollowerTrackID: follower.Passage.TrackID,
		GeometryID: path.ID, MethodID: FollowingEncounterMethodID + "/" + paramsHash,
		FirstUnixNanos: instants[0].CaptureUnixNanos, LastUnixNanos: instants[len(instants)-1].CaptureUnixNanos,
		Stage: path.Stage(), Instants: instants,
	}
	bands := FollowingBands()
	e.Accounting.BandNanos = make([]int64, len(bands))
	tallies := map[SuppressionReason]*ReasonTally{}
	var observed, coasted int
	var mc []mcInstant
	for _, inst := range instants {
		fs, _ := follower.SampleAt(inst.CaptureUnixNanos)
		e.Stage = min(e.Stage, fs.Stage)
		ls, synced := leader.SampleAt(inst.CaptureUnixNanos)
		if synced {
			e.Stage = min(e.Stage, ls.Stage)
		}
		counted := inst.IntervalNanos
		if inst.RecordGap {
			e.Accounting.RecordGapNanos += inst.IntervalNanos
			counted = 0
		}
		if inst.Unobserved {
			e.Accounting.UnobservedNanos += counted
		}
		pt := inst.Point
		var in mcInstant
		if inst.Valid {
			e.Accounting.ValidNanos += counted
			thw := SeriesPoint{inst.CaptureUnixNanos, *pt.TimeGap.ValueS, *pt.TimeGap.SigmaS}
			for b, band := range bands {
				if band.Contains(thw.Value) {
					e.Accounting.BandNanos[b] += counted
				}
			}
			e.NetTimeGapSeries = append(e.NetTimeGapSeries, thw)
			in.thw, in.hasThw, in.nanos = thw, true, counted
		} else {
			tally := tallies[inst.Reason]
			if tally == nil {
				tally = &ReasonTally{Reason: inst.Reason}
				tallies[inst.Reason] = tally
			}
			tally.Instants++
			tally.Nanos += counted
		}
		if pt == nil {
			continue
		}
		// A point exists only on a synchronised leader sample.
		for _, sup := range []SupportState{fs.Support, ls.Support} {
			if sup == SupportObserved {
				observed++
			} else {
				coasted++
			}
		}
		if spatialGapSupported(pt) {
			gap := SeriesPoint{inst.CaptureUnixNanos, pt.Gap.ValueM, pt.Gap.SigmaM}
			e.SpatialGapSeries = append(e.SpatialGapSeries, gap)
			in.gap, in.hasGap = gap, true
		}
		if pt.PredictedGap != nil && !pt.PredictedGap.Suppressed {
			e.PredictedGapSeries = append(e.PredictedGapSeries, PredictedPoint{
				CaptureUnixNanos: inst.CaptureUnixNanos, ValueM: pt.Gap.ValueM, SigmaM: pt.Gap.SigmaM,
				CoastAgeNanos: pt.CoastAgeNanos,
			})
		}
		if in.hasGap || in.hasThw {
			mc = append(mc, in)
		}
	}
	for _, r := range SuppressionReasons() {
		if t := tallies[r]; t != nil {
			e.Accounting.Suppressions = append(e.Accounting.Suppressions, *t)
		}
	}

	stats := encounterStatistics(mc, e.Accounting, params.Exposure,
		monteCarloSeed(FollowingEncounterMethodID, paramsHash, e.GeometryID, e.LeaderTrackID, e.FollowerTrackID))
	prov := Provenance{
		Version: VersionProvenance{
			EstimateStage: e.Stage, EstimatorID: follower.Estimate.EstimatorID, ObsModelID: follower.Estimate.ObsModelID,
			MethodID: e.MethodID, GeometryID: e.GeometryID, ParamHash: follower.Estimate.ParamHash,
		},
		Input: InputProvenance{
			ContributingTrackIDs: []string{e.LeaderTrackID, e.FollowerTrackID},
			FirstUnixNanos:       e.FirstUnixNanos, LastUnixNanos: e.LastUnixNanos,
			ObservedFrames: observed, CoastedFrames: coasted, PlanarFallback: true,
		},
	}
	classReason, err := ClassApplicability(MetricFollowingSpatialGap, follower.Passage.MotionClass, leader.Passage.MotionClass)
	if err != nil {
		return Encounter{}, err
	}
	if e.Measurements, err = encounterMeasurements(stats, classReason, e.Stage != StageFinal, params.Exposure, prov); err != nil {
		return Encounter{}, err
	}
	if e.Stage != StageFinal {
		if e.Provisional, err = encounterMeasurements(stats, classReason, false, params.Exposure, prov); err != nil {
			return Encounter{}, err
		}
	}
	return e, nil
}

// mcInstant is one instant's contribution to the Monte Carlo draw: its
// supported gap, its valid time gap, or both.
type mcInstant struct {
	gap, thw       SeriesPoint
	hasGap, hasThw bool
	nanos          int64 // valid time the instant stands for, toward bands
}

// intervalStat is a statistic's point value and its interval.
type intervalStat struct {
	value, lower, upper float64
}

// encounterStats are the statistics an encounter's measurements are built
// from; a nil statistic had an empty series.
type encounterStats struct {
	gapMin, gapP50, thwMin, thwP50 *intervalStat
	validNanos                     int64
	band, rate                     []intervalStat
	coverage                       float64
	samples                        int
}

func encounterStatistics(in []mcInstant, acc EncounterAccounting, p ExposureParams, seed uint64) encounterStats {
	bands := FollowingBands()
	st := encounterStats{validNanos: acc.ValidNanos, coverage: p.IntervalCoverage, samples: p.MonteCarloSamples}
	var gaps, thws []float64
	for _, x := range in {
		if x.hasGap {
			gaps = append(gaps, x.gap.Value)
		}
		if x.hasThw {
			thws = append(thws, x.thw.Value)
		}
	}
	n := p.MonteCarloSamples
	draws := func() []float64 { return make([]float64, n) }
	gapMin, gapP50, thwMin, thwP50 := draws(), draws(), draws(), draws()
	bandDraws := make([][]float64, len(bands))
	for b := range bands {
		bandDraws[b] = draws()
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	common, own := math.Sqrt(p.CommonModeFraction), math.Sqrt(1-p.CommonModeFraction)
	gapBuf, thwBuf := make([]float64, 0, len(gaps)), make([]float64, 0, len(thws))
	for j := 0; j < n; j++ {
		zc := rng.NormFloat64()
		gapBuf, thwBuf = gapBuf[:0], thwBuf[:0]
		bandNanos := make([]int64, len(bands))
		for _, x := range in {
			e := common*zc + own*rng.NormFloat64()
			if x.hasGap {
				gapBuf = append(gapBuf, x.gap.Value+x.gap.Sigma*e)
			}
			if x.hasThw {
				v := x.thw.Value + x.thw.Sigma*e
				thwBuf = append(thwBuf, v)
				for b, band := range bands {
					if band.Contains(v) {
						bandNanos[b] += x.nanos
					}
				}
			}
		}
		if len(gapBuf) > 0 {
			gapMin[j], gapP50[j] = minMedian(gapBuf)
		}
		if len(thwBuf) > 0 {
			thwMin[j], thwP50[j] = minMedian(thwBuf)
		}
		for b := range bands {
			bandDraws[b][j] = float64(bandNanos[b]) / 1e9
		}
	}
	interval := func(value float64, d []float64) *intervalStat {
		lo, hi := quantilePair(d, p.IntervalCoverage)
		return &intervalStat{value: value, lower: lo, upper: hi}
	}
	if len(gaps) > 0 {
		lo, med := minMedian(append([]float64(nil), gaps...))
		st.gapMin, st.gapP50 = interval(lo, gapMin), interval(med, gapP50)
	}
	if len(thws) > 0 {
		lo, med := minMedian(append([]float64(nil), thws...))
		st.thwMin, st.thwP50 = interval(lo, thwMin), interval(med, thwP50)
	}
	valid := float64(acc.ValidNanos) / 1e9
	for b := range bands {
		band := *interval(float64(acc.BandNanos[b])/1e9, bandDraws[b])
		st.band = append(st.band, band)
		if acc.ValidNanos > 0 {
			// Valid time is fixed across draws, so the rate's interval is the
			// band interval scaled; the ratio of integer nanoseconds keeps the
			// point value exact.
			st.rate = append(st.rate, intervalStat{
				value: float64(acc.BandNanos[b]) / float64(acc.ValidNanos),
				lower: band.lower / valid, upper: band.upper / valid,
			})
		}
	}
	return st
}

// minMedian sorts its argument in place and returns its minimum and median.
func minMedian(v []float64) (float64, float64) {
	sort.Float64s(v)
	n := len(v)
	if n%2 == 1 {
		return v[0], v[n/2]
	}
	return v[0], (v[n/2-1] + v[n/2]) / 2
}

// quantilePair sorts draws in place and returns the nearest-rank quantiles at
// (1 - coverage)/2 and (1 + coverage)/2.
func quantilePair(draws []float64, coverage float64) (float64, float64) {
	sort.Float64s(draws)
	rank := func(q float64) float64 {
		i := int(math.Ceil(q*float64(len(draws)))) - 1
		return draws[min(max(i, 0), len(draws)-1)]
	}
	return rank((1 - coverage) / 2), rank((1 + coverage) / 2)
}

func monteCarloSeed(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// encounterMeasurements builds the encounter's measurements in
// EncounterMetrics() order, each a value or its most fundamental reason:
// class, then minimum opportunity or an empty series, then, when guarded,
// publication stage.
func encounterMeasurements(st encounterStats, classReason SuppressionReason, notFinal bool,
	p ExposureParams, prov Provenance) ([]Measurement, error) {
	var base reasonSet
	base.add(classReason)
	if notFinal {
		base.add(ReasonEstimateNotFinal)
	}
	validSeconds := float64(st.validNanos) / 1e9
	opportunity := base
	if validSeconds < p.MinOpportunitySeconds {
		opportunity.add(ReasonInsufficientObservation)
	}
	bands := FollowingBands()

	var out []Measurement
	emit := func(id MetricID, reasons reasonSet, stat *intervalStat, withOpportunity bool) error {
		if stat == nil {
			reasons.add(ReasonInsufficientObservation)
		}
		if r := reasons.first(); r != ReasonUnspecified {
			m, err := NewSuppressedMeasurement(id, r, prov)
			out = append(out, m)
			return err
		}
		u := Uncertainty{
			Kind: UncertaintyInterval, Lower: ptr(stat.lower), Upper: ptr(stat.upper),
			Coverage: ptr(st.coverage), Method: MethodMonteCarlo, Samples: st.samples,
		}
		m, err := NewMeasurement(id, stat.value, u, prov)
		if err == nil && withOpportunity {
			m.OpportunitySeconds = ptr(validSeconds)
			err = m.Validate()
		}
		out = append(out, m)
		return err
	}

	// Valid time is an accounting sum: a value, even of zero, with its
	// uncertainty declared unavailable rather than claimed exact.
	if r := base.first(); r != ReasonUnspecified {
		m, err := NewSuppressedMeasurement(MetricFollowingValidTime, r, prov)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	} else {
		m, err := NewMeasurement(MetricFollowingValidTime, validSeconds, NoUncertainty(), prov)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	for _, s := range []struct {
		id   MetricID
		stat *intervalStat
	}{
		{MetricFollowingSpatialGapMin, st.gapMin}, {MetricFollowingSpatialGapP50, st.gapP50},
		{MetricFollowingNetTimeGapMin, st.thwMin}, {MetricFollowingNetTimeGapP50, st.thwP50},
	} {
		if err := emit(s.id, base, s.stat, false); err != nil {
			return nil, err
		}
	}
	for b, band := range bands {
		if err := emit(band.Duration, opportunity, &st.band[b], true); err != nil {
			return nil, err
		}
	}
	for b, band := range bands {
		var stat *intervalStat
		if b < len(st.rate) {
			stat = &st.rate[b]
		}
		if err := emit(band.Rate, opportunity, stat, true); err != nil {
			return nil, err
		}
	}
	return out, nil
}
