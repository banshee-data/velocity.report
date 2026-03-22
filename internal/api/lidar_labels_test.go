package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func setupLabelTestDB(t *testing.T) *dbpkg.DB {
	t.Helper()

	db, cleanup := dbpkg.NewTestDB(t)
	t.Cleanup(cleanup)

	_, err := db.Exec(`
		INSERT INTO lidar_run_records (
			run_id, created_at, source_type, source_path, sensor_id, params_json, status
		) VALUES
			('run-001', 1000000000, 'pcap', '/tmp/test.pcap', 'test-sensor', '{}', 'completed');
		INSERT INTO lidar_replay_cases (
			replay_case_id, sensor_id, pcap_file, reference_run_id, created_at_ns
		) VALUES ('case-001', 'test-sensor', '/tmp/test.pcap', 'run-001', 1000000000);
		INSERT INTO lidar_run_tracks (
			run_id, track_id, sensor_id, track_state, start_unix_nanos
		) VALUES
			('run-001', 'track-001', 'test-sensor', 'confirmed', 1000000000),
			('run-001', 'track-002', 'test-sensor', 'confirmed', 2000000000);
	`)
	if err != nil {
		t.Fatalf("failed to insert seed data: %v", err)
	}

	return db
}

func insertReplayAnnotation(t *testing.T, db *dbpkg.DB, labelID, replayCaseID, runID, trackID, classLabel string, startNs int64) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO lidar_replay_annotations (
			annotation_id, replay_case_id, run_id, track_id, class_label,
			start_timestamp_ns, created_at_ns
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, labelID, replayCaseID, runID, trackID, classLabel, startNs, startNs)
	if err != nil {
		t.Fatalf("failed to insert test label: %v", err)
	}
}

func TestLidarLabelAPI_CreateLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	replayCaseID := "case-001"
	runID := "run-001"
	label := LidarLabel{
		ReplayCaseID:     &replayCaseID,
		RunID:            &runID,
		TrackID:          "track-001",
		ClassLabel:       "car",
		StartTimestampNs: 1500000000,
	}

	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var created LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.LabelID == "" {
		t.Error("expected label_id to be generated")
	}
	if created.ReplayCaseID == nil || *created.ReplayCaseID != replayCaseID {
		t.Fatalf("expected replay_case_id %q, got %v", replayCaseID, created.ReplayCaseID)
	}
	if created.RunID == nil || *created.RunID != runID {
		t.Fatalf("expected run_id %q, got %v", runID, created.RunID)
	}
	if created.TrackID != "track-001" {
		t.Fatalf("expected track_id track-001, got %q", created.TrackID)
	}
}

func TestLidarLabelAPI_CreateLabel_RequiresReplayCase(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	label := LidarLabel{
		ClassLabel:       "car",
		StartTimestampNs: 1500000000,
	}

	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_CreateLabel_RequiresRunTrackPair(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	replayCaseID := "case-001"
	label := LidarLabel{
		ReplayCaseID:     &replayCaseID,
		TrackID:          "track-001",
		ClassLabel:       "car",
		StartTimestampNs: 1500000000,
	}

	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_ListLabels(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)
	insertReplayAnnotation(t, db, "label-002", "case-001", "run-001", "track-002", "bus", 2000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		Labels []LidarLabel `json:"labels"`
		Count  int          `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 2 {
		t.Fatalf("expected count 2, got %d", response.Count)
	}
}

func TestLidarLabelAPI_ListLabels_RejectsSessionID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?session_id=session-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_GetLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/label-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var label LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&label); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if label.LabelID != "label-001" {
		t.Fatalf("expected label-001, got %q", label.LabelID)
	}
}

func TestLidarLabelAPI_UpdateLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	endTs := int64(2000000000)
	confidence := float32(0.95)
	notes := "Updated notes"
	update := LidarLabel{
		ClassLabel:     "bus",
		EndTimestampNs: &endTs,
		Confidence:     &confidence,
		Notes:          &notes,
	}

	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var updated LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if updated.ClassLabel != "bus" {
		t.Fatalf("expected class_label bus, got %q", updated.ClassLabel)
	}
	if updated.UpdatedAtNs == nil {
		t.Fatal("expected updated_at_ns to be set")
	}
}

func TestLidarLabelAPI_DeleteLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/labels/label-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM lidar_replay_annotations WHERE annotation_id = ?", "label-001").Scan(&count); err != nil {
		t.Fatalf("failed to query database: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected label to be deleted, found %d rows", count)
	}
}

func TestLidarLabelAPI_Export(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)
	insertReplayAnnotation(t, db, "label-002", "case-001", "run-001", "track-002", "bus", 2000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/export", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var labels []LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&labels); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if rec.Header().Get("Content-Disposition") != "attachment; filename=lidar_track_annotations_export.json" {
		t.Fatalf("unexpected Content-Disposition: %s", rec.Header().Get("Content-Disposition"))
	}
}

func TestLidarLabelAPI_Export_FiltersByReplayCase(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)
	if _, err := db.Exec(`
		INSERT INTO lidar_run_records (
			run_id, created_at, source_type, source_path, sensor_id, params_json, status
		) VALUES ('run-002', 1000000001, 'pcap', '/tmp/test-2.pcap', 'test-sensor', '{}', 'completed');
		INSERT INTO lidar_replay_cases (
			replay_case_id, sensor_id, pcap_file, reference_run_id, created_at_ns
		) VALUES ('case-002', 'test-sensor', '/tmp/test-2.pcap', 'run-002', 1000000001);
		INSERT INTO lidar_run_tracks (
			run_id, track_id, sensor_id, track_state, start_unix_nanos
		) VALUES ('run-002', 'track-101', 'test-sensor', 'confirmed', 1000000001);
		INSERT INTO lidar_replay_annotations (
			annotation_id, replay_case_id, run_id, track_id, class_label, start_timestamp_ns, created_at_ns
		) VALUES ('label-002', 'case-002', 'run-002', 'track-101', 'bus', 2000000000, 2000000000);
	`); err != nil {
		t.Fatalf("failed to insert second replay case labels: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/export?replay_case_id=case-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var labels []LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&labels); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if labels[0].ReplayCaseID == nil || *labels[0].ReplayCaseID != "case-001" {
		t.Fatalf("expected replay_case_id case-001, got %v", labels[0].ReplayCaseID)
	}
}

func TestLidarLabelAPI_FilterByTrackIDRequiresRunID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)
	insertReplayAnnotation(t, db, "label-002", "case-001", "run-001", "track-002", "bus", 2000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?track_id=track-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_FilterByTrackID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)
	insertReplayAnnotation(t, db, "label-002", "case-001", "run-001", "track-002", "bus", 2000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?run_id=run-001&track_id=track-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		Labels []LidarLabel `json:"labels"`
		Count  int          `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 1 {
		t.Fatalf("expected count 1, got %d", response.Count)
	}
	if response.Labels[0].TrackID != "track-001" {
		t.Fatalf("expected track-001, got %q", response.Labels[0].TrackID)
	}
}

func TestLidarLabelAPI_ListLabels_WithClassFilter(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)
	insertReplayAnnotation(t, db, "label-002", "case-001", "run-001", "track-002", "bus", 2000000000)
	insertReplayAnnotation(t, db, "label-003", "case-001", "run-001", "track-001", "car", 3000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?class_label=car", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	var response struct {
		Labels []LidarLabel `json:"labels"`
		Count  int          `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 2 {
		t.Fatalf("expected count 2, got %d", response.Count)
	}
}

func TestLidarLabelAPI_ListLabels_WithTimeFilters(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO lidar_replay_annotations (
			annotation_id, replay_case_id, run_id, track_id, class_label,
			start_timestamp_ns, end_timestamp_ns, created_at_ns
		) VALUES
			('label-001', 'case-001', 'run-001', 'track-001', 'car', 1000000000, 1500000000, 1000000000),
			('label-002', 'case-001', 'run-001', 'track-002', 'bus', 2000000000, 2500000000, 2000000000),
			('label-003', 'case-001', 'run-001', 'track-001', 'car', 3000000000, 3500000000, 3000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test labels: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?start_ns=1500000000&end_ns=3000000000", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	var response struct {
		Labels []LidarLabel `json:"labels"`
		Count  int          `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Count != 1 {
		t.Fatalf("expected count 1, got %d", response.Count)
	}
}

func TestLidarLabelAPI_ListLabels_RejectsInvalidTimeFilters(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?start_ns=not-a-number", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_GetLabel_PreservesLegacyTrackID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO lidar_replay_annotations (
			annotation_id, replay_case_id, run_id, track_id, legacy_track_id, class_label,
			start_timestamp_ns, created_at_ns
		) VALUES
			('label-legacy', NULL, NULL, NULL, 'track-legacy', 'car', 1000000000, 1000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert legacy label: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/label-legacy", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var label LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&label); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if label.TrackID != "" {
		t.Fatalf("expected empty durable track_id for legacy row, got %q", label.TrackID)
	}
	if label.LegacyTrackID == nil || *label.LegacyTrackID != "track-legacy" {
		t.Fatalf("expected legacy_track_id track-legacy, got %v", label.LegacyTrackID)
	}
}

func TestLidarLabelAPI_GetLabel_NotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/missing", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_UpdateLabel_NotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	body, _ := json.Marshal(LidarLabel{ClassLabel: "bus"})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/missing", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_DeleteLabel_NotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/labels/missing", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_InvalidJSON(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader([]byte("invalid json{")))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestLidarLabelAPI_MethodNotAllowed(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/lidar/labels", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected status %d for %s, got %d", http.StatusMethodNotAllowed, method, rec.Code)
		}
	}
}

func TestLidarLabelAPI_WriteJSONError(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	rec := httptest.NewRecorder()

	api.writeJSONError(rec, http.StatusBadRequest, "invalid input")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var errResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if errResp["error"] != "invalid input" {
		t.Fatalf("expected invalid input, got %v", errResp["error"])
	}
}

func TestValidateQualityLabel(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"good", true},
		{"noisy", true},
		{"jitter_velocity", true},
		{"jitter_heading", true},
		{"merge", true},
		{"split", true},
		{"truncated", true},
		{"disconnected", true},
		{"good,noisy", true},
		{"jitter_velocity,jitter_heading", true},
		{"good, noisy", true},
		{"", false},
		{"invalid", false},
		{"good,invalid", false},
		{"good,", false},
		{",good", false},
	}

	for _, tc := range tests {
		if got := ValidateQualityLabel(tc.input); got != tc.want {
			t.Errorf("ValidateQualityLabel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
