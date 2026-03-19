package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// --- handleAutoTuneSuspend ---

func TestAutoTuneSuspend_NotConfigured(t *testing.T) {
	ws := &WebServer{}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto-tune/suspend", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspend(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestAutoTuneSuspend_Success(t *testing.T) {
	runner := &mockSweepHandlerRunner{}
	ws := &WebServer{autoTuneRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto-tune/suspend", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspend(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if runner.suspendCalls != 1 {
		t.Fatalf("expected 1 suspend call, got %d", runner.suspendCalls)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "suspended" {
		t.Fatalf("expected status=suspended, got %q", resp["status"])
	}
}

func TestAutoTuneSuspend_Error(t *testing.T) {
	runner := &mockSweepHandlerRunner{suspendErr: errors.New("cannot suspend: not running")}
	ws := &WebServer{autoTuneRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto-tune/suspend", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspend(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleAutoTuneSuspended ---

func setupSuspendedSweepStore(t *testing.T) (*sql.DB, *sqlite.SweepStore) {
	t.Helper()
	// Reuse the canonical schema from setupTestSweepStoreForHandlers to avoid
	// schema drift, then add the extra column needed for suspended sweeps.
	db, store := setupTestSweepStoreForHandlers(t)

	_, err := db.Exec(`ALTER TABLE lidar_sweeps ADD COLUMN checkpoint_round INTEGER DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		t.Fatalf("alter table: %v", err)
	}

	return db, store
}

func TestAutoTuneSuspended_NotConfigured(t *testing.T) {
	ws := &WebServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto-tune/suspended", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspended(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestAutoTuneSuspended_NotFound(t *testing.T) {
	db, store := setupSuspendedSweepStore(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto-tune/suspended", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspended(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["found"] != false {
		t.Fatalf("expected found=false, got %v", resp["found"])
	}
}

func TestAutoTuneSuspended_Found(t *testing.T) {
	db, store := setupSuspendedSweepStore(t)
	defer db.Close()

	// Insert a suspended sweep record.
	_, err := db.Exec(`INSERT INTO lidar_sweeps (sweep_id, sensor_id, mode, status, request, started_at, checkpoint_round)
		VALUES (?, ?, 'auto_tune', 'suspended', '{}', ?, 3)`,
		"sweep-susp-001", "sensor-a", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto-tune/suspended", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspended(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["found"] != true {
		t.Fatalf("expected found=true, got %v", resp["found"])
	}
	if resp["sweep_id"] != "sweep-susp-001" {
		t.Fatalf("expected sweep_id=sweep-susp-001, got %v", resp["sweep_id"])
	}
	if resp["sensor_id"] != "sensor-a" {
		t.Fatalf("expected sensor_id=sensor-a, got %v", resp["sensor_id"])
	}
	// checkpoint_round is JSON-decoded as float64.
	if resp["checkpoint_round"] != float64(3) {
		t.Fatalf("expected checkpoint_round=3, got %v", resp["checkpoint_round"])
	}
}

func TestAutoTuneSuspended_StoreError(t *testing.T) {
	// Use a store backed by a closed DB to trigger an error.
	db, store := setupSuspendedSweepStore(t)
	db.Close() // close it intentionally to trigger errors

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto-tune/suspended", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTuneSuspended(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleAutoTuneResume additional paths ---

func TestAutoTuneResume_ConflictError(t *testing.T) {
	runner := &mockSweepHandlerRunner{resumeErr: errors.New("already in progress")}
	ws := &WebServer{autoTuneRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto-tune/resume",
		strings.NewReader(`{"sweep_id":"s1"}`))
	w := httptest.NewRecorder()

	ws.handleAutoTuneResume(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAutoTuneResume_ValidationError(t *testing.T) {
	runner := &mockSweepHandlerRunner{resumeErr: errors.New("no checkpoint found")}
	ws := &WebServer{autoTuneRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto-tune/resume",
		strings.NewReader(`{"sweep_id":"bad"}`))
	w := httptest.NewRecorder()

	ws.handleAutoTuneResume(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleAutoTune method not allowed ---

func TestAutoTune_MethodNotAllowed(t *testing.T) {
	runner := &mockSweepHandlerRunner{}
	ws := &WebServer{autoTuneRunner: runner}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/sweep/auto", nil)
	w := httptest.NewRecorder()

	ws.handleAutoTune(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- handleHINT method not allowed ---

func TestHINT_MethodNotAllowed(t *testing.T) {
	runner := &mockHINTRunner{}
	ws := &WebServer{hintRunner: runner}

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/sweep/hint", nil)
	w := httptest.NewRecorder()

	ws.handleHINT(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// --- handleHINTStatus wait_for_change with client disconnect ---

func TestHINTStatus_WaitForChange_ClientDisconnect(t *testing.T) {
	runner := &mockHINTRunner{
		state: map[string]string{"status": "idle"},
	}
	// Override WaitForChange to block until context cancelled.
	blockingRunner := &blockingHINTRunner{mockHINTRunner: runner}
	ws := &WebServer{hintRunner: blockingRunner}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately to simulate client disconnect

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/hint/status?wait_for_change=idle", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	ws.handleHINTStatus(w, req)
	// When context is cancelled, the handler returns without writing a response body.
	// The response code may be 200 (default) but no JSON body should be encoded.
	// The key assertion is that it doesn't panic or hang.
}

// blockingHINTRunner wraps mockHINTRunner but blocks WaitForChange until ctx is done.
type blockingHINTRunner struct {
	*mockHINTRunner
}

func (b *blockingHINTRunner) WaitForChange(ctx context.Context, lastStatus string) interface{} {
	<-ctx.Done()
	return b.state
}

// --- handleHINTContinue error branches ---

func TestHINTContinue_GenericError(t *testing.T) {
	runner := &mockHINTRunner{continueErr: errors.New("internal failure")}
	ws := &WebServer{hintRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	ws.handleHINTContinue(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHINTContinue_MalformedJSON(t *testing.T) {
	runner := &mockHINTRunner{}
	ws := &WebServer{hintRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
		strings.NewReader(`{bad json`))
	w := httptest.NewRecorder()

	ws.handleHINTContinue(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleSweepCharts number primitive ---

func TestSweepCharts_NumberPrimitive(t *testing.T) {
	db, store := setupTestSweepStoreForHandlers(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
		strings.NewReader(`{"sweep_id":"test","charts":42}`))
	w := httptest.NewRecorder()

	ws.handleSweepCharts(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "must be a JSON array or object") {
		t.Fatalf("expected array/object error, got %q", w.Body.String())
	}
}

// --- handleGetSweep empty path ---

func TestGetSweep_EmptyPath(t *testing.T) {
	db, store := setupTestSweepStoreForHandlers(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps/", nil)
	w := httptest.NewRecorder()

	ws.handleGetSweep(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- handleListSweeps with sensor_id and limit ---

func TestListSweeps_WithSensorIDAndLimit(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store, sensorID: "sensor1"}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps?sensor_id=sensor1&limit=10", nil)
	w := httptest.NewRecorder()

	ws.handleListSweeps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListSweeps_DefaultSensorID(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store, sensorID: "sensor1"}
	// No sensor_id param — should use ws.sensorID as default
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps", nil)
	w := httptest.NewRecorder()

	ws.handleListSweeps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListSweeps_InvalidLimit(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store, sensorID: "sensor1"}
	// Invalid limit should use default of 20
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps?limit=abc", nil)
	w := httptest.NewRecorder()

	ws.handleListSweeps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
