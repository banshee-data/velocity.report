package followingreview

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

func minimumRow(id string, at int64, seconds float64) fieldrun.EvidenceRow {
	gap := seconds * 10
	provenance := l8behaviour.Provenance{Version: l8behaviour.VersionProvenance{EstimateStage: l8behaviour.StageOnline, EstimatorID: "estimator", ObsModelID: "model", MethodID: "method", ParamHash: "params"}, Input: l8behaviour.InputProvenance{ContributingTrackIDs: []string{id, "leader"}, FirstUnixNanos: at, LastUnixNanos: at, ObservedFrames: 2}}
	spatial, err := l8behaviour.NewSuppressedMeasurement(l8behaviour.MetricFollowingSpatialGap, l8behaviour.ReasonEstimateNotFinal, provenance)
	if err != nil {
		panic(err)
	}
	timeGap, err := l8behaviour.NewSuppressedMeasurement(l8behaviour.MetricFollowingNetTimeGap, l8behaviour.ReasonEstimateNotFinal, provenance)
	if err != nil {
		panic(err)
	}
	return fieldrun.EvidenceRow{FollowerTrackID: id, CaptureUnixNanos: at, IntervalNanos: 1, Bucket: "following_eligible",
		Decision: l8behaviour.PairingDecision{FollowerTrackID: id, CaptureUnixNanos: at, LeaderTrackID: "leader", Candidates: []l8behaviour.LeaderCandidate{{TrackID: "leader", Disposition: l8behaviour.DispositionLeader}}},
		Point:    &l8behaviour.FollowingPoint{FollowerTrackID: id, LeaderTrackID: "leader", CaptureUnixNanos: at, Gap: &l8behaviour.GapEstimate{ValueM: gap, LeaderSource: l8behaviour.EndpointTemporallyInferred, FollowerSource: l8behaviour.EndpointTemporallyInferred}, TimeGap: &l8behaviour.TimeGapEstimate{ValueS: &seconds}, SpatialGap: spatial, NetTimeGap: timeGap, SupportedOpportunity: true}}
}

func analysisFixture(t *testing.T, rows []fieldrun.EvidenceRow) *Session {
	t.Helper()
	f := newFixture(t)
	f.slice.Rows = rows
	f.receipt.Rows = len(rows)
	f.receipt.Slices[0].Rows = len(rows)
	f.receipt.Slices[0].SHA256 = digest(writeJSON(t, f.receipt.Slices[0].Path, f.slice))
	writeJSON(t, filepath.Join(f.root, "complete.json"), f.receipt)
	var cache Cache
	s, err := cache.Open(f.run)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFollowingMinimumHistogramPopulationAndEdges(t *testing.T) {
	var rows []fieldrun.EvidenceRow
	for i, v := range []float64{0.49, 0.5, 1, 1.5, 2, 3} {
		rows = append(rows, minimumRow(fmt.Sprint(i), 1000, v))
	}
	// More frames of one track must not create more histogram members.
	rows = append(rows, minimumRow("0", 1050, 0.8))
	excluded := minimumRow("excluded", 1000, 0.1)
	excluded.Bucket = "record_gap"
	rows = append(rows, excluded)
	terminal := minimumRow("terminal", 1000, 0.1)
	terminal.IntervalNanos = 0
	rows = append(rows, terminal)
	s := analysisFixture(t, rows)
	a, err := s.Analyse(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Tracks != 8 || a.WithoutTimeGap != 2 || len(a.Minima) != 6 || a.Rows != 9 {
		t.Fatal(a)
	}
	for i, want := range []int{1, 1, 1, 1, 2} {
		if a.Histogram[i].Count != want {
			t.Fatal(a.Histogram)
		}
	}
	if a.Minima[0].TimeGapS != 0.49 || a.Minima[0].Capture != "1000" {
		t.Fatal(a.Minima)
	}
}

func TestFollowingAnalysisCancellationVisitFailureAndAccounting(t *testing.T) {
	s := analysisFixture(t, []fieldrun.EvidenceRow{minimumRow("one", 1000, 1)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Analyse(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	failure := errors.New("export failed")
	if _, err := s.Analyse(context.Background(), func([]fieldrun.EvidenceRow) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	s.receipt.Rows++
	if _, err := s.Analyse(context.Background(), nil); err == nil {
		t.Fatal("accepted incomplete accounting")
	}
}

func TestFollowingAnalysisRefusesChangedSlice(t *testing.T) {
	s := analysisFixture(t, []fieldrun.EvidenceRow{minimumRow("one", 1000, 1)})
	writeJSON(t, s.receipt.Slices[0].Path, map[string]string{"changed": "evidence"})
	if _, err := s.Analyse(context.Background(), nil); err == nil {
		t.Fatal("accepted replacement evidence")
	}
}

func TestFollowingAnalysisRefusesUnorderedAndInvalidMeasurements(t *testing.T) {
	s := analysisFixture(t, []fieldrun.EvidenceRow{minimumRow("one", 1050, 1), minimumRow("one", 1000, 1)})
	if _, err := s.Analyse(context.Background(), nil); err == nil {
		t.Fatal("unordered samples accepted")
	}
	r := minimumRow("one", 1000, 1)
	r.Point.Gap.ValueM = -1
	s = analysisFixture(t, []fieldrun.EvidenceRow{r})
	if _, err := s.Analyse(context.Background(), nil); err == nil {
		t.Fatal("invalid gap accepted")
	}
}

func TestScopedDRACExposureAndEpisodePeaks(t *testing.T) {
	var rows []fieldrun.EvidenceRow
	add := func(at, duration int64, leader, status string, value float64) {
		r := minimumRow("follower", at, 1)
		r.Point = nil
		r.Bucket = "following_excluded" // DRAC is independent of headway eligibility.
		r.IntervalNanos = duration
		r.Decision.LeaderTrackID = leader
		r.Decision.Candidates[0].TrackID = leader
		r.DRAC.Status = status
		if status == "supported" {
			r.DRAC.Calculation = &l8behaviour.DRACResult{Status: l8behaviour.DRACSupported, ValueMPS2: &value}
		}
		rows = append(rows, r)
	}
	add(1000, 1, "leader", "supported", 0.4)
	add(1001, 1, "leader", "supported", 1)
	add(1002, 1, "other", "supported", 2)
	add(1003, 1, "other", "non_closing", 0)
	add(1004, 1, "other", "supported", 4)
	add(1006, 1, "other", "supported", 2)
	add(1008, 1, "leader", "", 0)
	add(1010, 5, "leader", "record_gap", 0)
	rows[len(rows)-1].Bucket = "record_gap"
	add(1042, 1, "leader", "supported", 4)
	add(1050, 1, "leader", "supported", 4)
	add(1100, 0, "leader", "supported", 4)
	s := analysisFixture(t, rows)
	for _, id := range []string{"follower", "leader", "other"} {
		path := s.paths[0]
		path.FollowerTrackID, path.First, path.Last = id, 1000, 1020
		s.paths = append(s.paths, path)
	}
	scope := &ReviewScope{Schema: ReviewScopeSchema, CatalogSHA: "catalog", Site: "site", Periods: []ReviewPeriod{{1000, 1040}, {1050, 1101}}}
	a, err := s.AnalyseScoped(context.Background(), scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]DRACExposure{
		"supported": {5, 5}, "non_closing": {1, 1}, "missing_drac_status": {1, 1}, "record_gap": {1, 5},
		"outside_catalogued_static_period": {1, 1}, "path_support_crosses_static_period": {1, 1},
	}
	d := a.ExperimentalDRAC
	if d == nil || d.Schema != "following_scoped_drac_v1" || d.TerminalSamples != 1 || !maps.Equal(d.Exposure, want) || d.EpisodePeakBins != [4]int{0, 1, 2, 1} {
		t.Fatalf("unexpected DRAC summary: %+v", d)
	}
	legacy, err := s.Analyse(context.Background(), nil)
	if err != nil || legacy.ExperimentalDRAC != nil {
		t.Fatalf("unscoped analysis changed: %+v, %v", legacy, err)
	}
}

func TestSupportedDRACRequiresValueAndLeader(t *testing.T) {
	d := DRACSummary{Exposure: map[string]DRACExposure{}}
	r := minimumRow("follower", 1000, 1)
	r.DRAC.Status = "supported"
	if err := d.record(r, "", nil); err == nil {
		t.Fatal("accepted supported DRAC without a calculation")
	}
}
