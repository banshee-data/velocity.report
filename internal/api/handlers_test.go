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

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

// TestSendCommandHandler tests the /command endpoint
func TestSendCommandHandler(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Test POST with command
	t.Run("POST_with_command", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/command", strings.NewReader("command=OJ"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.sendCommandHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Command sent successfully") {
			t.Errorf("Expected success message, got: %s", w.Body.String())
		}
	})

	// Test GET (method not allowed)
	t.Run("GET_method_not_allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/command", nil)
		w := httptest.NewRecorder()

		server.sendCommandHandler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", w.Code)
		}
	})
}

// TestHandleReports_List tests listing all reports
func TestHandleReports_List(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create some test reports
	for i := 0; i < 3; i++ {
		report := &db.SiteReport{
			SiteID:    0,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-07",
			Filepath:  "output/report.pdf",
			Filename:  "report.pdf",
			RunID:     fmt.Sprintf("run-%d", i),
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := dbInst.CreateSiteReport(report); err != nil {
			t.Fatalf("Failed to create report: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reports/", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var reports []db.SiteReport
	if err := json.NewDecoder(w.Body).Decode(&reports); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(reports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(reports))
	}
}

// TestHandleReports_ListSiteReports tests listing reports for a specific site
func TestHandleReports_ListSiteReports(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site
	site := &db.Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 21.0,
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create reports for this site
	for i := 0; i < 2; i++ {
		report := &db.SiteReport{
			SiteID:    site.ID,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-07",
			Filepath:  "output/report.pdf",
			Filename:  "report.pdf",
			RunID:     fmt.Sprintf("run-%d", i),
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := dbInst.CreateSiteReport(report); err != nil {
			t.Fatalf("Failed to create report: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/site/%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var reports []db.SiteReport
	if err := json.NewDecoder(w.Body).Decode(&reports); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(reports) != 2 {
		t.Errorf("Expected 2 reports, got %d", len(reports))
	}
}

// TestHandleReports_GetReport tests getting a single report
func TestHandleReports_GetReport(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
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

	var retrieved db.SiteReport
	if err := json.NewDecoder(w.Body).Decode(&retrieved); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if retrieved.ID != report.ID {
		t.Errorf("Expected report ID %d, got %d", report.ID, retrieved.ID)
	}
}

// TestHandleReports_GetReport_NotFound tests getting a non-existent report
func TestHandleReports_GetReport_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/99999", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleReports_DeleteReport tests deleting a report
func TestHandleReports_DeleteReport(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
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

	// Verify deletion
	_, err := dbInst.GetSiteReport(report.ID)
	if err == nil {
		t.Error("Expected error when getting deleted report")
	}
}

// TestHandleReports_DeleteReport_NotFound tests deleting a non-existent report
func TestHandleReports_DeleteReport_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/reports/99999", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleReports_InvalidID tests handling invalid report IDs
func TestHandleReports_InvalidID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/invalid", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleReports_MethodNotAllowed tests unsupported HTTP methods
func TestHandleReports_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPatch, "/api/reports/1", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleReports_SiteReports_MethodNotAllowed tests unsupported methods for site reports
func TestHandleReports_SiteReports_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/site/1", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleReports_Download_MethodNotAllowed tests unsupported methods for download
func TestHandleReports_Download_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/reports/%d/download", report.ID), nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestDownloadReport_InvalidFileType tests download with invalid file type
func TestDownloadReport_InvalidFileType(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download?file_type=invalid", report.ID), nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, report.ID, "invalid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestDownloadReport_ReportNotFound tests download when report doesn't exist
func TestDownloadReport_ReportNotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/99999/download", nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, 99999, "pdf")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestDownloadReport_ZipNotAvailable tests download when ZIP is not available
func TestDownloadReport_ZipNotAvailable(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
		// Note: ZipFilepath and ZipFilename are nil
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download?file_type=zip", report.ID), nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, report.ID, "zip")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestGenerateReport_MissingDates tests report generation without required dates
func TestGenerateReport_MissingDates(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{
			name: "missing start_date",
			body: map[string]interface{}{
				"end_date": "2024-01-07",
			},
		},
		{
			name: "missing end_date",
			body: map[string]interface{}{
				"start_date": "2024-01-01",
			},
		},
		{
			name: "both missing",
			body: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.generateReport(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestGenerateReport_CompareDatesRequiredTogether(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{
			name: "missing compare end date",
			body: map[string]interface{}{
				"start_date":        "2024-01-01",
				"end_date":          "2024-01-07",
				"compare_start_date": "2023-12-01",
			},
		},
		{
			name: "missing compare start date",
			body: map[string]interface{}{
				"start_date":      "2024-01-01",
				"end_date":        "2024-01-07",
				"compare_end_date": "2023-12-07",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.generateReport(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestGenerateReport_MethodNotAllowed tests report generation with wrong method
func TestGenerateReport_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/generate_report", nil)
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestGenerateReport_InvalidJSON tests report generation with invalid JSON
func TestGenerateReport_InvalidJSON(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestGenerateReport_MissingCosineErrorAngle tests validation
func TestGenerateReport_MissingCosineErrorAngle(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body := map[string]interface{}{
		"start_date": "2024-01-01",
		"end_date":   "2024-01-07",
		// Note: no cosine_error_angle and no site_id
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestGenerateReport_InvalidSiteID tests report generation with invalid site
func TestGenerateReport_InvalidSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	siteID := 99999
	body := map[string]interface{}{
		"start_date": "2024-01-01",
		"end_date":   "2024-01-07",
		"site_id":    siteID,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestStatusCodeColor tests the statusCodeColor helper
func TestStatusCodeColor(t *testing.T) {
	tests := []struct {
		code     int
		contains string
	}{
		{200, "200"},
		{201, "201"},
		{301, "301"},
		{400, "400"},
		{404, "404"},
		{500, "500"},
		{100, "100"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			result := statusCodeColor(tt.code)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %s, got %s", tt.contains, result)
			}
		})
	}
}

// TestLoggingResponseWriter tests the response writer wrapper
func TestLoggingResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	lrw := &loggingResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	// Test WriteHeader
	lrw.WriteHeader(http.StatusCreated)
	if lrw.statusCode != http.StatusCreated {
		t.Errorf("Expected status code 201, got %d", lrw.statusCode)
	}

	// Test Flush
	lrw.Flush() // Should not panic
}

// TestLoggingMiddleware tests the logging middleware
func TestLoggingMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	wrapped := LoggingMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestServeMux tests the ServeMux method
func TestServeMux(t *testing.T) {
	mux := serialmux.NewDisabledSerialMux()
	server := NewServer(mux, nil, "mph", "UTC")

	// First call should create mux
	m1 := server.ServeMux()
	if m1 == nil {
		t.Error("Expected ServeMux to return non-nil mux")
	}

	// Second call should return same mux
	m2 := server.ServeMux()
	if m1 != m2 {
		t.Error("Expected ServeMux to return same mux on subsequent calls")
	}
}

// TestGetPDFGeneratorDir tests the PDF generator directory logic
func TestGetPDFGeneratorDir(t *testing.T) {
	// Save and restore environment
	origDir := os.Getenv("PDF_GENERATOR_DIR")
	defer os.Setenv("PDF_GENERATOR_DIR", origDir)

	// Test with environment override
	t.Run("env_override", func(t *testing.T) {
		os.Setenv("PDF_GENERATOR_DIR", "/custom/path")
		dir, err := getPDFGeneratorDir()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if dir != "/custom/path" {
			t.Errorf("Expected '/custom/path', got '%s'", dir)
		}
	})

	// Test fallback to development location
	t.Run("development_fallback", func(t *testing.T) {
		os.Unsetenv("PDF_GENERATOR_DIR")
		dir, err := getPDFGeneratorDir()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		// Should contain tools/pdf-generator
		if !strings.Contains(dir, filepath.Join("tools", "pdf-generator")) {
			t.Errorf("Expected path to contain 'tools/pdf-generator', got '%s'", dir)
		}
	})
}

// TestShowRadarObjectStats_ValidSources tests valid data sources
func TestShowRadarObjectStats_ValidSources(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	sources := []string{"radar_objects", "radar_data_transits"}
	start := "1697318400"
	end := "1697404800"

	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				fmt.Sprintf("/api/radar_stats?start=%s&end=%s&source=%s", start, end, source), nil)
			w := httptest.NewRecorder()

			server.showRadarObjectStats(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestShowRadarObjectStats_InvalidHistogramParams tests histogram parameter validation
func TestShowRadarObjectStats_InvalidHistogramParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	start := "1697318400"
	end := "1697404800"

	tests := []struct {
		name  string
		query string
	}{
		{"invalid hist_bucket_size", "hist_bucket_size=abc"},
		{"invalid hist_max", "hist_max=xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				fmt.Sprintf("/api/radar_stats?start=%s&end=%s&%s", start, end, tt.query), nil)
			w := httptest.NewRecorder()

			server.showRadarObjectStats(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestListEvents_WithTimezoneParam tests timezone conversion
func TestListEvents_WithTimezoneParam(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name     string
		timezone string
		valid    bool
	}{
		{"valid UTC", "UTC", true},
		{"valid US/Pacific", "US/Pacific", true},
		{"invalid timezone", "Invalid/Zone", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/events?timezone="+tt.timezone, nil)
			w := httptest.NewRecorder()

			server.listEvents(w, req)

			if tt.valid {
				if w.Code != http.StatusOK {
					t.Errorf("Expected status 200, got %d", w.Code)
				}
			} else {
				if w.Code != http.StatusBadRequest {
					t.Errorf("Expected status 400, got %d", w.Code)
				}
			}
		})
	}
}

// TestSetTransitController tests the SetTransitController method
func TestSetTransitController(t *testing.T) {
	mux := serialmux.NewDisabledSerialMux()
	server := NewServer(mux, nil, "mph", "UTC")

	if server.transitController != nil {
		t.Error("Expected nil transit controller initially")
	}

	mockTC := &mockTransitController{}
	server.SetTransitController(mockTC)

	if server.transitController != mockTC {
		t.Error("Expected transit controller to be set")
	}
}
