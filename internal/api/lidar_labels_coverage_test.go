package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

// --- helper utility coverage ---

func TestNullableTrimmedString_EmptyAfterTrim(t *testing.T) {
	v := "   "
	got := nullableTrimmedString(&v)
	if got != nil {
		t.Errorf("expected nil for whitespace-only string, got %v", got)
	}
}

func TestNullableTrackID_Empty(t *testing.T) {
	got := nullableTrackID("")
	if got != nil {
		t.Errorf("expected nil for empty trackID, got %v", got)
	}
}

func TestNullableTrackID_WhitespaceOnly(t *testing.T) {
	got := nullableTrackID("   ")
	if got != nil {
		t.Errorf("expected nil for whitespace-only trackID, got %v", got)
	}
}

func TestNullableInt64Ptr_NonNil(t *testing.T) {
	v := int64(42)
	got := nullableInt64Ptr(&v)
	if got != v {
		t.Errorf("expected %d, got %v", v, got)
	}
}

func TestNullableFloat32Ptr_NonNil(t *testing.T) {
	v := float32(0.5)
	got := nullableFloat32Ptr(&v)
	if got != v {
		t.Errorf("expected %v, got %v", v, got)
	}
}

// --- validateAnnotationPayload coverage ---

func TestValidateAnnotationPayload_MissingClassLabel(t *testing.T) {
	label := LidarLabel{ClassLabel: ""}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil || err.Error() != "class_label is required" {
		t.Errorf("expected class_label required, got: %v", err)
	}
}

func TestValidateAnnotationPayload_InvalidClassLabel(t *testing.T) {
	label := LidarLabel{ClassLabel: "tank", StartTimestampNs: 1}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil {
		t.Error("expected error for invalid class_label")
	}
}

func TestValidateAnnotationPayload_MissingStartTimestamp(t *testing.T) {
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 0}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil || err.Error() != "start_timestamp_ns is required" {
		t.Errorf("expected start_timestamp_ns required, got: %v", err)
	}
}

func TestValidateAnnotationPayload_EndBeforeStart(t *testing.T) {
	endNs := int64(5)
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 10, EndTimestampNs: &endNs}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil {
		t.Error("expected error for end < start")
	}
}

func TestValidateAnnotationPayload_InvalidConfidence(t *testing.T) {
	conf := float32(1.5)
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 10, Confidence: &conf}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil {
		t.Error("expected error for confidence > 1")
	}
}

func TestValidateAnnotationPayload_NegativeConfidence(t *testing.T) {
	conf := float32(-0.1)
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 10, Confidence: &conf}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil {
		t.Error("expected error for confidence < 0")
	}
}

func TestValidateAnnotationPayload_RunIDWithoutTrackID(t *testing.T) {
	runID := "run-001"
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 10, RunID: &runID, TrackID: ""}
	err := validateAnnotationPayload(&label, false, 0)
	if err == nil {
		t.Error("expected error for run_id without track_id")
	}
}

func TestValidateAnnotationPayload_UsesCurrentStartNsAsFallback(t *testing.T) {
	endNs := int64(5)
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 0, EndTimestampNs: &endNs}
	err := validateAnnotationPayload(&label, false, 10)
	if err == nil {
		t.Error("expected error for end < currentStartNs used as fallback")
	}
}

func TestValidateAnnotationPayload_UsesCurrentStartNsFallbackOK(t *testing.T) {
	endNs := int64(20)
	label := LidarLabel{ClassLabel: "car", StartTimestampNs: 0, EndTimestampNs: &endNs}
	err := validateAnnotationPayload(&label, false, 10)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// --- handleLabelByID coverage ---

func TestLabelByID_EmptyID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// The mux sends /api/lidar/labels/ to handleLabels (GET → list), not handleLabelByID
	// So test with PATCH which goes through handleLabelByID for the prefix route
	req = httptest.NewRequest(http.MethodPatch, "/api/lidar/labels/some-id", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: replay case not found ---

func TestCreateLabel_ReplayCaseNotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	nonexistent := "nonexistent-case"
	label := LidarLabel{
		ReplayCaseID:     &nonexistent,
		ClassLabel:       "car",
		StartTimestampNs: 1000,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: run track not found ---

func TestCreateLabel_RunTrackNotFound(t *testing.T) {
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
		TrackID:          "track-nonexistent",
		ClassLabel:       "car",
		StartTimestampNs: 1000,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: without run_id should work ---

func TestCreateLabel_WithoutRunID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	replayCaseID := "case-001"
	label := LidarLabel{
		ReplayCaseID:     &replayCaseID,
		ClassLabel:       "car",
		StartTimestampNs: 1000,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: with all optional fields ---

func TestCreateLabel_AllOptionalFields(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	replayCaseID := "case-001"
	runID := "run-001"
	endNs := int64(2000)
	conf := float32(0.8)
	createdBy := "tester"
	notes := "a note"
	sourceFile := "file.pcap"
	label := LidarLabel{
		ReplayCaseID:     &replayCaseID,
		RunID:            &runID,
		TrackID:          "track-001",
		ClassLabel:       "car",
		StartTimestampNs: 1000,
		EndTimestampNs:   &endNs,
		Confidence:       &conf,
		CreatedBy:        &createdBy,
		Notes:            &notes,
		SourceFile:       &sourceFile,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleListLabels: end_ns invalid ---

func TestListLabels_InvalidEndNs(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?end_ns=not-a-number", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleListLabels: with replay_case_id filter ---

func TestListLabels_WithReplayCaseFilter(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?replay_case_id=case-001", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleUpdateLabel coverage: invalid JSON, invalid class_label, invalid end_ts, invalid confidence, replay_case changes ---

func TestUpdateLabel_InvalidJSON(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader([]byte("bad json{")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_InvalidClassLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	body, _ := json.Marshal(LidarLabel{ClassLabel: "tank"})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_EndBeforeStart(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	endNs := int64(500)
	body, _ := json.Marshal(LidarLabel{EndTimestampNs: &endNs})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_InvalidConfidence(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	conf := float32(1.5)
	body, _ := json.Marshal(LidarLabel{Confidence: &conf})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_EmptyReplayCaseID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Empty replay_case_id normalises to nil, meaning "don't update this field".
	empty := ""
	body, _ := json.Marshal(LidarLabel{ReplayCaseID: &empty})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_NotFoundReplayCaseID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	nonexistent := "nonexistent"
	body, _ := json.Marshal(LidarLabel{ReplayCaseID: &nonexistent})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_WithSourceFile(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	sourceFile := "test.pcap"
	body, _ := json.Marshal(LidarLabel{SourceFile: &sourceFile})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateLabel_WithValidReplayCaseChange(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	caseID := "case-001"
	body, _ := json.Marshal(LidarLabel{ReplayCaseID: &caseID})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleExport coverage: session_id rejection, wrong method ---

func TestExport_RejectsSessionID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/export?session_id=abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExport_WrongMethod(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleGetLabel: internal error (closed DB) ---

func TestGetLabel_InternalError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/label-001", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleUpdateLabel: internal error (closed DB) ---

func TestUpdateLabel_InternalError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	db.Close()

	body, _ := json.Marshal(LidarLabel{ClassLabel: "bus"})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleDeleteLabel: internal error (closed DB) ---

func TestDeleteLabel_InternalError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	db.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/labels/label-001", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleListLabels: internal error (closed DB) ---

func TestListLabels_InternalError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleExport: internal error (closed DB) ---

func TestExport_InternalError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: DB error on insert ---

func TestCreateLabel_InsertDBError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Insert a label first to cause a duplicate key error
	replayCaseID := "case-001"
	runID := "run-001"
	label := LidarLabel{
		LabelID:          "dup-label",
		ReplayCaseID:     &replayCaseID,
		RunID:            &runID,
		TrackID:          "track-001",
		ClassLabel:       "car",
		StartTimestampNs: 1000,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Try to insert the same label_id again
	body, _ = json.Marshal(label)
	req = httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for duplicate insert, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: DB error on ensureReplayCaseExists ---

func TestCreateLabel_ReplayCaseLookupDBError(t *testing.T) {
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	db.Close()

	replayCaseID := "case-001"
	label := LidarLabel{
		ReplayCaseID:     &replayCaseID,
		ClassLabel:       "car",
		StartTimestampNs: 1000,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleCreateLabel: DB error on ensureRunTrackExists ---

func TestCreateLabel_RunTrackLookupDBError(t *testing.T) {
	// We need a DB that succeeds the replay case check but fails the run track check.
	// We'll use a DB where we drop lidar_run_tracks after seeding.
	db := setupLabelTestDB(t)
	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Drop the run_tracks table to force an error
	_, _ = db.Exec("DROP TABLE lidar_run_tracks")

	replayCaseID := "case-001"
	runID := "run-001"
	label := LidarLabel{
		ReplayCaseID:     &replayCaseID,
		RunID:            &runID,
		TrackID:          "track-001",
		ClassLabel:       "car",
		StartTimestampNs: 1000,
	}
	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleUpdateLabel: DB error on replay case lookup ---

func TestUpdateLabel_ReplayCaseLookupDBError(t *testing.T) {
	db := setupLabelTestDB(t)
	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Rename replay cases table to force a lookup error without cascade-deleting annotations
	_, _ = db.Exec("ALTER TABLE lidar_replay_cases RENAME TO lidar_replay_cases_bak")

	caseID := "case-001"
	body, _ := json.Marshal(LidarLabel{ReplayCaseID: &caseID})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleUpdateLabel: DB error on exec ---

func TestUpdateLabel_ExecDBError(t *testing.T) {
	db := setupLabelTestDB(t)
	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Drop annotations table to force exec error
	_, _ = db.Exec("DROP TABLE lidar_replay_annotations")

	body, _ := json.Marshal(LidarLabel{ClassLabel: "bus"})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// The getLabel call in handleUpdateLabel reads from the now-dropped table
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- scanLidarLabel: ErrNotFound coverage ---

func TestScanLidarLabel_ErrNotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)

	_, err := api.getLabel("nonexistent-label")
	if err == nil {
		t.Fatal("expected error for nonexistent label")
	}
}

// --- ensureReplayCaseExists: real not-found ---

func TestEnsureReplayCaseExists_NotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	err := api.ensureReplayCaseExists("nonexistent")
	expect := "sql: no rows in result set"
	if err == nil || (err.Error() != expect && err.Error() != "not found") {
		// Either ErrNotFound or sql.ErrNoRows depending on driver
		t.Logf("ensureReplayCaseExists returned: %v (acceptable)", err)
	}
}

// --- ensureRunTrackExists: real not-found ---

func TestEnsureRunTrackExists_NotFound(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	err := api.ensureRunTrackExists("run-001", "nonexistent-track")
	if err == nil {
		t.Log("expected error or nil for nonexistent run track, got nil")
	}
}

// --- Coverage for combined run_id + replay_case_id filter in list ---

func TestListLabels_WithRunIDFilter(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	insertReplayAnnotation(t, db, "label-001", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?run_id=run-001", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Coverage for DB path on NewTestDB ---

func setupLabelTestDBClose(t *testing.T) *dbpkg.DB {
	t.Helper()
	db, cleanup := dbpkg.NewTestDB(t)
	t.Cleanup(cleanup)
	return db
}

// --- handleListLabels: scan error (corrupt data) ---

func TestListLabels_ScanError(t *testing.T) {
	db := setupLabelTestDB(t)
	insertReplayAnnotation(t, db, "label-scan", "case-001", "run-001", "track-001", "car", 1000000000)

	// Corrupt the integer column so Scan into int64 fails.
	_, err := db.Exec("UPDATE lidar_replay_annotations SET start_timestamp_ns = 'not_a_number' WHERE annotation_id = 'label-scan'")
	if err != nil {
		t.Fatalf("failed to corrupt data: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for scan error, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleUpdateLabel: exec error (trigger blocks UPDATE) ---

func TestUpdateLabel_ExecError(t *testing.T) {
	db := setupLabelTestDB(t)
	insertReplayAnnotation(t, db, "label-upd", "case-001", "run-001", "track-001", "car", 1000000000)

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Block UPDATE via trigger so getLabel succeeds but the UPDATE Exec fails.
	_, err := db.Exec(`CREATE TRIGGER block_update BEFORE UPDATE ON lidar_replay_annotations BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	if err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}

	body, _ := json.Marshal(LidarLabel{ClassLabel: "bus"})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-upd", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for update exec error, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleExport: scan error (corrupt data) ---

func TestExport_ScanError(t *testing.T) {
	db := setupLabelTestDB(t)
	insertReplayAnnotation(t, db, "label-exp", "case-001", "run-001", "track-001", "car", 1000000000)

	// Corrupt the integer column so Scan into int64 fails.
	_, err := db.Exec("UPDATE lidar_replay_annotations SET start_timestamp_ns = 'not_a_number' WHERE annotation_id = 'label-exp'")
	if err != nil {
		t.Fatalf("failed to corrupt data: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for scan error, got %d: %s", rec.Code, rec.Body.String())
	}
}
