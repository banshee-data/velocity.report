package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

// TestGenerateReport_UsesTypst tests the Go PDF report pipeline with a mocked
// Typst binary so no host-level PDF tooling is required.
func TestGenerateReport_UsesTypst(t *testing.T) {
	t.Setenv(typstbin.EnvPath, createMockTypstBinary(t))
	t.Setenv(reportOutputDirEnv, t.TempDir())
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected report generation success, got status %d: %s", w.Code, respBody)
	}
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
	assertGeneratedPDFMetadata(t, result)
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

// Regression for the "compare PDF report not accepting a second time
// period" report: previously, a future-dated comparison range returned a
// PDF with all zeros and -100% deltas, which looked like the comparison
// flow was broken.  We now reject the request with a clear error.
func TestGenerateReport_FutureCompareRangeRejected(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	dayAfter := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02")

	reqBody := ReportRequest{
		SiteID:        &site.ID,
		StartDate:     "2025-12-03",
		EndDate:       "2025-12-03",
		CompareStart:  tomorrow,
		CompareEnd:    dayAfter,
		Timezone:      "UTC",
		Units:         "mph",
		Group:         "1h",
		Source:        "radar_objects",
		CompareSource: "radar_objects",
	}

	w := postGenerateReportRequest(t, server, reqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for future comparison range, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "is in the future") {
		t.Fatalf("expected future-range message, got: %s", w.Body.String())
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
	t.Setenv(typstbin.EnvPath, createMockTypstBinary(t))
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

// TestGenerateReport_UsesNativePipeline confirms that report requests proceed
// through the native Typst pipeline without Python PDF hooks.
func TestGenerateReport_UsesNativePipeline(t *testing.T) {
	t.Setenv(typstbin.EnvPath, createMockTypstBinary(t))
	t.Setenv(reportOutputDirEnv, t.TempDir())
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

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

	if containsPythonMarker(respBody) {
		t.Errorf("response contains Python marker: %s", respBody)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected report generation success, got status %d: %s", w.Code, respBody)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(respBody), &result); err != nil {
		t.Fatalf("failed to unmarshal success response: %v", err)
	}
	assertGeneratedPDFMetadata(t, result)
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
	root := filepath.Join(string(os.PathSeparator), "tmp", "reports")
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
	root := filepath.Join(string(os.PathSeparator), "tmp", "reports")
	badPDF := filepath.Join(string(os.PathSeparator), "tmp", "outside", "report.pdf")
	zipPath := filepath.Join(root, "output", "run-1", "report_sources.zip")

	_, _, err := relativeReportPaths(root, badPDF, zipPath)
	if err == nil {
		t.Fatal("expected error for escaping pdf path")
	}
}

func createMockTypstBinary(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "mock.pdf")
	if err := os.WriteFile(pdfPath, minimalMockPDF(), 0o644); err != nil {
		t.Fatalf("write mock pdf: %v", err)
	}

	path := filepath.Join(dir, "typst")
	body := "#!/bin/sh\n" +
		"cat > /dev/null\n" +
		"cat " + shellSingleQuote(pdfPath) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write mock typst: %v", err)
	}

	return path
}

func minimalMockPDF() []byte {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, obj := range objects {
		offsets[index+1] = pdf.Len()
		pdf.WriteString(obj)
	}
	xrefStart := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	fmt.Fprintf(&pdf, "%010d %05d f \n", 0, 65535)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&pdf, "%010d %05d n \n", offsets[index], 0)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefStart)
	return pdf.Bytes()
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func assertGeneratedPDFMetadata(t *testing.T, result map[string]interface{}) {
	t.Helper()

	pdfPath, ok := result["pdf_path"].(string)
	if !ok || pdfPath == "" {
		t.Fatalf("expected string pdf_path in response, got %v", result["pdf_path"])
	}
	pdf, err := os.ReadFile(filepath.Join(os.Getenv(reportOutputDirEnv), pdfPath))
	if err != nil {
		t.Fatalf("read generated pdf: %v", err)
	}
	for _, want := range []string{
		"%PDF-1.4",
		"/Creator (velocity.report",
		"/Prev ",
		"startxref",
	} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Fatalf("generated pdf missing %q", want)
		}
	}
}

// seedChartTestData is defined in server_charts_test.go and shared here.
