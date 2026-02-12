package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

// Coverage tests for server.go - focused on uncovered paths

// cleanupClosedDB removes database files after a test that closed the DB early
func cleanupClosedDB(t *testing.T, fname string) {
	t.Helper()
	os.Remove(fname)
	os.Remove(fname + "-shm")
	os.Remove(fname + "-wal")
}

// TestListSites_DBError tests database error path in listSites
func TestListSites_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/sites/", nil)
	w := httptest.NewRecorder()

	server.listSites(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to retrieve sites") {
		t.Errorf("Expected error message about retrieving sites, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestListSites_JSONEncodeError tests JSON encode error path (line 546-549)
// This is hard to trigger directly, but we document it's covered by writeJSONError

// TestGetSite_DBError tests database error path (non-"not found" error)
func TestGetSite_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force a generic DB error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/sites/1", nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	// Should get error (404 or 500 depending on the DB state)
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 404 or 500, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestGetSite_JSONEncodeError tests JSON encode error path (line 564-567)
// This is covered by writeJSONError tests

// TestSendCommandHandler_NonPOST tests non-POST method rejection
func TestSendCommandHandler_NonPOST(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/command", nil)
	w := httptest.NewRecorder()

	server.sendCommandHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Method not allowed") {
		t.Errorf("Expected 'Method not allowed', got: %s", body)
	}
}

// TestSendCommandHandler_SendError - cannot test with DisabledSerialMux
// The DisabledSerialMux.SendCommand() always returns nil (success),
// so we cannot trigger the error path in sendCommandHandler without
// creating a mock SerialMux that returns errors. This is an acceptable
// coverage gap since the error handling code is straightforward.

// TestShowConfig_NonGET tests non-GET method rejection
func TestShowConfig_NonGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	w := httptest.NewRecorder()

	server.showConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Method not allowed") {
		t.Errorf("Expected 'Method not allowed', got: %s", errResp["error"])
	}
}

// TestShowConfig_JSONEncodeError tests JSON encode error (line 438-441)
// Covered by writeJSONError tests

// TestListEvents_NonGET tests non-GET method rejection
func TestListEvents_NonGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/events", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestListEvents_DBError tests database error path
func TestListEvents_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to retrieve events") {
		t.Errorf("Expected error about retrieving events, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestListEvents_JSONEncodeError tests JSON encode error (line 491-494)
// Covered by writeJSONError tests

// TestListAllReports_DBError tests database error in listAllReports
func TestListAllReports_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/reports/", nil)
	w := httptest.NewRecorder()

	server.listAllReports(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to retrieve reports") {
		t.Errorf("Expected error about retrieving reports, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestListAllReports_JSONEncodeError tests JSON encode error (line 1306-1309)
// Covered by writeJSONError tests

// TestListSiteReports_DBError tests database error in listSiteReports
func TestListSiteReports_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/reports/site/1", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to retrieve reports") {
		t.Errorf("Expected error about retrieving reports, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestListSiteReports_JSONEncodeError tests JSON encode error (line 1320-1323)
// Covered by writeJSONError tests

// TestGetReport_DBError tests database error (non-"not found")
func TestGetReport_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/reports/1", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	// Should get 404 or 500
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("Expected status 500 or 404, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestGetReport_JSONEncodeError tests JSON encode error (line 1338-1341)
// Covered by writeJSONError tests

// TestHandleDatabaseStats_NonGET tests non-GET method rejection
func TestHandleDatabaseStats_NonGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/db_stats", nil)
	w := httptest.NewRecorder()

	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Method not allowed") {
		t.Errorf("Expected 'Method not allowed', got: %s", errResp["error"])
	}
}

// TestHandleDatabaseStats_NilDB tests nil database handling
func TestHandleDatabaseStats_NilDB(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Temporarily set db to nil
	originalDB := server.db
	server.db = nil
	defer func() { server.db = originalDB }()

	req := httptest.NewRequest(http.MethodGet, "/api/db_stats", nil)
	w := httptest.NewRecorder()

	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Database not configured") {
		t.Errorf("Expected 'Database not configured', got: %s", errResp["error"])
	}
}

// TestHandleDatabaseStats_DBError tests database error path
func TestHandleDatabaseStats_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/db_stats", nil)
	w := httptest.NewRecorder()

	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to get database stats") {
		t.Errorf("Expected error about database stats, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestHandleDatabaseStats_JSONEncodeError tests JSON encode error (line 1495-1498)
// Covered by writeJSONError tests

// TestListSiteConfigPeriods_DBError tests database error path
func TestListSiteConfigPeriods_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/site_config_periods", nil)
	w := httptest.NewRecorder()

	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to retrieve site config periods") {
		t.Errorf("Expected error about site config periods, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestListSiteConfigPeriods_JSONEncodeError tests JSON encode error (line 676-679)
// Covered by writeJSONError tests

// TestHandleTimeline_NonGET tests non-GET method rejection
func TestHandleTimeline_NonGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/timeline?site_id=1", nil)
	w := httptest.NewRecorder()

	server.handleTimeline(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Method not allowed") {
		t.Errorf("Expected 'Method not allowed', got: %s", errResp["error"])
	}
}

// TestHandleTimeline_MissingSiteID tests missing site_id parameter
func TestHandleTimeline_MissingSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/timeline", nil)
	w := httptest.NewRecorder()

	server.handleTimeline(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "site_id is required") {
		t.Errorf("Expected 'site_id is required', got: %s", errResp["error"])
	}
}

// TestHandleTimeline_InvalidSiteID tests invalid site_id parameter
func TestHandleTimeline_InvalidSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name   string
		siteID string
	}{
		{"non-numeric", "abc"},
		{"negative", "-1"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/timeline?site_id="+tt.siteID, nil)
			w := httptest.NewRecorder()

			server.handleTimeline(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400 for site_id=%s, got %d", tt.siteID, w.Code)
			}

			var errResp map[string]string
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("Failed to decode error response: %v", err)
			}

			if !strings.Contains(errResp["error"], "Invalid 'site_id' parameter") {
				t.Errorf("Expected error about invalid site_id, got: %s", errResp["error"])
			}
		})
	}
}

// TestHandleTimeline_RadarDataRangeDBError tests DB error on RadarDataRange call
func TestHandleTimeline_RadarDataRangeDBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/timeline?site_id=1", nil)
	w := httptest.NewRecorder()

	server.handleTimeline(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to retrieve") {
		t.Errorf("Expected error about failed retrieval, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestHandleTimeline_JSONEncodeError tests JSON encode error (line 757-759)
// Covered by writeJSONError tests

// TestGenerateReport_NonPOST tests non-POST method rejection
func TestGenerateReport_NonPOST(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/generate_report", nil)
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Method not allowed") {
		t.Errorf("Expected 'Method not allowed', got: %s", errResp["error"])
	}
}

// TestGenerateReport_MismatchedCompareDates tests comparison date validation
func TestGenerateReport_MismatchedCompareDates(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name         string
		compareStart string
		compareEnd   string
	}{
		{"start only", "2023-01-01", ""},
		{"end only", "", "2023-01-31"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]string{
				"start_date":         "2024-01-01",
				"end_date":           "2024-01-31",
				"compare_start_date": tt.compareStart,
				"compare_end_date":   tt.compareEnd,
			}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.generateReport(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}

			var errResp map[string]string
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("Failed to decode error response: %v", err)
			}

			if !strings.Contains(errResp["error"], "compare_start_date and compare_end_date are required together") {
				t.Errorf("Expected error about comparison dates, got: %s", errResp["error"])
			}
		})
	}
}

// TestGenerateReport_NoSiteID tests missing site_id
func TestGenerateReport_NoSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	payload := map[string]string{
		"start_date": "2024-01-01",
		"end_date":   "2024-01-31",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/generate_report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "site_id is required") {
		t.Errorf("Expected error about site_id, got: %s", errResp["error"])
	}
}

// TestDownloadReport_InvalidFileTypeParam tests invalid file_type parameter
func TestDownloadReport_InvalidFileTypeParam(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/download?file_type=invalid", nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, 1, "invalid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Invalid file_type parameter") {
		t.Errorf("Expected error about invalid file_type, got: %s", errResp["error"])
	}
}

// TestGetPDFGeneratorDir_ReturnsPath tests successful directory resolution
func TestGetPDFGeneratorDir_ReturnsPath(t *testing.T) {
	dir, err := getPDFGeneratorDir()
	if err != nil {
		t.Fatalf("getPDFGeneratorDir() failed: %v", err)
	}

	if dir == "" {
		t.Error("Expected non-empty directory path")
	}
}

// TestDeleteReport_DBError tests database error in delete
func TestDeleteReport_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/reports/1", nil)
	w := httptest.NewRecorder()

	server.handleReports(w, req)

	// Should get 404 or 500
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 404 or 500, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestCreateSite_DBError tests database error during creation
func TestCreateSite_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	payload := map[string]string{
		"name":     "Test Site",
		"location": "Test Location",
	}
	body, _ := json.Marshal(payload)

	// Close DB to force error
	dbInst.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "Failed to create site") {
		t.Errorf("Expected error about creating site, got: %s", errResp["error"])
	}

	cleanupClosedDB(t, fname)
}

// TestCreateSite_JSONEncodeError tests JSON encode error after creation (line 593-596)
// This would require creating a ResponseWriter that fails on Encode - covered by writeJSONError tests

// TestCreateSite_MissingNameField tests validation of missing name
func TestCreateSite_MissingNameField(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	payload := map[string]string{
		"location": "Test Location",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "name is required") {
		t.Errorf("Expected error about missing name, got: %s", errResp["error"])
	}
}

// TestCreateSite_MissingLocationField tests validation of missing location
func TestCreateSite_MissingLocationField(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	payload := map[string]string{
		"name": "Test Site",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "location is required") {
		t.Errorf("Expected error about missing location, got: %s", errResp["error"])
	}
}

// TestWriteJSONError_EncodeFails tests the error path in writeJSONError itself
// This is the line 174-176 - it logs but doesn't return error to caller
// We can't easily test the log output, but the function is defensive

// TestStart_DevModeNoBuildDir verifies Start returns an error in dev mode
// when the web/build directory does not exist.
func TestStart_DevModeNoBuildDir(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Use a temp directory as CWD so web/build won't exist
	tmp := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx, "127.0.0.1:0", true)
	if err == nil {
		t.Fatal("Expected error when build directory does not exist")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected error about missing build dir, got: %v", err)
	}
}

// TestStart_ProductionModeListenAndShutdown verifies Start in production mode
// binds, serves requests, and shuts down cleanly on context cancellation.
func TestStart_ProductionModeListenAndShutdown(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Pick a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx, addr, false)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make a request to verify server is running - root should redirect to /app/
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	resp.Body.Close()

	// Make a request to /app/ to exercise the frontend handler
	resp, err = http.Get("http://" + addr + "/app/")
	if err != nil {
		t.Fatalf("failed to request /app/: %v", err)
	}
	resp.Body.Close()

	// Make request to /app/nonexistent to exercise SPA fallback
	resp, err = http.Get("http://" + addr + "/app/some/route")
	if err != nil {
		t.Fatalf("failed to request SPA route: %v", err)
	}
	resp.Body.Close()

	// Make request to /app/trailing/ to exercise trailing slash redirect
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err = client.Get("http://" + addr + "/app/trailing/")
	if err != nil {
		t.Fatalf("failed to request trailing slash: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected redirect for trailing slash, got %d", resp.StatusCode)
	}

	// Request /favicon.ico to exercise static handler
	resp, err = http.Get("http://" + addr + "/favicon.ico")
	if err != nil {
		t.Fatalf("failed to request favicon: %v", err)
	}
	resp.Body.Close()

	// Request a non-root non-app path to get 404
	resp, err = http.Get("http://" + addr + "/something")
	if err != nil {
		t.Fatalf("failed to request unknown path: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for unknown path, got %d", resp.StatusCode)
	}

	// Cancel context to trigger shutdown
	cancel()

	// Wait for server to stop
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Server returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not shut down within timeout")
	}
}

// TestStart_DevModeWithBuildDir verifies Start in dev mode when build dir exists.
func TestStart_DevModeWithBuildDir(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a temp directory to act as CWD with web/build
	tmp := t.TempDir()
	buildDir := filepath.Join(tmp, "web", "build")
	os.MkdirAll(buildDir, 0755)
	os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>test</html>"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	// Pick a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx, addr, true)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make a request to /app/ which should serve index.html from dev build dir
	resp, err := http.Get("http://" + addr + "/app/")
	if err != nil {
		t.Fatalf("failed to request /app/: %v", err)
	}
	resp.Body.Close()

	// Request /app/nonexistent - should fall back to index.html (SPA)
	resp, err = http.Get("http://" + addr + "/app/nonexistent")
	if err != nil {
		t.Fatalf("failed to request SPA route: %v", err)
	}
	resp.Body.Close()

	// Request /app/trailing/ with query string
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err = client.Get("http://" + addr + "/app/trailing/?foo=bar")
	if err != nil {
		t.Fatalf("failed to request trailing slash with query: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected redirect for trailing slash with query, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out")
	}
}

// TestStart_InvalidListenAddress verifies Start returns error for bad address.
func TestStart_InvalidListenAddress(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use an invalid listen address
	err := server.Start(ctx, "invalid-address-no-port", false)
	if err == nil {
		t.Fatal("Expected error for invalid listen address")
	}
}

// TestUpdateSite_NotFound tests updateSite when site doesn't exist.
func TestUpdateSite_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body := `{"name": "Test", "location": "Test Location"}`
	req := httptest.NewRequest(http.MethodPut, "/api/sites/99999", strings.NewReader(body))
	w := httptest.NewRecorder()
	server.updateSite(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// TestUpdateSite_InvalidJSON tests updateSite with malformed JSON.
func TestUpdateSite_InvalidJSONBody(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPut, "/api/sites/1", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()
	server.updateSite(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestUpdateSite_MissingName tests updateSite with empty name.
func TestUpdateSite_MissingName(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body := `{"name": "", "location": "Test Location"}`
	req := httptest.NewRequest(http.MethodPut, "/api/sites/1", strings.NewReader(body))
	w := httptest.NewRecorder()
	server.updateSite(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestUpdateSite_MissingLocation tests updateSite with empty location.
func TestUpdateSite_MissingLocation(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body := `{"name": "Test", "location": ""}`
	req := httptest.NewRequest(http.MethodPut, "/api/sites/1", strings.NewReader(body))
	w := httptest.NewRecorder()
	server.updateSite(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestUpdateSite_DBError tests updateSite when DB is closed.
func TestUpdateSite_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// First create a site, then close DB and try to update
	body := `{"name": "Test", "location": "Loc", "surveyor": "S", "contact": "C"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sites/", strings.NewReader(body))
	w := httptest.NewRecorder()
	server.createSite(w, req)

	dbInst.Close()

	body2 := `{"name": "Updated", "location": "Updated Loc"}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/sites/1", strings.NewReader(body2))
	w2 := httptest.NewRecorder()
	server.updateSite(w2, req2, 1)

	if w2.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w2.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestDeleteSite_NotFound tests deleteSite when site doesn't exist.
func TestDeleteSite_NotFoundCoverage(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/99999", nil)
	w := httptest.NewRecorder()
	server.deleteSite(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// TestDeleteSite_DBError tests deleteSite when DB is closed.
func TestDeleteSite_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/1", nil)
	w := httptest.NewRecorder()
	server.deleteSite(w, req, 1)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestGetSite_NotFound tests getSite when site doesn't exist.
func TestGetSite_NotFoundCoverage(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/99999", nil)
	w := httptest.NewRecorder()
	server.getSite(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// TestGetReport_NotFound tests getReport when report doesn't exist.
func TestGetReport_NotFoundCoverage(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/99999", nil)
	w := httptest.NewRecorder()
	server.getReport(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// TestDeleteReport_NotFound tests deleteReport when report doesn't exist.
func TestDeleteReport_NotFoundCoverage(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/reports/99999", nil)
	w := httptest.NewRecorder()
	server.deleteReport(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// TestHandleReports_InvalidSiteID tests handleReports with bad site ID.
func TestHandleReports_InvalidSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/site/abc", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestHandleReports_SiteReportsNonGET tests handleReports with non-GET for site reports.
func TestHandleReports_SiteReportsNonGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/site/1", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

// TestHandleReports_InvalidReportID tests handleReports with bad report ID.
func TestHandleReports_InvalidReportID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/abc", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestHandleReports_DownloadNonGET tests handleReports download with non-GET.
func TestHandleReports_DownloadNonGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/1/download", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

// TestHandleReports_DownloadWithFilename tests handleReports download with filename in path.
func TestHandleReports_DownloadWithFilename(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// No report exists but we want to exercise the download-with-filename code path
	req := httptest.NewRequest(http.MethodGet, "/api/reports/999/download/report.zip", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	// Should get 404 because report doesn't exist
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for nonexistent report download, got %d", w.Code)
	}
}

// TestHandleReports_DeleteMethodRouting tests handleReports DELETE routing.
func TestHandleReports_DeleteMethodRouting(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/reports/99999", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for nonexistent delete, got %d", w.Code)
	}
}

// TestHandleReports_UnsupportedMethod tests handleReports with PATCH.
func TestHandleReports_PatchMethod(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPatch, "/api/reports/1", nil)
	w := httptest.NewRecorder()
	server.handleReports(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

// TestListEvents_InvalidTimezone tests listEvents with invalid timezone parameter.
func TestListEvents_BadTimezone(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/events?timezone=invalid/zone", nil)
	w := httptest.NewRecorder()
	server.listEvents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestListEvents_InvalidUnits tests listEvents with invalid units parameter.
func TestListEvents_InvalidUnits(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/events?units=invalid", nil)
	w := httptest.NewRecorder()
	server.listEvents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestGenerateReport_InvalidJSON tests generateReport with malformed JSON body.
func TestGenerateReport_InvalidJSONBody(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", strings.NewReader("{bad json"))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestGenerateReport_MissingDates tests generateReport with empty dates.
func TestGenerateReport_EmptyDates(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body := `{"start_date": "", "end_date": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", strings.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestGenerateReport_SiteLoadFailure tests generateReport when site doesn't exist.
func TestGenerateReport_SiteLoadFailure(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	siteID := 99999
	body, _ := json.Marshal(map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
		"site_id":    siteID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for bad site_id, got %d", w.Code)
	}
}

// TestGenerateReport_NoActivePeriod tests generateReport when site has no active config period.
func TestGenerateReport_NoActivePeriod(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site but no config period
	siteBody := `{"name": "Test", "location": "Loc", "surveyor": "S", "contact": "C"}`
	reqSite := httptest.NewRequest(http.MethodPost, "/api/sites/", strings.NewReader(siteBody))
	wSite := httptest.NewRecorder()
	server.createSite(wSite, reqSite)

	var createdSite struct{ ID int }
	json.NewDecoder(wSite.Body).Decode(&createdSite)

	body, _ := json.Marshal(map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
		"site_id":    createdSite.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing config period, got %d", w.Code)
	}
}

// TestDownloadReport_NotFoundReport tests downloadReport when report doesn't exist.
func TestDownloadReport_NotFoundReport(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/99999/download?file_type=pdf", nil)
	w := httptest.NewRecorder()
	server.downloadReport(w, req, 99999, "pdf")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// TestDownloadReport_DBError tests downloadReport when DB is closed.
func TestDownloadReport_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/download", nil)
	w := httptest.NewRecorder()
	server.downloadReport(w, req, 1, "pdf")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestHandleTimeline_DBErrorConfigPeriods tests handleTimeline when
// GetSiteConfigPeriods fails (close DB after RadarDataRange succeeds).
func TestHandleTimeline_DBErrorConfigPeriods(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"

	// Create a site first so we have valid site_id
	siteBody := `{"name": "Test", "location": "Loc", "surveyor": "S", "contact": "C"}`
	reqSite := httptest.NewRequest(http.MethodPost, "/api/sites/", strings.NewReader(siteBody))
	wSite := httptest.NewRecorder()
	server.createSite(wSite, reqSite)

	// Close DB to make all queries fail
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/timeline?site_id=1", nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestShowConfig_ValidGET tests showConfig on successful GET.
func TestShowConfig_ValidGET(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	server.showConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestSendCommandHandler_POSTSuccess tests sendCommandHandler with valid POST.
func TestSendCommandHandler_POSTSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/command?command=test", nil)
	req.Form = map[string][]string{"command": {"test"}}
	w := httptest.NewRecorder()
	server.sendCommandHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestListSiteConfigPeriods_ValidRequest tests listSiteConfigPeriods with valid site_id.
func TestListSiteConfigPeriods_ValidRequest(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	siteBody := `{"name": "Test", "location": "Loc", "surveyor": "S", "contact": "C"}`
	reqSite := httptest.NewRequest(http.MethodPost, "/api/sites/", strings.NewReader(siteBody))
	wSite := httptest.NewRecorder()
	server.createSite(wSite, reqSite)

	req := httptest.NewRequest(http.MethodGet, "/api/site-config-periods?site_id=1", nil)
	w := httptest.NewRecorder()
	server.listSiteConfigPeriods(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestHandleTimeline_ValidRequest tests handleTimeline with valid parameters.
func TestHandleTimeline_ValidRequest(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	siteBody := `{"name": "Test", "location": "Loc", "surveyor": "S", "contact": "C"}`
	reqSite := httptest.NewRequest(http.MethodPost, "/api/sites/", strings.NewReader(siteBody))
	wSite := httptest.NewRecorder()
	server.createSite(wSite, reqSite)

	req := httptest.NewRequest(http.MethodGet, "/api/timeline?site_id=1", nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestGenerateReport_FullFlowCommandFails exercises the generateReport code path
// from site creation through config building and python command execution failure.
// This covers defaults, map fields, compare dates, and command execution error.
func TestGenerateReport_FullFlowCommandFails(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site with map fields populated to exercise lines 1006-1027
	lat := 51.5
	lng := -0.1
	angle := 45.0
	neLat := 51.6
	neLng := 0.0
	swLat := 51.4
	swLng := -0.2
	site := &db.Site{
		Name:       "Test Site",
		Location:   "Test Location",
		Surveyor:   "Tester",
		Contact:    "test@test.com",
		Latitude:   &lat,
		Longitude:  &lng,
		MapAngle:   &angle,
		BBoxNELat:  &neLat,
		BBoxNELng:  &neLng,
		BBoxSWLat:  &swLat,
		BBoxSWLng:  &swLng,
		IncludeMap: true,
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	// Create active config period
	notes := "test config"
	if err := dbInst.CreateSiteConfigPeriod(&db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		IsActive:           true,
		Notes:              &notes,
	}); err != nil {
		t.Fatalf("create config period: %v", err)
	}

	// Set PDF_GENERATOR_PYTHON to a failing command
	oldPy := os.Getenv("PDF_GENERATOR_PYTHON")
	os.Setenv("PDF_GENERATOR_PYTHON", "/usr/bin/false")
	defer func() {
		if oldPy == "" {
			os.Unsetenv("PDF_GENERATOR_PYTHON")
		} else {
			os.Setenv("PDF_GENERATOR_PYTHON", oldPy)
		}
	}()

	body, _ := json.Marshal(map[string]interface{}{
		"start_date":         "2025-01-01",
		"end_date":           "2025-01-31",
		"site_id":            site.ID,
		"compare_start_date": "2024-12-01",
		"compare_end_date":   "2024-12-31",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	// Should fail at command execution
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500 from failed command, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGenerateReport_DefaultSiteFields exercises the generateReport code path
// where site has empty location/surveyor/contact to trigger the default values.
func TestGenerateReport_DefaultSiteFields(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site with minimal fields (location/surveyor/contact empty)
	site := &db.Site{
		Name:     "Bare Site",
		Location: "",
		Surveyor: "",
		Contact:  "",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	notes := "test"
	if err := dbInst.CreateSiteConfigPeriod(&db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		IsActive:           true,
		Notes:              &notes,
	}); err != nil {
		t.Fatalf("create config period: %v", err)
	}

	oldPy := os.Getenv("PDF_GENERATOR_PYTHON")
	os.Setenv("PDF_GENERATOR_PYTHON", "/usr/bin/false")
	defer func() {
		if oldPy == "" {
			os.Unsetenv("PDF_GENERATOR_PYTHON")
		} else {
			os.Setenv("PDF_GENERATOR_PYTHON", oldPy)
		}
	}()

	body, _ := json.Marshal(map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
		"site_id":    site.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// TestShowRadarObjectStats_DBError tests the DB error path in showRadarObjectStats.
func TestShowRadarObjectStats_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/radar-stats?start=1000&end=2000", nil)
	w := httptest.NewRecorder()
	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}

	cleanupClosedDB(t, fname)
}

// TestShowRadarObjectStats_InvalidMinSpeed tests the invalid min_speed path.
func TestShowRadarObjectStats_InvalidMinSpeed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/radar-stats?start=1000&end=2000&min_speed=abc", nil)
	w := httptest.NewRecorder()
	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestShowRadarObjectStats_InvalidHistBucketSize tests invalid hist_bucket_size.
func TestShowRadarObjectStats_BadHistBucketSize(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/radar-stats?start=1000&end=2000&hist_bucket_size=abc", nil)
	w := httptest.NewRecorder()
	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestShowRadarObjectStats_InvalidHistMax tests invalid hist_max.
func TestShowRadarObjectStats_BadHistMax(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/radar-stats?start=1000&end=2000&hist_max=abc", nil)
	w := httptest.NewRecorder()
	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestDownloadReport_ZipNotAvailable tests downloadReport when zip not available.
func TestDownloadReport_ZipMissing(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2025-01-01",
		EndDate:   "2025-01-31",
		Filepath:  "output/test/report.pdf",
		Filename:  "report.pdf",
		RunID:     "test-run",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/download?file_type=zip", nil)
	w := httptest.NewRecorder()
	server.downloadReport(w, req, report.ID, "zip")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for missing zip, got %d", w.Code)
	}
}

// TestDownloadReport_FileNotFound tests downloadReport when file doesn't exist on disk.
func TestDownloadReport_FileNotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	zipPath := "output/test/report.zip"
	zipName := "report.zip"
	report := &db.SiteReport{
		SiteID:      0,
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-31",
		Filepath:    "output/test/report.pdf",
		Filename:    "report.pdf",
		ZipFilepath: &zipPath,
		ZipFilename: &zipName,
		RunID:       "test-run",
		Timezone:    "UTC",
		Units:       "mph",
		Source:      "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("create report: %v", err)
	}

	// Try PDF download - file won't exist (may get 403 from path validation or 404)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/download?file_type=pdf", nil)
	w := httptest.NewRecorder()
	server.downloadReport(w, req, report.ID, "pdf")

	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("Expected 404 or 403 for missing PDF file, got %d", w.Code)
	}

	// Try ZIP download - file won't exist (may get 403 from path validation or 404)
	req2 := httptest.NewRequest(http.MethodGet, "/api/reports/1/download?file_type=zip", nil)
	w2 := httptest.NewRecorder()
	server.downloadReport(w2, req2, report.ID, "zip")

	if w2.Code != http.StatusNotFound && w2.Code != http.StatusForbidden {
		t.Errorf("Expected 404 or 403 for missing ZIP file, got %d", w2.Code)
	}
}

// TestHandleDatabaseStats_Success tests handleDatabaseStats on a healthy DB.
func TestHandleDatabaseStats_Healthy(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/database-stats", nil)
	w := httptest.NewRecorder()
	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestListEvents_ValidWithParams tests listEvents with valid units and timezone.
func TestListEvents_ValidWithParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/events?units=kph&timezone=Europe/London", nil)
	w := httptest.NewRecorder()
	server.listEvents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestGetPDFGeneratorDir_EnvOverride tests getPDFGeneratorDir with env var set.
func TestGetPDFGeneratorDir_EnvOverride(t *testing.T) {
	old := os.Getenv("PDF_GENERATOR_DIR")
	os.Setenv("PDF_GENERATOR_DIR", "/tmp/test-pdf-dir")
	defer func() {
		if old == "" {
			os.Unsetenv("PDF_GENERATOR_DIR")
		} else {
			os.Setenv("PDF_GENERATOR_DIR", old)
		}
	}()

	dir, err := getPDFGeneratorDir()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if dir != "/tmp/test-pdf-dir" {
		t.Errorf("Expected /tmp/test-pdf-dir, got %s", dir)
	}
}

// TestListSiteConfigPeriods_NegativeSiteID tests negative site_id handling.
func TestListSiteConfigPeriods_NegativeSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/site-config-periods?site_id=-1", nil)
	w := httptest.NewRecorder()
	server.listSiteConfigPeriods(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestUpsertSiteConfigPeriod_InvalidJSON tests upsertSiteConfigPeriod with bad JSON.
func TestUpsertSiteConfigPeriod_BadJSON(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/site-config-periods", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	server.upsertSiteConfigPeriod(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestHandleSiteConfigPeriods_MethodNotAllowed tests DELETE on site config periods.
func TestHandleSiteConfigPeriods_DeleteNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/site-config-periods", nil)
	w := httptest.NewRecorder()
	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

// TestGenerateReport_PythonDiscovery exercises the python binary discovery path
// in generateReport when PDF_GENERATOR_PYTHON is not set.
func TestGenerateReport_PythonDiscovery(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:     "Discovery Site",
		Location: "Loc",
		Surveyor: "S",
		Contact:  "C",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	notes := "test"
	if err := dbInst.CreateSiteConfigPeriod(&db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		IsActive:           true,
		Notes:              &notes,
	}); err != nil {
		t.Fatalf("create config period: %v", err)
	}

	// Clear PDF_GENERATOR_PYTHON to exercise the discovery code path
	oldPy := os.Getenv("PDF_GENERATOR_PYTHON")
	os.Unsetenv("PDF_GENERATOR_PYTHON")
	defer func() {
		if oldPy != "" {
			os.Setenv("PDF_GENERATOR_PYTHON", oldPy)
		}
	}()

	body, _ := json.Marshal(map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
		"site_id":    site.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/generate-report", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.generateReport(w, req)

	// Will fail at PDF generation (no data) but exercises the path discovery code
	if w.Code == http.StatusOK {
		t.Error("Expected non-200 status from report generation without data")
	}
}

// TestDownloadReport_SuccessfulPDFDownload exercises the successful download path
// by creating a real file on disk.
func TestDownloadReport_SuccessfulPDFDownload(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a temp directory structure for the PDF
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output", "test-run")
	os.MkdirAll(outputDir, 0755)

	// Create a fake PDF file
	pdfContent := []byte("%PDF-1.4 fake content")
	pdfFilename := "report.pdf"
	pdfPath := filepath.Join(outputDir, pdfFilename)
	os.WriteFile(pdfPath, pdfContent, 0644)

	// Create a fake ZIP file
	zipContent := []byte("PK fake zip")
	zipFilename := "report.zip"
	zipPath := filepath.Join(outputDir, zipFilename)
	os.WriteFile(zipPath, zipContent, 0644)

	// Use relative paths from tmpDir
	relPdfPath := "output/test-run/report.pdf"
	relZipPath := "output/test-run/report.zip"

	report := &db.SiteReport{
		SiteID:      0,
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-31",
		Filepath:    relPdfPath,
		Filename:    pdfFilename,
		ZipFilepath: &relZipPath,
		ZipFilename: &zipFilename,
		RunID:       "test-run",
		Timezone:    "UTC",
		Units:       "mph",
		Source:      "radar_objects",
	}
	if err := dbInst.CreateSiteReport(report); err != nil {
		t.Fatalf("create report: %v", err)
	}

	// Set PDF_GENERATOR_DIR to point to our temp directory
	old := os.Getenv("PDF_GENERATOR_DIR")
	os.Setenv("PDF_GENERATOR_DIR", tmpDir)
	defer func() {
		if old == "" {
			os.Unsetenv("PDF_GENERATOR_DIR")
		} else {
			os.Setenv("PDF_GENERATOR_DIR", old)
		}
	}()

	// Download PDF
	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/download?file_type=pdf", nil)
	w := httptest.NewRecorder()
	server.downloadReport(w, req, report.ID, "pdf")

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for PDF download, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.Len() != len(pdfContent) {
		t.Errorf("Expected %d bytes, got %d", len(pdfContent), w.Body.Len())
	}

	// Download ZIP
	req2 := httptest.NewRequest(http.MethodGet, "/api/reports/1/download?file_type=zip", nil)
	w2 := httptest.NewRecorder()
	server.downloadReport(w2, req2, report.ID, "zip")

	if w2.Code != http.StatusOK {
		t.Errorf("Expected 200 for ZIP download, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestListSiteReports_Success tests listSiteReports happy path.
func TestListSiteReports_HappyPath(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{Name: "Test", Location: "Loc", Surveyor: "S", Contact: "C"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reports/site/1", nil)
	w := httptest.NewRecorder()
	server.listSiteReports(w, req, site.ID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestListAllReports_Success tests listAllReports happy path.
func TestListAllReports_HappyPath(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	w := httptest.NewRecorder()
	server.listAllReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// failWriter is a ResponseWriter that fails on Write to trigger json.Encode errors.
type failWriter struct {
	header     http.Header
	statusCode int
	writeFail  bool
}

func (fw *failWriter) Header() http.Header {
	if fw.header == nil {
		fw.header = make(http.Header)
	}
	return fw.header
}

func (fw *failWriter) Write(b []byte) (int, error) {
	if fw.writeFail {
		return 0, fmt.Errorf("simulated write failure")
	}
	return len(b), nil
}

func (fw *failWriter) WriteHeader(statusCode int) {
	fw.statusCode = statusCode
}

// TestListSites_EncodeError tests listSites json encode error path.
func TestListSites_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/sites/", nil)
	server.listSites(fw, req)
}

// TestGetSite_EncodeError tests getSite json encode error path.
func TestGetSite_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	site := &db.Site{Name: "Test", Location: "Loc", Surveyor: "S", Contact: "C"}
	dbInst.CreateSite(site)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/sites/1", nil)
	server.getSite(fw, req, site.ID)
}

// TestCreateSite_EncodeError tests createSite json encode error path.
func TestCreateSite_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	body := `{"name": "Test", "location": "Loc", "surveyor": "S", "contact": "C"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sites/", strings.NewReader(body))
	server.createSite(fw, req)
}

// TestUpdateSite_EncodeError tests updateSite json encode error path.
func TestUpdateSite_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{Name: "Test", Location: "Loc", Surveyor: "S", Contact: "C"}
	dbInst.CreateSite(site)

	fw := &failWriter{writeFail: true}
	body := `{"name": "Updated", "location": "Updated Loc"}`
	req := httptest.NewRequest(http.MethodPut, "/api/sites/1", strings.NewReader(body))
	server.updateSite(fw, req, site.ID)
}

// TestListSiteConfigPeriods_EncodeError tests listSiteConfigPeriods json encode error.
func TestListSiteConfigPeriods_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{Name: "Test", Location: "Loc", Surveyor: "S", Contact: "C"}
	dbInst.CreateSite(site)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/site-config-periods?site_id=1", nil)
	server.listSiteConfigPeriods(fw, req)
}

// TestHandleTimeline_EncodeError tests handleTimeline json encode error.
func TestHandleTimeline_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{Name: "Test", Location: "Loc", Surveyor: "S", Contact: "C"}
	dbInst.CreateSite(site)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/timeline?site_id=1", nil)
	server.handleTimeline(fw, req)
}

// TestListEvents_EncodeError tests listEvents json encode error.
func TestListEvents_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	server.listEvents(fw, req)
}

// TestListAllReports_EncodeError tests listAllReports json encode error.
func TestListAllReports_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	server.listAllReports(fw, req)
}

// TestListSiteReports_EncodeError tests listSiteReports json encode error.
func TestListSiteReports_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/reports/site/1", nil)
	server.listSiteReports(fw, req, 1)
}

// TestGetReport_EncodeError tests getReport json encode error.
func TestGetReport_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	report := &db.SiteReport{
		SiteID:    0,
		StartDate: "2025-01-01",
		EndDate:   "2025-01-31",
		Filepath:  "output/test/report.pdf",
		Filename:  "report.pdf",
		RunID:     "test-run",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	dbInst.CreateSiteReport(report)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/reports/1", nil)
	server.getReport(fw, req, report.ID)
}

// TestHandleDatabaseStats_EncodeError tests handleDatabaseStats json encode error.
func TestHandleDatabaseStats_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/database-stats", nil)
	server.handleDatabaseStats(fw, req)
}

// TestShowConfig_EncodeError tests showConfig json encode error.
func TestShowConfig_EncodeError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	fw := &failWriter{writeFail: true}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	server.showConfig(fw, req)
}
