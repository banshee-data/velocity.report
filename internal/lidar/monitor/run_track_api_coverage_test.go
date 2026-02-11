package monitor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// covSetupWS creates a WebServer wrapping the shared setupTestDB helper.
func covSetupWS(t *testing.T) (*WebServer, func()) {
	t.Helper()
	sqlDB, cleanup := setupTestDB(t)
	testDB := &db.DB{DB: sqlDB}
	return &WebServer{db: testDB}, cleanup
}

// covInsertRun inserts an analysis run and returns its ID.
func covInsertRun(t *testing.T, ws *WebServer, suffix string) string {
	t.Helper()
	store := lidar.NewAnalysisRunStore(ws.db.DB)
	run := &lidar.AnalysisRun{
		RunID:      "cov-run-" + suffix,
		SourceType: "pcap",
		SourcePath: "/test/file.pcap",
		SensorID:   "test-sensor",
		Status:     "completed",
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	return run.RunID
}

// covInsertTrack inserts a run track.
func covInsertTrack(t *testing.T, ws *WebServer, runID, trackID string) {
	t.Helper()
	store := lidar.NewAnalysisRunStore(ws.db.DB)
	track := &lidar.RunTrack{
		RunID:            runID,
		TrackID:          trackID,
		SensorID:         "test-sensor",
		TrackState:       "confirmed",
		ObservationCount: 10,
		AvgSpeedMps:      5.0,
		PeakSpeedMps:     8.0,
		StartUnixNanos:   1000000000,
		EndUnixNanos:     2000000000,
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack: %v", err)
	}
}

// --- handleRunTrackAPI dispatcher coverage ---

func TestCov_HandleRunTrackAPI_EndpointNotFound(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/unknown-endpoint", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleRunTrackAPI_ListRuns(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleRunTrackAPI_GetRun(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "get")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID, nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleRunTrackAPI_GetRun_NotFound(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleRunTrackAPI_DeleteRun(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "del")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/"+runID, nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleRunTrackAPI_DeleteRun_NotFound(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleRunTrackAPI_ListRunTracks(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "tracks")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/tracks", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleRunTrackAPI_ListRunTracks_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "tracks-method")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/tracks", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleRunTrackAPI_Reprocess(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "reproc")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestCov_HandleRunTrackAPI_Reprocess_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleRunTrackAPI_Evaluate_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/evaluate", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_GET(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "mr-get")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/missed-regions", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_POST(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "mr-post")

	body, _ := json.Marshal(map[string]interface{}{
		"center_x":      1.5,
		"center_y":      2.5,
		"time_start_ns": 1000,
		"time_end_ns":   2000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/missed-regions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_POST_MissingCenter(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "mr-nocenter")

	body, _ := json.Marshal(map[string]interface{}{
		"time_start_ns": 1000,
		"time_end_ns":   2000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/missed-regions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_POST_NoTimeStart(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "mr-nostart")

	body, _ := json.Marshal(map[string]interface{}{
		"center_x":    1.0,
		"center_y":    2.0,
		"time_end_ns": 2000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/missed-regions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_POST_NoTimeEnd(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "mr-noend")

	body, _ := json.Marshal(map[string]interface{}{
		"center_x":      1.0,
		"center_y":      2.0,
		"time_start_ns": 1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/missed-regions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/run-1/missed-regions", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegions_InvalidJSON(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "mr-badjson")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/missed-regions", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleRunTrackAPI_DeleteMissedRegion_NotFound(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-1/missed-regions/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleRunTrackAPI_DeleteMissedRegion_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/missed-regions/region-1", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleRunTrackAPI_MissedRegion_EmptyID(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-1/missed-regions/", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleRunTrackAPI_Tracks_EmptyTrackID(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleRunTrackAPI_Tracks_UnknownAction(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/track-1/unknown", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleRunTrackAPI_Tracks_DeleteTrack_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/track-1", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleRunTrackAPI_LabellingProgress(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "progress")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/labelling-progress", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleRunTrackAPI_LabellingProgress_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/run-1/labelling-progress", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleUpdateTrackFlags additional coverage ---

func TestCov_HandleUpdateTrackFlags_InvalidJSON(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/run-1/tracks/track-1/flags", bytes.NewReader([]byte("bad json")))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, "run-1", "track-1")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleUpdateTrackFlags_InvalidUserLabel(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]interface{}{
		"user_label":       "invalid_label",
		"linked_track_ids": []string{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/run-1/tracks/track-1/flags", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, "run-1", "track-1")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleUpdateTrackFlags_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/track-1/flags", nil)
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, "run-1", "track-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleUpdateTrackFlags_Split(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "flags-split")
	covInsertTrack(t, ws, runID, "track-1")

	body, _ := json.Marshal(map[string]interface{}{
		"user_label":       "split",
		"linked_track_ids": []string{"track-2"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/"+runID+"/tracks/track-1/flags", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, runID, "track-1")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCov_HandleUpdateTrackFlags_Merge(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "flags-merge")
	covInsertTrack(t, ws, runID, "track-1")

	body, _ := json.Marshal(map[string]interface{}{
		"user_label":       "merge",
		"linked_track_ids": []string{"track-2"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/"+runID+"/tracks/track-1/flags", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, runID, "track-1")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCov_HandleUpdateTrackFlags_EmptyLabel(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "flags-empty")
	covInsertTrack(t, ws, runID, "track-1")

	body, _ := json.Marshal(map[string]interface{}{
		"user_label":       "",
		"linked_track_ids": []string{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/"+runID+"/tracks/track-1/flags", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, runID, "track-1")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- handleDeleteRunTrack additional coverage ---

func TestCov_HandleDeleteRunTrack_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/track-1", nil)
	w := httptest.NewRecorder()
	ws.handleDeleteRunTrack(w, req, "run-1", "track-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleDeleteRunTrack_NotFound(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-1/tracks/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleDeleteRunTrack(w, req, "run-1", "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleDeleteRunTrack_Success(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "del-track")
	covInsertTrack(t, ws, runID, "track-del")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/"+runID+"/tracks/track-del", nil)
	w := httptest.NewRecorder()
	ws.handleDeleteRunTrack(w, req, runID, "track-del")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- handleDeleteRun additional coverage ---

func TestCov_HandleDeleteRun_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1", nil)
	w := httptest.NewRecorder()
	ws.handleDeleteRun(w, req, "run-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleDeleteRun_NilDB(t *testing.T) {
	ws := &WebServer{db: nil}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-1", nil)
	w := httptest.NewRecorder()
	ws.handleDeleteRun(w, req, "run-1")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- handleListRuns additional coverage ---

func TestCov_HandleListRuns_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/", nil)
	w := httptest.NewRecorder()
	ws.handleListRuns(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleListRuns_WithFilters(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	covInsertRun(t, ws, "filter")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/?sensor_id=test-sensor&status=completed&limit=10", nil)
	w := httptest.NewRecorder()
	ws.handleListRuns(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestCov_HandleListRuns_InvalidLimit(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/?limit=abc", nil)
	w := httptest.NewRecorder()
	ws.handleListRuns(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// --- handleGetRun additional coverage ---

func TestCov_HandleGetRun_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/run-1", nil)
	w := httptest.NewRecorder()
	ws.handleGetRun(w, req, "run-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleEvaluateRun additional coverage ---

func TestCov_HandleEvaluateRun_InvalidJSON(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/run-1/evaluate", bytes.NewReader([]byte("bad json")))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, "run-1")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleEvaluateRun_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/evaluate", nil)
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, "run-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleEvaluateRun_NoReferenceAndNoScene(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "eval-noref")

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/evaluate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, runID)

	// Should fail: no reference_run_id and no scene with reference
	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 for missing reference, got %d", w.Code)
	}
}

func TestCov_HandleEvaluateRun_WithExplicitReference(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	refRunID := covInsertRun(t, ws, "eval-ref")
	candRunID := covInsertRun(t, ws, "eval-cand")

	// Insert tracks for both runs so evaluation has data
	covInsertTrack(t, ws, refRunID, "ref-track-001")
	covInsertTrack(t, ws, candRunID, "cand-track-001")

	body, _ := json.Marshal(map[string]interface{}{
		"reference_run_id": refRunID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+candRunID+"/evaluate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, candRunID)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCov_HandleEvaluateRun_AutoDetectFromScene(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	refRunID := covInsertRun(t, ws, "eval-scene-ref")
	candRunID := covInsertRun(t, ws, "eval-scene-cand")

	// Insert a scene linking the sensor to the reference run
	sceneStore := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{
		SensorID:       "test-sensor",
		PCAPFile:       "/test/file.pcap",
		ReferenceRunID: refRunID,
	}
	if err := sceneStore.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene: %v", err)
	}

	// Insert tracks for evaluation
	covInsertTrack(t, ws, refRunID, "scene-ref-t1")
	covInsertTrack(t, ws, candRunID, "scene-cand-t1")

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+candRunID+"/evaluate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, candRunID)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- parseRunPath / parseTrackPath ---

func TestCov_ParseRunPath_NoMatch(t *testing.T) {
	runID, subPath := parseRunPath("/not/matching/path")
	if runID != "" || subPath != "" {
		t.Errorf("expected empty, got runID=%q subPath=%q", runID, subPath)
	}
}

func TestCov_ParseRunPath_EmptyRunID(t *testing.T) {
	runID, subPath := parseRunPath("/api/lidar/runs/")
	if runID != "" {
		t.Errorf("expected empty runID, got %q", runID)
	}
	if subPath != "" {
		t.Errorf("expected empty subPath, got %q", subPath)
	}
}

func TestCov_ParseRunPath_WithSubPath(t *testing.T) {
	runID, subPath := parseRunPath("/api/lidar/runs/run-123/tracks")
	if runID != "run-123" {
		t.Errorf("runID = %q, want run-123", runID)
	}
	if subPath != "tracks" {
		t.Errorf("subPath = %q, want tracks", subPath)
	}
}

func TestCov_ParseTrackPath_SinglePart(t *testing.T) {
	trackID, action := parseTrackPath("track-1")
	if trackID != "track-1" {
		t.Errorf("trackID = %q, want track-1", trackID)
	}
	if action != "" {
		t.Errorf("action = %q, want empty", action)
	}
}

func TestCov_ParseTrackPath_WithAction(t *testing.T) {
	trackID, action := parseTrackPath("track-1/label")
	if trackID != "track-1" {
		t.Errorf("trackID = %q, want track-1", trackID)
	}
	if action != "label" {
		t.Errorf("action = %q, want label", action)
	}
}

// --- handleDeleteMissedRegion with valid region ---

func TestCov_HandleDeleteMissedRegion_Success(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "del-mr")

	// Insert a missed region to delete
	store := lidar.NewMissedRegionStore(ws.db.DB)
	region := &lidar.MissedRegion{
		RunID:       runID,
		CenterX:     1.0,
		CenterY:     2.0,
		TimeStartNs: 1000,
		TimeEndNs:   2000,
	}
	if err := store.Insert(region); err != nil {
		t.Fatalf("Insert missed region: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/"+runID+"/missed-regions/"+region.RegionID, nil)
	w := httptest.NewRecorder()
	ws.handleDeleteMissedRegion(w, req, region.RegionID)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- handleReprocessRun ---

func TestCov_HandleReprocessRun_WrongMethod(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, "run-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleReprocessRun_Success(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/run-1/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, "run-1")

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}
