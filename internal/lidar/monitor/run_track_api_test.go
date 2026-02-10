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
		"user_label":       "good_vehicle",
		"quality_label":    "perfect",
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
	if result["user_label"] != "good_vehicle" {
		t.Errorf("expected user_label good_vehicle, got %v", result["user_label"])
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
			if track.UserLabel != "good_vehicle" {
				t.Errorf("expected user_label good_vehicle, got %s", track.UserLabel)
			}
			if track.QualityLabel != "perfect" {
				t.Errorf("expected quality_label perfect, got %s", track.QualityLabel)
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
		"quality_label":    "perfect",
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
		"user_label":       "good_vehicle",
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
	if err := store.UpdateTrackLabel(runID, "track-001", "good_vehicle", "perfect", 0.95, "test-user"); err != nil {
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
	if err := store.UpdateTrackLabel(runID, "track-001", "good_vehicle", "perfect", 0.95, "test-user"); err != nil {
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

	goodVehicleCount, ok := byClass["good_vehicle"].(float64)
	if !ok || int(goodVehicleCount) != 1 {
		t.Errorf("expected good_vehicle count 1, got %v", byClass["good_vehicle"])
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
