package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

// seedChartTestData creates a site, config period, and radar_objects rows
// that the chart endpoints can query. Returns the site.
func seedChartTestData(t *testing.T, dbInst *db.DB) *db.Site {
	t.Helper()

	site := &db.Site{
		Name:     "Chart Test Site",
		Location: "Chart Location",
		Surveyor: "Tester",
		Contact:  "chart@example.com",
	}
	if err := dbInst.CreateSite(context.Background(), site); err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	initialNotes := "Chart test config"
	cfg := &db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		IsActive:           true,
		Notes:              &initialNotes,
		CosineErrorAngle:   0.0,
	}
	if err := dbInst.CreateSiteConfigPeriod(cfg); err != nil {
		t.Fatalf("failed to create site config period: %v", err)
	}

	// Insert 20 radar_objects events across 2024-12-03 (UTC).
	baseTimestamp := int64(1733184000) // 2024-12-03 00:00:00 UTC
	speeds := []float64{8.0, 10.0, 12.0, 14.0, 16.0, 18.0, 20.0, 22.0, 25.0, 28.0}

	for i := 0; i < 20; i++ {
		speed := speeds[i%len(speeds)]
		ts := baseTimestamp + int64(i*1800) // every 30 min
		evt := map[string]interface{}{
			"site_id":         site.ID,
			"classifier":      "all",
			"start_time":      float64(ts),
			"end_time":        float64(ts + 2),
			"delta_time_msec": 100,
			"max_speed_mps":   speed,
			"min_speed_mps":   speed - 1.0,
			"speed_change":    1.0,
			"max_magnitude":   10,
			"avg_magnitude":   10,
			"total_frames":    1,
			"frames_per_mps":  1.0,
			"length_m":        3.5,
		}
		raw, _ := json.Marshal(evt)
		_, err := dbInst.Exec(
			`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`,
			string(raw), float64(ts),
		)
		if err != nil {
			t.Fatalf("failed to insert radar object %d: %v", i, err)
		}
	}

	return site
}

func TestInclusiveLocalDateEnd_HandlesDSTTransitions(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	tests := []struct {
		name string
		day  time.Time
		want string
	}{
		{
			name: "spring forward",
			day:  time.Date(2025, 3, 9, 0, 0, 0, 0, loc),
			want: "2025-03-09 23:59:59 PDT",
		},
		{
			name: "fall back",
			day:  time.Date(2025, 11, 2, 0, 0, 0, 0, loc),
			want: "2025-11-02 23:59:59 PST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inclusiveLocalDateEnd(tt.day).Format("2006-01-02 15:04:05 MST"); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// --- Phase 4b tests ---

func TestChartEndpoints_TimeSeries(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf("/api/charts/timeseries?site_id=%d&start=2024-12-03&end=2024-12-03&tz=UTC&units=mph&group=1h", site.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("expected Content-Type image/svg+xml, got %s", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store, no-cache, must-revalidate, max-age=0" {
		t.Errorf("expected no-store Cache-Control, got %s", cc)
	}
	if pragma := w.Header().Get("Pragma"); pragma != "no-cache" {
		t.Errorf("expected Pragma no-cache, got %s", pragma)
	}
	if expires := w.Header().Get("Expires"); expires != "0" {
		t.Errorf("expected Expires 0, got %s", expires)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Errorf("response body does not contain <svg root element")
	}
	if !strings.Contains(body, `width="190.000mm"`) {
		t.Errorf("expected A4 time-series width, got %s", body)
	}
	if !strings.Contains(body, `class="p98-reference"`) {
		t.Errorf("expected aggregate p98 reference line, got %s", body)
	}
	if !strings.Contains(body, `<title>`) {
		t.Errorf("expected hover tooltip titles in SVG, got %s", body)
	}
}

func TestChartEndpoints_TimeSeries_LetterPaperSize(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf("/api/charts/timeseries?site_id=%d&start=2024-12-03&end=2024-12-03&tz=UTC&units=mph&group=1h&paper_size=letter", site.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `width="195.900mm"`) {
		t.Fatalf("expected letter-sized time-series width, got %s", w.Body.String())
	}
}

func TestChartEndpoints_TimeSeries_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/charts/timeseries", nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestChartEndpoints_TimeSeries_MissingParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name string
		url  string
	}{
		{"missing site_id", "/api/charts/timeseries?start=2024-12-03&end=2024-12-03"},
		{"missing start", "/api/charts/timeseries?site_id=1&end=2024-12-03"},
		{"missing end", "/api/charts/timeseries?site_id=1&start=2024-12-03"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			server.ServeMux().ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestChartEndpoints_Histogram(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf("/api/charts/histogram?site_id=%d&start=2024-12-03&end=2024-12-03&tz=UTC&units=mph&bucket_size=5&max=70", site.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("expected Content-Type image/svg+xml, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Errorf("response body does not contain <svg root element")
	}
}

func TestChartEndpoints_Comparison(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf(
		"/api/charts/comparison?site_id=%d&start=2024-12-03&end=2024-12-03&compare_start=2024-12-03&compare_end=2024-12-03&tz=UTC&units=mph&bucket_size=5&max=70",
		site.ID,
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("expected Content-Type image/svg+xml, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Errorf("response body does not contain <svg root element")
	}
}

func TestChartEndpoints_Comparison_MissingCompareParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf("/api/charts/comparison?site_id=%d&start=2024-12-03&end=2024-12-03&tz=UTC&units=mph", site.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChartEndpoints_InvalidGroup(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf("/api/charts/timeseries?site_id=%d&start=2024-12-03&end=2024-12-03&group=invalid", site.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChartEndpoints_InvalidUnits(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	url := fmt.Sprintf("/api/charts/timeseries?site_id=%d&start=2024-12-03&end=2024-12-03&units=furlongs", site.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
