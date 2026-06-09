package report

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

// mockDB implements the DB interface with fixture data.
type mockDB struct {
	callCount int
	siteIDs   []int
	rollupFn  func(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string, modelVersion string, histBucketSize, histMax float64, siteID int, boundaryThreshold int) (*db.RadarStatsResult, error)
}

func (m *mockDB) RadarObjectRollupRange(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string, modelVersion string, histBucketSize, histMax float64, siteID int, boundaryThreshold int) (*db.RadarStatsResult, error) {
	m.callCount++
	m.siteIDs = append(m.siteIDs, siteID)
	if m.rollupFn != nil {
		return m.rollupFn(startUnix, endUnix, groupSeconds, minSpeed, dataSource, modelVersion, histBucketSize, histMax, siteID, boundaryThreshold)
	}
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	if groupSeconds == 0 {
		result := &db.RadarStatsResult{
			Metrics: []db.RadarObjectsRollupRow{
				{
					Classifier: "vehicle",
					StartTime:  base,
					Count:      1200,
					P50Speed:   11.176,
					P85Speed:   15.646,
					P98Speed:   20.117,
					MaxSpeed:   24.587,
				},
			},
			MinSpeedUsed: minSpeed,
		}
		if histBucketSize > 0 {
			result.Histogram = map[float64]int64{
				4.47:  50,
				6.71:  200,
				8.94:  400,
				11.18: 300,
				13.41: 150,
				15.65: 80,
				17.88: 20,
			}
		}
		return result, nil
	}

	rows := make([]db.RadarObjectsRollupRow, 4)
	for i := range rows {
		rows[i] = db.RadarObjectsRollupRow{
			Classifier: "vehicle",
			StartTime:  base.Add(time.Duration(i) * time.Hour),
			Count:      int64(100 + i*50),
			P50Speed:   10.0 + float64(i)*0.5,
			P85Speed:   14.0 + float64(i)*0.5,
			P98Speed:   18.0 + float64(i)*0.5,
			MaxSpeed:   22.0 + float64(i)*0.5,
		}
	}
	return &db.RadarStatsResult{
		Metrics:      rows,
		MinSpeedUsed: minSpeed,
	}, nil
}

func (m *mockDB) GetSite(ctx context.Context, id int) (*db.Site, error) {
	return &db.Site{
		ID:       id,
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Test Surveyor",
		Contact:  "test@example.com",
	}, nil
}

func TestPlanRun_InvalidGroup(t *testing.T) {
	_, err := planRun(Config{
		Group:     "invalid",
		Timezone:  "UTC",
		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported group") {
		t.Fatalf("expected unsupported group error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestPlanRun_InvalidTimezone(t *testing.T) {
	_, err := planRun(Config{
		Group:     "1h",
		Timezone:  "Not/A/Zone",
		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid timezone") {
		t.Fatalf("expected invalid timezone error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestPlanRun_InvalidDate(t *testing.T) {
	_, err := planRun(Config{
		Group:     "1h",
		Timezone:  "UTC",
		StartDate: "not-a-date",
		EndDate:   "2025-06-02",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid start date") {
		t.Fatalf("expected invalid start date error, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestPlanRun_EndDateUsesLocalCalendarDay(t *testing.T) {
	tests := []struct {
		name    string
		endDate string
		want    string
	}{
		{
			name:    "spring forward",
			endDate: "2025-03-09",
			want:    "2025-03-09 23:59:59 PDT",
		},
		{
			name:    "fall back",
			endDate: "2025-11-02",
			want:    "2025-11-02 23:59:59 PST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planRun(Config{
				Group:     "1h",
				Timezone:  "America/Los_Angeles",
				StartDate: tt.endDate,
				EndDate:   tt.endDate,
			})
			if err != nil {
				t.Fatalf("planRun error: %v", err)
			}
			if got := plan.endTime.Format("2006-01-02 15:04:05 MST"); got != tt.want {
				t.Fatalf("expected local end %q, got %q", tt.want, got)
			}
		})
	}
}

func TestPlanRun_PaperSizeNormalisation(t *testing.T) {
	plan, err := planRun(Config{
		Group:     "1h",
		Timezone:  "UTC",
		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
		PaperSize: "LETTER",
	})
	if err != nil {
		t.Fatalf("planRun error: %v", err)
	}
	if plan.paper != "letter" {
		t.Fatalf("expected letter paper, got %q", plan.paper)
	}

	planDefault, err := planRun(Config{
		Group:     "1h",
		Timezone:  "UTC",
		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
		PaperSize: "unsupported-size",
	})
	if err != nil {
		t.Fatalf("planRun default paper error: %v", err)
	}
	if planDefault.paper != "letter" {
		t.Fatalf("expected default letter paper, got %q", planDefault.paper)
	}
}

func TestLoadData_ReportStatsQueriesUsePythonCompatibleSiteIDs(t *testing.T) {
	cfg := Config{
		SiteID:        42,
		StartDate:     "2025-06-01",
		EndDate:       "2025-06-02",
		Timezone:      "UTC",
		Units:         "mph",
		Group:         "1h",
		Source:        "radar_data_transits",
		CompareSource: "radar_objects",
		MinSpeed:      5.0,
		Histogram:     true,
		CosineAngle:   21.0,
		CompareStart:  "2025-05-01",
		CompareEnd:    "2025-05-02",
	}
	plan, err := planRun(cfg)
	if err != nil {
		t.Fatalf("planRun error: %v", err)
	}

	m := &mockDB{}
	data, err := loadData(context.Background(), m, plan)
	if err != nil {
		t.Fatalf("loadData error: %v", err)
	}
	if data.compareResult == nil {
		t.Fatal("comparison data was not loaded")
	}
	if len(m.siteIDs) != 6 {
		t.Fatalf("expected 6 report stats queries, got %d site IDs: %v", len(m.siteIDs), m.siteIDs)
	}
	for i, got := range m.siteIDs[:3] {
		if got != 0 {
			t.Fatalf("primary transit query %d used siteID %d; want 0 to match legacy report metrics", i, got)
		}
	}
	for i, got := range m.siteIDs[3:] {
		if got != cfg.SiteID {
			t.Fatalf("comparison object query %d used siteID %d; want %d", i, got, cfg.SiteID)
		}
	}
}

func TestConvertToTimeSeriesPoints_ZeroCountUsesNaN(t *testing.T) {
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	points := ConvertToTimeSeriesPoints([]db.RadarObjectsRollupRow{
		{StartTime: base, Count: 0},
		{StartTime: base.Add(time.Hour), Count: 1, P50Speed: 10, P85Speed: 11, P98Speed: 12, MaxSpeed: 13},
	}, "mph", time.UTC)

	if len(points) != 2 {
		t.Fatalf("got %d points, want 2", len(points))
	}
	if !math.IsNaN(points[0].P50Speed) || !math.IsNaN(points[0].MaxSpeed) {
		t.Fatalf("zero-count row should use NaN speeds, got %+v", points[0])
	}
	if math.IsNaN(points[1].P50Speed) {
		t.Fatalf("non-zero row should have converted speeds, got %+v", points[1])
	}
}

func TestConvertHistogramKeys(t *testing.T) {
	got := ConvertHistogramKeys(map[float64]int64{10: 2}, "mph")
	if len(got) != 1 {
		t.Fatalf("got %d buckets, want 1", len(got))
	}
	for k, v := range got {
		if v != 2 {
			t.Fatalf("bucket count = %d, want 2", v)
		}
		if k <= 10 {
			t.Fatalf("expected mph-converted key greater than raw mps key, got %v", k)
		}
	}
	if ConvertHistogramKeys(nil, "mph") != nil {
		t.Fatal("nil histogram should stay nil")
	}
}

func TestNormaliseOutputDirMustBeAbsolute(t *testing.T) {
	_, err := normaliseOutputDir("relative/output/path", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("expected absolute output dir error, got: %v", err)
	}
}

func TestSafeOutputPathRejectsEscape(t *testing.T) {
	_, err := safeOutputPath(t.TempDir(), "../report.pdf")
	if err == nil || !strings.Contains(err.Error(), "escapes output dir") {
		t.Fatalf("expected escape error, got: %v", err)
	}
}

func TestSanitiseFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Main Street", "main_street"},
		{"Elm St & 5th Ave!", "elm_st__5th_ave"},
		{"cafe", "cafe"},
		{"test-site_01", "test-site_01"},
	}
	for _, tt := range tests {
		got := sanitiseFilename(tt.in)
		if got != tt.want {
			t.Errorf("sanitiseFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSupportedGroups(t *testing.T) {
	checks := map[string]int64{
		"1h":  3600,
		"all": 0,
		"24h": 86400,
		"7d":  604800,
	}
	for k, want := range checks {
		got, ok := supportedGroups[k]
		if !ok {
			t.Errorf("supportedGroups missing %q", k)
			continue
		}
		if got != want {
			t.Errorf("supportedGroups[%q] = %d, want %d", k, got, want)
		}
	}
}
