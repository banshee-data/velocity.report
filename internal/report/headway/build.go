package headway

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// statements are printed verbatim under the status on the first page. They
// state what the numbers are and are not, in the plan's terms (Sections 1,
// 6, 8.3, 9.2 and 10.4).
var statements = []string{
	"Every result is an observable of one passage pair at one site, with its unit, uncertainty and provenance. " +
		"Nothing here describes a road user beyond the observed passage.",
	"Spatial gap is bumper to bumper along the pair's shared directed path; net time gap is that gap over the " +
		"follower's along-path speed. Neither is centre to centre, and neither is passage headway at a fixed line.",
	"The named bands are descriptive bins with no established threshold (no_established_threshold). " +
		"They are not a safety standard, and nothing here sorts road users by them.",
	"A suppressed value carries its reason and no number. It is never shown as zero, and time that is not " +
		"valid following time is listed by reason rather than dropped.",
	"The predicted gap is review_only. It is shown apart, with its coast age, and never enters valid following " +
		"time, band exposure or an aggregate.",
	"Aggregates pool supported encounter values within one version group only, with their sample counts and " +
		"opportunity denominators.",
}

// Input is what a report is built from: its status and, per capture, the
// trajectories and their following analysis, held in memory.
type Input struct {
	Status   Status
	Captures []CaptureInput
}

// CaptureInput is one capture. Analysis must be AnalyseFollowing's result
// over Trajectories with Params.
type CaptureInput struct {
	ID          string
	Description string
	// Source locates the trajectories; each track's locator is Source, a
	// slash, and its track id.
	Source       string
	Trajectories []l8behaviour.Trajectory
	Params       l8behaviour.FollowingAnalysisParams
	Analysis     l8behaviour.FollowingAnalysis
}

// Build assembles the report from its input. It refuses a promoted status,
// a status that disagrees with where the trajectories came from, and any
// input the analysis could not have produced.
func Build(in Input) (Report, error) {
	if in.Status == StatusPromoted {
		return Report{}, ErrPromotionGated
	}
	if !in.Status.Valid() {
		return Report{}, fmt.Errorf("headway report status is %s", in.Status)
	}
	if len(in.Captures) == 0 {
		return Report{}, errors.New("headway report needs at least one capture")
	}
	if err := checkHistogramEdges(); err != nil {
		return Report{}, err
	}
	r := Report{
		Contract: ContractID, Status: in.Status, StatusLabel: in.Status.Label(), StatusNote: in.Status.Note(),
		Title: Title, Statements: append([]string(nil), statements...),
		Methods: Methods{
			Encounter: l8behaviour.FollowingEncounterMethodID, LocalPath: l8behaviour.LocalPathMethodID,
			Pairing: l8behaviour.PairingMethodID, Sync: l8behaviour.SyncMethodID,
			Pointwise: l8behaviour.FollowingMethodID, Exposure: l8behaviour.ExposureMethodID,
		},
		Bands: reportBands(),
	}
	seen := map[string]bool{}
	var sources []l8behaviour.Encounter // parallel to r.Encounters
	for _, c := range in.Captures {
		if c.ID == "" || c.Source == "" {
			return Report{}, errors.New("capture requires an id and a source")
		}
		if seen[c.ID] {
			return Report{}, fmt.Errorf("capture %s appears twice", c.ID)
		}
		seen[c.ID] = true
		capture, rows, encs, err := buildCapture(c, in.Status, len(r.Encounters))
		if err != nil {
			return Report{}, fmt.Errorf("capture %s: %w", c.ID, err)
		}
		r.Captures = append(r.Captures, capture)
		r.Encounters = append(r.Encounters, rows...)
		sources = append(sources, encs...)
	}
	aggs, err := buildAggregates(r.Encounters, sources)
	if err != nil {
		return Report{}, err
	}
	r.Aggregates = aggs
	if err := r.Validate(); err != nil {
		return Report{}, err
	}
	return r, nil
}

func reportBands() []Band {
	var out []Band
	for _, b := range l8behaviour.FollowingBands() {
		d, _ := l8behaviour.LookupMetric(b.Duration)
		rt, _ := l8behaviour.LookupMetric(b.Rate)
		out = append(out, Band{
			Seconds: b.Seconds, Display: formatWithUnit(b.Seconds, "s"),
			Duration: b.Duration, DurationBenchmark: d.Benchmark, Rate: b.Rate, RateBenchmark: rt.Benchmark,
		})
	}
	return out
}

// checkHistogramEdges requires every band threshold on a bin edge, so the
// distribution splits exactly where the bands do.
func checkHistogramEdges() error {
	for _, b := range l8behaviour.FollowingBands() {
		ms := int(math.Round(b.Seconds * 1000))
		if float64(ms)/1000 != b.Seconds || ms%HistogramBinMillis != 0 || ms > HistogramMaxMillis {
			return fmt.Errorf("band %g s is not an edge of the %d ms histogram bins", b.Seconds, HistogramBinMillis)
		}
	}
	return nil
}

// checkStatusSource ties the status to the trajectories' origin: analytic
// fixtures are a synthetic oracle, and a synthetic oracle is nothing else.
func checkStatusSource(status Status, estimate l8behaviour.EstimateIdentity) error {
	fixture := estimate == l8behaviour.FixtureEstimate()
	switch {
	case fixture && status != StatusSyntheticOracle:
		return fmt.Errorf("trajectories from the analytic fixture estimator must be reported as %s, not %s",
			StatusSyntheticOracle, status)
	case !fixture && status == StatusSyntheticOracle:
		return fmt.Errorf("a %s report may contain only analytic fixture trajectories, got estimator %s",
			StatusSyntheticOracle, estimate.EstimatorID)
	}
	return nil
}

func buildCapture(c CaptureInput, status Status, encounterBase int) (Capture, []Encounter, []l8behaviour.Encounter, error) {
	if len(c.Trajectories) == 0 {
		return Capture{}, nil, nil, errors.New("capture has no trajectories")
	}
	byID := make(map[string]l8behaviour.Trajectory, len(c.Trajectories))
	estimate := c.Trajectories[0].Estimate
	first, last := int64(math.MaxInt64), int64(math.MinInt64)
	for _, t := range c.Trajectories {
		if err := t.Validate(); err != nil {
			return Capture{}, nil, nil, err
		}
		if _, dup := byID[t.Passage.TrackID]; dup {
			return Capture{}, nil, nil, fmt.Errorf("track %s appears twice", t.Passage.TrackID)
		}
		if t.Estimate != estimate {
			return Capture{}, nil, nil, errors.New("capture mixes estimator versions")
		}
		byID[t.Passage.TrackID] = t
		first = min(first, t.Samples[0].CaptureUnixNanos)
		last = max(last, t.Samples[len(t.Samples)-1].CaptureUnixNanos)
	}
	if err := checkStatusSource(status, estimate); err != nil {
		return Capture{}, nil, nil, err
	}
	if err := c.Params.Validate(); err != nil {
		return Capture{}, nil, nil, err
	}
	if c.Analysis.MethodID != l8behaviour.FollowingEncounterMethodID || c.Analysis.ParamsHash != c.Params.Hash() {
		return Capture{}, nil, nil, fmt.Errorf("analysis %s/%s was not produced by %s with these parameters (%s)",
			c.Analysis.MethodID, c.Analysis.ParamsHash, l8behaviour.FollowingEncounterMethodID, c.Params.Hash())
	}

	capture := Capture{
		ID: c.ID, Description: c.Description, Source: c.Source, Estimate: estimate,
		MethodID: c.Analysis.MethodID, ParamsHash: c.Analysis.ParamsHash, Params: c.Params,
		FirstUnixNanos: first, LastUnixNanos: last, FirstUTC: formatUTC(first), LastUTC: formatUTC(last),
	}
	maxInterval := c.Params.Exposure.MaxIntervalNanos
	for _, tl := range c.Analysis.Timelines {
		t, ok := byID[tl.FollowerTrackID]
		if !ok {
			return Capture{}, nil, nil, fmt.Errorf("timeline for unknown track %s", tl.FollowerTrackID)
		}
		path, _ := c.Analysis.Path(tl.FollowerTrackID)
		f, err := buildFollower(tl, t, path, maxInterval)
		if err != nil {
			return Capture{}, nil, nil, err
		}
		capture.Followers = append(capture.Followers, f)
	}

	var rows []Encounter
	for _, e := range c.Analysis.Encounters {
		id := fmt.Sprintf("E%d", encounterBase+len(rows)+1)
		row, err := buildEncounter(id, c, byID, e)
		if err != nil {
			return Capture{}, nil, nil, fmt.Errorf("encounter %s -> %s: %w", e.LeaderTrackID, e.FollowerTrackID, err)
		}
		rows = append(rows, row)
	}
	return capture, rows, c.Analysis.Encounters, nil
}

// reasonKey orders reason/condition tallies by precedence.
type reasonKey struct {
	reason    l8behaviour.SuppressionReason
	condition l8behaviour.PathCondition
}

func orderedReasonTimes(m map[reasonKey]*ReasonTime) []ReasonTime {
	out := make([]ReasonTime, 0, len(m))
	for _, rt := range m {
		rt.Display = formatNanos(rt.Nanos)
		out = append(out, *rt)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Reason != out[j].Reason {
			return out[i].Reason < out[j].Reason
		}
		return out[i].Condition < out[j].Condition
	})
	return out
}

// buildFollower accounts one follower's passage by pairing decision, sample
// and hold as the encounter method does: each sample stands for the time to
// the next, the last for nothing, and an over-long interval is a record gap.
func buildFollower(tl l8behaviour.FollowerTimeline, t l8behaviour.Trajectory, path l8behaviour.LocalPathResult,
	maxInterval int64) (Follower, error) {
	if len(tl.Decisions) != len(t.Samples) {
		return Follower{}, fmt.Errorf("follower %s has %d decisions for %d samples", tl.FollowerTrackID, len(tl.Decisions), len(t.Samples))
	}
	f := Follower{
		TrackID: tl.FollowerTrackID, GeometryID: tl.GeometryID, Instants: len(tl.Decisions),
		PathConditions: append([]l8behaviour.PathCondition(nil), path.Conditions...),
	}
	suppressed := map[reasonKey]*ReasonTime{}
	for i, dec := range tl.Decisions {
		var counted int64
		if i+1 < len(t.Samples) {
			counted = t.Samples[i+1].CaptureUnixNanos - t.Samples[i].CaptureUnixNanos
		}
		if counted > maxInterval {
			f.RecordGapNanos += counted
			counted = 0
		}
		switch {
		case dec.LeaderTrackID != "":
			f.LeaderNanos += counted
		case dec.Reason != l8behaviour.ReasonUnspecified:
			k := reasonKey{dec.Reason, dec.Condition}
			if suppressed[k] == nil {
				suppressed[k] = &ReasonTime{Reason: dec.Reason, Condition: dec.Condition}
			}
			suppressed[k].Instants++
			suppressed[k].Nanos += counted
		default:
			f.FreeFlowNanos += counted
		}
	}
	f.Suppressed = orderedReasonTimes(suppressed)
	f.LeaderDisplay, f.FreeFlowDisplay = formatNanos(f.LeaderNanos), formatNanos(f.FreeFlowNanos)
	f.RecordGapDisplay = formatNanos(f.RecordGapNanos)
	return f, nil
}

func buildTrack(c CaptureInput, t l8behaviour.Trajectory) Track {
	return Track{
		TrackID: t.Passage.TrackID, Locator: c.Source + "/" + t.Passage.TrackID,
		SiteID: t.Passage.SiteID, SensorID: t.Passage.SensorID,
		MotionClass: t.Passage.MotionClass, ClassLabel: t.Passage.ClassLabel, Estimate: t.Estimate,
		Samples:  len(t.Samples),
		FirstUTC: formatUTC(t.Samples[0].CaptureUnixNanos), LastUTC: formatUTC(t.Samples[len(t.Samples)-1].CaptureUnixNanos),
	}
}

// valueBlock picks the list a row reads: production for a final encounter,
// the review-labelled provisional list otherwise.
func valueBlock(e l8behaviour.Encounter) ([]l8behaviour.Measurement, ValueBlock) {
	if e.Stage == l8behaviour.StageFinal {
		return e.Measurements, ValueBlockMeasurements
	}
	return e.Provisional, ValueBlockProvisional
}

// counted is the time an instant stands for: none when it is a record gap.
func counted(inst l8behaviour.EncounterInstant) int64 {
	if inst.RecordGap {
		return 0
	}
	return inst.IntervalNanos
}

func buildEncounter(id string, c CaptureInput, byID map[string]l8behaviour.Trajectory, e l8behaviour.Encounter) (Encounter, error) {
	leader, ok := byID[e.LeaderTrackID]
	if !ok {
		return Encounter{}, fmt.Errorf("unknown leader %s", e.LeaderTrackID)
	}
	follower, ok := byID[e.FollowerTrackID]
	if !ok {
		return Encounter{}, fmt.Errorf("unknown follower %s", e.FollowerTrackID)
	}
	if len(e.Instants) == 0 {
		return Encounter{}, errors.New("encounter has no instants")
	}
	pathResult, _ := c.Analysis.Path(e.FollowerTrackID)
	if pathResult.Path == nil || pathResult.Path.ID != e.GeometryID {
		return Encounter{}, fmt.Errorf("encounter geometry %s is not the follower's path", e.GeometryID)
	}
	block, vb := valueBlock(e)
	ids := l8behaviour.EncounterMetrics()
	if len(block) != len(ids) {
		return Encounter{}, fmt.Errorf("%s holds %d measurements, want %d", vb, len(block), len(ids))
	}
	lastInst := e.Instants[len(e.Instants)-1]
	end := lastInst.CaptureUnixNanos + counted(lastInst)
	row := Encounter{
		ID: id, CaptureID: c.ID, Path: buildPath(pathResult),
		Leader: buildTrack(c, leader), Follower: buildTrack(c, follower),
		FirstUnixNanos: e.FirstUnixNanos, LastUnixNanos: e.LastUnixNanos,
		FirstUTC: formatUTC(e.FirstUnixNanos), LastUTC: formatUTC(e.LastUnixNanos),
		DurationNanos: end - e.FirstUnixNanos, DurationDisplay: formatNanos(end - e.FirstUnixNanos),
		Stage: e.Stage, ValueBlock: vb,
		Version: block[0].Provenance.Version, Input: cloneInput(block[0].Provenance.Input),
	}
	for i, m := range block {
		if m.Name != ids[i] {
			return Encounter{}, fmt.Errorf("%s measurement %d is %s, want %s", vb, i, m.Name, ids[i])
		}
		v, err := valueRow(m)
		if err != nil {
			return Encounter{}, err
		}
		row.Measurements = append(row.Measurements, v)
	}
	row.Accounting = buildAccounting(e.Accounting)
	row.Endpoints = buildEndpoints(e)
	row.Unsupported = buildUnsupported(e)
	row.Predicted = buildPredicted(e)
	row.Series = buildSeries(e)
	return row, nil
}

func buildPath(r l8behaviour.LocalPathResult) Path {
	p := Path{
		GeometryID: r.Path.ID, MethodID: r.MethodID, Stage: r.Path.EstimateStage,
		LengthM: r.Path.LengthM(), LengthDisplay: formatWithUnit(r.Path.LengthM(), "m"),
		Knots: len(r.Path.Knots), MemberTrackIDs: append([]string(nil), r.MemberTrackIDs...),
	}
	for _, k := range r.Path.Knots {
		if k.Bridged {
			p.BridgedKnots++
		}
	}
	return p
}

func cloneInput(in l8behaviour.InputProvenance) l8behaviour.InputProvenance {
	in.ContributingTrackIDs = append([]string(nil), in.ContributingTrackIDs...)
	return in
}

func cloneFloat(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneUncertainty(u *l8behaviour.Uncertainty) *l8behaviour.Uncertainty {
	if u == nil {
		return nil
	}
	c := *u
	c.Sigma, c.Lower, c.Upper, c.Coverage = cloneFloat(u.Sigma), cloneFloat(u.Lower), cloneFloat(u.Upper), cloneFloat(u.Coverage)
	if u.Scopes != nil {
		s := l8behaviour.UncertaintyScopes{
			TrackLocal: cloneFloat(u.Scopes.TrackLocal), SharedSensor: cloneFloat(u.Scopes.SharedSensor),
			SharedGeometry: cloneFloat(u.Scopes.SharedGeometry),
		}
		c.Scopes = &s
	}
	return &c
}

// valueRow copies a registered measurement into the report, with the text it
// prints. The copy shares nothing with the analysis.
func valueRow(m l8behaviour.Measurement) (Value, error) {
	if err := m.Validate(); err != nil {
		return Value{}, err
	}
	def, _ := l8behaviour.LookupMetric(m.Name) // Validate checked it is registered
	v := Value{
		Metric: m.Name, Unit: m.Unit, Estimator: def.Estimator, Visibility: def.Visibility, Benchmark: def.Benchmark,
		Suppressed: m.Suppressed, Reason: m.Reason,
	}
	if m.Suppressed {
		v.Display = suppressedDisplay(m.Reason)
		return v, nil
	}
	v.Value, v.Uncertainty = cloneFloat(m.Value), cloneUncertainty(m.Uncertainty)
	v.Display = formatWithUnit(*m.Value, m.Unit)
	v.UncertaintyDisplay = formatUncertainty(m.Uncertainty, m.Unit)
	if m.OpportunitySeconds != nil {
		v.OpportunitySeconds = cloneFloat(m.OpportunitySeconds)
		v.OpportunityDisplay = formatWithUnit(*m.OpportunitySeconds, "s")
	}
	return v, nil
}

func buildAccounting(a l8behaviour.EncounterAccounting) Accounting {
	out := Accounting{
		ValidNanos: a.ValidNanos, AccountedNanos: a.ValidNanos,
		UnobservedNanos: a.UnobservedNanos, RecordGapNanos: a.RecordGapNanos,
		Suppressions: []ReasonTime{},
	}
	for _, t := range a.Suppressions {
		out.AccountedNanos += t.Nanos
		out.Suppressions = append(out.Suppressions, ReasonTime{
			Reason: t.Reason, Instants: t.Instants, Nanos: t.Nanos, Display: formatNanos(t.Nanos),
		})
	}
	out.ValidDisplay, out.AccountedDisplay = formatNanos(out.ValidNanos), formatNanos(out.AccountedNanos)
	out.UnobservedDisplay, out.RecordGapDisplay = formatNanos(out.UnobservedNanos), formatNanos(out.RecordGapNanos)
	return out
}

// buildEndpoints tallies the endpoint source of every instant whose gap was
// computed, published or not, so a prior-dominated endpoint is counted even
// where its gap was suppressed, and records both endpoints at the minimum
// supported gap (the first such instant on a tie).
func buildEndpoints(e l8behaviour.Encounter) Endpoints {
	leaderSrc, followerSrc := map[l8behaviour.EndpointSource]int{}, map[l8behaviour.EndpointSource]int{}
	byCapture := map[int64]l8behaviour.EncounterInstant{}
	for _, inst := range e.Instants {
		byCapture[inst.CaptureUnixNanos] = inst
		if inst.Point == nil || inst.Point.Gap == nil {
			continue
		}
		leaderSrc[inst.Point.Gap.LeaderSource]++
		followerSrc[inst.Point.Gap.FollowerSource]++
	}
	tally := func(m map[l8behaviour.EndpointSource]int) []SourceCount {
		out := []SourceCount{}
		for _, s := range l8behaviour.EndpointSources() {
			if m[s] > 0 {
				out = append(out, SourceCount{Source: s, Instants: m[s]})
			}
		}
		return out
	}
	out := Endpoints{LeaderTrailing: tally(leaderSrc), FollowerLeading: tally(followerSrc)}
	best := -1
	for i, p := range e.SpatialGapSeries {
		if best < 0 || p.Value < e.SpatialGapSeries[best].Value {
			best = i
		}
	}
	if best >= 0 {
		p := e.SpatialGapSeries[best]
		pt := byCapture[p.CaptureUnixNanos].Point
		endpoint := func(ep l8behaviour.Endpoint) EndpointEstimate {
			return EndpointEstimate{
				TrackID: ep.TrackID, Extremity: ep.Extremity, ArcM: ep.ArcM, SigmaM: ep.SigmaM,
				Source: ep.Source, Support: ep.Support,
				Display: formatWithUnit(ep.ArcM, "m") + ", sigma " + formatWithUnit(ep.SigmaM, "m"),
			}
		}
		offset := p.CaptureUnixNanos - e.FirstUnixNanos
		out.AtMinimumGap = &EndpointPair{
			OffsetNanos: offset, OffsetDisplay: formatNanos(offset),
			Leader: endpoint(pt.Leader.Trailing), Follower: endpoint(pt.Follower.Leading),
			GapDisplay: formatWithUnit(p.Value, "m") + ", sigma " + formatWithUnit(p.Sigma, "m"),
		}
	}
	return out
}

// adjacent reports whether b is the follower sample right after a, with no
// record gap between them.
func adjacent(a, b l8behaviour.EncounterInstant) bool {
	return !a.RecordGap && a.CaptureUnixNanos+a.IntervalNanos == b.CaptureUnixNanos
}

// buildUnsupported groups consecutive instants that are not valid following
// time by reason, path condition and the leader's role. A run ends at a
// change of any of them, at an instant the encounter skips, and at a record
// gap.
func buildUnsupported(e l8behaviour.Encounter) []Interval {
	out := []Interval{}
	var prev *l8behaviour.EncounterInstant
	for i := range e.Instants {
		inst := e.Instants[i]
		if inst.Valid {
			prev = nil
			continue
		}
		n := len(out)
		if prev != nil && n > 0 && adjacent(*prev, inst) && out[n-1].Reason == inst.Reason &&
			out[n-1].Condition == inst.Condition && out[n-1].Role == inst.Role {
			out[n-1].Instants++
			out[n-1].Nanos += counted(inst)
			out[n-1].EndNanos = inst.CaptureUnixNanos + counted(inst) - e.FirstUnixNanos
		} else {
			start := inst.CaptureUnixNanos - e.FirstUnixNanos
			out = append(out, Interval{
				StartNanos: start, EndNanos: start + counted(inst), Instants: 1, Nanos: counted(inst),
				Reason: inst.Reason, Condition: inst.Condition, Role: inst.Role,
			})
		}
		prev = &e.Instants[i]
	}
	for i := range out {
		out[i].RangeDisplay = formatRange(out[i].StartNanos, out[i].EndNanos)
		out[i].DurationDisplay = formatNanos(out[i].Nanos)
	}
	return out
}

// buildPredicted groups consecutive review-only predicted gaps into runs.
func buildPredicted(e l8behaviour.Encounter) []PredictedInterval {
	out := []PredictedInterval{}
	byCapture := map[int64]l8behaviour.EncounterInstant{}
	for _, inst := range e.Instants {
		byCapture[inst.CaptureUnixNanos] = inst
	}
	type run struct {
		first, last l8behaviour.EncounterInstant
		pts         []l8behaviour.PredictedPoint
	}
	var runs []run
	for _, p := range e.PredictedGapSeries {
		inst := byCapture[p.CaptureUnixNanos]
		if n := len(runs); n > 0 && adjacent(runs[n-1].last, inst) {
			runs[n-1].last = inst
			runs[n-1].pts = append(runs[n-1].pts, p)
			continue
		}
		runs = append(runs, run{first: inst, last: inst, pts: []l8behaviour.PredictedPoint{p}})
	}
	for _, r := range runs {
		pi := PredictedInterval{
			Metric: l8behaviour.MetricFollowingPredictedGap, Visibility: l8behaviour.VisibilityReviewOnly,
			StartNanos: r.first.CaptureUnixNanos - e.FirstUnixNanos,
			EndNanos:   r.last.CaptureUnixNanos + counted(r.last) - e.FirstUnixNanos,
			Instants:   len(r.pts),
		}
		gapLo, gapHi, sigmaHi := math.Inf(1), math.Inf(-1), 0.0
		pi.CoastAgeMinNanos, pi.CoastAgeMaxNanos = math.MaxInt64, 0
		for _, p := range r.pts {
			gapLo, gapHi, sigmaHi = math.Min(gapLo, p.ValueM), math.Max(gapHi, p.ValueM), math.Max(sigmaHi, p.SigmaM)
			pi.CoastAgeMinNanos, pi.CoastAgeMaxNanos = min(pi.CoastAgeMinNanos, p.CoastAgeNanos), max(pi.CoastAgeMaxNanos, p.CoastAgeNanos)
		}
		pi.RangeDisplay = formatRange(pi.StartNanos, pi.EndNanos)
		pi.CoastAgeDisplay = formatSpread(float64(pi.CoastAgeMinNanos)/1e9, float64(pi.CoastAgeMaxNanos)/1e9, "s")
		pi.GapDisplay = formatSpread(gapLo, gapHi, "m")
		pi.SigmaMaxDisplay = formatWithUnit(sigmaHi, "m")
		out = append(out, pi)
	}
	return out
}

func buildSeries(e l8behaviour.Encounter) Series {
	s := Series{Gap: []SeriesPoint{}, NetTimeGap: []SeriesPoint{}, Predicted: []PredictedPoint{}}
	for _, p := range e.SpatialGapSeries {
		s.Gap = append(s.Gap, SeriesPoint{OffsetNanos: p.CaptureUnixNanos - e.FirstUnixNanos, Value: p.Value, Sigma: p.Sigma})
	}
	for _, p := range e.NetTimeGapSeries {
		s.NetTimeGap = append(s.NetTimeGap, SeriesPoint{OffsetNanos: p.CaptureUnixNanos - e.FirstUnixNanos, Value: p.Value, Sigma: p.Sigma})
	}
	for _, p := range e.PredictedGapSeries {
		s.Predicted = append(s.Predicted, PredictedPoint{
			OffsetNanos: p.CaptureUnixNanos - e.FirstUnixNanos, ValueM: p.ValueM, SigmaM: p.SigmaM, CoastAgeNanos: p.CoastAgeNanos,
		})
	}
	return s
}
