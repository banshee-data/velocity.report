package server

import (
	"context"
	"time"
)

const (
	// liveReconcileInterval is how often the pipeline is checked against what
	// the sensor is actually doing.
	liveReconcileInterval = 5 * time.Second

	// livePacketWindow is how recently a packet must have arrived for the
	// sensor to count as streaming. Comfortably longer than a Pandar40P frame
	// so a momentary gap is not read as the sensor going away.
	livePacketWindow = 3 * time.Second
)

// runLiveReconciler periodically reconciles the pipeline with reality: if the
// sensor is streaming and nothing is replaying, the pipeline should be live.
//
// Returning to live only at the end of a replay is not enough. When a replay
// finishes before the sensor is connected there is nothing to ingest, and
// without this the pipeline stays that way indefinitely — the packets start
// arriving later and nothing notices, because the only code that reconsidered
// the source had already run.
//
// It never interrupts a replay. A replay is the operator asking for something
// other than live, and live packets arriving mid-replay are not a request to
// abandon it; the reconciler waits until the replay is over.
func (ws *Server) runLiveReconciler(ctx context.Context) {
	ticker := time.NewTicker(liveReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ws.reconcileLiveOnce()
		}
	}
}

// reconcileLiveOnce performs one reconciliation pass. Split out so tests can
// drive it directly rather than waiting on a ticker.
func (ws *Server) reconcileLiveOnce() {
	state := ws.PipelineState()

	// A replay owns the pipeline until it finishes. This covers both passes of
	// a settle-before-recording run.
	if state.ReplayActive {
		return
	}

	// A finished analysis replay is parked deliberately: it retains its grid so
	// the operator can inspect it, and POST /api/lidar/pcap/resume_live is how
	// they leave. Snapping it back to live because the sensor happens to be
	// streaming would discard the thing they asked to keep.
	if state.Source == SourceModePCAP && state.GridPreserved {
		return
	}

	streaming := ws.sensorIsStreaming()

	// Nothing is replaying, so the listener should be up regardless of whether
	// packets have arrived yet — otherwise there is nothing to notice them
	// with. This is the case a replay that ended before the sensor was
	// connected leaves behind.
	if !state.LiveListenerRunning {
		ws.dataSourceMu.Lock()
		err := ws.startLiveListenerLocked()
		ws.dataSourceMu.Unlock()
		if err != nil {
			diagf("[DataSource] live listener not yet available: %v", err)
			return
		}
		diagf("[DataSource] live listener started by reconciler for sensor=%s", ws.sensorID)
	}

	// Packets are arriving but the pipeline still names a replay as its source.
	if streaming && state.Source != SourceModeLive {
		if err := ws.ReturnToLive("live packets arrived while idle"); err != nil {
			opsf("[DataSource] could not return to live after packets resumed: %v", err)
		}
	}
}

// sensorIsStreaming reports whether a packet has arrived recently enough to
// treat the sensor as live.
func (ws *Server) sensorIsStreaming() bool {
	if ws.stats == nil {
		return false
	}
	last := ws.stats.LastPacketAt()
	if last.IsZero() {
		return false
	}
	return time.Since(last) <= livePacketWindow
}
