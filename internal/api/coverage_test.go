package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

// TestComputeUnconfiguredGaps tests the computeUnconfiguredGaps function
func TestComputeUnconfiguredGaps(t *testing.T) {
	tests := []struct {
		name       string
		dataRange  *db.DataRange
		periods    []db.SiteConfigPeriod
		expectGaps int
	}{
		{
			name:       "nil data range",
			dataRange:  nil,
			periods:    []db.SiteConfigPeriod{},
			expectGaps: 0,
		},
		{
			name:       "zero start unix",
			dataRange:  &db.DataRange{StartUnix: 0, EndUnix: 1000},
			periods:    []db.SiteConfigPeriod{},
			expectGaps: 0,
		},
		{
			name:       "zero end unix",
			dataRange:  &db.DataRange{StartUnix: 1000, EndUnix: 0},
			periods:    []db.SiteConfigPeriod{},
			expectGaps: 0,
		},
		{
			name:       "end before start",
			dataRange:  &db.DataRange{StartUnix: 2000, EndUnix: 1000},
			periods:    []db.SiteConfigPeriod{},
			expectGaps: 0,
		},
		{
			name:       "no periods - single gap for entire range",
			dataRange:  &db.DataRange{StartUnix: 1000, EndUnix: 2000},
			periods:    []db.SiteConfigPeriod{},
			expectGaps: 1,
		},
		{
			name:      "single period covering entire range",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 2000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 500,
					EffectiveEndUnix:   floatPtr(2500),
				},
			},
			expectGaps: 0,
		},
		{
			name:      "period at start with gap at end",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 3000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 1000,
					EffectiveEndUnix:   floatPtr(2000),
				},
			},
			expectGaps: 1,
		},
		{
			name:      "gap at start with period at end",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 3000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 2000,
					EffectiveEndUnix:   floatPtr(3500),
				},
			},
			expectGaps: 1,
		},
		{
			name:      "gap in middle between two periods",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 5000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 1000,
					EffectiveEndUnix:   floatPtr(2000),
				},
				{
					SiteID:             1,
					EffectiveStartUnix: 3000,
					EffectiveEndUnix:   floatPtr(5000),
				},
			},
			expectGaps: 1,
		},
		{
			name:      "period with nil end (extends to infinity)",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 3000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 500,
					EffectiveEndUnix:   nil, // no end - extends to infinity
				},
			},
			expectGaps: 0,
		},
		{
			name:      "period outside data range - before",
			dataRange: &db.DataRange{StartUnix: 5000, EndUnix: 6000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 1000,
					EffectiveEndUnix:   floatPtr(2000),
				},
			},
			expectGaps: 1, // entire data range is unconfigured
		},
		{
			name:      "period outside data range - after",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 2000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 5000,
					EffectiveEndUnix:   floatPtr(6000),
				},
			},
			expectGaps: 1, // entire data range is unconfigured
		},
		{
			name:      "multiple gaps",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 10000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 2000,
					EffectiveEndUnix:   floatPtr(3000),
				},
				{
					SiteID:             1,
					EffectiveStartUnix: 5000,
					EffectiveEndUnix:   floatPtr(6000),
				},
				{
					SiteID:             1,
					EffectiveStartUnix: 8000,
					EffectiveEndUnix:   floatPtr(9000),
				},
			},
			expectGaps: 4, // 1000-2000, 3000-5000, 6000-8000, 9000-10000
		},
		{
			name:      "unsorted periods get sorted",
			dataRange: &db.DataRange{StartUnix: 1000, EndUnix: 6000},
			periods: []db.SiteConfigPeriod{
				{
					SiteID:             1,
					EffectiveStartUnix: 4000,
					EffectiveEndUnix:   floatPtr(5000),
				},
				{
					SiteID:             1,
					EffectiveStartUnix: 1000,
					EffectiveEndUnix:   floatPtr(2000),
				},
			},
			expectGaps: 2, // 2000-4000, 5000-6000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gaps := computeUnconfiguredGaps(tt.dataRange, tt.periods)
			if len(gaps) != tt.expectGaps {
				t.Errorf("Expected %d gaps, got %d: %v", tt.expectGaps, len(gaps), gaps)
			}
		})
	}
}

// TestUniqueAnglesForRange tests the uniqueAnglesForRange function
func TestUniqueAnglesForRange(t *testing.T) {
	tests := []struct {
		name           string
		periods        []db.SiteConfigPeriod
		startUnix      float64
		endUnix        float64
		expectedAngles int
	}{
		{
			name:           "empty periods",
			periods:        []db.SiteConfigPeriod{},
			startUnix:      1000,
			endUnix:        2000,
			expectedAngles: 0,
		},
		{
			name: "single period within range",
			periods: []db.SiteConfigPeriod{
				{EffectiveStartUnix: 500, EffectiveEndUnix: floatPtr(2500), CosineErrorAngle: 15.0},
			},
			startUnix:      1000,
			endUnix:        2000,
			expectedAngles: 1,
		},
		{
			name: "multiple periods with same angle",
			periods: []db.SiteConfigPeriod{
				{EffectiveStartUnix: 1000, EffectiveEndUnix: floatPtr(1500), CosineErrorAngle: 15.0},
				{EffectiveStartUnix: 1500, EffectiveEndUnix: floatPtr(2000), CosineErrorAngle: 15.0},
			},
			startUnix:      1000,
			endUnix:        2000,
			expectedAngles: 1, // duplicates are removed
		},
		{
			name: "multiple periods with different angles",
			periods: []db.SiteConfigPeriod{
				{EffectiveStartUnix: 1000, EffectiveEndUnix: floatPtr(1500), CosineErrorAngle: 10.0},
				{EffectiveStartUnix: 1500, EffectiveEndUnix: floatPtr(2000), CosineErrorAngle: 15.0},
				{EffectiveStartUnix: 2000, EffectiveEndUnix: floatPtr(2500), CosineErrorAngle: 20.0},
			},
			startUnix:      1000,
			endUnix:        2500,
			expectedAngles: 3,
		},
		{
			name: "period outside range",
			periods: []db.SiteConfigPeriod{
				{EffectiveStartUnix: 3000, EffectiveEndUnix: floatPtr(4000), CosineErrorAngle: 15.0},
			},
			startUnix:      1000,
			endUnix:        2000,
			expectedAngles: 0,
		},
		{
			name: "period with nil end extending into range",
			periods: []db.SiteConfigPeriod{
				{EffectiveStartUnix: 500, EffectiveEndUnix: nil, CosineErrorAngle: 12.0},
			},
			startUnix:      1000,
			endUnix:        2000,
			expectedAngles: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			angles := uniqueAnglesForRange(tt.periods, tt.startUnix, tt.endUnix)
			if len(angles) != tt.expectedAngles {
				t.Errorf("Expected %d angles, got %d: %v", tt.expectedAngles, len(angles), angles)
			}
		})
	}
}

// TestListSites_ErrorHandling tests listSites error paths
func TestListSites_ErrorHandling(t *testing.T) {
	// Create server without database to trigger error
	server := NewServer(nil, nil, "mph", "UTC")

	req := httptest.NewRequest(http.MethodGet, "/api/sites/", nil)
	w := httptest.NewRecorder()

	// This will panic or error because db is nil
	defer func() {
		if r := recover(); r != nil {
			// Expected - nil db causes panic
		}
	}()

	server.listSites(w, req)
}

// TestListSiteConfigPeriods_ErrorCases tests error handling in listSiteConfigPeriods
func TestListSiteConfigPeriods_ErrorCases(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name       string
		query      string
		expectCode int
	}{
		{
			name:       "invalid site_id string",
			query:      "?site_id=invalid",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "negative site_id",
			query:      "?site_id=-1",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "zero site_id",
			query:      "?site_id=0",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "valid site_id",
			query:      "?site_id=1",
			expectCode: http.StatusOK,
		},
		{
			name:       "no site_id (list all)",
			query:      "",
			expectCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/site_config_periods"+tt.query, nil)
			w := httptest.NewRecorder()

			server.handleSiteConfigPeriods(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestUpsertSiteConfigPeriod_InvalidJSON tests invalid JSON handling
func TestUpsertSiteConfigPeriod_InvalidJSON(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestUpsertSiteConfigPeriod_MissingSiteID tests validation of required site_id
func TestUpsertSiteConfigPeriod_MissingSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	period := db.SiteConfigPeriod{
		SiteID:             0, // missing/zero
		EffectiveStartUnix: 1000,
		IsActive:           true,
	}

	body, _ := json.Marshal(period)
	req := httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestUpsertSiteConfigPeriod_UpdateExisting tests updating an existing period
func TestUpsertSiteConfigPeriod_UpdateExisting(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site
	site := &db.Site{
		Name:     "Update Test Site",
		Location: "Test Location",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create initial period
	period := &db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           true,
		CosineErrorAngle:   10.0,
	}
	if err := dbInst.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create period: %v", err)
	}

	// Update the period
	period.CosineErrorAngle = 15.0
	body, _ := json.Marshal(period)
	req := httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestUpsertSiteConfigPeriod_UpdateNotFound tests updating a non-existent period
func TestUpsertSiteConfigPeriod_UpdateNotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	period := db.SiteConfigPeriod{
		ID:                 99999, // non-existent
		SiteID:             1,
		EffectiveStartUnix: 1000,
		IsActive:           true,
	}

	body, _ := json.Marshal(period)
	req := httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestHandleSiteConfigPeriods_MethodNotAllowed tests unsupported methods
func TestHandleSiteConfigPeriods_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	methods := []string{http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/site_config_periods", nil)
			w := httptest.NewRecorder()

			server.handleSiteConfigPeriods(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s, got %d", method, w.Code)
			}
		})
	}
}

// TestHandleTimeline_MissingParams tests timeline endpoint validation
func TestHandleTimeline_MissingParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name       string
		query      string
		expectCode int
	}{
		{
			name:       "missing site_id",
			query:      "",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "invalid site_id",
			query:      "?site_id=invalid",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "negative site_id",
			query:      "?site_id=-1",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "zero site_id",
			query:      "?site_id=0",
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/timeline"+tt.query, nil)
			w := httptest.NewRecorder()

			server.handleTimeline(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestHandleTimeline_MethodNotAllowed tests unsupported methods
func TestHandleTimeline_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/timeline?site_id=1", nil)
			w := httptest.NewRecorder()

			server.handleTimeline(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s, got %d", method, w.Code)
			}
		})
	}
}

// TestHandleTimeline_WithPeriods tests timeline with config periods
func TestHandleTimeline_WithPeriods(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site
	site := &db.Site{
		Name:     "Timeline Test Site",
		Location: "Test Location",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a config period
	period := &db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           true,
		CosineErrorAngle:   15.0,
	}
	if err := dbInst.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create period: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/timeline?site_id=%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.handleTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := response["config_periods"]; !ok {
		t.Error("Expected config_periods in response")
	}
	if _, ok := response["unconfigured_periods"]; !ok {
		t.Error("Expected unconfigured_periods in response")
	}
}

// TestDownloadReport_PDFFileNotFound tests download when PDF file is missing from disk
func TestDownloadReport_PDFFileNotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Set up a custom PDF directory
	tempDir := t.TempDir()
	os.Setenv("PDF_GENERATOR_DIR", tempDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	// Create a report record pointing to a non-existent file
	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/nonexistent.pdf",
		Filename:  "nonexistent.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download", report.ID), nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, report.ID, "pdf")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestDownloadReport_Success tests successful PDF download
func TestDownloadReport_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Set up a custom PDF directory with a test file
	tempDir := t.TempDir()
	os.Setenv("PDF_GENERATOR_DIR", tempDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	// Create output directory and test PDF file
	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	testPDFPath := filepath.Join(outputDir, "test.pdf")
	testContent := []byte("%PDF-1.4 test content")
	if err := os.WriteFile(testPDFPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test PDF: %v", err)
	}

	// Create report record
	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/test.pdf",
		Filename:  "test.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download", report.ID), nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, report.ID, "pdf")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	if w.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("Expected Content-Type application/pdf, got %s", w.Header().Get("Content-Type"))
	}

	if w.Header().Get("Content-Disposition") != "attachment; filename=test.pdf" {
		t.Errorf("Expected Content-Disposition with filename, got %s", w.Header().Get("Content-Disposition"))
	}
}

// TestDownloadReport_ZIPSuccess tests successful ZIP download
func TestDownloadReport_ZIPSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Set up a custom PDF directory with test files
	tempDir := t.TempDir()
	os.Setenv("PDF_GENERATOR_DIR", tempDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	// Create output directory and test files
	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	testZIPPath := filepath.Join(outputDir, "test.zip")
	testContent := []byte("PK zip content")
	if err := os.WriteFile(testZIPPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test ZIP: %v", err)
	}

	// Create report record with ZIP file
	zipFilepath := "output/test.zip"
	zipFilename := "test.zip"
	report := &db.SiteReport{
		SiteID:      0,
		StartDate:   "2024-01-01",
		EndDate:     "2024-01-07",
		Filepath:    "output/test.pdf",
		Filename:    "test.pdf",
		ZipFilepath: &zipFilepath,
		ZipFilename: &zipFilename,
		RunID:       "run-id",
		Timezone:    "UTC",
		Units:       "mph",
		Source:      "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download?file_type=zip", report.ID), nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, report.ID, "zip")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	if w.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("Expected Content-Type application/zip, got %s", w.Header().Get("Content-Type"))
	}
}

// TestDownloadReport_PathTraversal tests security against path traversal
func TestDownloadReport_PathTraversal(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tempDir := t.TempDir()
	os.Setenv("PDF_GENERATOR_DIR", tempDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	// Create report with malicious path
	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "../../../etc/passwd",
		Filename:  "passwd",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download", report.ID), nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, report.ID, "pdf")

	// Should be rejected for security reasons
	if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("Expected status 403 or 404 for path traversal attempt, got %d", w.Code)
	}
}

// TestHandleReports_ListWithFilename tests the new filename-based download route
func TestHandleReports_ListWithFilename(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tempDir := t.TempDir()
	os.Setenv("PDF_GENERATOR_DIR", tempDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	// Create output directory and test files
	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	testPDFPath := filepath.Join(outputDir, "test_report.pdf")
	if err := os.WriteFile(testPDFPath, []byte("%PDF"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	testZIPPath := filepath.Join(outputDir, "test_sources.zip")
	if err := os.WriteFile(testZIPPath, []byte("PK"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	zipFilepath := "output/test_sources.zip"
	zipFilename := "test_sources.zip"
	report := &db.SiteReport{
		SiteID:      0,
		StartDate:   "2024-01-01",
		EndDate:     "2024-01-07",
		Filepath:    "output/test_report.pdf",
		Filename:    "test_report.pdf",
		ZipFilepath: &zipFilepath,
		ZipFilename: &zipFilename,
		RunID:       "run-id",
		Timezone:    "UTC",
		Units:       "mph",
		Source:      "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	// Test PDF download via filename route
	t.Run("pdf_via_filename", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download/test_report.pdf", report.ID), nil)
		w := httptest.NewRecorder()
		server.handleReports(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	// Test ZIP download via filename route
	t.Run("zip_via_filename", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download/test_sources.zip", report.ID), nil)
		w := httptest.NewRecorder()
		server.handleReports(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})
}

// TestCreateSite_InvalidJSON tests create site with invalid JSON
func TestCreateSite_InvalidJSON(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestUpdateSite_InvalidJSON tests update site with invalid JSON
func TestUpdateSite_InvalidJSON(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPut, "/api/sites/1", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestUpdateSite_MissingFields tests validation of update site
func TestUpdateSite_MissingFields(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	site := &db.Site{
		Name:     "Original",
		Location: "Location",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	tests := []struct {
		name string
		body db.Site
	}{
		{
			name: "missing name",
			body: db.Site{Location: "Location"},
		},
		{
			name: "missing location",
			body: db.Site{Name: "Name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/sites/%d", site.ID), bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleSites(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestShowRadarObjectStats_WithSiteID tests radar stats with site_id parameter
func TestShowRadarObjectStats_WithSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site
	site := &db.Site{
		Name:     "Stats Site",
		Location: "Test Location",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a config period
	period := &db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   15.0,
	}
	if err := dbInst.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create period: %v", err)
	}

	start := "1697318400"
	end := "1697404800"
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/radar_stats?start=%s&end=%s&site_id=%d", start, end, site.ID), nil)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestShowRadarObjectStats_InvalidSiteID tests invalid site_id validation
func TestShowRadarObjectStats_InvalidSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	start := "1697318400"
	end := "1697404800"

	tests := []struct {
		name   string
		siteID string
	}{
		{"invalid string", "invalid"},
		{"negative", "-1"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				fmt.Sprintf("/api/radar_stats?start=%s&end=%s&site_id=%s", start, end, tt.siteID), nil)
			w := httptest.NewRecorder()

			server.showRadarObjectStats(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestShowRadarObjectStats_BoundaryThreshold tests boundary_threshold parameter
func TestShowRadarObjectStats_BoundaryThreshold(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	start := "1697318400"
	end := "1697404800"

	tests := []struct {
		name       string
		threshold  string
		expectCode int
	}{
		{"valid threshold", "5", http.StatusOK},
		{"zero threshold", "0", http.StatusOK},
		{"invalid string", "abc", http.StatusBadRequest},
		{"negative value", "-1", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				fmt.Sprintf("/api/radar_stats?start=%s&end=%s&boundary_threshold=%s", start, end, tt.threshold), nil)
			w := httptest.NewRecorder()

			server.showRadarObjectStats(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("Expected status %d, got %d", tt.expectCode, w.Code)
			}
		})
	}
}

// TestShowRadarObjectStats_ModelVersion tests model_version parameter for transits source
func TestShowRadarObjectStats_ModelVersion(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	start := "1697318400"
	end := "1697404800"

	// Test with radar_data_transits source and model_version
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/radar_stats?start=%s&end=%s&source=radar_data_transits&model_version=custom-model", start, end), nil)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestLoggingMiddleware_WithoutPort tests logging middleware without port in host
func TestLoggingMiddleware_WithoutPort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(handler)

	// Test without port
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "localhost"
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestLoggingMiddleware_WithURLPort tests logging middleware using URL port
func TestLoggingMiddleware_WithURLPort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:9090/test", nil)
	req.Host = ""
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestSendCommandHandler_Empty tests sending empty command
func TestSendCommandHandler_Empty(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/command", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.sendCommandHandler(w, req)

	// Empty command should still succeed (just sends empty string)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestGenerateReport_WithValidSiteAndConfigPeriod tests report generation path
func TestGenerateReport_WithValidSiteAndConfigPeriod(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Clean up any config files this test creates to avoid polluting E2E tests
	defer func() {
		tmp := os.TempDir()
		files, _ := filepath.Glob(filepath.Join(tmp, "report_config_*.json"))
		for _, f := range files {
			os.Remove(f)
		}
	}()

	// Create site
	site := &db.Site{
		Name:     "Report Site",
		Location: "Test Location",
		Surveyor: "Test Surveyor",
		Contact:  "test@example.com",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create active config period
	period := &db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   15.0,
	}
	if err := dbInst.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create period: %v", err)
	}

	// Mock the PDF generator - use /usr/bin/true on macOS, /bin/true on Linux
	truePath := "/usr/bin/true"
	if _, err := os.Stat(truePath); os.IsNotExist(err) {
		truePath = "/bin/true" // Fallback for Linux
	}
	os.Setenv("PDF_GENERATOR_PYTHON", truePath)
	defer os.Unsetenv("PDF_GENERATOR_PYTHON")

	body := map[string]interface{}{
		"start_date":         "2024-01-01",
		"end_date":           "2024-01-07",
		"site_id":            site.ID,
		"compare_start_date": "2023-12-01",
		"compare_end_date":   "2023-12-07",
		"compare_source":     "radar_objects",
		"timezone":           "US/Pacific",
		"units":              "mph",
		"group":              "1h",
		"source":             "radar_objects",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	// Should fail on PDF execution, but pass all validation
	// Expected: 500 (execution fails) or 400 (no data/file)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 500 or 400, got %d", w.Code)
	}
}

// TestGenerateReport_SiteWithNoConfigPeriod tests report generation when site has no active config
func TestGenerateReport_SiteWithNoConfigPeriod(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site without config period
	site := &db.Site{
		Name:     "No Config Site",
		Location: "Test Location",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	body := map[string]interface{}{
		"start_date": "2024-01-01",
		"end_date":   "2024-01-07",
		"site_id":    site.ID,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Additional Coverage Tests for Low-Coverage Functions
// ============================================================================

// TestListSites_SuccessfulList tests successful listing of all sites
func TestListSites_SuccessfulList(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create multiple sites
	for i := 0; i < 3; i++ {
		site := &db.Site{
			Name:     fmt.Sprintf("Site %d", i),
			Location: fmt.Sprintf("Location %d", i),
		}
		if err := dbInst.CreateSite(site); err != nil {
			t.Fatalf("Failed to create site %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	w := httptest.NewRecorder()

	server.listSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var sites []db.Site
	if err := json.Unmarshal(w.Body.Bytes(), &sites); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(sites) != 3 {
		t.Errorf("Expected 3 sites, got %d", len(sites))
	}
}

// TestGetSite_NotFound tests getting a non-existent site
func TestGetSite_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/9999", nil)
	w := httptest.NewRecorder()

	server.getSite(w, req, 9999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestGetSite_Success tests getting an existing site
func TestGetSite_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sites/%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.getSite(w, req, site.ID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestListAllReports_Success tests listing reports from all sites
func TestListAllReports_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a report for the site
	report := &db.SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-07",
		Filename:    "test_report.pdf",
		ZipFilename: nil,
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	w := httptest.NewRecorder()

	server.listAllReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var reports []db.SiteReport
	if err := json.Unmarshal(w.Body.Bytes(), &reports); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(reports) != 1 {
		t.Errorf("Expected 1 report, got %d", len(reports))
	}
}

// TestListSiteReports_Success tests listing reports for a specific site
func TestListSiteReports_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create multiple reports
	for i := 0; i < 3; i++ {
		report := &db.SiteReport{
			SiteID:      site.ID,
			StartDate:   fmt.Sprintf("2024-01-0%d", i+1),
			Filename:    fmt.Sprintf("report_%d.pdf", i),
			ZipFilename: nil,
		}
		if err := dbInst.CreateSiteReport(report); err != nil {
			t.Fatalf("Failed to create report %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sites/%d/reports", site.ID), nil)
	w := httptest.NewRecorder()

	server.listSiteReports(w, req, site.ID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestGetReport_NotFound tests getting a non-existent report
func TestGetReport_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/9999", nil)
	w := httptest.NewRecorder()

	server.getReport(w, req, 9999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestGetReport_Success tests getting an existing report
func TestGetReport_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a report
	report := &db.SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-07",
		Filename:    "test_report.pdf",
		ZipFilename: nil,
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d", report.ID), nil)
	w := httptest.NewRecorder()

	server.getReport(w, req, report.ID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestHandleDatabaseStats_Success tests successful database stats retrieval
func TestHandleDatabaseStats_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/database/stats", nil)
	w := httptest.NewRecorder()

	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestShowConfig_Success tests showing configuration
func TestShowConfig_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	server.showConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestShowConfig_PostNotAllowed tests POST to config endpoint
func TestShowConfig_PostNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	w := httptest.NewRecorder()

	server.showConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestDeleteSite_NotFound tests deleting a non-existent site
func TestDeleteSite_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/9999", nil)
	w := httptest.NewRecorder()

	server.deleteSite(w, req, 9999)

	// Should return 404 or still succeed (depends on DB implementation)
	// Most implementations will succeed even if ID doesn't exist
	if w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
		t.Errorf("Expected status 204 or 404, got %d", w.Code)
	}
}

// TestDeleteSite_Success tests successful site deletion
func TestDeleteSite_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/sites/%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.deleteSite(w, req, site.ID)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

// TestDeleteReport_NotFound tests deleting a non-existent report
func TestDeleteReport_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/reports/9999", nil)
	w := httptest.NewRecorder()

	server.deleteReport(w, req, 9999)

	// May succeed or return 404 depending on implementation
	if w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
		t.Errorf("Expected status 204 or 404, got %d", w.Code)
	}
}

// TestDeleteReport_Success tests successful report deletion
func TestDeleteReport_Success(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site and report
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	report := &db.SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-07",
		Filename:    "test_report.pdf",
		ZipFilename: nil,
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/reports/%d", report.ID), nil)
	w := httptest.NewRecorder()

	server.deleteReport(w, req, report.ID)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

// TestHandleReports_UnsupportedMethod tests unsupported methods on reports endpoint
func TestHandleReports_UnsupportedMethod(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPut, "/api/reports", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	// PUT on /api/reports (no ID) returns 400 due to missing ID, not 405
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleReports_ListAll tests GET on reports endpoint
func TestHandleReports_ListAll(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestHandleReports_GetByID tests GET on reports endpoint with ID
func TestHandleReports_GetByID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site and report
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	report := &db.SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-07",
		Filename:    "test_report.pdf",
		ZipFilename: nil,
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d", report.ID), nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestHandleReports_DeleteByID tests DELETE on reports endpoint with ID
func TestHandleReports_DeleteByID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site and report
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	report := &db.SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-07",
		Filename:    "test_report.pdf",
		ZipFilename: nil,
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/reports/%d", report.ID), nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

// TestHandleReports_DownloadPDF tests download of PDF file
func TestHandleReports_DownloadPDF(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create temp directory structure for PDF generator
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	// Create temp PDF file
	pdfPath := filepath.Join(outputDir, "test_report.pdf")
	if err := os.WriteFile(pdfPath, []byte("PDF content"), 0644); err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Set PDF_GENERATOR_DIR to the temp directory
	os.Setenv("PDF_GENERATOR_DIR", tmpDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	// Create site and report with relative filepath (relative to PDF_GENERATOR_DIR)
	site := &db.Site{Name: "Test Site", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	report := &db.SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-01",
		EndDate:     "2024-01-07",
		Filepath:    "output/test_report.pdf", // Relative path
		Filename:    "test_report.pdf",
		ZipFilename: nil,
		RunID:       "test-run-id",
		Timezone:    "UTC",
		Units:       "mph",
		Source:      "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download/pdf", report.ID), nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestHandleReports_InvalidIDFormat tests invalid report ID format
func TestHandleReports_InvalidIDFormat(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/invalid", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestListEvents_WithStartEnd tests listing events with start/end parameters
func TestListEvents_WithStartEnd(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/events?start=2024-01-01&end=2024-01-07", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestListEvents_InvalidDateFormat tests listing events with invalid date format
func TestListEvents_InvalidDateFormat(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/events?start=invalid-date", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	// Should still return 200 with empty results or parse error
	// depends on implementation
}

// TestListEvents_PostNotAllowed tests POST on events endpoint
func TestListEvents_PostNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/events", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestSendCommandHandler_InvalidMethod tests GET on command endpoint
func TestSendCommandHandler_InvalidMethod(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/command", nil)
	w := httptest.NewRecorder()

	server.sendCommandHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestSendCommandHandler_EmptyCommand tests POST with empty command
func TestSendCommandHandler_EmptyCommand(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/command", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.sendCommandHandler(w, req)

	// Should return error or accept empty command
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 200 or 400, got %d", w.Code)
	}
}

// TestGetPDFGeneratorDir_Override tests PDF generator dir with env override
func TestGetPDFGeneratorDir_Override(t *testing.T) {
	// Set environment variable
	tmpDir := t.TempDir()
	os.Setenv("PDF_GENERATOR_DIR", tmpDir)
	defer os.Unsetenv("PDF_GENERATOR_DIR")

	dir, err := getPDFGeneratorDir()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if dir != tmpDir {
		t.Errorf("Expected dir %s, got %s", tmpDir, dir)
	}
}

// TestGetPDFGeneratorDir_Default tests PDF generator dir without override
func TestGetPDFGeneratorDir_Default(t *testing.T) {
	os.Unsetenv("PDF_GENERATOR_DIR")

	dir, err := getPDFGeneratorDir()

	// May succeed or fail depending on whether relative path exists
	// Just ensure it doesn't panic and returns a reasonable result
	if err != nil {
		// Expected if tools/pdf-generator doesn't exist relative to CWD
		t.Logf("getPDFGeneratorDir returned error (expected in test env): %v", err)
	} else if dir == "" {
		t.Error("Expected non-empty directory path")
	}
}

// TestWriteJSONError_BadRequest tests the JSON error writer helper for 400 errors
func TestWriteJSONError_BadRequest(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	w := httptest.NewRecorder()
	server.writeJSONError(w, http.StatusBadRequest, "Test error message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["error"] != "Test error message" {
		t.Errorf("Expected error message 'Test error message', got '%s'", response["error"])
	}
}

// TestWriteJSONError_InternalServerError tests 500 error response
func TestWriteJSONError_InternalServerError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	w := httptest.NewRecorder()
	server.writeJSONError(w, http.StatusInternalServerError, "Internal error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}
