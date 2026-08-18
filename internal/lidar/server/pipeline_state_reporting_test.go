package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if !ws.tryBeginPCAPReplay(ReplayConfig{AnalysisMode: true, SettleBeforeRecording: true}) {
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
