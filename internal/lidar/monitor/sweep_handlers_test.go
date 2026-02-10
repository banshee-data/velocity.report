package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockSweepHandlerRunner struct {
	startErr   error
	startCalls int
	stopCalls  int
	lastReq    interface{}
	state      interface{}
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
