package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func newGridChartServer(t *testing.T, sensorID string) *Server {
	t.Helper()
	bm := l3grid.NewBackgroundManagerDI(sensorID, 4, 120, l3grid.BackgroundParams{}, nil)
	l3grid.RegisterBackgroundManager(sensorID, bm)
	populateGridCell(bm, 0, 0, 10, 8)
	populateGridCell(bm, 1, 3, 12, 2)
	populateGridCell(bm, 3, 60, 20, 30)
	return &Server{sensorID: sensorID}
}

func TestHandleGridChartRendersSVG(t *testing.T) {
	ws := newGridChartServer(t, "grid-chart-sensor")
	for _, metric := range []string{"", "fill_rate", "settle_rate", "unsettled_ratio", "frozen_ratio", "mean_times_seen"} {
		t.Run("metric="+metric, func(t *testing.T) {
			url := "/api/charts/grid?azimuth_bucket_deg=6&settled_threshold=5"
			if metric != "" {
				url += "&metric=" + metric
			}
			rec := httptest.NewRecorder()
			ws.handleGridChart(rec, httptest.NewRequest(http.MethodGet, url, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
				t.Errorf("Content-Type = %q", ct)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "<svg") || !strings.Contains(body, "Background grid: ") {
				t.Errorf("not a grid chart: %.200s", body)
			}
			if !strings.Contains(body, "grid-chart-sensor at ") {
				t.Error("subtitle should name the sensor")
			}
			if !strings.Contains(body, "Filled: 3") {
				t.Errorf("summary should count the three filled cells")
			}
		})
	}
}

func TestHandleGridChartNoGridIsAMessage(t *testing.T) {
	ws := &Server{sensorID: "grid-chart-missing"}
	rec := httptest.NewRecorder()
	ws.handleGridChart(rec, httptest.NewRequest(http.MethodGet, "/api/charts/grid", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No background grid yet") {
		t.Error("expected the labelled message chart")
	}
}

func TestHandleGridChartRejectsBadQuery(t *testing.T) {
	ws := newGridChartServer(t, "grid-chart-bad")
	for _, q := range []string{
		"metric=nonsense",
		"azimuth_bucket_deg=0.5",
		"azimuth_bucket_deg=abc",
		"azimuth_bucket_deg=91",
		"settled_threshold=-1",
	} {
		rec := httptest.NewRecorder()
		ws.handleGridChart(rec, httptest.NewRequest(http.MethodGet, "/api/charts/grid?"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestGridMetricsUndefinedBuckets(t *testing.T) {
	empty := l3grid.CoarseBucket{}
	for name, m := range gridMetrics {
		if _, ok := m.value(empty); ok {
			t.Errorf("%s should be undefined for an empty bucket", name)
		}
	}
	if got := gridMetricNames(); got != "fill_rate, frozen_ratio, mean_times_seen, settle_rate, unsettled_ratio" {
		t.Errorf("gridMetricNames = %q", got)
	}
}

func TestGridSummary(t *testing.T) {
	if got := gridSummary(nil); got != nil {
		t.Errorf("gridSummary(nil) = %v, want nil", got)
	}
	got := gridSummary(map[string]interface{}{"total_filled": 4.0, "fill_rate": 0.5, "total_frozen": 1})
	if len(got) != 1 || got[0] != "Filled: 4 (50.0%) · Frozen: 1" {
		t.Errorf("gridSummary = %v", got)
	}
}
