package perframeeval

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// gapInstant is one synthetic sample of a reviewed leader record: a scored
// gap with error errM against a reference of the given half-width, whose
// leader choice had outcome lc.
func gapInstant(follower, leader string, sample int, errM, halfWidth, sigma float64, lc string) PhysicalFollowingInstant {
	value := 5.0
	lo, hi := value-halfWidth, value+halfWidth
	e := errM
	outside := 0.0
	return PhysicalFollowingInstant{
		EpisodeID: "ep", FollowingID: "follow-" + follower, FollowerObjectID: follower, LeaderObjectID: leader,
		Decision: annotation.FollowingLeader, SampleID: sample,
		ReferenceGap: &annotation.FollowingGap{SampleID: sample, LowerM: &lo, UpperM: &hi, ValueM: &value},
		PredictedGap: &PredictedGap{ValueM: value + errM, SigmaM: sigma},
		ErrorM:       &e, OutsideBoundM: &outside,
		Outcome:      Outcome{Category: OutcomeScored},
		LeaderChoice: &LeaderChoice{FollowerTrack: "seq-" + follower, Answer: AnswerLeader, Outcome: lc},
	}
}

// acceptanceRef is a physical reference carrying only what acceptance reads:
// the classes and the criteria.
func acceptanceRef(c *FollowingCriteria) *PhysicalReference {
	pr := &PhysicalReference{labels: map[string]string{}, criteria: c}
	if c != nil {
		pr.Identity.FollowingCriteria = &FollowingCriteriaCitation{Path: "criteria.json", RecordID: c.RecordID,
			Digest: "sha256:test", Status: c.Status, Decision: c.Decision, Unset: c.Missing(false)}
	}
	return pr
}

// pairs is n body pairs, each one follower behind its own leader for cases
// samples, every gap with the same error.
func pairs(n, cases int, errM float64, lc string) []PhysicalFollowingInstant {
	var out []PhysicalFollowingInstant
	for p := 0; p < n; p++ {
		for s := 0; s < cases; s++ {
			out = append(out, gapInstant(fmt.Sprintf("f%02d", p), fmt.Sprintf("l%02d", p), s, errM, 0.1, 0.2, lc))
		}
	}
	return out
}

func stratum(t *testing.T, fa *FollowingAcceptance, kind string) FollowingStratumScore {
	t.Helper()
	for _, s := range fa.Strata {
		if s.Kind == kind {
			return s
		}
	}
	t.Fatalf("no %s stratum in %+v", kind, fa.Strata)
	return FollowingStratumScore{}
}

func test(t *testing.T, s FollowingStratumScore, name string) IntervalTest {
	t.Helper()
	for _, x := range s.Tests {
		if x.Test == name {
			return x
		}
	}
	t.Fatalf("no test %s in %+v", name, s.Tests)
	return IntervalTest{}
}

// Strata count reviewed bodies and body pairs, never tracks: a follower
// fragmented over two tracks is still one body pair, two followers behind one
// leader are three bodies and two pairs, and an instant two episodes share is
// one case.
func TestFollowingAcceptanceCountsBodiesNotTracks(t *testing.T) {
	c := pinnedTestCriteria()
	var following []PhysicalFollowingInstant
	for s := 0; s < 5; s++ {
		f := gapInstant("obj_a", "obj_lead", s, 0.1, 0.1, 0.2, LeaderRight)
		if s >= 3 {
			f.LeaderChoice.FollowerTrack = "seq-000009"
		}
		following = append(following, f)
	}
	following = append(following, gapInstant("obj_b", "obj_lead", 0, 0.1, 0.1, 0.2, LeaderRight))
	overlap := gapInstant("obj_a", "obj_lead", 2, 0.1, 0.1, 0.2, LeaderRight)
	overlap.EpisodeID = "ep-overlapping"
	following = append(following, overlap)

	fa := scoreFollowingAcceptance(acceptanceRef(&c), following, LeaderChoiceSummary{})
	gap := stratum(t, fa, StratumGap)
	if gap.Bodies != 3 || gap.BodyPairs != 2 || gap.Units != 2 || gap.Cases != 6 {
		t.Fatalf("gap stratum bodies %d, pairs %d, units %d, cases %d", gap.Bodies, gap.BodyPairs, gap.Units, gap.Cases)
	}
	// Five cases on one pair and one on the other are 36/26 effective units.
	if math.Abs(gap.EffectiveUnits-36.0/26.0) > 1e-12 {
		t.Fatalf("effective units %v", gap.EffectiveUnits)
	}
	if fa.Leader.Counts[annotation.FollowingLeader][LeaderRight] != 6 {
		t.Fatalf("leader counts %+v", fa.Leader.Counts)
	}
	lc := stratum(t, fa, StratumLeaderChoice)
	if lc.Decision != annotation.FollowingLeader || lc.BodyPairs != 2 || lc.FollowerClass != "unlabelled" {
		t.Fatalf("leader-choice stratum %+v", lc)
	}
}

// A no_leader record's unit is the follower body, with no pair.
func TestFollowingAcceptanceNoLeaderUnitsAreBodies(t *testing.T) {
	c := pinnedTestCriteria()
	var following []PhysicalFollowingInstant
	for s := 0; s < 3; s++ {
		following = append(following, PhysicalFollowingInstant{
			EpisodeID: "ep", FollowerObjectID: "obj_lead", Decision: annotation.FollowingNoLeader, SampleID: s,
			Outcome: Outcome{Category: OutcomeNotFollowing, Reason: string(annotation.FollowingNoLeader)},
			LeaderChoice: &LeaderChoice{Answer: AnswerLeader, LeaderTrack: "seq-000004", Outcome: LeaderWrong,
				Reason: ReasonRecordNamesNoLeader},
		})
	}
	fa := scoreFollowingAcceptance(acceptanceRef(&c), following, LeaderChoiceSummary{})
	if len(fa.Strata) != 1 {
		t.Fatalf("a no_leader record made a gap stratum: %+v", fa.Strata)
	}
	s := fa.Strata[0]
	if s.Decision != annotation.FollowingNoLeader || s.Bodies != 1 || s.BodyPairs != 0 || s.Units != 1 {
		t.Fatalf("no_leader stratum %+v", s)
	}
	if w := test(t, s, TestWrongLeader); w.Point == nil || *w.Point != 1 {
		t.Fatalf("wrong-leader share %+v", w)
	}
}

func TestFollowingAcceptanceVerdicts(t *testing.T) {
	pinned := pinnedTestCriteria()

	// Enough independent pairs, each gap well inside the limit and its
	// interval: every test passes.
	fa := scoreFollowingAcceptance(acceptanceRef(&pinned), pairs(12, 3, 0.1, LeaderRight), LeaderChoiceSummary{})
	if fa.Verdict != FollowingPass || fa.Bootstrap.Source != "criteria_record" || fa.Bootstrap.Draws != 400 ||
		fa.Bootstrap.Tests != 5 || math.Abs(fa.Bootstrap.IntervalLevel-0.99) > 1e-12 {
		t.Fatalf("passing set: %s, bootstrap %+v, strata %+v", fa.Verdict, fa.Bootstrap, fa.Strata)
	}
	for _, s := range fa.Strata {
		for _, x := range s.Tests {
			if x.Verdict != FollowingPass {
				t.Fatalf("%s %s: %+v", s.Kind, x.Test, x)
			}
		}
	}

	// The same pairs with every gap 2 m out fail the error limit and the
	// coverage range.
	fa = scoreFollowingAcceptance(acceptanceRef(&pinned), pairs(12, 3, 2.0, LeaderRight), LeaderChoiceSummary{})
	gap := stratum(t, fa, StratumGap)
	if fa.Verdict != FollowingFail || gap.Verdict != FollowingFail ||
		!reflect.DeepEqual(gap.Failed, []string{TestGapErrorP95, TestCoverage}) {
		t.Fatalf("failing set: %s, gap %+v", fa.Verdict, gap)
	}

	// Two pairs are fewer than the floor of three: insufficient, while the
	// failures the intervals show are still listed.
	fa = scoreFollowingAcceptance(acceptanceRef(&pinned), pairs(2, 3, 2.0, LeaderRight), LeaderChoiceSummary{})
	gap = stratum(t, fa, StratumGap)
	if gap.Verdict != FollowingInsufficient || !reflect.DeepEqual(gap.Insufficient, []string{InsufficientEncounters}) ||
		len(gap.Failed) == 0 || fa.Verdict != FollowingInsufficient {
		t.Fatalf("too few pairs: %s, gap %+v", fa.Verdict, gap)
	}

	// One bad pair among twelve: the point estimate breaks the limit, but
	// the interval straddles it, so the test is insufficient, not a fail.
	mixed := pairs(11, 1, 0.1, LeaderRight)
	mixed = append(mixed, gapInstant("bad", "bad-lead", 0, 0.9, 0.1, 2.0, LeaderRight))
	fa = scoreFollowingAcceptance(acceptanceRef(&pinned), mixed, LeaderChoiceSummary{})
	p95 := test(t, stratum(t, fa, StratumGap), TestGapErrorP95)
	if p95.Verdict != FollowingInsufficient || *p95.Point != 0.9 || *p95.Lower != 0.1 || *p95.Upper != 0.9 {
		t.Fatalf("straddling interval: %+v", p95)
	}

	// Wrong leaders fail the tolerance, and their gaps are not scored even
	// though each would be accurate.
	wrong := pairs(12, 3, 0.1, LeaderWrong)
	fa = scoreFollowingAcceptance(acceptanceRef(&pinned), wrong, LeaderChoiceSummary{})
	lc, gap := stratum(t, fa, StratumLeaderChoice), stratum(t, fa, StratumGap)
	if lc.Verdict != FollowingFail || gap.Outcomes[OpportunityWrongLeader] != 36 || gap.Outcomes[OpportunityScored] != 0 ||
		test(t, gap, TestGapErrorP95).Verdict != FollowingInsufficient {
		t.Fatalf("wrong leaders: leader %+v, gap %+v", lc, gap)
	}

	// References wider than the precision limit never count toward a pass.
	var wide []PhysicalFollowingInstant
	for _, f := range pairs(12, 3, 0.1, LeaderRight) {
		hi := *f.ReferenceGap.ValueM + 1
		f.ReferenceGap.UpperM = &hi
		wide = append(wide, f)
	}
	fa = scoreFollowingAcceptance(acceptanceRef(&pinned), wide, LeaderChoiceSummary{})
	gap = stratum(t, fa, StratumGap)
	if gap.Verdict != FollowingInsufficient || gap.Outcomes[OpportunityImprecise] != 36 || gap.Units != 0 ||
		!reflect.DeepEqual(gap.Insufficient, []string{InsufficientEncounters, InsufficientReferencePrecision}) {
		t.Fatalf("imprecise references: %+v", gap)
	}

	// A leader choice that could not be read leaves every stratum
	// insufficient.
	fa = scoreFollowingAcceptance(acceptanceRef(&pinned), pairs(12, 3, 0.1, LeaderUnscored), LeaderChoiceSummary{Unavailable: "no trajectories"})
	if gap := stratum(t, fa, StratumGap); gap.Verdict != FollowingInsufficient || gap.Insufficient[0] != InsufficientLeaderChoice {
		t.Fatalf("unavailable leader choice: %+v", gap)
	}
}

// With no record, or a draft that leaves values unset, every stratum is
// insufficient, the intervals are still reported, and the bootstrap says
// where its plan came from.
func TestFollowingAcceptanceWithoutCriteria(t *testing.T) {
	following := pairs(12, 3, 0.1, LeaderRight)
	fa := scoreFollowingAcceptance(acceptanceRef(nil), following, LeaderChoiceSummary{})
	if fa.Verdict != FollowingInsufficient || fa.Criteria != nil || fa.Bootstrap.Source != "scorer_fallback" {
		t.Fatalf("no record: %s, %+v", fa.Verdict, fa.Bootstrap)
	}
	for _, s := range fa.Strata {
		if s.Verdict != FollowingInsufficient || s.Insufficient[0] != InsufficientNoCriterion {
			t.Fatalf("stratum without criteria: %+v", s)
		}
		for _, x := range s.Tests {
			if x.Point == nil || x.Lower == nil || x.Upper == nil {
				t.Fatalf("interval not reported without criteria: %+v", x)
			}
		}
	}
	draft := pinnedTestCriteria()
	draft.Status, draft.Decision, draft.PinnedOn = CriteriaDraft, "", ""
	draft.Limits.GapErrorLimitM = nil
	fa = scoreFollowingAcceptance(acceptanceRef(&draft), following, LeaderChoiceSummary{})
	gap := stratum(t, fa, StratumGap)
	if gap.Verdict != FollowingInsufficient || gap.Insufficient[0] != InsufficientNoCriterion ||
		test(t, gap, TestGapErrorP95).Limit != nil {
		t.Fatalf("draft with the gap limit unset: %+v", gap)
	}
}

// The bootstrap is seeded: the same plan gives the same intervals, and the
// seed is what moves them.
func TestFollowingAcceptanceIsSeeded(t *testing.T) {
	c := pinnedTestCriteria()
	mixed := pairs(6, 2, 0.1, LeaderRight)
	for i := 0; i < 6; i++ {
		mixed = append(mixed, gapInstant(fmt.Sprintf("g%d", i), fmt.Sprintf("m%d", i), 0, 0.1*float64(i+2), 0.1, 0.2, LeaderRight))
	}
	a := scoreFollowingAcceptance(acceptanceRef(&c), mixed, LeaderChoiceSummary{})
	b := scoreFollowingAcceptance(acceptanceRef(&c), mixed, LeaderChoiceSummary{})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed gave different results")
	}
	seed := int64(8)
	c.Bootstrap.Seed = &seed
	d := scoreFollowingAcceptance(acceptanceRef(&c), mixed, LeaderChoiceSummary{})
	if d.Bootstrap.Seed != 8 {
		t.Fatalf("seed %d", d.Bootstrap.Seed)
	}
}

func TestOpportunityOf(t *testing.T) {
	scored := func(mod func(f *PhysicalFollowingInstant)) PhysicalFollowingInstant {
		f := gapInstant("a", "b", 0, 0.1, 0.1, 0.2, LeaderRight)
		mod(&f)
		return f
	}
	precision := f64(0.3)
	for _, c := range []struct {
		name string
		f    PhysicalFollowingInstant
		want string
	}{
		{"scored", scored(func(*PhysicalFollowingInstant) {}), OpportunityScored},
		{"a gap the behaviour layer would suppress", scored(func(f *PhysicalFollowingInstant) { f.PredictedGap.PredictionReason = "below_speed_floor" }), OpportunitySuppressed},
		{"a wrong leader", scored(func(f *PhysicalFollowingInstant) { f.LeaderChoice.Outcome = LeaderWrong }), OpportunityWrongLeader},
		{"a refused choice", scored(func(f *PhysicalFollowingInstant) { f.LeaderChoice.Outcome = LeaderRefused }), OpportunitySuppressed},
		{"a missed leader", scored(func(f *PhysicalFollowingInstant) { f.LeaderChoice.Outcome = LeaderMissed }), OpportunityMissing},
		{"a follower never evaluated", scored(func(f *PhysicalFollowingInstant) {
			f.LeaderChoice.Outcome, f.LeaderChoice.Reason = LeaderUnscored, ReasonFollowerNotEvaluated
		}), OpportunitySuppressed},
		{"an unmatched leader", scored(func(f *PhysicalFollowingInstant) {
			f.LeaderChoice.Outcome, f.LeaderChoice.Reason = LeaderUnscored, ReasonLeaderUnidentifiable
			f.Outcome = Outcome{Category: OutcomeUnmatched, Reason: ReasonLeaderUnmatched}
		}), OpportunityMissing},
		{"no prediction at the instant", scored(func(f *PhysicalFollowingInstant) {
			f.Outcome = Outcome{Category: OutcomeMissingPrediction, Reason: ReasonNoPredictionAtInstant}
		}), OpportunityMissing},
		{"an unprojectable follower", scored(func(f *PhysicalFollowingInstant) {
			f.Outcome = Outcome{Category: OutcomeMissingPrediction, Reason: "follower_unresolved_orientation"}
		}), OpportunitySuppressed},
		{"an imprecise reference", scored(func(f *PhysicalFollowingInstant) { hi := 6.0; f.ReferenceGap.UpperM = &hi }), OpportunityImprecise},
		{"unknown geometry", scored(func(f *PhysicalFollowingInstant) {
			f.Outcome = Outcome{Category: OutcomeUnknownGeometry, Reason: ReasonNoGapReference}
		}), ""},
		{"a no_leader record", scored(func(f *PhysicalFollowingInstant) { f.Decision = annotation.FollowingNoLeader }), ""},
		{"an unreviewed record", scored(func(f *PhysicalFollowingInstant) { f.LeaderChoice = nil }), ""},
	} {
		if got := opportunityOf(c.f, precision); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
