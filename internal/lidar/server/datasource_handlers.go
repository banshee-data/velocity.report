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
)

type replayFrameBuilder interface {
	SetBlockOnFrameChannel(block bool)
	DroppedFrames() uint64
}

var (
	countPCAPPackets       = network.CountPCAPPackets
	readPCAPFile           = network.ReadPCAPFile
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
)

// StartLiveListener starts the live UDP listener via DataSourceManager.
func (ws *Server) StartLiveListener(ctx context.Context) error {
	return ws.dataSourceManager.StartLiveListener(ctx)
}

// StopLiveListener stops the live UDP listener via DataSourceManager.
func (ws *Server) StopLiveListener() error {
	return ws.dataSourceManager.StopLiveListener()
}

// GetCurrentSource returns the currently active data source.
func (ws *Server) GetCurrentSource() DataSource {
	return ws.dataSourceManager.CurrentSource()
}

// GetCurrentPCAPFile returns the current PCAP file being replayed.
func (ws *Server) GetCurrentPCAPFile() string {
	return ws.dataSourceManager.CurrentPCAPFile()
}

// IsPCAPInProgress returns true if PCAP replay is currently active.
func (ws *Server) IsPCAPInProgress() bool {
	return ws.dataSourceManager.IsPCAPInProgress()
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

// StartPCAPForSweep starts a PCAP replay suitable for sweep use.
// It stops the live listener, resets all state (grid, frame builder, tracker),
// and begins the replay. It retries on conflict (another PCAP in progress)
// up to maxRetries times with 5-second delays.
func (ws *Server) StartPCAPForSweep(pcapFile string, analysisMode bool, speedMode string, speedRatio float64,
	startSeconds, durationSeconds float64, maxRetries int, disableRecording bool) error {

	if maxRetries <= 0 {
		maxRetries = 60
	}

	for retry := 0; retry < maxRetries; retry++ {
		ws.dataSourceMu.Lock()

		if ws.currentSource == DataSourcePCAP || ws.currentSource == DataSourcePCAPAnalysis {
			ws.dataSourceMu.Unlock()
			if retry == 0 {
				diagf("PCAP replay in progress, waiting...")
			}
			time.Sleep(5 * time.Second)
			continue
		}

		ws.stopLiveListenerLocked()

		if err := ws.resetAllState(); err != nil {
			// Try to restart live listener on failure
			_ = ws.startLiveListenerLocked()
			ws.dataSourceMu.Unlock()
			return fmt.Errorf("reset state: %w", err)
		}

		// Set source path on BackgroundManager for region restoration
		if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
			mgr.SetSourcePath(pcapFile)
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

		ws.currentSource = DataSourcePCAP
		ws.dataSourceMu.Unlock()

		if ws.onPCAPStarted != nil {
			ws.onPCAPStarted()
		}
		return nil
	}
	return fmt.Errorf("timeout waiting for PCAP replay slot")
}

// StopPCAPForSweep cancels any running PCAP replay and restores live mode.
func (ws *Server) StopPCAPForSweep() error {
	ws.dataSourceMu.Lock()
	if ws.currentSource != DataSourcePCAP && ws.currentSource != DataSourcePCAPAnalysis {
		ws.dataSourceMu.Unlock()
		return nil // not in PCAP mode — nothing to do
	}

	ws.pcapMu.Lock()
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapDone = nil
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

	ws.pcapMu.Lock()
	analysisMode := ws.pcapAnalysisMode
	ws.pcapAnalysisMode = false
	ws.pcapMu.Unlock()

	if !analysisMode {
		if err := ws.resetAllState(); err != nil {
			return fmt.Errorf("reset state after stop: %w", err)
		}
	} else {
		ws.resetFrameBuilder()
	}

	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		mgr.SetSourcePath("")
	}

	if err := ws.startLiveListenerLocked(); err != nil {
		return fmt.Errorf("restart live listener: %w", err)
	}

	ws.currentSource = DataSourceLive
	ws.currentPCAPFile = ""

	if ws.onPCAPStopped != nil {
		ws.onPCAPStopped()
	}
	return nil
}

// PCAPDone returns a channel that is closed when the current PCAP replay
// finishes, or nil if no replay is in progress. The caller must not close
// the returned channel.
func (ws *Server) PCAPDone() <-chan struct{} {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.pcapDone
}

// LastAnalysisRunID returns the run ID set during the most recent PCAP
// replay in analysis mode.
func (ws *Server) LastAnalysisRunID() string {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.pcapLastRunID
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
		return nil
	}
	baseCtx := ws.baseContext()
	if baseCtx == nil {
		return errors.New("webserver base context not initialized")
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
		errCh <- err
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
		return fmt.Errorf("failed to start UDP listener: %w", err)
	case <-time.After(500 * time.Millisecond):
		// Timeout elapsed without receiving an error.
		// This means Start() successfully bound the socket and entered the read loop.
		// The listener is now running in the background goroutine.
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
}

func (ws *Server) resolvePCAPPath(candidate string) (string, error) {
	if candidate == "" {
		return "", &switchError{status: http.StatusBadRequest, err: errors.New("missing 'pcap_file' parameter")}
	}
	if ws.pcapSafeDir == "" {
		return "", &switchError{status: http.StatusInternalServerError, err: errors.New("pcap safe directory not configured")}
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

func (ws *Server) ensureAnalysisRunManager(sensorID string) *sqlite.AnalysisRunManager {
	if ws.analysisRunManager != nil {
		return ws.analysisRunManager
	}
	if ws.db == nil {
		return nil
	}

	effectiveSensorID := strings.TrimSpace(sensorID)
	if effectiveSensorID == "" {
		effectiveSensorID = ws.sensorID
	}
	ws.analysisRunManager = sqlite.NewAnalysisRunManager(ws.db, effectiveSensorID)
	if effectiveSensorID != "" {
		sqlite.RegisterAnalysisRunManager(effectiveSensorID, ws.analysisRunManager)
	}
	return ws.analysisRunManager
}

func (ws *Server) snapshotReplayEffectiveConfig(config ReplayConfig) *cfgpkg.TuningConfig {
	sensorID := ws.replayAnalysisSensorID(config)

	var bgManager *l3grid.BackgroundManager
	if sensorID != "" {
		bgManager = l3grid.GetBackgroundManager(sensorID)
	}

	cfg := ws.runtimeTuningConfigForSource(bgManager, DataSourcePCAP)
	if sensorID != "" {
		cfg.L1.Sensor = sensorID
	}
	return cfg
}

func (ws *Server) startReplayAnalysisRun(sourcePath string, config ReplayConfig) (string, error) {
	manager := ws.ensureAnalysisRunManager(ws.replayAnalysisSensorID(config))
	if manager == nil {
		return "", nil
	}

	return manager.StartRunWithConfig(sqlite.AnalysisRunStartOptions{
		PreferredRunID:      strings.TrimSpace(config.PreferredRunID),
		SourceType:          "pcap",
		SourcePath:          sourcePath,
		SensorID:            ws.replayAnalysisSensorID(config),
		ParentRunID:         strings.TrimSpace(config.ParentRunID),
		ReplayCaseID:        strings.TrimSpace(config.ReplayCaseID),
		RequestedParamSetID: strings.TrimSpace(config.RequestedParamSetID),
		RequestedParamsJSON: config.RequestedParamsJSON,
		EffectiveConfig:     ws.snapshotReplayEffectiveConfig(config),
	})
}

func (ws *Server) failReplayAnalysisRun(runID, errMsg string) {
	if runID == "" || ws.analysisRunManager == nil {
		return
	}
	if err := ws.analysisRunManager.FailRun(errMsg); err != nil {
		opsf("Warning: Failed to mark analysis run %s as failed: %v", runID, err)
	}
}

func (ws *Server) resetFailedPCAPStartState() {
	ws.pcapMu.Lock()
	ws.pcapInProgress = false
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapAnalysisMode = false
	ws.pcapDisableRecording = false
	ws.pcapSpeedMode = ""
	ws.pcapSpeedRatio = 0
	ws.plotsEnabled = false
	ws.pcapLastRunID = ""
	ws.pcapMu.Unlock()
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

	ws.pcapMu.Lock()
	if ws.pcapInProgress {
		ws.pcapMu.Unlock()
		return &switchError{status: http.StatusConflict, err: errors.New("pcap replay already in progress")}
	}
	ws.pcapInProgress = true
	ws.pcapAnalysisMode = replayCfg.AnalysisMode
	ws.pcapDisableRecording = replayCfg.DisableRecording
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapSpeedMode = ""
	ws.pcapSpeedRatio = 0
	ws.plotsEnabled = false
	ws.pcapLastRunID = ""
	ws.pcapMu.Unlock()

	resolvedPath, resolveErr := ws.resolvePCAPPath(pcapFile)
	if resolveErr != nil {
		if replayCfg.AnalysisMode {
			if runID, err := ws.startReplayAnalysisRun(pcapFile, replayCfg); err != nil {
				opsf("Warning: Failed to create analysis run for failed PCAP start: %v", err)
			} else if runID != "" {
				ws.failReplayAnalysisRun(runID, resolveErr.Error())
			}
		}
		ws.resetFailedPCAPStartState()
		return resolveErr
	}

	runID := ""
	if replayCfg.AnalysisMode {
		startRunID, err := ws.startReplayAnalysisRun(resolvedPath, replayCfg)
		if err != nil {
			ws.resetFailedPCAPStartState()
			return fmt.Errorf("start analysis run: %w", err)
		}
		runID = startRunID
	}

	baseCtx := ws.baseContext()
	if baseCtx == nil {
		if runID != "" {
			ws.failReplayAnalysisRun(runID, "webserver base context not initialized")
		}
		ws.resetFailedPCAPStartState()
		return &switchError{status: http.StatusInternalServerError, err: errors.New("webserver base context not initialized")}
	}

	ctx, cancel := context.WithCancel(baseCtx)
	done := make(chan struct{})
	ws.pcapMu.Lock()
	ws.pcapCancel = cancel
	ws.pcapDone = done
	ws.pcapSpeedMode = replayCfg.SpeedMode
	ws.pcapSpeedRatio = replayCfg.SpeedRatio
	ws.plotsEnabled = replayCfg.EnablePlots
	ws.pcapLastRunID = runID
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

	ws.currentPCAPFile = resolvedPath

	go func(path string, ctx context.Context, cancel context.CancelFunc, finished chan struct{}, replayCfg ReplayConfig, runID string) {
		defer close(finished)
		defer cancel()
		sensorID := ws.replayAnalysisSensorID(replayCfg)
		diagf("Starting PCAP replay from file: %s (sensor: %s, mode: %s, ratio: %.2f)", path, sensorID, replayCfg.SpeedMode, replayCfg.SpeedRatio)

		// Disable DB track persistence during analysis replays and sweeps that
		// have recording disabled — prevents polluting the production track store.
		if replayCfg.AnalysisMode || replayCfg.DisableRecording {
			ws.pcapDisableTrackPersistence.Store(true)
			defer ws.pcapDisableTrackPersistence.Store(false)
		}

		var recordingStarted bool
		if runID != "" && !replayCfg.DisableRecording && ws.onRecordingStart != nil {
			ws.onRecordingStart(runID)
			recordingStarted = true
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

		// Pre-count packets for progress tracking and timeline display.
		countResult, countErr := countPCAPPackets(path, ws.udpPort)
		if countErr != nil {
			opsf("Warning: failed to pre-count PCAP packets: %v (progress disabled)", countErr)
		} else {
			ws.pcapMu.Lock()
			ws.pcapTotalPackets = countResult.Count
			ws.pcapCurrentPacket = 0
			ws.pcapMu.Unlock()
			diagf("PCAP pre-count: %d packets", countResult.Count)
			if ws.onPCAPTimestamps != nil {
				ws.onPCAPTimestamps(countResult.FirstTimestampNs, countResult.LastTimestampNs)
			}
		}

		// Progress callback: update internal state and notify external listeners
		onProgress := func(current, total uint64) {
			ws.pcapMu.Lock()
			ws.pcapCurrentPacket = current
			ws.pcapTotalPackets = total
			ws.pcapMu.Unlock()
			if ws.onPCAPProgress != nil {
				ws.onPCAPProgress(current, total)
			}
		}

		var err error
		// Enable blocking frame channel for ALL PCAP replay modes so every
		// sensor rotation is processed without silent drops. This guarantees
		// deterministic 1:1 PCAP-frame-to-VRLOG-frame mapping regardless of
		// playback speed.
		if fb := getReplayFrameBuilder(sensorID); fb != nil {
			fb.SetBlockOnFrameChannel(true)
			defer fb.SetBlockOnFrameChannel(false)
		}
		if replayCfg.SpeedMode == "analysis" {
			err = readPCAPFile(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats, ws.packetForwarder, replayCfg.StartSeconds, replayCfg.DurationSeconds, 0, countResult.Count, onProgress)
		} else {
			// Apply PCAP-friendly background params and restore afterward.
			var restoreParams func()
			if bgManager := l3grid.GetBackgroundManager(sensorID); bgManager != nil {
				orig := bgManager.GetParams()
				tuned := orig
				tuned.SeedFromFirstObservation = true
				tuned.ClosenessSensitivityMultiplier = 2.0
				tuned.NoiseRelativeFraction = 0.02
				tuned.NeighbourConfirmationCount = 5
				tuned.SafetyMarginMetres = 0.3
				_ = bgManager.SetParams(tuned)
				restoreParams = func() { _ = bgManager.SetParams(orig) }
			}
			if restoreParams != nil {
				defer restoreParams()
			}

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

			err = readPCAPFileRealtime(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats, config)
		}

		if err != nil && !errors.Is(err, context.Canceled) {
			opsf("PCAP replay error: %v", err)
			// Mark analysis run as failed if active
			if runID != "" && ws.analysisRunManager != nil {
				if failErr := ws.analysisRunManager.FailRun(err.Error()); failErr != nil {
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
			if runID != "" && ws.analysisRunManager != nil {
				if completeErr := ws.analysisRunManager.CompleteRun(); completeErr != nil {
					opsf("Warning: Failed to complete analysis run: %v", completeErr)
				}
			}
		}

		if recordingStarted && ws.onRecordingStop != nil {
			vrlogPath := ws.onRecordingStop(runID)
			if vrlogPath != "" && ws.db != nil {
				store := sqlite.NewAnalysisRunStore(ws.db)
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
		ws.pcapInProgress = false
		ws.pcapCancel = nil
		ws.pcapDone = nil
		ws.pcapSpeedMode = ""
		ws.pcapSpeedRatio = 0.0
		ws.plotsEnabled = false
		ws.pcapMu.Unlock()

		ws.dataSourceMu.Lock()
		if ws.currentSource == DataSourcePCAP || ws.currentSource == DataSourcePCAPAnalysis {
			ws.pcapMu.Lock()
			analysisMode := ws.pcapAnalysisMode
			ws.pcapMu.Unlock()

			if analysisMode {
				// Analysis mode: keep grid intact, switch to analysis state
				ws.currentSource = DataSourcePCAPAnalysis
				diagf("[DataSource] PCAP analysis complete for sensor=%s, grid preserved for inspection", ws.sensorID)
			} else {
				// Normal mode: reset all state and return to live
				if err := ws.resetAllState(); err != nil {
					opsf("Failed to reset state after PCAP: %v", err)
				}
				if err := ws.startLiveListenerLocked(); err != nil {
					opsf("Failed to restart live listener after PCAP: %v", err)
				} else {
					ws.currentSource = DataSourceLive
					ws.currentPCAPFile = ""
					diagf("[DataSource] auto-switched to Live after PCAP for sensor=%s", ws.sensorID)

					// Notify visualiser gRPC server that replay has ended
					if ws.onPCAPStopped != nil {
						ws.onPCAPStopped()
					}
				}
			}
		}
		ws.dataSourceMu.Unlock()
	}(resolvedPath, ctx, cancel, done, replayCfg, runID)

	return nil
}
