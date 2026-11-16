package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func handoverRequest(t *testing.T, ws *Server) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recording")
	mustMkdirRecording(t, path)
	ws.vrlogSafeDir = filepath.Dir(path)
	return httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/lidar/vrlog/load",
		strings.NewReader(`{"vrlog_path":"`+path+`"}`))
}

type handoverFrameBuilder struct{ flush, drain func() }

func (*handoverFrameBuilder) SetBlockOnFrameChannel(bool) {}
func (*handoverFrameBuilder) DroppedFrames() uint64       { return 0 }
func (fb *handoverFrameBuilder) FlushPendingFrames()      { fb.flush() }
func (fb *handoverFrameBuilder) WaitForCallbacks()        { fb.drain() }

func TestVRLogHandoverDrainsLiveCallbacksWithoutRestartingUDP(t *testing.T) {
	t.Cleanup(restoreDatasourceHandlerSeams())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	ws := &Server{state: newPipelineState(), udpListenerCancel: cancel, udpListenerDone: done}
	ws.setLiveListenerRunning(true)
	go func() { <-ctx.Done(); close(done) }()
	flushed, drained := false, false
	getReplayFrameBuilder = func(string) replayFrameBuilder {
		return &handoverFrameBuilder{flush: func() {
			select {
			case <-done:
			default:
				t.Error("flushed before UDP input stopped")
			}
			flushed = true
		}, drain: func() {
			// A queued perception callback may need this lock.
			ws.dataSourceMu.Lock()
			defer ws.dataSourceMu.Unlock()
			if !flushed {
				t.Error("waited before flushing the partial live frame")
			}
			drained = true
		}}
	}
	ws.onVRLogLoad = func(string) (string, error) {
		if !drained || ws.PipelineState().LiveListenerRunning {
			t.Error("published recorded frames before live callbacks drained")
		}
		return "protobuf", nil
	}
	w, req := handoverRequest(t, ws)
	ws.handleVRLogLoad(w, req)
	if w.Code != http.StatusOK || !drained {
		t.Fatalf("load: %d %s", w.Code, w.Body.String())
	}
}

func TestVRLogHandoverDrainsPacketCallbacksBeforePublishing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	ws := &Server{state: newPipelineState(), pcapCancel: cancel, pcapDone: done}
	ws.state.Source, ws.state.SourcePath, ws.state.ReplayActive = SourceModeCapture, "old.pcap", true
	ws.setRecording("old-run", "old-output")
	go func() {
		<-ctx.Done()
		// Producer completion is allowed to acquire dataSourceMu. Holding
		// it while waiting for this drain would deadlock the handover.
		ws.dataSourceMu.Lock()
		ws.dataSourceMu.Unlock()
		ws.clearRecording()
		ws.parkFinishedReplay("cancelled packet source drained")
		close(done)
	}()
	loaded := false
	ws.onVRLogLoad = func(string) (string, error) {
		select {
		case <-done:
		default:
			t.Error("recorded frames started before the packet producer drained")
		}
		if state := ws.PipelineState(); state.Recording || state.LiveListenerRunning {
			t.Errorf("another producer is still active at load: %+v", state)
		}
		loaded = true
		return "protobuf", nil
	}
	w, req := handoverRequest(t, ws)
	ws.handleVRLogLoad(w, req)
	if w.Code != http.StatusOK || !loaded {
		t.Fatalf("load: %d %s", w.Code, w.Body.String())
	}
	s := ws.PipelineState()
	if s.Source != SourceModeVRLog || !s.ReplayActive || s.Recording || s.RecordingRunID != "" || s.GridPreserved {
		t.Fatalf("incoherent recording source: %+v", s)
	}
}

func TestVRLogHandoverSerialisesConcurrentStop(t *testing.T) {
	loading, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var stops atomic.Int32
	ws := &Server{state: newPipelineState(), onVRLogStop: func() { stops.Add(1) }}
	ws.onVRLogLoad = func(string) (string, error) {
		close(loading)
		<-release
		return "protobuf", nil
	}
	w, req := handoverRequest(t, ws)
	loaded := make(chan struct{})
	go func() { ws.handleVRLogLoad(w, req); close(loaded) }()
	<-loading
	go func() { _ = ws.ReturnToLive("operator stop during load"); close(stopped) }()
	select {
	case <-stopped:
		t.Error("stop finished while a later load could still publish")
	case <-time.After(20 * time.Millisecond):
	}
	if stops.Load() != 1 {
		t.Error("stop detached the publisher before the load could attach its recording")
	}
	close(release)
	<-loaded
	<-stopped
	if w.Code != http.StatusOK || ws.PipelineState().Source != SourceModeLive || stops.Load() != 2 {
		t.Fatalf("overlapping stop/load: status=%d state=%+v stops=%d", w.Code, ws.PipelineState(), stops.Load())
	}
}

func TestVRLogHandoverFailureAndCancelledRequest(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "load failure", true: "cancelled before switch"}[cancelled], func(t *testing.T) {
			called := false
			ws := &Server{state: newPipelineState(), onVRLogLoad: func(string) (string, error) {
				called = true
				return "", errors.New("invalid recording")
			}}
			w, req := handoverRequest(t, ws)
			want := http.StatusInternalServerError
			if cancelled {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
				want = http.StatusRequestTimeout
			}
			ws.handleVRLogLoad(w, req)
			if w.Code != want || called == cancelled || ws.PipelineState().Source != SourceModeLive {
				t.Fatalf("failure: status=%d called=%v state=%+v", w.Code, called, ws.PipelineState())
			}
		})
	}
}

func TestVRLogHandoverCancelledDuringDrainDoesNotStartRecording(t *testing.T) {
	loaded := false
	ws := &Server{state: newPipelineState(), onVRLogLoad: func(string) (string, error) {
		loaded = true
		return "protobuf", nil
	}}
	w, req := handoverRequest(t, ws)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	ws.onPCAPStopped = cancel
	ws.handleVRLogLoad(w, req.WithContext(ctx))
	if w.Code != http.StatusRequestTimeout || loaded || ws.PipelineState().Source != SourceModeLive {
		t.Fatalf("cancelled drain: status=%d loaded=%v state=%+v", w.Code, loaded, ws.PipelineState())
	}
}

func TestParkedSourceCannotStopAReplacement(t *testing.T) {
	for _, reason := range []string{"cancelled watcher", "another recording", "another source kind", "same recording restarted"} {
		t.Run(reason, func(t *testing.T) {
			var stopped bool
			ws := &Server{state: newPipelineState(), onVRLogStop: func() { stopped = true }}
			ws.setSourceVRLog("old-recording")
			ws.state.ReplayActive = false
			parked := ws.PipelineState()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch reason {
			case "cancelled watcher":
				cancel()
			case "another recording":
				ws.setSourceVRLog("new-recording")
				ws.state.ReplayActive = false
			case "another source kind":
				ws.state.Source = SourceModeCapture
			case "same recording restarted":
				ws.setSourceVRLog("old-recording")
			}
			before := ws.PipelineState()
			if err := ws.returnParkedSourceToLive(ctx, parked); err != nil || stopped || ws.PipelineState() != before {
				t.Fatalf("stale watcher changed source: err=%v stopped=%v state=%+v", err, stopped, ws.PipelineState())
			}
		})
	}
}
