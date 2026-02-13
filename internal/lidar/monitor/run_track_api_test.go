package monitor

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// setupTestRun creates a test run with tracks using the existing setupTestDB
func setupTestRun(t *testing.T, store *lidar.AnalysisRunStore, runID string) {
	t.Helper()

	// Create a test run
	run := &lidar.AnalysisRun{
		RunID:           runID,
		SensorID:        "test-sensor",
		SourceType:      "pcap",
		SourcePath:      "/test/data.pcap",
		ParamsJSON:      []byte(`{"version":"1.0"}`),
		DurationSecs:    10.0,
		TotalFrames:     100,
		TotalClusters:   50,
		TotalTracks:     5,
		ConfirmedTracks: 3,
		Status:          "completed",
	}

	if err := store.InsertRun(run); err != nil {
		t.Fatalf("failed to insert test run: %v", err)
	}

	// Insert some test tracks
	tracks := []*lidar.RunTrack{
		{
			RunID:            runID,
			TrackID:          "track-001",
			SensorID:         "test-sensor",
			TrackState:       "confirmed",
			StartUnixNanos:   1000000000,
			EndUnixNanos:     2000000000,
			ObservationCount: 10,
			AvgSpeedMps:      5.5,
			PeakSpeedMps:     8.0,
		},
		{
			RunID:            runID,
			TrackID:          "track-002",
			SensorID:         "test-sensor",
			TrackState:       "confirmed",
			StartUnixNanos:   1500000000,
			EndUnixNanos:     2500000000,
			ObservationCount: 15,
			AvgSpeedMps:      6.2,
			PeakSpeedMps:     9.5,
		},
		{
			RunID:            runID,
			TrackID:          "track-003",
			SensorID:         "test-sensor",
			TrackState:       "tentative",
			StartUnixNanos:   2000000000,
			EndUnixNanos:     2800000000,
			ObservationCount: 5,
			AvgSpeedMps:      3.1,
			PeakSpeedMps:     4.5,
		},
	}

	for _, track := range tracks {
		if err := store.InsertRunTrack(track); err != nil {
			t.Fatalf("failed to insert test track %s: %v", track.TrackID, err)
		}
	}
}

// TestUpdateTrackLabelValid tests updating a track label with valid labels
func TestUpdateTrackLabelValid(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-001"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	// Test valid label update
	reqBody := map[string]interface{}{
		"user_label":       "car",
		"quality_label":    "good",
		"label_confidence": 0.95,
		"labeler_id":       "test-user",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/test-run-001/tracks/track-001/label", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	ws.handleUpdateTrackLabel(w, req, runID, "track-001")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify the label was stored
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result["status"])
	}
	if result["user_label"] != "car" {
		t.Errorf("expected user_label car, got %v", result["user_label"])
	}

	// Verify in database
	tracks, err := store.GetRunTracks(runID)
	if err != nil {
		t.Fatalf("failed to get tracks: %v", err)
	}

	found := false
	for _, track := range tracks {
		if track.TrackID == "track-001" {
			found = true
			if track.UserLabel != "car" {
				t.Errorf("expected user_label car, got %s", track.UserLabel)
			}
			if track.QualityLabel != "good" {
				t.Errorf("expected quality_label good, got %s", track.QualityLabel)
			}
			if track.LabelerID != "test-user" {
				t.Errorf("expected labeler_id test-user, got %s", track.LabelerID)
			}
			break
		}
	}

	if !found {
		t.Error("track-001 not found in database")
	}
}

// TestUpdateTrackLabelInvalid tests that invalid labels are rejected
func TestUpdateTrackLabelInvalid(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-002"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	// Test invalid user_label
	reqBody := map[string]interface{}{
		"user_label":       "invalid_label",
		"quality_label":    "good",
		"label_confidence": 0.95,
		"labeler_id":       "test-user",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/test-run-002/tracks/track-001/label", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	ws.handleUpdateTrackLabel(w, req, runID, "track-001")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid user_label, got %d", resp.StatusCode)
	}

	// Test invalid quality_label
	reqBody = map[string]interface{}{
		"user_label":       "car",
		"quality_label":    "invalid_quality",
		"label_confidence": 0.95,
		"labeler_id":       "test-user",
	}
	bodyBytes, _ = json.Marshal(reqBody)

	req = httptest.NewRequest(http.MethodPut, "/api/lidar/runs/test-run-002/tracks/track-001/label", bytes.NewReader(bodyBytes))
	w = httptest.NewRecorder()

	ws.handleUpdateTrackLabel(w, req, runID, "track-001")

	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid quality_label, got %d", resp.StatusCode)
	}
}

// TestUpdateTrackLabelClear tests clearing labels with empty strings
func TestUpdateTrackLabelClear(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-003"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	// First, set a label
	if err := store.UpdateTrackLabel(runID, "track-001", "car", "good", 0.95, "test-user", "human_manual"); err != nil {
		t.Fatalf("failed to set initial label: %v", err)
	}

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	// Clear labels with empty strings
	reqBody := map[string]interface{}{
		"user_label":       "",
		"quality_label":    "",
		"label_confidence": 0.0,
		"labeler_id":       "test-user",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/test-run-003/tracks/track-001/label", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	ws.handleUpdateTrackLabel(w, req, runID, "track-001")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify labels are cleared
	tracks, err := store.GetRunTracks(runID)
	if err != nil {
		t.Fatalf("failed to get tracks: %v", err)
	}

	for _, track := range tracks {
		if track.TrackID == "track-001" {
			if track.UserLabel != "" {
				t.Errorf("expected empty user_label, got %s", track.UserLabel)
			}
			if track.QualityLabel != "" {
				t.Errorf("expected empty quality_label, got %s", track.QualityLabel)
			}
			break
		}
	}
}

// TestListRuns tests listing analysis runs
func TestListRuns(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	store := lidar.NewAnalysisRunStore(sqlDB)

	// Create multiple test runs
	runs := []string{"run-001", "run-002", "run-003"}
	for _, runID := range runs {
		setupTestRun(t, store, runID)
	}

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs?limit=10", nil)
	w := httptest.NewRecorder()

	ws.handleListRuns(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	count, ok := result["count"].(float64)
	if !ok {
		t.Fatalf("expected count to be a number, got %T", result["count"])
	}

	if int(count) != 3 {
		t.Errorf("expected 3 runs, got %d", int(count))
	}
}

// TestLabellingProgress tests the labelling progress endpoint
func TestLabellingProgress(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-004"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	// Label one track
	if err := store.UpdateTrackLabel(runID, "track-001", "car", "good", 0.95, "test-user", "human_manual"); err != nil {
		t.Fatalf("failed to label track: %v", err)
	}

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/test-run-004/labelling-progress", nil)
	w := httptest.NewRecorder()

	ws.handleLabellingProgress(w, req, runID)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	total, ok := result["total"].(float64)
	if !ok || int(total) != 3 {
		t.Errorf("expected total 3, got %v", result["total"])
	}

	labelled, ok := result["labelled"].(float64)
	if !ok || int(labelled) != 1 {
		t.Errorf("expected labelled 1, got %v", result["labelled"])
	}

	progressPct, ok := result["progress_pct"].(float64)
	if !ok {
		t.Fatalf("expected progress_pct to be a number, got %T", result["progress_pct"])
	}

	expectedPct := 100.0 / 3.0 // 1 out of 3 tracks labelled
	if progressPct < expectedPct-0.1 || progressPct > expectedPct+0.1 {
		t.Errorf("expected progress_pct ~%.2f%%, got %.2f%%", expectedPct, progressPct)
	}

	byClass, ok := result["by_class"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected by_class to be a map, got %T", result["by_class"])
	}

	goodVehicleCount, ok := byClass["car"].(float64)
	if !ok || int(goodVehicleCount) != 1 {
		t.Errorf("expected car count 1, got %v", byClass["car"])
	}
}

// TestGetRun tests retrieving a specific run
func TestGetRun(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-005"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/test-run-005", nil)
	w := httptest.NewRecorder()

	ws.handleGetRun(w, req, runID)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var run lidar.AnalysisRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if run.RunID != runID {
		t.Errorf("expected run_id %s, got %s", runID, run.RunID)
	}
	if run.SensorID != "test-sensor" {
		t.Errorf("expected sensor_id test-sensor, got %s", run.SensorID)
	}
	if run.Status != "completed" {
		t.Errorf("expected status completed, got %s", run.Status)
	}
}

// TestGetRunNotFound tests retrieving a non-existent run
func TestGetRunNotFound(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/nonexistent", nil)
	w := httptest.NewRecorder()

	ws.handleGetRun(w, req, "nonexistent")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status 404, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestReprocessRun tests the reprocess endpoint (should return 501)
func TestReprocessRun(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-006"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/test-run-006/reprocess", nil)
	w := httptest.NewRecorder()

	ws.handleReprocessRun(w, req, runID)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["error"] != "not_implemented" {
		t.Errorf("expected error not_implemented, got %v", result["error"])
	}
}

// TestUpdateTrackFlags tests updating track quality flags
func TestUpdateTrackFlags(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "test-run-007"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	testDB := &db.DB{DB: sqlDB}
	ws := &WebServer{db: testDB}

	// Test split flag
	reqBody := map[string]interface{}{
		"linked_track_ids": []string{"track-002", "track-003"},
		"user_label":       "split",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/test-run-007/tracks/track-001/flags", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	ws.handleUpdateTrackFlags(w, req, runID, "track-001")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify in database
	tracks, err := store.GetRunTracks(runID)
	if err != nil {
		t.Fatalf("failed to get tracks: %v", err)
	}

	for _, track := range tracks {
		if track.TrackID == "track-001" {
			if !track.IsSplitCandidate {
				t.Error("expected is_split_candidate to be true")
			}
			if track.IsMergeCandidate {
				t.Error("expected is_merge_candidate to be false")
			}
			if len(track.LinkedTrackIDs) != 2 {
				t.Errorf("expected 2 linked tracks, got %d", len(track.LinkedTrackIDs))
			}
			break
		}
	}
}

// TestParseRunPath tests the path parsing utility
func TestParseRunPath(t *testing.T) {
	tests := []struct {
		path    string
		wantRun string
		wantSub string
	}{
		{"/api/lidar/runs/", "", ""},
		{"/api/lidar/runs/run-001", "run-001", ""},
		{"/api/lidar/runs/run-001/tracks", "run-001", "tracks"},
		{"/api/lidar/runs/run-001/tracks/track-001/label", "run-001", "tracks/track-001/label"},
		{"/api/lidar/runs/run-001/labelling-progress", "run-001", "labelling-progress"},
		{"/api/lidar/runs/run-001/reprocess", "run-001", "reprocess"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotRun, gotSub := parseRunPath(tt.path)
			if gotRun != tt.wantRun {
				t.Errorf("parseRunPath(%q) run = %q, want %q", tt.path, gotRun, tt.wantRun)
			}
			if gotSub != tt.wantSub {
				t.Errorf("parseRunPath(%q) sub = %q, want %q", tt.path, gotSub, tt.wantSub)
			}
		})
	}
}

// TestParseTrackPath tests the track path parsing utility
func TestParseTrackPath(t *testing.T) {
	tests := []struct {
		path       string
		wantTrack  string
		wantAction string
	}{
		{"track-001/label", "track-001", "label"},
		{"track-001/flags", "track-001", "flags"},
		{"track-001", "track-001", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotTrack, gotAction := parseTrackPath(tt.path)
			if gotTrack != tt.wantTrack {
				t.Errorf("parseTrackPath(%q) track = %q, want %q", tt.path, gotTrack, tt.wantTrack)
			}
			if gotAction != tt.wantAction {
				t.Errorf("parseTrackPath(%q) action = %q, want %q", tt.path, gotAction, tt.wantAction)
			}
		})
	}
}

func TestWebServer_HandleDeleteRunTrack_Success(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}

	ws := &WebServer{db: testDB}

	// Create a test run and track
	store := lidar.NewAnalysisRunStore(sqlDB)
	run := &lidar.AnalysisRun{
		RunID:      "run-001",
		SensorID:   "test-sensor",
		SourceType: "pcap",
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("failed to insert run: %v", err)
	}

	track := &lidar.RunTrack{
		RunID:            "run-001",
		TrackID:          "track-001",
		SensorID:         "test-sensor",
		TrackState:       "confirmed",
		StartUnixNanos:   1000000000,
		EndUnixNanos:     2000000000,
		ObservationCount: 10,
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("failed to insert track: %v", err)
	}

	// DELETE request
	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-001/tracks/track-001", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRunTrack(w, req, "run-001", "track-001")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify track is deleted
	tracks, err := store.GetRunTracks("run-001")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks after delete, got %d", len(tracks))
	}
}

func TestWebServer_HandleDeleteRunTrack_NotFound(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}

	ws := &WebServer{db: testDB}

	// DELETE non-existent track
	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-001/tracks/track-999", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRunTrack(w, req, "run-001", "track-999")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestWebServer_HandleDeleteRunTrack_MethodNotAllowed(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}

	ws := &WebServer{db: testDB}

	// GET request (not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-001/tracks/track-001", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRunTrack(w, req, "run-001", "track-001")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestWebServer_HandleDeleteRun_Success(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}

	ws := &WebServer{db: testDB}

	// Create a test run
	store := lidar.NewAnalysisRunStore(sqlDB)
	run := &lidar.AnalysisRun{
		RunID:      "run-001",
		SensorID:   "test-sensor",
		SourceType: "pcap",
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("failed to insert run: %v", err)
	}

	// DELETE request
	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-001", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRun(w, req, "run-001")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify run is deleted
	_, err := store.GetRun("run-001")
	if err == nil {
		t.Error("expected error getting deleted run, got nil")
	}
}

func TestWebServer_HandleDeleteRun_NotFound(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}

	ws := &WebServer{db: testDB}

	// DELETE non-existent run
	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-999", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRun(w, req, "run-999")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestWebServer_HandleDeleteRun_MethodNotAllowed(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	testDB := &db.DB{DB: sqlDB}

	ws := &WebServer{db: testDB}

	// GET request (not allowed)
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-001", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRun(w, req, "run-001")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestWebServer_HandleDeleteRun_NoDB(t *testing.T) {
	ws := &WebServer{db: nil}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-001", nil)
	w := httptest.NewRecorder()

	ws.handleDeleteRun(w, req, "run-001")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestRunTrackAPI_DispatchErrors(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	ws := &WebServer{db: &db.DB{DB: sqlDB}}

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "missing track id",
			method:     http.MethodPut,
			path:       "/api/lidar/runs/run-1/tracks//label",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown track action",
			method:     http.MethodPut,
			path:       "/api/lidar/runs/run-1/tracks/track-1/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown run subpath",
			method:     http.MethodGet,
			path:       "/api/lidar/runs/run-1/does-not-exist",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing region id",
			method:     http.MethodDelete,
			path:       "/api/lidar/runs/run-1/missed-regions/",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			ws.handleRunTrackAPI(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected %d got %d body=%s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestRunTrackAPI_MissedRegionsLifecycle(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "run-missed-001"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	ws := &WebServer{db: &db.DB{DB: sqlDB}}

	// Create region.
	createBody := map[string]interface{}{
		"center_x":      12.3,
		"center_y":      -4.5,
		"time_start_ns": 1000,
		"time_end_ns":   2000,
		"notes":         "test region",
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/missed-regions", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d got %d body=%s", http.StatusCreated, w.Code, w.Body.String())
	}

	var created lidar.MissedRegion
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created region: %v", err)
	}
	if created.RegionID == "" {
		t.Fatal("expected created region id")
	}

	// List should include the created region.
	req = httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/missed-regions", nil)
	w = httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var listResp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Count != 1 {
		t.Fatalf("expected list count 1, got %d", listResp.Count)
	}

	// Delete should succeed.
	req = httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/"+runID+"/missed-regions/"+created.RegionID, nil)
	w = httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	// Deleting again should return 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/"+runID+"/missed-regions/"+created.RegionID, nil)
	w = httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestRunTrackAPI_EvaluateRun(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	runID := "eval-run"
	store := lidar.NewAnalysisRunStore(sqlDB)
	setupTestRun(t, store, runID)

	ws := &WebServer{db: &db.DB{DB: sqlDB}}

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/evaluate", nil)
		w := httptest.NewRecorder()
		ws.handleRunTrackAPI(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/evaluate", bytes.NewReader([]byte("{")))
		w := httptest.NewRecorder()
		ws.handleRunTrackAPI(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("auto detect no scene reference", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/evaluate", bytes.NewReader([]byte(`{}`)))
		w := httptest.NewRecorder()
		ws.handleRunTrackAPI(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("explicit reference success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/evaluate", bytes.NewReader([]byte(`{"reference_run_id":"reference-run"}`)))
		w := httptest.NewRecorder()
		ws.handleRunTrackAPI(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["reference_run_id"] != "reference-run" {
			t.Fatalf("unexpected reference_run_id: %v", resp["reference_run_id"])
		}
		if resp["candidate_run_id"] != runID {
			t.Fatalf("unexpected candidate_run_id: %v", resp["candidate_run_id"])
		}
		if _, ok := resp["score"]; !ok {
			t.Fatalf("expected score in response")
		}
	})
}
