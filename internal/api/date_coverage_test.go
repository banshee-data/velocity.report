package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

// seedDescribedSite creates a site with a description plus an active config
// period (the minimum needed for report generation) and returns it.
func seedDescribedSite(t *testing.T, dbInst *db.DB) *db.Site {
	t.Helper()
	desc := "A described survey site"
	site := &db.Site{Name: "Described Site", Location: "L", Surveyor: "S", Contact: "c@e", SiteDescription: &desc}
	if err := dbInst.CreateSite(context.Background(), site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	notes := "cfg"
	if err := dbInst.CreateSiteConfigPeriod(&db.SiteConfigPeriod{
		SiteID: site.ID, EffectiveStartUnix: 0, IsActive: true, Notes: &notes,
	}); err != nil {
		t.Fatalf("create config period: %v", err)
	}
	return site
}

func describedReportBody(siteID int) []byte {
	body, _ := json.Marshal(ReportRequest{
		SiteID: &siteID, StartDate: "2025-12-03", EndDate: "2025-12-03",
		Timezone: "UTC", Units: "mph", Group: "1h", Source: "radar_objects",
	})
	return body
}

// ---------------------------------------------------------------------------
// server_charts.go — small query-param helpers (direct unit tests)
// ---------------------------------------------------------------------------

func TestParseMinSpeed(t *testing.T) {
	if got := parseMinSpeed(url.Values{}, "mph"); got != 0 {
		t.Errorf("empty: got %v, want 0", got)
	}
	if got := parseMinSpeed(url.Values{"min_speed": {"abc"}}, "mph"); got != 0 {
		t.Errorf("invalid: got %v, want 0", got)
	}
	if got := parseMinSpeed(url.Values{"min_speed": {"10"}}, "mph"); got <= 0 {
		t.Errorf("valid: got %v, want > 0 (converted to mps)", got)
	}
}

func TestParseModelVersion(t *testing.T) {
	if got := parseModelVersion(url.Values{}); got != "" {
		t.Errorf("empty: got %q, want \"\"", got)
	}
	if got := parseModelVersion(url.Values{"model_version": {"v2"}}); got != "v2" {
		t.Errorf("set: got %q, want v2", got)
	}
}

func TestParseBoundaryThreshold(t *testing.T) {
	cases := map[string]int{"": 0, "5": 5, "abc": 0, "-1": 0}
	for in, want := range cases {
		q := url.Values{}
		if in != "" {
			q.Set("boundary_threshold", in)
		}
		if got := parseBoundaryThreshold(q); got != want {
			t.Errorf("boundary_threshold=%q: got %d, want %d", in, got, want)
		}
	}
}

func TestParseHistogramParams(t *testing.T) {
	if b, m := parseHistogramParams(url.Values{}, "mph"); b != 5.0 || m != 70.0 {
		t.Errorf("mph defaults: got (%v,%v), want (5,70)", b, m)
	}
	if _, m := parseHistogramParams(url.Values{}, "kph"); m != 110.0 {
		t.Errorf("kph default max: got %v, want 110", m)
	}
	if b, m := parseHistogramParams(url.Values{"bucket_size": {"2"}, "max": {"90"}}, "mph"); b != 2 || m != 90 {
		t.Errorf("custom: got (%v,%v), want (2,90)", b, m)
	}
	// Invalid / non-positive values fall back to defaults.
	if b, m := parseHistogramParams(url.Values{"bucket_size": {"x"}, "max": {"-1"}}, "mph"); b != 5 || m != 70 {
		t.Errorf("invalid: got (%v,%v), want (5,70)", b, m)
	}
}

func TestParseExpandedChart(t *testing.T) {
	if !parseExpandedChart(url.Values{"expanded_chart": {"true"}}) {
		t.Error("expanded_chart=true should be true")
	}
	if !parseExpandedChart(url.Values{"expanded": {"1"}}) {
		t.Error("expanded=1 (alias) should be true")
	}
	if parseExpandedChart(url.Values{"expanded_chart": {"false"}}) {
		t.Error("false should be false")
	}
	if parseExpandedChart(url.Values{}) {
		t.Error("absent should be false")
	}
}

func TestParseSourceAndPaperSize(t *testing.T) {
	if got := parseSource(url.Values{}); got != "radar_objects" {
		t.Errorf("default source: got %q", got)
	}
	if got := parseSource(url.Values{"source": {"radar_data"}}); got != "radar_data" {
		t.Errorf("explicit source: got %q", got)
	}
	// parsePaperSize just normalises; exercise both an explicit and default value.
	_ = parsePaperSize(url.Values{"paper_size": {"letter"}})
	_ = parsePaperSize(url.Values{})
}

func TestParseLocalDateRange_StartAndEndErrors(t *testing.T) {
	utc, _ := time.LoadLocation("UTC")
	if _, _, err := parseLocalDateRange("nope", "2024-01-02", utc); err == nil {
		t.Error("expected start-date error")
	}
	if _, _, err := parseLocalDateRange("2024-01-01", "nope", utc); err == nil {
		t.Error("expected end-date error")
	}
}

// ---------------------------------------------------------------------------
// server_charts.go — resolveTimeSeriesP98Reference
// ---------------------------------------------------------------------------

func TestResolveTimeSeriesP98Reference(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	startUnix := int64(1733184000)      // 2024-12-03 00:00:00 UTC
	endUnix := startUnix + 24*60*60 - 1 // inclusive end of day

	t.Run("explicit p98_ref short-circuits the DB", func(t *testing.T) {
		ref, err := server.resolveTimeSeriesP98Reference(
			url.Values{"p98_ref": {"42"}}, site.ID, startUnix, endUnix, 0, "mph")
		if err != nil || ref != 42 {
			t.Fatalf("got (%v,%v), want (42,nil)", ref, err)
		}
	})

	t.Run("computed from seeded data", func(t *testing.T) {
		ref, err := server.resolveTimeSeriesP98Reference(
			url.Values{}, site.ID, startUnix, endUnix, 0, "mph")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref <= 0 {
			t.Errorf("expected a positive p98 reference, got %v", ref)
		}
	})

	t.Run("empty window yields NaN, no error", func(t *testing.T) {
		ref, err := server.resolveTimeSeriesP98Reference(
			url.Values{}, site.ID, 100, 200, 0, "mph")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !math.IsNaN(ref) {
			t.Errorf("expected NaN for empty window, got %v", ref)
		}
	})
}

func TestResolveTimeSeriesP98Reference_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	if _, err := server.resolveTimeSeriesP98Reference(url.Values{}, 1, 1, 2, 0, "mph"); err == nil {
		t.Error("expected DB error from closed database")
	}
}

// ---------------------------------------------------------------------------
// server_charts.go — writeSVG write-failure path
// ---------------------------------------------------------------------------

func TestWriteSVG_WriteFailure(t *testing.T) {
	// Must not panic when the ResponseWriter errors; the failure is logged.
	writeSVG(&failWriter{writeFail: true}, []byte("<svg/>"))
}

// ---------------------------------------------------------------------------
// server_charts.go — chart handlers (method, validation, expanded, DB error, happy)
// ---------------------------------------------------------------------------

func chartReq(method, url string) (*http.Request, *httptest.ResponseRecorder) {
	return httptest.NewRequest(method, url, nil), httptest.NewRecorder()
}

func TestHandleChartTimeSeries_Paths(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	day := fmt.Sprintf("start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&site_id=%d", site.ID)

	t.Run("method not allowed", func(t *testing.T) {
		req, w := chartReq(http.MethodPost, "/api/charts/timeseries")
		server.handleChartTimeSeries(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("bad params (missing site_id)", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/timeseries?start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z")
		server.handleChartTimeSeries(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("invalid group", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/timeseries?"+day+"&group=nope")
		server.handleChartTimeSeries(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("bad source", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/timeseries?"+day+"&source=nope")
		server.handleChartTimeSeries(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("happy + expanded_chart", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/timeseries?"+day+"&group=1h&expanded_chart=true")
		server.handleChartTimeSeries(w, req)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/svg+xml" {
			t.Errorf("got %d %q", w.Code, w.Header().Get("Content-Type"))
		}
	})
}

func TestHandleChartTimeSeries_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	req, w := chartReq(http.MethodGet, "/api/charts/timeseries?site_id=1&start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&group=1h")
	server.handleChartTimeSeries(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", w.Code)
	}
}

func TestHandleChartHistogram_Paths(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	day := fmt.Sprintf("start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&site_id=%d", site.ID)

	t.Run("method not allowed", func(t *testing.T) {
		req, w := chartReq(http.MethodPost, "/api/charts/histogram")
		server.handleChartHistogram(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("bad params", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/histogram?start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z")
		server.handleChartHistogram(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("bad source", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/histogram?"+day+"&source=nope")
		server.handleChartHistogram(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("happy", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/histogram?"+day+"&bucket_size=5&max=70")
		server.handleChartHistogram(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("got %d", w.Code)
		}
	})
}

func TestHandleChartHistogram_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	req, w := chartReq(http.MethodGet, "/api/charts/histogram?site_id=1&start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC")
	server.handleChartHistogram(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", w.Code)
	}
}

func TestHandleChartComparison_Paths(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	base := fmt.Sprintf("start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&site_id=%d", site.ID)
	cmp := "compare_start=2024-12-03T00:00:00Z&compare_end=2024-12-03T23:59:59Z"

	t.Run("method not allowed", func(t *testing.T) {
		req, w := chartReq(http.MethodPost, "/api/charts/comparison")
		server.handleChartComparison(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("bad params", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/comparison?start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z")
		server.handleChartComparison(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("missing compare window", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/comparison?"+base)
		server.handleChartComparison(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("invalid compare_start instant", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/comparison?"+base+"&compare_start=nope&compare_end=2024-12-03T23:59:59Z")
		server.handleChartComparison(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("invalid compare_end instant", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/comparison?"+base+"&compare_start=2024-12-03T00:00:00Z&compare_end=nope")
		server.handleChartComparison(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("bad compare_source", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/comparison?"+base+"&"+cmp+"&compare_source=nope")
		server.handleChartComparison(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("happy", func(t *testing.T) {
		req, w := chartReq(http.MethodGet, "/api/charts/comparison?"+base+"&"+cmp+"&bucket_size=5&max=70")
		server.handleChartComparison(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("got %d", w.Code)
		}
	})
}

func TestHandleChartComparison_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	req, w := chartReq(http.MethodGet, "/api/charts/comparison?site_id=1&start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&compare_start=2024-12-03T00:00:00Z&compare_end=2024-12-03T23:59:59Z")
	server.handleChartComparison(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", w.Code)
	}
}

// ---------------------------------------------------------------------------
// server_radar.go — valid min_speed, histogram output, encode-error
// ---------------------------------------------------------------------------

func TestShowRadarObjectStats_ValidMinSpeedAndHistogram(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	url := fmt.Sprintf(
		"/api/radar_stats?start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&group=1h&units=mph&min_speed=5&site_id=%d&compute_histogram=true&hist_bucket_size=5&hist_max=100",
		site.ID,
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d. Body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["histogram"]; !ok {
		t.Error("expected histogram in response for seeded data")
	}
}

func TestShowRadarObjectStats_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	url := fmt.Sprintf(
		"/api/radar_stats?start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&group=1h&site_id=%d",
		site.ID,
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	// failWriter errors on Write, exercising the JSON-encode failure branch.
	server.showRadarObjectStats(&failWriter{writeFail: true}, req)
}

// ---------------------------------------------------------------------------
// server_reports_generate.go — compare-date validation branch
// (relativeReportPaths / getReportOutputRoot / buildReportConfig are already
// covered by existing tests; the residual branches are deploy-dir / Getwd /
// Rel-error fault-injection paths.)
// ---------------------------------------------------------------------------

func TestGenerateReport_CompareDateError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	site := seedChartTestData(t, dbInst)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(map[string]interface{}{
		"site_id":            site.ID,
		"start_date":         "2024-12-03",
		"end_date":           "2024-12-03",
		"compare_start_date": "not-a-date",
		"compare_end_date":   "2024-12-02",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400. Body: %s", w.Code, w.Body.String())
	}
}

// TestParseChartParams_InvalidSiteID covers the non-numeric site_id branch.
func TestParseChartParams_InvalidSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req, w := chartReq(http.MethodGet,
		"/api/charts/timeseries?site_id=abc&start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC")
	server.handleChartTimeSeries(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}

// TestBuildReportConfig_IncludesSiteMap covers the site-map branch.
func TestBuildReportConfig_IncludesSiteMap(t *testing.T) {
	siteID := 7
	svg := []byte("<svg>map</svg>")
	site := &db.Site{ID: siteID, IncludeMap: true, MapSVGData: &svg}
	req := ReportRequest{SiteID: &siteID, StartDate: "2024-12-03", EndDate: "2024-12-03", Timezone: "UTC", Units: "mph"}

	cfg := buildReportConfig(req, site, 0, "loc", "surv", "c@e", 25, "desc")
	if !cfg.IncludeMap || string(cfg.MapSVG) != string(svg) {
		t.Errorf("expected site map carried into config: includeMap=%v", cfg.IncludeMap)
	}
}

// TestRelativeReportPaths_RejectZipEscape covers the zip-outside-root branch
// (the existing _RejectEscape test only covers the pdf path).
func TestRelativeReportPaths_RejectZipEscape(t *testing.T) {
	root := filepath.Join(string(os.PathSeparator), "srv", "reports")
	goodPDF := filepath.Join(root, "output", "r.pdf")
	badZIP := filepath.Join(string(os.PathSeparator), "tmp", "outside", "r.zip")
	if _, _, err := relativeReportPaths(root, goodPDF, badZIP); err == nil {
		t.Error("expected error for zip outside root")
	}
}

// TestGenerateReport_DescribedSite exercises the happy generateReportGo path
// and the site.SiteDescription != nil branch.
func TestGenerateReport_DescribedSite(t *testing.T) {
	useMockTypstBinary(t)
	t.Setenv(reportOutputDirEnv, t.TempDir())
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedDescribedSite(t, dbInst)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(describedReportBody(site.ID)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200. Body: %s", w.Code, w.Body.String())
	}
}

// TestGenerateReport_EncodeError covers the final JSON-encode failure branch:
// the report generates successfully but writing the response fails.
func TestGenerateReport_EncodeError(t *testing.T) {
	useMockTypstBinary(t)
	t.Setenv(reportOutputDirEnv, t.TempDir())
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedDescribedSite(t, dbInst)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(describedReportBody(site.ID)))
	req.Header.Set("Content-Type", "application/json")
	// failWriter errors on Write, exercising the encode-failure branch after a
	// successful generation.
	server.generateReport(&failWriter{writeFail: true}, req)
}
