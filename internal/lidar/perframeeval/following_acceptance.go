package perframeeval

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/stats"
)

// Following acceptance, method following_acceptance_v1 (experiment l8fol0004,
// review R6 of docs/lidar/operations/0.5.2-sprint-review.md).
//
//   - Opportunities. Each sample of a reviewed, independent leader record
//     whose gap reference is usable (the gap's instant is not unknown
//     geometry) is one opportunity, once per follower and sample however many
//     episodes hold it. A reference whose half-width exceeds the record's
//     reference precision is imprecise: counted, never scored toward a pass.
//     Otherwise the opportunity is a wrong leader (the estimator chose another
//     body), suppressed (a refused leader choice, a follower the estimator
//     never evaluated, a body it could not project, or a gap the behaviour
//     layer would not publish), missing (no match, no prediction, free flow)
//     or scored.
//   - Strata. A gap stratum is the follower's and leader's reference class;
//     its unit is the body pair, by reviewed object id. A leader-choice
//     stratum is the record's decision and the follower's class; its unit is
//     the body pair for a leader record and the follower body for a no_leader
//     or ambiguous one. Each stratum reports its bodies, body pairs, units and
//     the Kish effective count of its units weighted by their cases, which
//     is the count the encounter floor is read against: one long encounter
//     beside a short one is nearer one encounter than two.
//   - Bootstrap. Each stratum's units are drawn with replacement, seeded per
//     stratum from the plan's seed and the stratum's key, and each test's
//     statistic is recomputed on every draw. A test's interval is the
//     percentile interval at 1 - alpha/m, alpha one less the family
//     confidence and m the number of tests across the arm's strata
//     (Bonferroni).
//   - Tests. A gap stratum tests the 95th percentile of |gap error| (nearest
//     rank) against the gap error limit, empirical coverage against the
//     coverage range, and the missing and suppressed shares of its
//     opportunities against their limits. A leader-choice stratum tests the
//     wrong-leader share of its scored instants against the tolerance. An
//     upper-bounded test passes when its interval's upper end is within the
//     limit and fails when its lower end is beyond it; a range test passes
//     inside the range and fails wholly outside it; otherwise, or when the
//     statistic is undefined on some draw, or the limit is unset, it is
//     insufficient.
//   - Verdicts. A stratum is insufficient when a limit it needs is unset,
//     its effective count is below the floor, its leader choice could not be
//     read, or none of its opportunities had a precise reference; it still
//     lists the tests it fails. Otherwise it fails if a test fails, is
//     insufficient if a test is, and passes. The arm fails if a stratum
//     fails, is insufficient if a stratum is or there is none, and passes.

// FollowingAcceptanceMethodID versions opportunities, strata, bootstrap and
// verdicts.
const FollowingAcceptanceMethodID = "following_acceptance_v1"

// Opportunity outcomes.
const (
	OpportunityScored      = "scored"
	OpportunityMissing     = "missing"
	OpportunitySuppressed  = "suppressed"
	OpportunityWrongLeader = "wrong_leader"
	OpportunityImprecise   = "imprecise_reference"
)

// Stratum kinds.
const (
	StratumGap          = "gap"
	StratumLeaderChoice = "leader_choice"
)

// Tests.
const (
	TestGapErrorP95   = "p95_abs_gap_error_m"
	TestCoverage      = "coverage"
	TestMissing       = "missing_share"
	TestSuppressed    = "suppressed_share"
	TestWrongLeader   = "wrong_leader_share"
	gapErrorQuantile  = 0.95
	fallbackDraws     = 1000
	fallbackSeed      = 1
	fallbackFamilyCov = 0.95
)

// Verdicts.
const (
	FollowingPass         = "pass"
	FollowingFail         = "fail"
	FollowingInsufficient = "insufficient"
)

// Why a stratum is insufficient whatever its tests say.
const (
	InsufficientNoCriterion        = "no_criterion"
	InsufficientEncounters         = "too_few_independent_encounters"
	InsufficientLeaderChoice       = "leader_choice_unavailable"
	InsufficientReferencePrecision = "insufficient_reference_precision"
)

// FollowingStratum is one cell of the following stratification.
type FollowingStratum struct {
	Kind          string `json:"kind"`
	FollowerClass string `json:"follower_class"`
	// LeaderClass is a gap stratum's; Decision a leader-choice stratum's.
	LeaderClass string                       `json:"leader_class,omitempty"`
	Decision    annotation.FollowingDecision `json:"decision,omitempty"`
}

func (s FollowingStratum) key() string {
	return s.Kind + "|" + s.FollowerClass + "|" + s.LeaderClass + "|" + string(s.Decision)
}

// IntervalTest is one test's point estimate, bootstrap interval and verdict.
type IntervalTest struct {
	Test string `json:"test"`
	// Point is the statistic on every unit once; Lower and Upper its
	// interval. Each is absent where the statistic had no cases.
	Point *float64 `json:"point,omitempty"`
	Lower *float64 `json:"lower,omitempty"`
	Upper *float64 `json:"upper,omitempty"`
	// Limit is an upper-bounded test's; LimitLower and LimitUpper a range
	// test's. Absent when the record leaves them unset.
	Limit      *float64 `json:"limit,omitempty"`
	LimitLower *float64 `json:"limit_lower,omitempty"`
	LimitUpper *float64 `json:"limit_upper,omitempty"`
	// UndefinedDraws counts draws on which the statistic had no cases.
	UndefinedDraws int    `json:"undefined_draws,omitempty"`
	Verdict        string `json:"verdict"`
}

// FollowingStratumScore is one stratum's counts, tests and verdict.
type FollowingStratumScore struct {
	FollowingStratum
	// Bodies and BodyPairs count reviewed object ids, never track ids.
	Bodies    int `json:"bodies"`
	BodyPairs int `json:"body_pairs"`
	// Units are what the bootstrap draws; EffectiveUnits their Kish count.
	Units          int     `json:"units"`
	EffectiveUnits float64 `json:"effective_units"`
	Cases          int     `json:"cases"`
	// Outcomes counts opportunities (gap) or leader-choice outcomes.
	Outcomes     map[string]int `json:"outcomes"`
	Tests        []IntervalTest `json:"tests"`
	Insufficient []string       `json:"insufficient,omitempty"`
	Failed       []string       `json:"failed,omitempty"`
	Verdict      string         `json:"verdict"`
}

// FollowingBootstrapUsed is the bootstrap as run, and where its plan came
// from: the record, or the scorer's fallback where the record leaves it
// unset.
type FollowingBootstrapUsed struct {
	Draws            int     `json:"draws"`
	Seed             int64   `json:"seed"`
	FamilyConfidence float64 `json:"family_confidence"`
	Correction       string  `json:"correction"`
	// Tests is m, and IntervalLevel each interval's two-sided level.
	Tests         int     `json:"tests"`
	IntervalLevel float64 `json:"interval_level"`
	Source        string  `json:"source"`
}

// FollowingAcceptance is one arm's following acceptance.
type FollowingAcceptance struct {
	Method   string                     `json:"method"`
	Criteria *FollowingCriteriaCitation `json:"criteria,omitempty"`
	// Leader is the leader choice's identity and accounting.
	Leader    LeaderChoiceSummary     `json:"leader_choice"`
	Bootstrap FollowingBootstrapUsed  `json:"bootstrap"`
	Strata    []FollowingStratumScore `json:"strata"`
	Verdict   string                  `json:"verdict"`
}

// followingCase is one opportunity or leader-choice instant of a stratum.
type followingCase struct {
	outcome string
	// errM and covered are a scored gap's.
	errM    float64
	covered bool
}

type stratumUnits struct {
	stratum FollowingStratum
	units   map[string][]followingCase
	bodies  map[string]bool
	pairs   map[[2]string]bool
	counts  map[string]int
}

// referenceHalfWidth is a reference gap's half-width: the farther distance
// from its quoted value to either end, as the summary reports it.
func referenceHalfWidth(g annotation.FollowingGap) float64 {
	v := referenceGapValue(g)
	return math.Max(v-*g.LowerM, *g.UpperM-v)
}

// opportunityOf classifies one following instant as an opportunity, or
// returns "" when it is not one.
func opportunityOf(f PhysicalFollowingInstant, precisionM *float64) string {
	if f.Decision != annotation.FollowingLeader || f.LeaderChoice == nil ||
		f.Outcome.Category == OutcomeUnknownGeometry || f.Outcome.Category == OutcomeNotFollowing {
		return ""
	}
	if precisionM != nil && f.ReferenceGap != nil && referenceHalfWidth(*f.ReferenceGap) > *precisionM {
		return OpportunityImprecise
	}
	switch lc := f.LeaderChoice; {
	case lc.Outcome == LeaderWrong:
		return OpportunityWrongLeader
	case lc.Outcome == LeaderRefused:
		return OpportunitySuppressed
	case lc.Outcome == LeaderMissed:
		return OpportunityMissing
	case lc.Outcome == LeaderUnscored && lc.Reason == ReasonFollowerNotEvaluated:
		return OpportunitySuppressed
	}
	switch f.Outcome.Category {
	case OutcomeScored:
		if f.PredictedGap != nil && f.PredictedGap.PredictionReason != "" {
			return OpportunitySuppressed
		}
		return OpportunityScored
	case OutcomeMissingPrediction:
		if f.Outcome.Reason == ReasonNoPredictionAtInstant || f.Outcome.Reason == ReasonPredictionInstantsDiffer {
			return OpportunityMissing
		}
		return OpportunitySuppressed
	}
	return OpportunityMissing
}

// className is a body's reference class, or "unlabelled".
func className(labels map[string]string, obj string) string {
	if c := labels[obj]; c != "" {
		return c
	}
	return "unlabelled"
}

// scoreFollowingAcceptance builds one arm's strata from its scored following
// instants and reads each against the criteria, if any.
func scoreFollowingAcceptance(pr *PhysicalReference, following []PhysicalFollowingInstant, leader LeaderChoiceSummary) *FollowingAcceptance {
	crit := pr.criteria
	acc := &FollowingAcceptance{Method: FollowingAcceptanceMethodID, Criteria: pr.Identity.FollowingCriteria, Leader: leader}
	var limits FollowingLimits
	if crit != nil {
		limits = crit.Limits
	}
	acc.Leader.Counts = map[annotation.FollowingDecision]map[string]int{}

	strata := map[string]*stratumUnits{}
	add := func(s FollowingStratum, unit string, pair [2]string, c followingCase) {
		su := strata[s.key()]
		if su == nil {
			su = &stratumUnits{stratum: s, units: map[string][]followingCase{}, bodies: map[string]bool{},
				pairs: map[[2]string]bool{}, counts: map[string]int{}}
			strata[s.key()] = su
		}
		su.counts[c.outcome]++
		if c.outcome == OpportunityImprecise || c.outcome == LeaderUnscored {
			return
		}
		su.units[unit] = append(su.units[unit], c)
		for _, b := range pair {
			if b != "" {
				su.bodies[b] = true
			}
		}
		if pair[1] != "" {
			su.pairs[pair] = true
		}
	}
	z := math.NaN()
	if limits.NominalCoverage != nil {
		z = math.Sqrt2 * math.Erfinv(*limits.NominalCoverage)
	}
	seen := map[string]bool{}
	for _, f := range following {
		if f.LeaderChoice == nil {
			continue
		}
		// One follower at one sample is one instant, whichever episodes
		// hold it.
		k := fmt.Sprintf("%s@%d", f.FollowerObjectID, f.SampleID)
		if seen[k] {
			continue
		}
		seen[k] = true
		if acc.Leader.Counts[f.Decision] == nil {
			acc.Leader.Counts[f.Decision] = map[string]int{}
		}
		acc.Leader.Counts[f.Decision][f.LeaderChoice.Outcome]++

		fc := className(pr.labels, f.FollowerObjectID)
		pair := [2]string{f.FollowerObjectID, f.LeaderObjectID}
		unit := f.FollowerObjectID + "|" + f.LeaderObjectID
		if f.Decision != annotation.FollowingLeader {
			unit = f.FollowerObjectID + "|" + string(f.Decision)
		}
		add(FollowingStratum{Kind: StratumLeaderChoice, FollowerClass: fc, Decision: f.Decision}, unit, pair,
			followingCase{outcome: f.LeaderChoice.Outcome})

		op := opportunityOf(f, limits.ReferencePrecisionM)
		if op == "" {
			continue
		}
		c := followingCase{outcome: op}
		if op == OpportunityScored {
			c.errM = *f.ErrorM
			c.covered = !math.IsNaN(z) && math.Abs(*f.ErrorM) <= z*f.PredictedGap.SigmaM
		}
		add(FollowingStratum{Kind: StratumGap, FollowerClass: fc, LeaderClass: className(pr.labels, f.LeaderObjectID)}, unit, pair, c)
	}

	keys := make([]string, 0, len(strata))
	for k := range strata {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	plan := bootstrapPlan(crit)
	for _, k := range keys {
		if strata[k].stratum.Kind == StratumGap {
			plan.Tests += 4
		} else {
			plan.Tests++
		}
	}
	plan.IntervalLevel = 1
	if plan.Tests > 0 {
		plan.IntervalLevel = 1 - (1-plan.FamilyConfidence)/float64(plan.Tests)
	}
	acc.Bootstrap = plan

	anyFail, anyInsufficient := false, len(keys) == 0
	for _, k := range keys {
		score := scoreFollowingStratum(strata[k], limits, plan, leader.Unavailable != "")
		if crit == nil || len(crit.Missing(false)) > 0 {
			score.Insufficient = append([]string{InsufficientNoCriterion}, score.Insufficient...)
			score.Verdict = FollowingInsufficient
		}
		anyFail = anyFail || score.Verdict == FollowingFail
		anyInsufficient = anyInsufficient || score.Verdict == FollowingInsufficient
		acc.Strata = append(acc.Strata, score)
	}
	switch {
	case anyFail:
		acc.Verdict = FollowingFail
	case anyInsufficient:
		acc.Verdict = FollowingInsufficient
	default:
		acc.Verdict = FollowingPass
	}
	return acc
}

// bootstrapPlan is the record's bootstrap, with the scorer's fallback for
// what it leaves unset. A fallback can only shape a preview: a record that
// leaves any of it unset judges no stratum.
func bootstrapPlan(crit *FollowingCriteria) FollowingBootstrapUsed {
	p := FollowingBootstrapUsed{Draws: fallbackDraws, Seed: fallbackSeed, FamilyConfidence: fallbackFamilyCov,
		Correction: CorrectionBonferroni, Source: "scorer_fallback"}
	if crit == nil {
		return p
	}
	b := crit.Bootstrap
	if b.Draws != nil && b.Seed != nil && b.FamilyConfidence != nil && b.Correction != "" {
		p.Source = "criteria_record"
	}
	if b.Draws != nil {
		p.Draws = *b.Draws
	}
	if b.Seed != nil {
		p.Seed = *b.Seed
	}
	if b.FamilyConfidence != nil {
		p.FamilyConfidence = *b.FamilyConfidence
	}
	return p
}

// statistic is one test's statistic over a draw's cases, and whether it was
// defined.
type statistic func(cases []followingCase) (float64, bool)

func gapErrorP95(cases []followingCase) (float64, bool) {
	var abs []float64
	for _, c := range cases {
		if c.outcome == OpportunityScored {
			abs = append(abs, math.Abs(c.errM))
		}
	}
	if len(abs) == 0 {
		return 0, false
	}
	sort.Float64s(abs)
	return stats.NearestRank(abs, gapErrorQuantile), true
}

func coverageOf(cases []followingCase) (float64, bool) {
	n, covered := 0, 0
	for _, c := range cases {
		if c.outcome == OpportunityScored {
			n++
			if c.covered {
				covered++
			}
		}
	}
	if n == 0 {
		return 0, false
	}
	return float64(covered) / float64(n), true
}

// shareOf is the share of cases with the outcome, over all cases or only
// those whose outcome is in among.
func shareOf(outcome string, among ...string) statistic {
	return func(cases []followingCase) (float64, bool) {
		n, hit := 0, 0
		for _, c := range cases {
			in := len(among) == 0
			for _, a := range among {
				in = in || c.outcome == a
			}
			if !in {
				continue
			}
			n++
			if c.outcome == outcome {
				hit++
			}
		}
		if n == 0 {
			return 0, false
		}
		return float64(hit) / float64(n), true
	}
}

// scoreFollowingStratum counts a stratum's bodies and units, bootstraps each
// of its tests and reads its verdict.
func scoreFollowingStratum(su *stratumUnits, limits FollowingLimits, plan FollowingBootstrapUsed, leaderUnavailable bool) FollowingStratumScore {
	score := FollowingStratumScore{FollowingStratum: su.stratum, Bodies: len(su.bodies), BodyPairs: len(su.pairs),
		Units: len(su.units), Outcomes: su.counts}
	names := make([]string, 0, len(su.units))
	var sum, sumSq float64
	for name, cases := range su.units {
		names = append(names, name)
		w := float64(len(cases))
		sum, sumSq = sum+w, sumSq+w*w
		score.Cases += len(cases)
	}
	sort.Strings(names)
	if sumSq > 0 {
		score.EffectiveUnits = sum * sum / sumSq
	}

	type spec struct {
		test     string
		stat     statistic
		upper    *float64
		lo, hi   *float64
		isRange  bool
		disabled bool
	}
	var specs []spec
	if su.stratum.Kind == StratumGap {
		specs = []spec{
			{test: TestGapErrorP95, stat: gapErrorP95, upper: limits.GapErrorLimitM},
			{test: TestCoverage, stat: coverageOf, lo: limits.MinCoverage, hi: limits.MaxCoverage, isRange: true,
				disabled: limits.NominalCoverage == nil},
			{test: TestMissing, stat: shareOf(OpportunityMissing), upper: limits.MaxMissingOpportunity},
			{test: TestSuppressed, stat: shareOf(OpportunitySuppressed), upper: limits.MaxSuppressedOpportunity},
		}
	} else {
		specs = []spec{{test: TestWrongLeader, stat: shareOf(LeaderWrong, LeaderRight, LeaderWrong, LeaderMissed, LeaderRefused),
			upper: limits.WrongLeaderTolerance}}
	}

	// Every test of a stratum reads the same draws.
	h := fnv.New64a()
	_, _ = h.Write([]byte(su.stratum.key()))
	rng := rand.New(rand.NewSource(plan.Seed ^ int64(h.Sum64())))
	draws := make([][]followingCase, 0, plan.Draws)
	if len(names) > 0 {
		for d := 0; d < plan.Draws; d++ {
			var drawn []followingCase
			for range names {
				drawn = append(drawn, su.units[names[rng.Intn(len(names))]]...)
			}
			draws = append(draws, drawn)
		}
	}
	var all []followingCase
	for _, n := range names {
		all = append(all, su.units[n]...)
	}

	alpha := 1 - plan.IntervalLevel
	for _, sp := range specs {
		t := IntervalTest{Test: sp.test, Limit: sp.upper, LimitLower: sp.lo, LimitUpper: sp.hi, Verdict: FollowingInsufficient}
		point, defined := sp.stat(all)
		if defined {
			t.Point = &point
		}
		var values []float64
		for _, d := range draws {
			if v, ok := sp.stat(d); ok {
				values = append(values, v)
			} else {
				t.UndefinedDraws++
			}
		}
		var lower, upper float64
		if len(values) > 0 {
			sort.Float64s(values)
			lower, upper = stats.NearestRank(values, alpha/2), stats.NearestRank(values, 1-alpha/2)
			t.Lower, t.Upper = &lower, &upper
		}
		judged := defined && len(values) > 0 && t.UndefinedDraws == 0 && !sp.disabled
		switch {
		case !judged:
		case sp.isRange && sp.lo != nil && sp.hi != nil:
			switch {
			case upper < *sp.lo || lower > *sp.hi:
				t.Verdict = FollowingFail
			case lower >= *sp.lo && upper <= *sp.hi:
				t.Verdict = FollowingPass
			}
		case !sp.isRange && sp.upper != nil:
			switch {
			case lower > *sp.upper:
				t.Verdict = FollowingFail
			case upper <= *sp.upper:
				t.Verdict = FollowingPass
			}
		}
		if t.Verdict == FollowingFail {
			score.Failed = append(score.Failed, sp.test)
		}
		score.Tests = append(score.Tests, t)
	}

	if limits.MinIndependentEncounters != nil && score.EffectiveUnits < float64(*limits.MinIndependentEncounters) {
		score.Insufficient = append(score.Insufficient, InsufficientEncounters)
	}
	if leaderUnavailable {
		score.Insufficient = append(score.Insufficient, InsufficientLeaderChoice)
	}
	if su.stratum.Kind == StratumGap && score.Units == 0 && su.counts[OpportunityImprecise] > 0 {
		score.Insufficient = append(score.Insufficient, InsufficientReferencePrecision)
	}
	anyInsufficient := false
	for _, t := range score.Tests {
		anyInsufficient = anyInsufficient || t.Verdict == FollowingInsufficient
	}
	switch {
	case len(score.Insufficient) > 0:
		score.Verdict = FollowingInsufficient
	case len(score.Failed) > 0:
		score.Verdict = FollowingFail
	case anyInsufficient:
		score.Verdict = FollowingInsufficient
	default:
		score.Verdict = FollowingPass
	}
	return score
}
