package followingreview

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

func scopeCatalog(site string, windows ...[2]int64) []byte {
	parts := []map[string]string{}
	for _, w := range windows {
		parts = append(parts, map[string]string{"start": time.Unix(0, w[0]).UTC().Format(time.RFC3339Nano), "end": time.Unix(0, w[1]).UTC().Format(time.RFC3339Nano)})
	}
	b, _ := json.Marshal([]any{map[string]any{"id": "other"}, map[string]any{"id": site, "static_parts": parts}})
	return b
}

func TestStaticScopeClipsAndMergesOnlyCoveredPeriods(t *testing.T) {
	b := scopeCatalog("site", [2]int64{500, 900}, [2]int64{200, 300}, [2]int64{0, 50}, [2]int64{90, 200}, [2]int64{150, 250})
	s, err := StaticReviewScope(b, "site", 100, 800)
	if err != nil {
		t.Fatal(err)
	}
	if s.Schema != ReviewScopeSchema || s.CatalogSHA != digest(b) || s.Site != "site" || len(s.Periods) != 2 || s.Periods[0] != (ReviewPeriod{100, 300}) || s.Periods[1] != (ReviewPeriod{500, 800}) {
		t.Fatal(s)
	}
	if s.period(100, 299) != 1 || s.period(500, 799) != 2 || s.period(299, 500) != 0 || s.period(500, 499) != 0 || s.period(800, 800) != 0 {
		t.Fatal("scope bridged a gap or included an exclusive end")
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip ReviewScope
	if err := json.Unmarshal(encoded, &roundTrip); err != nil || roundTrip.Periods[0] != s.Periods[0] {
		t.Fatal(string(encoded), err)
	}
}

func TestStaticScopeRefusesMissingAmbiguousAndInvalidCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, site string
		data       []byte
		first, end int64
	}{
		{"json", "site", []byte("{"), 0, 100},
		{"no site", "", scopeCatalog("site", [2]int64{0, 100}), 0, 100},
		{"empty interval", "site", scopeCatalog("site", [2]int64{0, 100}), 100, 100},
		{"missing", "absent", scopeCatalog("site", [2]int64{0, 100}), 0, 100},
		{"outside", "site", scopeCatalog("site", [2]int64{0, 100}), 200, 300},
		{"reversed", "site", scopeCatalog("site", [2]int64{100, 0}), 0, 100},
		{"duplicate", "site", []byte(`[{"id":"site"},{"id":"site"}]`), 0, 100},
		{"unrepresentable", "site", []byte(`[{"id":"site","static_parts":[{"start":"1500-01-01T00:00:00Z","end":"2026-01-01T00:00:00Z"}]}]`), 0, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := StaticReviewScope(tc.data, tc.site, tc.first, tc.end); err == nil {
				t.Fatal("unverified coverage accepted")
			}
		})
	}
}

func TestStaticReviewRequiresIntervalAndBothActorsPathSupport(t *testing.T) {
	s := &Session{Metadata: Metadata{Start: 0, End: 2000, Binding: Binding{SiteID: "site"}}, paths: []GeometryPath{
		{FollowerTrackID: "f", First: 100, Last: 300, Path: &l8behaviour.LocalPath{}},
		{FollowerTrackID: "leader", First: 100, Last: 300, Path: &l8behaviour.LocalPath{}},
		{FollowerTrackID: "crossing", First: 100, Last: 1100, Path: &l8behaviour.LocalPath{}},
		{FollowerTrackID: "missing", First: 100, Last: 300},
	}}
	scope, err := StaticReviewScope(scopeCatalog("site", [2]int64{0, 500}, [2]int64{1000, 2000}), "site", 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := s.ReviewFilter(scope)
	if err != nil {
		t.Fatal(err)
	}
	row := minimumRow("f", 200, 1)
	if filter(row) != "" {
		t.Fatal("fully supported interval refused")
	}
	row.CaptureUnixNanos = 500
	if filter(row) != "outside_catalogued_static_period" {
		t.Fatal("gap admitted")
	}
	row.CaptureUnixNanos, row.IntervalNanos = 499, 2
	if filter(row) != "outside_catalogued_static_period" {
		t.Fatal("interval across the gap admitted")
	}
	row.CaptureUnixNanos, row.IntervalNanos = 499, 1
	if filter(row) != "" {
		t.Fatal("half-open exposure ending at coverage boundary refused")
	}
	row.CaptureUnixNanos, row.IntervalNanos = 200, 1<<63-1
	if filter(row) != "outside_catalogued_static_period" {
		t.Fatal("overflowing exposure admitted")
	}
	row.CaptureUnixNanos, row.IntervalNanos = 200, 1
	for _, id := range []string{"crossing", "missing", "absent"} {
		row.Decision.LeaderTrackID = id
		if filter(row) != "path_support_crosses_static_period" {
			t.Fatal(id)
		}
	}
	row.Bucket, row.DRAC.Status = "headway_speed_floor", "supported"
	if filter(row) != "path_support_crosses_static_period" {
		t.Fatal("DRAC bypassed path support")
	}
	row.DRAC.Status = "unavailable"
	if filter(row) != "" {
		t.Fatal("existing exclusion reason lost inside covered period")
	}
	legacy, err := s.ReviewFilter(nil)
	if err != nil || legacy(row) != "" {
		t.Fatal(err)
	}
	for _, change := range []func(*ReviewScope){
		func(v *ReviewScope) { v.Schema = "future" }, func(v *ReviewScope) { v.CatalogSHA = "" },
		func(v *ReviewScope) { v.Site = "other" }, func(v *ReviewScope) { v.Periods = nil },
		func(v *ReviewScope) { v.Periods = []ReviewPeriod{{-1, 100}} },
		func(v *ReviewScope) { v.Periods = []ReviewPeriod{{0, 2001}} },
		func(v *ReviewScope) { v.Periods = []ReviewPeriod{{100, 100}} },
		func(v *ReviewScope) { v.Periods = []ReviewPeriod{{0, 100}, {100, 200}} },
	} {
		bad := *scope
		change(&bad)
		if _, err := s.ReviewFilter(&bad); err == nil {
			t.Fatal(bad)
		}
		if _, err := s.AnalyseScoped(context.Background(), &bad, nil); err == nil {
			t.Fatal("analysis accepted incompatible scope")
		}
	}
}

func TestStaticHistogramExcludesLowerMinimumInUnverifiedGap(t *testing.T) {
	s := analysisFixture(t, []fieldrun.EvidenceRow{minimumRow("f", 1030, 1.2), minimumRow("f", 1060, 0.1), minimumRow("outside", 1060, 0.2)})
	s.paths = []GeometryPath{{FollowerTrackID: "f", First: 1021, Last: 1049, Path: &l8behaviour.LocalPath{}}, {FollowerTrackID: "leader", First: 1021, Last: 1049, Path: &l8behaviour.LocalPath{}}}
	scope, err := StaticReviewScope(scopeCatalog(s.Metadata.Binding.SiteID, [2]int64{1020, 1050}), s.Metadata.Binding.SiteID, s.Metadata.Start, s.Metadata.End)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.AnalyseScoped(context.Background(), scope, nil)
	if err != nil || a.Rows != 3 || a.Tracks != 2 || a.WithoutTimeGap != 1 || len(a.Minima) != 1 || a.Minima[0].TimeGapS != 1.2 || a.Buckets["outside_catalogued_static_period"] != 2 || a.Histogram[2].Count != 1 {
		t.Fatal(a, err)
	}
	invalid := analysisFixture(t, []fieldrun.EvidenceRow{minimumRow("bad", 1060, 0)})
	if _, err := invalid.AnalyseScoped(context.Background(), scope, nil); err == nil {
		t.Fatal("selection hid an invalid eligible measurement")
	}
}
