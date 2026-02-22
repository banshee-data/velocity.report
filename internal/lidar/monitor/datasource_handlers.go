package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// StartLiveListener starts the live UDP listener via DataSourceManager.
func (ws *WebServer) StartLiveListener(ctx context.Context) error {
	return ws.dataSourceManager.StartLiveListener(ctx)
}

// StopLiveListener stops the live UDP listener via DataSourceManager.
func (ws *WebServer) StopLiveListener() error {
	return ws.dataSourceManager.StopLiveListener()
}

// GetCurrentSource returns the currently active data source.
func (ws *WebServer) GetCurrentSource() DataSource {
	return ws.dataSourceManager.CurrentSource()
}

// GetCurrentPCAPFile returns the current PCAP file being replayed.
func (ws *WebServer) GetCurrentPCAPFile() string {
	return ws.dataSourceManager.CurrentPCAPFile()
}

// IsPCAPInProgress returns true if PCAP replay is currently active.
func (ws *WebServer) IsPCAPInProgress() bool {
	return ws.dataSourceManager.IsPCAPInProgress()
}

// --- WebServerDataSourceOperations implementation ---

// StartLiveListenerInternal starts the UDP listener (called by RealDataSourceManager).
func (ws *WebServer) StartLiveListenerInternal(ctx context.Context) error {
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()
	return ws.startLiveListenerLocked()
}

// StopLiveListenerInternal stops the UDP listener (called by RealDataSourceManager).
func (ws *WebServer) StopLiveListenerInternal() {
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()
	ws.stopLiveListenerLocked()
}

// StartPCAPInternal starts PCAP replay (called by RealDataSourceManager).
func (ws *WebServer) StartPCAPInternal(pcapFile string, config ReplayConfig) error {
	return ws.startPCAPLocked(
		pcapFile,
		config.SpeedMode,
		config.SpeedRatio,
		config.StartSeconds,
		config.DurationSeconds,
		config.DebugRingMin,
		config.DebugRingMax,
		config.DebugAzMin,
		config.DebugAzMax,
		config.EnableDebug,
		config.EnablePlots,
	)
}

// StopPCAPInternal stops the current PCAP replay (called by RealDataSourceManager).
func (ws *WebServer) StopPCAPInternal() {
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
func (ws *WebServer) StartPCAPForSweep(pcapFile string, analysisMode bool, speedMode string,
	startSeconds, durationSeconds float64, maxRetries int, disableRecording bool) error {

	if maxRetries <= 0 {
		maxRetries = 60
	}

	for retry := 0; retry < maxRetries; retry++ {
		ws.dataSourceMu.Lock()

		if ws.currentSource == DataSourcePCAP || ws.currentSource == DataSourcePCAPAnalysis {
			ws.dataSourceMu.Unlock()
			if retry == 0 {
				log.Printf("PCAP replay in progress, waiting...")
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

		if err := ws.startPCAPLocked(pcapFile, speedMode, 1.0, startSeconds, durationSeconds,
			0, 0, 0, 0, false, false); err != nil {
			_ = ws.startLiveListenerLocked()
			ws.dataSourceMu.Unlock()
			return fmt.Errorf("start PCAP: %w", err)
		}

		ws.pcapMu.Lock()
		ws.pcapAnalysisMode = analysisMode
		ws.pcapDisableRecording = disableRecording
		ws.pcapMu.Unlock()

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
func (ws *WebServer) StopPCAPForSweep() error {
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
func (ws *WebServer) PCAPDone() <-chan struct{} {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.pcapDone
}

// LastAnalysisRunID returns the run ID set during the most recent PCAP
// replay in analysis mode.
func (ws *WebServer) LastAnalysisRunID() string {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.pcapLastRunID
}

// ResetAllStateDirect exposes the internal resetAllState for in-process callers.
func (ws *WebServer) ResetAllStateDirect() error {
	return ws.resetAllState()
}

// BaseContext returns the base context for operations.
func (ws *WebServer) BaseContext() context.Context {
	return ws.baseContext()
}

// Ensure WebServer implements WebServerDataSourceOperations.
var _ WebServerDataSourceOperations = (*WebServer)(nil)

func (ws *WebServer) startLiveListenerLocked() error {
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

		// Try to send the error (whether nil or actual error) to the startup channel.
		// This will succeed only if the parent is still waiting; otherwise it's buffered or ignored.
		select {
		case errCh <- err:
		default:
			// Parent already timed out or succeeded; log if there was a runtime error
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Lidar UDP listener error: %v", err)
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
		return fmt.Errorf("failed to start UDP listener: %w", err)
	case <-time.After(500 * time.Millisecond):
		// Timeout elapsed without receiving an error.
		// This means Start() successfully bound the socket and entered the read loop.
		// The listener is now running in the background goroutine.
		return nil
	}
}

func (ws *WebServer) stopLiveListenerLocked() {
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

func (ws *WebServer) resolvePCAPPath(candidate string) (string, error) {
	if candidate == "" {
		return "", &switchError{status: http.StatusBadRequest, err: errors.New("missing 'pcap_file' parameter")}
	}
	if ws.pcapSafeDir == "" {
		return "", &switchError{status: http.StatusInternalServerError, err: errors.New("pcap safe directory not configured")}
	}

	safeDirAbs, err := filepath.Abs(ws.pcapSafeDir)
	if err != nil {
		return "", &switchError{status: http.StatusInternalServerError, err: fmt.Errorf("invalid PCAP safe directory configuration: %w", err)}
	}

	candidatePath := filepath.Join(safeDirAbs, candidate)
	resolvedPath, err := filepath.Abs(candidatePath)
	if err != nil {
		return "", &switchError{status: http.StatusBadRequest, err: fmt.Errorf("invalid pcap_file path: %w", err)}
	}

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

	fileInfo, err := os.Stat(canonicalPath)
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

func (ws *WebServer) startPCAPLocked(pcapFile string, speedMode string, speedRatio float64, startSeconds float64, durationSeconds float64, debugRingMin int, debugRingMax int, debugAzMin float32, debugAzMax float32, enableDebug bool, enablePlots bool) error {
	resolvedPath, err := ws.resolvePCAPPath(pcapFile)
	if err != nil {
		return err
	}

	baseCtx := ws.baseContext()
	if baseCtx == nil {
		return &switchError{status: http.StatusInternalServerError, err: errors.New("webserver base context not initialized")}
	}

	ws.pcapMu.Lock()
	if ws.pcapInProgress {
		ws.pcapMu.Unlock()
		return &switchError{status: http.StatusConflict, err: errors.New("pcap replay already in progress")}
	}
	ctx, cancel := context.WithCancel(baseCtx)
	done := make(chan struct{})
	ws.pcapInProgress = true
	ws.pcapCancel = cancel
	ws.pcapDone = done
	ws.plotsEnabled = enablePlots
	ws.pcapLastRunID = "" // Clear previous run ID before starting new PCAP
	ws.pcapMu.Unlock()

	// Initialize grid plotter if enabled
	if enablePlots && ws.plotsBaseDir != "" {
		outputDir := MakePlotOutputDir(ws.plotsBaseDir, resolvedPath)
		ws.gridPlotter = NewGridPlotter(ws.sensorID, debugRingMin, debugRingMax, float64(debugAzMin), float64(debugAzMax))
		if err := ws.gridPlotter.Start(outputDir); err != nil {
			log.Printf("Warning: Failed to start grid plotter: %v", err)
			ws.gridPlotter = nil
		} else {
			log.Printf("Grid plotter enabled, output: %s", outputDir)
		}
	}

	ws.currentPCAPFile = resolvedPath
	// Store the requested playback mode for UI visibility
	ws.pcapMu.Lock()
	ws.pcapSpeedMode = speedMode
	ws.pcapSpeedRatio = speedRatio
	ws.pcapMu.Unlock()

	go func(path string, ctx context.Context, cancel context.CancelFunc, finished chan struct{}) {
		defer close(finished)
		defer cancel()
		log.Printf("Starting PCAP replay from file: %s (sensor: %s, mode: %s, ratio: %.2f)", path, ws.sensorID, speedMode, speedRatio)

		// Check if we should start an analysis run (only in analysis mode)
		ws.pcapMu.Lock()
		isAnalysisMode := ws.pcapAnalysisMode
		disableRecording := ws.pcapDisableRecording
		ws.pcapMu.Unlock()

		var runID string
		var recordingStarted bool
		if isAnalysisMode && ws.analysisRunManager != nil {
			// Build run parameters from current background manager settings
			runParams := sqlite.DefaultRunParams()
			if bgManager := l3grid.GetBackgroundManager(ws.sensorID); bgManager != nil {
				runParams.Background = sqlite.FromBackgroundParams(bgManager.GetParams())
			}

			var startErr error
			runID, startErr = ws.analysisRunManager.StartRun(path, runParams)
			if startErr != nil {
				log.Printf("Warning: Failed to start analysis run: %v", startErr)
			} else if runID != "" {
				// Store the run ID so the sweep runner can retrieve it
				ws.pcapMu.Lock()
				ws.pcapLastRunID = runID
				ws.pcapMu.Unlock()

				if !disableRecording && ws.onRecordingStart != nil {
					ws.onRecordingStart(runID)
					recordingStarted = true
				}
			}
		}

		// Configure parser to use LiDAR timestamps for PCAP replay
		// This ensures that replayed data has original timestamps, not current system time
		if p, ok := ws.parser.(interface{ SetTimestampMode(parse.TimestampMode) }); ok {
			log.Printf("Switching parser to TimestampModeLiDAR for PCAP replay")
			p.SetTimestampMode(parse.TimestampModeLiDAR)
			defer func() {
				log.Printf("Restoring parser to TimestampModeSystemTime after PCAP replay")
				p.SetTimestampMode(parse.TimestampModeSystemTime)
			}()
		}

		// Belt-and-braces: ensure replay mode is set before any frames are
		// produced, even if the handler's onPCAPStarted call raced with us.
		if ws.onPCAPStarted != nil {
			ws.onPCAPStarted()
		}

		// Start the packet forwarder for PCAP replay.
		// The forwarder was stopped when the live UDP listener was stopped,
		// so we need to restart it with the PCAP context to forward packets.
		if ws.packetForwarder != nil {
			ws.packetForwarder.Start(ctx)
			log.Printf("PacketForwarder started for PCAP replay")
		}

		// Pre-count packets for progress tracking and timeline display.
		countResult, countErr := network.CountPCAPPackets(path, ws.udpPort)
		if countErr != nil {
			log.Printf("Warning: failed to pre-count PCAP packets: %v (progress disabled)", countErr)
		} else {
			ws.pcapMu.Lock()
			ws.pcapTotalPackets = countResult.Count
			ws.pcapCurrentPacket = 0
			ws.pcapMu.Unlock()
			log.Printf("PCAP pre-count: %d packets", countResult.Count)
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
		if speedMode == "fastest" {
			err = network.ReadPCAPFile(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats, ws.packetForwarder, startSeconds, durationSeconds, 0, countResult.Count, onProgress)
		} else {
			// Apply PCAP-friendly background params and restore afterward.
			var restoreParams func()
			if bgManager := l3grid.GetBackgroundManager(ws.sensorID); bgManager != nil {
				orig := bgManager.GetParams()
				tuned := orig
				tuned.SeedFromFirstObservation = true
				tuned.ClosenessSensitivityMultiplier = 2.0
				tuned.NoiseRelativeFraction = 0.02
				tuned.NeighborConfirmationCount = 5
				tuned.SafetyMarginMeters = 0.3
				_ = bgManager.SetParams(tuned)
				restoreParams = func() { _ = bgManager.SetParams(orig) }
			}
			if restoreParams != nil {
				defer restoreParams()
			}

			// Realtime or fixed ratio
			multiplier := speedRatio
			if speedMode == "realtime" {
				multiplier = 1.0
			}
			if multiplier <= 0 {
				multiplier = 1.0
			}

			// Initialise foreground forwarder
			var fgForwarder *network.ForegroundForwarder
			fgForwarder, err = network.NewForegroundForwarder("localhost", 2370, nil)
			if err != nil {
				log.Printf("Warning: Failed to create foreground forwarder: %v", err)
			} else {
				fgForwarder.Start(ctx)
				defer fgForwarder.Close()
			}

			bgManager := l3grid.GetBackgroundManager(ws.sensorID)

			// Apply debug range parameters to background manager if specified
			if bgManager != nil && (debugRingMin > 0 || debugRingMax > 0 || debugAzMin > 0 || debugAzMax > 0) {
				params := bgManager.GetParams()
				params.DebugRingMin = debugRingMin
				params.DebugRingMax = debugRingMax
				params.DebugAzMin = debugAzMin
				params.DebugAzMax = debugAzMax
				_ = bgManager.SetParams(params)
				// Enable diagnostics only if enableDebug is true
				if enableDebug {
					bgManager.SetEnableDiagnostics(true)
					log.Printf("PCAP replay: FG_DEBUG enabled for rings[%d-%d], azimuth[%.1f-%.1f]", debugRingMin, debugRingMax, debugAzMin, debugAzMax)
				} else {
					log.Printf("PCAP replay: debug range configured rings[%d-%d], azimuth[%.1f-%.1f] but FG_DEBUG is OFF", debugRingMin, debugRingMax, debugAzMin, debugAzMax)
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
				StartSeconds:        startSeconds,
				DurationSeconds:     durationSeconds,
				PacketForwarder:     ws.packetForwarder,
				ForegroundForwarder: fgForwarder,
				BackgroundManager:   bgManager,
				SensorID:            ws.sensorID,
				// Increase warmup to ~4000 packets (approx 20 frames / 2 seconds) to allow background grid to stabilize
				WarmupPackets:   4000,
				DebugRingMin:    debugRingMin,
				DebugRingMax:    debugRingMax,
				DebugAzMin:      debugAzMin,
				DebugAzMax:      debugAzMax,
				OnFrameCallback: onFrameCallback,
				TotalPackets:    countResult.Count,
				OnProgress:      onProgress,
			}

			err = network.ReadPCAPFileRealtime(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats, config)
		}

		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("PCAP replay error: %v", err)
			// Mark analysis run as failed if active
			if runID != "" && ws.analysisRunManager != nil {
				if failErr := ws.analysisRunManager.FailRun(err.Error()); failErr != nil {
					log.Printf("Warning: Failed to mark analysis run as failed: %v", failErr)
				}
			}
		} else {
			log.Printf("PCAP replay completed: %s", path)
			// Log quality summary: cumulative dropped frame count for this sensor.
			if fb := l2frames.GetFrameBuilder(ws.sensorID); fb != nil {
				dropped := fb.DroppedFrames()
				if dropped > 0 {
					log.Printf("[PCAP quality] %d frames dropped (callback queue full; cumulative across replays for this sensor)", dropped)
				}
			}
			// Complete analysis run if active
			if runID != "" && ws.analysisRunManager != nil {
				if completeErr := ws.analysisRunManager.CompleteRun(); completeErr != nil {
					log.Printf("Warning: Failed to complete analysis run: %v", completeErr)
				}
			}
		}

		if recordingStarted && ws.onRecordingStop != nil {
			vrlogPath := ws.onRecordingStop(runID)
			if vrlogPath != "" && ws.db != nil {
				store := sqlite.NewAnalysisRunStore(ws.db.DB)
				if updateErr := store.UpdateRunVRLogPath(runID, vrlogPath); updateErr != nil {
					log.Printf("Warning: Failed to update vrlog_path for run %s: %v", runID, updateErr)
				}
			} else if vrlogPath == "" {
				log.Printf("Warning: VRLOG recording did not produce a path for run %s", runID)
			}
		}

		// Generate plots if plotter was enabled
		if ws.gridPlotter != nil && ws.gridPlotter.IsEnabled() {
			ws.gridPlotter.Stop()
			plotCount, plotErr := ws.gridPlotter.GeneratePlots()
			if plotErr != nil {
				log.Printf("Warning: Failed to generate plots: %v", plotErr)
			} else if plotCount > 0 {
				log.Printf("Generated %d ring plots in %s", plotCount, ws.gridPlotter.GetOutputDir())
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
				log.Printf("[DataSource] PCAP analysis complete for sensor=%s, grid preserved for inspection", ws.sensorID)
			} else {
				// Normal mode: reset all state and return to live
				if err := ws.resetAllState(); err != nil {
					log.Printf("Failed to reset state after PCAP: %v", err)
				}
				if err := ws.startLiveListenerLocked(); err != nil {
					log.Printf("Failed to restart live listener after PCAP: %v", err)
				} else {
					ws.currentSource = DataSourceLive
					ws.currentPCAPFile = ""
					log.Printf("[DataSource] auto-switched to Live after PCAP for sensor=%s", ws.sensorID)

					// Notify visualiser gRPC server that replay has ended
					if ws.onPCAPStopped != nil {
						ws.onPCAPStopped()
					}
				}
			}
		}
		ws.dataSourceMu.Unlock()
	}(resolvedPath, ctx, cancel, done)

	return nil
}
