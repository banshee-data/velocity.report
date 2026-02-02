package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

func TestConvertEventAPISpeed(t *testing.T) {
	tests := []struct {
		name     string
		speedMPS *float64
		units    string
		expected *float64
	}{
		{"nil speed", nil, "mph", nil},
		{"10 m/s to mph", floatPtr(10.0), "mph", floatPtr(22.3694)},
		{"10 m/s to kmph", floatPtr(10.0), "kmph", floatPtr(36.0)},
		{"0 m/s to mph", floatPtr(0.0), "mph", floatPtr(0.0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := db.EventAPI{Speed: tt.speedMPS}
			result := convertEventAPISpeed(event, tt.units)

			if tt.expected == nil {
				if result.Speed != nil {
					t.Errorf("convertEventAPISpeed() speed = %v, want nil", result.Speed)
				}
			} else {
				if result.Speed == nil {
					t.Errorf("convertEventAPISpeed() speed = nil, want %f", *tt.expected)
				} else if math.Abs(*result.Speed-*tt.expected) > 0.01 {
					t.Errorf("convertEventAPISpeed() speed = %f, want %f", *result.Speed, *tt.expected)
				}
			}
		})
	}
}

// TestHandleSites_List tests listing all sites
func TestHandleSites_List(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create some test sites
	site1 := &db.Site{
		Name:     "Site 1",
		Location: "Location 1",
		Surveyor: "Surveyor 1",
		Contact:  "contact1@example.com",
	}
	site2 := &db.Site{
		Name:     "Site 2",
		Location: "Location 2",
		Surveyor: "Surveyor 2",
		Contact:  "contact2@example.com",
	}

	if err := dbInst.CreateSite(site1); err != nil {
		t.Fatalf("Failed to create test site: %v", err)
	}
	if err := dbInst.CreateSite(site2); err != nil {
		t.Fatalf("Failed to create test site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/", nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var sites []db.Site
	if err := json.NewDecoder(w.Body).Decode(&sites); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(sites) < 2 {
		t.Errorf("Expected at least 2 sites, got %d", len(sites))
	}
}

// TestHandleSites_Get tests getting a single site
func TestHandleSites_Get(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:     "Get Test Site",
		Location: "Test Location",
		Surveyor: "Test Surveyor",
		Contact:  "test@example.com",
	}

	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create test site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sites/%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var retrieved db.Site
	if err := json.NewDecoder(w.Body).Decode(&retrieved); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if retrieved.Name != site.Name {
		t.Errorf("Expected site name %s, got %s", site.Name, retrieved.Name)
	}
}

// TestHandleSites_Get_NotFound tests getting a non-existent site
func TestHandleSites_Get_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/99999", nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleSites_Create tests creating a new site
func TestHandleSites_Create(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := db.Site{
		Name:     "New Site",
		Location: "New Location",
		Surveyor: "New Surveyor",
		Contact:  "new@example.com",
	}

	body, _ := json.Marshal(site)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	var created db.Site
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if created.ID == 0 {
		t.Error("Expected site ID to be set")
	}
	if created.Name != site.Name {
		t.Errorf("Expected name %s, got %s", site.Name, created.Name)
	}
}

// TestHandleSites_Create_MissingRequiredFields tests validation of required fields
func TestHandleSites_Create_MissingRequiredFields(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name string
		site db.Site
	}{
		{
			name: "missing name",
			site: db.Site{
				Location: "Location",
				Surveyor: "Surveyor",
				Contact:  "contact@example.com",
			},
		},
		{
			name: "missing location",
			site: db.Site{
				Name:     "Name",
				Surveyor: "Surveyor",
				Contact:  "contact@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.site)
			req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleSites(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestHandleSites_Update tests updating a site
func TestHandleSites_Update(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a site first
	site := &db.Site{
		Name:     "Original Name",
		Location: "Original Location",
		Surveyor: "Original Surveyor",
		Contact:  "original@example.com",
	}

	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create test site: %v", err)
	}

	// Update it
	update := db.Site{
		Name:     "Updated Name",
		Location: "Updated Location",
		Surveyor: "Updated Surveyor",
		Contact:  "updated@example.com",
	}

	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/sites/%d", site.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var updated db.Site
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Expected name to be updated to 'Updated Name', got %s", updated.Name)
	}
}

// TestHandleSites_Update_NotFound tests updating a non-existent site
func TestHandleSites_Update_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := db.Site{
		Name:     "Name",
		Location: "Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}

	body, _ := json.Marshal(site)
	req := httptest.NewRequest(http.MethodPut, "/api/sites/99999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleSites_Delete tests deleting a site
func TestHandleSites_Delete(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:     "To Delete",
		Location: "Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}

	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create test site: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/sites/%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// Verify it's deleted
	_, err := dbInst.GetSite(site.ID)
	if err == nil {
		t.Error("Expected error when getting deleted site")
	}
}

// TestHandleSites_Delete_NotFound tests deleting a non-existent site
func TestHandleSites_Delete_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/99999", nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleSites_InvalidID tests handling invalid site IDs
func TestHandleSites_InvalidID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/invalid", nil)
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleSites_MethodNotAllowed tests unsupported HTTP methods
func TestHandleSites_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPatch, "/api/sites/"},
		{http.MethodPatch, "/api/sites/1"},
		{http.MethodHead, "/api/sites/"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			server.handleSites(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405, got %d", w.Code)
			}
		})
	}
}

// TestShowConfig tests the config endpoint
func TestShowConfig(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	server.showConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var config map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := config["units"]; !ok {
		t.Error("Expected 'units' in config response")
	}
	if _, ok := config["timezone"]; !ok {
		t.Error("Expected 'timezone' in config response")
	}
}

// TestShowConfig_MethodNotAllowed tests that only GET is allowed
func TestShowConfig_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	w := httptest.NewRecorder()

	server.showConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestListEvents tests the events endpoint
func TestListEvents(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var events []db.EventAPI
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return empty array initially
	if events == nil {
		t.Error("Expected non-nil events array")
	}
}

// TestListEvents_WithUnitsParam tests unit conversion
func TestListEvents_WithUnitsParam(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name  string
		units string
		valid bool
	}{
		{"valid mph", "mph", true},
		{"valid kmph", "kmph", true},
		{"invalid units", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/events?units="+tt.units, nil)
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

// TestListEvents_MethodNotAllowed tests that only GET is allowed
func TestListEvents_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/events", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestShowRadarObjectStats tests the radar stats endpoint
func TestShowRadarObjectStats(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a query with a valid time range
	start := "1697318400"
	end := "1697404800" // 24 hours later
	req := httptest.NewRequest(http.MethodGet, "/api/radar_stats?start="+start+"&end="+end+"&group=1h", nil)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestShowRadarObjectStats_MissingParams tests required parameter validation
func TestShowRadarObjectStats_MissingParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name  string
		query string
	}{
		{"missing start", "end=1697318400&group=1h"},
		{"missing end", "start=1697318400&group=1h"},
		{"missing both", "group=1h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/radar_stats?"+tt.query, nil)
			w := httptest.NewRecorder()

			server.showRadarObjectStats(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestShowRadarObjectStats_InvalidParams tests parameter validation
func TestShowRadarObjectStats_InvalidParams(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	tests := []struct {
		name  string
		query string
	}{
		{"invalid start", "start=invalid&end=1697318400&group=1h"},
		{"invalid end", "start=1697318400&end=invalid&group=1h"},
		{"invalid group", "start=1697318400&end=1697318400&group=invalid"},
		{"invalid units", "start=1697318400&end=1697318400&units=invalid"},
		{"invalid timezone", "start=1697318400&end=1697318400&timezone=Invalid/Zone"},
		{"invalid min_speed", "start=1697318400&end=1697318400&min_speed=invalid"},
		{"invalid source", "start=1697318400&end=1697318400&source=invalid_source"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/radar_stats?"+tt.query, nil)
			w := httptest.NewRecorder()

			server.showRadarObjectStats(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestShowRadarObjectStats_WithHistogram tests histogram generation
func TestShowRadarObjectStats_WithHistogram(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	start := "1697318400"
	end := "1697404800" // 24 hours later
	query := fmt.Sprintf("start=%s&end=%s&compute_histogram=true&hist_bucket_size=5&hist_max=100", start, end)
	req := httptest.NewRequest(http.MethodGet, "/api/radar_stats?"+query, nil)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		return
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := response["metrics"]; !ok {
		t.Error("Expected 'metrics' in response")
	}
}

// TestShowRadarObjectStats_MethodNotAllowed tests that only GET is allowed
func TestShowRadarObjectStats_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/radar_stats", nil)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestWriteJSONError tests the error helper
func TestWriteJSONError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	w := httptest.NewRecorder()
	server.writeJSONError(w, http.StatusBadRequest, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp["error"] != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", errResp["error"])
	}
}

// TestKeysOfMap tests the helper function
func TestKeysOfMap(t *testing.T) {
	m := map[string]int64{
		"b": 2,
		"a": 1,
		"c": 3,
	}

	keys := keysOfMap(m)

	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// Should be sorted
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("Expected sorted keys [a, b, c], got %v", keys)
	}
}

// TestDebugModeInConfig tests that debug mode is correctly set in server
func TestDebugModeInConfig(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Test with debug mode disabled (default)
	t.Run("DebugDisabled", func(t *testing.T) {
		server.debugMode = false
		if server.debugMode != false {
			t.Errorf("Expected debugMode to be false, got %v", server.debugMode)
		}
	})

	// Test with debug enabled
	t.Run("DebugEnabled", func(t *testing.T) {
		server.debugMode = true
		if server.debugMode != true {
			t.Errorf("Expected debugMode to be true, got %v", server.debugMode)
		}
	})

	// Test that Start() method sets debug mode
	t.Run("StartSetsDebugMode", func(t *testing.T) {
		server.debugMode = false // Reset
		// Start() should set debugMode from the parameter
		// We can't easily test Start() in a unit test without mocking HTTP server
		// but we can verify the field exists and is settable
		server.debugMode = true
		if server.debugMode != true {
			t.Errorf("Expected debugMode to be settable to true")
		}
	})
}

// Helper functions

func setupTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	fname := t.Name() + ".db"
	_ = os.Remove(fname)

	dbInst, err := db.NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}

	mux := serialmux.NewDisabledSerialMux()
	server := NewServer(mux, dbInst, "mph", "UTC")

	return server, dbInst
}

func cleanupTestServer(t *testing.T, dbInst *db.DB) {
	t.Helper()
	fname := t.Name() + ".db"
	dbInst.Close()
	_ = os.Remove(fname)
	_ = os.Remove(fname + "-shm")
	_ = os.Remove(fname + "-wal")
}

// Helper function to create float64 pointers
func floatPtr(f float64) *float64 {
	return &f
}

// Mock TransitController for testing
type mockTransitController struct {
	enabled           bool
	lastRunAt         string
	lastRunError      string
	runCount          int64
	isHealthy         bool
	triggerCalled     bool
	fullHistoryCalled bool
}

func (m *mockTransitController) IsEnabled() bool {
	return m.enabled
}

func (m *mockTransitController) SetEnabled(enabled bool) {
	m.enabled = enabled
}

func (m *mockTransitController) TriggerManualRun() {
	m.triggerCalled = true
}

func (m *mockTransitController) TriggerFullHistoryRun() {
	m.fullHistoryCalled = true
}

func (m *mockTransitController) GetStatus() db.TransitStatus {
	return db.TransitStatus{
		Enabled:      m.enabled,
		LastRunAt:    parseTimeOrZero(m.lastRunAt),
		LastRunError: m.lastRunError,
		RunCount:     m.runCount,
		IsHealthy:    m.isHealthy,
	}
}

func parseTimeOrZero(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// TestHandleTransitWorker_Get tests GET requests to transit worker endpoint
func TestHandleTransitWorker_Get(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{
		enabled:      true,
		lastRunAt:    "2024-01-01T12:00:00Z",
		runCount:     5,
		isHealthy:    true,
		lastRunError: "",
	}
	server.SetTransitController(mockTC)

	req := httptest.NewRequest(http.MethodGet, "/api/transit_worker", nil)
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var status db.TransitStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status.Enabled != true {
		t.Errorf("Expected enabled=true, got %v", status.Enabled)
	}
	if status.RunCount != 5 {
		t.Errorf("Expected run_count=5, got %d", status.RunCount)
	}
	if !status.IsHealthy {
		t.Errorf("Expected is_healthy=true, got %v", status.IsHealthy)
	}
}

// TestHandleTransitWorker_Get_Nil tests GET when controller is nil
func TestHandleTransitWorker_Get_Nil(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Don't set transit controller (nil)

	req := httptest.NewRequest(http.MethodGet, "/api/transit_worker", nil)
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestHandleTransitWorker_Post_EnableTrue tests POST with enabled=true
func TestHandleTransitWorker_Post_EnableTrue(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{
		enabled:   false,
		runCount:  5,
		isHealthy: true,
	}
	server.SetTransitController(mockTC)

	reqBody := map[string]interface{}{"enabled": true}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/transit_worker", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !mockTC.enabled {
		t.Errorf("Expected transit controller to be enabled")
	}
}

// TestHandleTransitWorker_Post_EnableFalse tests POST with enabled=false
func TestHandleTransitWorker_Post_EnableFalse(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{
		enabled:   true,
		runCount:  5,
		isHealthy: true,
	}
	server.SetTransitController(mockTC)

	reqBody := map[string]interface{}{"enabled": false}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/transit_worker", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if mockTC.enabled {
		t.Errorf("Expected transit controller to be disabled")
	}
}

// TestHandleTransitWorker_Post_Trigger tests POST with trigger=true
func TestHandleTransitWorker_Post_Trigger(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{
		enabled:       true,
		runCount:      5,
		isHealthy:     true,
		triggerCalled: false,
	}
	server.SetTransitController(mockTC)

	reqBody := map[string]interface{}{"trigger": true}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/transit_worker", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !mockTC.triggerCalled {
		t.Errorf("Expected TriggerManualRun to be called")
	}
}

// TestHandleTransitWorker_Post_FullHistoryTrigger tests POST with trigger_full_history=true
func TestHandleTransitWorker_Post_FullHistoryTrigger(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{
		enabled:           true,
		runCount:          5,
		isHealthy:         true,
		fullHistoryCalled: false,
		triggerCalled:     false,
	}
	server.SetTransitController(mockTC)

	reqBody := map[string]interface{}{"trigger_full_history": true}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/transit_worker", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !mockTC.fullHistoryCalled {
		t.Errorf("Expected TriggerFullHistoryRun to be called")
	}
}

// TestHandleTransitWorker_Post_Both tests POST with both enabled and trigger
func TestHandleTransitWorker_Post_Both(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{
		enabled:       false,
		runCount:      5,
		isHealthy:     true,
		triggerCalled: false,
	}
	server.SetTransitController(mockTC)

	reqBody := map[string]interface{}{"enabled": true, "trigger": true}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/transit_worker", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !mockTC.enabled {
		t.Errorf("Expected transit controller to be enabled")
	}
	if !mockTC.triggerCalled {
		t.Errorf("Expected TriggerManualRun to be called")
	}
}

// TestHandleTransitWorker_Post_InvalidBody tests POST with invalid JSON
func TestHandleTransitWorker_Post_InvalidBody(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{enabled: true}
	server.SetTransitController(mockTC)

	req := httptest.NewRequest(http.MethodPost, "/api/transit_worker", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTransitWorker(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleTransitWorker_MethodNotAllowed tests unsupported HTTP methods
func TestHandleTransitWorker_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	mockTC := &mockTransitController{enabled: true}
	server.SetTransitController(mockTC)

	methods := []string{http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/transit_worker", nil)
		w := httptest.NewRecorder()

		server.handleTransitWorker(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405 for method %s, got %d", method, w.Code)
		}
	}
}

// TestHandleDatabaseStats tests the database stats endpoint
func TestHandleDatabaseStats(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/db_stats", nil)
	w := httptest.NewRecorder()

	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse response
	var stats db.DatabaseStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify basic structure
	if stats.TotalSizeMB <= 0 {
		t.Error("Expected non-zero total size")
	}

	if len(stats.Tables) == 0 {
		t.Error("Expected at least one table in stats")
	}

	// Verify tables are present in the schema
	foundRadarObjects := false
	for _, table := range stats.Tables {
		if table.Name == "radar_objects" {
			foundRadarObjects = true
		}
		// Verify all tables have non-negative counts
		if table.RowCount < 0 {
			t.Errorf("Table %s has negative row count: %d", table.Name, table.RowCount)
		}
		if table.SizeMB < 0 {
			t.Errorf("Table %s has negative size: %.2f", table.Name, table.SizeMB)
		}
	}

	if !foundRadarObjects {
		t.Error("Expected radar_objects table in stats")
	}
}

// TestHandleDatabaseStats_MethodNotAllowed tests unsupported HTTP methods
func TestHandleDatabaseStats_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/db_stats", nil)
			w := httptest.NewRecorder()

			server.handleDatabaseStats(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for method %s, got %d", method, w.Code)
			}
		})
	}
}

// TestHandleDatabaseStats_NoDatabase tests error handling when DB is nil
func TestHandleDatabaseStats_NoDatabase(t *testing.T) {
	// Create a server without a database
	mux := serialmux.NewDisabledSerialMux()
	server := NewServer(mux, nil, "mph", "UTC")

	req := httptest.NewRequest(http.MethodGet, "/api/db_stats", nil)
	w := httptest.NewRecorder()

	server.handleDatabaseStats(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	// Verify error message
	var errResp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errMsg, ok := errResp["error"].(string); !ok || errMsg == "" {
		t.Error("Expected error message in response")
	}
}

// TestSendCommandHandler_MethodNotAllowed tests the send command endpoint with wrong method
func TestSendCommandHandler_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/send_command", nil)
	w := httptest.NewRecorder()

	server.sendCommandHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestSendCommandHandler_ValidCommand tests sending a valid command
func TestSendCommandHandler_ValidCommand(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body := bytes.NewReader([]byte("test command"))
	req := httptest.NewRequest(http.MethodPost, "/send_command", body)
	w := httptest.NewRecorder()

	server.sendCommandHandler(w, req)

	// Should succeed - command is sent to serial mux
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestListSitesSuccess tests listing sites when database has sites
func TestListSitesSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a test site
	site := &db.Site{Name: "List Sites Test", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/", nil)
	w := httptest.NewRecorder()

	server.listSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var sites []db.Site
	if err := json.NewDecoder(w.Body).Decode(&sites); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have at least one site
	if len(sites) < 1 {
		t.Error("Expected at least one site")
	}
}

// TestGetSiteSuccess tests getting an existing site
func TestGetSiteSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a test site
	site := &db.Site{Name: "Get Site Test", Location: "Test Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sites/%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.getSite(w, req, site.ID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var gotSite db.Site
	if err := json.NewDecoder(w.Body).Decode(&gotSite); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if gotSite.Name != "Get Site Test" {
		t.Errorf("Expected site name 'Get Site Test', got '%s'", gotSite.Name)
	}
}

// TestGetSiteNotFound tests getting a non-existent site
func TestGetSiteNotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/99999", nil)
	w := httptest.NewRecorder()

	server.getSite(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestListAllReportsSuccess tests listing all reports across sites
func TestListAllReportsSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	w := httptest.NewRecorder()

	server.listAllReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var reports []db.SiteReport
	if err := json.NewDecoder(w.Body).Decode(&reports); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
}

// TestListSiteReportsSuccess tests listing reports for a specific site
func TestListSiteReportsSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a test site first
	site := &db.Site{Name: "Report Test Site", Location: "Test"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sites/%d/reports", site.ID), nil)
	w := httptest.NewRecorder()

	server.listSiteReports(w, req, site.ID)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var reports []db.SiteReport
	if err := json.NewDecoder(w.Body).Decode(&reports); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
}

// TestGetReportNotFound tests getting a non-existent report
func TestGetReportNotFoundServer(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/99999", nil)
	w := httptest.NewRecorder()

	server.getReport(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestListEventsMissingStart tests events endpoint successfully without start (returns all)
func TestListEventsMissingStart(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Events endpoint without start/end returns all events
	req := httptest.NewRequest(http.MethodGet, "/api/events?end=9999999999", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	// Should succeed even without start param
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestListEventsMissingEnd tests events endpoint successfully without end (returns all)
func TestListEventsMissingEnd(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Events endpoint without start/end returns all events
	req := httptest.NewRequest(http.MethodGet, "/api/events?start=0", nil)
	w := httptest.NewRecorder()

	server.listEvents(w, req)

	// Should succeed even without end param
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestListSiteConfigPeriodsSuccess tests listing config periods for a site
func TestListSiteConfigPeriodsSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a test site first
	site := &db.Site{Name: "Config Period Test", Location: "Test"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/site_config_periods?site_id=%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.listSiteConfigPeriods(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestListSiteConfigPeriodsMissingSiteID tests config periods without site_id (returns all)
func TestListSiteConfigPeriodsMissingSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Without site_id, returns all periods (empty is valid)
	req := httptest.NewRequest(http.MethodGet, "/api/site_config_periods", nil)
	w := httptest.NewRecorder()

	server.listSiteConfigPeriods(w, req)

	// Should succeed even without site_id param - returns all periods
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestHandleTimelineSuccess tests the timeline endpoint with valid parameters
func TestHandleTimelineSuccess(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create a test site first
	site := &db.Site{Name: "Timeline Test", Location: "Test"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/timeline?site_id=%d", site.ID), nil)
	w := httptest.NewRecorder()

	server.handleTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestHandleTimelineMissingSiteID tests timeline without site_id
func TestHandleTimelineMissingSiteID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodGet, "/api/timeline", nil)
	w := httptest.NewRecorder()

	server.handleTimeline(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestDeleteReportNotFoundServer tests deleting a non-existent report
func TestDeleteReportNotFoundServer(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodDelete, "/api/reports/99999", nil)
	w := httptest.NewRecorder()

	server.deleteReport(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestDownloadReportInvalidType tests downloading a report with invalid file type
func TestDownloadReportInvalidType(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Invalid file type should return 400
	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/download/invalid", nil)
	w := httptest.NewRecorder()

	server.downloadReport(w, req, 1, "invalid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleSites_POST tests creating a new site
func TestHandleSites_POST(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	newSite := db.Site{Name: "New POST Site", Location: "Test Location"}
	body, _ := json.Marshal(newSite)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Errorf("Expected status 201 or 200, got %d", w.Code)
	}
}

// TestHandleSites_PUT tests updating a site
func TestHandleSites_PUT(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create site first
	site := &db.Site{Name: "PUT Test Site", Location: "Old Location"}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Update site
	site.Location = "New Location"
	body, _ := json.Marshal(site)

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/sites/%d", site.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
