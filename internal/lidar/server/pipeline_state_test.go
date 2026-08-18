package server

import (
	"sync"
	"testing"
)

func TestNewPipelineStateStartsLive(t *testing.T) {
	s := newPipelineState()
	if s.Source != SourceModeLive {
		t.Errorf("Source = %q, want %q", s.Source, SourceModeLive)
	}
	if s.TotalPasses != 1 {
		t.Errorf("TotalPasses = %d, want 1", s.TotalPasses)
	}
	if s.ReplayActive || s.Recording || s.GridPreserved {
		t.Errorf("fresh state should be idle, got %+v", s)
	}
}

func TestPipelineStateDataSourceWire(t *testing.T) {
	tests := []struct {
		name          string
		source        SourceMode
		replayActive  bool
		gridPreserved bool
		want          string
	}{
		{"live idle", SourceModeLive, false, false, "live"},
		{"live with preserved grid after resume", SourceModeLive, false, true, "live"},
		{"pcap replaying", SourceModePCAP, true, false, "pcap"},
		{"pcap replaying in analysis mode", SourceModePCAP, true, true, "pcap"},
		{"pcap finished without preservation", SourceModePCAP, false, false, "pcap"},
		{"pcap finished with grid preserved", SourceModePCAP, false, true, "pcap_analysis"},
		{"vrlog replaying", SourceModeVRLog, true, false, "vrlog"},
		{"vrlog idle", SourceModeVRLog, false, false, "vrlog"},
		{"vrlog never aliases to analysis", SourceModeVRLog, false, true, "vrlog"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := PipelineState{Source: tt.source, ReplayActive: tt.replayActive, GridPreserved: tt.gridPreserved}
			if got := s.DataSourceWire(); got != tt.want {
				t.Errorf("DataSourceWire() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPipelineStatePCAPInProgressStaysNarrow guards the sweep runner:
// Client.WaitForPCAPComplete gates on this field, so reporting true during a
// VRLOG replay would make every sweep block until its timeout.
func TestPipelineStatePCAPInProgressStaysNarrow(t *testing.T) {
	tests := []struct {
		name  string
		state PipelineState
		want  bool
	}{
		{"pcap replaying", PipelineState{Source: SourceModePCAP, ReplayActive: true}, true},
		{"pcap finished", PipelineState{Source: SourceModePCAP, ReplayActive: false}, false},
		{"vrlog replaying", PipelineState{Source: SourceModeVRLog, ReplayActive: true}, false},
		{"live", PipelineState{Source: SourceModeLive}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.PCAPInProgress(); got != tt.want {
				t.Errorf("PCAPInProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPipelineStateAnalysisMode(t *testing.T) {
	tests := []struct {
		name  string
		state PipelineState
		want  bool
	}{
		{"pcap analysis replaying", PipelineState{Source: SourceModePCAP, ReplayActive: true, GridPreserved: true}, true},
		{"pcap analysis finished", PipelineState{Source: SourceModePCAP, GridPreserved: true}, true},
		{"pcap normal replay", PipelineState{Source: SourceModePCAP, ReplayActive: true}, false},
		{"live after resume with grid kept", PipelineState{Source: SourceModeLive, GridPreserved: true}, false},
		{"vrlog", PipelineState{Source: SourceModeVRLog, GridPreserved: true}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.AnalysisMode(); got != tt.want {
				t.Errorf("AnalysisMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPipelineStateStatusLabel covers the HTML status page rendering, which
// previously fell through to "Live UDP" whenever the source was pcap_analysis.
func TestPipelineStateStatusLabel(t *testing.T) {
	tests := []struct {
		name  string
		state PipelineState
		want  string
	}{
		{"live", PipelineState{Source: SourceModeLive}, "Live UDP"},
		{"live with preserved grid", PipelineState{Source: SourceModeLive, GridPreserved: true}, "Live UDP"},
		{"pcap replaying", PipelineState{Source: SourceModePCAP, ReplayActive: true}, "PCAP Replay"},
		{"pcap analysis finished", PipelineState{Source: SourceModePCAP, GridPreserved: true}, "PCAP Analysis"},
		{"pcap finished without preservation", PipelineState{Source: SourceModePCAP}, "PCAP Analysis"},
		{"vrlog replaying", PipelineState{Source: SourceModeVRLog, ReplayActive: true}, "VRLOG Replay"},
		{"unknown source falls back to live", PipelineState{Source: SourceMode("nonsense")}, "Live UDP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.StatusLabel(); got != tt.want {
				t.Errorf("StatusLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPipelineStateSnapshotIsCopy(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.mutateState(func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.SourcePath = "/data/original.pcapng"
	})

	snap := ws.PipelineState()
	snap.Source = SourceModeVRLog
	snap.SourcePath = "/data/mutated"

	after := ws.PipelineState()
	if after.Source != SourceModePCAP {
		t.Errorf("mutating the snapshot changed server state: Source = %q", after.Source)
	}
	if after.SourcePath != "/data/original.pcapng" {
		t.Errorf("mutating the snapshot changed server state: SourcePath = %q", after.SourcePath)
	}
}

func TestPipelineStateReplayPassTransitions(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	if got := ws.PipelineState().Pass; got != ReplayPassNone {
		t.Errorf("initial Pass = %q, want %q", got, ReplayPassNone)
	}

	ws.setReplayPass(ReplayPassSettling)
	if got := ws.PipelineState().Pass; got != ReplayPassSettling {
		t.Errorf("Pass = %q, want %q", got, ReplayPassSettling)
	}

	ws.setReplayPass(ReplayPassRecording)
	if got := ws.PipelineState().Pass; got != ReplayPassRecording {
		t.Errorf("Pass = %q, want %q", got, ReplayPassRecording)
	}

	ws.setReplayPass(ReplayPassNone)
	if got := ws.PipelineState().Pass; got != ReplayPassNone {
		t.Errorf("Pass = %q, want %q", got, ReplayPassNone)
	}
}

func TestPipelineStateSetReplayProgress(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setReplayProgress(42, 1000)

	got := ws.PipelineState()
	if got.CurrentPacket != 42 || got.TotalPackets != 1000 {
		t.Errorf("progress = %d/%d, want 42/1000", got.CurrentPacket, got.TotalPackets)
	}
}

func TestPipelineStateSetRecordingFrames(t *testing.T) {
	ws := &Server{state: newPipelineState()}
	ws.setRecordingFrames(7)
	if got := ws.PipelineState().RecordingFrames; got != 7 {
		t.Errorf("RecordingFrames = %d, want 7", got)
	}
}

// TestPipelineStateConcurrentProgressAndSnapshot exercises the state lock under
// -race. The whole point of the single store is that a reader sees a coherent
// snapshot rather than fields captured under two different mutexes.
func TestPipelineStateConcurrentProgressAndSnapshot(t *testing.T) {
	ws := &Server{state: newPipelineState()}

	var wg sync.WaitGroup
	const iterations = 500

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ws.setReplayProgress(uint64(i), iterations)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ws.mutateState(func(s *PipelineState) {
				s.Source = SourceModePCAP
				s.ReplayActive = true
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			snap := ws.PipelineState()
			// A snapshot must be internally consistent: whenever the source is
			// PCAP with a replay running, the derived views must agree.
			if snap.Source == SourceModePCAP && snap.ReplayActive && !snap.PCAPInProgress() {
				t.Errorf("incoherent snapshot: %+v", snap)
				return
			}
		}
	}()

	wg.Wait()
}
