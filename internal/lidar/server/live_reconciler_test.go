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

// TestReconcilerLeavesAnalysisRetentionAlone guards the other deliberate hold:
// a finished analysis replay keeps its grid for inspection and is left via
// resume-live. Snapping it to live would discard what the operator asked to keep.
func TestReconcilerLeavesAnalysisRetentionAlone(t *testing.T) {
	ws := &Server{sensorID: "sensor-analysis-hold", state: newPipelineState(), stats: streamingStats()}
	ws.mutateState("test-setup", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.SourcePath = "a.pcapng"
		s.GridPreserved = true
	})

	ws.reconcileLiveOnce()

	if got := ws.PipelineState(); got.Source != SourceModePCAP || !got.GridPreserved {
		t.Errorf("reconciler discarded a retained analysis grid: %s", got)
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
