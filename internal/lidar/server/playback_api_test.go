package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlePCAPStartSettleBeforeRecordingRequiresAnalysisMode(t *testing.T) {
	ws := NewServer(Config{SensorID: "sensor-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-1",
		bytes.NewBufferString(`{"pcap_file":"capture.pcap","analysis_mode":false,"settle_before_recording":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	ws.handlePCAPStart(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("requires analysis_mode=true")) {
		t.Fatalf("response = %s", rec.Body.String())
	}
}

func TestHandlePCAPStartRequiresAFileOrASequence(t *testing.T) {
	ws := NewServer(Config{SensorID: "sensor-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-1",
		bytes.NewBufferString(`{"analysis_mode":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	ws.handlePCAPStart(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("pcap_files")) {
		t.Fatalf("the error should name both spellings; got %s", rec.Body.String())
	}
}

// A sequence is replayed against one continuous clock, which realtime and
// scaled playback cannot do — they pace packets against a single file's own
// timestamps. Reaching that refusal also proves pcap_files arrived in the
// replay config, since the guard is keyed on the sequence being longer than one.
func TestHandlePCAPStartAcceptsAPacedSequence(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	for _, name := range []string{"part-a.pcap", "part-b.pcap"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), testPCAPHeader, 0o644); err != nil {
			t.Fatalf("WriteFile(): %v", err)
		}
	}

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    "sensor-sequence",
		PCAPSafeDir: tmpDir,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-sequence",
		bytes.NewBufferString(
			`{"pcap_files":["part-a.pcap","part-b.pcap"],"analysis_mode":false,"speed_mode":"realtime"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	ws.handlePCAPStart(rec, req)

	// A paced sequence is no longer refused: the reader shares one pacing clock
	// across the joins, so realtime and scaled can cross them. Whatever else
	// this fixture fails on, it must not fail on the speed mode.
	if bytes.Contains(rec.Body.Bytes(), []byte("multi-file replay requires")) {
		t.Fatalf("a paced sequence was refused: %s", rec.Body.String())
	}
}

// TestHandlePCAPStartOmittedAnalysisModeDefaultsToTrue is the direct
// regression test for the bug this plan phase fixes: handlePCAPStart
// initialises analysisMode to true, but request parsing used to overwrite it
// with the JSON/form zero value (false) whenever the client omitted the
// field entirely — which every existing test avoided by always sending it
// explicitly, true or false, so the bug went uncaught.
func TestHandlePCAPStartOmittedAnalysisModeDefaultsToTrue(t *testing.T) {
	newTestServer := func(t *testing.T) (*Server, string) {
		t.Helper()
		tmpDir := resolveSymlinks(t, t.TempDir())
		if err := os.WriteFile(filepath.Join(tmpDir, "capture.pcap"), testPCAPHeader, 0o644); err != nil {
			t.Fatalf("WriteFile(): %v", err)
		}
		ws := NewServer(Config{
			Address:     ":0",
			Stats:       NewPacketStats(),
			SensorID:    "sensor-omitted-mode",
			PCAPSafeDir: tmpDir,
		})
		ws.setBaseContext(context.Background())
		return ws, tmpDir
	}

	t.Run("JSON body omitting analysis_mode", func(t *testing.T) {
		ws, _ := newTestServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-omitted-mode",
			bytes.NewBufferString(`{"pcap_file":"capture.pcap"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		ws.handlePCAPStart(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["analysis_mode"] != true {
			t.Errorf("analysis_mode = %v, want true (the default an omitted field must preserve)", resp["analysis_mode"])
		}
		waitForPCAPDone(t, ws)
	})

	t.Run("form body omitting analysis_mode", func(t *testing.T) {
		ws, _ := newTestServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-omitted-mode",
			bytes.NewBufferString("pcap_file=capture.pcap"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		ws.handlePCAPStart(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["analysis_mode"] != true {
			t.Errorf("analysis_mode = %v, want true (the default an omitted field must preserve)", resp["analysis_mode"])
		}
		waitForPCAPDone(t, ws)
	})

	t.Run("form body with explicit analysis_mode=false is honoured, not overridden", func(t *testing.T) {
		ws, _ := newTestServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-omitted-mode",
			bytes.NewBufferString("pcap_file=capture.pcap&analysis_mode=false"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		ws.handlePCAPStart(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["analysis_mode"] != false {
			t.Errorf("analysis_mode = %v, want false (an explicit value present in the form)", resp["analysis_mode"])
		}
		waitForPCAPDone(t, ws)
	})
}

// TestClientStartPCAPReplaySendsExplicitAnalysisModeFalse covers the "legacy
// simple-client replay" path named in the plan: Client.StartPCAPReplay (used
// by the standalone parameter-sweep tool, which reads live tracking output
// rather than a recorded analysis run) used to send only {"pcap_file": ...},
// which the server's old bug happened to interpret as analysis_mode=false.
// Fixing the server bug without touching this client would have silently
// flipped every sweep replay to analysis_mode=true; it must keep sending its
// intent explicitly instead.
func TestClientStartPCAPReplaySendsExplicitAnalysisModeFalse(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	if err := c.StartPCAPReplay("capture.pcap", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, ok := received["analysis_mode"]; !ok || got != false {
		t.Fatalf("analysis_mode should be sent explicitly as false, got %v (present: %v)", got, ok)
	}
}

func TestSequenceSpeedMultiplierMapsTheModes(t *testing.T) {
	cases := []struct {
		name string
		cfg  ReplayConfig
		want float64
	}{
		{"analysis reads as fast as the pipeline allows", ReplayConfig{SpeedMode: "analysis"}, 0},
		{"an unset mode is analysis", ReplayConfig{}, 0},
		{"realtime paces at one", ReplayConfig{SpeedMode: "realtime"}, 1},
		{"scaled carries its ratio", ReplayConfig{SpeedMode: "scaled", SpeedRatio: 0.5}, 0.5},
		// Falling back to analysis here would hand a caller who asked to slow
		// down the fastest replay there is — the opposite of the request.
		{"scaled without a ratio falls back to realtime, not to analysis",
			ReplayConfig{SpeedMode: "scaled"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sequenceSpeedMultiplier(tc.cfg); got != tc.want {
				t.Errorf("sequenceSpeedMultiplier() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHandlePlaybackStatus tests the GET /api/lidar/playback/status endpoint.
// stubPlaybackProbe supplies a fixed replay position.
type stubPlaybackProbe struct{ pos PlaybackPosition }

func (s stubPlaybackProbe) PlaybackPosition() PlaybackPosition { return s.pos }

// TestHandlePlaybackStatus drives the handler from real server state.
//
// The previous version of this test stubbed the getPlaybackStatus callback,
// which is why it passed while production was broken: no wiring ever set that
// callback, so the handler always took the nil branch and reported a hardcoded
// live status regardless of what was actually playing.
func TestHandlePlaybackStatus(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(ws *Server)
		checkResponse func(t *testing.T, resp map[string]interface{})
	}{
		{
			name:  "idle server reports live",
			setup: func(ws *Server) {},
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assertField(t, resp, "mode", "live")
				assertField(t, resp, "replay_active", false)
				assertField(t, resp, "recording", false)
				assertField(t, resp, "seekable", false)
			},
		},
		{
			name:  "VRLOG replay reports vrlog and its path",
			setup: func(ws *Server) { ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc") },
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assertField(t, resp, "mode", "vrlog")
				assertField(t, resp, "replay_active", true)
				assertField(t, resp, "vrlog_path", "/var/lib/velocity-report/vrlog/run-abc")
			},
		},
		{
			name:  "finished analysis replay reports pcap_analysis with the grid kept",
			setup: func(ws *Server) { ws.setTestSourcePCAPAnalysis() },
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assertField(t, resp, "mode", "pcap_analysis")
				assertField(t, resp, "replay_active", false)
				assertField(t, resp, "grid_preserved", true)
			},
		},
		{
			name: "active recording reports its path and run",
			setup: func(ws *Server) {
				ws.setTestSourcePCAPAnalysisReplaying()
				ws.setRecording("run-abc", "/data/vrlog/run-abc")
				ws.setRecordingFrames(12)
			},
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assertField(t, resp, "recording", true)
				assertField(t, resp, "recording_path", "/data/vrlog/run-abc")
				assertField(t, resp, "recording_run_id", "run-abc")
				assertField(t, resp, "recording_frames", float64(12))
			},
		},
		{
			name: "settling pass is distinguishable from the recorded pass",
			setup: func(ws *Server) {
				_, _ = ws.tryBeginPCAPReplay(ReplayConfig{AnalysisMode: true, SettleBeforeRecording: true})
				ws.setReplayPass(ReplayPassSettling)
			},
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assertField(t, resp, "replay_pass", "settling")
				assertField(t, resp, "replay_total_passes", float64(2))
				assertField(t, resp, "recording", false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &Server{state: newPipelineState()}
			tt.setup(ws)

			req := httptest.NewRequest(http.MethodGet, "/api/lidar/playback/status", nil)
			w := httptest.NewRecorder()
			ws.handlePlaybackStatus(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
			}
			var resp map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			tt.checkResponse(t, resp)
		})
	}
}

// TestHandlePlaybackStatusUsesProbeForPosition verifies that replay position is
// pulled from the streaming layer rather than mirrored into server state.
func TestHandlePlaybackStatusUsesProbeForPosition(t *testing.T) {
	ws := &Server{
		state: newPipelineState(),
		playbackProbe: stubPlaybackProbe{pos: PlaybackPosition{
			Paused:       true,
			Rate:         1.5,
			Seekable:     true,
			CurrentFrame: 42,
			TotalFrames:  500,
			ReplayEpoch:  3,
		}},
	}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/playback/status", nil)
	w := httptest.NewRecorder()
	ws.handlePlaybackStatus(w, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	assertField(t, resp, "mode", "vrlog")
	assertField(t, resp, "paused", true)
	assertField(t, resp, "seekable", true)
	assertField(t, resp, "current_frame", float64(42))
	assertField(t, resp, "total_frames", float64(500))
	assertField(t, resp, "replay_epoch", float64(3))
	assertField(t, resp, "rate", float64(1.5))
}

// TestHandlePlaybackStatusWithoutProbe verifies the handler still reports mode
// truthfully when no streaming layer is attached.
func TestHandlePlaybackStatusWithoutProbe(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/playback/status", nil)
	w := httptest.NewRecorder()
	ws.handlePlaybackStatus(w, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	assertField(t, resp, "mode", "vrlog")
	assertField(t, resp, "current_frame", float64(0))
	assertField(t, resp, "rate", float64(1))
}

func assertField(t *testing.T, resp map[string]interface{}, key string, want interface{}) {
	t.Helper()
	if got, ok := resp[key]; !ok {
		t.Errorf("response has no %q field", key)
	} else if got != want {
		t.Errorf("%s = %v (%T), want %v (%T)", key, got, got, want, want)
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
			ws := &Server{
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
			ws := &Server{
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
			ws := &Server{
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
			ws := &Server{
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

// TestHandleVRLogLoad tests the POST /api/lidar/vrlog/load endpoint. Path
// values are computed against a real resolved temp directory rather than a
// literal like /var/lib/velocity-report: the safe-directory check now
// resolves symlinks (security.ResolvePathWithinDirectory), which requires
// the safe directory to actually exist on disk.
func TestHandleVRLogLoad(t *testing.T) {
	safeDir := resolveSymlinks(t, t.TempDir())
	inSafeDir := filepath.Join(safeDir, "test.vrlog")

	outsideDir := resolveSymlinks(t, t.TempDir())
	symlinkEscape := filepath.Join(safeDir, "escape")
	if err := os.Symlink(outsideDir, symlinkEscape); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}

	tests := []struct {
		name           string
		method         string
		body           string
		onLoad         func(string) (string, error)
		vrlogSafeDir   string
		expectedStatus int
	}{
		{
			name:           "POST without callback returns not implemented",
			method:         http.MethodPost,
			body:           fmt.Sprintf(`{"vrlog_path": %q}`, inSafeDir),
			onLoad:         nil,
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "POST with vrlog_path succeeds",
			method:         http.MethodPost,
			body:           fmt.Sprintf(`{"vrlog_path": %q}`, inSafeDir),
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST with relative path returns bad request",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "relative/path.vrlog"}`,
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with load error returns internal error",
			method:         http.MethodPost,
			body:           fmt.Sprintf(`{"vrlog_path": %q}`, inSafeDir),
			onLoad:         func(path string) (string, error) { return "", errors.New("load failed") },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "POST with path outside allowed directory returns bad request",
			method:         http.MethodPost,
			body:           `{"vrlog_path": "/tmp/test.vrlog"}`,
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with directory traversal returns bad request",
			method:         http.MethodPost,
			body:           fmt.Sprintf(`{"vrlog_path": %q}`, filepath.Join(safeDir, "..", "..", "..", "etc", "passwd")),
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusBadRequest,
		},
		{
			// A symlink inside the safe directory pointing outside it. A plain
			// string-prefix check on the literal path would accept this, since
			// the string itself starts with the safe directory; the resolved
			// (symlink-following) check must not.
			name:           "POST with a symlink escaping the safe directory returns bad request",
			method:         http.MethodPost,
			body:           fmt.Sprintf(`{"vrlog_path": %q}`, filepath.Join(symlinkEscape, "secret.vrlog")),
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with no run_id or vrlog_path returns bad request",
			method:         http.MethodPost,
			body:           `{}`,
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "POST with invalid body returns bad request",
			method:         http.MethodPost,
			body:           `invalid json`,
			onLoad:         func(path string) (string, error) { return "proto", nil },
			vrlogSafeDir:   safeDir,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &Server{
				onVRLogLoad:  tt.onLoad,
				vrlogSafeDir: tt.vrlogSafeDir,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/vrlog/load", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()

			ws.handleVRLogLoad(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d; body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
			if tt.expectedStatus == http.StatusOK {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}
				if got := resp["frame_encoding"]; got != "proto" {
					t.Errorf("expected frame_encoding=proto, got %v", got)
				}
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
			expectedStatus: http.StatusOK,
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
			ws := &Server{
				onVRLogStop: tt.onStop,
			}

			req := httptest.NewRequest(tt.method, "/api/lidar/vrlog/stop", nil)
			w := httptest.NewRecorder()

			ws.handleReplayStop(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandleVRLogLoadWithRunID tests the run_id lookup path.
func TestHandleVRLogLoadWithRunID(t *testing.T) {
	t.Run("POST with run_id but no db returns internal error", func(t *testing.T) {
		ws := &Server{
			onVRLogLoad:  func(path string) (string, error) { return "proto", nil },
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
