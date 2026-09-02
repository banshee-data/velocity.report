package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Parking keeps a finished replay's last frame on screen, which is what an
// operator wants when there is nothing else to show. It should not outlast the
// sensor being quiet: a replay loaded yesterday was still the source this
// morning with the sensor streaming, because nothing was watching.
//
// The live listener has to run for a packet to be seen at all, so parking
// starts it, and the recording stays the source until a packet actually
// arrives.

func TestCancelParkedLiveWatchIsSafeWhenNoneIsArmed(t *testing.T) {
	ws := &Server{}
	ws.cancelParkedLiveWatch() // must not panic
}

func TestCancelParkedLiveWatchClearsTheWatcher(t *testing.T) {
	ws := &Server{}
	cancelled := false
	ws.parkedLiveWatchCancel = func() { cancelled = true }

	ws.cancelParkedLiveWatch()

	if !cancelled {
		t.Error("the armed watcher was not cancelled")
	}
	if ws.parkedLiveWatchCancel != nil {
		t.Error("the cancel function was left in place; a second call would fire it again")
	}
}

// A new replay supersedes a parked one, so its watcher must not survive to
// take the pipeline live under the replay that replaced it.
func TestStartingAReplayCancelsTheParkedWatch(t *testing.T) {
	ws := &Server{}
	ws.state.Source = SourceModeVRLog

	for _, tt := range []struct {
		name  string
		start func()
	}{
		{"vrlog", func() { ws.setSourceVRLog("/tmp/run") }},
		{"pcap", func() { ws.tryBeginPCAPReplay(ReplayConfig{}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ws.state.ReplayActive = false
			cancelled := false
			ws.parkedLiveWatchCancel = func() { cancelled = true }

			tt.start()

			if !cancelled {
				t.Error("starting a replay left the parked watch armed")
			}
		})
	}
}

// Only packets that arrive after parking count. LastPacketAt survives a replay,
// so an older timestamp says the sensor was streaming before, not that it is
// streaming now.
func TestPacketsBeforeParkingDoNotCount(t *testing.T) {
	stats := &PacketStats{}
	stats.AddPacket(100)

	parkedAt := time.Now()

	if stats.LastPacketAt().After(parkedAt) {
		t.Error("a packet from before parking counted as live input resuming")
	}

	stats.AddPacket(100)
	if !stats.LastPacketAt().After(parkedAt) {
		t.Error("a packet after parking was not seen")
	}
}

func TestWatchForLiveWhileParkedRequiresContextAndStats(t *testing.T) {
	tests := []struct {
		name string
		ws   *Server
	}{
		{
			name: "base context",
			ws:   &Server{stats: NewPacketStats()},
		},
		{
			name: "packet stats",
			ws: func() *Server {
				ws := &Server{}
				ws.setBaseContext(context.Background())
				return ws
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.ws.watchForLiveWhileParked()
			if tt.ws.parkedLiveWatchCancel != nil {
				t.Error("watcher was armed without all of its prerequisites")
			}
		})
	}
}

func TestWatchForLiveWhileParkedReplacesExistingWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	previousCancelled := false
	ws := &Server{
		stats:                 NewPacketStats(),
		udpListener:           network.NewUDPListener(network.UDPListenerConfig{Address: ":0"}),
		parkedLiveWatchCancel: func() { previousCancelled = true },
	}
	ws.setBaseContext(ctx)

	ws.watchForLiveWhileParked()
	t.Cleanup(ws.cancelParkedLiveWatch)

	if !previousCancelled {
		t.Error("replacement watcher did not cancel the previous one")
	}
	if ws.parkedLiveWatchCancel == nil {
		t.Error("replacement watcher was not armed")
	}
}

func TestWatchForLiveWhileParkedLeavesReplayParkedWhenListenerFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ws := &Server{
		state:             newPipelineState(),
		stats:             NewPacketStats(),
		udpListenerConfig: network.UDPListenerConfig{Address: "not-a-udp-address"},
	}
	ws.setBaseContext(ctx)
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-listener-error")

	ws.watchForLiveWhileParked()
	t.Cleanup(ws.cancelParkedLiveWatch)

	got := ws.PipelineState()
	if got.Source != SourceModeVRLog {
		t.Errorf("listener failure changed source to %q, want vrlog", got.Source)
	}
	if got.LiveListenerRunning {
		t.Error("failed listener was reported running")
	}
}

func TestParkedWatcherDoesNotTearDownAnAlreadyLiveSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stoppedCalls atomic.Int32
	ws := &Server{
		state:       newPipelineState(),
		stats:       NewPacketStats(),
		udpListener: network.NewUDPListener(network.UDPListenerConfig{Address: ":0"}),
		onPCAPStopped: func() {
			stoppedCalls.Add(1)
		},
	}
	ws.setBaseContext(ctx)
	ws.setSourceLive(false)

	ws.watchForLiveWhileParked()
	t.Cleanup(ws.cancelParkedLiveWatch)
	ws.stats.AddPacket(100)
	time.Sleep(parkedLiveWatchInterval + 100*time.Millisecond)

	if got := ws.PipelineState().Source; got != SourceModeLive {
		t.Errorf("source = %q, want live", got)
	}
	if got := stoppedCalls.Load(); got != 0 {
		t.Errorf("already-live watcher ran replay teardown %d time(s)", got)
	}
}

func TestParkedWatcherStillClaimsLiveWhenStateResetFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const sensorID = "parked-live-reset-error"
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})
	t.Cleanup(func() {
		_ = l3grid.NewBackgroundManager(sensorID, 2, 2, l3grid.BackgroundParams{}, nil)
	})

	stopped := make(chan struct{}, 1)
	stats := NewPacketStats()
	ws := &Server{
		state:       newPipelineState(),
		sensorID:    sensorID,
		stats:       stats,
		udpListener: network.NewUDPListener(network.UDPListenerConfig{Address: ":0"}),
		onPCAPStopped: func() {
			stopped <- struct{}{}
		},
	}
	ws.setBaseContext(ctx)
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-reset-error")

	ws.watchForLiveWhileParked()
	t.Cleanup(ws.cancelParkedLiveWatch)
	stats.AddPacket(100)

	select {
	case <-stopped:
	case <-time.After(3 * parkedLiveWatchInterval):
		t.Fatal("watcher did not attempt the live handover")
	}

	if got := ws.PipelineState().Source; got != SourceModeLive {
		t.Errorf("reset failure left source at %q, want live", got)
	}
}

func TestParkedReplayHandsOverOnlyAfterANewLivePacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stats := NewPacketStats()
	stats.AddPacket(100) // packet from before the replay parked
	ws := &Server{
		state:       newPipelineState(),
		sensorID:    "parked-live-watch",
		stats:       stats,
		udpListener: network.NewUDPListener(network.UDPListenerConfig{Address: ":0"}),
	}
	ws.setBaseContext(ctx)
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	ws.ParkFinishedReplay("replay reached the end")

	parked := ws.PipelineState()
	if parked.Source != SourceModeVRLog {
		t.Fatalf("source immediately after parking = %q, want vrlog", parked.Source)
	}
	if !parked.LiveListenerRunning {
		t.Fatal("parking did not arm live input; a new sensor packet could never be observed")
	}

	// Let at least one watch interval pass. The packet recorded before parking
	// must not take the source live.
	time.Sleep(parkedLiveWatchInterval + 100*time.Millisecond)
	if got := ws.PipelineState().Source; got != SourceModeVRLog {
		t.Fatalf("old packet took the parked replay live: source=%q", got)
	}

	stats.AddPacket(100)
	deadline := time.Now().Add(3 * parkedLiveWatchInterval)
	for ws.PipelineState().Source != SourceModeLive && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	got := ws.PipelineState()
	if got.Source != SourceModeLive {
		t.Fatalf("new packet did not take the parked replay live: %+v", got)
	}
	if !got.LiveListenerRunning {
		t.Error("source is live but the listener is not reported running")
	}
}
