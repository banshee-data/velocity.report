package server

import (
	"testing"
	"time"
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
