package headway

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

func TestStatusVocabulary(t *testing.T) {
	if _, err := json.Marshal(StatusUnspecified); err == nil {
		t.Error("an unspecified status must refuse to serialise")
	}
	if _, err := json.Marshal(Status(99)); err == nil {
		t.Error("an out-of-range status must refuse to serialise")
	}
	for _, s := range Statuses() {
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal %s: %v", s, err)
		}
		var back Status
		if err := json.Unmarshal(raw, &back); err != nil || back != s {
			t.Errorf("%s round-trips to %s (%v)", s, back, err)
		}
		if s.Label() == "" || s.Note() == "" || strings.ToUpper(strings.ReplaceAll(s.String(), "_", " ")) != s.Label() {
			t.Errorf("%s has label %q", s, s.Label())
		}
	}
	var s Status
	if err := s.UnmarshalText([]byte("final")); err == nil {
		t.Error("an unregistered token must not parse")
	}
	if StatusUnspecified.Label() != "" || StatusUnspecified.String() != "unspecified" || Status(99).String() != "status(99)" {
		t.Error("invalid statuses must print as diagnostics, never as a label")
	}
}

func TestBuildRefusesPromotedAndUnspecified(t *testing.T) {
	in, err := OracleInput()
	if err != nil {
		t.Fatal(err)
	}
	in.Status = StatusPromoted
	if _, err := Build(in); !errors.Is(err, ErrPromotionGated) {
		t.Errorf("promoted build error = %v, want ErrPromotionGated", err)
	}
	in.Status = StatusUnspecified
	if _, err := Build(in); err == nil {
		t.Error("a build without a status must fail")
	}
}

// TestBuildTiesStatusToSource: analytic fixture trajectories cannot be
// labelled provisional, and a synthetic oracle cannot carry anything else.
func TestBuildTiesStatusToSource(t *testing.T) {
	in, err := OracleInput()
	if err != nil {
		t.Fatal(err)
	}
	in.Status = StatusProvisional
	if _, err := Build(in); err == nil || !strings.Contains(err.Error(), "must be reported as synthetic_oracle") {
		t.Errorf("fixtures labelled provisional: error = %v", err)
	}

	field := fieldInput(t, l8behaviour.StageFixedLag)
	field.Status = StatusSyntheticOracle
	if _, err := Build(field); err == nil || !strings.Contains(err.Error(), "only analytic fixture trajectories") {
		t.Errorf("field data labelled synthetic: error = %v", err)
	}
}

// fieldInput re-identifies the steady approach as a non-fixture estimator
// run at the given stage and analyses it afresh, as the provisional slice
// will feed persisted estimates.
func fieldInput(t *testing.T, stage l8behaviour.EstimateStage) Input {
	t.Helper()
	sc := l8behaviour.ScenarioSteadyApproach()
	id := l8behaviour.EstimateIdentity{EstimatorID: "test_estimator_v1", ObsModelID: "test_obs_v1", ParamHash: "test/params_v1"}
	for i := range sc.Trajectories {
		sc.Trajectories[i].Estimate = id
		for j := range sc.Trajectories[i].Samples {
			sc.Trajectories[i].Samples[j].Stage = stage
		}
	}
	a, err := l8behaviour.AnalyseFollowing(sc.Trajectories, sc.Params)
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	return Input{Status: StatusProvisional, Captures: []CaptureInput{{
		ID: "field_capture", Description: "re-identified steady approach", Source: "test/field_capture",
		Trajectories: sc.Trajectories, Params: sc.Params, Analysis: a,
	}}}
}

// TestProvisionalReportReadsTheProvisionalBlock: over fixed-lag estimates
// every production measurement is suppressed with estimate_not_final, and
// the provisional report shows the review-labelled values instead, saying
// which block it read, and pools them apart from any final values.
func TestProvisionalReportReadsTheProvisionalBlock(t *testing.T) {
	in := fieldInput(t, l8behaviour.StageFixedLag)
	for _, m := range in.Captures[0].Analysis.Encounters[0].Measurements {
		if !m.Suppressed {
			t.Fatalf("precondition: fixed-lag production measurement %s is not suppressed", m.Name)
		}
	}
	r, err := Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.StatusLabel != "PROVISIONAL" {
		t.Errorf("status label = %q", r.StatusLabel)
	}
	e := r.Encounters[0]
	if e.Stage != l8behaviour.StageFixedLag || e.ValueBlock != ValueBlockProvisional {
		t.Errorf("stage %s reads %s", e.Stage, e.ValueBlock)
	}
	wantValue(t, e, l8behaviour.MetricFollowingSpatialGapMin, 7.75)
	wantValue(t, e, l8behaviour.MetricFollowingNetTimeGapP50, 1.3875)
	wantValue(t, e, l8behaviour.MetricFollowingTimeBelow1500ms, 2.8)
	if e.Leader.Locator != "test/field_capture/trk_s_steady_leader" {
		t.Errorf("leader locator = %q", e.Leader.Locator)
	}
	if len(r.Aggregates) != 1 || r.Aggregates[0].Version.EstimateStage != l8behaviour.StageFixedLag ||
		r.Aggregates[0].ValueBlock != ValueBlockProvisional {
		t.Errorf("aggregates = %+v", r.Aggregates)
	}
}

func TestBuildRejectsInconsistentInput(t *testing.T) {
	cases := map[string]func(*Input){
		"no captures":      func(in *Input) { in.Captures = nil },
		"duplicate id":     func(in *Input) { in.Captures[1].ID = in.Captures[0].ID },
		"missing source":   func(in *Input) { in.Captures[0].Source = "" },
		"no trajectories":  func(in *Input) { in.Captures[0].Trajectories = nil },
		"stale parameters": func(in *Input) { in.Captures[0].Params.Exposure.MinOpportunitySeconds = 2 },
		"foreign analysis": func(in *Input) { in.Captures[0].Analysis = in.Captures[1].Analysis },
		"duplicate track": func(in *Input) {
			c := &in.Captures[0]
			c.Trajectories = append(c.Trajectories, c.Trajectories[0])
		},
		"truncated measurements": func(in *Input) {
			e := &in.Captures[0].Analysis.Encounters[0]
			e.Measurements = e.Measurements[:3]
		},
		"reordered measurements": func(in *Input) {
			e := &in.Captures[0].Analysis.Encounters[0]
			ms := append([]l8behaviour.Measurement(nil), e.Measurements...)
			ms[1], ms[2] = ms[2], ms[1]
			e.Measurements = ms
		},
		"wrong geometry": func(in *Input) {
			in.Captures[0].Analysis.Encounters[0].GeometryID = "following_local_path_v1/other"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in, err := OracleInput()
			if err != nil {
				t.Fatal(err)
			}
			mutate(&in)
			if _, err := Build(in); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// TestValidateCatchesTampering: a report edited after building cannot print
// a number where a suppression belongs, a label it has not earned, or a name
// the registry does not know.
func TestValidateCatchesTampering(t *testing.T) {
	firstSuppressed := func(r *Report) *Value {
		for i := range r.Encounters {
			for j := range r.Encounters[i].Measurements {
				if r.Encounters[i].Measurements[j].Suppressed {
					return &r.Encounters[i].Measurements[j]
				}
			}
		}
		t.Fatal("oracle has no suppressed value")
		return nil
	}
	cases := map[string]func(*Report){
		"suppressed prints zero": func(r *Report) { firstSuppressed(r).Display = "0 s" },
		"suppressed gains a value": func(r *Report) {
			zero := 0.0
			firstSuppressed(r).Value = &zero
		},
		"value prints another number": func(r *Report) { r.Encounters[0].Measurements[1].Display = "7.5 m" },
		"unregistered metric": func(r *Report) {
			r.Encounters[0].Measurements[1].Metric = "interaction.following_headway_m"
		},
		"wrong unit":        func(r *Report) { r.Encounters[0].Measurements[1].Unit = "ft" },
		"relabelled status": func(r *Report) { r.StatusLabel = "PROMOTED" },
		"promoted status": func(r *Report) {
			r.Status, r.StatusLabel, r.StatusNote = StatusPromoted, "PROMOTED", StatusPromoted.Note()
		},
		"missing status":     func(r *Report) { r.Status = StatusUnspecified },
		"wrong value block":  func(r *Report) { r.Encounters[0].ValueBlock = ValueBlockProvisional },
		"contract version":   func(r *Report) { r.Contract = "headway_report_v0" },
		"renamed report":     func(r *Report) { r.Title = "Following score" },
		"band rate and zero": func(r *Report) { r.Aggregates[1].Bands[0].RateReason = 0 },
		"histogram leaks": func(r *Report) {
			r.Aggregates[0].Histogram.Excluded = r.Aggregates[0].Histogram.Excluded[1:]
		},
		"predicted relabelled": func(r *Report) {
			for i := range r.Encounters {
				if len(r.Encounters[i].Predicted) > 0 {
					r.Encounters[i].Predicted[0].Visibility = l8behaviour.VisibilityPublic
				}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := oracleReport(t)
			if err := r.Validate(); err != nil {
				t.Fatalf("untouched oracle: %v", err)
			}
			mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestHistogramBinIsHalfOpen(t *testing.T) {
	for v, want := range map[float64]int{
		0: 0, 0.2499: 0, 0.25: 1, 0.775: 3, 0.9999: 3, 1.0: 4, 1.4999999: 5, 1.5: 6, 1.9999: 7, 2.0: 8,
		2.9999: 11, 3.0: 12, 7.5: 12,
	} {
		if got := histogramBin(v); got != want {
			t.Errorf("histogramBin(%v) = %d, want %d", v, got, want)
		}
	}
}

func TestFormatting(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{formatWithUnit(13.875, "m"), "13.875 m"},
		{formatWithUnit(1.3875, "s"), "1.3875 s"},
		{formatWithUnit(48.0/49, "ratio"), "0.9796"},
		{formatWithUnit(-0.00001, "s"), "0 s"},
		{formatNanos(4_800_000_000), "4.8 s"},
		{formatRange(1_500_000_000, 2_000_000_000), "1.5 to 2 s"},
		{formatSpread(0.1, 0.5, "s"), "0.1 to 0.5 s"},
		{formatSpread(15.75, 15.75, "m"), "15.75 m"},
		{formatShare(0.25), "25.0%"},
		{formatUTC(l8behaviour.FixtureBaseUnixNanos), "2025-06-15T15:06:40.000Z"},
		{suppressedDisplay(l8behaviour.ReasonAmbiguousLeader), "suppressed: ambiguous_leader"},
		{formatUncertainty(&l8behaviour.Uncertainty{Kind: l8behaviour.UncertaintyNone}, "s"), "none"},
		{formatUncertainty(ptrUncertainty(l8behaviour.SigmaUncertainty(0.25, l8behaviour.MethodLinearised)), "m"),
			"sigma 0.25 m (linearised)"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func ptrUncertainty(u l8behaviour.Uncertainty) *l8behaviour.Uncertainty { return &u }
