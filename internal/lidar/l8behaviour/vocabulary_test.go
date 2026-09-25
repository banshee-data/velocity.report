package l8behaviour

import (
	"encoding"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// vocabularyCase exercises one closed vocabulary through its public surface
// only, so every type's one-line methods are held to the same contract.
type vocabularyCase struct {
	kind   string
	count  int // the unexported end sentinel
	tokens func() []string
	parse  func(string) (fmt.Stringer, error)
	zero   interface {
		fmt.Stringer
		encoding.TextMarshaler
	}
	decode func([]byte) (fmt.Stringer, error)
	valid  func(int) bool
}

func tokensOf[T fmt.Stringer](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = v.String()
	}
	return out
}

func decoder[T any, P interface {
	*T
	encoding.TextUnmarshaler
	fmt.Stringer
}]() func([]byte) (fmt.Stringer, error) {
	return func(b []byte) (fmt.Stringer, error) {
		var v T
		err := P(&v).UnmarshalText(b)
		return P(&v), err
	}
}

func parser[T fmt.Stringer](fn func(string) (T, error)) func(string) (fmt.Stringer, error) {
	return func(s string) (fmt.Stringer, error) { return fn(s) }
}

func vocabularyCases() []vocabularyCase {
	return []vocabularyCase{
		{"suppression reason", int(suppressionReasonEnd), func() []string { return tokensOf(SuppressionReasons()) },
			parser(ParseSuppressionReason), ReasonUnspecified, decoder[SuppressionReason](),
			func(i int) bool { return SuppressionReason(i).Valid() }},
		{"support state", int(supportStateEnd), func() []string { return tokensOf(SupportStates()) },
			parser(ParseSupportState), SupportUnspecified, decoder[SupportState](),
			func(i int) bool { return SupportState(i).Valid() }},
		{"estimate stage", int(estimateStageEnd), func() []string { return tokensOf(EstimateStages()) },
			parser(ParseEstimateStage), StageUnspecified, decoder[EstimateStage](),
			func(i int) bool { return EstimateStage(i).Valid() }},
		{"estimation state", int(estimationStateEnd), func() []string { return tokensOf(EstimationStates()) },
			parser(ParseEstimationState), EstimationUnspecified, decoder[EstimationState](),
			func(i int) bool { return EstimationState(i).Valid() }},
		{"endpoint source", int(endpointSourceEnd), func() []string { return tokensOf(EndpointSources()) },
			parser(ParseEndpointSource), EndpointSourceUnspecified, decoder[EndpointSource](),
			func(i int) bool { return EndpointSource(i).Valid() }},
		{"motion class", int(motionClassEnd), func() []string { return tokensOf(MotionClasses()) },
			parser(ParseMotionClass), MotionClassUnspecified, decoder[MotionClass](),
			func(i int) bool { return MotionClass(i).Valid() }},
		{"reference point", int(referencePointEnd), func() []string { return tokensOf(ReferencePoints()) },
			parser(ParseReferencePoint), ReferenceUnspecified, decoder[ReferencePoint](),
			func(i int) bool { return ReferencePoint(i).Valid() }},
		{"belief provenance", int(beliefProvenanceEnd), func() []string { return tokensOf(BeliefProvenances()) },
			parser(ParseBeliefProvenance), ProvenanceUnspecified, decoder[BeliefProvenance](),
			func(i int) bool { return BeliefProvenance(i).Valid() }},
		{"path extremity", int(pathExtremityEnd), func() []string { return tokensOf(PathExtremities()) },
			parser(ParsePathExtremity), ExtremityUnspecified, decoder[PathExtremity](),
			func(i int) bool { return PathExtremity(i).Valid() }},
		{"uncertainty kind", int(uncertaintyKindEnd), func() []string { return tokensOf(UncertaintyKinds()) },
			parser(ParseUncertaintyKind), UncertaintyUnspecified, decoder[UncertaintyKind](),
			func(i int) bool { return UncertaintyKind(i).Valid() }},
		{"propagation method", int(propagationMethodEnd), func() []string { return tokensOf(PropagationMethods()) },
			parser(ParsePropagationMethod), MethodUnspecified, decoder[PropagationMethod](),
			func(i int) bool { return PropagationMethod(i).Valid() }},
		{"benchmark kind", int(benchmarkKindEnd), func() []string { return tokensOf(BenchmarkKinds()) },
			parser(ParseBenchmarkKind), BenchmarkUnspecified, decoder[BenchmarkKind](),
			func(i int) bool { return BenchmarkKind(i).Valid() }},
		{"visibility", int(visibilityEnd), func() []string { return tokensOf(Visibilities()) },
			parser(ParseVisibility), VisibilityUnspecified, decoder[Visibility](),
			func(i int) bool { return Visibility(i).Valid() }},
		{"path condition", int(pathConditionEnd), func() []string { return tokensOf(PathConditions()) },
			parser(ParsePathCondition), PathConditionUnspecified, decoder[PathCondition](),
			func(i int) bool { return PathCondition(i).Valid() }},
		{"candidate disposition", int(candidateDispositionEnd), func() []string { return tokensOf(CandidateDispositions()) },
			parser(ParseCandidateDisposition), DispositionUnspecified, decoder[CandidateDisposition](),
			func(i int) bool { return CandidateDisposition(i).Valid() }},
	}
}

func TestVocabulariesAreClosedAndRoundTrip(t *testing.T) {
	for _, c := range vocabularyCases() {
		t.Run(c.kind, func(t *testing.T) {
			tokens := c.tokens()
			// The names table and the constants agree: one token per value.
			if len(tokens) != c.count-1 {
				t.Fatalf("%d tokens for %d values", len(tokens), c.count-1)
			}
			seen := map[string]bool{}
			for i, tok := range tokens {
				if tok == "" || tok == "unspecified" || seen[tok] || !snakeCase.MatchString(tok) {
					t.Fatalf("token %d %q is empty, reserved, duplicated or not snake_case", i+1, tok)
				}
				seen[tok] = true
				v, err := c.parse(tok)
				if err != nil || v.String() != tok {
					t.Fatalf("parse(%q) = %v, %v", tok, v, err)
				}
				d, err := c.decode([]byte(tok))
				if err != nil || d.String() != tok {
					t.Fatalf("decode(%q) = %v, %v", tok, d, err)
				}
				if !c.valid(i + 1) {
					t.Fatalf("value %d is not valid", i+1)
				}
			}
			if c.valid(0) || c.valid(c.count) {
				t.Fatal("the zero value or the end sentinel is valid")
			}
			// Unspecified refuses to serialise, and nothing unknown parses.
			if c.zero.String() != "unspecified" {
				t.Fatalf("zero value names itself %q", c.zero.String())
			}
			if _, err := c.zero.MarshalText(); err == nil {
				t.Fatal("the unspecified value serialised")
			}
			for _, bad := range []string{"", "unspecified", "Observed", strings.ToUpper(tokens[0])} {
				if _, err := c.parse(bad); err == nil {
					t.Fatalf("parse(%q) succeeded", bad)
				}
				if _, err := c.decode([]byte(bad)); err == nil {
					t.Fatalf("decode(%q) succeeded", bad)
				}
			}
		})
	}
}

func TestOutOfRangeValuesNameThemselvesAndRefuseToSerialise(t *testing.T) {
	r := SuppressionReason(200)
	if r.Valid() || r.String() != "suppression reason(200)" {
		t.Fatalf("out of range reason: valid=%v name=%q", r.Valid(), r.String())
	}
	if _, err := json.Marshal(struct {
		R SuppressionReason `json:"r"`
	}{r}); err == nil {
		t.Fatal("out of range reason serialised")
	}
}

func TestSuppressionReasonsCoverThePlanVocabulary(t *testing.T) {
	// Section 7.2's list, which must survive verbatim, plus the following
	// reasons this package adds from Sections 8.3, 9.1 and 9.2.
	want := []string{
		"class_not_supported", "insufficient_observation", "trajectory_uncertainty_too_high",
		"interaction_type_uncertain", "no_common_path", "road_geometry_unavailable",
		"lane_geometry_unavailable", "metric_not_observable", "model_degraded",
		"extent_not_converged", "planar_fallback_insufficient",
		"not_observed", "ambiguous_leader", "orientation_unresolved", "non_positive_gap",
		"below_speed_floor", "estimate_not_final",
	}
	got := map[string]bool{}
	for _, r := range SuppressionReasons() {
		got[r.String()] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing reason %s", w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d reasons registered, %d expected: a new reason needs a plan citation here", len(got), len(want))
	}
	// The reason set is a bitmask; it must hold every reason.
	if int(suppressionReasonEnd) > 32 {
		t.Fatalf("%d reasons overflow the uint32 reason set", suppressionReasonEnd)
	}
}

func TestReasonPrecedence(t *testing.T) {
	order := SuppressionReasons()
	pos := map[SuppressionReason]int{}
	for i, r := range order {
		pos[r] = i
	}
	// The orderings the evaluator relies on, stated as the plan motivates
	// them: undefined before degraded before unobserved before geometry
	// before value conditions, with publication stage last of all.
	for _, pair := range [][2]SuppressionReason{
		{ReasonClassNotSupported, ReasonModelDegraded},
		{ReasonModelDegraded, ReasonInsufficientObservation},
		{ReasonInsufficientObservation, ReasonNotObserved},
		{ReasonNotObserved, ReasonNoCommonPath},
		{ReasonNoCommonPath, ReasonOrientationUnresolved},
		{ReasonOrientationUnresolved, ReasonExtentNotConverged},
		{ReasonExtentNotConverged, ReasonNonPositiveGap},
		{ReasonNonPositiveGap, ReasonBelowSpeedFloor},
		{ReasonBelowSpeedFloor, ReasonEstimateNotFinal},
	} {
		if pos[pair[0]] >= pos[pair[1]] {
			t.Errorf("%s must precede %s", pair[0], pair[1])
		}
	}
	if order[len(order)-1] != ReasonEstimateNotFinal {
		t.Fatalf("estimate_not_final must be last, got %s", order[len(order)-1])
	}

	var set reasonSet
	set.add(ReasonEstimateNotFinal, ReasonUnspecified, ReasonClassNotSupported, ReasonEstimateNotFinal)
	if got := set.ordered(); len(got) != 2 || got[0] != ReasonClassNotSupported || got[1] != ReasonEstimateNotFinal {
		t.Fatalf("ordered set %v", got)
	}
	if set.firstOutside(map[SuppressionReason]bool{ReasonClassNotSupported: true}) != ReasonEstimateNotFinal {
		t.Fatal("firstOutside skipped the wrong reason")
	}
	if !set.onlyWithin(map[SuppressionReason]bool{ReasonClassNotSupported: true, ReasonEstimateNotFinal: true}) {
		t.Fatal("onlyWithin")
	}
	if (reasonSet(0)).first() != ReasonUnspecified {
		t.Fatal("an empty set has no first reason")
	}
}

func TestSupportStateExposureRules(t *testing.T) {
	for _, s := range SupportStates() {
		if s.CountsTowardExposure() != (s == SupportObserved) {
			t.Errorf("%s counts toward exposure = %v; only observed time does", s, s.CountsTowardExposure())
		}
		if s.IsDefectSignal() != (s == SupportMissedUnknown) {
			t.Errorf("%s defect signal = %v; only an unexplained miss is one", s, s.IsDefectSignal())
		}
	}
}

func TestEndpointSourceOrdering(t *testing.T) {
	for _, c := range []struct{ a, b, want EndpointSource }{
		{EndpointDirectlyObserved, EndpointTemporallyInferred, EndpointTemporallyInferred},
		{EndpointPriorDominated, EndpointDirectlyObserved, EndpointPriorDominated},
		{EndpointTemporallyInferred, EndpointTemporallyInferred, EndpointTemporallyInferred},
	} {
		if got := WeakerEndpointSource(c.a, c.b); got != c.want {
			t.Errorf("weaker(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

func TestSmallVocabularyPredicates(t *testing.T) {
	for _, s := range EstimationStates() {
		want := s == EstimationGeometryConverging || s == EstimationEstablished || s == EstimationTemporarilyDegraded
		if s.AllowsReportedPose() != want {
			t.Errorf("%s allows pose = %v", s, s.AllowsReportedPose())
		}
	}
	for _, r := range ReferencePoints() {
		if r.IsPhysical() == (r == ReferenceClusterMedoid) {
			t.Errorf("%s physical = %v", r, r.IsPhysical())
		}
	}
	for _, p := range BeliefProvenances() {
		if p.IsEvidence() == (p == ProvenanceClassPrior) {
			t.Errorf("%s evidence = %v", p, p.IsEvidence())
		}
	}
	for _, v := range Visibilities() {
		if v.EntersDistribution() != (v == VisibilityPublic) {
			t.Errorf("%s enters distribution = %v", v, v.EntersDistribution())
		}
	}
}
