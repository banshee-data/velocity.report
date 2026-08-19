package server

import (
	"testing"
	"time"
)

func streamingStats() *PacketStats {
	ps := NewPacketStats()
	ps.AddPacket(1024) // stamps lastPacketAt
	return ps
}

// TestReconcilerNeverInterruptsAReplay is the constraint that matters most: a
// replay is the operator asking for something other than live, and packets
// arriving mid-replay are not a request to abandon it.
func TestReconcilerNeverInterruptsAReplay(t *testing.T) {
	for _, source := range []SourceMode{SourceModePCAP, SourceModeVRLog} {
		t.Run(string(source), func(t *testing.T) {
			ws := &Server{sensorID: "sensor-reconcile", state: newPipelineState(), stats: streamingStats()}
			ws.mutateState("test-setup", func(s *PipelineState) {
				s.Source = source
				s.SourcePath = "input"
				s.ReplayActive = true
			})

			ws.reconcileLiveOnce()

			if got := ws.PipelineState(); got.Source != source || !got.ReplayActive {
				t.Errorf("reconciler disturbed a running replay: %s", got)
			}
		})
	}
}

// TestStreamingSensorReclaimsAnAnalysisHold covers the rule that live presence
// decides: a finished analysis replay holds its grid only while there is
// nothing to go back to. Once packets arrive the pipeline is handed back.
func TestStreamingSensorReclaimsAnAnalysisHold(t *testing.T) {
	ws := &Server{sensorID: "sensor-analysis-hold", state: newPipelineState(), stats: streamingStats()}
	ws.mutateState("test-setup", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.SourcePath = "a.pcapng"
		s.GridPreserved = true
		s.LiveListenerRunning = true
	})

	ws.reconcileLiveOnce()

	if got := ws.PipelineState(); got.Source != SourceModeLive {
		t.Errorf("Source = %q, want live: a streaming sensor takes precedence over grid retention (%s)", got.Source, got)
	}
}

// TestAnalysisHoldSurvivesWithoutLive is the other half: with no packets there
// is nothing to hand back to, so the hold stands and resume-live remains the
// deliberate way out.
func TestAnalysisHoldSurvivesWithoutLive(t *testing.T) {
	ws := &Server{sensorID: "sensor-no-live", state: newPipelineState(), stats: NewPacketStats()}
	ws.mutateState("test-setup", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.SourcePath = "a.pcapng"
		s.GridPreserved = true
		s.LiveListenerRunning = true
	})

	ws.reconcileLiveOnce()

	if got := ws.PipelineState(); got.Source != SourceModePCAP || !got.GridPreserved {
		t.Errorf("hold was released with no live input to return to: %s", got)
	}
}

// TestReconcilerReturnsToLiveWhenPacketsArrive covers the reported failure: the
// replay finished while the sensor was not connected, so returning to live at
// that moment had nothing to ingest. Packets arriving later must be noticed.
func TestReconcilerReturnsToLiveWhenPacketsArrive(t *testing.T) {
	ws := &Server{sensorID: "sensor-late-packets", state: newPipelineState(), stats: streamingStats()}
	ws.mutateState("test-setup", func(s *PipelineState) {
		s.Source = SourceModeVRLog
		s.SourcePath = "run/"
		s.LiveListenerRunning = true // listener is up; the source is simply stale
	})

	ws.reconcileLiveOnce()

	if got := ws.PipelineState(); got.Source != SourceModeLive {
		t.Errorf("Source = %q, want live once packets resumed: %s", got.Source, got)
	}
}

// TestReconcilerIgnoresAStaleSensor checks the window: a sensor that streamed
// once long ago is not streaming now, and must not drag the pipeline off a
// source the operator chose.
func TestReconcilerIgnoresAStaleSensor(t *testing.T) {
	ps := NewPacketStats()
	ps.mu.Lock()
	ps.lastPacketAt = time.Now().Add(-time.Hour)
	ps.mu.Unlock()

	ws := &Server{sensorID: "sensor-stale", state: newPipelineState(), stats: ps}
	ws.mutateState("test-setup", func(s *PipelineState) {
		s.Source = SourceModeVRLog
		s.SourcePath = "run/"
		s.LiveListenerRunning = true
	})

	ws.reconcileLiveOnce()

	if got := ws.PipelineState(); got.Source != SourceModeVRLog {
		t.Errorf("a sensor last seen an hour ago pulled the pipeline to live: %s", got)
	}
}

// TestSensorIsStreamingWindow pins the freshness rule itself.
func TestSensorIsStreamingWindow(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"just arrived", 0, true},
		{"inside the window", livePacketWindow / 2, true},
		{"outside the window", livePacketWindow * 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := NewPacketStats()
			ps.mu.Lock()
			ps.lastPacketAt = time.Now().Add(-tt.age)
			ps.mu.Unlock()
			ws := &Server{stats: ps}
			if got := ws.sensorIsStreaming(); got != tt.want {
				t.Errorf("sensorIsStreaming() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("no packet ever", func(t *testing.T) {
		ws := &Server{stats: NewPacketStats()}
		if ws.sensorIsStreaming() {
			t.Error("a sensor that never sent a packet reported as streaming")
		}
	})
}

// TestEndOfReplayFollowsLivePresence pins the rule that replaced the
// configuration-based decision. settle_before_recording always carries the
// analysis flag — the handler requires analysis_mode=true so the second pass
// can record a VRLOG — so reading that flag to decide whether to hold the
// pipeline stranded every recording run on pcap_analysis. Live presence decides
// instead, which is the same answer for every replay kind.
func TestEndOfReplayFollowsLivePresence(t *testing.T) {
	tests := []struct {
		name          string
		streaming     bool
		gridPreserved bool
		wantSource    SourceMode
	}{
		{"streaming sensor reclaims a plain replay", true, false, SourceModeLive},
		{"streaming sensor reclaims an analysis run", true, true, SourceModeLive},
		{"no live leaves a plain replay parked", false, false, SourceModePCAP},
		{"no live leaves an analysis run holding its grid", false, true, SourceModePCAP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewPacketStats()
			if tt.streaming {
				stats.AddPacket(1024)
			}
			ws := &Server{sensorID: "sensor-end-of-replay", state: newPipelineState(), stats: stats}
			ws.mutateState("test-setup", func(s *PipelineState) {
				s.Source = SourceModePCAP
				s.SourcePath = "a.pcapng"
				s.GridPreserved = tt.gridPreserved
				s.LiveListenerRunning = true
			})

			// Mirrors the end-of-replay teardown's decision.
			if ws.sensorIsStreaming() {
				if err := ws.returnToLive("test end of replay", false); err != nil {
					t.Fatalf("returnToLive: %v", err)
				}
			} else {
				ws.endReplay(ws.PipelineState().AnalysisMode())
			}

			got := ws.PipelineState()
			if got.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q: %s", got.Source, tt.wantSource, got)
			}
			if got.ReplayActive {
				t.Errorf("ReplayActive still set after the replay ended: %s", got)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("end of replay left an invalid state: %v (%s)", err, got)
			}
		})
	}
}

// TestReturnToLiveClearsACaptureBuiltGrid covers the double background: two
// scenes drawn at once, the capture's and the live one.
//
// The visualiser renders a single background buffer, so both scenes have to
// arrive inside one snapshot — which happens when live returns settle into a
// grid the capture already filled. Returning to live preserved that grid for
// analysis replays, and once a streaming sensor started reclaiming those
// automatically, every such fallback produced the overlay.
func TestReturnToLiveClearsACaptureBuiltGrid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*PipelineState)
	}{
		{"analysis replay retaining its grid", func(s *PipelineState) {
			s.Source = SourceModePCAP
			s.SourcePath = "a.pcapng"
			s.GridPreserved = true
		}},
		{"plain replay", func(s *PipelineState) {
			s.Source = SourceModePCAP
			s.SourcePath = "a.pcapng"
		}},
		{"vrlog replay", func(s *PipelineState) {
			s.Source = SourceModeVRLog
			s.SourcePath = "run/"
			s.ReplayActive = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := &Server{sensorID: "sensor-grid-reset", state: newPipelineState()}
			ws.mutateState("test-setup", tc.setup)

			_ = ws.ReturnToLive("test")

			got := ws.PipelineState()
			if got.Source != SourceModeLive {
				t.Errorf("Source = %q, want live", got.Source)
			}
			if got.GridPreserved {
				t.Error("a capture-built grid was carried into live; live returns would settle " +
					"into it and the next snapshot would hold both scenes")
			}
			if err := got.Validate(); err != nil {
				t.Errorf("invalid state after returning to live: %v (%s)", err, got)
			}
		})
	}
}
