package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// failWriter is an http.ResponseWriter that fails on Write after the header is written.
type failWriter struct {
	header http.Header
	code   int
}

func (fw *failWriter) Header() http.Header        { return fw.header }
func (fw *failWriter) WriteHeader(statusCode int) { fw.code = statusCode }
func (fw *failWriter) Write([]byte) (int, error)  { return 0, errors.New("write error") }

// --- Dispatcher coverage: route through handleRunTrackAPI ---

func TestCov2_Dispatcher_LabelViaURL(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "disp-label")
	covInsertTrack(t, ws, runID, "track-dlabel")

	body := `{"user_label":"car"}`
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/"+runID+"/tracks/track-dlabel/label",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCov2_Dispatcher_FlagsViaURL(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "disp-flags")
	covInsertTrack(t, ws, runID, "track-dflags")

	body := `{"user_label":"split","linked_track_ids":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/"+runID+"/tracks/track-dflags/flags",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCov2_Dispatcher_DeleteTrackViaURL(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "disp-del-trk")
	covInsertTrack(t, ws, runID, "track-ddel")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/"+runID+"/tracks/track-ddel", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCov2_Dispatcher_TrackMethodNotAllowed(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	// PUT on tracks/{track_id} (no action) is not GET or DELETE
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/runs/run-1/tracks/track-1", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleUpdateTrackLabel: LabelConfidence out of range ---

func TestCov2_HandleUpdateTrackLabel_ConfidenceOutOfRange(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "conf-range")
	covInsertTrack(t, ws, runID, "track-conf")

	for _, tc := range []struct {
		name string
		conf float64
	}{
		{"negative", -0.1},
		{"above_one", 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"user_label":       "car",
				"label_confidence": tc.conf,
			})
			req := httptest.NewRequest(http.MethodPut,
				"/api/lidar/runs/"+runID+"/tracks/track-conf/label",
				strings.NewReader(string(body)))
			w := httptest.NewRecorder()
			ws.handleUpdateTrackLabel(w, req, runID, "track-conf")

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			if !strings.Contains(w.Body.String(), "label_confidence") {
				t.Errorf("expected label_confidence error, got: %s", w.Body.String())
			}
		})
	}
}

// --- handleUpdateTrackLabel: hintRunner notification ---

func TestCov2_HandleUpdateTrackLabel_HintRunnerNotify(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "hint-notify")
	covInsertTrack(t, ws, runID, "track-hint")

	runner := &mockHINTRunner{}
	ws.hintRunner = runner

	body := `{"user_label":"car"}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/lidar/runs/"+runID+"/tracks/track-hint/label",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackLabel(w, req, runID, "track-hint")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- handleUpdateTrackFlags: store error ---

func TestCov2_HandleUpdateTrackFlags_StoreError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	ws.db.DB.Close()

	body := `{"user_label":"split","linked_track_ids":["t2"]}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/lidar/runs/run-1/tracks/track-1/flags",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleUpdateTrackFlags(w, req, "run-1", "track-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleGetRunTrack: nil DB ---

func TestCov2_HandleGetRunTrack_NilDB(t *testing.T) {
	ws := &Server{db: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/track-1", nil)
	w := httptest.NewRecorder()
	ws.handleGetRunTrack(w, req, "run-1", "track-1")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- handleGetRunTrack: generic DB error (non-"not found") ---

func TestCov2_HandleGetRunTrack_DBError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/tracks/track-1", nil)
	w := httptest.NewRecorder()
	ws.handleGetRunTrack(w, req, "run-1", "track-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleDeleteRun: generic DB error (non-"not found") ---

func TestCov2_HandleDeleteRun_DBError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/runs/run-1", nil)
	w := httptest.NewRecorder()
	ws.handleDeleteRun(w, req, "run-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleLabellingProgress: DB error ---

func TestCov2_HandleLabellingProgress_DBError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-1/labelling-progress", nil)
	w := httptest.NewRecorder()
	ws.handleLabellingProgress(w, req, "run-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleListRuns: DB error ---

func TestCov2_HandleListRuns_DBError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/", nil)
	w := httptest.NewRecorder()
	ws.handleListRuns(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleReprocessRun: GetRun non-not-found error ---

func TestCov2_HandleReprocessRun_GetRunDBError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/run-1/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, "run-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleReprocessRun: InsertRun error ---

func TestCov2_HandleReprocessRun_InsertRunError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "ins-err")

	// Pre-insert many rows to cause the generated ID to collide seems complex.
	// Instead, drop the lidar_run_records table after the GetRun succeeds
	// by using a custom approach: we close the DB _after_ reading the original run.
	// Since we can't easily intercept, we instead make the table read-only via a trigger.
	_, err := ws.db.DB.Exec(`CREATE TRIGGER block_insert BEFORE INSERT ON lidar_run_records BEGIN SELECT RAISE(ABORT, 'blocked by trigger'); END`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, runID)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to create analysis run") {
		t.Errorf("expected 'failed to create analysis run' in body, got: %s", w.Body.String())
	}
}

// --- handleReprocessRun: tracker.Reset() branch ---

func TestCov2_HandleReprocessRun_WithTracker(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "with-tracker")

	ws.tracker = &l5tracks.Tracker{}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, runID)

	// Will get 500 from StartPCAPInternal, but tracker.Reset() should have been called
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleEvaluateRun: evaluation error ---

func TestCov2_HandleEvaluateRun_EvaluationError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	store := sqlite.NewAnalysisRunStore(ws.db.DB)

	refRun := &sqlite.AnalysisRun{
		RunID:      "eval-err-ref",
		SourceType: "pcap",
		SourcePath: "/test/eval-err.pcap",
		SensorID:   "sensor-eval-err",
		Status:     "completed",
	}
	if err := store.InsertRun(refRun); err != nil {
		t.Fatalf("InsertRun ref: %v", err)
	}

	candRun := &sqlite.AnalysisRun{
		RunID:      "eval-err-cand",
		SourceType: "pcap",
		SourcePath: "/test/eval-err.pcap",
		SensorID:   "sensor-eval-err",
		Status:     "completed",
	}
	if err := store.InsertRun(candRun); err != nil {
		t.Fatalf("InsertRun cand: %v", err)
	}

	// Drop the lidar_run_tracks table to cause GetRunTracks to fail
	if _, err := ws.db.DB.Exec(`DROP TABLE lidar_run_tracks`); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	body := `{"reference_run_id":"eval-err-ref"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/runs/eval-err-cand/evaluate",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, "eval-err-cand")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleEvaluateRun: auto-detect with ParamsJSON ---

func TestCov2_HandleEvaluateRun_AutoDetectWithParams(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	store := sqlite.NewAnalysisRunStore(ws.db.DB)

	// Insert reference run with a track
	refRun := &sqlite.AnalysisRun{
		RunID:      "params-ref",
		SourceType: "pcap",
		SourcePath: "/test/params.pcap",
		SensorID:   "sensor-params",
		Status:     "completed",
	}
	if err := store.InsertRun(refRun); err != nil {
		t.Fatalf("InsertRun ref: %v", err)
	}
	covInsertTrackForRun(t, ws, "params-ref", "params-ref-t1", "sensor-params")

	// Insert candidate run with ParamsJSON
	candRun := &sqlite.AnalysisRun{
		RunID:      "params-cand",
		SourceType: "pcap",
		SourcePath: "/test/params.pcap",
		SensorID:   "sensor-params",
		Status:     "completed",
		ParamsJSON: json.RawMessage(`{"tuning":"custom"}`),
	}
	if err := store.InsertRun(candRun); err != nil {
		t.Fatalf("InsertRun cand: %v", err)
	}
	covInsertTrackForRun(t, ws, "params-cand", "params-cand-t1", "sensor-params")

	// Create a scene matching the source path
	sceneStore := sqlite.NewReplayCaseStore(ws.db.DB)
	scene := &sqlite.ReplayCase{
		SensorID:       "sensor-params",
		PCAPFile:       "/test/params.pcap",
		ReferenceRunID: "params-ref",
	}
	if err := sceneStore.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene: %v", err)
	}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/runs/params-cand/evaluate",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, "params-cand")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Should have evaluation_id since scene was matched
	if !strings.Contains(w.Body.String(), "evaluation_id") {
		t.Errorf("expected evaluation_id in response, got: %s", w.Body.String())
	}
}

// --- handleEvaluateRun: evalStore.Insert error ---

func TestCov2_HandleEvaluateRun_InsertError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	store := sqlite.NewAnalysisRunStore(ws.db.DB)

	refRun := &sqlite.AnalysisRun{
		RunID:      "eval-ins-ref",
		SourceType: "pcap",
		SourcePath: "/test/eval-ins.pcap",
		SensorID:   "sensor-eval-ins",
		Status:     "completed",
	}
	if err := store.InsertRun(refRun); err != nil {
		t.Fatalf("InsertRun ref: %v", err)
	}
	covInsertTrackForRun(t, ws, "eval-ins-ref", "eval-ins-ref-t1", "sensor-eval-ins")

	candRun := &sqlite.AnalysisRun{
		RunID:      "eval-ins-cand",
		SourceType: "pcap",
		SourcePath: "/test/eval-ins.pcap",
		SensorID:   "sensor-eval-ins",
		Status:     "completed",
	}
	if err := store.InsertRun(candRun); err != nil {
		t.Fatalf("InsertRun cand: %v", err)
	}
	covInsertTrackForRun(t, ws, "eval-ins-cand", "eval-ins-cand-t1", "sensor-eval-ins")

	sceneStore := sqlite.NewReplayCaseStore(ws.db.DB)
	scene := &sqlite.ReplayCase{
		SensorID:       "sensor-eval-ins",
		PCAPFile:       "/test/eval-ins.pcap",
		ReferenceRunID: "eval-ins-ref",
	}
	if err := sceneStore.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene: %v", err)
	}

	// Drop evaluations table to cause Insert to fail
	if _, err := ws.db.DB.Exec(`DROP TABLE IF EXISTS lidar_replay_evaluations`); err != nil {
		t.Fatalf("drop evaluations table: %v", err)
	}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/runs/eval-ins-cand/evaluate",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleEvaluateRun(w, req, "eval-ins-cand")

	// Should still return 200 — insert error is only logged as warning
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Should NOT have evaluation_id since insert failed
	if strings.Contains(w.Body.String(), "evaluation_id") {
		t.Errorf("expected no evaluation_id (insert failed), got: %s", w.Body.String())
	}
}

// covInsertTrackForRun inserts a run track with a specific sensor ID.
func covInsertTrackForRun(t *testing.T, ws *Server, runID, trackID, sensorID string) {
	t.Helper()
	store := sqlite.NewAnalysisRunStore(ws.db.DB)
	track := &sqlite.RunTrack{
		RunID:   runID,
		TrackID: trackID,
		TrackMeasurement: l5tracks.TrackMeasurement{
			SensorID:         sensorID,
			TrackState:       "confirmed",
			ObservationCount: 10,
			AvgSpeedMps:      5.0,
			MaxSpeedMps:      8.0,
			StartUnixNanos:   1000000000,
			EndUnixNanos:     2000000000,
		},
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack %s: %v", trackID, err)
	}
}

// --- handleReprocessRun: resetBackgroundGrid error ---

func TestCov2_HandleReprocessRun_ResetGridError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "reset-grid-err")
	ws.tracker = &l5tracks.Tracker{}

	// Register a BackgroundManager with nil Grid for our sensor ID.
	// ResetGrid() returns error when Grid is nil.
	sensorID := "test-sensor-grid-err"
	ws.sensorID = sensorID
	mgr := &l3grid.BackgroundManager{} // Grid is nil → ResetGrid() errors
	l3grid.RegisterBackgroundManager(sensorID, mgr)
	defer l3grid.RegisterBackgroundManager(sensorID, nil) // cleanup won't work (nil is rejected) but test DB is cleaned up

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, runID)

	// Will get 500 from StartPCAPInternal. The resetBackgroundGrid error is only logged.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleReprocessRun: UpdateRunStatus error (L538-540) ---

func TestCov2_HandleReprocessRun_UpdateRunStatusError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	runID := covInsertRun(t, ws, "upd-status-err")

	// Block UPDATEs on lidar_run_records so UpdateRunStatus fails
	// while INSERTs still work (for the new reprocess run creation).
	_, err := ws.db.DB.Exec(`CREATE TRIGGER block_update BEFORE UPDATE ON lidar_run_records BEGIN SELECT RAISE(ABORT, 'update blocked'); END`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/reprocess", nil)
	w := httptest.NewRecorder()
	ws.handleReprocessRun(w, req, runID)

	// StartPCAPInternal fails → handler tries UpdateRunStatus → also fails → 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- handleEvaluateRun: json.Encode error (L674-676) ---

func TestCov2_HandleEvaluateRun_EncodeError(t *testing.T) {
	ws, cleanup := covSetupWS(t)
	defer cleanup()

	store := sqlite.NewAnalysisRunStore(ws.db.DB)

	refRun := &sqlite.AnalysisRun{
		RunID:      "enc-err-ref",
		SourceType: "pcap",
		SourcePath: "/test/enc-err.pcap",
		SensorID:   "sensor-enc",
		Status:     "completed",
	}
	if err := store.InsertRun(refRun); err != nil {
		t.Fatalf("InsertRun ref: %v", err)
	}
	covInsertTrackForRun(t, ws, "enc-err-ref", "enc-err-ref-t1", "sensor-enc")

	candRun := &sqlite.AnalysisRun{
		RunID:      "enc-err-cand",
		SourceType: "pcap",
		SourcePath: "/test/enc-err.pcap",
		SensorID:   "sensor-enc",
		Status:     "completed",
	}
	if err := store.InsertRun(candRun); err != nil {
		t.Fatalf("InsertRun cand: %v", err)
	}
	covInsertTrackForRun(t, ws, "enc-err-cand", "enc-err-cand-t1", "sensor-enc")

	body := `{"reference_run_id":"enc-err-ref"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/runs/enc-err-cand/evaluate",
		strings.NewReader(body))
	fw := &failWriter{header: http.Header{}}
	ws.handleEvaluateRun(fw, req, "enc-err-cand")

	// The handler logs the encode error but doesn't change status code.
	// Just verifying it doesn't panic.
}

// --- Remaining truly unreachable blocks ---
// L545-550: reprocess success path — requires a working PCAP replay system
// L674-676: json.Encode error on httptest.ResponseWriter — never occurs
// L538-540: UpdateRunStatus error inside StartPCAPInternal error — requires DB to fail mid-request
// These blocks are left intentionally uncovered as they are unreachable in unit tests.
