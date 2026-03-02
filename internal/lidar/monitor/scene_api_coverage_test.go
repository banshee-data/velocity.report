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
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// --- handleUpdateScene additional coverage ---

func TestCov_HandleUpdateScene_InvalidJSON(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewSceneStore(ws.db.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
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

	store := sqlite.NewSceneStore(ws.db.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
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

	store := sqlite.NewSceneStore(ws.db.DB)
	startSecs := 0.5
	durSecs := 5.0
	scene := &sqlite.Scene{
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

	store := sqlite.NewSceneStore(ws.db.DB)
	scene := &sqlite.Scene{
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

	// PUT is not supported for evaluations (GET and POST are)
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/scene-1/evaluations", nil)
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
	store := sqlite.NewSceneStore(ws.db.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
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
	store := sqlite.NewSceneStore(ws.db.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
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
	store := sqlite.NewSceneStore(ws.db.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
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

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
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
	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
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
		tracker:     l5tracks.NewTracker(l5tracks.DefaultTrackerConfig()),
	}

	// Register a valid BackgroundManager so resetBackgroundGrid exercises the Reset path
	bgm := l3grid.NewBackgroundManager(sensorID, 16, 360, l3grid.BackgroundParams{}, nil)
	l3grid.RegisterBackgroundManager(sensorID, bgm)

	// Mark PCAP as already in progress
	ws.pcapMu.Lock()
	ws.pcapInProgress = true
	ws.pcapMu.Unlock()

	// Insert scene
	store := sqlite.NewSceneStore(testDB.DB)
	startSecs := 1.0
	durSecs := 5.0
	scene := &sqlite.Scene{
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
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})

	// Insert scene
	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: sensorID, PCAPFile: "test.pcap"}
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

	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{
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

	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{
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

	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: "sensor-cov2-updateerr", PCAPFile: "test.pcap"}
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

	store := sqlite.NewSceneStore(testDB.DB)
	startSecs := 0.5
	durSecs := 10.0
	scene := &sqlite.Scene{
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

// --- handleCreateSceneEvaluation coverage ---

// setupTestSceneAPIDBWithEvaluations creates a full test DB with all tables needed
// for evaluation testing (scenes, analysis runs, run tracks, evaluations).
func setupTestSceneAPIDBWithEvaluations(t *testing.T) *db.DB {
	t.Helper()
	testDB := setupTestSceneAPIDBFull(t)

	// Add lidar_evaluations table
	_, err := testDB.DB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_evaluations (
			evaluation_id       TEXT PRIMARY KEY,
			scene_id            TEXT NOT NULL,
			reference_run_id    TEXT NOT NULL,
			candidate_run_id    TEXT NOT NULL,
			detection_rate      REAL,
			fragmentation       REAL,
			false_positive_rate REAL,
			velocity_coverage   REAL,
			quality_premium     REAL,
			truncation_rate     REAL,
			velocity_noise_rate REAL,
			stopped_recovery_rate REAL,
			composite_score     REAL,
			matched_count       INTEGER,
			reference_count     INTEGER,
			candidate_count     INTEGER,
			params_json         TEXT,
			created_at          INTEGER NOT NULL,
			FOREIGN KEY (scene_id) REFERENCES lidar_scenes(scene_id) ON DELETE CASCADE,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id),
			FOREIGN KEY (candidate_run_id) REFERENCES lidar_analysis_runs(run_id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_pair ON lidar_evaluations(reference_run_id, candidate_run_id);
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_evaluations table: %v", err)
	}

	// Add lidar_run_tracks table (needed for GroundTruthEvaluator)
	_, err = testDB.DB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_run_tracks (
			run_id TEXT NOT NULL,
			track_id TEXT NOT NULL,
			sensor_id TEXT,
			track_state TEXT,
			start_unix_nanos INTEGER,
			end_unix_nanos INTEGER,
			observation_count INTEGER DEFAULT 0,
			avg_speed_mps REAL DEFAULT 0,
			peak_speed_mps REAL DEFAULT 0,
			p50_speed_mps REAL DEFAULT 0,
			p85_speed_mps REAL DEFAULT 0,
			p95_speed_mps REAL DEFAULT 0,
			bounding_box_length_avg REAL DEFAULT 0,
			bounding_box_width_avg REAL DEFAULT 0,
			bounding_box_height_avg REAL DEFAULT 0,
			height_p95_max REAL DEFAULT 0,
			intensity_mean_avg REAL DEFAULT 0,
			object_class TEXT,
			object_confidence REAL DEFAULT 0,
			classification_model TEXT,
			user_label TEXT,
			label_confidence REAL DEFAULT 0,
			labeler_id TEXT,
			labeled_at INTEGER,
			quality_label TEXT,
			label_source TEXT,
			is_split_candidate INTEGER DEFAULT 0,
			is_merge_candidate INTEGER DEFAULT 0,
			linked_track_ids TEXT,
			PRIMARY KEY (run_id, track_id),
			FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_run_tracks table: %v", err)
	}

	return testDB
}

func TestCov_HandleCreateSceneEvaluation_SceneNotFound(t *testing.T) {
	testDB := setupTestSceneAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &WebServer{db: testDB}

	body := `{"candidate_run_id": "cand-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/nonexistent/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateSceneEvaluation(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCov_HandleCreateSceneEvaluation_NoReferenceRun(t *testing.T) {
	testDB := setupTestSceneAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &WebServer{db: testDB}

	// Insert scene without reference_run_id
	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert scene: %v", err)
	}

	body := `{"candidate_run_id": "cand-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateSceneEvaluation(w, req, scene.SceneID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no reference run") {
		t.Errorf("expected 'no reference run' in error, got: %s", w.Body.String())
	}
}

func TestCov_HandleCreateSceneEvaluation_InvalidJSON(t *testing.T) {
	testDB := setupTestSceneAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &WebServer{db: testDB}

	// Insert run and scene with reference
	runStore := sqlite.NewAnalysisRunStore(testDB.DB)
	refRun := &sqlite.AnalysisRun{RunID: "ref-run-eval", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap", ReferenceRunID: "ref-run-eval"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert scene: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/evaluations", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()
	ws.handleCreateSceneEvaluation(w, req, scene.SceneID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleCreateSceneEvaluation_MissingCandidateID(t *testing.T) {
	testDB := setupTestSceneAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &WebServer{db: testDB}

	// Insert run and scene with reference
	runStore := sqlite.NewAnalysisRunStore(testDB.DB)
	refRun := &sqlite.AnalysisRun{RunID: "ref-run-eval2", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap", ReferenceRunID: "ref-run-eval2"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert scene: %v", err)
	}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateSceneEvaluation(w, req, scene.SceneID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "candidate_run_id is required") {
		t.Errorf("expected 'candidate_run_id is required', got: %s", w.Body.String())
	}
}

func TestCov_HandleCreateSceneEvaluation_Success(t *testing.T) {
	testDB := setupTestSceneAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &WebServer{db: testDB}

	// Insert reference and candidate runs with tracks
	runStore := sqlite.NewAnalysisRunStore(testDB.DB)
	refRun := &sqlite.AnalysisRun{RunID: "ref-run-success", SourceType: "pcap", SensorID: "sensor-001", Status: "completed", ParamsJSON: json.RawMessage(`{}`)}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	candRun := &sqlite.AnalysisRun{RunID: "cand-run-success", SourceType: "pcap", SensorID: "sensor-001", Status: "completed", ParamsJSON: json.RawMessage(`{"eps": 0.5}`)}
	if err := runStore.InsertRun(candRun); err != nil {
		t.Fatalf("insert cand run: %v", err)
	}

	// Insert tracks for both runs so evaluation can work
	for _, rt := range []struct{ runID, trackID string }{
		{"ref-run-success", "ref-track-1"},
		{"cand-run-success", "cand-track-1"},
	} {
		track := &sqlite.RunTrack{
			RunID:            rt.runID,
			TrackID:          rt.trackID,
			SensorID:         "sensor-001",
			TrackState:       "confirmed",
			StartUnixNanos:   1000000000,
			EndUnixNanos:     2000000000,
			ObservationCount: 10,
			AvgSpeedMps:      5.0,
		}
		if err := runStore.InsertRunTrack(track); err != nil {
			t.Fatalf("insert track %s: %v", rt.trackID, err)
		}
	}

	// Create scene with reference run
	store := sqlite.NewSceneStore(testDB.DB)
	scene := &sqlite.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap", ReferenceRunID: "ref-run-success"}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("insert scene: %v", err)
	}

	body := `{"candidate_run_id": "cand-run-success"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateSceneEvaluation(w, req, scene.SceneID)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify the evaluation was persisted
	var resp sqlite.Evaluation
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.EvaluationID == "" {
		t.Error("expected evaluation_id in response")
	}
	if resp.ReferenceRunID != "ref-run-success" {
		t.Errorf("reference_run_id = %s, want ref-run-success", resp.ReferenceRunID)
	}
	if resp.CandidateRunID != "cand-run-success" {
		t.Errorf("candidate_run_id = %s, want cand-run-success", resp.CandidateRunID)
	}

	// Verify listing works
	evalStore := sqlite.NewEvaluationStore(testDB.DB)
	evals, err := evalStore.ListByScene(scene.SceneID)
	if err != nil {
		t.Fatalf("ListByScene: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(evals))
	}
}

func TestCov_HandleListSceneEvaluations_WithData(t *testing.T) {
	testDB := setupTestSceneAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &WebServer{db: testDB}

	// Insert scene and a pre-existing evaluation
	_, err := testDB.DB.Exec(`INSERT INTO lidar_scenes (scene_id, sensor_id, pcap_file, created_at_ns) VALUES ('scene-eval-list', 'sensor-1', 'test.pcap', 1000)`)
	if err != nil {
		t.Fatalf("insert scene: %v", err)
	}
	_, err = testDB.DB.Exec(`INSERT INTO lidar_analysis_runs (run_id, created_at, source_type, sensor_id, status) VALUES ('ref-list', 0, 'pcap', 'sensor-1', 'completed')`)
	if err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	_, err = testDB.DB.Exec(`INSERT INTO lidar_analysis_runs (run_id, created_at, source_type, sensor_id, status) VALUES ('cand-list', 0, 'pcap', 'sensor-1', 'completed')`)
	if err != nil {
		t.Fatalf("insert cand run: %v", err)
	}

	evalStore := sqlite.NewEvaluationStore(testDB.DB)
	eval := &sqlite.Evaluation{
		SceneID:        "scene-eval-list",
		ReferenceRunID: "ref-list",
		CandidateRunID: "cand-list",
		CompositeScore: 0.85,
		DetectionRate:  0.9,
		MatchedCount:   8,
		ReferenceCount: 10,
		CandidateCount: 9,
	}
	if err := evalStore.Insert(eval); err != nil {
		t.Fatalf("insert evaluation: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-eval-list/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleListSceneEvaluations(w, req, "scene-eval-list")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "0.85") {
		t.Errorf("expected composite_score 0.85 in response, got: %s", w.Body.String())
	}
}
