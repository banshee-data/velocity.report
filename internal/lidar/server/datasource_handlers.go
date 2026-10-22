package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

type replayFrameBuilder interface {
	SetBlockOnFrameChannel(block bool)
	DroppedFrames() uint64
}

type replayCallbackDrainer interface {
	FlushPendingFrames()
	WaitForCallbacks()
}

// errRecordingUnavailable fails a replay that asked for a VRLOG recording the
// server could not start, before any packets are replayed into it.
var errRecordingUnavailable = errors.New("VRLOG recording could not start (is the visualiser publisher running? start the server with --lidar-forward-mode=grpc or both); replay not run")

var (
	countPCAPPackets       = network.CountPCAPPackets
	readPCAPFile           = network.ReadPCAPFile
	readPCAPSequence       = network.ReadPCAPSequence
	readPCAPFileRealtime   = network.ReadPCAPFileRealtime
	newForegroundForwarder = network.NewForegroundForwarder
	absPath                = filepath.Abs
	statPath               = os.Stat
	getReplayFrameBuilder  = func(sensorID string) replayFrameBuilder {
		fb := l2frames.GetFrameBuilder(sensorID)
		if fb == nil {
			return nil
		}
		return fb
	}
	prepareSettledPCAPReplay = func(ws *Server, sensorID, sourcePath string) error {
		if fb := getReplayFrameBuilder(sensorID); fb != nil {
			if drainer, ok := fb.(replayCallbackDrainer); ok {
				drainer.FlushPendingFrames()
				drainer.WaitForCallbacks()
			}
		}

		mgr := l3grid.GetBackgroundManager(sensorID)
		if mgr == nil {
			return fmt.Errorf("no background manager is registered for sensor %s", sensorID)
		}
		if !mgr.IsSettlingComplete() {
			return fmt.Errorf("PCAP ended before the background grid settled")
		}
		if err := ws.resetAllState(); err != nil {
			return fmt.Errorf("reset state between PCAP passes: %w", err)
		}
		restored, err := mgr.RestoreSettledSnapshotBySourcePath(sourcePath)
		if err != nil {
			return err
		}
		if !restored {
			return fmt.Errorf("no settled background snapshot was persisted for %s", sourcePath)
		}
		return nil
	}
)

// StartLiveListener starts the live UDP listener via DataSourceManager.
func (ws *Server) StartLiveListener(ctx context.Context) error {
	return ws.dataSourceManager.StartLiveListener(ctx)
}

// StopLiveListener stops the live UDP listener via DataSourceManager.
func (ws *Server) StopLiveListener() error {
	return ws.dataSourceManager.StopLiveListener()
}

// --- ServerDataSourceOperations implementation ---

// StartLiveListenerInternal starts the UDP listener (called by RealDataSourceManager).
func (ws *Server) StartLiveListenerInternal(ctx context.Context) error {
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()
	return ws.startLiveListenerLocked()
}

// StopLiveListenerInternal stops the UDP listener (called by RealDataSourceManager).
func (ws *Server) StopLiveListenerInternal() {
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()
	ws.stopLiveListenerLocked()
}

// StartPCAPInternal starts PCAP replay (called by RealDataSourceManager).
func (ws *Server) StartPCAPInternal(pcapFile string, config ReplayConfig) error {
	return ws.startPCAPLockedWithConfig(pcapFile, config)
}

// StopPCAPInternal stops the current PCAP replay (called by RealDataSourceManager).
func (ws *Server) StopPCAPInternal() {
	ws.pcapMu.Lock()
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// --- Exported helpers for DirectBackend (in-process sweep) ---

// sweepSlotRetryDelay is how long StartPCAPForSweep waits between attempts
// to claim a replay slot that another replay holds. A variable so tests can
// exercise the wait without sleeping for it.
var sweepSlotRetryDelay = 5 * time.Second

// StartPCAPForSweep starts a PCAP replay suitable for sweep use.
// It stops the live listener, resets all state (grid, frame builder, tracker),
// and begins the replay. While another replay is running it retries up to
// maxRetries times with sweepSlotRetryDelay between attempts, then returns
// an error wrapping sweep.ErrReplaySlotBusy.
//
// Only a running replay holds the slot. A finished analysis replay leaves
// the pipeline in pcap_analysis with its grid retained for inspection, and a
// sweep takes the pipeline over from that state exactly as
// POST /api/lidar/pcap/start does: the sweep resets all state for every
// combination anyway, and releases the pipeline back to live input when it
// ends. Gating on the PCAP source instead, as this once did, made a sweep
// started after an analysis replay wait out every combination's retries and
// skip it, so the whole sweep finished with no results.
func (ws *Server) StartPCAPForSweep(pcapFile string, analysisMode bool, speedMode string, speedRatio float64,
	startSeconds, durationSeconds float64, maxRetries int, disableRecording bool) error {

	if maxRetries <= 0 {
		maxRetries = 60
	}

	for retry := 0; retry < maxRetries; retry++ {
		ws.dataSourceMu.Lock()

		// ReplayActive rather than PCAPInProgress: a VRLOG replay holds the
		// slot too, and claiming it would reset the grid under that replay
		// before tryBeginPCAPReplay refused the start.
		if ws.PipelineState().ReplayActive {
			ws.dataSourceMu.Unlock()
			if retry == 0 {
				diagf("PCAP replay in progress, waiting...")
			}
			if retry < maxRetries-1 {
				time.Sleep(sweepSlotRetryDelay)
			}
			continue
		}

		ws.stopLiveListenerLocked()

		if err := ws.resetAllState(); err != nil {
			// Try to restart live listener on failure
			if startErr := ws.startLiveListenerLocked(); startErr != nil {
				opsf("failed to restart live listener after reset error: %v", startErr)
			}
			ws.dataSourceMu.Unlock()
			return fmt.Errorf("reset state: %w", err)
		}

		if err := ws.startPCAPLockedWithConfig(pcapFile, ReplayConfig{
			StartSeconds:     startSeconds,
			DurationSeconds:  durationSeconds,
			SpeedMode:        speedMode,
			SpeedRatio:       speedRatio,
			AnalysisMode:     analysisMode,
			DisableRecording: disableRecording,
			SensorID:         ws.sensorID,
		}); err != nil {
			_ = ws.startLiveListenerLocked()
			ws.dataSourceMu.Unlock()
			return fmt.Errorf("start PCAP: %w", err)
		}

		ws.dataSourceMu.Unlock()

		if ws.onPCAPStarted != nil {
			ws.onPCAPStarted()
		}
		return nil
	}
	return fmt.Errorf("%w: a replay was still running after %d attempts; stop it via POST /api/lidar/replay/stop",
		sweep.ErrReplaySlotBusy, maxRetries)
}

// StopPCAPForSweep cancels any running PCAP replay and restores live mode.
func (ws *Server) StopPCAPForSweep() error {
	ws.dataSourceMu.Lock()
	if ws.PipelineState().Source != SourceModeCapture {
		ws.dataSourceMu.Unlock()
		return nil // not in PCAP mode: nothing to do
	}

	ws.pcapMu.Lock()
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapMu.Unlock()

	// Unlock before waiting so PCAP goroutine can finish
	ws.dataSourceMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	analysisMode := ws.PipelineState().AnalysisMode()

	if !analysisMode {
		if err := ws.resetAllState(); err != nil {
			return fmt.Errorf("reset state after stop: %w", err)
		}
	} else {
		ws.resetFrameBuilder()
	}

	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		mgr.SetSourcePath("")
		mgr.SetRegionStoreUse(true, true)
	}

	if err := ws.startLiveListenerLocked(); err != nil {
		return fmt.Errorf("restart live listener: %w", err)
	}

	// The sweep stop path resets the grid unless the replay was an analysis run.
	ws.setSourceLive(false)

	if ws.onPCAPStopped != nil {
		ws.onPCAPStopped()
	}
	return nil
}

// PCAPDone returns the current or most-recent replay's completion channel.
// A completed replay leaves a closed channel in place so callers cannot mistake
// cleanup in progress for the absence of a replay. The caller must not close it.
func (ws *Server) PCAPDone() <-chan struct{} {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.pcapDone
}

// LastRunID returns the run ID set during the most recent PCAP
// replay in analysis mode.
func (ws *Server) LastRunID() string {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.PipelineState().LastRunID
}

// ResetAllStateDirect exposes the internal resetAllState for in-process callers.
func (ws *Server) ResetAllStateDirect() error {
	return ws.resetAllState()
}

// BaseContext returns the base context for operations.
func (ws *Server) BaseContext() context.Context {
	return ws.baseContext()
}

// Ensure Server implements ServerDataSourceOperations.
var _ ServerDataSourceOperations = (*Server)(nil)

func (ws *Server) startLiveListenerLocked() error {
	if ws.udpListener != nil {
		ws.setLiveListenerRunning(true)
		return nil
	}
	baseCtx := ws.baseContext()
	if baseCtx == nil {
		return errors.New("web server base context is not initialised: check server startup sequence")
	}

	ws.udpListener = network.NewUDPListener(ws.udpListenerConfig)
	listenerCtx, cancel := context.WithCancel(baseCtx)
	ws.udpListenerCancel = cancel
	done := make(chan struct{})
	ws.udpListenerDone = done

	// Create error channel to receive startup result
	startupErr := make(chan error, 1)

	go func(listener *network.UDPListener, ctx context.Context, finished chan struct{}, errCh chan error) {
		defer close(finished)

		// listener.Start() blocks until context is cancelled or a fatal error occurs.
		// It returns immediately with an error if socket binding fails.
		err := listener.Start(ctx)
		select {
		case errCh <- err:
		default:
			// Parent already took the timeout path (startup succeeded).
			// Log post-startup runtime errors so they are not silently lost.
			if err != nil && !errors.Is(err, context.Canceled) {
				opsf("LiDAR UDP listener error: %v", err)
			}
		}
	}(ws.udpListener, listenerCtx, done, startupErr)

	// Wait for either:
	// 1. Immediate startup error (socket bind failure) - returned quickly
	// 2. Timeout if listener hangs during startup
	//
	// Note: successful socket binding means Start() will block in the read loop,
	// so we won't receive anything on startupErr channel in the success case.
	// We use a short timeout to detect startup completion.
	select {
	case err := <-startupErr:
		// Received an error from Start() - this means socket binding failed
		// or listener exited immediately for another reason
		cancel()
		<-done
		ws.udpListener = nil
		ws.udpListenerCancel = nil
		ws.udpListenerDone = nil
		ws.setLiveListenerRunning(false)
		return fmt.Errorf("failed to start UDP listener: %w", err)
	case <-time.After(500 * time.Millisecond):
		// Timeout elapsed without receiving an error.
		// This means Start() successfully bound the socket and entered the read loop.
		// The listener is now running in the background goroutine.
		ws.setLiveListenerRunning(true)
		return nil
	}
}

func (ws *Server) stopLiveListenerLocked() {
	cancel := ws.udpListenerCancel
	done := ws.udpListenerDone

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	ws.udpListener = nil
	ws.udpListenerCancel = nil
	ws.udpListenerDone = nil
	ws.setLiveListenerRunning(false)
}

func (ws *Server) resolvePCAPPath(candidate string) (string, error) {
	if candidate == "" {
		return "", &switchError{status: http.StatusBadRequest, err: errors.New("the pcap_file parameter is required")}
	}
	if ws.pcapSafeDir == "" {
		return "", &switchError{status: http.StatusInternalServerError, err: errors.New("pcap safe directory is not configured: check server startup configuration")}
	}

	safeDirAbs, err := absPath(ws.pcapSafeDir)
	if err != nil {
		return "", &switchError{status: http.StatusInternalServerError, err: fmt.Errorf("invalid PCAP safe directory configuration: %w", err)}
	}

	candidatePath := filepath.Join(safeDirAbs, candidate)
	resolvedPath := candidatePath

	canonicalPath, err := filepath.EvalSymlinks(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &switchError{status: http.StatusNotFound, err: errors.New("pcap file not found")}
		}
		return "", &switchError{status: http.StatusBadRequest, err: fmt.Errorf("cannot resolve PCAP file path: %w", err)}
	}

	relPath, err := filepath.Rel(safeDirAbs, canonicalPath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return "", &switchError{
			status: http.StatusForbidden,
			err:    fmt.Errorf("access denied: pcap_file must be within safe directory (%s)", ws.pcapSafeDir),
		}
	}

	fileInfo, err := statPath(canonicalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &switchError{status: http.StatusNotFound, err: errors.New("pcap file not found")}
		}
		return "", &switchError{status: http.StatusBadRequest, err: fmt.Errorf("cannot access PCAP file: %w", err)}
	}

	if !fileInfo.Mode().IsRegular() {
		return "", &switchError{status: http.StatusBadRequest, err: errors.New("pcap_file must be a regular file")}
	}

	ext := strings.ToLower(filepath.Ext(canonicalPath))
	if ext != ".pcap" && ext != ".pcapng" {
		return "", &switchError{status: http.StatusBadRequest, err: errors.New("pcap_file must have .pcap or .pcapng extension")}
	}

	return canonicalPath, nil
}

func (ws *Server) replayAnalysisSensorID(config ReplayConfig) string {
	if sensorID := strings.TrimSpace(config.SensorID); sensorID != "" {
		return sensorID
	}
	return ws.sensorID
}

func (ws *Server) ensureRunManager(sensorID string) *sqlite.RunManager {
	if ws.runManager != nil {
		return ws.runManager
	}
	if ws.db == nil {
		return nil
	}

	effectiveSensorID := strings.TrimSpace(sensorID)
	if effectiveSensorID == "" {
		effectiveSensorID = ws.sensorID
	}
	ws.runManager = sqlite.NewRunManager(ws.db, effectiveSensorID)
	if effectiveSensorID != "" {
		sqlite.RegisterRunManager(effectiveSensorID, ws.runManager)
	}
	return ws.runManager
}

func (ws *Server) snapshotReplayEffectiveConfig(config ReplayConfig) *cfgpkg.TuningConfig {
	sensorID := ws.replayAnalysisSensorID(config)

	var bgManager *l3grid.BackgroundManager
	if sensorID != "" {
		bgManager = l3grid.GetBackgroundManager(sensorID)
	}

	cfg := ws.runtimeTuningConfigForSource(bgManager, DataSourceCapture)
	if sensorID != "" {
		cfg.L1.Sensor = sensorID
	}
	return cfg
}

func (ws *Server) startReplayRun(sourcePath string, config ReplayConfig) (string, error) {
	manager := ws.ensureRunManager(ws.replayAnalysisSensorID(config))
	if manager == nil {
		return "", nil
	}

	return manager.StartRunWithConfig(sqlite.RunStartOptions{
		PreferredRunID:      strings.TrimSpace(config.PreferredRunID),
		SourceType:          "pcap",
		SourcePath:          sourcePath,
		SensorID:            ws.replayAnalysisSensorID(config),
		ParentRunID:         strings.TrimSpace(config.ParentRunID),
		ClipID:              strings.TrimSpace(config.ClipID),
		RequestedParamSetID: strings.TrimSpace(config.RequestedParamSetID),
		RequestedParamsJSON: config.RequestedParamsJSON,
		EffectiveConfig:     ws.snapshotReplayEffectiveConfig(config),
	})
}

func (ws *Server) failReplayRun(runID, errMsg string) {
	if runID == "" || ws.runManager == nil {
		return
	}
	if err := ws.runManager.FailRun(errMsg); err != nil {
		opsf("Warning: Failed to mark analysis run %s as failed: %v", runID, err)
	}
}

func (ws *Server) resetFailedPCAPStartState() {
	ws.pcapMu.Lock()
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapMu.Unlock()
	ws.abandonReplayStart()
}

func (ws *Server) startPCAPLocked(pcapFile string, speedMode string, speedRatio float64, startSeconds float64, durationSeconds float64, debugRingMin int, debugRingMax int, debugAzMin float32, debugAzMax float32, enableDebug bool, enablePlots bool) error {
	return ws.startPCAPLockedWithConfig(pcapFile, ReplayConfig{
		StartSeconds:    startSeconds,
		DurationSeconds: durationSeconds,
		SpeedMode:       speedMode,
		SpeedRatio:      speedRatio,
		DebugRingMin:    debugRingMin,
		DebugRingMax:    debugRingMax,
		DebugAzMin:      debugAzMin,
		DebugAzMax:      debugAzMax,
		EnableDebug:     enableDebug,
		EnablePlots:     enablePlots,
	})
}

func (ws *Server) startPCAPLockedWithConfig(pcapFile string, config ReplayConfig) error {
	replayCfg := config
	if replayCfg.SpeedRatio <= 0 {
		replayCfg.SpeedRatio = 1.0
	}

	if ok, blocker := ws.tryBeginPCAPReplay(replayCfg); !ok {
		// Name what is holding the slot, but only ever one endpoint to clear
		// it. Pointing PCAP and VRLOG callers at different stops is what
		// produced a pair of errors that referred callers to each other.
		kind := "a PCAP replay"
		if blocker == SourceModeVRLog {
			kind = "a VRLOG replay"
		}
		return &switchError{status: http.StatusConflict, err: fmt.Errorf(
			"%s is already running: stop it first via POST /api/lidar/replay/stop", kind)}
	}
	ws.pcapMu.Lock()
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapMu.Unlock()

	resolvedPath, resolveErr := ws.resolvePCAPPath(pcapFile)
	if resolveErr != nil {
		if replayCfg.AnalysisMode {
			if runID, err := ws.startReplayRun(pcapFile, replayCfg); err != nil {
				opsf("Warning: Failed to create analysis run for failed PCAP start: %v", err)
			} else if runID != "" {
				ws.failReplayRun(runID, resolveErr.Error())
			}
		}
		ws.resetFailedPCAPStartState()
		return resolveErr
	}

	// A multi-file replay is planned before anything starts, so a join too wide
	// to cross is reported to the caller instead of surfacing as a log line
	// half way through a twenty-minute replay.
	var plan *replayPlan
	if len(replayCfg.ReplayFiles) > 1 {
		builtPlan, planErr := ws.buildReplaySequence(
			replayCfg.ReplayFiles, replayCfg.StartSeconds, replayCfg.DurationSeconds)
		if planErr != nil {
			ws.resetFailedPCAPStartState()
			return planErr
		}
		plan = builtPlan
		diagf("PCAP sequence planned: %d files, %d join(s), %v lost at joins, %d revolution(s) will be dropped",
			len(plan.Steps), plan.JoinsCrossed, plan.Lost, plan.FrameDrops)
	}

	// Identify persisted region snapshots by the resolved path. Callers pass a
	// bare filename — the documented curl example and the status page form both
	// do — and the settled-snapshot restore looks up the resolved path, so
	// recording the caller's spelling here meant the two never matched and
	// settle-before-recording failed on every relative path.
	if mgr := l3grid.GetBackgroundManager(ws.replayAnalysisSensorID(replayCfg)); mgr != nil {
		mgr.SetSourcePath(resolvedPath)
		// A replay runs from its own packets: it restores no region map a
		// previous run persisted, and persists one only for its own settling
		// pass to reload. Otherwise the same capture replayed twice against
		// one database runs the first replay's regions the second time.
		mgr.SetRegionStoreUse(false, replayCfg.SettleBeforeRecording)
	}

	runID := ""
	if replayCfg.AnalysisMode {
		startRunID, err := ws.startReplayRun(resolvedPath, replayCfg)
		if err != nil {
			ws.resetFailedPCAPStartState()
			return fmt.Errorf("start analysis run: %w", err)
		}
		runID = startRunID
	}

	baseCtx := ws.baseContext()
	if baseCtx == nil {
		if runID != "" {
			ws.failReplayRun(runID, "webserver base context not initialized")
		}
		ws.resetFailedPCAPStartState()
		return &switchError{status: http.StatusInternalServerError, err: errors.New("webserver base context not initialized")}
	}

	ctx, cancel := context.WithCancel(baseCtx)
	done := make(chan struct{})
	ws.pcapMu.Lock()
	ws.pcapCancel = cancel
	ws.pcapDone = done
	ws.pcapMu.Unlock()

	// Initialize grid plotter if enabled
	if replayCfg.EnablePlots && ws.plotsBaseDir != "" {
		sensorID := ws.replayAnalysisSensorID(replayCfg)
		outputDir := l9endpoints.MakePlotOutputDir(ws.plotsBaseDir, resolvedPath)
		ws.gridPlotter = l9endpoints.NewGridPlotter(sensorID, replayCfg.DebugRingMin, replayCfg.DebugRingMax, float64(replayCfg.DebugAzMin), float64(replayCfg.DebugAzMax))
		if err := ws.gridPlotter.Start(outputDir); err != nil {
			opsf("Warning: Failed to start grid plotter: %v", err)
			ws.gridPlotter = nil
		} else {
			diagf("Grid plotter enabled, output: %s", outputDir)
		}
	}

	ws.markReplayRunning(resolvedPath, replayCfg.SpeedMode, replayCfg.SpeedRatio, runID)

	go func(path string, ctx context.Context, cancel context.CancelFunc, finished chan struct{}, replayCfg ReplayConfig, runID string, plan *replayPlan) {
		defer close(finished)
		// Release the slot claimed by tryBeginPCAPReplay on every exit path,
		// whatever the source is by then. See releaseReplaySlot for what the
		// old conditional teardown stranded.
		defer ws.releaseReplaySlot()
		defer cancel()
		sensorID := ws.replayAnalysisSensorID(replayCfg)
		diagf("Starting PCAP replay from file: %s (sensor: %s, mode: %s, ratio: %.2f)", path, sensorID, replayCfg.SpeedMode, replayCfg.SpeedRatio)

		// Disable DB track persistence during analysis replays and sweeps that
		// have recording disabled: prevents polluting the production track store.
		if replayCfg.AnalysisMode || replayCfg.DisableRecording {
			ws.pcapDisableTrackPersistence.Store(true)
			defer ws.pcapDisableTrackPersistence.Store(false)
		}

		// Configure parser to use LiDAR timestamps for PCAP replay
		// This ensures that replayed data has original timestamps, not current system time
		if p, ok := ws.parser.(interface{ SetTimestampMode(parse.TimestampMode) }); ok {
			diagf("Switching parser to TimestampModeLiDAR for PCAP replay")
			p.SetTimestampMode(parse.TimestampModeLiDAR)
			defer func() {
				diagf("Restoring parser to TimestampModeSystemTime after PCAP replay")
				p.SetTimestampMode(parse.TimestampModeSystemTime)
			}()
		}

		// Note: onPCAPStarted is called by the caller (StartPCAPForSweep or
		// handlePlayPCAP) after startPCAPLocked returns, so we do not call
		// it here to avoid double-invocation.

		// Start the packet forwarder for PCAP replay.
		// The forwarder was stopped when the live UDP listener was stopped,
		// so we need to restart it with the PCAP context to forward packets.
		if ws.packetForwarder != nil {
			ws.packetForwarder.Start(ctx)
			diagf("PacketForwarder started for PCAP replay")
		}

		// Packet totals for progress tracking and timeline display. A planned
		// sequence already holds its files' extents, so its aggregate figures
		// are used rather than re-counting the primary file, whose own count
		// would scale progress to the wrong total. A single capture takes its
		// extent from the capture index when the index can vouch for it, and
		// is counted otherwise: counting reads the whole file before the replay
		// reads it again.
		var countResult network.PCAPCountResult
		counted := false
		if plan != nil {
			countResult = network.PCAPCountResult{
				Count:            plan.TotalPackets,
				FirstTimestampNs: plan.FirstTimestampNs,
				LastTimestampNs:  plan.LastTimestampNs,
			}
			counted = true
			diagf("PCAP sequence: %d packets across %d files", countResult.Count, len(plan.Steps))
		} else if indexed, ok := ws.loadIndexedCaptures().extent(path, ws.udpPort); ok {
			countResult = network.PCAPCountResult{
				Count:            indexed.PacketCount,
				FirstTimestampNs: indexed.FirstPacket.UnixNano(),
				LastTimestampNs:  indexed.LastPacket.UnixNano(),
			}
			counted = true
			diagf("PCAP extent from the capture index: %d packets", countResult.Count)
		} else if result, countErr := countPCAPPackets(path, ws.udpPort); countErr != nil {
			opsf("Warning: failed to pre-count PCAP packets: %v (progress disabled)", countErr)
		} else {
			countResult = result
			counted = true
			diagf("PCAP pre-count: %d packets", countResult.Count)
		}
		if counted {
			ws.setReplayProgress(0, countResult.Count)
			if ws.onPCAPTimestamps != nil {
				ws.onPCAPTimestamps(countResult.FirstTimestampNs, countResult.LastTimestampNs)
			}
		}

		// Progress callback: update internal state and notify external listeners
		onProgress := func(current, total uint64) {
			ws.setReplayProgress(current, total)
			if ws.onPCAPProgress != nil {
				ws.onPCAPProgress(current, total)
			}
		}

		// runPass replays the requested window once: as a multi-file sequence
		// when one was planned, and as the single file otherwise. Both passes
		// below share it so the settling and recording passes cannot drift
		// apart in how they read.
		runPass := func(forwarder *network.PacketForwarder) error {
			if plan == nil {
				return readPCAPFile(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats,
					forwarder, replayCfg.StartSeconds, replayCfg.DurationSeconds, 0,
					countResult.Count, onProgress)
			}
			res, seqErr := readPCAPSequence(ctx, plan.Steps, network.SequenceReplayConfig{
				UDPPort:      ws.udpPort,
				Parser:       ws.parser,
				FrameBuilder: ws.frameBuilder,
				Stats:        ws.stats,
				Forwarder:    forwarder,
				OnProgress:   onProgress,
			})
			if seqErr == nil && res.FramesDropped > 0 {
				diagf("PCAP sequence: dropped %d revolution(s) straddling capture-file joins",
					res.FramesDropped)
			}
			return seqErr
		}

		var err error
		// A replay that will record checks it can before its first pass. The
		// recorder itself starts only after settling, so without this a run
		// that cannot record still reads the whole capture once to settle.
		willRecord := runID != "" && !replayCfg.DisableRecording && ws.onRecordingStart != nil
		if willRecord && ws.recordingAvailable != nil && !ws.recordingAvailable() {
			err = errRecordingUnavailable
		}

		// Enable blocking frame channel for ALL PCAP replay modes so every
		// sensor rotation is processed without silent drops. This guarantees
		// deterministic 1:1 PCAP-frame-to-VRLOG-frame mapping regardless of
		// playback speed.
		if fb := getReplayFrameBuilder(sensorID); fb != nil {
			fb.SetBlockOnFrameChannel(true)
			defer fb.SetBlockOnFrameChannel(false)
		}

		if err == nil && replayCfg.SettleBeforeRecording {
			// Name the pass. Both passes replay the same window and report the
			// same source and in-progress flag, and packet progress restarts
			// between them, so without this a progress display runs 0-100%
			// twice with nothing to say why.
			ws.setReplayPass(ReplayPassSettling)
			diagf("Starting PCAP settling pass before VRLOG recording: %s", path)
			err = runPass(nil)
			if err == nil {
				err = prepareSettledPCAPReplay(ws, sensorID, path)
			}
			if err == nil {
				if ws.stats != nil {
					ws.stats.GetAndReset()
				}
				ws.setReplayProgress(0, countResult.Count)
				ws.setReplayPass(ReplayPassRecording)
				diagf("Settled background restored from disk; starting recorded PCAP pass from the requested offset")
			}
		}

		var recordingStarted bool
		if err == nil && willRecord {
			// Publish where the recorder is writing so the status surfaces can
			// report it. Previously this was a goroutine-local bool paired with
			// a closure local in the radar command, so nothing could observe it.
			//
			// An empty path means the recorder could not start — most often no
			// visualiser publisher is configured. Reporting recording=true then
			// would be its own small lie, so only the successful case is marked.
			//
			// It also fails the run before the recorded pass, for a recorder
			// that failed although RecordingAvailable said yes (a VRLOG
			// directory it cannot create, say). The caller asked
			// for a recording, and replaying without one still ends the run
			// 'completed' with no VRLOG: a scene batch once spent forty minutes
			// a site that way, every site, before anything read the output.
			recordingPath := ws.onRecordingStart(runID)
			recordingStarted = true
			if recordingPath != "" {
				ws.setRecording(runID, recordingPath)
			} else {
				err = errRecordingUnavailable
			}
		}

		if err == nil && replayCfg.SpeedMode == "analysis" {
			err = runPass(ws.packetForwarder)
		} else if err == nil {
			// A paced replay runs the tuning file's L3 as written, as analysis
			// mode and live input do. Until October 2026 it substituted five
			// L3 values here for the replay's duration, so a paced replay ran
			// an L3 its tuning hash did not name (perf0002).
			// Realtime or scaled ratio
			multiplier := replayCfg.SpeedRatio
			if replayCfg.SpeedMode == "realtime" {
				multiplier = 1.0
			}

			// Initialise foreground forwarder
			var fgForwarder *network.ForegroundForwarder
			fgForwarder, err = newForegroundForwarder("localhost", 2370, nil)
			if err != nil {
				opsf("Warning: Failed to create foreground forwarder: %v", err)
			} else {
				fgForwarder.Start(ctx)
				defer fgForwarder.Close()
			}

			bgManager := l3grid.GetBackgroundManager(sensorID)

			// Apply debug range parameters to background manager if specified
			if bgManager != nil && (replayCfg.DebugRingMin > 0 || replayCfg.DebugRingMax > 0 || replayCfg.DebugAzMin > 0 || replayCfg.DebugAzMax > 0) {
				params := bgManager.GetParams()
				params.DebugRingMin = replayCfg.DebugRingMin
				params.DebugRingMax = replayCfg.DebugRingMax
				params.DebugAzMin = replayCfg.DebugAzMin
				params.DebugAzMax = replayCfg.DebugAzMax
				_ = bgManager.SetParams(params)
				// Enable diagnostics only if enableDebug is true
				if replayCfg.EnableDebug {
					bgManager.SetEnableDiagnostics(true)
					diagf("PCAP replay: FG_DEBUG enabled for rings[%d-%d], azimuth[%.1f-%.1f]", replayCfg.DebugRingMin, replayCfg.DebugRingMax, replayCfg.DebugAzMin, replayCfg.DebugAzMax)
				} else {
					diagf("PCAP replay: debug range configured rings[%d-%d], azimuth[%.1f-%.1f] but FG_DEBUG is OFF", replayCfg.DebugRingMin, replayCfg.DebugRingMax, replayCfg.DebugAzMin, replayCfg.DebugAzMax)
				}
			}

			// Create frame callback for grid plotting if enabled
			var onFrameCallback func(*l3grid.BackgroundManager, []l2frames.PointPolar)
			if ws.gridPlotter != nil && ws.gridPlotter.IsEnabled() {
				plotter := ws.gridPlotter // capture for closure
				onFrameCallback = func(mgr *l3grid.BackgroundManager, points []l2frames.PointPolar) {
					plotter.SampleWithPoints(mgr, points)
				}
			}

			config := network.RealtimeReplayConfig{
				SpeedMultiplier:     multiplier,
				StartSeconds:        replayCfg.StartSeconds,
				DurationSeconds:     replayCfg.DurationSeconds,
				PacketForwarder:     ws.packetForwarder,
				ForegroundForwarder: fgForwarder,
				BackgroundManager:   bgManager,
				SensorID:            sensorID,
				// Increase warmup to ~4000 packets (approx 20 frames / 2 seconds) to allow background grid to stabilize
				WarmupPackets:   4000,
				DebugRingMin:    replayCfg.DebugRingMin,
				DebugRingMax:    replayCfg.DebugRingMax,
				DebugAzMin:      replayCfg.DebugAzMin,
				DebugAzMax:      replayCfg.DebugAzMax,
				OnFrameCallback: onFrameCallback,
				TotalPackets:    countResult.Count,
				OnProgress:      onProgress,
			}

			if plan != nil {
				// A paced sequence reads the same way a paced single file
				// does, one clock shared across the joins, so the ordered
				// list plays as one stream.
				res, seqErr := readPCAPSequence(ctx, plan.Steps, network.SequenceReplayConfig{
					UDPPort:      ws.udpPort,
					Parser:       ws.parser,
					FrameBuilder: ws.frameBuilder,
					Stats:        ws.stats,
					Forwarder:    ws.packetForwarder,
					OnProgress:   onProgress,
					Paced:        config,
				})
				if seqErr == nil && res.FramesDropped > 0 {
					diagf("PCAP sequence: dropped %d revolution(s) straddling capture-file joins",
						res.FramesDropped)
				}
				err = seqErr
			} else {
				err = readPCAPFileRealtime(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats, config)
			}
		}
		if fb := getReplayFrameBuilder(sensorID); fb != nil {
			if drainer, ok := fb.(replayCallbackDrainer); ok {
				drainer.FlushPendingFrames()
				drainer.WaitForCallbacks()
			}
		}

		if err != nil && !errors.Is(err, context.Canceled) {
			opsf("PCAP replay error: %v", err)
			// Mark analysis run as failed if active
			if runID != "" && ws.runManager != nil {
				if failErr := ws.runManager.FailRun(err.Error()); failErr != nil {
					opsf("Warning: Failed to mark analysis run as failed: %v", failErr)
				}
			}
		} else {
			diagf("PCAP replay completed: %s", path)
			// Log quality summary: cumulative dropped frame count for this sensor.
			if fb := getReplayFrameBuilder(sensorID); fb != nil {
				dropped := fb.DroppedFrames()
				if dropped > 0 {
					opsf("[PCAP quality] %d frames dropped (callback queue full; cumulative across replays for this sensor)", dropped)
				}
			}
			// Complete analysis run if active
			if runID != "" && ws.runManager != nil {
				if completeErr := ws.runManager.CompleteRun(); completeErr != nil {
					opsf("Warning: Failed to complete analysis run: %v", completeErr)
				}
			}
		}

		if recordingStarted {
			ws.clearRecording()
		}
		if recordingStarted && ws.onRecordingStop != nil {
			vrlogPath := ws.onRecordingStop(runID)
			if vrlogPath != "" && ws.db != nil {
				store := sqlite.NewRunStore(ws.db)
				if updateErr := store.UpdateRunVRLogPath(runID, vrlogPath); updateErr != nil {
					opsf("Warning: Failed to update vrlog_path for run %s: %v", runID, updateErr)
				}
			} else if vrlogPath == "" {
				opsf("Warning: VRLOG recording did not produce a path for run %s", runID)
			}
		}

		// Generate plots if plotter was enabled
		if ws.gridPlotter != nil && ws.gridPlotter.IsEnabled() {
			ws.gridPlotter.Stop()
			plotCount, plotErr := ws.gridPlotter.GeneratePlots()
			if plotErr != nil {
				opsf("Warning: Failed to generate plots: %v", plotErr)
			} else if plotCount > 0 {
				diagf("Generated %d ring plots in %s", plotCount, ws.gridPlotter.GetOutputDir())
			}
		}

		ws.pcapMu.Lock()
		ws.pcapCancel = nil
		ws.pcapMu.Unlock()

		// A finished replay stays the data source until an operator says
		// otherwise. Playback stops, the last frame stays on screen, and the
		// grid the replay built is left intact for inspection.
		//
		// Deciding this automatically is what the previous designs kept getting
		// wrong. Choosing from the analysis flag stranded settle-before-recording
		// runs, which carry that flag only because the handler requires
		// analysis_mode=true so the second pass can record. Choosing from whether
		// the sensor happened to be streaming replaced that with a different
		// surprise: the pipeline could be taken back to live, and the grid reset,
		// while someone was still looking at the replay. Going live is now an
		// explicit request — POST /api/lidar/pcap/stop, which the visualiser's
		// Live toggle calls.
		ws.parkFinishedReplay("PCAP replay finished")
	}(resolvedPath, ctx, cancel, done, replayCfg, runID, plan)

	return nil
}
