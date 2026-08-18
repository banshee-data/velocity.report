package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The reported failure was a single wedged state reached by ordinary use, in
// which all three of these were true at once:
//
//   - POST /pcap/start   -> "PCAP replay is already in progress"
//   - POST /pcap/stop    -> "system is not in PCAP mode"
//   - plugging in the sensor produced no logs and no state change
//
// All three come from ReplayActive being stranded true while Source is not
// PCAP: the start guard sees a replay, the stop guard sees the wrong source,
// and the live listener is never restarted because the restart lives inside
// the teardown that was skipped.

// TestReleaseReplaySlotClearsWedgeRegardlessOfSource covers the route that
// created it: the replay teardown used to sit inside an `if Source == pcap`
// branch, so a source switch while a replay was running skipped it entirely.
func TestReleaseReplaySlotClearsWedgeRegardlessOfSource(t *testing.T) {
	for _, source := range []SourceMode{SourceModeLive, SourceModeVRLog, SourceModePCAP} {
		t.Run(string(source), func(t *testing.T) {
			ws := &Server{state: newPipelineState()}
			if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{}); !ok {
				t.Fatal("tryBeginPCAPReplay refused a fresh server")
			}
			// A concurrent request moves the source out from under the replay.
			ws.mutateState("test-source-switch", func(s *PipelineState) { s.Source = source })

			ws.releaseReplaySlot()

			if got := ws.PipelineState(); got.ReplayActive {
				t.Errorf("ReplayActive still set after release with Source=%q: %s", source, got)
			}
			// The slot release must not seize the source back from its owner.
			if got := ws.PipelineState().Source; got != source {
				t.Errorf("Source = %q, want %q: releasing the slot must not change the source", got, source)
			}
		})
	}
}

// TestReplaySlotIsReclaimableAfterRelease is the end of the user's loop: once
// the slot is released a new replay must be able to start.
func TestReplaySlotIsReclaimableAfterRelease(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{}); !ok {
		t.Fatal("first start refused")
	}
	ws.mutateState("test-source-switch", func(s *PipelineState) { s.Source = SourceModeVRLog })
	ws.releaseReplaySlot()

	ok, blocker := ws.tryBeginPCAPReplay(ReplayConfig{})
	if !ok {
		t.Fatalf("second start refused with blocker %q; the slot was never released", blocker)
	}
}

// TestTryBeginPCAPReplayNamesTheBlockingReplay covers the contradictory error
// messages. A VRLOG replay holding the slot used to be reported as a PCAP
// replay, sending callers to /pcap/stop, which answered "not in PCAP mode".
func TestTryBeginPCAPReplayNamesTheBlockingReplay(t *testing.T) {
	tests := []struct {
		name string
		set  func(*PipelineState)
		want SourceMode
	}{
		{"vrlog holds the slot", func(s *PipelineState) {
			s.Source = SourceModeVRLog
			s.ReplayActive = true
		}, SourceModeVRLog},
		{"pcap holds the slot", func(s *PipelineState) {
			s.Source = SourceModePCAP
			s.ReplayActive = true
		}, SourceModePCAP},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &Server{state: newPipelineState()}
			ws.mutateState("test-setup", tt.set)

			ok, blocker := ws.tryBeginPCAPReplay(ReplayConfig{})
			if ok {
				t.Fatal("expected the start to be refused")
			}
			if blocker != tt.want {
				t.Errorf("blocker = %q, want %q: the error message names the wrong endpoint", blocker, tt.want)
			}
		})
	}
}

// TestPipelineStateValidateNamesImpossibleCombinations checks the invariants
// that turn a silent wedge into a logged one.
func TestPipelineStateValidateNamesImpossibleCombinations(t *testing.T) {
	valid := []struct {
		name  string
		state PipelineState
	}{
		{"fresh", newPipelineState()},
		{"pcap replaying", PipelineState{Source: SourceModePCAP, SourcePath: "a.pcapng", ReplayActive: true, TotalPasses: 1}},
		{"vrlog loaded and played out", PipelineState{Source: SourceModeVRLog, SourcePath: "run/", ReplayActive: true, TotalPasses: 1}},
		{"pcap analysis retained", PipelineState{Source: SourceModePCAP, SourcePath: "a.pcapng", GridPreserved: true, TotalPasses: 1}},
		{"recording attributed", PipelineState{Source: SourceModePCAP, SourcePath: "a.pcapng", ReplayActive: true, TotalPasses: 1, Recording: true, RecordingRunID: "run-1"}},
	}
	for _, tt := range valid {
		t.Run("valid/"+tt.name, func(t *testing.T) {
			if err := tt.state.Validate(); err != nil {
				t.Errorf("Validate() rejected a legitimate state: %v (%s)", err, tt.state)
			}
		})
	}

	invalid := []struct {
		name  string
		state PipelineState
	}{
		{"the wedge: replay active on live", PipelineState{Source: SourceModeLive, ReplayActive: true, TotalPasses: 1}},
		{"pass without a replay", PipelineState{Source: SourceModePCAP, Pass: ReplayPassSettling, TotalPasses: 1}},
		{"zero passes", PipelineState{Source: SourceModeLive}},
		{"unattributed recording", PipelineState{Source: SourceModePCAP, SourcePath: "a.pcapng", ReplayActive: true, TotalPasses: 1, Recording: true}},
		{"live with a leftover path", PipelineState{Source: SourceModeLive, SourcePath: "stale.pcapng", TotalPasses: 1}},
	}
	for _, tt := range invalid {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			if err := tt.state.Validate(); err == nil {
				t.Errorf("Validate() accepted an impossible state: %s", tt.state)
			}
		})
	}
}

// TestEveryTransitionHelperLeavesAValidState runs each setter against the
// states it can legitimately follow and asserts the invariants hold, so a new
// helper that strands a field is caught here rather than in production.
func TestEveryTransitionHelperLeavesAValidState(t *testing.T) {
	transitions := []struct {
		name  string
		apply func(*Server)
	}{
		{"setSourceLive", func(ws *Server) { ws.setSourceLive(false) }},
		{"setSourceLive preserving grid", func(ws *Server) { ws.setSourceLive(true) }},
		{"setSourceVRLog", func(ws *Server) { ws.setSourceVRLog("run/") }},
		{"endReplay", func(ws *Server) { ws.endReplay(false) }},
		{"releaseReplaySlot", func(ws *Server) { ws.releaseReplaySlot() }},
		{"abandonReplayStart", func(ws *Server) { ws.abandonReplayStart() }},
		{"setLiveListenerRunning", func(ws *Server) { ws.setLiveListenerRunning(true) }},
		{"setReplayProgress", func(ws *Server) { ws.setReplayProgress(5, 10) }},
	}

	starts := []struct {
		name string
		set  func(*PipelineState)
	}{
		{"from live", func(s *PipelineState) {}},
		{"from pcap replaying", func(s *PipelineState) {
			s.Source = SourceModePCAP
			s.SourcePath = "a.pcapng"
			s.ReplayActive = true
		}},
		{"from vrlog", func(s *PipelineState) {
			s.Source = SourceModeVRLog
			s.SourcePath = "run/"
			s.ReplayActive = true
		}},
	}

	for _, start := range starts {
		for _, tr := range transitions {
			t.Run(start.name+"/"+tr.name, func(t *testing.T) {
				ws := &Server{state: newPipelineState()}
				ws.mutateState("test-setup", start.set)
				tr.apply(ws)
				if err := ws.PipelineState().Validate(); err != nil {
					t.Errorf("%s left an invalid state: %v (%s)", tr.name, err, ws.PipelineState())
				}
			})
		}
	}
}

// TestPCAPStopExplainsAVRLogReplay covers the user-visible half of the loop.
// With a VRLOG replay holding the slot, POST /pcap/stop used to answer "system
// is not in PCAP mode: stop the current data source first" without naming which
// source or which endpoint, while POST /pcap/start blamed a PCAP replay. The
// message must now point at the endpoint that can actually stop it.
func TestPCAPStopExplainsAVRLogReplay(t *testing.T) {
	const sensorID = "sensor-wedge-vrlog"
	ws := &Server{sensorID: sensorID, state: newPipelineState()}
	ws.mutateState("test-setup", func(s *PipelineState) {
		s.Source = SourceModeVRLog
		s.SourcePath = "run/"
		s.ReplayActive = true
	})

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/stop?sensor_id="+sensorID, nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "vrlog/stop") {
		t.Errorf("stop error does not name the endpoint that can stop the replay: %s", body)
	}
	if !strings.Contains(body, "VRLOG") {
		t.Errorf("stop error does not name VRLOG as the active replay: %s", body)
	}
}

// TestPCAPStopExplainsAnIdleSource covers the same surface when nothing is
// replaying at all: the caller is told what the source actually is.
func TestPCAPStopExplainsAnIdleSource(t *testing.T) {
	const sensorID = "sensor-wedge-idle"
	ws := &Server{sensorID: sensorID, state: newPipelineState()}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/stop?sensor_id="+sensorID, nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "live") {
		t.Errorf("stop error does not report the actual source: %s", body)
	}
}
