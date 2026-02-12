package monitor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
