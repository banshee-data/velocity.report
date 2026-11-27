package headway

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

func mixedRateCapture(t *testing.T, id string, frameNS int64, gapS float64, n int, missing bool) CaptureInput {
	t.Helper()
	pair := l8behaviour.ScenarioSteadyApproach().Trajectories
	for i := range pair {
		seed := pair[i].Samples[0]
		// At 3 s gap the follower needs a post-encounter tail to
		// establish a shared spatial path with the leader.
		tail := 0
		if gapS > 2 && i == 0 {
			tail = int(4e9 / frameNS)
		}
		if gapS > 2 && i == 1 {
			tail = -1
		}
		pair[i].Samples = make([]l8behaviour.TrajectorySample, n+1+tail)
		for k := range pair[i].Samples {
			s := seed
			s.CaptureUnixNanos = l8behaviour.FixtureBaseUnixNanos + int64(k)*frameNS
			s.LastObservedUnixNanos = s.CaptureUnixNanos
			s.X = 20 + float64(int64(k)*frameNS)*10/1e9
			s.VX = 10
			if i == 1 {
				s.X += 4.25 + 10*gapS
			}
			if missing && i == 0 && k >= 40 && k < 42 {
				s.Support = l8behaviour.SupportCoasted
				s.Faces = l8behaviour.FaceVisibility{}
				s.LastObservedUnixNanos = l8behaviour.FixtureBaseUnixNanos + 39*frameNS
			}
			pair[i].Samples[k] = s
		}
	}
	params := l8behaviour.EncounterScenarioParams()
	a, err := l8behaviour.AnalyseFollowing(pair, params)
	if err != nil {
		t.Fatal(err)
	}
	return CaptureInput{ID: id, Source: "mixed-rate synthetic oracle/" + id, Trajectories: pair, Params: params, Analysis: a}
}

func TestReportMixedRateFollowingExposure(t *testing.T) {
	for _, missing := range []bool{false, true} {
		n := 100
		if missing {
			n += 2
		}
		r, err := Build(Input{Status: StatusSyntheticOracle, Captures: []CaptureInput{mixedRateCapture(t, "10Hz", 100_000_000, 1, n, missing), mixedRateCapture(t, "20Hz", 50_000_000, 3, 40, false)}})
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		if len(r.Encounters) != 2 || len(r.Aggregates) != 1 {
			t.Fatalf("encounters=%d groups=%d", len(r.Encounters), len(r.Aggregates))
		}
		a := r.Aggregates[0]
		wantDen := int64(12e9)
		if missing {
			wantDen += 200_000_000
		}
		if a.Histogram.DenominatorNanos != wantDen {
			t.Fatalf("histogram denominator=%d expected=%d", a.Histogram.DenominatorNanos, wantDen)
		}
		var short, long, excluded int64
		var shortShare float64
		for _, bin := range a.Histogram.Bins {
			if bin.LowerMillis < 2000 {
				short += bin.Nanos
				shortShare += bin.Share
			} else {
				long += bin.Nanos
			}
		}
		for _, e := range a.Histogram.Excluded {
			if e.Reason != l8behaviour.ReasonNotObserved {
				t.Fatalf("unexpected exclusion reason: %s", e.Reason)
			}
			excluded += e.Nanos
		}
		if short != 10e9 || long != 2e9 || excluded != wantDen-12e9 {
			t.Fatalf("short=%d long=%d excluded=%d", short, long, excluded)
		}
		if math.Abs(shortShare-float64(short)/float64(wantDen)) > 1e-12 {
			t.Fatalf("share=%f", shortShare)
		}
		bandFound, metricFound := false, false
		for _, band := range a.Bands {
			if band.Seconds != 2 {
				continue
			}
			bandFound = true
			if band.BelowNanos != short || band.ValidNanos != short+long || band.RateValue == nil || math.Abs(*band.RateValue-5.0/6) > 1e-12 {
				t.Fatalf("pooled 2 s band: %+v", band)
			}
		}
		for _, m := range a.Metrics {
			if m.Metric != l8behaviour.MetricFollowingNetTimeGapP50 {
				continue
			}
			metricFound = true
			if m.Supported != 2 || m.Min == nil || m.Max == nil || m.P50 == nil || math.Abs(*m.Min-1) > 1e-12 || math.Abs(*m.Max-3) > 1e-12 || math.Abs(*m.P50-2) > 1e-12 {
				t.Fatalf("encounter distribution: %+v", m)
			}
		}
		if !bandFound || !metricFound {
			t.Fatalf("report omitted expected 2 s band (%t) or encounter P50 (%t)", bandFound, metricFound)
		}
		t.Logf("missing=%t short=%d long=%d excluded=%d valid_share=%.6f display_histogram_short_share=%.6f encounters=%d", missing, short, long, excluded, float64(short)/float64(short+long), shortShare, len(r.Encounters))
	}
}
