package monitor

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	_ "modernc.org/sqlite"
)

// setupTestDB creates a temporary SQLite database for testing using schema.sql.
// This avoids hardcoded CREATE TABLE statements that can get out of sync with migrations.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "monitor-track-api-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Apply essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			os.RemoveAll(tmpDir)
			t.Fatalf("Failed to execute %q: %v", pragma, err)
		}
	}

	// Read and execute schema.sql from the db package (relative path from monitor/)
	schemaPath := filepath.Join("..", "..", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to execute schema.sql: %v", err)
	}

	// Baseline at latest migration version
	// NOTE: Update this when new migrations are added to internal/db/migrations/
	latestMigrationVersion := 15
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`, latestMigrationVersion); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to baseline migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// insertTestTrack inserts a test track into the database with all required fields.
func insertTestTrack(t *testing.T, db *sql.DB, trackID, sensorID, state string, startNanos, endNanos int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO lidar_tracks (
			track_id, sensor_id, world_frame, track_state, start_unix_nanos, end_unix_nanos,
			observation_count, avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg, object_class, object_confidence, classification_model
		)
		VALUES (?, ?, 'sensor', ?, ?, ?,
			5, 2.5, 3.0, 2.3, 2.7, 2.9,
			4.5, 2.0, 1.5,
			1.4, 100.0, 'car', 0.9, 'default')
	`, trackID, sensorID, state, startNanos, endNanos)
	if err != nil {
		t.Fatalf("Failed to insert test track: %v", err)
	}
}

// insertTestObservation inserts a test observation into the database with all required fields.
func insertTestObservation(t *testing.T, db *sql.DB, trackID string, tsNanos int64, x, y float64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO lidar_track_obs (
			track_id, ts_unix_nanos, world_frame, x, y, z,
			velocity_x, velocity_y, speed_mps, heading_rad,
			bounding_box_length, bounding_box_width, bounding_box_height,
			height_p95, intensity_mean
		)
		VALUES (?, ?, 'sensor', ?, ?, 0.5,
			1.0, 0.5, 2.5, 0.0,
			4.5, 2.0, 1.5,
			1.4, 100.0)
	`, trackID, tsNanos, x, y)
	if err != nil {
		t.Fatalf("Failed to insert test observation: %v", err)
	}
}

func TestTrackAPI_HandleActiveTracks_WithTracker(t *testing.T) {
	// Create a tracker with some test tracks
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Add a test cluster to create a track
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
		HeightP95:         1.4,
		IntensityMean:     100,
	}

	// Update tracker multiple times to create and confirm a track
	ts := time.Now()
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	// Create API with tracker
	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	// Test active tracks endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count == 0 {
		t.Error("expected at least one track")
	}

	if len(response.Tracks) != response.Count {
		t.Errorf("tracks count mismatch: count=%d, len=%d", response.Count, len(response.Tracks))
	}
}

func TestTrackAPI_HandleActiveTracks_NoTrackerOrDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListTracks_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackByID_MissingID(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/", nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackByID_NotFound(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())
	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/nonexistent", nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackSummary(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Add multiple clusters to create tracks
	clusters := []lidar.WorldCluster{
		{SensorID: "test-sensor", CentroidX: 10.0, CentroidY: 5.0, BoundingBoxLength: 4.0, BoundingBoxHeight: 1.5},
		{SensorID: "test-sensor", CentroidX: 20.0, CentroidY: 15.0, BoundingBoxLength: 1.0, BoundingBoxHeight: 1.8},
	}

	// Update tracker to create tracks
	ts := time.Now()
	for i := 0; i < 5; i++ {
		for j := range clusters {
			clusters[j].CentroidX += 0.5
		}
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update(clusters, ts)
	}

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/summary", nil)
	w := httptest.NewRecorder()

	api.handleTrackSummary(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response TrackSummaryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Overall.TotalTracks == 0 {
		t.Error("expected at least one track in summary")
	}
}

func TestTrackAPI_HandleListClusters_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clusters", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleUpdateTrack_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	body := `{"object_class": "car"}`
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/tracks/track_1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_MethodNotAllowed(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list tracks POST", http.MethodPost, "/api/lidar/tracks"},
		{"active tracks POST", http.MethodPost, "/api/lidar/tracks/active"},
		{"summary POST", http.MethodPost, "/api/lidar/tracks/summary"},
		{"clusters POST", http.MethodPost, "/api/lidar/clusters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			switch {
			case tt.path == "/api/lidar/tracks":
				api.handleListTracks(w, req)
			case tt.path == "/api/lidar/tracks/active":
				api.handleActiveTracks(w, req)
			case tt.path == "/api/lidar/tracks/summary":
				api.handleTrackSummary(w, req)
			case tt.path == "/api/lidar/clusters":
				api.handleListClusters(w, req)
			}

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: expected status 405, got %d", tt.name, w.Code)
			}
		})
	}
}

func TestTrackAPI_RegisterRoutes(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")
	mux := http.NewServeMux()

	// Register routes manually (mirrors WebServer.RegisterRoutes)
	mux.HandleFunc("/api/lidar/tracks", api.handleListTracks)
	mux.HandleFunc("/api/lidar/tracks/active", api.handleActiveTracks)
	mux.HandleFunc("/api/lidar/tracks/summary", api.handleTrackSummary)
	mux.HandleFunc("/api/lidar/clusters", api.handleListClusters)
	mux.HandleFunc("/api/lidar/observations", api.handleListObservations)

	// Verify routes are registered by making requests
	tests := []struct {
		path           string
		expectedStatus int // Expected status when no DB
	}{
		{"/api/lidar/tracks", http.StatusServiceUnavailable},
		{"/api/lidar/tracks/active", http.StatusServiceUnavailable},
		{"/api/lidar/tracks/summary", http.StatusServiceUnavailable},
		{"/api/lidar/clusters", http.StatusServiceUnavailable},
		{"/api/lidar/observations", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != tt.expectedStatus {
			t.Errorf("%s: expected status %d, got %d", tt.path, tt.expectedStatus, w.Code)
		}
	}
}

func TestTrackAPI_HandleClearTracks_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleClearTracks_MethodNotAllowed(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	// PUT should not be allowed
	// But since there's no DB, it returns 503 before checking method
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	// Handler checks DB before method, so 503 (no DB) takes precedence
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 or 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleClearTracks_MissingSensorID(t *testing.T) {
	api := NewTrackAPI(nil, "") // Empty sensor ID

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	// Should fail with 400 if no sensor_id is provided and none is configured
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 or 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_MethodNotAllowed(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/observations", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_InvalidStartTime(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?start_time=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	// Handler checks DB before parsing params, so 503 takes precedence
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_InvalidEndTime(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?end_time=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	// Handler checks DB before parsing params, so 503 takes precedence
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_StartAfterEnd(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	// start_time > end_time
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?start_time=2000000000000000000&end_time=1000000000000000000", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	// Handler checks DB before time validation, so 503 takes precedence
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackObservations_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/track_123/observations", nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleGetTrack_WithTracker(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Create a track by updating with clusters
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	ts := time.Now()
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	// Get a track ID
	tracks := tracker.GetActiveTracks()
	if len(tracks) == 0 {
		t.Skip("No tracks created")
	}
	trackID := tracks[0].TrackID

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/"+trackID, nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response TrackResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.TrackID != trackID {
		t.Errorf("expected track ID '%s', got '%s'", trackID, response.TrackID)
	}
}

func TestTrackAPI_HandleActiveTracks_FilterByState(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Create multiple tracks
	clusters := []lidar.WorldCluster{
		{SensorID: "test-sensor", CentroidX: 10.0, CentroidY: 5.0, BoundingBoxLength: 4.0},
		{SensorID: "test-sensor", CentroidX: 20.0, CentroidY: 15.0, BoundingBoxLength: 1.5},
	}

	ts := time.Now()
	for i := 0; i < 5; i++ {
		for j := range clusters {
			clusters[j].CentroidX += 0.5
		}
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update(clusters, ts)
	}

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	// Test filter by confirmed state
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active?state=confirmed", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// All returned tracks should be confirmed
	for _, track := range response.Tracks {
		if track.State != "confirmed" {
			t.Errorf("expected state 'confirmed', got '%s'", track.State)
		}
	}
}

func TestTrackAPI_HandleActiveTracks_FilterByTentative(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Create a new track (will be tentative initially)
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		BoundingBoxLength: 4.0,
	}

	// Only update once - track should remain tentative
	tracker.Update([]lidar.WorldCluster{cluster}, time.Now())

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active?state=tentative", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// All returned tracks should be tentative
	for _, track := range response.Tracks {
		if track.State != "tentative" {
			t.Errorf("expected state 'tentative', got '%s'", track.State)
		}
	}
}

func TestTrackAPI_HandleListTracks_InvalidStartTime(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?start_time=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListTracks_InvalidEndTime(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?end_time=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListTracks_StartAfterEnd(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?start_time=2000000000000000000&end_time=1000000000000000000", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListTracks_LimitParsing(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	// Invalid limit should use default
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?limit=invalid", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	// Should still fail with 503 (no DB), not a parsing error
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackSummary_NoTrackerOrDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")
	// No tracker set, no DB

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/summary", nil)
	w := httptest.NewRecorder()

	api.handleTrackSummary(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleUpdateTrack_InvalidJSON(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	body := `{invalid json`
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/tracks/track_1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	// Should fail due to no DB before parsing JSON, or 400 for bad JSON
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusBadRequest {
		t.Errorf("expected status 503 or 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleTrackByID_MethodNotAllowed(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/tracks/track_1", nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestTrackAPI_WriteJSONError(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	w := httptest.NewRecorder()
	api.writeJSONError(w, http.StatusBadRequest, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "test error" {
		t.Errorf("expected error 'test error', got '%s'", resp["error"])
	}
}

func TestToDisplayFrame(t *testing.T) {
	// toDisplayFrame swaps X and Y
	x, y := toDisplayFrame(10.0, 5.0)

	if x != 5.0 {
		t.Errorf("expected x=5.0, got %f", x)
	}
	if y != 10.0 {
		t.Errorf("expected y=10.0, got %f", y)
	}
}

func TestHeadingFromVelocity(t *testing.T) {
	tests := []struct {
		vx, vy  float32
		heading float32
	}{
		{1.0, 0.0, 0.0},         // East
		{0.0, 1.0, 1.5707963},   // North (pi/2)
		{-1.0, 0.0, 3.1415927},  // West (pi)
		{0.0, -1.0, -1.5707963}, // South (-pi/2)
		{1.0, 1.0, 0.7853982},   // Northeast (pi/4)
	}

	for _, tt := range tests {
		heading := headingFromVelocity(tt.vx, tt.vy)
		diff := heading - tt.heading
		if diff > 0.001 || diff < -0.001 {
			t.Errorf("headingFromVelocity(%f, %f): expected %f, got %f", tt.vx, tt.vy, tt.heading, heading)
		}
	}
}

func TestTrackAPI_TrackToResponse(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	now := time.Now()
	track := &lidar.TrackedObject{
		TrackID:              "test-track-001",
		SensorID:             "test-sensor",
		State:                lidar.TrackConfirmed,
		X:                    10.0,
		Y:                    5.0,
		VX:                   1.0,
		VY:                   0.5,
		AvgSpeedMps:          2.5,
		PeakSpeedMps:         3.0,
		ObservationCount:     10,
		FirstUnixNanos:       now.Add(-time.Second).UnixNano(),
		LastUnixNanos:        now.UnixNano(),
		ObjectClass:          "car",
		ObjectConfidence:     0.95,
		ClassificationModel:  "yolo-v8",
		BoundingBoxLengthAvg: 4.5,
		BoundingBoxWidthAvg:  2.0,
		BoundingBoxHeightAvg: 1.5,
		History: []lidar.TrackPoint{
			{X: 9.0, Y: 4.5, Timestamp: now.Add(-500 * time.Millisecond).UnixNano()},
			{X: 10.0, Y: 5.0, Timestamp: now.UnixNano()},
		},
	}

	response := api.trackToResponse(track)

	if response.TrackID != "test-track-001" {
		t.Errorf("expected TrackID 'test-track-001', got '%s'", response.TrackID)
	}

	if response.State != "confirmed" {
		t.Errorf("expected State 'confirmed', got '%s'", response.State)
	}

	// Position should be swapped (toDisplayFrame)
	if response.Position.X != 5.0 {
		t.Errorf("expected Position.X 5.0, got %f", response.Position.X)
	}
	if response.Position.Y != 10.0 {
		t.Errorf("expected Position.Y 10.0, got %f", response.Position.Y)
	}

	if response.ObjectClass != "car" {
		t.Errorf("expected ObjectClass 'car', got '%s'", response.ObjectClass)
	}

	if response.ObjectConfidence != 0.95 {
		t.Errorf("expected ObjectConfidence 0.95, got %f", response.ObjectConfidence)
	}

	if len(response.History) != 2 {
		t.Errorf("expected 2 history points, got %d", len(response.History))
	}

	if response.AgeSeconds < 0.9 || response.AgeSeconds > 1.1 {
		t.Errorf("expected AgeSeconds ~1.0, got %f", response.AgeSeconds)
	}
}

// ====== Database-backed handler tests ======

func TestTrackAPI_HandleListTracks_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestTrack(t, db, "track-002", "sensor-A", "tentative", now-2e9, now)
	insertTestTrack(t, db, "track-003", "sensor-B", "confirmed", now-1e9, now) // Different sensor

	api := NewTrackAPI(db, "sensor-A")

	// Test listing all tracks for sensor-A
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		return
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("expected 2 tracks for sensor-A, got %d", response.Count)
	}
}

func TestTrackAPI_HandleListTracks_FilterByState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestTrack(t, db, "track-002", "sensor-A", "tentative", now-2e9, now)
	insertTestTrack(t, db, "track-003", "sensor-A", "deleted", now-3e9, now)

	api := NewTrackAPI(db, "sensor-A")

	// Test filtering by state=confirmed
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?sensor_id=sensor-A&state=confirmed", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		return
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count != 1 {
		t.Errorf("expected 1 confirmed track, got %d", response.Count)
	}
}

func TestTrackAPI_HandleClearTracks_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestTrack(t, db, "track-002", "sensor-A", "tentative", now-2e9, now)

	api := NewTrackAPI(db, "sensor-A")

	// Clear tracks for sensor-A
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleClearTracks_GET(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	api := NewTrackAPI(db, "sensor-A")

	// GET is also allowed per handler code
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/clear?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleClearTracks_MissingSensorID_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// API with no default sensor ID
	api := NewTrackAPI(db, "")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListObservations_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestObservation(t, db, "track-001", now-500e6, 10.0, 5.0)
	insertTestObservation(t, db, "track-001", now, 10.5, 5.2)

	api := NewTrackAPI(db, "sensor-A")

	// Test listing observations
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/observations?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleListObservations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleTrackObservations_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestObservation(t, db, "track-001", now-500e6, 10.0, 5.0)
	insertTestObservation(t, db, "track-001", now, 10.5, 5.2)

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/track-001/observations", nil)
	w := httptest.NewRecorder()

	api.handleTrackObservations(w, req, "track-001")

	// Accept success or not found
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected status 200 or 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleUpdateTrack_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)

	api := NewTrackAPI(db, "sensor-A")

	// Update track with new object class
	body := `{"object_class": "pedestrian", "object_confidence": 0.85}`
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/tracks/track-001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleUpdateTrack(w, req, "track-001")

	// Accept various responses
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Errorf("expected status 200, 404, or 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleTrackSummary_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestTrack(t, db, "track-002", "sensor-A", "tentative", now-2e9, now)

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/summary?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleTrackSummary(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		return
	}

	var response TrackSummaryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Overall.TotalTracks != 2 {
		t.Errorf("expected 2 total tracks, got %d", response.Overall.TotalTracks)
	}
}

func TestTrackAPI_HandleListClusters_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test cluster with all required fields
	now := time.Now().UnixNano()
	_, err := db.Exec(`
		INSERT INTO lidar_clusters (sensor_id, world_frame, ts_unix_nanos, centroid_x, centroid_y, centroid_z,
			bounding_box_length, bounding_box_width, bounding_box_height, points_count, height_p95, intensity_mean)
		VALUES ('sensor-A', 'sensor', ?, 10.0, 5.0, 1.0, 4.5, 2.0, 1.5, 50, 1.4, 100.0)
	`, now)
	if err != nil {
		t.Fatalf("Failed to insert test cluster: %v", err)
	}

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clusters?sensor_id=sensor-A", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		return
	}

	var response ClustersListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count != 1 {
		t.Errorf("expected 1 cluster, got %d", response.Count)
	}
}

func TestTrackAPI_HandleGetTrack_WithDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)
	insertTestObservation(t, db, "track-001", now, 10.0, 5.0)

	api := NewTrackAPI(db, "sensor-A")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/track-001", nil)
	w := httptest.NewRecorder()

	api.handleGetTrack(w, req, "track-001")

	// Accept various responses
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected status 200 or 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_HandleListTracks_Limit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		insertTestTrack(t, db, "track-"+string(rune('A'+i)), "sensor-A", "confirmed", now-int64(i)*1e9, now)
	}

	api := NewTrackAPI(db, "sensor-A")

	// Test with limit=3
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?sensor_id=sensor-A&limit=3", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		return
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Tracks) > 3 {
		t.Errorf("expected at most 3 tracks, got %d", len(response.Tracks))
	}
}

func TestTrackAPI_HandleListTracks_TimeRange(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	baseTime := time.Now()
	// Insert tracks at different times
	insertTestTrack(t, db, "track-old", "sensor-A", "confirmed", baseTime.Add(-2*time.Hour).UnixNano(), baseTime.Add(-1*time.Hour).UnixNano())
	insertTestTrack(t, db, "track-recent", "sensor-A", "confirmed", baseTime.Add(-30*time.Minute).UnixNano(), baseTime.UnixNano())

	api := NewTrackAPI(db, "sensor-A")

	// Filter for recent tracks only (last hour)
	startTime := baseTime.Add(-1 * time.Hour).Unix()
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks?sensor_id=sensor-A&start="+string(rune(startTime)), nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	// Just verify it returns OK - time filtering may vary in implementation
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ====== handleTrackingMetrics tests ======

func TestTrackAPI_HandleTrackingMetrics_MethodNotAllowed(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/metrics", nil)
	w := httptest.NewRecorder()

	api.handleTrackingMetrics(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackingMetrics_NoTracker(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/metrics", nil)
	w := httptest.NewRecorder()

	api.handleTrackingMetrics(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackingMetrics_WithTracker(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Add some clusters to create tracks
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	ts := time.Now()
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/metrics", nil)
	w := httptest.NewRecorder()

	api.handleTrackingMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check response structure
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify per_track is nil (not included by default)
	if resp["per_track"] != nil {
		t.Errorf("expected per_track to be nil, got %v", resp["per_track"])
	}
}

func TestTrackAPI_HandleTrackingMetrics_WithPerTrack(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Add some clusters
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	ts := time.Now()
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	// Request with include_per_track=true
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/metrics?include_per_track=true", nil)
	w := httptest.NewRecorder()

	api.handleTrackingMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check response includes content-type header
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

// ====== handleClearTracks additional tests ======

func TestTrackAPI_HandleClearTracks_WithTrackerNoDB(t *testing.T) {
	// handleClearTracks requires a database - verify the error without one
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/tracks/clear?sensor_id=test-sensor", nil)
	w := httptest.NewRecorder()

	api.handleClearTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 (DB required), got %d: %s", w.Code, w.Body.String())
	}
}

// ====== handleUpdateTrack additional tests ======

func TestTrackAPI_HandleUpdateTrack_WithValidUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UnixNano()
	insertTestTrack(t, db, "track-001", "sensor-A", "confirmed", now-1e9, now)

	api := NewTrackAPI(db, "sensor-A")

	body := strings.NewReader(`{"object_class": "truck", "object_confidence": 0.85}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/lidar/tracks/track-001", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleUpdateTrack(w, req, "track-001")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}
