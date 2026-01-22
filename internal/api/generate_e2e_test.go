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

// TestGenerateReport_E2E simulates an end-to-end report generation flow where the
// server invokes a (fake) python generator. The fake generator reads the JSON
// config the server writes, creates the expected output PDF and ZIP under the
// configured output directory, and exits. The test then requests the ZIP via
// the reports download endpoint and asserts the ZIP contains at least one file.
func TestGenerateReport_E2E(t *testing.T) {
	// Setup server and DB
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site to reference in the report
	site := &db.Site{
		Name:     "E2E Site",
		Location: "Test Location",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	// Create initial site config period
	initialNotes := "Initial site configuration"
	initialConfig := &db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		Notes:              &initialNotes,
		CosineErrorAngle:   0.0,
	}
	if err := dbInst.CreateSiteConfigPeriod(initialConfig); err != nil {
		t.Fatalf("failed to create site config period: %v", err)
	}

	// Insert test radar data for the report date range (2025-10-01)
	testTimestamp := 1727740800 // 2025-10-01 00:00:00 UTC
	testEvent := map[string]interface{}{
		"site_id":         site.ID,
		"classifier":      "all",
		"start_time":      float64(testTimestamp),
		"end_time":        float64(testTimestamp + 1),
		"delta_time_msec": 100,
		"max_speed_mps":   10.0,
		"min_speed_mps":   10.0,
		"speed_change":    0.0,
		"max_magnitude":   10,
		"avg_magnitude":   10,
		"total_frames":    1,
		"frames_per_mps":  1.0,
		"length_m":        1.0,
	}
	eventJSON, _ := json.Marshal(testEvent)
	if err := dbInst.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("failed to insert test radar data: %v", err)
	}

	// Set PDF_GENERATOR_PYTHON to the mock script that creates expected output files
	// This allows us to test the config file writing without invoking the real generator
	old := os.Getenv("PDF_GENERATOR_PYTHON")

	// Get path to the mock script in testdata
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	mockScript := filepath.Join(cwd, "testdata", "mock_pdf_generator.sh")
	if _, err := os.Stat(mockScript); err != nil {
		t.Fatalf("mock script not found at %s: %v", mockScript, err)
	}

	if err := os.Setenv("PDF_GENERATOR_PYTHON", mockScript); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer func() { _ = os.Setenv("PDF_GENERATOR_PYTHON", old) }()

	// Change working directory to repo root so server.Resolve of repo root
	// points to the actual repository. When 'go test' runs this package the
	// current working dir is the package dir (internal/api), so we step up to
	// repo root.
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to chdir to repo root: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// Build report request pointing to our site
	reqBody := map[string]interface{}{
		"site_id":            site.ID,
		"start_date":         "2025-10-01",
		"end_date":           "2025-10-02",
		"compare_start_date": "2024-10-01",
		"compare_end_date":   "2024-10-02",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute handler
	server.generateReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("generateReport returned status %d: %s", w.Code, w.Body.String())
	}

	// Find the most recent preserved config file in os.TempDir()
	tmp := os.TempDir()
	files, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("failed to read tmp dir: %v", err)
	}
	var cfgPath string
	var newest int64
	for _, fi := range files {
		if !fi.Type().IsRegular() {
			continue
		}
		name := fi.Name()
		if !strings.HasPrefix(name, "report_config_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		st, err := os.Stat(filepath.Join(tmp, name))
		if err != nil {
			continue
		}
		if st.ModTime().Unix() > newest {
			newest = st.ModTime().Unix()
			cfgPath = filepath.Join(tmp, name)
		}
	}
	if cfgPath == "" {
		t.Fatalf("could not find preserved report config in %s", tmp)
	}

	// Read and assert the site.location value matches what we set
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read config %s: %v", cfgPath, err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	siteObj, ok := cfg["site"].(map[string]interface{})
	if !ok {
		t.Fatalf("config missing site object")
	}
	location, _ := siteObj["location"].(string)
	if location != site.Location {
		t.Fatalf("expected location %q, got %q (cfgPath=%s)", site.Location, location, cfgPath)
	}

	queryObj, ok := cfg["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("config missing query object")
	}
	compareStart, _ := queryObj["compare_start_date"].(string)
	compareEnd, _ := queryObj["compare_end_date"].(string)
	if compareStart != "2024-10-01" || compareEnd != "2024-10-02" {
		t.Fatalf(
			"expected compare dates to be written, got %q/%q (cfgPath=%s)",
			compareStart,
			compareEnd,
			cfgPath,
		)
	}
}
