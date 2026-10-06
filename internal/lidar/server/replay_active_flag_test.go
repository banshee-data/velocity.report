package server

import "testing"

// The tracking pipeline's frame-rate throttle reads ReplayActive through a
// lock-free flag rather than the state lock, because it consults it once per
// frame. The flag is a projection of PipelineState, so every path that writes
// the state has to publish it — including tryBeginPCAPReplay, which does its
// own compare-and-set instead of going through mutateState and is exactly the
// transition that starts a replay.

func TestReplayActiveFlagFollowsPCAPReplay(t *testing.T) {
	ws := &Server{state: newPipelineState()}

	if ws.ReplayActiveFlag().Load() {
		t.Fatal("flag set before any replay started")
	}

	ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{})
	if !ok {
		t.Fatal("could not claim the replay slot")
	}
	if !ws.ReplayActiveFlag().Load() {
		t.Error("flag not set after a PCAP replay began; the throttle would stay off during replay catch-up")
	}

	ws.endReplay(false)
	if ws.ReplayActiveFlag().Load() {
		t.Error("flag still set after the replay ended; live frames would be throttled")
	}
}

func TestReplayActiveFlagFollowsVRLogReplay(t *testing.T) {
	ws := &Server{state: newPipelineState()}

	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")
	if !ws.ReplayActiveFlag().Load() {
		t.Error("flag not set during a VRLOG replay")
	}

	ws.setSourceLive(false)
	if ws.ReplayActiveFlag().Load() {
		t.Error("flag still set after returning to live")
	}
}

// The teardown path releases the slot and claims live in one mutation; the
// flag must clear with it.
func TestReplayActiveFlagClearsOnReturnToLive(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	ws.endReplayAndClaimLive()

	if ws.ReplayActiveFlag().Load() {
		t.Error("flag still set after endReplayAndClaimLive; live input would be throttled")
	}
}

func TestReplayActiveFlagClearsWhenAReplayStartFails(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{}); !ok {
		t.Fatal("could not claim the replay slot")
	}

	ws.abandonReplayStart()

	if ws.ReplayActiveFlag().Load() {
		t.Error("flag still set after an abandoned replay start; live input would be throttled indefinitely")
	}
}
