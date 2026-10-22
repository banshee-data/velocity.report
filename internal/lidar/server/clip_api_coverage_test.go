package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
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
	_ "modernc.org/sqlite"
)

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (errReadCloser) Close() error             { return nil }

// --- handleUpdateClip additional coverage ---

func TestCov_HandleUpdateClip_InvalidJSON(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/"+clip.ClipID, strings.NewReader("not json"))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleUpdateClip_NotFound(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	desc := "updated"
	body, _ := json.Marshal(UpdateClipRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/nonexistent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleUpdateClip_AllFields(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	desc := "new description"
	refRun := "run-ref"
	startSecs := 1.5
	durSecs := 10.0
	optParams := json.RawMessage(`{"key":"value"}`)

	body, _ := json.Marshal(UpdateClipRequest{
		Description:       &desc,
		ReferenceRunID:    &refRun,
		PCAPStartSecs:     &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: &optParams,
	})

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/"+clip.ClipID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify all fields were updated
	updated, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("get clip: %v", err)
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

// --- handleDeleteClip additional coverage ---

func TestCov_HandleDeleteClip_NotFound(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/clips/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- handleReplayClip with optimal params ---

func TestCov_HandleReplayClip_WithOptimalParams(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	startSecs := 0.5
	durSecs := 5.0
	clip := &sqlite.Clip{
		SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, PCAPStartSecs: &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"threshold": 0.5}`),
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// May fail with PCAP error but should get past the params selection logic
	if w.Code == http.StatusBadRequest || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("unexpected validation error: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCov_HandleReplayClip_WithRequestParams(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{
		SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"params_json": map[string]interface{}{"threshold": 0.8},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// Should get past validation; PCAP replay itself may fail
	if w.Code == http.StatusBadRequest || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("unexpected validation error: status=%d body=%s", w.Code, w.Body.String())
	}
}

// --- handleClipByID missing clip_id ---

func TestCov_HandleClipByID_NilDB(t *testing.T) {
	ws := &Server{db: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestCov_HandleClipByID_MissingClipID(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- handleClips NilDB for POST ---

func TestCov_HandleClips_NilDB_POST(t *testing.T) {
	ws := &Server{db: nil}

	body, _ := json.Marshal(CreateClipRequest{SensorID: "s1", PCAPFiles: []string{"p.pcap"}})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClips(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- handleListClipEvaluations wrong method ---

func TestCov_HandleListClipEvaluations_WrongMethod(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	// PUT is not supported for evaluations (GET and POST are)
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/clip-1/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleReplayClip evaluations that dispatch via handleClipByID method filtering ---

func TestCov_HandleClipByID_ReplayWrongMethod(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleClipByID POST on clip (not replay) ---
func TestCov_HandleClipByID_PostOnClip(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/clip-1", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- writeJSON helper ---

func TestCov_WriteJSON(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()
	ws := &Server{db: &db.DB{DB: testDB}}

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
	ws := &Server{}

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

func TestCov_HandleListClips_DBError(t *testing.T) {
	ws := setupTestClipServer(t)
	ws.db.DB.Close() // force DB error

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips", nil)
	w := httptest.NewRecorder()
	ws.handleClips(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCov_HandleGetClip_GenericDBError(t *testing.T) {
	ws := setupTestClipServer(t)
	ws.db.DB.Close() // force generic DB error (not "not found")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleUpdateClip_GenericDBError(t *testing.T) {
	ws := setupTestClipServer(t)
	ws.db.DB.Close() // force generic DB error in GetClip

	desc := "updated"
	body, _ := json.Marshal(UpdateClipRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/clip-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleUpdateClip_StoreError(t *testing.T) {
	ws := setupTestClipServer(t)

	// Insert a clip, then drop the table so UpdateClip fails
	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Rename/drop the table to break update but allow read
	// Actually - just close after creating the clip won't work for GetClip.
	// Let's corrupt: drop a column that UpdateClip needs
	_, _ = ws.db.DB.Exec("DROP TABLE lidar_clips")
	_, _ = ws.db.DB.Exec(`CREATE TABLE lidar_clips (clip_id TEXT PRIMARY KEY)`) // minimal table

	desc := "updated"
	body, _ := json.Marshal(UpdateClipRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/"+clip.ClipID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// GetClip will fail since columns are missing
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want error status; body: %s", w.Code, w.Body.String())
	}
}

func TestCov_HandleDeleteClip_GenericDBError(t *testing.T) {
	ws := setupTestClipServer(t)
	ws.db.DB.Close() // force generic DB error (not "not found")

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/clips/clip-1", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleCreateClip_DBError(t *testing.T) {
	ws := setupTestClipServer(t)
	ws.db.DB.Close() // force DB error on InsertClip

	body, _ := json.Marshal(CreateClipRequest{SensorID: "s1", PCAPFiles: []string{"p.pcap"}})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClips(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleReplayClip_InsertRunDBError(t *testing.T) {
	ws := setupTestClipServer(t)

	// Insert clip first
	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Drop runs table to force InsertRun failure
	_, _ = ws.db.DB.Exec("DROP TABLE lidar_runs")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleReplayClip_GenericDBError(t *testing.T) {
	ws := setupTestClipServer(t)

	// Insert clip, then close DB so GetClip fails with generic error
	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}
	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestCov_HandleListClips_EmptyReturnsArray(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	// No clips inserted — should return empty array, not null
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips", nil)
	w := httptest.NewRecorder()
	ws.handleClips(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	clips, ok := resp["clips"].([]interface{})
	if !ok {
		t.Fatal("expected clips to be an array")
	}
	if len(clips) != 0 {
		t.Errorf("expected 0 clips, got %d", len(clips))
	}
}

func TestCov_HandleListClipEvaluations_GET(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "clip-1") {
		t.Errorf("expected clip_id in response, got: %s", w.Body.String())
	}
}

func TestCov_HandleClipByID_EvaluationsPostDispatch(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/nonexistent/evaluations", strings.NewReader(`{"candidate_run_id":"cand-1"}`))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleUpdateClip_ReadBodyError(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-read-err", Files: []sqlite.ClipCapture{{PCAPFile: "read-error.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/"+clip.ClipID, nil)
	req.Body = errReadCloser{}
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "failed to read body") {
		t.Fatalf("expected read body error, got %s", w.Body.String())
	}
}

func TestCov_HandleUpdateClip_UpdateError(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-update-err", Files: []sqlite.ClipCapture{{PCAPFile: "update-error.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := ws.db.DB.Exec(`
		CREATE TRIGGER fail_clip_update BEFORE UPDATE ON lidar_clips
		BEGIN
			SELECT RAISE(FAIL, 'clip update blocked');
		END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	desc := "should fail"
	body, _ := json.Marshal(UpdateClipRequest{Description: &desc})
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/clips/"+clip.ClipID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// setupTestClipAPIDBFull creates a test DB with the full lidar_runs
// schema so that InsertRun succeeds (the original helper only has 2 columns).
func setupTestClipAPIDBFull(t *testing.T) *db.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Full lidar_runs table (matching InsertRun query)
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_runs (
			run_id TEXT PRIMARY KEY,
			created_at INTEGER,
			source_type TEXT,
			source_path TEXT,
			sensor_id TEXT,
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
		t.Fatalf("failed to create lidar_runs table: %v", err)
	}

	// lidar_param_sets table (needed by normalizeRecommendedParamSet)
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

	// lidar_clips table
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

	return &db.DB{DB: sqlDB}
}

// --- handleReplayClip: invalid JSON body ---

func TestClipCov2_ReplayInvalidJSON(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &Server{db: testDB}

	// Insert a clip so GetClip succeeds
	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Send malformed JSON body — should trigger err != io.EOF branch
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay",
		strings.NewReader("{bad json"))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in body, got: %s", w.Body.String())
	}
}

// --- handleReplayClip: pcap in progress, with tracker + background manager ---

func TestClipCov2_ReplayPCAPInProgress(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	// Create temp dir with dummy .pcap file for resolvePCAPPath
	tmpDir := t.TempDir()
	pcapName := "test.pcap"
	if err := os.WriteFile(filepath.Join(tmpDir, pcapName), []byte("dummy pcap"), 0644); err != nil {
		t.Fatalf("write pcap: %v", err)
	}

	sensorID := "sensor-cov2-replay"

	ws := &Server{
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
	ws.mutateState("test", func(s *PipelineState) { s.Source = SourceModeCapture; s.ReplayActive = true })
	ws.pcapMu.Unlock()

	// Insert clip
	store := sqlite.NewClipStore(testDB.DB)
	startSecs := 1.0
	durSecs := 5.0
	clip := &sqlite.Clip{
		SensorID: sensorID, Files: []sqlite.ClipCapture{{PCAPFile: pcapName}}, PCAPStartSecs: &startSecs,
		PCAPDurationSecs: &durSecs,
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// StartPCAPInternal fails because pcap already in progress
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PCAP replay") {
		t.Errorf("expected PCAP replay error, got: %s", w.Body.String())
	}
}

// --- handleReplayClip: broken BackgroundManager (nil grid) logs warning ---

func TestClipCov2_ReplayBrokenBGManager(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	sensorID := "sensor-cov2-brokenbgm"

	ws := &Server{
		db:       testDB,
		sensorID: sensorID,
	}

	// Register a BackgroundManager with nil Grid — ResetGrid returns error
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})

	// Insert clip
	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: sensorID, Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// Should still get past resetBackgroundGrid (error is just logged)
	// Then fail at StartPCAPInternal because pcapSafeDir is empty
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayClip: neither request params nor optimal params ---

func TestClipCov2_ReplayNoParams(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &Server{db: testDB, sensorID: "sensor-cov2-noparams"}

	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{
		SensorID: "sensor-cov2-noparams", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}},
		// No OptimalParamsJSON, no PCAPStartSecs, no PCAPDurationSecs
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Empty body — no params_json
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// InsertRun should succeed (full table), then StartPCAPInternal fails
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayClip: request params override clip optimal params ---

func TestClipCov2_ReplayRequestParamsOverride(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &Server{db: testDB, sensorID: "sensor-cov2-override"}

	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{
		SensorID: "sensor-cov2-override", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, OptimalParamsJSON: json.RawMessage(`{"threshold": 0.5}`),
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Send request params that should override clip's optimal params
	body, _ := json.Marshal(map[string]interface{}{
		"params_json": map[string]interface{}{"threshold": 0.9},
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// InsertRun succeeds, then StartPCAPInternal fails
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayClip: UpdateRunStatus also fails ---

func TestClipCov2_ReplayUpdateStatusError(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &Server{db: testDB, sensorID: "sensor-cov2-updateerr"}

	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-cov2-updateerr", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Add trigger to block UPDATE on lidar_runs, so UpdateRunStatus fails
	// while InsertRun still succeeds.
	_, err := testDB.DB.Exec(`
		CREATE TRIGGER prevent_update_runs BEFORE UPDATE ON lidar_runs
		BEGIN
			SELECT RAISE(FAIL, 'update blocked by trigger');
		END
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// InsertRun succeeds → StartPCAPInternal fails → UpdateRunStatus fails (trigger)
	// → handler still returns 500 from the StartPCAPInternal error
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

// --- handleReplayClip: clip with OptimalParamsJSON, full DB ---

func TestClipCov2_ReplayWithOptimalParamsFullDB(t *testing.T) {
	testDB := setupTestClipAPIDBFull(t)
	defer testDB.DB.Close()

	ws := &Server{db: testDB, sensorID: "sensor-cov2-optparams"}

	store := sqlite.NewClipStore(testDB.DB)
	startSecs := 0.5
	durSecs := 10.0
	clip := &sqlite.Clip{
		SensorID: "sensor-cov2-optparams", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, PCAPStartSecs: &startSecs,
		PCAPDurationSecs:  &durSecs,
		OptimalParamsJSON: json.RawMessage(`{"threshold": 0.5}`),
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	// InsertRun succeeds with full DB, then StartPCAPInternal fails (no pcapSafeDir)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PCAP replay") {
		t.Errorf("expected PCAP replay error, got: %s", w.Body.String())
	}
}

// --- parseClipPath edge cases ---

func TestClipCov2_ParseClipPathNoPrefix(t *testing.T) {
	clipID, action := parseClipPath("/some/other/path")
	if clipID != "" || action != "" {
		t.Errorf("expected empty results for non-matching path, got clipID=%q action=%q", clipID, action)
	}
}

func TestClipCov2_ParseClipPathWithAction(t *testing.T) {
	clipID, action := parseClipPath("/api/lidar/clips/abc-123/replay")
	if clipID != "abc-123" {
		t.Errorf("clipID = %q, want %q", clipID, "abc-123")
	}
	if action != "replay" {
		t.Errorf("action = %q, want %q", action, "replay")
	}
}

func TestClipCov2_ParseClipPathClipOnly(t *testing.T) {
	clipID, action := parseClipPath("/api/lidar/clips/abc-123")
	if clipID != "abc-123" {
		t.Errorf("clipID = %q, want %q", clipID, "abc-123")
	}
	if action != "" {
		t.Errorf("action = %q, want empty", action)
	}
}

// --- handleClipByID: unknown action ---

func TestClipCov2_UnknownAction(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1/bogus", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "endpoint not found") {
		t.Errorf("expected 'endpoint not found', got: %s", w.Body.String())
	}
}

// --- handleClips: unsupported method ---

func TestClipCov2_HandleClipsMethodNotAllowed(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodPatch, "/api/lidar/clips", nil)
	w := httptest.NewRecorder()
	ws.handleClips(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// --- handleCreateClipEvaluation coverage ---

// setupTestClipAPIDBWithEvaluations creates a full test DB with all tables needed
// for evaluation testing (clips, analysis runs, run tracks, evaluations).
func setupTestClipAPIDBWithEvaluations(t *testing.T) *db.DB {
	t.Helper()
	testDB := setupTestClipAPIDBFull(t)

	// Add lidar_clip_evaluations table
	_, err := testDB.DB.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_clip_evaluations (
			evaluation_id       TEXT PRIMARY KEY,
			clip_id            TEXT NOT NULL,
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
			created_at          INTEGER NOT NULL,
			FOREIGN KEY (clip_id) REFERENCES lidar_clips(clip_id) ON DELETE CASCADE,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_runs(run_id),
			FOREIGN KEY (candidate_run_id) REFERENCES lidar_runs(run_id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_pair ON lidar_clip_evaluations(reference_run_id, candidate_run_id);
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_clip_evaluations table: %v", err)
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
			max_speed_mps REAL DEFAULT 0,
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
			FOREIGN KEY (run_id) REFERENCES lidar_runs(run_id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_run_tracks table: %v", err)
	}

	return testDB
}

func TestCov_HandleCreateClipEvaluation_ClipNotFound(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	body := `{"candidate_run_id": "cand-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/nonexistent/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCov_HandleCreateClipEvaluation_NoReferenceRun(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	// Insert clip without reference_run_id
	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	body := `{"candidate_run_id": "cand-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, clip.ClipID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no reference run") {
		t.Errorf("expected 'no reference run' in error, got: %s", w.Body.String())
	}
}

func TestCov_HandleCreateClipEvaluation_InvalidJSON(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	// Insert run and clip with reference
	runStore := sqlite.NewRunStore(testDB.DB)
	refRun := &sqlite.Run{RunID: "ref-run-eval", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, ReferenceRunID: "ref-run-eval"}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/evaluations", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, clip.ClipID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleCreateClipEvaluation_MissingCandidateID(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	// Insert run and clip with reference
	runStore := sqlite.NewRunStore(testDB.DB)
	refRun := &sqlite.Run{RunID: "ref-run-eval2", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, ReferenceRunID: "ref-run-eval2"}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, clip.ClipID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "candidate_run_id is required") {
		t.Errorf("expected 'candidate_run_id is required', got: %s", w.Body.String())
	}
}

func TestCov_HandleCreateClipEvaluation_Success(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	// Insert reference and candidate runs with tracks
	runStore := sqlite.NewRunStore(testDB.DB)
	refRun := &sqlite.Run{RunID: "ref-run-success", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	candRun := &sqlite.Run{RunID: "cand-run-success", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(candRun); err != nil {
		t.Fatalf("insert cand run: %v", err)
	}

	// Insert tracks for both runs so evaluation can work
	for _, rt := range []struct{ runID, trackID string }{
		{"ref-run-success", "ref-track-1"},
		{"cand-run-success", "cand-track-1"},
	} {
		track := &sqlite.RunTrack{
			RunID:   rt.runID,
			TrackID: rt.trackID, TrackMeasurement: l5tracks.TrackMeasurement{SensorID: "sensor-001",
				TrackState:       "confirmed",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     2000000000,
				ObservationCount: 10,
				AvgSpeedMps:      5.0},
		}
		if err := runStore.InsertRunTrack(track); err != nil {
			t.Fatalf("insert track %s: %v", rt.trackID, err)
		}
	}

	// Create clip with reference run
	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, ReferenceRunID: "ref-run-success"}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	body := `{"candidate_run_id": "cand-run-success"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/evaluations", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, clip.ClipID)

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
	evals, err := evalStore.ListByClip(clip.ClipID)
	if err != nil {
		t.Fatalf("ListByClip: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(evals))
	}
}

func TestCov_HandleListClipEvaluations_WithData(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	// Insert clip and a pre-existing evaluation
	_, err := testDB.DB.Exec(`INSERT INTO lidar_clips (clip_id, sensor_id, created_at_ns) VALUES ('clip-eval-list', 'sensor-1', 1000)`)
	if err != nil {
		t.Fatalf("insert clip: %v", err)
	}
	_, err = testDB.DB.Exec(`INSERT INTO lidar_runs (run_id, created_at, source_type, sensor_id, status) VALUES ('ref-list', 0, 'pcap', 'sensor-1', 'completed')`)
	if err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	_, err = testDB.DB.Exec(`INSERT INTO lidar_runs (run_id, created_at, source_type, sensor_id, status) VALUES ('cand-list', 0, 'pcap', 'sensor-1', 'completed')`)
	if err != nil {
		t.Fatalf("insert cand run: %v", err)
	}

	evalStore := sqlite.NewEvaluationStore(testDB.DB)
	eval := &sqlite.Evaluation{
		ClipID:         "clip-eval-list",
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

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-eval-list/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleListClipEvaluations(w, req, "clip-eval-list")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "0.85") {
		t.Errorf("expected composite_score 0.85 in response, got: %s", w.Body.String())
	}
}

func TestCov_HandleListClipEvaluations_DBError(t *testing.T) {
	ws := setupTestClipServer(t)
	ws.db.DB.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clips/clip-1/evaluations", nil)
	w := httptest.NewRecorder()
	ws.handleListClipEvaluations(w, req, "clip-1")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCov_HandleCreateClipEvaluation_LoadClipDBError(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	ws := &Server{db: testDB}
	testDB.DB.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/clip-db-error/evaluations", strings.NewReader(`{"candidate_run_id":"cand-1"}`))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, "clip-db-error")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCov_HandleCreateClipEvaluation_EvaluateError(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	runStore := sqlite.NewRunStore(testDB.DB)
	refRun := &sqlite.Run{RunID: "ref-run-eval-error", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	candRun := &sqlite.Run{RunID: "cand-run-eval-error", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(candRun); err != nil {
		t.Fatalf("insert candidate run: %v", err)
	}

	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, ReferenceRunID: refRun.RunID}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	if _, err := testDB.DB.Exec(`DROP TABLE lidar_run_tracks`); err != nil {
		t.Fatalf("drop lidar_run_tracks: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/evaluations", strings.NewReader(`{"candidate_run_id":"cand-run-eval-error"}`))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, clip.ClipID)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "evaluation failed") {
		t.Fatalf("expected evaluation error, got %s", w.Body.String())
	}
}

func TestCov_HandleCreateClipEvaluation_InsertError(t *testing.T) {
	testDB := setupTestClipAPIDBWithEvaluations(t)
	defer testDB.DB.Close()
	ws := &Server{db: testDB}

	runStore := sqlite.NewRunStore(testDB.DB)
	refRun := &sqlite.Run{RunID: "ref-run-insert-error", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(refRun); err != nil {
		t.Fatalf("insert ref run: %v", err)
	}
	candRun := &sqlite.Run{RunID: "cand-run-insert-error", SourceType: "pcap", SensorID: "sensor-001", Status: "completed"}
	if err := runStore.InsertRun(candRun); err != nil {
		t.Fatalf("insert candidate run: %v", err)
	}

	for _, rt := range []struct{ runID, trackID string }{
		{refRun.RunID, "ref-track-insert-error"},
		{candRun.RunID, "cand-track-insert-error"},
	} {
		track := &sqlite.RunTrack{
			RunID:   rt.runID,
			TrackID: rt.trackID,
			TrackMeasurement: l5tracks.TrackMeasurement{
				SensorID:         "sensor-001",
				TrackState:       "confirmed",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     2000000000,
				ObservationCount: 5,
				AvgSpeedMps:      3.0,
			},
		}
		if err := runStore.InsertRunTrack(track); err != nil {
			t.Fatalf("insert track %s: %v", rt.trackID, err)
		}
	}

	store := sqlite.NewClipStore(testDB.DB)
	clip := &sqlite.Clip{SensorID: "sensor-001", Files: []sqlite.ClipCapture{{PCAPFile: "test.pcap"}}, ReferenceRunID: refRun.RunID}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	evalStore := sqlite.NewEvaluationStore(testDB.DB)
	eval := &sqlite.Evaluation{
		ClipID:         clip.ClipID,
		ReferenceRunID: refRun.RunID,
		CandidateRunID: candRun.RunID,
	}
	if err := evalStore.Insert(eval); err != nil {
		t.Fatalf("seed evaluation: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/evaluations", strings.NewReader(`{"candidate_run_id":"cand-run-insert-error"}`))
	w := httptest.NewRecorder()
	ws.handleCreateClipEvaluation(w, req, clip.ClipID)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to persist evaluation") {
		t.Fatalf("expected persist error, got %s", w.Body.String())
	}
}

func TestCov_HandleReplayClip_FallsBackToPreferredRunID(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()

	store := sqlite.NewClipStore(ws.db.DB)
	clip := &sqlite.Clip{SensorID: "sensor-clip-fallback", Files: []sqlite.ClipCapture{{PCAPFile: "fallback.pcap"}}}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	origStart := startPCAPInternalForClip
	origLast := lastRunIDForClip
	startPCAPInternalForClip = func(_ *Server, _ string, _ ReplayConfig) error { return nil }
	lastRunIDForClip = func(_ *Server) string { return "" }
	defer func() {
		startPCAPInternalForClip = origStart
		lastRunIDForClip = origLast
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", nil)
	w := httptest.NewRecorder()
	ws.handleClipByID(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"clip_id":"`+clip.ClipID+`"`) {
		t.Fatalf("expected clip id in response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"run_id":"replay-`) {
		t.Fatalf("expected preferred run id fallback, got %s", w.Body.String())
	}
}
