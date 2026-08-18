package server

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// ReturnToLive stops whatever replay is driving the pipeline and restores live
// input. It is the single teardown path: POST /pcap/stop, POST /vrlog/stop, the
// end of a PCAP replay, and the end of a VRLOG replay all arrive here.
//
// It is idempotent and never refuses on a state mismatch. The previous design
// had a stop per replay kind, each guarded by a check on the current source,
// which is what produced the wedge: with ReplayActive stranded true and Source
// not PCAP, /pcap/start answered "already in progress" while /pcap/stop
// answered "not in PCAP mode", and no request could reach the teardown that
// restarts the live listener. "Make the pipeline live" is always a legitimate
// request, so it always succeeds — from live it is simply a no-op.
//
// reason is recorded in the state trace so the log shows what triggered the
// return: an operator request, or a replay reaching its end.
func (ws *Server) ReturnToLive(reason string) error {
	before := ws.PipelineState()
	diagf("[DataSource] returning to live (%s) from %s", reason, before)

	// Stop a VRLOG replay first. This calls into l9endpoints, so it must happen
	// with no lock held: StopVRLogReplay waits on the replay goroutine.
	//
	// Called unconditionally rather than only when the source reads as vrlog.
	// The operation is idempotent, and gating it on the state we are trying to
	// repair is what let a replay keep running behind a source that said
	// otherwise.
	if ws.onVRLogStop != nil {
		ws.onVRLogStop()
	}

	// Cancel a running PCAP replay and wait for its goroutine, which releases
	// the replay slot on its way out.
	ws.pcapMu.Lock()
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	// Whatever the replay left behind, the slot is free now. The PCAP goroutine
	// releases it too; doing it here as well covers a VRLOG replay and a slot
	// stranded by an earlier failure, and the operation is idempotent.
	ws.releaseReplaySlot()

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	// Analysis replays keep their grid for inspection; everything else starts
	// clean so live data is not composited onto a recorded scene.
	preserveGrid := before.AnalysisMode()
	var resetErr error
	if preserveGrid {
		ws.resetFrameBuilder()
		diagf("[DataSource] preserving grid from PCAP analysis for sensor=%s", ws.sensorID)
	} else if err := ws.resetAllState(); err != nil {
		// Surface this — a grid that would not clear means live data composites
		// onto a recorded scene — but do not return before the source is set
		// live below. The replay really has stopped, and leaving the source
		// naming it is how the state goes stale behind a failed teardown.
		resetErr = fmt.Errorf("reset state while returning to live: %w", err)
	}

	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		mgr.SetSourcePath("")
	}

	// A listener that will not bind is a real fault, but it is a separate one
	// from the request that was made: the replay has stopped and the source is
	// live either way. Failing the stop here would leave callers unable to
	// clear a replay whenever the socket was busy, which is the wedge again in
	// a new disguise. Report it loudly instead; LiveListenerRunning stays false
	// in the state and on the status surfaces.
	if err := ws.startLiveListenerLocked(); err != nil {
		opsf("[DataSource] returned to live but the listener did not restart (%s): %v", reason, err)
	}

	ws.setSourceLive(preserveGrid)
	ws.pcapBenchmarkMode.Store(false)

	if ws.onPCAPStopped != nil {
		ws.onPCAPStopped()
	}

	if resetErr != nil {
		opsf("[DataSource] returned to live with a failed state reset (%s): %v", reason, resetErr)
		return resetErr
	}

	diagf("[DataSource] live input restored for sensor=%s (%s)", ws.sensorID, reason)
	return nil
}
