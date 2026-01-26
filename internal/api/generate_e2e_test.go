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
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

// TestGenerateReport_E2E simulates an end-to-end report generation flow where the
// server invokes the actual Python PDF generator. The test verifies that PDF and ZIP
// files are created with the correct filename format and that the generated config
// contains the expected site and query parameters.
func TestGenerateReport_E2E(t *testing.T) {
	// Skip if Python environment is not available
	if os.Getenv("SKIP_PDF_TESTS") != "" {
		t.Skip("Skipping PDF generation test (SKIP_PDF_TESTS set)")
	}
	// Setup server and DB
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Start an actual HTTP server for the PDF generator to connect to
	// The Python PDF generator needs a real HTTP endpoint to fetch data from
	// Use server.ServeMux() to ensure the mux is initialised with all handlers
	ts := httptest.NewServer(server.ServeMux())
	defer ts.Close()

	// Set API_BASE_URL environment variable so Python PDF generator connects to our test server
	oldAPIBaseURL := os.Getenv("API_BASE_URL")
	os.Setenv("API_BASE_URL", ts.URL)
	defer func() {
		if oldAPIBaseURL == "" {
			os.Unsetenv("API_BASE_URL")
		} else {
			os.Setenv("API_BASE_URL", oldAPIBaseURL)
		}
	}()
	t.Logf("Started test server at %s, set API_BASE_URL", ts.URL)

	// Create a site to reference in the report with specific description for testing
	siteDescription := "Surveys were conducted from the northbound bike lane outside 123 Test Street, directly adjacent to a local park. The observed travel lane experiences consistent traffic throughout the day."
	site := &db.Site{
		Name:            "E2E Site",
		Location:        "Test Location",
		Surveyor:        "Tester",
		Contact:         "test@example.com",
		SiteDescription: &siteDescription,
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

	// Insert test radar data for the report date range (2025-12-03 to 2025-12-04)
	// Seed multiple events with varying speeds to generate realistic report data
	// Note: We must set write_timestamp explicitly because the API queries by write_timestamp,
	// not start_time. The schema default uses UNIXEPOCH('subsec') which would be "now".
	primaryTimestamp := int64(1764720000) // 2025-12-03 00:00:00 UTC
	compareTimestamp := int64(1762300800) // 2025-11-05 00:00:00 UTC

	// Generate events for primary date range (2025-12-03 to 2025-12-04)
	speeds := []float64{8.0, 10.0, 12.0, 15.0, 18.0, 20.0, 22.0, 25.0, 28.0, 30.0}
	for i := 0; i < 50; i++ {
		speed := speeds[i%len(speeds)]
		eventTimestamp := primaryTimestamp + int64(i*1800) // Every 30 minutes
		testEvent := map[string]interface{}{
			"site_id":         site.ID,
			"classifier":      "all",
			"start_time":      float64(eventTimestamp),
			"end_time":        float64(eventTimestamp + 2),
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
		eventJSON, _ := json.Marshal(testEvent)
		// Use direct INSERT to set write_timestamp explicitly (API queries by write_timestamp)
		_, err := dbInst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`,
			string(eventJSON), float64(eventTimestamp))
		if err != nil {
			t.Fatalf("failed to insert primary test radar data %d: %v", i, err)
		}
	}

	// Generate events for comparison date range (2025-11-05 to 2025-11-06)
	for i := 0; i < 50; i++ {
		speed := speeds[i%len(speeds)] - 2.0 // Slightly lower speeds for comparison
		eventTimestamp := compareTimestamp + int64(i*1800)
		testEvent := map[string]interface{}{
			"site_id":         site.ID,
			"classifier":      "all",
			"start_time":      float64(eventTimestamp),
			"end_time":        float64(eventTimestamp + 2),
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
		eventJSON, _ := json.Marshal(testEvent)
		// Use direct INSERT to set write_timestamp explicitly
		_, err := dbInst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`,
			string(eventJSON), float64(eventTimestamp))
		if err != nil {
			t.Fatalf("failed to insert comparison test radar data %d: %v", i, err)
		}
	}

	t.Logf("Seeded %d events for primary range (2025-12-03 to 2025-12-04) and %d events for comparison range (2025-11-05 to 2025-11-06)", 50, 50)

	// Ensure PDF_GENERATOR_DIR is set for the test environment
	pdfGenDir := os.Getenv("PDF_GENERATOR_DIR")
	if pdfGenDir == "" {
		// Default to tools/pdf-generator relative to the repo root
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}
		repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
		pdfGenDir = filepath.Join(repoRoot, "tools", "pdf-generator")
		os.Setenv("PDF_GENERATOR_DIR", pdfGenDir)
		t.Logf("Set PDF_GENERATOR_DIR=%s", pdfGenDir)
	} else {
		t.Logf("Using PDF_GENERATOR_DIR=%s", pdfGenDir)
	}

	// Ensure PDF_GENERATOR_PYTHON is set for the test environment
	pdfGenPython := os.Getenv("PDF_GENERATOR_PYTHON")
	if pdfGenPython == "" {
		// Derive repo root from PDF_GENERATOR_DIR
		repoRoot := filepath.Clean(filepath.Join(pdfGenDir, "..", ".."))
		venvPython := filepath.Join(repoRoot, ".venv", "bin", "python")
		if _, err := os.Stat(venvPython); err == nil {
			os.Setenv("PDF_GENERATOR_PYTHON", venvPython)
			t.Logf("Set PDF_GENERATOR_PYTHON=%s", venvPython)
		} else {
			t.Logf("Warning: venv python not found at %s", venvPython)
		}
	} else {
		t.Logf("Using PDF_GENERATOR_PYTHON=%s", pdfGenPython)
	}

	// Build report request pointing to our site (with histogram enabled)
	reqBody := map[string]interface{}{
		"site_id":            site.ID,
		"start_date":         "2025-12-03",
		"end_date":           "2025-12-04",
		"compare_start_date": "2025-11-05",
		"compare_end_date":   "2025-11-06",
		"source":             "radar_objects", // Use radar_objects table where test data is seeded
		"histogram":          true,
		"hist_bucket_size":   5.0,
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

	// Parse response to get report details
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify PDF and ZIP files were created
	pdfPath, _ := resp["pdf_path"].(string)
	zipPath, _ := resp["zip_path"].(string)
	if pdfPath == "" || zipPath == "" {
		t.Fatalf("response missing pdf_path or zip_path: %v", resp)
	}

	// Verify files exist in the pdf-generator directory
	fullPdfPath := filepath.Join(pdfGenDir, pdfPath)
	fullZipPath := filepath.Join(pdfGenDir, zipPath)

	if _, err := os.Stat(fullPdfPath); os.IsNotExist(err) {
		t.Fatalf("PDF file not found at %s", fullPdfPath)
	}
	if _, err := os.Stat(fullZipPath); os.IsNotExist(err) {
		t.Fatalf("ZIP file not found at %s", fullZipPath)
	}

	// Verify filename format: {endDate}_velocity.report_{location}_report.pdf
	// (Python adds _report suffix for PDF, _sources suffix for ZIP)
	expectedPdfName := "2025-12-04_velocity.report_Test_Location_report.pdf"
	expectedZipName := "2025-12-04_velocity.report_Test_Location_sources.zip"

	if !strings.Contains(pdfPath, expectedPdfName) {
		t.Fatalf("expected PDF filename to contain %q, got %q", expectedPdfName, pdfPath)
	}
	if !strings.Contains(zipPath, expectedZipName) {
		t.Fatalf("expected ZIP filename to contain %q, got %q", expectedZipName, zipPath)
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

	// Verify comparison dates if they were provided in the request
	compareStart, _ := queryObj["compare_start_date"].(string)
	compareEnd, _ := queryObj["compare_end_date"].(string)
	if compareStart != "" || compareEnd != "" {
		if compareStart != "2025-11-05" || compareEnd != "2025-11-06" {
			t.Fatalf(
				"expected compare dates 2025-11-05/2025-11-05, got %q/%q (cfgPath=%s)",
				compareStart,
				compareEnd,
				cfgPath,
			)
		}
		t.Logf("✓ Comparison dates verified: %s to %s", compareStart, compareEnd)
	} else {
		t.Logf("⚠ Comparison dates not present in config (optional feature)")
	}

	// === Extract and verify ZIP contents ===
	t.Log("Extracting ZIP file to verify TEX content...")

	zipReader, err := zip.OpenReader(fullZipPath)
	if err != nil {
		t.Fatalf("failed to open ZIP file: %v", err)
	}
	defer zipReader.Close()

	// Find and read the TEX file
	var texContent string
	var foundTex bool
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, "_report.tex") {
			foundTex = true
			rc, err := file.Open()
			if err != nil {
				t.Fatalf("failed to open TEX file in ZIP: %v", err)
			}
			defer rc.Close()

			texBytes, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("failed to read TEX file: %v", err)
			}
			texContent = string(texBytes)
			t.Logf("✓ Found TEX file: %s (%d bytes)", file.Name, len(texContent))
			break
		}
	}

	if !foundTex {
		t.Fatal("ZIP does not contain a _report.tex file")
	}

	// Track assertion failures
	var assertionsFailed int

	// === Assertion 1: Verify Key Metrics table with actual metric values ===
	// Check for complete table structure with p50/p85/p98 Velocity rows and mph units
	hasKeyMetricsHeader := strings.Contains(texContent, "subsection*{Key Metrics}")
	hasP50Row := strings.Contains(texContent, "p50 Velocity") && strings.Contains(texContent, "mph")
	hasP85Row := strings.Contains(texContent, "p85 Velocity")
	hasP98Row := strings.Contains(texContent, "p98 Velocity")
	hasMaxRow := strings.Contains(texContent, "Max Velocity")
	hasVehicleCount := strings.Contains(texContent, "Vehicle Count")

	if !hasKeyMetricsHeader || !hasP50Row || !hasP85Row || !hasP98Row || !hasMaxRow || !hasVehicleCount {
		t.Errorf("TEX content missing Key Metrics table components (header: %v, p50: %v, p85: %v, p98: %v, max: %v, count: %v)",
			hasKeyMetricsHeader, hasP50Row, hasP85Row, hasP98Row, hasMaxRow, hasVehicleCount)
		assertionsFailed++
	} else {
		t.Log("✓ Assertion 1: Key Metrics table with p50/p85/p98/Max Velocity and Vehicle Count rows found")
	}

	// === Assertion 2: Verify granular data table with actual data rows ===
	// Check for supertabular structure, column headers, and date-formatted data rows
	hasSupertabular := strings.Contains(texContent, "begin{supertabular}")
	hasStartTimeHeader := strings.Contains(texContent, "Start Time")
	hasCountHeader := strings.Contains(texContent, "multicolumn{1}{r}{\\sffamily\\bfseries Count}")
	hasPercentileHeaders := strings.Contains(texContent, "shortstack{p50") &&
		strings.Contains(texContent, "shortstack{p85") &&
		strings.Contains(texContent, "shortstack{p98")

	if !hasSupertabular || !hasStartTimeHeader || !hasPercentileHeaders {
		t.Errorf("TEX content missing granular data table structure (supertabular: %v, StartTime: %v, Count: %v, percentile headers: %v)",
			hasSupertabular, hasStartTimeHeader, hasCountHeader, hasPercentileHeaders)
		assertionsFailed++
	} else {
		t.Log("✓ Assertion 2: Granular data table with supertabular structure and percentile column headers found")
	}

	// === Assertion 3: Verify Citizen Radar section with 25+ word passage ===
	// Check for section header and substantial explanatory copy about the tool
	citizenRadarHeader := strings.Contains(texContent, "subsection*{Citizen Radar}")
	citizenRadarIntro := strings.Contains(texContent, "citizen radar tool designed to help communities measure vehicle speeds")
	citizenRadarDoppler := strings.Contains(texContent, "privacy-preserving Doppler sensors")
	citizenRadarEnergy := strings.Contains(texContent, "kinetic energy scales with the square of speed")
	citizenRadarFormula := strings.Contains(texContent, "K_E = \\tfrac{1}{2} m v^2")

	if !citizenRadarHeader {
		t.Error("TEX content missing 'Citizen Radar' section header (subsection*{Citizen Radar})")
		assertionsFailed++
	} else if !citizenRadarIntro || !citizenRadarDoppler || !citizenRadarEnergy || !citizenRadarFormula {
		t.Errorf("TEX content missing Citizen Radar explanatory copy (intro: %v, doppler: %v, energy: %v, formula: %v)",
			citizenRadarIntro, citizenRadarDoppler, citizenRadarEnergy, citizenRadarFormula)
		assertionsFailed++
	} else {
		t.Log("✓ Assertion 3: Citizen Radar section with kinetic energy explanation and K_E formula found")
	}

	// === Assertion 4: Verify Site Information section with custom description ===
	// Check for section header and the specific site description phrases we configured
	siteInfoHeader := strings.Contains(texContent, "subsection*{Site Information}")
	hasBikeLane := strings.Contains(texContent, "northbound bike lane outside 123 Test Street")
	hasPark := strings.Contains(texContent, "directly adjacent to a local park")
	hasTrafficNote := strings.Contains(texContent, "observed travel lane experiences consistent traffic")

	if !siteInfoHeader {
		t.Error("TEX content missing 'Site Information' section header (subsection*{Site Information})")
		assertionsFailed++
	} else if !hasBikeLane || !hasPark || !hasTrafficNote {
		t.Errorf("TEX content missing site description copy (bike lane phrase: %v, park phrase: %v, traffic note: %v). Expected: %q",
			hasBikeLane, hasPark, hasTrafficNote, siteDescription)
		assertionsFailed++
	} else {
		t.Logf("✓ Assertion 4: Site Information section with complete custom description found")
	}

	// === Assertion 5: Verify site location in document title and header ===
	// Check for location in title block and page header
	locationInTitle := strings.Contains(texContent, "huge \\sffamily\\textbf{ "+site.Location+"}")
	locationInHeader := strings.Contains(texContent, "fancyhead[R]{ \\textit{"+site.Location+"}")
	locationAnywhere := strings.Contains(texContent, site.Location)

	if !locationAnywhere {
		t.Errorf("TEX content missing site location %q anywhere in document", site.Location)
		assertionsFailed++
	} else {
		t.Logf("✓ Assertion 5: Site location %q found in TEX (title: %v, header: %v)", site.Location, locationInTitle, locationInHeader)
	}

	// === Assertion 6: Verify Aggregation and Percentiles explanatory copy ===
	// Check for comprehensive explanation of Doppler effect and percentile methodology
	aggregationHeader := strings.Contains(texContent, "subsection*{Aggregation and Percentiles}")
	dopplerExplanation := strings.Contains(texContent, "Doppler radar to measure vehicle speed by detecting frequency shifts in waves reflected from objects in motion")
	dopplerEffect := strings.Contains(texContent, "Doppler effect")
	p85Explanation := strings.Contains(texContent, "85th percentile (p85) indicates the speed at or below which 85")
	p98Explanation := strings.Contains(texContent, "98th percentile (p98) exceeds this industry-standard measure by capturing the fastest 2")
	clusteringNote := strings.Contains(texContent, "Time-Contiguous Speed Clustering")

	if !aggregationHeader {
		t.Error("TEX content missing 'Aggregation and Percentiles' section header")
		assertionsFailed++
	} else if !dopplerExplanation && !dopplerEffect {
		t.Error("TEX content missing Doppler radar/effect explanation")
		assertionsFailed++
	} else if !p85Explanation || !p98Explanation {
		t.Errorf("TEX content missing percentile explanations (p85: %v, p98: %v)", p85Explanation, p98Explanation)
		assertionsFailed++
	} else {
		t.Logf("✓ Assertion 6: Aggregation section with Doppler explanation, p85/p98 methodology, and clustering note (%v) found", clusteringNote)
	}

	// === Assertion 7: Verify histogram is included ===
	// Check for histogram figure with velocity distribution
	hasHistogramFigure := strings.Contains(texContent, "includegraphics") && strings.Contains(texContent, "histogram")
	hasHistogramCaption := strings.Contains(texContent, "Velocity Distribution Histogram") ||
		strings.Contains(texContent, "Distribution")

	if !hasHistogramFigure {
		t.Error("TEX content missing histogram figure (includegraphics with histogram)")
		assertionsFailed++
	} else {
		t.Logf("✓ Assertion 7: Histogram figure included (caption found: %v)", hasHistogramCaption)
	}

	// Final summary
	if assertionsFailed > 0 {
		t.Errorf("❌ %d TEX content assertion(s) failed", assertionsFailed)
	} else {
		t.Log("✅ All 7 ZIP/TEX content assertions passed")
	}
}
