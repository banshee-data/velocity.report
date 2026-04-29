package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

// TestGenerateReport_RequiresTools tests the Go PDF report pipeline.
// It verifies the handler invokes the Go pipeline directly (no Python fallback).
// If rsvg-convert or xelatex are not installed the test still validates the
// error originates from the Go pipeline, not a Python exec path.
func TestGenerateReport_RequiresTools(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	reqBody := ReportRequest{
		SiteID:            &site.ID,
		StartDate:         "2025-12-03",
		EndDate:           "2025-12-03",
		Timezone:          "UTC",
		Units:             "mph",
		Group:             "1h",
		Source:            "radar_objects",
		Histogram:         true,
		HistBucketSize:    5,
		HistMax:           70,
		BoundaryThreshold: 0,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	respBody := w.Body.String()

	if w.Code == http.StatusOK {
		// Full success — both tools are installed. Validate JSON shape.
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(respBody), &result); err != nil {
			t.Fatalf("failed to unmarshal success response: %v", err)
		}
		if result["success"] != true {
			t.Errorf("expected success=true, got %v", result["success"])
		}
		if _, ok := result["report_id"]; !ok {
			t.Error("expected report_id in response")
		}
		if _, ok := result["pdf_path"]; !ok {
			t.Error("expected pdf_path in response")
		}
		if _, ok := result["zip_path"]; !ok {
			t.Error("expected zip_path in response")
		}
		t.Log("Go backend produced full PDF successfully")
	} else {
		// Expected when rsvg-convert or xelatex are not installed.
		// Verify the error originates from the Go pipeline.
		if containsPythonMarker(respBody) {
			t.Errorf("expected Go pipeline error, but got Python-path marker: %s", respBody)
		}
		t.Logf("Go backend returned expected tool-missing error (status %d): %s", w.Code, respBody)
	}
}

// TestGenerateReport_InvalidTimezoneReturns400 verifies that validation
// failures from report.Generate (ErrInvalidConfig) are mapped to HTTP 400,
// not the 500 used for internal/tooling failures.
func TestGenerateReport_InvalidTimezoneReturns400(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	reqBody := ReportRequest{
		SiteID:    &site.ID,
		StartDate: "2025-12-03",
		EndDate:   "2025-12-03",
		Timezone:  "Not/A/Zone",
		Units:     "mph",
		Group:     "1h",
		Source:    "radar_objects",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid timezone, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGenerateReport_InvalidUnitsReturns400(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	reqBody := ReportRequest{
		SiteID:    &site.ID,
		StartDate: "2025-12-03",
		EndDate:   "2025-12-03",
		Timezone:  "UTC",
		Units:     "knots",
		Group:     "1h",
		Source:    "radar_objects",
	}

	w := postGenerateReportRequest(t, server, reqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid units, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid 'units'") {
		t.Fatalf("expected invalid units message, got: %s", w.Body.String())
	}
}

func TestGenerateReport_InvalidSourceReturns400(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	reqBody := ReportRequest{
		SiteID:    &site.ID,
		StartDate: "2025-12-03",
		EndDate:   "2025-12-03",
		Timezone:  "UTC",
		Units:     "mph",
		Group:     "1h",
		Source:    "bad_source",
	}

	w := postGenerateReportRequest(t, server, reqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid source, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid 'source'") {
		t.Fatalf("expected invalid source message, got: %s", w.Body.String())
	}
}

func TestGenerateReport_InvalidCompareSourceReturns400(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	reqBody := ReportRequest{
		SiteID:        &site.ID,
		StartDate:     "2025-12-03",
		EndDate:       "2025-12-03",
		CompareStart:  "2025-12-02",
		CompareEnd:    "2025-12-02",
		Timezone:      "UTC",
		Units:         "mph",
		Group:         "1h",
		Source:        "radar_objects",
		CompareSource: "bad_compare_source",
	}

	w := postGenerateReportRequest(t, server, reqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid compare source, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid 'compare_source'") {
		t.Fatalf("expected invalid compare source message, got: %s", w.Body.String())
	}
}

func TestReportCosineMetadataForRange(t *testing.T) {
	firstEnd := 100.0
	periods := []db.SiteConfigPeriod{
		{
			EffectiveStartUnix: 0,
			EffectiveEndUnix:   &firstEnd,
			CosineErrorAngle:   5,
		},
		{
			EffectiveStartUnix: 100,
			CosineErrorAngle:   15,
		},
	}

	single := reportCosineMetadataForRange(periods, 10, 90)
	if single.angle != 5 || single.label != "" {
		t.Fatalf("single-period metadata = %+v, want angle 5 and empty label", single)
	}

	multiple := reportCosineMetadataForRange(periods, 10, 110)
	if multiple.angle != 0 {
		t.Fatalf("multiple-period angle = %v, want 0", multiple.angle)
	}
	if multiple.label != "multiple periods: 5.0°, 15.0°" {
		t.Fatalf("multiple-period label = %q", multiple.label)
	}

	none := reportCosineMetadataForRange(periods, -100, -10)
	if none.angle != 0 || none.label != "" {
		t.Fatalf("non-overlapping metadata = %+v, want zero value", none)
	}
}

// TestGenerateReport_ConfigMapping verifies that the handler correctly maps
// ReportRequest fields through to the Go pipeline by confirming a request with
// comparison params and non-default units reaches the Go pipeline without panic.
func TestGenerateReport_ConfigMapping(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	// Test with comparison params to exercise the full config mapping.
	reqBody := ReportRequest{
		SiteID:            &site.ID,
		StartDate:         "2025-12-03",
		EndDate:           "2025-12-03",
		CompareStart:      "2025-12-03",
		CompareEnd:        "2025-12-03",
		Timezone:          "UTC",
		Units:             "kph",
		Group:             "4h",
		Source:            "radar_objects",
		MinSpeed:          10.0,
		BoundaryThreshold: 5,
		Histogram:         true,
		HistBucketSize:    5,
		HistMax:           110,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	respBody := w.Body.String()

	// Verify the Go path was taken (no Python markers).
	if containsPythonMarker(respBody) {
		t.Errorf("expected Go pipeline path but response contains Python marker: %s", respBody)
	}
}

// TestBuildReportConfig_FieldMapping is a pure unit test confirming that
// buildReportConfig maps ReportRequest fields to report.Config correctly.
func TestBuildReportConfig_FieldMapping(t *testing.T) {
	siteID := 42
	req := ReportRequest{
		SiteID:             &siteID,
		StartDate:          "2025-01-01",
		EndDate:            "2025-01-31",
		CompareStart:       "2024-01-01",
		CompareEnd:         "2024-01-31",
		Timezone:           "US/Pacific",
		Units:              "kph",
		Group:              "4h",
		Source:             "radar_data_transits",
		MinSpeed:           5.0,
		BoundaryThreshold:  3,
		Histogram:          true,
		HistBucketSize:     10.0,
		HistMax:            120.0,
		PaperSize:          "letter",
		ExpandedChart:      true,
		CompareCosineAngle: 7.5,
	}

	cfg := buildReportConfig(req, nil, 3.5, "Test Location", "Test Surveyor", "test@example.com", 30, "Test description")

	if cfg.CompareCosineAngle != 7.5 {
		t.Errorf("CompareCosineAngle: got %v, want 7.5", cfg.CompareCosineAngle)
	}
	if cfg.CosineAngle != 3.5 {
		t.Errorf("CosineAngle: got %v, want 3.5", cfg.CosineAngle)
	}
	if cfg.CompareStart != "2024-01-01" {
		t.Errorf("CompareStart: got %q, want %q", cfg.CompareStart, "2024-01-01")
	}
	if cfg.CompareEnd != "2024-01-31" {
		t.Errorf("CompareEnd: got %q, want %q", cfg.CompareEnd, "2024-01-31")
	}
	if cfg.PaperSize != "letter" {
		t.Errorf("PaperSize: got %q, want %q", cfg.PaperSize, "letter")
	}
	if !cfg.ExpandedChart {
		t.Errorf("ExpandedChart: got false, want true")
	}
	if cfg.Units != "kph" {
		t.Errorf("Units: got %q, want %q", cfg.Units, "kph")
	}
	if cfg.SiteID != 42 {
		t.Errorf("SiteID: got %d, want 42", cfg.SiteID)
	}
	if cfg.Location != "Test Location" {
		t.Errorf("Location: got %q, want %q", cfg.Location, "Test Location")
	}
}

// TestGenerateReport_NoPythonEnvNeeded confirms that a report request proceeds
// via the Go pipeline regardless of whether VELOCITY_PDF_BACKEND is set.
// The response must not contain Python error markers.
func TestGenerateReport_NoPythonEnvNeeded(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	// Explicitly clear any env var that might have leaked from another test.
	os.Unsetenv("VELOCITY_PDF_BACKEND")

	reqBody := ReportRequest{
		SiteID:         &site.ID,
		StartDate:      "2025-12-03",
		EndDate:        "2025-12-03",
		Timezone:       "UTC",
		Units:          "mph",
		Group:          "1h",
		Source:         "radar_objects",
		Histogram:      true,
		HistBucketSize: 5,
		HistMax:        70,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	respBody := w.Body.String()

	// Whether success or tool-missing failure, no Python markers should appear.
	if containsPythonMarker(respBody) {
		t.Errorf("response contains Python marker without VELOCITY_PDF_BACKEND set: %s", respBody)
	}

	// The result must be either HTTP 200 or a Go-pipeline error.
	if w.Code != http.StatusOK && !isGoPipelineError(respBody) {
		t.Errorf("unexpected non-200 response without a recognised Go-pipeline error: status=%d body=%s", w.Code, respBody)
	}
}

// containsPythonMarker checks if the response body contains markers that
// indicate the Python exec path was taken.
func containsPythonMarker(body string) bool {
	pythonMarkers := []string{
		"pdf_generator",
		"python3",
		"No module named",
	}
	for _, m := range pythonMarkers {
		if bytes.Contains([]byte(body), []byte(m)) {
			return true
		}
	}
	return false
}

// isGoPipelineError checks if the error message looks like it came from the
// Go report pipeline.
func isGoPipelineError(body string) bool {
	goMarkers := []string{
		"rsvg-convert",
		"xelatex",
		"unsupported group",
	}
	for _, m := range goMarkers {
		if bytes.Contains([]byte(body), []byte(m)) {
			return true
		}
	}
	return false
}

func postGenerateReportRequest(t *testing.T, server *Server, reqBody ReportRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.generateReport(w, req)
	return w
}

func TestRelativeReportPaths_Valid(t *testing.T) {
	root := filepath.Join(string(os.PathSeparator), "tmp", "pdf-generator")
	pdfPath := filepath.Join(root, "output", "run-1", "report.pdf")
	zipPath := filepath.Join(root, "output", "run-1", "report_sources.zip")

	pdfRel, zipRel, err := relativeReportPaths(root, pdfPath, zipPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if pdfRel != filepath.Join("output", "run-1", "report.pdf") {
		t.Fatalf("unexpected pdf rel path: %s", pdfRel)
	}
	if zipRel != filepath.Join("output", "run-1", "report_sources.zip") {
		t.Fatalf("unexpected zip rel path: %s", zipRel)
	}
}

func TestRelativeReportPaths_RejectEscape(t *testing.T) {
	root := filepath.Join(string(os.PathSeparator), "tmp", "pdf-generator")
	badPDF := filepath.Join(string(os.PathSeparator), "tmp", "outside", "report.pdf")
	zipPath := filepath.Join(root, "output", "run-1", "report_sources.zip")

	_, _, err := relativeReportPaths(root, badPDF, zipPath)
	if err == nil {
		t.Fatal("expected error for escaping pdf path")
	}
}

func createMockReportTools(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	rsvg := filepath.Join(binDir, "rsvg-convert")
	rsvgScript := `#!/bin/sh
output=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) output="$2"; shift 2 ;;
        *) shift ;;
    esac
done
if [ -n "$output" ]; then
    echo "%PDF-1.4 mock" > "$output"
fi
`
	if err := os.WriteFile(rsvg, []byte(rsvgScript), 0755); err != nil {
		t.Fatalf("write mock rsvg-convert: %v", err)
	}

	xelatex := filepath.Join(binDir, "xelatex")
	xelatexScript := `#!/bin/sh
texfile=""
for arg in "$@"; do
    case "$arg" in
        *.tex) texfile="$arg" ;;
    esac
done
if [ -n "$texfile" ]; then
    base=$(echo "$texfile" | sed 's/\.tex$//')
    echo "%PDF-1.4 mock xelatex output" > "${base}.pdf"
    echo "mock log" > "${base}.log"
    echo "mock aux" > "${base}.aux"
fi
`
	if err := os.WriteFile(xelatex, []byte(xelatexScript), 0755); err != nil {
		t.Fatalf("write mock xelatex: %v", err)
	}

	return binDir
}

// seedChartTestData is defined in server_charts_test.go and shared here.
