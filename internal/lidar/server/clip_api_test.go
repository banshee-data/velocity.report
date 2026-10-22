package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	_ "modernc.org/sqlite"
)

// setupTestClipAPIDB creates a test database with required tables.
func setupTestClipAPIDB(t *testing.T) *db.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create lidar_runs table (referenced by FK)
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_runs (
			run_id TEXT PRIMARY KEY,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_runs table: %v", err)
	}

	// Create lidar_param_sets table (needed by normalizeRecommendedParamSet)
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_param_sets (
			param_set_id TEXT PRIMARY KEY,
			params_hash TEXT NOT NULL DEFAULT '',
			schema_version TEXT NOT NULL DEFAULT '',
			param_set_type TEXT NOT NULL DEFAULT '',
			params_json TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_param_sets table: %v", err)
	}

	// Create lidar_clips table
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_clips (
			clip_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			pcap_start_secs REAL,
			pcap_duration_secs REAL,
			description TEXT,
			reference_run_id TEXT,
			created_at_ns INTEGER NOT NULL,
			updated_at_ns INTEGER,
			recommended_param_set_id TEXT,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_runs(run_id) ON DELETE SET NULL
		);

	CREATE TABLE IF NOT EXISTS lidar_clip_captures (
		clip_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		capture_file_id TEXT,
		pcap_file TEXT NOT NULL,
		PRIMARY KEY (clip_id, ordinal)
	)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_clips table: %v", err)
	}

	// Create lidar_clip_evaluations table
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_clip_evaluations (
			evaluation_id TEXT PRIMARY KEY,
			clip_id TEXT NOT NULL,
			reference_run_id TEXT NOT NULL,
			candidate_run_id TEXT NOT NULL,
			detection_rate REAL,
			fragmentation REAL,
			false_positive_rate REAL,
			velocity_coverage REAL,
			quality_premium REAL,
			truncation_rate REAL,
			velocity_noise_rate REAL,
			stopped_recovery_rate REAL,
			composite_score REAL,
			matched_count INTEGER,
			reference_count INTEGER,
			candidate_count INTEGER,
			created_at INTEGER NOT NULL,
			FOREIGN KEY (clip_id) REFERENCES lidar_clips(clip_id) ON DELETE CASCADE,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_runs(run_id),
			FOREIGN KEY (candidate_run_id) REFERENCES lidar_runs(run_id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_pair ON lidar_clip_evaluations(reference_run_id, candidate_run_id);
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_clip_evaluations table: %v", err)
	}

	return &db.DB{DB: sqlDB}
}

// setupTestClipServer creates a Server for testing clip API.
func setupTestClipServer(t *testing.T) *Server {
	t.Helper()
	testDB := setupTestClipAPIDB(t)

	ws := &Server{
		db: testDB,
	}

	return ws
}

func TestClipAPI_CreateClip(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
		wantError  string
	}{
		{
			name: "valid clip",
			body: CreateClipRequest{
				SensorID:    "sensor-001",
				PCAPFiles:   []string{"test.pcap"},
				Description: "Test clip",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing sensor_id",
			body: CreateClipRequest{
				PCAPFiles: []string{"test.pcap"},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "sensor_id is required",
		},
		{
			name: "missing pcap_file",
			body: CreateClipRequest{
				SensorID: "sensor-001",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "pcap_files is required",
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

			req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			ws.handleClips(w, req)

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
				var resp sqlite.Clip
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if resp.ClipID == "" {
					t.Error("expected clip_id to be set")
				}
				if resp.SensorID != "sensor-001" {
					t.Errorf("sensor_id = %s, want sensor-001", resp.SensorID)
				}
			}
		})
	}
}

func TestClipAPI_ListClips(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)

	// Insert test clips
	clips := []*sqlite.Clip{
		{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "clip1.pcap"}}},
		{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "clip2.pcap"}}},
		{SensorID: "sensor-002", Files: []sqlite.ClipCapture{{PCAPFile: "clip3.pcap"}}},
	}
	for _, clip := range clips {
		if err := store.InsertClip(clip); err != nil {
			t.Fatalf("failed to insert clip: %v", err)
		}
	}

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "all clips",
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
			req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips"+tt.query, nil)
			w := httptest.NewRecorder()

			ws.handleClips(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			clips, ok := resp["clips"].([]interface{})
			if !ok {
				t.Fatal("expected clips array in response")
			}

			if len(clips) != tt.wantCount {
				t.Errorf("got %d clips, want %d", len(clips), tt.wantCount)
			}
		})
	}
}

func TestClipAPI_GetClip(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)

	// Insert test clip
	clip := &sqlite.Clip{
		SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, Description: "Test clip",
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("failed to insert clip: %v", err)
	}

	tests := []struct {
		name       string
		clipID     string
		wantStatus int
	}{
		{
			name:       "existing clip",
			clipID:     clip.ClipID,
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent clip",
			clipID:     "non-existent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/"+tt.clipID, nil)
			w := httptest.NewRecorder()

			ws.handleClipByID(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp sqlite.Clip
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if resp.ClipID != clip.ClipID {
					t.Errorf("clip_id = %s, want %s", resp.ClipID, clip.ClipID)
				}
			}
		})
	}
}

func TestClipAPI_UpdateClip(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)

	// Insert test clip
	clip := &sqlite.Clip{
		SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, Description: "Original",
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("failed to insert clip: %v", err)
	}

	newDesc := "Updated description"
	newRunID := "run-123"
	updateReq := UpdateClipRequest{
		Description:    &newDesc,
		ReferenceRunID: &newRunID,
	}

	bodyBytes, _ := json.Marshal(updateReq)
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/"+clip.ClipID, bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	ws.handleClipByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update
	retrieved, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("failed to get clip: %v", err)
	}
	if retrieved.Description != newDesc {
		t.Errorf("description = %s, want %s", retrieved.Description, newDesc)
	}
	if retrieved.ReferenceRunID != newRunID {
		t.Errorf("reference_run_id = %s, want %s", retrieved.ReferenceRunID, newRunID)
	}
}

func TestClipAPI_DeleteClip(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)

	// Insert test clip
	clip := &sqlite.Clip{
		SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("failed to insert clip: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/clips/"+clip.ClipID, nil)
	w := httptest.NewRecorder()

	ws.handleClipByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify deletion
	_, err := store.GetClip(clip.ClipID)
	if err == nil {
		t.Error("expected clip to be deleted")
	}
}

func TestClipAPI_ReplayClip(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)

	// Insert test clip
	clip := &sqlite.Clip{
		SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("failed to insert clip: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()

	ws.handleClipByID(w, req)

	// Should return 202 Accepted (creates run and starts PCAP replay)
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
		if resp["clip_id"] != clip.ClipID {
			t.Errorf("clip_id = %v, want %s", resp["clip_id"], clip.ClipID)
		}
	}
}

func TestClipAPI_MethodNotAllowed(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "PATCH on clips collection",
			method: http.MethodPatch,
			path:   "/api/lidar/clips",
		},
		{
			name:   "POST on individual clip",
			method: http.MethodPost,
			path:   "/api/lidar/clips/clip-123",
		},
		{
			name:   "DELETE on replay endpoint",
			method: http.MethodDelete,
			path:   "/api/lidar/clips/clip-123/replay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			if strings.HasSuffix(tt.path, "/clips") {
				ws.handleClips(w, req)
			} else {
				ws.handleClipByID(w, req)
			}

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestClipAPI_NoDatabaseConfigured(t *testing.T) {
	ws := &Server{
		db: nil, // No database configured
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips", nil)
	w := httptest.NewRecorder()

	ws.handleClips(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestParseClipPath(t *testing.T) {
	tests := []struct {
		path     string
		wantClip string
		wantAct  string
	}{
		{"/api/lidar/clips/clip-1", "clip-1", ""},
		{"/api/lidar/clips/clip-1/replay", "clip-1", "replay"},
		{"/api/lidar/clips/clip-1/evaluations", "clip-1", "evaluations"},
		{"/wrong/prefix", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			clipID, action := parseClipPath(tt.path)
			if clipID != tt.wantClip {
				t.Fatalf("clipID mismatch: got %q want %q", clipID, tt.wantClip)
			}
			if action != tt.wantAct {
				t.Fatalf("action mismatch: got %q want %q", action, tt.wantAct)
			}
		})
	}
}

func TestClipAPI_ListClipEvaluations(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-xyz/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "clip-xyz") {
		t.Fatalf("expected clip_id in response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "evaluations") {
		t.Fatalf("expected evaluations key in response, got %s", w.Body.String())
	}
}

func TestClipAPI_ReplayClip_ErrorBranches(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	t.Run("clip not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/nonexistent/replay", nil)
		w := httptest.NewRecorder()
		ws.handleClipByID(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("invalid json body", func(t *testing.T) {
		store := sqlite.NewClipStore(ws.db.DB)
		clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
		if err := store.InsertClip(clip); err != nil {
			t.Fatalf("failed to insert clip: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", strings.NewReader("{"))
		w := httptest.NewRecorder()
		ws.handleClipByID(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
	})
}

func TestClipAPI_HandleClipByID_UnknownAction(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1/unknown-action", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestClipCutoverRetiresTheOldRouteAndPathProjection(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	for _, path := range []string{"/api/lidar/scenes", "/api/lidar/scenes/old/replay"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("retired route %s: %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/lidar/clips", strings.NewReader(`{"sensor_id":"sensor","pcap_file":"old.pcap"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("retired path projection accepted: %d", response.Code)
	}
}
