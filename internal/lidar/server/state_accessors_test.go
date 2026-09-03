package server

import (
	"testing"
)

// TestCurrentPCAPFileReportsPathOnlyForPCAPSource covers both arms of the
// source check. The path is shared state across every replay kind, so
// reporting it without checking the source would name a VRLOG as the current
// PCAP.
func TestCurrentPCAPFileReportsPathOnlyForPCAPSource(t *testing.T) {
	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats()})

	if got := ws.CurrentPCAPFile(); got != "" {
		t.Errorf("expected no PCAP file before any replay, got %q", got)
	}

	ws.mutateState("test", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.SourcePath = "/captures/kirk0.pcapng"
	})
	if got := ws.CurrentPCAPFile(); got != "/captures/kirk0.pcapng" {
		t.Errorf("CurrentPCAPFile() = %q, want the active PCAP path", got)
	}

	// The same path under a VRLOG source is not a PCAP file.
	ws.mutateState("test", func(s *PipelineState) {
		s.Source = SourceModeVRLog
	})
	if got := ws.CurrentPCAPFile(); got != "" {
		t.Errorf("CurrentPCAPFile() = %q during a VRLOG replay, want empty", got)
	}
}

// TestPCAPSpeedRatioReflectsPipelineState covers the speed-ratio accessor,
// which surfaces on the status page and in the visualiser's playback readout.
func TestPCAPSpeedRatioReflectsPipelineState(t *testing.T) {
	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats()})

	if got := ws.PCAPSpeedRatio(); got != 0 {
		t.Errorf("expected a zero speed ratio before any replay, got %v", got)
	}

	ws.markReplayRunning("/captures/kirk0.pcapng", "realtime", 2.5, "run-1")

	if got := ws.PCAPSpeedRatio(); got != 2.5 {
		t.Errorf("PCAPSpeedRatio() = %v, want 2.5", got)
	}
}

// TestDisableTrackPersistenceFlagIsTheServersOwnFlag checks the accessor hands
// back the server's flag rather than a copy. The tracking pipeline holds this
// pointer for the life of the run, so a copy would leave analysis replays
// writing to the production track store.
func TestDisableTrackPersistenceFlagIsTheServersOwnFlag(t *testing.T) {
	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats()})

	flag := ws.DisableTrackPersistenceFlag()
	if flag == nil {
		t.Fatal("expected a non-nil flag")
	}
	if flag.Load() {
		t.Error("expected persistence to be enabled by default")
	}

	// A write through the server must be visible through the returned pointer.
	ws.pcapDisableTrackPersistence.Store(true)
	if !flag.Load() {
		t.Error("the returned pointer does not alias the server's own flag")
	}

	// And the reverse, which is the direction the pipeline actually writes in.
	flag.Store(false)
	if ws.pcapDisableTrackPersistence.Load() {
		t.Error("a write through the returned pointer did not reach the server")
	}
}
