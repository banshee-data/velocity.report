package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateReport_GoBackend_RequiresTools tests the Go PDF backend path.
// It verifies the handler enters the Go branch when VELOCITY_PDF_BACKEND=go
// and exercises the config mapping logic. If rsvg-convert or xelatex are not
// installed the test still validates the error comes from the Go pipeline
// (not the Python exec path).
func TestGenerateReport_GoBackend_RequiresTools(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	// Set the feature flag.
	os.Setenv("VELOCITY_PDF_BACKEND", "go")
	defer os.Unsetenv("VELOCITY_PDF_BACKEND")

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

	// The handler should NOT invoke the Python path. If rsvg-convert/xelatex
	// are missing the error message will mention those tools — not "python"
	// or "PDF generation failed" from the Python subprocess exec.
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
		// Verify the error originates from the Go pipeline, not the Python path.
		if isPythonError(respBody) {
			t.Errorf("expected Go pipeline error, but got Python-path error: %s", respBody)
		}
		t.Logf("Go backend returned expected tool-missing error (status %d): %s", w.Code, respBody)
	}
}

// TestGenerateReport_PythonPath_WhenFlagUnset verifies the Python path is used
// when the VELOCITY_PDF_BACKEND env var is absent. The test expects a Python
// error (no python env in CI) — the important thing is the error is from the
// Python exec path, not the Go pipeline.
func TestGenerateReport_PythonPath_WhenFlagUnset(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	// Ensure feature flag is NOT set.
	os.Unsetenv("VELOCITY_PDF_BACKEND")

	reqBody := ReportRequest{
		SiteID:    &site.ID,
		StartDate: "2025-12-03",
		EndDate:   "2025-12-03",
		Timezone:  "UTC",
		Units:     "mph",
		Group:     "1h",
		Source:    "radar_objects",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	// Without a Python env the handler should fail. The error should NOT
	// mention rsvg-convert or xelatex (those are Go-pipeline errors).
	if w.Code == http.StatusOK {
		t.Log("Python PDF generation succeeded (full environment available)")
		return
	}

	respBody := w.Body.String()
	if isGoPipelineError(respBody) {
		t.Errorf("expected Python-path error, but got Go-pipeline error: %s", respBody)
	}
}

// isPythonError checks if the error message looks like it came from the
// Python exec path.
func isPythonError(body string) bool {
	pythonMarkers := []string{
		"pdf_generator",
		"python3",
		"python",
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

// TestGenerateReport_GoBackend_ConfigMapping verifies that the Go backend
// correctly maps ReportRequest fields to report.Config fields by checking
// the handler function can be invoked without panic.
func TestGenerateReport_GoBackend_ConfigMapping(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)

	os.Setenv("VELOCITY_PDF_BACKEND", "go")
	defer os.Unsetenv("VELOCITY_PDF_BACKEND")

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

	// This exercises config construction. We don't need full success —
	// just no panic and evidence the Go path was taken.
	server.ServeMux().ServeHTTP(w, req)

	respBody := w.Body.String()

	// Verify it hit the Go path (not Python).
	if isPythonError(respBody) {
		t.Errorf("expected Go pipeline path but got Python error: %s", respBody)
	}
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

// seedChartTestData is defined in server_charts_test.go and shared here.
