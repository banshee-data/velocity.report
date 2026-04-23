package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/report"
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

func TestGenerateReport_BothBackend_IncludesComparisonTeX(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := seedChartTestData(t, dbInst)
	pdfDir := t.TempDir()
	t.Setenv("PDF_GENERATOR_DIR", pdfDir)
	t.Setenv("VELOCITY_PDF_BACKEND", "both")
	t.Setenv("PATH", createMockReportTools(t)+":"+os.Getenv("PATH"))

	originalRunner := runPythonPDFGenerator
	runPythonPDFGenerator = func(pythonBin, pdfDir, configFile string) ([]byte, error) {
		var config struct {
			Query struct {
				EndDate string `json:"end_date"`
			} `json:"query"`
			Site struct {
				Location string `json:"location"`
			} `json:"site"`
			Output struct {
				OutputDir string `json:"output_dir"`
			} `json:"output"`
		}

		configData, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(configData, &config); err != nil {
			return nil, err
		}

		artifacts := buildPythonReportArtifacts(pdfDir, config.Output.OutputDir, config.Query.EndDate, config.Site.Location)
		zipData, err := report.BuildZip(map[string][]byte{
			"stub_report.tex": []byte("python comparison tex"),
		})
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(artifacts.FullZIPPath), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(artifacts.FullZIPPath, zipData, 0644); err != nil {
			return nil, err
		}

		return []byte("python comparison ok"), nil
	}
	defer func() {
		runPythonPDFGenerator = originalRunner
	}()

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
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	zipPath := filepath.Join(pdfDir, response["zip_path"].(string))
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	entries := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		entries[f.Name] = string(data)
	}

	if entries["comparison/go/report.tex"] == "" {
		t.Fatal("comparison/go/report.tex not found in ZIP")
	}
	if entries["comparison/python/report.tex"] != "python comparison tex" {
		t.Fatalf("comparison/python/report.tex = %q, want %q", entries["comparison/python/report.tex"], "python comparison tex")
	}
	if entries["comparison/go/report.tex"] != entries["report.tex"] {
		t.Fatal("comparison/go/report.tex did not match root report.tex")
	}
	if _, err := os.Stat(filepath.Join(pdfDir, "output", filepath.Base(filepath.Dir(zipPath)), "python-compare")); !os.IsNotExist(err) {
		t.Fatal("expected python-compare temp directory to be removed")
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
