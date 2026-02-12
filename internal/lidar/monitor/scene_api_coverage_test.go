package monitor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// --- handleUpdateScene additional coverage ---

func TestCov_HandleUpdateScene_InvalidJSON(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/"+scene.SceneID, strings.NewReader("not json"))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleUpdateScene_NotFound(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	desc := "updated"
	body, _ := json.Marshal(UpdateSceneRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/nonexistent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleUpdateScene_AllFields(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	desc := "new description"
	refRun := "run-ref"
	startSecs := 1.5
	durSecs := 10.0
	optParams := json.RawMessage(`{"key":"value"}`)

	body, _ := json.Marshal(UpdateSceneRequest{
		Description:       &desc,
		ReferenceRunID:    &refRun,
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: &optParams,
	})

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/"+scene.SceneID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify all fields were updated
	updated, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("get scene: %v", err)
	}
	if updated.Description != desc {
		t.Errorf("description = %q, want %q", updated.Description, desc)
	}
	if updated.ReferenceRunID != refRun {
		t.Errorf("reference_run_id = %q, want %q", updated.ReferenceRunID, refRun)
	}
	if updated.PCAPStartSecs == nil || *updated.PCAPStartSecs != startSecs {
		t.Errorf("pcap_start_secs = %v, want %v", updated.PCAPStartSecs, startSecs)
	}
	if updated.PCAPDurationSecs == nil || *updated.PCAPDurationSecs != durSecs {
		t.Errorf("pcap_duration_secs = %v, want %v", updated.PCAPDurationSecs, durSecs)
	}
}

// --- handleDeleteScene additional coverage ---

func TestCov_HandleDeleteScene_NotFound(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/scenes/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- handleReplayScene with optimal params ---

func TestCov_HandleReplayScene_WithOptimalParams(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)
	startSecs := 0.5
	durSecs := 5.0
	scene := &lidar.Scene{
		SensorID:          "sensor-001",
		PCAPFile:          "test.pcap",
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"threshold": 0.5}`),
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// May fail with PCAP error but should get past the params selection logic
	if w.Code == http.StatusBadRequest || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("unexpected validation error: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCov_HandleReplayScene_WithRequestParams(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"params_json": map[string]interface{}{"threshold": 0.8},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/replay", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// Should get past validation; PCAP replay itself may fail
	if w.Code == http.StatusBadRequest || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("unexpected validation error: status=%d body=%s", w.Code, w.Body.String())
	}
}

// --- handleSceneByID missing scene_id ---

func TestCov_HandleSceneByID_NilDB(t *testing.T) {
	ws := &WebServer{db: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-1", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestCov_HandleSceneByID_MissingSceneID(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- handleScenes NilDB for POST ---

func TestCov_HandleScenes_NilDB_POST(t *testing.T) {
	ws := &WebServer{db: nil}

	body, _ := json.Marshal(CreateSceneRequest{SensorID: "s1", PCAPFile: "p.pcap"})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleScenes(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- handleListSceneEvaluations wrong method ---

func TestCov_HandleListSceneEvaluations_WrongMethod(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/scene-1/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleReplayScene evaluations that dispatch via handleSceneByID method filtering ---

func TestCov_HandleSceneByID_ReplayWrongMethod(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-1/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleSceneByID POST on scene (not replay) ---
func TestCov_HandleSceneByID_PostOnScene(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/scene-1", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- writeJSON helper ---

func TestCov_WriteJSON(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()
	ws := &WebServer{db: &db.DB{DB: testDB}}

	w := httptest.NewRecorder()
	ws.writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestCov_WriteJSONError(t *testing.T) {
	ws := &WebServer{}

	w := httptest.NewRecorder()
	ws.writeJSONError(w, http.StatusBadRequest, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "test error") {
		t.Errorf("body = %q, want to contain 'test error'", w.Body.String())
	}
}

// --- DB error paths (closed DB) ---

func TestCov_HandleListScenes_DBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	ws.db.DB.Close() // force DB error

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes", nil)
	w := httptest.NewRecorder()
	ws.handleScenes(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCov_HandleGetScene_GenericDBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	ws.db.DB.Close() // force generic DB error (not "not found")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-1", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleUpdateScene_GenericDBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	ws.db.DB.Close() // force generic DB error in GetScene

	desc := "updated"
	body, _ := json.Marshal(UpdateSceneRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/scene-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleUpdateScene_StoreError(t *testing.T) {
	ws := setupTestSceneWebServer(t)

	// Insert a scene, then drop the table so UpdateScene fails
	store := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Rename/drop the table to break update but allow read
	// Actually - just close after creating the scene won't work for GetScene.
	// Let's corrupt: drop a column that UpdateScene needs
	_, _ = ws.db.DB.Exec("DROP TABLE lidar_scenes")
	_, _ = ws.db.DB.Exec(`CREATE TABLE lidar_scenes (scene_id TEXT PRIMARY KEY)`) // minimal table

	desc := "updated"
	body, _ := json.Marshal(UpdateSceneRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/"+scene.SceneID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// GetScene will fail since columns are missing
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want error status; body: %s", w.Code, w.Body.String())
	}
}

func TestCov_HandleDeleteScene_GenericDBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	ws.db.DB.Close() // force generic DB error (not "not found")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/scenes/scene-1", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleCreateScene_DBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	ws.db.DB.Close() // force DB error on InsertScene

	body, _ := json.Marshal(CreateSceneRequest{SensorID: "s1", PCAPFile: "p.pcap"})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleScenes(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleReplayScene_InsertRunDBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)

	// Insert scene first
	store := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Drop analysis_runs table to force InsertRun failure
	_, _ = ws.db.DB.Exec("DROP TABLE lidar_analysis_runs")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleReplayScene_GenericDBError(t *testing.T) {
	ws := setupTestSceneWebServer(t)

	// Insert scene, then close DB so GetScene fails with generic error
	store := lidar.NewSceneStore(ws.db.DB)
	scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}
	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleListScenes_EmptyReturnsArray(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	// No scenes inserted — should return empty array, not null
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes", nil)
	w := httptest.NewRecorder()
	ws.handleScenes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	scenes, ok := resp["scenes"].([]interface{})
	if !ok {
		t.Fatal("expected scenes to be an array")
	}
	if len(scenes) != 0 {
		t.Errorf("expected 0 scenes, got %d", len(scenes))
	}
}

func TestCov_HandleListSceneEvaluations_GET(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-1/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(w.Body.String(), "scene-1") {
		t.Errorf("expected scene_id in response, got: %s", w.Body.String())
	}
}

// setupTestSceneAPIDBFull creates a test DB with the full lidar_analysis_runs
// schema so that InsertRun succeeds (the original helper only has 2 columns).
func setupTestSceneAPIDBFull(t *testing.T) *db.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Full lidar_analysis_runs table (17 columns matching InsertRun query)
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
			run_id TEXT PRIMARY KEY,
			created_at INTEGER,
			source_type TEXT,
			source_path TEXT,
			sensor_id TEXT,
			params_json TEXT,
			duration_secs REAL DEFAULT 0,
			total_frames INTEGER DEFAULT 0,
			total_clusters INTEGER DEFAULT 0,
			total_tracks INTEGER DEFAULT 0,
			confirmed_tracks INTEGER DEFAULT 0,
			processing_time_ms INTEGER DEFAULT 0,
			status TEXT DEFAULT 'running',
			error_message TEXT,
			parent_run_id TEXT,
			notes TEXT,
			vrlog_path TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_analysis_runs table: %v", err)
	}

	// lidar_scenes table
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_scenes (
			scene_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			pcap_file TEXT NOT NULL,
			pcap_start_secs REAL,
			pcap_duration_secs REAL,
			description TEXT,
			reference_run_id TEXT,
			optimal_params_json TEXT,
			created_at_ns INTEGER NOT NULL,
			updated_at_ns INTEGER,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE SET NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_scenes table: %v", err)
	}

	return &db.DB{DB: sqlDB}
}

// --- handleReplayScene: invalid JSON body ---

func TestSceneCov2_ReplayInvalidJSON(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &WebServer{db: testDB}

	// Insert a scene so GetScene succeeds
	store := lidar.NewSceneStore(testDB.DB)
	scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Send malformed JSON body — should trigger err != io.EOF branch
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay",
		strings.NewReader("{bad json"))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in body, got: %s", w.Body.String())
	}
}

// --- handleReplayScene: pcap in progress, with tracker + background manager ---

func TestSceneCov2_ReplayPCAPInProgress(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	// Create temp dir with dummy .pcap file for resolvePCAPPath
	tmpDir := t.TempDir()
	pcapName := "test.pcap"
	if err := os.WriteFile(filepath.Join(tmpDir, pcapName), []byte("dummy pcap"), 0644); err != nil {
		t.Fatalf("write pcap: %v", err)
	}

	sensorID := "sensor-cov2-replay"

	ws := &WebServer{
		db:          testDB,
		sensorID:    sensorID,
		pcapSafeDir: tmpDir,
		baseCtx:     context.Background(),
		tracker:     lidar.NewTracker(lidar.DefaultTrackerConfig()),
	}

	// Register a valid BackgroundManager so resetBackgroundGrid exercises the Reset path
	bgm := lidar.NewBackgroundManager(sensorID, 16, 360, lidar.BackgroundParams{}, nil)
	lidar.RegisterBackgroundManager(sensorID, bgm)

	// Mark PCAP as already in progress
	ws.pcapMu.Lock()
	ws.pcapInProgress = true
	ws.pcapMu.Unlock()

	// Insert scene
	store := lidar.NewSceneStore(testDB.DB)
	startSecs := 1.0
	durSecs := 5.0
	scene := &lidar.Scene{
		SensorID:         sensorID,
		PCAPFile:         pcapName,
		PCAPStartSecs:    &startSecs,
		PCAPDurationSecs: &durSecs,
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// StartPCAPInternal fails because pcap already in progress
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PCAP replay") {
		t.Errorf("expected PCAP replay error, got: %s", w.Body.String())
	}
}

// --- handleReplayScene: broken BackgroundManager (nil grid) logs warning ---

func TestSceneCov2_ReplayBrokenBGManager(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	sensorID := "sensor-cov2-brokenbgm"

	ws := &WebServer{
		db:       testDB,
		sensorID: sensorID,
	}

	// Register a BackgroundManager with nil Grid — ResetGrid returns error
	lidar.RegisterBackgroundManager(sensorID, &lidar.BackgroundManager{})

	// Insert scene
	store := lidar.NewSceneStore(testDB.DB)
	scene := &lidar.Scene{SensorID: sensorID, PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// Should still get past resetBackgroundGrid (error is just logged)
	// Then fail at StartPCAPInternal because pcapSafeDir is empty
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayScene: neither request params nor optimal params ---

func TestSceneCov2_ReplayNoParams(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &WebServer{db: testDB, sensorID: "sensor-cov2-noparams"}

	store := lidar.NewSceneStore(testDB.DB)
	scene := &lidar.Scene{
		SensorID: "sensor-cov2-noparams",
		PCAPFile: "test.pcap",
		// No OptimalParamsJSON, no PCAPStartSecs, no PCAPDurationSecs
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Empty body — no params_json
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// InsertRun should succeed (full table), then StartPCAPInternal fails
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayScene: request params override scene optimal params ---

func TestSceneCov2_ReplayRequestParamsOverride(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &WebServer{db: testDB, sensorID: "sensor-cov2-override"}

	store := lidar.NewSceneStore(testDB.DB)
	scene := &lidar.Scene{
		SensorID:          "sensor-cov2-override",
		PCAPFile:          "test.pcap",
		OptimalParamsJSON: json.RawMessage(`{"threshold": 0.5}`),
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Send request params that should override scene's optimal params
	body, _ := json.Marshal(map[string]interface{}{
		"params_json": map[string]interface{}{"threshold": 0.9},
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// InsertRun succeeds, then StartPCAPInternal fails
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayScene: UpdateRunStatus also fails ---

func TestSceneCov2_ReplayUpdateStatusError(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &WebServer{db: testDB, sensorID: "sensor-cov2-updateerr"}

	store := lidar.NewSceneStore(testDB.DB)
	scene := &lidar.Scene{SensorID: "sensor-cov2-updateerr", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Add trigger to block UPDATE on lidar_analysis_runs, so UpdateRunStatus fails
	// while InsertRun still succeeds.
	_, err := testDB.DB.Exec(`
		CREATE TRIGGER prevent_update_runs BEFORE UPDATE ON lidar_analysis_runs
		BEGIN
			SELECT RAISE(FAIL, 'update blocked by trigger');
		END
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// InsertRun succeeds → StartPCAPInternal fails → UpdateRunStatus fails (trigger)
	// → handler still returns 500 from the StartPCAPInternal error
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayScene: scene with OptimalParamsJSON, full DB ---

func TestSceneCov2_ReplayWithOptimalParamsFullDB(t *testing.T) {
	testDB := setupTestSceneAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &WebServer{db: testDB, sensorID: "sensor-cov2-optparams"}

	store := lidar.NewSceneStore(testDB.DB)
	startSecs := 0.5
	durSecs := 10.0
	scene := &lidar.Scene{
		SensorID:          "sensor-cov2-optparams",
		PCAPFile:          "test.pcap",
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"threshold": 0.5}`),
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	// InsertRun succeeds with full DB, then StartPCAPInternal fails (no pcapSafeDir)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PCAP replay") {
		t.Errorf("expected PCAP replay error, got: %s", w.Body.String())
	}
}

// --- parseScenePath edge cases ---

func TestSceneCov2_ParseScenePathNoPrefix(t *testing.T) {
	sceneID, action := parseScenePath("/some/other/path")
	if sceneID != "" || action != "" {
		t.Errorf("expected empty results for non-matching path, got sceneID=%q action=%q", sceneID, action)
	}
}

func TestSceneCov2_ParseScenePathWithAction(t *testing.T) {
	sceneID, action := parseScenePath("/api/lidar/scenes/abc-123/replay")
	if sceneID != "abc-123" {
		t.Errorf("sceneID = %q, want %q", sceneID, "abc-123")
	}
	if action != "replay" {
		t.Errorf("action = %q, want %q", action, "replay")
	}
}

func TestSceneCov2_ParseScenePathSceneOnly(t *testing.T) {
	sceneID, action := parseScenePath("/api/lidar/scenes/abc-123")
	if sceneID != "abc-123" {
		t.Errorf("sceneID = %q, want %q", sceneID, "abc-123")
	}
	if action != "" {
		t.Errorf("action = %q, want empty", action)
	}
}

// --- handleSceneByID: unknown action ---

func TestSceneCov2_UnknownAction(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-1/bogus", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "endpoint not found") {
		t.Errorf("expected 'endpoint not found', got: %s", w.Body.String())
	}
}

// --- handleScenes: unsupported method ---

func TestSceneCov2_HandleScenesMethodNotAllowed(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPatch, "/api/lidar/scenes", nil)
	w := httptest.NewRecorder()
	ws.handleScenes(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
