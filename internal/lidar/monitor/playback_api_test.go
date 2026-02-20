package monitor

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandlePlaybackStatus tests the GET /api/lidar/playback/status endpoint.
func TestHandlePlaybackStatus(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		getStatus      func() *PlaybackStatusInfo
		expectedStatus int
		checkResponse  func(t *testing.T, resp map[string]interface{})
	}{
		{
			name:           "GET without callback returns default live status",
			method:         http.MethodGet,
			getStatus:      nil,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				if resp["mode"] != "live" {
					t.Errorf("expected mode=live, got %v", resp["mode"])
				}
				if resp["seekable"].(bool) {
					t.Error("expected seekable=false")
				}
			},
		},
		{
			name:   "GET with callback returns callback status",
			method: http.MethodGet,
			getStatus: func() *PlaybackStatusInfo {
				return &PlaybackStatusInfo{
					Mode:         "vrlog",
					Paused:       true,
					Rate:         1.5,
					Seekable:     true,
					CurrentFrame: 100,
					TotalFrames:  500,
				}
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				if resp["mode"] != "vrlog" {
					t.Errorf("expected mode=vrlog, got %v", resp["mode"])
				}
				if !resp["paused"].(bool) {
					t.Error("expected paused=true")
				}
				if !resp["seekable"].(bool) {
					t.Error("expected seekable=true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				getPlaybackStatus: tt.getStatus,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/playback/status", nil)
			w := httptest.NewRecorder()

			ws.handlePlaybackStatus(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.checkResponse != nil && w.Code == http.StatusOK {
				var resp map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				tt.checkResponse(t, resp)
			}
		})
	}
}

// TestHandlePlaybackPause tests the POST /api/lidar/playback/pause endpoint.
func TestHandlePlaybackPause(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		onPause        func()
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			onPause:        nil,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with callback succeeds",
			method:         http.MethodPost,
			onPause:        func() {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				onPlaybackPause: tt.onPause,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/playback/pause", nil)
			w := httptest.NewRecorder()

			ws.handlePlaybackPause(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandlePlaybackPlay tests the POST /api/lidar/playback/play endpoint.
func TestHandlePlaybackPlay(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		onPlay         func()
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			onPlay:         nil,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with callback succeeds",
			method:         http.MethodPost,
			onPlay:         func() {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				onPlaybackPlay: tt.onPlay,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/playback/play", nil)
			w := httptest.NewRecorder()

			ws.handlePlaybackPlay(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandlePlaybackSeek tests the POST /api/lidar/playback/seek endpoint.
func TestHandlePlaybackSeek(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		onSeek         func(int64) error
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			body:           `{"timestamp_ns": 123456}`,
			onSeek:         nil,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with callback succeeds",
			method:         http.MethodPost,
			body:           `{"timestamp_ns": 123456}`,
			onSeek:         func(ts int64) error { return nil },
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST with seek error returns internal error",
			method:         http.MethodPost,
			body:           `{"timestamp_ns": 123456}`,
			onSeek:         func(ts int64) error { return errors.New("seek failed") },
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST with invalid body returns bad request",
			method:         http.MethodPost,
			body:           `invalid json`,
			onSeek:         func(ts int64) error { return nil },
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				onPlaybackSeek: tt.onSeek,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/playback/seek", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()

			ws.handlePlaybackSeek(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandlePlaybackRate tests the POST /api/lidar/playback/rate endpoint.
func TestHandlePlaybackRate(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		onRate         func(float32)
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			body:           `{"rate": 1.5}`,
			onRate:         nil,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with callback succeeds",
			method:         http.MethodPost,
			body:           `{"rate": 1.5}`,
			onRate:         func(r float32) {},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST with zero rate returns bad request",
			method:         http.MethodPost,
			body:           `{"rate": 0}`,
			onRate:         func(r float32) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with negative rate returns bad request",
			method:         http.MethodPost,
			body:           `{"rate": -1}`,
			onRate:         func(r float32) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with rate exceeding maximum returns bad request",
			method:         http.MethodPost,
			body:           `{"rate": 101}`,
			onRate:         func(r float32) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with invalid body returns bad request",
			method:         http.MethodPost,
			body:           `invalid json`,
			onRate:         func(r float32) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				onPlaybackRate: tt.onRate,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/playback/rate", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()

			ws.handlePlaybackRate(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandleVRLogLoad tests the POST /api/lidar/vrlog/load endpoint.
func TestHandleVRLogLoad(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		onLoad         func(string) error
		vrlogSafeDir   string
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "/var/lib/velocity-report/test.vrlog"}`,
			onLoad:         nil,
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with vrlog_path succeeds",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "/var/lib/velocity-report/test.vrlog"}`,
			onLoad:         func(path string) error { return nil },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST with relative path returns bad request",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "relative/path.vrlog"}`,
			onLoad:         func(path string) error { return nil },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with load error returns internal error",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "/var/lib/velocity-report/test.vrlog"}`,
			onLoad:         func(path string) error { return errors.New("load failed") },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST with path outside allowed directory returns bad request",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "/tmp/test.vrlog"}`,
			onLoad:         func(path string) error { return nil },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with directory traversal returns bad request",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "/var/lib/velocity-report/../../../etc/passwd"}`,
			onLoad:         func(path string) error { return nil },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with no run_id or vrlog_path returns bad request",
			method:         http.MethodPost,
			body:           `{}`,
			onLoad:         func(path string) error { return nil },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with invalid body returns bad request",
			method:         http.MethodPost,
			body:           `invalid json`,
			onLoad:         func(path string) error { return nil },
			vrlogSafeDir:   "/var/lib/velocity-report",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				onVRLogLoad:  tt.onLoad,
				vrlogSafeDir: tt.vrlogSafeDir,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/vrlog/load", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()

			ws.handleVRLogLoad(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandleVRLogStop tests the POST /api/lidar/vrlog/stop endpoint.
func TestHandleVRLogStop(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		onStop         func()
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			onStop:         nil,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with callback succeeds",
			method:         http.MethodPost,
			onStop:         func() {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &WebServer{
				onVRLogStop: tt.onStop,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/vrlog/stop", nil)
			w := httptest.NewRecorder()

			ws.handleVRLogStop(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandleVRLogLoadWithRunID tests the run_id lookup path.
func TestHandleVRLogLoadWithRunID(t *testing.T) {
	t.Run("POST with run_id but no db returns internal error", func(t *testing.T) {
		ws := &WebServer{
			onVRLogLoad:  func(path string) error { return nil },
			vrlogSafeDir: "/var/lib/velocity-report",
			db:           nil, // No database configured
		}

		body := `{"run_id": "test-run-123"}`
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/vrlog/load", bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		ws.handleVRLogLoad(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})
}
