package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func decodeDataSource(t *testing.T, ws *Server) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/data_source?sensor_id=test-sensor", nil)
	w := httptest.NewRecorder()
	ws.handleDataSource(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("data_source status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode data_source: %v", err)
	}
	return resp
}

// TestHandleDataSourceReportsVRLogReplay covers the case that motivated this
// work: a VRLOG replay used to leave currentSource untouched, so the data
// source API kept reporting whatever preceded it — normally "live".
func TestHandleDataSourceReportsVRLogReplay(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	resp := decodeDataSource(t, ws)
	assertField(t, resp, "data_source", "vrlog")
	assertField(t, resp, "source_path", "/var/lib/velocity-report/vrlog/run-abc")
	assertField(t, resp, "replay_active", true)
	// A VRLOG replay is not a PCAP replay, and pcap_file has no meaning here.
	assertField(t, resp, "pcap_in_progress", false)
	assertField(t, resp, "pcap_file", "")
}

// TestPCAPInProgressFalseDuringVRLogReplay guards the sweep runner.
// Client.WaitForPCAPComplete gates on pcap_in_progress and sweep/runner.go
// calls it once per parameter combination, so reporting true for any active
// replay would make every sweep block until its timeout.
func TestPCAPInProgressFalseDuringVRLogReplay(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	if got := ws.PipelineState().PCAPInProgress(); got {
		t.Error("PCAPInProgress() = true during a VRLOG replay; sweeps would hang")
	}
	assertField(t, decodeDataSource(t, ws), "pcap_in_progress", false)
}

// TestHandleDataSourceGridPreservedAfterResumeLive verifies that resume-live's
// grid retention survives past its own response. It used to appear only in the
// resume_live body and then vanish, because the source enum had nowhere to
// record it.
func TestHandleDataSourceGridPreservedAfterResumeLive(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	ws.setTestSourcePCAPAnalysis()

	resp := decodeDataSource(t, ws)
	assertField(t, resp, "data_source", "pcap_analysis")
	assertField(t, resp, "grid_preserved", true)

	// Resume live keeping the grid.
	ws.setSourceLive(true)

	resp = decodeDataSource(t, ws)
	assertField(t, resp, "data_source", "live")
	assertField(t, resp, "grid_preserved", true)

	// Stopping instead resets the grid.
	ws.setSourceLive(false)
	assertField(t, decodeDataSource(t, ws), "grid_preserved", false)
}

// TestHandleDataSourceReportsRecording verifies recording is observable. It
// previously lived only as closure locals in the radar command and a bool
// inside the replay goroutine.
func TestHandleDataSourceReportsRecording(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	ws.setTestSourcePCAPAnalysisReplaying()
	ws.setRecording("run-abc", "/data/vrlog/run-abc")

	resp := decodeDataSource(t, ws)
	assertField(t, resp, "recording", true)
	assertField(t, resp, "recording_path", "/data/vrlog/run-abc")

	ws.clearRecording()
	assertField(t, decodeDataSource(t, ws), "recording", false)
}

// TestHandleDataSourceReportsReplayPasses verifies the settling pass is
// distinguishable from the recorded pass. Both passes report the same source
// and in-progress flag, and packet progress restarts between them.
func TestHandleDataSourceReportsReplayPasses(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{AnalysisMode: true, SettleBeforeRecording: true}); !ok {
		t.Fatal("tryBeginPCAPReplay returned false on an idle server")
	}

	ws.setReplayPass(ReplayPassSettling)
	resp := decodeDataSource(t, ws)
	assertField(t, resp, "replay_pass", "settling")
	assertField(t, resp, "replay_total_passes", float64(2))

	ws.setReplayPass(ReplayPassRecording)
	assertField(t, decodeDataSource(t, ws), "replay_pass", "recording")
}

// TestHandleLidarStatusRendersPCAPAnalysisMode covers the HTML status page,
// whose mode switch had no case for the analysis state and so rendered
// "Live UDP" while the pipeline sat on a preserved PCAP grid.
func TestHandleLidarStatusRendersPCAPAnalysisMode(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	ws.setTestSourcePCAPAnalysis()

	if got := ws.PipelineState().StatusLabel(); got != "PCAP Analysis" {
		t.Errorf("StatusLabel() = %q, want %q", got, "PCAP Analysis")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/status?sensor_id=test-sensor", nil)
	w := httptest.NewRecorder()
	ws.handleLidarStatus(w, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	assertField(t, resp, "data_source", "pcap_analysis")
}

// TestHandleVRLogLoadStopsLiveListener verifies that loading a VRLOG takes the
// live UDP listener down. Without this the listener keeps feeding L1/L2 while
// recorded frames stream to the visualiser, so live and replayed frames
// interleave — and reporting "vrlog" while live packets are still processed
// would trade one false status for another.
func TestHandleVRLogLoadStopsLiveListener(t *testing.T) {
	tmp := t.TempDir()
	ws := &Server{
		sensorID:     "test-sensor",
		state:        newPipelineState(),
		vrlogSafeDir: tmp,
		onVRLogLoad:  func(string) (string, error) { return "protobuf", nil },
		onVRLogStop:  func() {},
	}
	ws.setLiveListenerRunning(true)

	body := `{"vrlog_path":"` + filepath.Join(tmp, "run-abc") + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/vrlog/load", strings.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleVRLogLoad(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("vrlog/load status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	state := ws.PipelineState()
	if state.Source != SourceModeVRLog {
		t.Errorf("Source = %q, want %q", state.Source, SourceModeVRLog)
	}
	if state.LiveListenerRunning {
		t.Error("live listener still running during a VRLOG replay")
	}
	assertField(t, decodeDataSource(t, ws), "live_listener_running", false)
}

// TestHandleVRLogStopReturnsToLive verifies the source returns to live when the
// replay is stopped.
func TestHandleVRLogStopReturnsToLive(t *testing.T) {
	ws := &Server{
		sensorID:    "test-sensor",
		state:       newPipelineState(),
		onVRLogStop: func() {},
	}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/vrlog/stop", nil)
	w := httptest.NewRecorder()
	ws.handleReplayStop(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("vrlog/stop status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := ws.PipelineState().Source; got != SourceModeLive {
		t.Errorf("Source = %q, want %q", got, SourceModeLive)
	}
	assertField(t, decodeDataSource(t, ws), "data_source", "live")
}

// TestRecordingNotReportedWhenRecorderCannotStart guards against reporting a
// recording that never began. OnRecordingStart returns an empty path when the
// recorder could not be created — most often because no visualiser publisher is
// configured — and claiming recording=true then is its own small falsehood.
func TestRecordingNotReportedWhenRecorderCannotStart(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}

	// A recorder that started successfully reports its path.
	ws.setRecording("run-abc", "/data/vrlog/run-abc")
	state := ws.PipelineState()
	if !state.Recording || state.RecordingPath == "" {
		t.Fatalf("expected an active recording, got %+v", state)
	}

	// One that could not start must leave the state alone.
	ws.clearRecording()
	if got := ws.PipelineState(); got.Recording {
		t.Errorf("Recording = true after clearRecording, want false")
	}
	assertField(t, decodeDataSource(t, ws), "recording", false)
}

// TestRecordingPathClearedWhenANewReplayStarts verifies the recording path is
// retained after completion — so a caller polling later can see what was
// written — but does not follow an unrelated replay around.
func TestRecordingPathClearedWhenANewReplayStarts(t *testing.T) {
	ws := &Server{sensorID: "test-sensor", state: newPipelineState()}
	ws.setTestSourcePCAPAnalysisReplaying()
	ws.setRecording("run-abc", "/data/vrlog/run-abc")
	ws.endReplay(true)

	// Retained in the terminal analysis state.
	if got := ws.PipelineState().RecordingPath; got != "/data/vrlog/run-abc" {
		t.Errorf("RecordingPath = %q after completion, want it retained", got)
	}

	// Cleared once a VRLOG replay takes over.
	ws.setSourceVRLog("/data/vrlog/run-xyz")
	if got := ws.PipelineState().RecordingPath; got != "" {
		t.Errorf("RecordingPath = %q during an unrelated VRLOG replay, want empty", got)
	}

	// Cleared once a new PCAP replay starts.
	ws.setSourceLive(false)
	ws.setRecording("run-abc", "/data/vrlog/run-abc")
	ws.endReplay(false)
	if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{}); !ok {
		t.Fatal("tryBeginPCAPReplay returned false on an idle server")
	}
	if got := ws.PipelineState().RecordingPath; got != "" {
		t.Errorf("RecordingPath = %q during a new PCAP replay, want empty", got)
	}
}
