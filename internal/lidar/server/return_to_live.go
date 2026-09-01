package server

import (
	"context"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// parkedLiveWatchInterval is how often a parked replay checks for live input.
// Coarse on purpose: this decides whether to change the data source, not
// anything a frame depends on.
const parkedLiveWatchInterval = 500 * time.Millisecond

// parkFinishedReplay records that a replay has run to its end without changing
// the data source.
//
// The replay slot is released so another replay can start, but the source keeps
// naming the recording, the live listener stays down, and the grid is left
// alone. The visualiser therefore holds the final frame and its Live toggle
// stays off until an operator turns it back on, which is the only thing that
// returns the pipeline to live input.
// ParkFinishedReplay is the exported form, for the replay-ended callback the
// radar command wires into the visualiser publisher.
func (ws *Server) ParkFinishedReplay(reason string) { ws.parkFinishedReplay(reason) }

func (ws *Server) parkFinishedReplay(reason string) {
	ws.mutateState("parkFinishedReplay", func(s *PipelineState) {
		s.ReplayActive = false
		s.Pass = ReplayPassNone
		s.TotalPasses = 1
		s.SpeedMode = ""
		s.SpeedRatio = 0
		s.Recording = false
	})
	diagf("[DataSource] %s for sensor=%s; holding this source and watching for live input", reason, ws.sensorID)
	ws.watchForLiveWhileParked()
}

// watchForLiveWhileParked brings the live listener back up under a parked
// replay and hands the pipeline to live input as soon as a packet arrives.
//
// Parking keeps the recording on screen, which is what an operator wants when
// there is nothing else to show. It should not outlast the sensor being quiet:
// a replay loaded yesterday was still the source this morning with the sensor
// streaming, because nothing was watching. The listener has to run for a packet
// to be seen at all, so it is started here rather than waited for.
//
// The recording stays the source until a packet actually arrives, so the last
// frame is held rather than replaced by an empty grid.
func (ws *Server) watchForLiveWhileParked() {
	baseCtx := ws.baseContext()
	if baseCtx == nil || ws.stats == nil {
		return
	}

	ws.pcapMu.Lock()
	if ws.parkedLiveWatchCancel != nil {
		ws.parkedLiveWatchCancel()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	ws.parkedLiveWatchCancel = cancel
	if err := ws.startLiveListenerLocked(); err != nil {
		ws.pcapMu.Unlock()
		opsf("[DataSource] could not start the live listener under a parked replay: %v", err)
		cancel()
		return
	}
	ws.pcapMu.Unlock()

	// Only packets that arrive after parking count. LastPacketAt survives a
	// replay, so an older timestamp says the sensor was streaming before, not
	// that it is streaming now.
	parkedAt := time.Now()

	go func() {
		ticker := time.NewTicker(parkedLiveWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !ws.stats.LastPacketAt().After(parkedAt) {
					continue
				}
				// Re-check under the current state: an operator may have gone
				// live or loaded another replay while this was waiting.
				if ws.PipelineState().Source == SourceModeLive {
					return
				}
				diagf("[DataSource] live input resumed under a parked replay for sensor=%s; taking it live", ws.sensorID)
				if err := ws.returnToLive("live input resumed under a parked replay", true); err != nil {
					opsf("[DataSource] failed to take live input under a parked replay: %v", err)
				}
				return
			}
		}
	}()
}

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
	return ws.returnToLive(reason, true)
}

// returnToLive does the work. stopActiveReplay must be false when the caller is
// the replay goroutine tearing itself down: pcapDone only closes when that
// goroutine exits, so waiting on it from inside would deadlock the replay
// against itself. Every other caller passes true and waits for the replay to
// finish before the pipeline is declared live.
func (ws *Server) returnToLive(reason string, stopActiveReplay bool) error {
	before := ws.PipelineState()

	// Whatever brought us here supersedes a parked replay's wait for live.
	ws.cancelParkedLiveWatch()
	diagf("[DataSource] returning to live (%s) from %s", reason, before)

	// Stop a VRLOG replay first. This calls into l9endpoints, so it must happen
	// with no lock held: StopVRLogReplay waits on the replay goroutine.
	//
	// Called unconditionally rather than only when the source reads as vrlog.
	// The operation is idempotent, and gating it on the state we are trying to
	// repair is what let a replay keep running behind a source that said
	// otherwise.
	if stopActiveReplay && ws.onVRLogStop != nil {
		ws.onVRLogStop()
	}

	// Cancel a running PCAP replay and wait for its goroutine, which releases
	// the replay slot on its way out.
	if stopActiveReplay {
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
	}

	// Whatever the replay left behind, the slot is free now. The PCAP goroutine
	// releases it too; doing it here as well covers a VRLOG replay and a slot
	// stranded by an earlier failure, and the operation is idempotent.
	// Release the slot and name live as the source together. Doing them apart
	// leaves "source is a replay, nothing is replaying" visible for the width of
	// the teardown, which is what reconcileLiveOnce fires on — see
	// endReplayAndClaimLive.
	ws.endReplayAndClaimLive()

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	// The grid is always reset here, analysis replays included. This path exists
	// to start ingesting live data, and live returns settle into whatever cells
	// the grid already holds: keeping a grid built from a capture means the next
	// snapshot carries the recorded scene and the live one together, which
	// renders as two overlaid backgrounds.
	//
	// Retention is not lost, it just belongs to the parked state rather than to
	// going live. A finished analysis replay keeps its grid for as long as it
	// stays parked, and POST /api/lidar/pcap/resume_live is the deliberate way
	// to go live while keeping it — that handler starts the listener without
	// resetting, and is the only place where the two scenes are asked for.
	var resetErr error
	if err := ws.resetAllState(); err != nil {
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

	// The source was already set live above, before the listener work, so the
	// reconciler could never observe the intermediate state. This keeps the
	// remaining reset (packet counters, grid retention) in one place.
	ws.setSourceLive(false)
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

// cancelParkedLiveWatch stops any watcher waiting to hand a parked replay over
// to live input. Called whenever something else decides the source, so a
// watcher armed under an earlier replay cannot fire under a later one.
func (ws *Server) cancelParkedLiveWatch() {
	ws.pcapMu.Lock()
	cancel := ws.parkedLiveWatchCancel
	ws.parkedLiveWatchCancel = nil
	ws.pcapMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
