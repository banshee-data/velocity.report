package server

import "testing"

// A teardown must never be observable as "a replay is the source and nothing is
// replaying". That combination is exactly what reconcileLiveOnce treats as an
// idle pipeline that live packets should reclaim, so a teardown already under
// way would be joined by a second, redundant one. On 2026-08-26 the window was
// 512ms wide — the live listener's bind wait — and the reconciler fired
// "returning to live (live packets arrived while idle)" into a teardown that
// had already stopped the replay.

func TestEndReplayAndClaimLiveLeavesNoIdleReplayWindow(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	before := ws.PipelineState()
	if before.Source != SourceModeVRLog || !before.ReplayActive {
		t.Fatalf("setup: want an active VRLOG replay, got %+v", before)
	}

	ws.endReplayAndClaimLive()

	got := ws.PipelineState()
	if got.ReplayActive {
		t.Error("ReplayActive still set after the replay ended")
	}
	if got.Source != SourceModeLive {
		t.Errorf("Source = %q, want %q: the source must not still name the replay once the slot is free",
			got.Source, SourceModeLive)
	}
	if got.SourcePath != "" {
		t.Errorf("SourcePath = %q, want empty", got.SourcePath)
	}
}

// TestReconcilerDoesNotFireOnAFreshlyEndedReplay drives the reconciler against
// the state a teardown leaves behind. It must find nothing to do.
func TestReconcilerDoesNotFireOnAFreshlyEndedReplay(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")
	ws.endReplayAndClaimLive()

	state := ws.PipelineState()

	// This mirrors reconcileLiveOnce's decision: a replay still owns the
	// pipeline, or the source names something other than live while packets
	// are arriving.
	if state.ReplayActive {
		t.Fatal("sanity: the replay slot should be free")
	}
	if state.Source != SourceModeLive {
		t.Errorf("the reconciler would fire a redundant ReturnToLive: source=%q with no replay active",
			state.Source)
	}
}

// The intermediate state must not claim the listener is up: it has not been
// started at that point, and the reconciler relies on the flag to restart it.
func TestEndReplayAndClaimLiveDoesNotClaimTheListener(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	ws.endReplayAndClaimLive()

	if ws.PipelineState().LiveListenerRunning {
		t.Error("LiveListenerRunning set before the listener was started; the reconciler would skip the restart")
	}
}
