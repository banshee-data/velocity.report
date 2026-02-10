package monitor

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
	_ "github.com/mattn/go-sqlite3"
)

// setupTestSceneAPIDB creates a test database with required tables.
func setupTestSceneAPIDB(t *testing.T) *db.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create lidar_analysis_runs table (referenced by FK)
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
			run_id TEXT PRIMARY KEY,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_analysis_runs table: %v", err)
	}

	// Create lidar_scenes table
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

// setupTestSceneWebServer creates a WebServer for testing scene API.
func setupTestSceneWebServer(t *testing.T) *WebServer {
	t.Helper()
	testDB := setupTestSceneAPIDB(t)

	ws := &WebServer{
		db: testDB,
	}

	return ws
}

func TestSceneAPI_CreateScene(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
		wantError  string
	}{
		{
			name: "valid scene",
			body: CreateSceneRequest{
				SensorID:    "sensor-001",
				PCAPFile:    "test.pcap",
				Description: "Test scene",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing sensor_id",
			body: CreateSceneRequest{
				PCAPFile: "test.pcap",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "sensor_id is required",
		},
		{
			name: "missing pcap_file",
			body: CreateSceneRequest{
				SensorID: "sensor-001",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "pcap_file is required",
		},
		{
			name:       "invalid JSON",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			var err error

			if str, ok := tt.body.(string); ok {
				bodyBytes = []byte(str)
			} else {
				bodyBytes, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			ws.handleScenes(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantError != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantError) {
					t.Errorf("error message = %q, want to contain %q", body, tt.wantError)
				}
			}

			if w.Code == http.StatusCreated {
				var resp lidar.Scene
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if resp.SceneID == "" {
					t.Error("expected scene_id to be set")
				}
				if resp.SensorID != "sensor-001" {
					t.Errorf("sensor_id = %s, want sensor-001", resp.SensorID)
				}
			}
		})
	}
}

func TestSceneAPI_ListScenes(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)

	// Insert test scenes
	scenes := []*lidar.Scene{
		{SensorID: "sensor-001", PCAPFile: "scene1.pcap"},
		{SensorID: "sensor-001", PCAPFile: "scene2.pcap"},
		{SensorID: "sensor-002", PCAPFile: "scene3.pcap"},
	}
	for _, scene := range scenes {
		if err := store.InsertScene(scene); err != nil {
			t.Fatalf("failed to insert scene: %v", err)
		}
	}

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "all scenes",
			query:     "",
			wantCount: 3,
		},
		{
			name:      "filter by sensor_id",
			query:     "?sensor_id=sensor-001",
			wantCount: 2,
		},
		{
			name:      "no matches",
			query:     "?sensor_id=sensor-999",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes"+tt.query, nil)
			w := httptest.NewRecorder()

			ws.handleScenes(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			scenes, ok := resp["scenes"].([]interface{})
			if !ok {
				t.Fatal("expected scenes array in response")
			}

			if len(scenes) != tt.wantCount {
				t.Errorf("got %d scenes, want %d", len(scenes), tt.wantCount)
			}
		})
	}
}

func TestSceneAPI_GetScene(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)

	// Insert test scene
	scene := &lidar.Scene{
		SensorID:    "sensor-001",
		PCAPFile:    "test.pcap",
		Description: "Test scene",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("failed to insert scene: %v", err)
	}

	tests := []struct {
		name       string
		sceneID    string
		wantStatus int
	}{
		{
			name:       "existing scene",
			sceneID:    scene.SceneID,
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent scene",
			sceneID:    "non-existent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/"+tt.sceneID, nil)
			w := httptest.NewRecorder()

			ws.handleSceneByID(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp lidar.Scene
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if resp.SceneID != scene.SceneID {
					t.Errorf("scene_id = %s, want %s", resp.SceneID, scene.SceneID)
				}
			}
		})
	}
}

func TestSceneAPI_UpdateScene(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)

	// Insert test scene
	scene := &lidar.Scene{
		SensorID:    "sensor-001",
		PCAPFile:    "test.pcap",
		Description: "Original",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("failed to insert scene: %v", err)
	}

	newDesc := "Updated description"
	newRunID := "run-123"
	updateReq := UpdateSceneRequest{
		Description:    &newDesc,
		ReferenceRunID: &newRunID,
	}

	bodyBytes, _ := json.Marshal(updateReq)
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/scenes/"+scene.SceneID, bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	ws.handleSceneByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update
	retrieved, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("failed to get scene: %v", err)
	}
	if retrieved.Description != newDesc {
		t.Errorf("description = %s, want %s", retrieved.Description, newDesc)
	}
	if retrieved.ReferenceRunID != newRunID {
		t.Errorf("reference_run_id = %s, want %s", retrieved.ReferenceRunID, newRunID)
	}
}

func TestSceneAPI_DeleteScene(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)

	// Insert test scene
	scene := &lidar.Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("failed to insert scene: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/scenes/"+scene.SceneID, nil)
	w := httptest.NewRecorder()

	ws.handleSceneByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify deletion
	_, err := store.GetScene(scene.SceneID)
	if err == nil {
		t.Error("expected scene to be deleted")
	}
}

func TestSceneAPI_ReplayScene(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	store := lidar.NewSceneStore(ws.db.DB)

	// Insert test scene
	scene := &lidar.Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("failed to insert scene: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/replay", nil)
	w := httptest.NewRecorder()

	ws.handleSceneByID(w, req)

	// Should return 202 Accepted (Phase 2.4 implemented - creates run and starts PCAP replay)
	// Note: PCAP replay will fail without pcap build tag, but run creation should succeed
	if w.Code != http.StatusAccepted && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d or %d (PCAP replay may fail without build tag)",
			w.Code, http.StatusAccepted, http.StatusInternalServerError)
	}

	// If successful, verify response contains run_id
	if w.Code == http.StatusAccepted {
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if _, ok := resp["run_id"]; !ok {
			t.Error("response should contain run_id field")
		}
		if resp["scene_id"] != scene.SceneID {
			t.Errorf("scene_id = %v, want %s", resp["scene_id"], scene.SceneID)
		}
	}
}

func TestSceneAPI_MethodNotAllowed(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "PATCH on scenes collection",
			method: http.MethodPatch,
			path:   "/api/lidar/scenes",
		},
		{
			name:   "POST on individual scene",
			method: http.MethodPost,
			path:   "/api/lidar/scenes/scene-123",
		},
		{
			name:   "DELETE on replay endpoint",
			method: http.MethodDelete,
			path:   "/api/lidar/scenes/scene-123/replay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			if strings.HasSuffix(tt.path, "/scenes") {
				ws.handleScenes(w, req)
			} else {
				ws.handleSceneByID(w, req)
			}

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestSceneAPI_NoDatabaseConfigured(t *testing.T) {
	ws := &WebServer{
		db: nil, // No database configured
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes", nil)
	w := httptest.NewRecorder()

	ws.handleScenes(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestParseScenePath(t *testing.T) {
	tests := []struct {
		path      string
		wantScene string
		wantAct   string
	}{
		{"/api/lidar/scenes/scene-1", "scene-1", ""},
		{"/api/lidar/scenes/scene-1/replay", "scene-1", "replay"},
		{"/api/lidar/scenes/scene-1/evaluations", "scene-1", "evaluations"},
		{"/wrong/prefix", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			sceneID, action := parseScenePath(tt.path)
			if sceneID != tt.wantScene {
				t.Fatalf("sceneID mismatch: got %q want %q", sceneID, tt.wantScene)
			}
			if action != tt.wantAct {
				t.Fatalf("action mismatch: got %q want %q", action, tt.wantAct)
			}
		})
	}
}

func TestSceneAPI_ListSceneEvaluations_NotImplemented(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-xyz/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotImplemented, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not_implemented") {
		t.Fatalf("expected not_implemented marker in body, got %s", w.Body.String())
	}
}

func TestSceneAPI_ReplayScene_ErrorBranches(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	t.Run("scene not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/nonexistent/replay", nil)
		w := httptest.NewRecorder()
		ws.handleSceneByID(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("invalid json body", func(t *testing.T) {
		store := lidar.NewSceneStore(ws.db.DB)
		scene := &lidar.Scene{SensorID: "sensor-001", PCAPFile: "test.pcap"}
		if err := store.InsertScene(scene); err != nil {
			t.Fatalf("failed to insert scene: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/lidar/scenes/"+scene.SceneID+"/replay", strings.NewReader("{"))
		w := httptest.NewRecorder()
		ws.handleSceneByID(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
	})
}

func TestSceneAPI_HandleSceneByID_UnknownAction(t *testing.T) {
	ws := setupTestSceneWebServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/scenes/scene-1/unknown-action", nil)
	w := httptest.NewRecorder()
	ws.handleSceneByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
