package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Clip replay must retain the source binding and stored window when an operator
// requests a complete background/foreground recording for geometry review.
func TestClipReplayRecordingOptions(t *testing.T) {
	for _, tc := range []struct {
		name, body           string
		stream, full, settle bool
		mode                 string
		ratio                float64
		status               int
	}{
		{"legacy", `{}`, false, false, false, "", 0, http.StatusAccepted},
		{"full and settled", `{"full_frames":true,"settle_before_recording":true,"speed_mode":"analysis"}`, true, true, true, "analysis", 0, http.StatusAccepted},
		{"settled foreground", `{"settle_before_recording":true}`, true, false, true, "", 0, http.StatusAccepted},
		{"no publisher", `{"full_frames":true}`, false, false, false, "", 0, http.StatusBadRequest},
		{"scaled", `{"speed_mode":"scaled","speed_ratio":0.5}`, false, false, false, "scaled", 0.5, http.StatusAccepted},
		{"bad mode", `{"speed_mode":"fast"}`, false, false, false, "", 0, http.StatusBadRequest},
		{"bad ratio", `{"speed_mode":"scaled","speed_ratio":-1}`, false, false, false, "", 0, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := setupTestClipServer(t)
			defer ws.db.DB.Close()
			if tc.stream {
				ws.fullFrames = &fakeFullFrames{}
			}
			start, duration := 12.5, 230.0
			clip := &sqlite.Clip{SensorID: "recording-options", Description: "source-bound review", Files: []sqlite.ClipCapture{{PCAPFile: "review.pcap"}}, PCAPStartSecs: &start, PCAPDurationSecs: &duration}
			if err := sqlite.NewClipStore(ws.db).InsertClip(clip); err != nil {
				t.Fatal(err)
			}
			oldStart, oldLast := startPCAPInternalForClip, lastRunIDForClip
			defer func() { startPCAPInternalForClip, lastRunIDForClip = oldStart, oldLast }()
			notified := 0
			ws.onPCAPStarted = func() { notified++ }
			called := false
			startPCAPInternalForClip = func(_ *Server, path string, cfg ReplayConfig) error {
				if notified != 0 {
					t.Fatal("stream notified before successful start")
				}
				called = true
				if path != "review.pcap" || cfg.ClipID != clip.ClipID || cfg.SensorID != clip.SensorID || cfg.StartSeconds != start || cfg.DurationSeconds != duration || !cfg.AnalysisMode || cfg.SpeedMode != tc.mode || cfg.SpeedRatio != tc.ratio || cfg.FullFrames != tc.full || cfg.SettleBeforeRecording != tc.settle {
					t.Fatalf("lost recording request or source/window binding: %s %+v", path, cfg)
				}
				return nil
			}
			lastRunIDForClip = func(*Server) string { return "recording-run" }
			w := httptest.NewRecorder()
			ws.handleClipByID(w, httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			wantNotifications := 0
			if tc.status == http.StatusAccepted {
				wantNotifications = 1
			}
			if notified != wantNotifications {
				t.Fatalf("stream notified %d times, want %d", notified, wantNotifications)
			}
			if called != (tc.status == http.StatusAccepted) {
				t.Fatalf("start called=%v for status %d", called, w.Code)
			}
			if tc.status == http.StatusAccepted && (!strings.Contains(w.Body.String(), clip.ClipID) || !strings.Contains(w.Body.String(), "recording-run")) {
				t.Fatal(w.Body.String())
			}
		})
	}
}

func TestClipReplayRejectsPacedMultiCapture(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()
	clip := &sqlite.Clip{SensorID: "multi-recording", Files: []sqlite.ClipCapture{{Ordinal: 0, PCAPFile: "one.pcap"}, {Ordinal: 1, PCAPFile: "two.pcap"}}}
	if err := sqlite.NewClipStore(ws.db).InsertClip(clip); err != nil {
		t.Fatal(err)
	}
	old := startPCAPInternalForClip
	defer func() { startPCAPInternalForClip = old }()
	startPCAPInternalForClip = func(*Server, string, ReplayConfig) error { t.Fatal("started an incompatible replay"); return nil }
	w := httptest.NewRecorder()
	ws.handleClipByID(w, httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", strings.NewReader(`{"speed_mode":"scaled","speed_ratio":0.5}`)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "multi-capture") {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}

func TestClipReplayStartFailureDoesNotNotifyStream(t *testing.T) {
	ws := setupTestClipServer(t)
	defer ws.db.DB.Close()
	clip := &sqlite.Clip{SensorID: "failed-recording", Files: []sqlite.ClipCapture{{PCAPFile: "one.pcap"}}}
	if err := sqlite.NewClipStore(ws.db).InsertClip(clip); err != nil {
		t.Fatal(err)
	}
	old := startPCAPInternalForClip
	defer func() { startPCAPInternalForClip = old }()
	startPCAPInternalForClip = func(*Server, string, ReplayConfig) error { return errors.New("recording unavailable") }
	ws.onPCAPStarted = func() { t.Fatal("failed replay advertised to the stream") }
	w := httptest.NewRecorder()
	ws.handleClipByID(w, httptest.NewRequest(http.MethodPost, "/api/lidar/clips/"+clip.ClipID+"/replay", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}
