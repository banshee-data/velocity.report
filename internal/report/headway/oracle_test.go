package headway

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

func oracleReport(t *testing.T) Report {
	t.Helper()
	r, err := Oracle()
	if err != nil {
		t.Fatalf("Oracle: %v", err)
	}
	return r
}

func findEncounter(t *testing.T, r Report, capture, leader, follower string) Encounter {
	t.Helper()
	for _, e := range r.Encounters {
		if e.CaptureID == capture && e.Leader.TrackID == leader && e.Follower.TrackID == follower {
			return e
		}
	}
	t.Fatalf("no encounter %s -> %s in %s", leader, follower, capture)
	return Encounter{}
}

func valueOf(t *testing.T, e Encounter, id l8behaviour.MetricID) Value {
	t.Helper()
	for _, v := range e.Measurements {
		if v.Metric == id {
			return v
		}
	}
	t.Fatalf("encounter %s has no %s", e.ID, id)
	return Value{}
}

// literal is the exact decimal of a known value and its unit, written
// independently of the report's own formatter. Gaps, time gaps and durations
// in the scenarios are short exact decimals and print in full; a ratio such
// as 48/49 does not terminate, so its printed form is its four-decimal
// rounding while its data value stays exact.
func literal(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i-1 > 4 {
		s = strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 4, 64), "0"), ".")
	}
	if unit == "ratio" {
		return s
	}
	return s + " " + unit
}

func wantValue(t *testing.T, e Encounter, id l8behaviour.MetricID, want float64) {
	t.Helper()
	v := valueOf(t, e, id)
	if v.Suppressed || v.Value == nil {
		t.Errorf("%s %s is suppressed (%s), want %v", e.ID, id, v.Reason, want)
		return
	}
	if *v.Value != want {
		t.Errorf("%s %s = %v, want exactly %v", e.ID, id, *v.Value, want)
	}
	if lit := literal(want, v.Unit); v.Display != lit {
		t.Errorf("%s %s prints %q, want %q", e.ID, id, v.Display, lit)
	}
}

func wantSuppressed(t *testing.T, e Encounter, id l8behaviour.MetricID, reason l8behaviour.SuppressionReason) {
	t.Helper()
	v := valueOf(t, e, id)
	if !v.Suppressed || v.Value != nil || v.Reason != reason {
		t.Errorf("%s %s = suppressed %v, reason %s; want suppressed with %s", e.ID, id, v.Suppressed, v.Reason, reason)
	}
	if v.Display != "suppressed: "+reason.String() {
		t.Errorf("%s %s prints %q", e.ID, id, v.Display)
	}
}

// TestOracleReportsEveryKnownValueExactly drives the report from every frozen
// scenario and holds each printed gap, time gap, valid time, band duration,
// rate and suppression tally to the value the scenario states.
func TestOracleReportsEveryKnownValueExactly(t *testing.T) {
	r := oracleReport(t)
	if r.Status != StatusSyntheticOracle || r.StatusLabel != "SYNTHETIC ORACLE" {
		t.Fatalf("oracle status = %s %q", r.Status, r.StatusLabel)
	}
	total := 0
	for _, sc := range l8behaviour.EncounterScenarios() {
		minOpportunity := sc.Params.Exposure.MinOpportunitySeconds
		n := 0
		for _, e := range r.Encounters {
			if e.CaptureID == sc.Name {
				n++
			}
		}
		if n != len(sc.Encounters) {
			t.Errorf("%s: report has %d encounters, scenario states %d", sc.Name, n, len(sc.Encounters))
		}
		for _, x := range sc.Encounters {
			total++
			e := findEncounter(t, r, sc.Name, x.LeaderTrackID, x.FollowerTrackID)
			if e.Stage != l8behaviour.StageFinal || e.ValueBlock != ValueBlockMeasurements {
				t.Errorf("%s: stage %s reads %s", e.ID, e.Stage, e.ValueBlock)
			}
			valid := float64(x.ValidNanos) / 1e9
			wantValue(t, e, l8behaviour.MetricFollowingValidTime, valid)
			if e.Accounting.ValidNanos != x.ValidNanos || e.Accounting.UnobservedNanos != x.UnobservedNanos {
				t.Errorf("%s accounting valid %d unobserved %d, want %d and %d", e.ID,
					e.Accounting.ValidNanos, e.Accounting.UnobservedNanos, x.ValidNanos, x.UnobservedNanos)
			}

			for _, s := range []struct {
				id   l8behaviour.MetricID
				want *float64
			}{
				{l8behaviour.MetricFollowingSpatialGapMin, x.SpatialGapMin},
				{l8behaviour.MetricFollowingSpatialGapP50, x.SpatialGapP50},
				{l8behaviour.MetricFollowingNetTimeGapMin, x.NetTimeGapMin},
				{l8behaviour.MetricFollowingNetTimeGapP50, x.NetTimeGapP50},
			} {
				if s.want == nil {
					wantSuppressed(t, e, s.id, l8behaviour.ReasonInsufficientObservation)
				} else {
					wantValue(t, e, s.id, *s.want)
				}
			}

			for b, band := range l8behaviour.FollowingBands() {
				if valid < minOpportunity {
					wantSuppressed(t, e, band.Duration, l8behaviour.ReasonInsufficientObservation)
					wantSuppressed(t, e, band.Rate, l8behaviour.ReasonInsufficientObservation)
					continue
				}
				wantValue(t, e, band.Duration, float64(x.BandNanos[b])/1e9)
				wantValue(t, e, band.Rate, float64(x.BandNanos[b])/float64(x.ValidNanos))
				if o := valueOf(t, e, band.Rate).OpportunityDisplay; o != literal(valid, "s") {
					t.Errorf("%s %s opportunity prints %q, want %q", e.ID, band.Rate, o, literal(valid, "s"))
				}
			}

			var got []l8behaviour.ReasonTally
			for _, s := range e.Accounting.Suppressions {
				got = append(got, l8behaviour.ReasonTally{Reason: s.Reason, Instants: s.Instants, Nanos: s.Nanos})
				if s.Display != literal(float64(s.Nanos)/1e9, "s") {
					t.Errorf("%s %s time prints %q", e.ID, s.Reason, s.Display)
				}
			}
			if !reflect.DeepEqual(got, x.Suppressions) {
				t.Errorf("%s suppressions = %+v, want %+v", e.ID, got, x.Suppressions)
			}

			predicted := 0
			for _, p := range e.Predicted {
				predicted += p.Instants
			}
			if predicted != x.PredictedGaps || len(e.Series.Predicted) != x.PredictedGaps {
				t.Errorf("%s predicted instants = %d (series %d), want %d", e.ID, predicted, len(e.Series.Predicted), x.PredictedGaps)
			}
			if len(e.Series.Gap) != x.SpatialGapSize || len(e.Series.NetTimeGap) != x.NetTimeGapSize {
				t.Errorf("%s series sizes %d, %d; want %d, %d", e.ID, len(e.Series.Gap), len(e.Series.NetTimeGap),
					x.SpatialGapSize, x.NetTimeGapSize)
			}
		}
	}
	if total != len(r.Encounters) || total != 8 {
		t.Errorf("checked %d encounters of %d; the scenarios state 8", total, len(r.Encounters))
	}
}

// TestOracleSteadyApproachEndpointsAndIntervals pins the one scenario that
// crosses every band: its endpoints at the minimum gap, their sources, and
// the absence of any unsupported interval.
func TestOracleSteadyApproachEndpointsAndIntervals(t *testing.T) {
	r := oracleReport(t)
	e := findEncounter(t, r, "steady_approach", "trk_s_steady_leader", "trk_s_steady_follower")
	if len(e.Unsupported) != 0 || len(e.Predicted) != 0 {
		t.Errorf("steady approach has unsupported %v and predicted %v, want none", e.Unsupported, e.Predicted)
	}
	m := e.Endpoints.AtMinimumGap
	if m == nil {
		t.Fatal("steady approach has no endpoints at its minimum gap")
	}
	// Frame 49: leader centre 44.25 + 0.75 x 49 = 81 m, rear 78.75 m;
	// follower centre 69 m, front 71 m; gap 7.75 m.
	if m.OffsetDisplay != "4.9 s" || m.GapDisplay != "7.75 m, sigma 0.25 m" {
		t.Errorf("minimum gap at %s is %s", m.OffsetDisplay, m.GapDisplay)
	}
	if m.Leader.Extremity != l8behaviour.ExtremityTrailing || m.Leader.Source != l8behaviour.EndpointTemporallyInferred ||
		m.Follower.Extremity != l8behaviour.ExtremityLeading || m.Follower.Source != l8behaviour.EndpointDirectlyObserved {
		t.Errorf("endpoint extremities and sources = %+v / %+v", m.Leader, m.Follower)
	}
	if m.Leader.ArcM-m.Follower.ArcM != 7.75 {
		t.Errorf("endpoint arcs %v and %v are not 7.75 m apart", m.Leader.ArcM, m.Follower.ArcM)
	}
	want := []SourceCount{{Source: l8behaviour.EndpointDirectlyObserved, Instants: 50}}
	if !reflect.DeepEqual(e.Endpoints.FollowerLeading, want) {
		t.Errorf("follower leading sources = %+v", e.Endpoints.FollowerLeading)
	}
}

// TestOraclePartialViewCountsPriorDominatedEndpoints: the follower's front is
// a class prior for five frames. Those gaps are suppressed, and the endpoint
// tally still says so rather than showing only the good frames.
func TestOraclePartialViewCountsPriorDominatedEndpoints(t *testing.T) {
	r := oracleReport(t)
	e := findEncounter(t, r, "partial_view", "trk_s_partial_leader", "trk_s_partial_follower")
	want := []SourceCount{
		{Source: l8behaviour.EndpointDirectlyObserved, Instants: 25},
		{Source: l8behaviour.EndpointPriorDominated, Instants: 5},
	}
	if !reflect.DeepEqual(e.Endpoints.FollowerLeading, want) {
		t.Errorf("follower leading sources = %+v, want %+v", e.Endpoints.FollowerLeading, want)
	}
	if len(e.Unsupported) != 1 || e.Unsupported[0].Reason != l8behaviour.ReasonExtentNotConverged ||
		e.Unsupported[0].RangeDisplay != "0 to 0.5 s" || e.Unsupported[0].Instants != 5 {
		t.Errorf("unsupported = %+v", e.Unsupported)
	}
}

// TestOraclePredictedOnlyIntervalsAreExcludedFromAggregates: the occlusion's
// coasted frames appear as one review-only predicted run with its coast age,
// are counted as unsupported time, and change nothing in any aggregate even
// when their values are replaced.
func TestOraclePredictedOnlyIntervalsAreExcludedFromAggregates(t *testing.T) {
	in, err := OracleInput()
	if err != nil {
		t.Fatal(err)
	}
	base, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	e := findEncounter(t, base, "occlusion", "trk_s_occlusion_leader", "trk_s_occlusion_follower")
	if len(e.Predicted) != 1 {
		t.Fatalf("predicted runs = %+v, want one", e.Predicted)
	}
	p := e.Predicted[0]
	if p.Instants != 5 || p.RangeDisplay != "1.5 to 2 s" || p.CoastAgeDisplay != "0.1 to 0.5 s" ||
		p.GapDisplay != "15.75 m" || p.SigmaMaxDisplay != "0.375 m" || p.Visibility != l8behaviour.VisibilityReviewOnly {
		t.Errorf("predicted run = %+v", p)
	}
	wantUnsupported := []string{"not_observed 1.5 to 1.8 s", "model_degraded 1.8 to 2 s"}
	var got []string
	for _, u := range e.Unsupported {
		got = append(got, u.Reason.String()+" "+u.RangeDisplay)
	}
	if strings.Join(got, "; ") != strings.Join(wantUnsupported, "; ") {
		t.Errorf("unsupported = %v, want %v", got, wantUnsupported)
	}

	// Replace every predicted gap with a value inside every band and the
	// aggregates must not move.
	for ci := range in.Captures {
		for ei := range in.Captures[ci].Analysis.Encounters {
			enc := &in.Captures[ci].Analysis.Encounters[ei]
			pred := append([]l8behaviour.PredictedPoint(nil), enc.PredictedGapSeries...)
			for i := range pred {
				pred[i].ValueM, pred[i].SigmaM = 0.5, 0.01
			}
			enc.PredictedGapSeries = pred
		}
	}
	mutated, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base.Aggregates, mutated.Aggregates) {
		t.Error("changing review-only predicted gaps changed an aggregate")
	}
	if reflect.DeepEqual(base.Encounters, mutated.Encounters) {
		t.Error("the mutation did not reach the predicted rows, so the test proves nothing")
	}
}

// TestOracleAggregates pins both version groups: the six encounters under
// the shared scenario parameters, and the ambiguous pair under its own.
func TestOracleAggregates(t *testing.T) {
	r := oracleReport(t)
	if len(r.Aggregates) != 2 {
		t.Fatalf("aggregates = %d, want 2 version groups", len(r.Aggregates))
	}
	a1, a2 := r.Aggregates[0], r.Aggregates[1]
	if len(a1.EncounterIDs) != 6 || len(a2.EncounterIDs) != 2 {
		t.Fatalf("group sizes %d and %d, want 6 and 2", len(a1.EncounterIDs), len(a2.EncounterIDs))
	}
	if a1.Version.MethodID == a2.Version.MethodID {
		t.Error("the groups must differ in method parameters")
	}

	// Pooled over steady, occlusion, standstill, lane, partial and crossing.
	wantBelow := []string{"20.6 s", "9.6 s", "0.8 s"}
	for b, be := range a1.Bands {
		if be.Encounters != 6 || be.ValidDisplay != "20.7 s" || be.BelowDisplay != wantBelow[b] {
			t.Errorf("A1 band %s: %d encounters, below %s of %s", be.Display, be.Encounters, be.BelowDisplay, be.ValidDisplay)
		}
		if *be.RateValue != float64(be.BelowNanos)/float64(be.ValidNanos) {
			t.Errorf("A1 band %s rate is not the pooled ratio", be.Display)
		}
	}
	for _, be := range a2.Bands {
		if be.Encounters != 0 || be.RateValue != nil || be.RateReason != l8behaviour.ReasonInsufficientObservation ||
			be.RateDisplay != "suppressed: insufficient_observation" {
			t.Errorf("A2 band %s = %+v, want suppressed for want of opportunity", be.Display, be)
		}
		if !reflect.DeepEqual(be.Excluded, []ReasonCount{{Reason: l8behaviour.ReasonInsufficientObservation, Encounters: 2}}) {
			t.Errorf("A2 band %s excluded = %+v", be.Display, be.Excluded)
		}
	}

	h1 := a1.Histogram
	if h1.DenominatorDisplay != "23.4 s" || h1.Encounters != 6 {
		t.Errorf("A1 histogram denominator %s over %d encounters", h1.DenominatorDisplay, h1.Encounters)
	}
	wantExcluded := map[l8behaviour.SuppressionReason]int64{
		l8behaviour.ReasonModelDegraded: 2e8, l8behaviour.ReasonNotObserved: 3e8, l8behaviour.ReasonNoCommonPath: 7e8,
		l8behaviour.ReasonExtentNotConverged: 5e8, l8behaviour.ReasonBelowSpeedFloor: 1e9,
	}
	if len(h1.Excluded) != len(wantExcluded) {
		t.Errorf("A1 excluded = %+v", h1.Excluded)
	}
	for _, x := range h1.Excluded {
		if wantExcluded[x.Reason] != x.Nanos {
			t.Errorf("A1 excluded %s = %d ns, want %d", x.Reason, x.Nanos, wantExcluded[x.Reason])
		}
	}
	// The valid time left of each band's rule is exactly the pooled time
	// below the band.
	for _, a := range r.Aggregates {
		for _, be := range a.Bands {
			var left int64
			for _, bin := range a.Histogram.Bins {
				if bin.UpperMillis != nil && *bin.UpperMillis <= int(be.Seconds*1000) {
					left += bin.Nanos
				}
			}
			if left != be.BelowNanos {
				t.Errorf("%s: histogram time below %s is %d ns, band exposure says %d", a.ID, be.Display, left, be.BelowNanos)
			}
		}
	}

	h2 := a2.Histogram
	if h2.DenominatorDisplay != "4.9 s" || h2.Encounters != 0 {
		t.Errorf("A2 histogram denominator %s over %d encounters", h2.DenominatorDisplay, h2.Encounters)
	}
	wantA2 := []ExcludedTime{
		{Reason: l8behaviour.ReasonInsufficientObservation, Nanos: 9e8, Display: "0.9 s"},
		{Reason: l8behaviour.ReasonAmbiguousLeader, Nanos: 4e9, Display: "4 s"},
	}
	for i := range h2.Excluded {
		h2.Excluded[i].Share, h2.Excluded[i].ShareDisplay = 0, ""
	}
	if !reflect.DeepEqual(h2.Excluded, wantA2) {
		t.Errorf("A2 excluded = %+v, want %+v", h2.Excluded, wantA2)
	}

	// Distributions count suppressed encounters by reason, never as zero.
	for _, d := range a2.Metrics {
		if d.Metric != l8behaviour.MetricFollowingSpatialGapMin {
			continue
		}
		if d.Supported != 1 || d.MinDisplay != "15.75 m" ||
			!reflect.DeepEqual(d.Suppressed, []ReasonCount{{Reason: l8behaviour.ReasonInsufficientObservation, Encounters: 1}}) {
			t.Errorf("A2 spatial gap minimum = %+v", d)
		}
	}
	// Encounter minima 0.775, 1.2, 1.375, 1.575, 1.575 and 1.575 s: the
	// median of six is the mean of the middle two.
	for _, d := range a1.Metrics {
		if d.Metric == l8behaviour.MetricFollowingNetTimeGapMin &&
			(d.Supported != 6 || d.MinDisplay != "0.775 s" || d.P50Display != "1.475 s" || d.MaxDisplay != "1.575 s") {
			t.Errorf("A1 net time gap minimum = %+v", d)
		}
	}
}

// TestOracleSuppressedValuesNeverPrintAsZero walks every suppression the
// report prints and checks it has no number at all, in the model and in the
// marshalled data.
func TestOracleSuppressedValuesNeverPrintAsZero(t *testing.T) {
	r := oracleReport(t)
	suppressed := 0
	check := func(where, display string) {
		if strings.ContainsAny(display, "0123456789") || !strings.HasPrefix(display, "suppressed: ") {
			t.Errorf("%s prints %q", where, display)
		}
	}
	for _, e := range r.Encounters {
		for _, v := range e.Measurements {
			if v.Suppressed {
				suppressed++
				check(e.ID+" "+string(v.Metric), v.Display)
			}
		}
	}
	for _, a := range r.Aggregates {
		for _, b := range a.Bands {
			if b.RateValue == nil {
				check(a.ID+" "+string(b.Rate), b.RateDisplay)
			}
		}
		for _, d := range a.Metrics {
			if d.Supported == 0 {
				check(a.ID+" "+string(d.Metric), d.MinDisplay)
			}
		}
	}
	if suppressed == 0 {
		t.Fatal("the oracle must exercise suppression")
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Encounters []struct {
			Measurements []map[string]any `json:"measurements"`
		} `json:"encounters"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, e := range decoded.Encounters {
		for _, m := range e.Measurements {
			v, present := m["value"]
			if m["suppressed"] == true && (!present || v != nil) {
				t.Errorf("suppressed %v marshals value %v; want an explicit null", m["metric"], v)
			}
		}
	}
}

func TestReportRoundTripsThroughJSON(t *testing.T) {
	r := oracleReport(t)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Report
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("decoded report: %v", err)
	}
	again, _ := json.Marshal(back)
	if string(again) != string(raw) {
		t.Error("report does not round-trip byte for byte")
	}
}
