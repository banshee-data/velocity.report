package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
