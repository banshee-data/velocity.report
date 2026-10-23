package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// fakeFullFrames stands in for the visualiser's frame adapter.
type fakeFullFrames struct {
	mu     sync.Mutex
	on     bool
	record bool
	calls  []bool
	// recordCalls are the SetRecordFullFrames calls.
	recordCalls []bool
}

func (f *fakeFullFrames) SetRecordFullFrames(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record = on
	f.recordCalls = append(f.recordCalls, on)
}

func (f *fakeFullFrames) recording() (bool, []bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.record, append([]bool(nil), f.recordCalls...)
}

func (f *fakeFullFrames) SetFullFrames(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = on
	f.calls = append(f.calls, on)
}

func (f *fakeFullFrames) FullFrames() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on || f.record
}

func (f *fakeFullFrames) history() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.calls...)
}

func toggleFullFrames(t *testing.T, ws *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/replay/full_frames", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ws.handleReplayFullFrames(rec, req)
	return rec
}

// startFullFramesReplay starts a replay of an empty capture with the given
// request and waits for it to reach its end.
func startFullFramesReplay(t *testing.T, sensorID string, control *fakeFullFrames, build func(file string) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	dir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "capture.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	ws := NewServer(Config{Stats: NewPacketStats(), SensorID: sensorID, PCAPSafeDir: dir, FullFrames: control})
	ws.setBaseContext(context.Background())
	rec := httptest.NewRecorder()
	ws.handlePCAPStart(rec, build("capture.pcap"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body.String())
	}
	waitForPCAPDone(t, ws)
	if control.FullFrames() {
		t.Error("full frames outlived the replay: end of file must restore split streaming")
	}
	return rec
}

func TestHandlePCAPStartFullFramesOption(t *testing.T) {
	jsonStart := func(body string) func(string) *http.Request {
		return func(string) *http.Request {
			req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-ff", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			return req
		}
	}
	formStart := func(values url.Values) func(string) *http.Request {
		return func(string) *http.Request {
			req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-ff", strings.NewReader(values.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return req
		}
	}
	for _, tc := range []struct {
		name  string
		build func(string) *http.Request
		on    bool
	}{
		{"json on", jsonStart(`{"pcap_file":"capture.pcap","analysis_mode":false,"full_frames":true}`), true},
		{"json off", jsonStart(`{"pcap_file":"capture.pcap","analysis_mode":false,"full_frames":false}`), false},
		{"json omitted", jsonStart(`{"pcap_file":"capture.pcap","analysis_mode":false}`), false},
		{"form true", formStart(url.Values{"pcap_file": {"capture.pcap"}, "analysis_mode": {"false"}, "full_frames": {"true"}}), true},
		{"form 1", formStart(url.Values{"pcap_file": {"capture.pcap"}, "analysis_mode": {"false"}, "full_frames": {"1"}}), true},
		{"form omitted", formStart(url.Values{"pcap_file": {"capture.pcap"}, "analysis_mode": {"false"}}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control := &fakeFullFrames{}
			startFullFramesReplay(t, "sensor-ff", control, tc.build)
			calls := control.history()
			turnedOn := len(calls) > 0 && calls[0]
			if turnedOn != tc.on {
				t.Fatalf("calls %v: full frames turned on = %v, want %v", calls, turnedOn, tc.on)
			}
			if len(calls) == 0 || calls[len(calls)-1] {
				t.Errorf("calls %v: the replay's end must switch full frames off", calls)
			}
			// A replay started with full frames records them; one started
			// without never asks to.
			record, recordCalls := control.recording()
			recordedFull := len(recordCalls) > 0 && recordCalls[0]
			if recordedFull != tc.on || record {
				t.Errorf("record calls %v: recording full = %v, want %v, and off at the end", recordCalls, recordedFull, tc.on)
			}
		})
	}
}

// A server without a visualiser stream refuses the option before anything is
// stopped, rather than start a replay that cannot do what was asked.
func TestHandlePCAPStartFullFramesNeedsTheVisualiserStream(t *testing.T) {
	ws := NewServer(Config{SensorID: "sensor-ff"})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-ff",
		bytes.NewBufferString(`{"pcap_file":"capture.pcap","full_frames":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ws.handlePCAPStart(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "full_frames needs the visualiser") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestReplayFullFramesToggleRefusesOutsideAPCAPReplay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state func(ws *Server)
	}{
		{"live", func(ws *Server) { ws.setTestSourceLive() }},
		{"vrlog replay", func(ws *Server) { ws.setSourceVRLog("/var/lib/velocity-report/run-1") }},
		{"finished analysis replay", func(ws *Server) { ws.setTestSourcePCAPAnalysis() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control := &fakeFullFrames{}
			ws := NewServer(Config{SensorID: "sensor-ff", FullFrames: control})
			tc.state(ws)
			rec := toggleFullFrames(t, ws, `{"enabled":true}`)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409; body %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "only for a running PCAP replay") {
				t.Errorf("body %s does not say why", rec.Body.String())
			}
			if len(control.history()) != 0 {
				t.Errorf("the refused request switched the mode: %v", control.history())
			}
		})
	}

	ws := NewServer(Config{SensorID: "sensor-ff"})
	ws.setTestSourcePCAPReplaying()
	if rec := toggleFullFrames(t, ws, `{"enabled":true}`); rec.Code != http.StatusNotImplemented {
		t.Errorf("no visualiser stream: status %d, want 501", rec.Code)
	}
}

func TestReplayFullFramesToggleDuringAPCAPReplay(t *testing.T) {
	control := &fakeFullFrames{}
	ws := NewServer(Config{SensorID: "sensor-ff", FullFrames: control})
	ws.setTestSourcePCAPReplaying()

	for _, bad := range []string{``, `{}`, `{"enabled":"yes"}`} {
		if rec := toggleFullFrames(t, ws, bad); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", bad, rec.Code)
		}
	}

	rec := toggleFullFrames(t, ws, `{"enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["full_frames"] != true {
		t.Fatalf("response %s (%v)", rec.Body.String(), err)
	}
	if !control.FullFrames() {
		t.Fatal("the toggle did not switch full frames on")
	}

	// The status the visualiser reads says so.
	status := httptest.NewRecorder()
	ws.handlePlaybackStatus(status, httptest.NewRequest(http.MethodGet, "/api/lidar/playback/status", nil))
	var info PlaybackStatusInfo
	if err := json.Unmarshal(status.Body.Bytes(), &info); err != nil || !info.FullFrames {
		t.Fatalf("playback status %s (%v): want full_frames true", status.Body.String(), err)
	}

	if rec := toggleFullFrames(t, ws, `{"enabled":false}`); rec.Code != http.StatusOK || control.FullFrames() {
		t.Fatalf("switching off: status %d, on %v", rec.Code, control.FullFrames())
	}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/replay/full_frames?sensor_id=other", strings.NewReader(`{"enabled":true}`))
	wrong := httptest.NewRecorder()
	ws.handleReplayFullFrames(wrong, req)
	if wrong.Code != http.StatusNotFound {
		t.Errorf("another sensor: status %d, want 404", wrong.Code)
	}
}

// A replay started with full frames records them, so the toggle cannot switch
// them off partway; one started without can switch them on and off freely,
// and its recording never changes kind.
func TestReplayFullFramesToggleCannotStopARecordedFullFramesReplay(t *testing.T) {
	control := &fakeFullFrames{}
	ws := NewServer(Config{SensorID: "sensor-ff", FullFrames: control})
	ws.setTestSourcePCAPReplaying()

	if rec := toggleFullFrames(t, ws, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("on in a foreground run: %d", rec.Code)
	}
	if _, recordCalls := control.recording(); len(recordCalls) != 0 {
		t.Fatalf("the toggle changed what is recorded: %v", recordCalls)
	}
	if rec := toggleFullFrames(t, ws, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("off in a foreground run: %d", rec.Code)
	}

	ws.beginReplayFullFrames(true)
	rec := toggleFullFrames(t, ws, `{"enabled":false}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "records full frames") {
		t.Fatalf("off in a full-frames run: %d %s", rec.Code, rec.Body.String())
	}
	if !control.FullFrames() {
		t.Fatal("a refused switch-off switched them off")
	}
	if rec := toggleFullFrames(t, ws, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Errorf("on in a full-frames run: %d", rec.Code)
	}
	ws.endReplayFullFrames()
	if record, _ := control.recording(); record || control.FullFrames() {
		t.Error("the replay's end left full frames on")
	}
}

func TestReplayFrameModeFollowsTheStartOption(t *testing.T) {
	if got := replayFrameMode(ReplayConfig{FullFrames: true}); got != "full" {
		t.Errorf("full frames: %q", got)
	}
	if got := replayFrameMode(ReplayConfig{}); got != "foreground" {
		t.Errorf("default: %q", got)
	}
}

// Every way out of a PCAP replay restores split streaming.
func TestFullFramesEndWithTheReplay(t *testing.T) {
	newReplaying := func(t *testing.T) (*Server, *fakeFullFrames, context.CancelFunc) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		control := &fakeFullFrames{}
		ws := &Server{
			sensorID:          "sensor-ff",
			udpListenerConfig: network.UDPListenerConfig{Address: ":0"},
			fullFrames:        control,
		}
		ws.setBaseContext(ctx)
		ws.setTestSourcePCAPReplaying()
		ws.pcapMu.Lock()
		ws.pcapDone = make(chan struct{})
		close(ws.pcapDone)
		ws.pcapMu.Unlock()
		if rec := toggleFullFrames(t, ws, `{"enabled":true}`); rec.Code != http.StatusOK || !control.FullFrames() {
			t.Fatalf("could not switch full frames on: %d %s", rec.Code, rec.Body.String())
		}
		return ws, control, cancel
	}
	stopListener := func(ws *Server) {
		ws.dataSourceMu.Lock()
		ws.stopLiveListenerLocked()
		ws.dataSourceMu.Unlock()
	}

	t.Run("stop", func(t *testing.T) {
		ws, control, cancel := newReplaying(t)
		defer cancel()
		rec := httptest.NewRecorder()
		ws.handleReplayStop(rec, httptest.NewRequest(http.MethodPost, "/api/lidar/replay/stop", nil))
		defer stopListener(ws)
		if rec.Code != http.StatusOK || control.FullFrames() {
			t.Fatalf("stop: status %d, full frames %v", rec.Code, control.FullFrames())
		}
		// And the toggle cannot bring them back on live input.
		if rec := toggleFullFrames(t, ws, `{"enabled":true}`); rec.Code != http.StatusConflict || control.FullFrames() {
			t.Fatalf("after stop: status %d, full frames %v", rec.Code, control.FullFrames())
		}
	})

	t.Run("end of file", func(t *testing.T) {
		ws, control, cancel := newReplaying(t)
		defer cancel()
		ws.parkFinishedReplay("PCAP replay finished")
		defer ws.cancelParkedLiveWatch()
		defer stopListener(ws)
		if control.FullFrames() {
			t.Fatal("a parked replay kept full frames")
		}
	})

	t.Run("resume live", func(t *testing.T) {
		ws, control, cancel := newReplaying(t)
		defer cancel()
		// Resume-live is from a finished analysis replay; full frames were
		// left on here only to show it clears them whatever came before.
		ws.setTestSourcePCAPAnalysis()
		rec := httptest.NewRecorder()
		ws.handlePCAPResumeLive(rec, httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/resume_live?sensor_id=sensor-ff", nil))
		defer stopListener(ws)
		if rec.Code != http.StatusOK || control.FullFrames() {
			t.Fatalf("resume live: status %d, full frames %v", rec.Code, control.FullFrames())
		}
	})
}
