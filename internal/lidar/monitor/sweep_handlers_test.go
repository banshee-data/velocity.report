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

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

type mockSweepHandlerRunner struct {
	startErr     error
	startCalls   int
	stopCalls    int
	suspendCalls int
	resumeCalls  int
	suspendErr   error
	resumeErr    error
	lastReq      interface{}
	state        interface{}
}

func (m *mockSweepHandlerRunner) Start(_ context.Context, req interface{}) error {
	m.startCalls++
	m.lastReq = req
	return m.startErr
}

func (m *mockSweepHandlerRunner) GetState() interface{} {
	return m.state
}

func (m *mockSweepHandlerRunner) Stop() {
	m.stopCalls++
}

func (m *mockSweepHandlerRunner) Suspend() error {
	m.suspendCalls++
	return m.suspendErr
}

func (m *mockSweepHandlerRunner) Resume(_ context.Context, _ string) error {
	m.resumeCalls++
	return m.resumeErr
}

func TestWebServer_SetSweepAndAutoTuneRunner(t *testing.T) {
	ws := &WebServer{}

	sweepRunner := &mockSweepHandlerRunner{}
	autoRunner := &mockSweepHandlerRunner{}

	ws.SetSweepRunner(sweepRunner)
	ws.SetAutoTuneRunner(autoRunner)

	if ws.sweepRunner != sweepRunner {
		t.Fatal("SetSweepRunner did not assign runner")
	}
	if ws.autoTuneRunner != autoRunner {
		t.Fatal("SetAutoTuneRunner did not assign runner")
	}
}

func TestSweepHandlers_SweepStart(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/start", nil)
		w := httptest.NewRecorder()

		ws.handleSweepStart(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("runner not configured", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/start", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		ws.handleSweepStart(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		ws := &WebServer{sweepRunner: &mockSweepHandlerRunner{}}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/start", strings.NewReader(`{`))
		w := httptest.NewRecorder()

		ws.handleSweepStart(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("already in progress conflict", func(t *testing.T) {
		runner := &mockSweepHandlerRunner{startErr: errors.New("already in progress")}
		ws := &WebServer{sweepRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/start", strings.NewReader(`{"mode":"multi"}`))
		w := httptest.NewRecorder()

		ws.handleSweepStart(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected %d got %d", http.StatusConflict, w.Code)
		}
	})

	t.Run("validation error bad request", func(t *testing.T) {
		runner := &mockSweepHandlerRunner{startErr: errors.New("invalid request")}
		ws := &WebServer{sweepRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/start", strings.NewReader(`{"mode":"multi"}`))
		w := httptest.NewRecorder()

		ws.handleSweepStart(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		runner := &mockSweepHandlerRunner{}
		ws := &WebServer{sweepRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/start", strings.NewReader(`{"mode":"multi"}`))
		w := httptest.NewRecorder()

		ws.handleSweepStart(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}
		if runner.startCalls != 1 {
			t.Fatalf("expected Start to be called once, got %d", runner.startCalls)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["status"] != "started" {
			t.Fatalf("expected status started, got %q", resp["status"])
		}
	})
}

func TestSweepHandlers_SweepStatusAndStop(t *testing.T) {
	t.Run("status method not allowed", func(t *testing.T) {
		ws := &WebServer{sweepRunner: &mockSweepHandlerRunner{}}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/status", nil)
		w := httptest.NewRecorder()
		ws.handleSweepStatus(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("status no runner", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/status", nil)
		w := httptest.NewRecorder()
		ws.handleSweepStatus(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("status success", func(t *testing.T) {
		ws := &WebServer{sweepRunner: &mockSweepHandlerRunner{state: map[string]interface{}{"status": "running"}}}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/status", nil)
		w := httptest.NewRecorder()
		ws.handleSweepStatus(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}
		if !strings.Contains(w.Body.String(), "running") {
			t.Fatalf("expected body to contain running, got %q", w.Body.String())
		}
	})

	t.Run("stop method not allowed", func(t *testing.T) {
		ws := &WebServer{sweepRunner: &mockSweepHandlerRunner{}}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/stop", nil)
		w := httptest.NewRecorder()
		ws.handleSweepStop(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("stop no runner", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/stop", nil)
		w := httptest.NewRecorder()
		ws.handleSweepStop(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("stop success", func(t *testing.T) {
		runner := &mockSweepHandlerRunner{}
		ws := &WebServer{sweepRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/stop", nil)
		w := httptest.NewRecorder()
		ws.handleSweepStop(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}
		if runner.stopCalls != 1 {
			t.Fatalf("expected Stop to be called once, got %d", runner.stopCalls)
		}
	})
}

func TestSweepHandlers_AutoTune(t *testing.T) {
	t.Run("dispatcher method not allowed", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweep/auto", nil)
		w := httptest.NewRecorder()
		ws.handleAutoTune(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("start no runner", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		ws.handleAutoTuneStart(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("start invalid json", func(t *testing.T) {
		ws := &WebServer{autoTuneRunner: &mockSweepHandlerRunner{}}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto", strings.NewReader(`{`))
		w := httptest.NewRecorder()
		ws.handleAutoTuneStart(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("start conflict", func(t *testing.T) {
		ws := &WebServer{autoTuneRunner: &mockSweepHandlerRunner{startErr: errors.New("already in progress")}}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto", strings.NewReader(`{"x":1}`))
		w := httptest.NewRecorder()
		ws.handleAutoTuneStart(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected %d got %d", http.StatusConflict, w.Code)
		}
	})

	t.Run("start bad request", func(t *testing.T) {
		ws := &WebServer{autoTuneRunner: &mockSweepHandlerRunner{startErr: errors.New("invalid")}}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto", strings.NewReader(`{"x":1}`))
		w := httptest.NewRecorder()
		ws.handleAutoTuneStart(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("start success", func(t *testing.T) {
		runner := &mockSweepHandlerRunner{}
		ws := &WebServer{autoTuneRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto", strings.NewReader(`{"x":1}`))
		w := httptest.NewRecorder()
		ws.handleAutoTune(w, req) // Exercise dispatcher POST path
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}
		if runner.startCalls != 1 {
			t.Fatalf("expected Start to be called once, got %d", runner.startCalls)
		}
	})

	t.Run("status no runner", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto", nil)
		w := httptest.NewRecorder()
		ws.handleAutoTuneStatus(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("status success", func(t *testing.T) {
		ws := &WebServer{autoTuneRunner: &mockSweepHandlerRunner{state: map[string]interface{}{"status": "running"}}}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto", nil)
		w := httptest.NewRecorder()
		ws.handleAutoTune(w, req) // Exercise dispatcher GET path
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}
		if !strings.Contains(w.Body.String(), "running") {
			t.Fatalf("expected body to contain running, got %q", w.Body.String())
		}
	})

	t.Run("stop method not allowed", func(t *testing.T) {
		ws := &WebServer{autoTuneRunner: &mockSweepHandlerRunner{}}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/auto/stop", nil)
		w := httptest.NewRecorder()
		ws.handleAutoTuneStop(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("stop no runner", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto/stop", nil)
		w := httptest.NewRecorder()
		ws.handleAutoTuneStop(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("stop success", func(t *testing.T) {
		runner := &mockSweepHandlerRunner{}
		ws := &WebServer{autoTuneRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/auto/stop", nil)
		w := httptest.NewRecorder()
		ws.handleAutoTuneStop(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}
		if runner.stopCalls != 1 {
			t.Fatalf("expected Stop to be called once, got %d", runner.stopCalls)
		}
	})
}

// setupTestSweepStoreForHandlers creates an in-memory database and SweepStore for handler testing
func setupTestSweepStoreForHandlers(t *testing.T) (*sql.DB, *sqlite.SweepStore) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS lidar_sweeps (
id INTEGER PRIMARY KEY AUTOINCREMENT,
sweep_id TEXT NOT NULL UNIQUE,
sensor_id TEXT NOT NULL,
mode TEXT NOT NULL DEFAULT 'sweep',
status TEXT NOT NULL DEFAULT 'running',
request TEXT NOT NULL,
results TEXT,
charts TEXT,
recommendation TEXT,
round_results TEXT,
error TEXT,
started_at DATETIME NOT NULL,
completed_at DATETIME,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
`)
	if err != nil {
		t.Fatalf("failed to create lidar_sweeps table: %v", err)
	}

	return db, sqlite.NewSweepStore(db)
}

func TestSweepHandlers_ListSweeps(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweeps", nil)
		w := httptest.NewRecorder()

		ws.handleListSweeps(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("store not configured", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps", nil)
		w := httptest.NewRecorder()

		ws.handleListSweeps(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})
}

func TestSweepHandlers_GetSweep(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweeps/test-id", nil)
		w := httptest.NewRecorder()

		ws.handleGetSweep(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("store not configured", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps/test-id", nil)
		w := httptest.NewRecorder()

		ws.handleGetSweep(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("missing sweep_id", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps/", nil)
		w := httptest.NewRecorder()

		ws.handleGetSweep(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestSweepHandlers_SweepCharts(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps/charts", nil)
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("store not configured", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts", strings.NewReader(`{`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("missing sweep_id", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts", strings.NewReader(`{"charts":[]}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
		if !strings.Contains(w.Body.String(), "sweep_id is required") {
			t.Fatalf("expected sweep_id error, got %q", w.Body.String())
		}
	})

	t.Run("double encoded JSON string rejected", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		// Send charts as a JSON-encoded string instead of array
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":"[{\"id\":\"chart1\"}]"}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
		if !strings.Contains(w.Body.String(), "must be a JSON array or object") {
			t.Fatalf("expected double-encoded error, got %q", w.Body.String())
		}
	})

	t.Run("invalid JSON in charts", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		// Send invalid JSON in charts field
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":{invalid}}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("null primitive rejected", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":null}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
		if !strings.Contains(w.Body.String(), "must be a JSON array or object") {
			t.Fatalf("expected array/object error, got %q", w.Body.String())
		}
	})

	t.Run("boolean primitive rejected", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":true}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
		if !strings.Contains(w.Body.String(), "must be a JSON array or object") {
			t.Fatalf("expected array/object error, got %q", w.Body.String())
		}
	})

	t.Run("number primitive rejected", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":123}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d", http.StatusBadRequest, w.Code)
		}
		if !strings.Contains(w.Body.String(), "must be a JSON array or object") {
			t.Fatalf("expected array/object error, got %q", w.Body.String())
		}
	})

	t.Run("success with array", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":[{"id":"chart1","type":"line"}]}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["status"] != "saved" {
			t.Fatalf("expected status saved, got %q", resp["status"])
		}
	})

	t.Run("success with object", func(t *testing.T) {
		db, store := setupTestSweepStoreForHandlers(t)
		defer db.Close()
		ws := &WebServer{sweepStore: store}
		req := httptest.NewRequest(http.MethodPut, "/api/lidar/sweeps/charts",
			strings.NewReader(`{"sweep_id":"test","charts":{"config":"value"}}`))
		w := httptest.NewRecorder()

		ws.handleSweepCharts(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}
	})
}

// --- HINT Handler Tests ---

type mockHINTRunner struct {
	startErr      error
	startCalls    int
	stopCalls     int
	lastReq       interface{}
	state         interface{}
	continueErr   error
	continueCalls int
	lastDuration  int
	lastAddRound  bool
}

func (m *mockHINTRunner) Start(_ context.Context, req interface{}) error {
	m.startCalls++
	m.lastReq = req
	return m.startErr
}

func (m *mockHINTRunner) GetState() interface{} {
	return m.state
}

func (m *mockHINTRunner) WaitForChange(_ context.Context, _ string) interface{} {
	return m.state
}

func (m *mockHINTRunner) Stop() {
	m.stopCalls++
}

func (m *mockHINTRunner) ContinueFromLabels(nextDurationMins int, addRound bool) error {
	m.continueCalls++
	m.lastDuration = nextDurationMins
	m.lastAddRound = addRound
	return m.continueErr
}

func (m *mockHINTRunner) NotifyLabelUpdate() {}

func TestWebServer_SetHINTRunner(t *testing.T) {
	ws := &WebServer{}
	runner := &mockHINTRunner{}
	ws.SetHINTRunner(runner)
	if ws.hintRunner != runner {
		t.Fatal("SetHINTRunner did not assign runner")
	}
}

func TestHINTHandlers_Start(t *testing.T) {
	t.Run("method not allowed on DELETE", func(t *testing.T) {
		runner := &mockHINTRunner{}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodDelete, "/api/lidar/sweep/hint", nil)
		w := httptest.NewRecorder()

		ws.handleHINT(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("not configured returns 503", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint",
			strings.NewReader(`{"scene_id":"s1"}`))
		w := httptest.NewRecorder()

		ws.handleHINT(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		runner := &mockHINTRunner{}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint",
			strings.NewReader(`not json`))
		w := httptest.NewRecorder()

		ws.handleHINT(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("already running returns 409", func(t *testing.T) {
		runner := &mockHINTRunner{startErr: errors.New("sweep already in progress")}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint",
			strings.NewReader(`{"scene_id":"s1"}`))
		w := httptest.NewRecorder()

		ws.handleHINT(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected %d got %d body=%s", http.StatusConflict, w.Code, w.Body.String())
		}
	})

	t.Run("success returns started", func(t *testing.T) {
		runner := &mockHINTRunner{}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint",
			strings.NewReader(`{"scene_id":"s1","num_rounds":2}`))
		w := httptest.NewRecorder()

		ws.handleHINT(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}
		if runner.startCalls != 1 {
			t.Fatalf("expected Start called once, got %d", runner.startCalls)
		}
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "started" {
			t.Fatalf("expected status started, got %q", resp["status"])
		}
	})
}

func TestHINTHandlers_Status(t *testing.T) {
	t.Run("returns current state", func(t *testing.T) {
		runner := &mockHINTRunner{
			state: map[string]interface{}{"status": "awaiting_labels", "mode": "hint"},
		}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/hint", nil)
		w := httptest.NewRecorder()

		ws.handleHINT(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "awaiting_labels" {
			t.Fatalf("expected status awaiting_labels, got %v", resp["status"])
		}
	})
}

func TestHINTHandlers_Continue(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		ws := &WebServer{hintRunner: &mockHINTRunner{}}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/hint/continue", nil)
		w := httptest.NewRecorder()

		ws.handleHINTContinue(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("not configured returns 503", func(t *testing.T) {
		ws := &WebServer{}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
			strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		ws.handleHINTContinue(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %d got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("threshold not met returns 400", func(t *testing.T) {
		runner := &mockHINTRunner{continueErr: errors.New("label threshold not met: 30.0% < 90.0%")}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
			strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		ws.handleHINTContinue(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("not awaiting labels returns 409", func(t *testing.T) {
		runner := &mockHINTRunner{continueErr: errors.New("not in awaiting_labels state")}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
			strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		ws.handleHINTContinue(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected %d got %d body=%s", http.StatusConflict, w.Code, w.Body.String())
		}
	})

	t.Run("success with duration and add_round", func(t *testing.T) {
		runner := &mockHINTRunner{}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
			strings.NewReader(`{"next_sweep_duration_mins":120,"add_round":true}`))
		w := httptest.NewRecorder()

		ws.handleHINTContinue(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}
		if runner.continueCalls != 1 {
			t.Fatalf("expected Continue called once, got %d", runner.continueCalls)
		}
		if runner.lastDuration != 120 {
			t.Fatalf("expected duration 120, got %d", runner.lastDuration)
		}
		if !runner.lastAddRound {
			t.Fatal("expected addRound true")
		}
	})

	t.Run("empty body uses defaults", func(t *testing.T) {
		runner := &mockHINTRunner{}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/continue",
			strings.NewReader(``))
		w := httptest.NewRecorder()

		ws.handleHINTContinue(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
		}
		if runner.lastDuration != 0 {
			t.Fatalf("expected duration 0, got %d", runner.lastDuration)
		}
		if runner.lastAddRound {
			t.Fatal("expected addRound false")
		}
	})
}

func TestHINTHandlers_Stop(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		ws := &WebServer{hintRunner: &mockHINTRunner{}}
		req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/hint/stop", nil)
		w := httptest.NewRecorder()

		ws.handleHINTStop(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		runner := &mockHINTRunner{}
		ws := &WebServer{hintRunner: runner}
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/sweep/hint/stop", nil)
		w := httptest.NewRecorder()

		ws.handleHINTStop(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %d got %d", http.StatusOK, w.Code)
		}
		if runner.stopCalls != 1 {
			t.Fatalf("expected Stop called once, got %d", runner.stopCalls)
		}
	})
}
