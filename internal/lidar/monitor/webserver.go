package monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	"github.com/banshee-data/velocity.report/internal/version"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// ParamDef defines a configuration parameter for display and editing
type ParamDef struct {
	Key    string      // JSON key
	Label  string      // Display label
	Value  interface{} // Current value
	Format string      // Printf format string (optional)
}

//go:embed status.html
var StatusHTML embed.FS

//go:embed assets/*
var EchartsAssets embed.FS

//go:embed dashboard.html
var dashboardHTML string

//go:embed regions_dashboard.html
var regionsDashboardHTML string

const echartsAssetsPrefix = "/assets/"

type DataSource string

const (
	DataSourceLive         DataSource = "live"
	DataSourcePCAP         DataSource = "pcap"
	DataSourcePCAPAnalysis DataSource = "pcap_analysis"
)

type switchError struct {
	status int
	err    error
}

func (e *switchError) Error() string { return e.err.Error() }

func (e *switchError) Unwrap() error { return e.err }

// WebServer handles the HTTP interface for monitoring LiDAR statistics
// It provides endpoints for health checks and real-time status information
type WebServer struct {
	address           string
	stats             *PacketStats
	server            *http.Server
	forwardingEnabled bool
	forwardAddr       string
	forwardPort       int
	parsingEnabled    bool
	udpPort           int
	db                *db.DB
	sensorID          string
	parser            network.Parser
	frameBuilder      network.FrameBuilder
	pcapSafeDir       string // Safe directory for PCAP file access
	packetForwarder   *network.PacketForwarder

	// UDP listener lifecycle (live data source)
	udpListenerConfig network.UDPListenerConfig
	dataSourceMu      sync.RWMutex
	currentSource     DataSource
	currentPCAPFile   string
	udpListener       *network.UDPListener
	udpListenerCancel context.CancelFunc
	udpListenerDone   chan struct{}
	baseCtxMu         sync.RWMutex
	baseCtx           context.Context

	// PCAP replay state
	pcapMu           sync.Mutex
	pcapInProgress   bool
	pcapCancel       context.CancelFunc
	pcapDone         chan struct{}
	pcapAnalysisMode bool // When true, preserve grid after PCAP completion
	pcapSpeedMode    string
	pcapSpeedRatio   float64

	// Track API for tracking endpoints
	trackAPI *TrackAPI

	// Analysis run manager for PCAP analysis mode
	analysisRunManager *lidar.AnalysisRunManager

	// Grid plotter for visualization during PCAP replay
	gridPlotter  *GridPlotter
	plotsBaseDir string // Base directory for plot output (e.g., "plots")
	plotsEnabled bool   // Whether plots are enabled for current run

	// latestFgCounts holds counts from the most recent foreground snapshot for status UI.
	fgCountsMu     sync.RWMutex
	latestFgCounts map[string]int
}

// WebServerConfig contains configuration options for the web server
type WebServerConfig struct {
	Address           string
	Stats             *PacketStats
	ForwardingEnabled bool
	ForwardAddr       string
	ForwardPort       int
	ParsingEnabled    bool
	UDPPort           int
	DB                *db.DB
	SensorID          string
	Parser            network.Parser
	FrameBuilder      network.FrameBuilder
	PCAPSafeDir       string // Safe directory for PCAP file access (restricts path traversal)
	PacketForwarder   *network.PacketForwarder
	UDPListenerConfig network.UDPListenerConfig
	PlotsBaseDir      string // Base directory for plot output (e.g., "plots")
}

// NewWebServer creates a new web server with the provided configuration
func NewWebServer(config WebServerConfig) *WebServer {
	listenerConfig := config.UDPListenerConfig
	if listenerConfig.Stats == nil {
		listenerConfig.Stats = config.Stats
	}
	if listenerConfig.Parser == nil {
		listenerConfig.Parser = config.Parser
	}
	if listenerConfig.FrameBuilder == nil {
		listenerConfig.FrameBuilder = config.FrameBuilder
	}
	if listenerConfig.DB == nil {
		listenerConfig.DB = config.DB
	}
	if listenerConfig.Forwarder == nil {
		listenerConfig.Forwarder = config.PacketForwarder
	}
	if listenerConfig.Address == "" && config.UDPPort != 0 {
		listenerConfig.Address = fmt.Sprintf(":%d", config.UDPPort)
	}

	ws := &WebServer{
		address:           config.Address,
		stats:             config.Stats,
		forwardingEnabled: config.ForwardingEnabled,
		forwardAddr:       config.ForwardAddr,
		forwardPort:       config.ForwardPort,
		parsingEnabled:    config.ParsingEnabled,
		udpPort:           config.UDPPort,
		db:                config.DB,
		sensorID:          config.SensorID,
		parser:            config.Parser,
		frameBuilder:      config.FrameBuilder,
		pcapSafeDir:       config.PCAPSafeDir,
		packetForwarder:   config.PacketForwarder,
		udpListenerConfig: listenerConfig,
		currentSource:     DataSourceLive,
		latestFgCounts:    make(map[string]int),
		plotsBaseDir:      config.PlotsBaseDir,
	}

	// Initialise TrackAPI if database is configured
	if config.DB != nil {
		ws.trackAPI = NewTrackAPI(config.DB.DB, config.SensorID)
		// Initialize AnalysisRunManager for PCAP analysis runs
		ws.analysisRunManager = lidar.NewAnalysisRunManager(config.DB.DB, config.SensorID)
		lidar.RegisterAnalysisRunManager(config.SensorID, ws.analysisRunManager)
	}

	ws.server = &http.Server{
		Addr:    ws.address,
		Handler: api.LoggingMiddleware(ws.setupRoutes()),
	}

	return ws
}

func (ws *WebServer) setBaseContext(ctx context.Context) {
	ws.baseCtxMu.Lock()
	ws.baseCtx = ctx
	ws.baseCtxMu.Unlock()
}

func (ws *WebServer) baseContext() context.Context {
	ws.baseCtxMu.RLock()
	defer ws.baseCtxMu.RUnlock()
	return ws.baseCtx
}

// updateLatestFgCounts refreshes cached foreground counts for the status UI.
func (ws *WebServer) updateLatestFgCounts(sensorID string) {
	ws.fgCountsMu.Lock()
	defer ws.fgCountsMu.Unlock()

	for k := range ws.latestFgCounts {
		delete(ws.latestFgCounts, k)
	}

	if sensorID == "" {
		return
	}

	snap := lidar.GetForegroundSnapshot(sensorID)
	if snap == nil {
		return
	}

	ws.latestFgCounts["total"] = snap.TotalPoints
	ws.latestFgCounts["foreground"] = snap.ForegroundCount
	ws.latestFgCounts["background"] = snap.BackgroundCount
}

// getLatestFgCounts returns a copy to avoid races in templates.
func (ws *WebServer) getLatestFgCounts() map[string]int {
	ws.fgCountsMu.RLock()
	defer ws.fgCountsMu.RUnlock()

	if len(ws.latestFgCounts) == 0 {
		return nil
	}

	copyMap := make(map[string]int, len(ws.latestFgCounts))
	for k, v := range ws.latestFgCounts {
		copyMap[k] = v
	}
	return copyMap
}

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
		return "", &switchError{status: http.StatusBadRequest, err: errors.New("missing 'pcap_file' in request body")}
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
		ws.pcapMu.Unlock()

		var runID string
		if isAnalysisMode && ws.analysisRunManager != nil {
			// Build run parameters from current background manager settings
			runParams := lidar.DefaultRunParams()
			if bgManager := lidar.GetBackgroundManager(ws.sensorID); bgManager != nil {
				runParams.Background = lidar.FromBackgroundParams(bgManager.GetParams())
			}

			var startErr error
			runID, startErr = ws.analysisRunManager.StartRun(path, runParams)
			if startErr != nil {
				log.Printf("Warning: Failed to start analysis run: %v", startErr)
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

		// Start the packet forwarder for PCAP replay.
		// The forwarder was stopped when the live UDP listener was stopped,
		// so we need to restart it with the PCAP context to forward packets.
		if ws.packetForwarder != nil {
			ws.packetForwarder.Start(ctx)
			log.Printf("PacketForwarder started for PCAP replay")
		}

		var err error
		if speedMode == "fastest" {
			err = network.ReadPCAPFile(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats, ws.packetForwarder, startSeconds, durationSeconds)
		} else {
			// Apply PCAP-friendly background params and restore afterward.
			var restoreParams func()
			if bgManager := lidar.GetBackgroundManager(ws.sensorID); bgManager != nil {
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

			bgManager := lidar.GetBackgroundManager(ws.sensorID)

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
			var onFrameCallback func(*lidar.BackgroundManager, []lidar.PointPolar)
			if ws.gridPlotter != nil && ws.gridPlotter.IsEnabled() {
				plotter := ws.gridPlotter // capture for closure
				onFrameCallback = func(mgr *lidar.BackgroundManager, points []lidar.PointPolar) {
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
			// Complete analysis run if active
			if runID != "" && ws.analysisRunManager != nil {
				if completeErr := ws.analysisRunManager.CompleteRun(); completeErr != nil {
					log.Printf("Warning: Failed to complete analysis run: %v", completeErr)
				}
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
				}
			}
		}
		ws.dataSourceMu.Unlock()
	}(resolvedPath, ctx, cancel, done)

	return nil
}

func (ws *WebServer) resetBackgroundGrid() error {
	mgr := lidar.GetBackgroundManager(ws.sensorID)
	if mgr == nil {
		return nil
	}
	if err := mgr.ResetGrid(); err != nil {
		return err
	}
	return nil
}

// resetFrameBuilder clears all buffered frame state to prevent stale data
// from contaminating a new data source.
func (ws *WebServer) resetFrameBuilder() {
	fb := lidar.GetFrameBuilder(ws.sensorID)
	if fb != nil {
		fb.Reset()
	}
}

// resetAllState performs a comprehensive reset of all processing state
// when switching data sources. This includes the background grid, frame
// builder, and any other stateful components.
func (ws *WebServer) resetAllState() error {
	// Reset frame builder first to discard any in-flight frames
	ws.resetFrameBuilder()

	// Reset background grid
	if err := ws.resetBackgroundGrid(); err != nil {
		return err
	}

	return nil
}

func (ws *WebServer) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Start begins the HTTP server in a goroutine and handles graceful shutdown
func (ws *WebServer) Start(ctx context.Context) error {
	ws.setBaseContext(ctx)

	ws.dataSourceMu.Lock()
	if ws.currentSource == DataSourceLive && ws.udpListener == nil {
		if err := ws.startLiveListenerLocked(); err != nil {
			ws.dataSourceMu.Unlock()
			return err
		}
	}
	ws.dataSourceMu.Unlock()

	// Start server in a goroutine so it doesn't block
	go func() {
		log.Printf("Starting HTTP server on %s", ws.address)
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	// Wait for context cancellation to shut down server
	<-ctx.Done()
	log.Println("shutting down HTTP server...")

	ws.dataSourceMu.Lock()
	if ws.udpListener != nil {
		ws.stopLiveListenerLocked()
	}
	ws.dataSourceMu.Unlock()

	ws.pcapMu.Lock()
	pcapCancel := ws.pcapCancel
	pcapDone := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapMu.Unlock()

	if pcapCancel != nil {
		pcapCancel()
	}
	if pcapDone != nil {
		<-pcapDone
	}

	// Create a shutdown context with a shorter timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := ws.server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
		// Force close the server if graceful shutdown fails
		if err := ws.server.Close(); err != nil {
			log.Printf("HTTP server force close error: %v", err)
		}
	}

	log.Printf("HTTP server routine stopped")
	return nil
}

// RegisterRoutes registers all Lidar monitor routes on the provided mux
func (ws *WebServer) RegisterRoutes(mux *http.ServeMux) {
	assetsFS, err := fs.Sub(EchartsAssets, "assets")
	if err != nil {
		log.Printf("failed to prepare echarts assets: %v", err)
		assetsFS = nil
	}

	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/api/lidar/monitor", ws.handleStatus)
	mux.HandleFunc("/api/lidar/status", ws.handleLidarStatus)
	mux.HandleFunc("/api/lidar/persist", ws.handleLidarPersist)
	mux.HandleFunc("/api/lidar/snapshot", ws.handleLidarSnapshot)
	mux.HandleFunc("/api/lidar/snapshots", ws.handleLidarSnapshots)
	mux.HandleFunc("/api/lidar/snapshots/cleanup", ws.handleLidarSnapshotsCleanup)
	mux.HandleFunc("/api/lidar/export_snapshot", ws.handleExportSnapshotASC)
	mux.HandleFunc("/api/lidar/export_next_frame", ws.handleExportNextFrameASC)
	mux.HandleFunc("/api/lidar/export_frame_sequence", ws.handleExportFrameSequenceASC)
	mux.HandleFunc("/api/lidar/export_foreground", ws.handleExportForegroundASC)
	mux.HandleFunc("/api/lidar/traffic", ws.handleTrafficStats)
	mux.HandleFunc("/api/lidar/acceptance", ws.handleAcceptanceMetrics)
	mux.HandleFunc("/api/lidar/acceptance/reset", ws.handleAcceptanceReset)
	mux.HandleFunc("/api/lidar/params", ws.handleBackgroundParams)
	mux.HandleFunc("/api/lidar/grid_status", ws.handleGridStatus)
	mux.HandleFunc("/api/lidar/grid_reset", ws.handleGridReset)
	mux.HandleFunc("/api/lidar/grid_heatmap", ws.handleGridHeatmap)
	mux.HandleFunc("/api/lidar/background/grid", ws.handleBackgroundGrid)                            // Full background grid
	mux.HandleFunc("/debug/lidar/background/regions", ws.handleBackgroundRegions)                    // Region debug info
	mux.HandleFunc("/debug/lidar/background/regions/dashboard", ws.handleBackgroundRegionsDashboard) // Region visualization
	if assetsFS != nil {
		mux.Handle(echartsAssetsPrefix, http.StripPrefix(echartsAssetsPrefix, http.FileServer(http.FS(assetsFS))))
	}
	mux.HandleFunc("/debug/lidar", ws.handleLidarDebugDashboard)
	mux.HandleFunc("/debug/lidar/background/polar", ws.handleBackgroundGridPolar)
	mux.HandleFunc("/debug/lidar/background/heatmap", ws.handleBackgroundGridHeatmapChart)
	mux.HandleFunc("/debug/lidar/foreground", ws.handleForegroundFrameChart)
	mux.HandleFunc("/debug/lidar/traffic", ws.handleTrafficChart)
	mux.HandleFunc("/debug/lidar/clusters", ws.handleClustersChart)
	mux.HandleFunc("/debug/lidar/tracks", ws.handleTracksChart)
	mux.HandleFunc("/api/lidar/data_source", ws.handleDataSource)
	mux.HandleFunc("/api/lidar/pcap/start", ws.handlePCAPStart)
	mux.HandleFunc("/api/lidar/pcap/stop", ws.handlePCAPStop)
	mux.HandleFunc("/api/lidar/pcap/resume_live", ws.handlePCAPResumeLive)

	// Register track API routes if available
	if ws.trackAPI != nil {
		ws.trackAPI.RegisterRoutes(mux)
	}
}

// setupRoutes configures the HTTP routes and handlers
func (ws *WebServer) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleStatus)
	ws.RegisterRoutes(mux)
	return mux
}

// handleBackgroundParams allows reading and updating simple background parameters
// Query params: sensor_id (required)
// GET: returns { "noise_relative": <float>, "enable_diagnostics": <bool>, "closeness_multiplier": <float>, "neighbor_confirmation_count": <int> }
// POST: accepts JSON { "noise_relative": <float>, "enable_diagnostics": <bool>, "closeness_multiplier": <float>, "neighbor_confirmation_count": <int> }
func (ws *WebServer) handleBackgroundParams(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	switch r.Method {
	case http.MethodGet:
		params := bm.GetParams()
		resp := map[string]interface{}{
			"noise_relative":                params.NoiseRelativeFraction,
			"enable_diagnostics":            bm.EnableDiagnostics,
			"closeness_multiplier":          params.ClosenessSensitivityMultiplier,
			"neighbor_confirmation_count":   params.NeighborConfirmationCount,
			"seed_from_first":               params.SeedFromFirstObservation,
			"warmup_duration_nanos":         params.WarmupDurationNanos,
			"warmup_min_frames":             params.WarmupMinFrames,
			"post_settle_update_fraction":   params.PostSettleUpdateFraction,
			"foreground_min_cluster_points": params.ForegroundMinClusterPoints,
			"foreground_dbscan_eps":         params.ForegroundDBSCANEps,
		}

		if r.URL.Query().Get("format") == "pretty" {
			w.Header().Set("Content-Type", "application/json")
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			enc.Encode(resp)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	case http.MethodPost:
		var body struct {
			NoiseRelative              *float64 `json:"noise_relative"`
			EnableDiagnostics          *bool    `json:"enable_diagnostics"`
			ClosenessMultiplier        *float64 `json:"closeness_multiplier"`
			NeighborConfirmation       *int     `json:"neighbor_confirmation_count"`
			SeedFromFirst              *bool    `json:"seed_from_first"`
			WarmupDurationNanos        *int64   `json:"warmup_duration_nanos"`
			WarmupMinFrames            *int     `json:"warmup_min_frames"`
			PostSettleUpdateFraction   *float64 `json:"post_settle_update_fraction"`
			ForegroundMinClusterPoints *int     `json:"foreground_min_cluster_points"`
			ForegroundDBSCANEps        *float64 `json:"foreground_dbscan_eps"`
		}

		// Check if this is a form submission from the status page
		contentType := r.Header.Get("Content-Type")
		if r.FormValue("config_json") != "" || contentType == "application/x-www-form-urlencoded" {
			configJSON := r.FormValue("config_json")
			if configJSON == "" {
				ws.writeJSONError(w, http.StatusBadRequest, "missing config_json form value")
				return
			}
			if err := json.Unmarshal([]byte(configJSON), &body); err != nil {
				ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON in config_json: "+err.Error())
				return
			}
		} else {
			// Standard JSON body
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
		}

		if body.NoiseRelative != nil {
			if err := bm.SetNoiseRelativeFraction(float32(*body.NoiseRelative)); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.EnableDiagnostics != nil {
			bm.SetEnableDiagnostics(*body.EnableDiagnostics)
		}
		if body.ClosenessMultiplier != nil {
			if err := bm.SetClosenessSensitivityMultiplier(float32(*body.ClosenessMultiplier)); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.NeighborConfirmation != nil {
			if err := bm.SetNeighborConfirmationCount(*body.NeighborConfirmation); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.SeedFromFirst != nil {
			if err := bm.SetSeedFromFirstObservation(*body.SeedFromFirst); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.WarmupDurationNanos != nil || body.WarmupMinFrames != nil {
			dur := bm.GetParams().WarmupDurationNanos
			if body.WarmupDurationNanos != nil {
				dur = *body.WarmupDurationNanos
			}
			frames := bm.GetParams().WarmupMinFrames
			if body.WarmupMinFrames != nil {
				frames = *body.WarmupMinFrames
			}
			if err := bm.SetWarmupParams(dur, frames); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.PostSettleUpdateFraction != nil {
			if err := bm.SetPostSettleUpdateFraction(float32(*body.PostSettleUpdateFraction)); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.ForegroundMinClusterPoints != nil || body.ForegroundDBSCANEps != nil {
			minPts := bm.GetParams().ForegroundMinClusterPoints
			if body.ForegroundMinClusterPoints != nil {
				minPts = *body.ForegroundMinClusterPoints
			}
			eps := bm.GetParams().ForegroundDBSCANEps
			if body.ForegroundDBSCANEps != nil {
				eps = float32(*body.ForegroundDBSCANEps)
			}
			if err := bm.SetForegroundClusterParams(minPts, eps); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Read back current params for confirmation
		cur := bm.GetParams()
		// Emit an info log so operators can see applied changes in the app logs
		log.Printf("[Monitor] Applied background params for sensor=%s: noise_relative=%.6f, enable_diagnostics=%v", sensorID, cur.NoiseRelativeFraction, bm.EnableDiagnostics)

		// Log D: API call timing for params with all active settings
		timestamp := time.Now().UnixNano()
		log.Printf("[API:params] sensor=%s noise_rel=%.6f closeness=%.3f neighbors=%d seed_from_first=%v warmup_ns=%d warmup_frames=%d post_settle_alpha=%.4f fg_min_pts=%d fg_eps=%.3f timestamp=%d",
			sensorID, cur.NoiseRelativeFraction, cur.ClosenessSensitivityMultiplier,
			cur.NeighborConfirmationCount, cur.SeedFromFirstObservation, cur.WarmupDurationNanos, cur.WarmupMinFrames, cur.PostSettleUpdateFraction, cur.ForegroundMinClusterPoints, cur.ForegroundDBSCANEps, timestamp)

		// If this was a form submission, redirect back to status page
		if r.FormValue("config_json") != "" {
			http.Redirect(w, r, fmt.Sprintf("/lidar/monitor?sensor_id=%s", sensorID), http.StatusSeeOther)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":                        "ok",
			"noise_relative":                cur.NoiseRelativeFraction,
			"enable_diagnostics":            bm.EnableDiagnostics,
			"closeness_multiplier":          cur.ClosenessSensitivityMultiplier,
			"neighbor_confirmation_count":   cur.NeighborConfirmationCount,
			"seed_from_first":               cur.SeedFromFirstObservation,
			"warmup_duration_nanos":         cur.WarmupDurationNanos,
			"warmup_min_frames":             cur.WarmupMinFrames,
			"post_settle_update_fraction":   cur.PostSettleUpdateFraction,
			"foreground_min_cluster_points": cur.ForegroundMinClusterPoints,
			"foreground_dbscan_eps":         cur.ForegroundDBSCANEps,
		})
		return
	default:
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}

// handleGridStatus returns simple statistics about the in-memory BackgroundGrid
// for a sensor: distribution of TimesSeenCount, number of frozen cells, and totals.
// Query params: sensor_id (required)
func (ws *WebServer) handleGridStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := lidar.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}
	status := mgr.GridStatus()
	if status == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to compute grid status")
		return
	}
	resp := status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleTrafficStats returns the latest packet/point throughput snapshot.
// Query params: sensor_id (optional; defaults to configured sensor)
func (ws *WebServer) handleTrafficStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if ws.stats == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no packet stats available")
		return
	}

	snap := ws.stats.GetLatestSnapshot()
	if snap == nil {
		snap = &StatsSnapshot{Timestamp: time.Now()}
	}

	uptime := ws.stats.GetUptime().Seconds()
	resp := map[string]interface{}{
		"packets_per_sec": snap.PacketsPerSec,
		"mb_per_sec":      snap.MBPerSec,
		"points_per_sec":  snap.PointsPerSec,
		"dropped_recent":  snap.DroppedCount,
		"parse_enabled":   snap.ParseEnabled,
		"timestamp":       snap.Timestamp.Format(time.RFC3339Nano),
		"uptime_secs":     uptime,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGridReset zeros the BackgroundGrid stats (times seen, averages, spreads)
// and acceptance counters. This is intended only for testing A/B sweeps.
// Method: POST. Query params: sensor_id (required)
func (ws *WebServer) handleGridReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := lidar.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}

	// Log C: API call timing for grid_reset
	beforeNanos := time.Now().UnixNano()

	// Reset frame builder to clear any buffered frames
	fb := lidar.GetFrameBuilder(sensorID)
	if fb != nil {
		fb.Reset()
	}

	if err := mgr.ResetGrid(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("reset error: %v", err))
		return
	}

	afterNanos := time.Now().UnixNano()
	elapsedMs := float64(afterNanos-beforeNanos) / 1e6

	log.Printf("[API:grid_reset] sensor=%s reset_duration_ms=%.3f timestamp=%d",
		sensorID, elapsedMs, afterNanos)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "sensor_id": sensorID})
}

// handleGridHeatmap returns aggregated grid metrics in coarse spatial buckets
// for visualization and analysis of filled vs settled cells.
// Query params:
//   - sensor_id (required)
//   - azimuth_bucket_deg (optional, default 3.0)
//   - settled_threshold (optional, default 5)
func (ws *WebServer) handleGridHeatmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "only GET supported")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	// Parse optional parameters
	azBucketDeg := 3.0
	if azStr := r.URL.Query().Get("azimuth_bucket_deg"); azStr != "" {
		if val, err := strconv.ParseFloat(azStr, 64); err == nil && val > 0 {
			azBucketDeg = val
		}
	}

	settledThreshold := uint32(5)
	if stStr := r.URL.Query().Get("settled_threshold"); stStr != "" {
		if val, err := strconv.ParseUint(stStr, 10, 32); err == nil {
			settledThreshold = uint32(val)
		}
	}

	heatmap := bm.GetGridHeatmap(azBucketDeg, settledThreshold)
	if heatmap == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to generate heatmap")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(heatmap)
}

func (ws *WebServer) handleDataSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed; use GET")
		return
	}

	ws.dataSourceMu.RLock()
	currentSource := ws.currentSource
	currentPCAPFile := ws.currentPCAPFile
	ws.dataSourceMu.RUnlock()

	ws.pcapMu.Lock()
	pcapInProgress := ws.pcapInProgress
	analysisMode := ws.pcapAnalysisMode
	ws.pcapMu.Unlock()

	response := map[string]interface{}{
		"status":           "ok",
		"data_source":      string(currentSource),
		"pcap_file":        currentPCAPFile,
		"pcap_in_progress": pcapInProgress,
		"analysis_mode":    analysisMode,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleBackgroundGridPolar renders a quick polar plot (HTML) of the background grid using go-echarts.
// This is a debugging-only endpoint (no auth) to visually compare grid vs observations without the Svelte UI.
// Query params:
//   - sensor_id (optional; defaults to configured sensor)
//   - max_points (optional; default 8000) to reduce payload size
func (ws *WebServer) handleBackgroundGridPolar(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	maxPoints := 8000
	if mp := r.URL.Query().Get("max_points"); mp != "" {
		if v, err := strconv.Atoi(mp); err == nil && v > 100 && v <= 50000 {
			maxPoints = v
		}
	}

	cells := bm.GetGridCells()
	if len(cells) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no background cells available")
		return
	}

	// Downsample by stride to stay within maxPoints
	stride := 1
	if len(cells) > maxPoints {
		stride = int(math.Ceil(float64(len(cells)) / float64(maxPoints)))
	}

	data := make([]opts.ScatterData, 0, len(cells)/stride+1)
	maxAbs := 0.0
	maxSeen := float64(0)
	for i := 0; i < len(cells); i += stride {
		c := cells[i]
		theta := float64(c.AzimuthDeg) * math.Pi / 180.0
		x := float64(c.Range) * math.Cos(theta)
		y := float64(c.Range) * math.Sin(theta)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}

		seen := float64(c.TimesSeen)
		if seen > maxSeen {
			maxSeen = seen
		}

		data = append(data, opts.ScatterData{Value: []interface{}{x, y, seen}})
	}

	// Add a small padding so points at the edges are visible
	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	if maxSeen == 0 {
		maxSeen = 1
	}

	// Force a square plot by using equal width/height and symmetric axis ranges
	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Background (Polar->XY)", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "LiDAR Background Grid", Subtitle: fmt.Sprintf("sensor=%s points=%d stride=%d", sensorID, len(data), stride)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxSeen),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)

	scatter.AddSeries("background", data, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 3}))

	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render chart: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleLidarDebugDashboard renders a simple dashboard with iframes to the debug charts.
func (ws *WebServer) handleLidarDebugDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}
	safeSensorID := html.EscapeString(sensorID)
	qs := ""
	if sensorID != "" {
		qs = "?sensor_id=" + url.QueryEscape(sensorID)
	}
	safeQs := html.EscapeString(qs)

	doc := fmt.Sprintf(dashboardHTML, safeSensorID, safeQs)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(doc))
}

// handleTrafficChart renders a simple bar chart of packet/point throughput.
func (ws *WebServer) handleTrafficChart(w http.ResponseWriter, r *http.Request) {
	if ws.stats == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no packet stats available")
		return
	}

	snap := ws.stats.GetLatestSnapshot()
	if snap == nil {
		snap = &StatsSnapshot{Timestamp: time.Now()}
	}

	x := []string{"Packets/s", "MB/s", "Points/s", "Dropped (recent)"}
	y := []opts.BarData{
		{Value: snap.PacketsPerSec},
		{Value: snap.MBPerSec},
		{Value: snap.PointsPerSec},
		{Value: snap.DroppedCount},
	}

	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "720px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "LiDAR Traffic", Subtitle: snap.Timestamp.Format(time.RFC3339)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
	)
	bar.SetXAxis(x).
		AddSeries("traffic", y,
			charts.WithLabelOpts(opts.Label{Show: opts.Bool(true), Position: "top"}),
		)

	page := components.NewPage()
	page.SetAssetsHost(echartsAssetsPrefix)
	page.AddCharts(bar)

	var buf bytes.Buffer
	if err := page.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("render error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleBackgroundGridHeatmapChart renders a coarse heatmap (as colored scatter)
// using the aggregated buckets returned by GetGridHeatmap.
func (ws *WebServer) handleBackgroundGridHeatmapChart(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	azBucketDeg := 3.0
	if v := r.URL.Query().Get("azimuth_bucket_deg"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 1.0 {
			azBucketDeg = parsed
		}
	}
	settledThreshold := uint32(5)
	if v := r.URL.Query().Get("settled_threshold"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			settledThreshold = uint32(parsed)
		}
	}

	heatmap := bm.GetGridHeatmap(azBucketDeg, settledThreshold)
	if heatmap == nil || len(heatmap.Buckets) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no heatmap buckets available")
		return
	}

	// Build scatter points colored by MeanTimesSeen (or settled cells)
	points := make([]opts.ScatterData, 0, len(heatmap.Buckets))
	maxVal := 0.0
	for _, b := range heatmap.Buckets {
		if b.FilledCells == 0 {
			continue
		}
		val := b.MeanTimesSeen
		if val == 0 {
			val = float64(b.SettledCells)
		}
		if val > maxVal {
			maxVal = val
		}
	}
	if maxVal == 0 {
		maxVal = 1.0
	}

	maxAbs := 0.0
	for _, b := range heatmap.Buckets {
		if b.FilledCells == 0 {
			continue
		}
		azMid := (b.AzimuthDegStart + b.AzimuthDegEnd) / 2.0
		rRange := b.MeanRangeMeters
		if rRange == 0 {
			rRange = (b.MinRangeMeters + b.MaxRangeMeters) / 2.0
		}
		theta := azMid * math.Pi / 180.0
		x := rRange * math.Cos(theta)
		y := rRange * math.Sin(theta)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}

		norm := b.MeanTimesSeen / maxVal
		if norm < 0 {
			norm = 0
		}
		if norm > 1 {
			norm = 1
		}

		pt := opts.ScatterData{Value: []interface{}{x, y, b.MeanTimesSeen}}
		points = append(points, pt)
	}

	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Background Heatmap", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "LiDAR Background Heatmap", Subtitle: fmt.Sprintf("sensor=%s buckets=%d az=%g", sensorID, len(points), azBucketDeg)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxVal),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)
	scatter.AddSeries("heatmap", points, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 10}))

	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render heatmap chart: %v", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleClustersChart renders recent clusters as scatter points (color by point count).
func (ws *WebServer) handleClustersChart(w http.ResponseWriter, r *http.Request) {
	if ws.trackAPI == nil || ws.trackAPI.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "track DB not configured")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	// time window (seconds)
	var startNanos, endNanos int64
	if s := r.URL.Query().Get("start"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			startNanos = parsed * 1e9
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if parsed, err := strconv.ParseInt(e, 10, 64); err == nil {
			endNanos = parsed * 1e9
		}
	}
	if endNanos == 0 {
		endNanos = time.Now().UnixNano()
	}
	if startNanos == 0 {
		startNanos = endNanos - int64(time.Hour)
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	clusters, err := lidar.GetRecentClusters(ws.trackAPI.db, sensorID, startNanos, endNanos, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clusters: %v", err))
		return
	}

	pts := make([]opts.ScatterData, 0, len(clusters))
	maxPts := 1
	for _, c := range clusters {
		if c.PointsCount > maxPts {
			maxPts = c.PointsCount
		}
	}
	maxAbs := 0.0
	for _, c := range clusters {
		x := float64(c.CentroidX)
		y := float64(c.CentroidY)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		pt := opts.ScatterData{Value: []interface{}{x, y, c.PointsCount}}
		pts = append(pts, pt)
	}
	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Clusters", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "Recent Clusters", Subtitle: fmt.Sprintf("sensor=%s count=%d", sensorID, len(pts))}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxPts),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)
	scatter.AddSeries("clusters", pts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 10}))
	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render clusters chart: %v", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleTracksChart renders current track positions (and optionally recent observations) as a scatter overlay.
func (ws *WebServer) handleTracksChart(w http.ResponseWriter, r *http.Request) {
	if ws.trackAPI == nil || ws.trackAPI.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "track DB not configured")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	state := r.URL.Query().Get("state")
	tracks, err := lidar.GetActiveTracks(ws.trackAPI.db, sensorID, state)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get tracks: %v", err))
		return
	}

	pts := make([]opts.ScatterData, 0, len(tracks))
	maxAbs := 0.0
	maxObs := 0
	for _, t := range tracks {
		x := float64(t.X)
		y := float64(t.Y)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		if t.ObservationCount > maxObs {
			maxObs = t.ObservationCount
		}
		pt := opts.ScatterData{Value: []interface{}{x, y, t.ObservationCount}}
		pts = append(pts, pt)
	}
	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}
	if maxObs == 0 {
		maxObs = 1
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Tracks", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "Active Tracks", Subtitle: fmt.Sprintf("sensor=%s count=%d", sensorID, len(pts))}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxObs),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)
	scatter.AddSeries("tracks", pts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 8}))
	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render tracks chart: %v", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleForegroundFrameChart renders the most recent foreground frame cached from the pipeline.
// Points are pulled from the in-memory foreground snapshot instead of the track observations table
// so the chart reflects the full per-point mask output (point density, foreground fraction, etc.).
func (ws *WebServer) handleForegroundFrameChart(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	snapshot := lidar.GetForegroundSnapshot(sensorID)
	if snapshot == nil || (len(snapshot.ForegroundPoints) == 0 && len(snapshot.BackgroundPoints) == 0) {
		ws.writeJSONError(w, http.StatusNotFound, "no foreground snapshot available")
		return
	}

	fgPts := make([]opts.ScatterData, 0, len(snapshot.ForegroundPoints))
	bgPts := make([]opts.ScatterData, 0, len(snapshot.BackgroundPoints))
	maxAbs := 0.0

	for _, p := range snapshot.BackgroundPoints {
		x := p.X
		y := p.Y
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		bgPts = append(bgPts, opts.ScatterData{Value: []interface{}{x, y}})
	}

	for _, p := range snapshot.ForegroundPoints {
		x := p.X
		y := p.Y
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		fgPts = append(fgPts, opts.ScatterData{Value: []interface{}{x, y}})
	}

	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	fraction := 0.0
	if snapshot.TotalPoints > 0 {
		fraction = float64(snapshot.ForegroundCount) / float64(snapshot.TotalPoints)
	}

	subtitle := fmt.Sprintf(
		"sensor=%s ts=%s fg=%d bg=%d total=%d (%.2f%% fg)",
		sensorID,
		snapshot.Timestamp.UTC().Format(time.RFC3339),
		snapshot.ForegroundCount,
		snapshot.BackgroundCount,
		snapshot.TotalPoints,
		fraction*100,
	)

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Foreground Frame", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "Foreground vs Background", Subtitle: subtitle}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
	)

	scatter.AddSeries("background", bgPts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 4}), charts.WithItemStyleOpts(opts.ItemStyle{Color: "#9e9e9e"}))
	scatter.AddSeries("foreground", fgPts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 6}), charts.WithItemStyleOpts(opts.ItemStyle{Color: "#ff5252"}))

	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render foreground chart: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleExportSnapshotASC triggers an export to ASC for a given snapshot_id (or latest if not provided).
// Query params: sensor_id (required), snapshot_id (optional)
// Note: The out parameter is ignored for security - the file path is generated internally.
func (ws *WebServer) handleExportSnapshotASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	var snap *lidar.BgSnapshot
	snapID := r.URL.Query().Get("snapshot_id")
	if snapID != "" {
		// TODO: implement lookup by snapshot_id if needed
		ws.writeJSONError(w, http.StatusNotImplemented, "snapshot_id lookup not implemented yet")
		return
	} else {
		if ws.db == nil {
			ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
			return
		}
		var err error
		snap, err = ws.db.GetLatestBgSnapshot(sensorID)
		if err != nil || snap == nil {
			ws.writeJSONError(w, http.StatusNotFound, "no snapshot found for sensor")
			return
		}
	}
	// Build elevations argument from embedded config (if available).
	var elevs []float64
	if cfg, err := parse.LoadEmbeddedPandar40PConfig(); err == nil {
		if e := parse.ElevationsFromConfig(cfg); e != nil && len(e) == snap.Rings {
			elevs = e
		}
	}

	// The export path is generated internally by ExportBgSnapshotToASC
	// to prevent user-controlled data from flowing into file system operations.
	if _, err := lidar.ExportBgSnapshotToASC(snap, elevs); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("export error: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "File exported to temp directory"})
}

// handleExportFrameSequenceASC exports a background snapshot plus the next 5 frames and 5 foreground snapshots.
// Query params: sensor_id (required)
// Note: Export paths are generated internally for security - files are written to the temp directory.
func (ws *WebServer) handleExportFrameSequenceASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	fb := lidar.GetFrameBuilder(sensorID)
	if fb == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no FrameBuilder for sensor")
		return
	}

	// Export latest background snapshot immediately
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
		return
	}
	snap, err := ws.db.GetLatestBgSnapshot(sensorID)
	if err != nil || snap == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no snapshot found for sensor")
		return
	}
	var elevs []float64
	if cfg, err := parse.LoadEmbeddedPandar40PConfig(); err == nil {
		if e := parse.ElevationsFromConfig(cfg); e != nil && len(e) == snap.Rings {
			elevs = e
		}
	}
	// Export paths are generated internally by the export functions for security
	if _, err := lidar.ExportBgSnapshotToASC(snap, elevs); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("background export error: %v", err))
		return
	}

	// Queue next 5 frames for export
	fb.RequestExportFrameBatchASC(5)

	// Kick off foreground snapshot exports asynchronously (paths are ignored, generated internally)
	go ws.exportForegroundSequenceInternal(sensorID, 5)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "scheduled",
		"note":   "Background exported immediately to temp directory; frames and foreground exports scheduled. Check temp directory for files with 'export_' prefix.",
	})
}

// handleExportNextFrameASC triggers an export to ASC for the next completed LiDARFrame for a sensor.
// Query params: sensor_id (required)
// Note: The out parameter is ignored for security - the file path is generated internally.
func (ws *WebServer) handleExportNextFrameASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	// Find FrameBuilder for sensorID (assume registry or global)
	fb := lidar.GetFrameBuilder(sensorID)
	if fb == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no FrameBuilder for sensor")
		return
	}

	// The export path is generated internally by the export functions
	fb.RequestExportNextFrameASC()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "Will export next completed frame to temp directory"})
}

// handleExportForegroundASC exports the latest foreground snapshot to an ASC file for quick inspection.
// Query params: sensor_id (required)
// Note: The out parameter is ignored for security - the file path is generated internally.
func (ws *WebServer) handleExportForegroundASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	snap := lidar.GetForegroundSnapshot(sensorID)
	if snap == nil || len(snap.ForegroundPoints) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no foreground snapshot available")
		return
	}

	// The export path is generated internally by ExportForegroundSnapshotToASC
	if _, err := lidar.ExportForegroundSnapshotToASC(snap); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("export error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "File exported to temp directory"})
}

// exportForegroundSequenceInternal captures and exports the next N foreground snapshots for a sensor.
// Runs asynchronously and logs progress; intended for batch export orchestration.
// Paths are generated internally by the export functions for security.
func (ws *WebServer) exportForegroundSequenceInternal(sensorID string, count int) {
	if count <= 0 {
		return
	}

	deadline := time.Now().Add(30 * time.Second)
	var last time.Time
	exported := 0

	for exported < count && time.Now().Before(deadline) {
		snap := lidar.GetForegroundSnapshot(sensorID)
		if snap == nil || snap.Timestamp.IsZero() || len(snap.ForegroundPoints) == 0 || !snap.Timestamp.After(last) {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Export path is generated internally by ExportForegroundSnapshotToASC
		if _, err := lidar.ExportForegroundSnapshotToASC(snap); err != nil {
			log.Printf("[ExportSequence] foreground export failed (%d/%d) sensor=%s: %v", exported+1, count, sensorID, err)
		} else {
			log.Printf("[ExportSequence] exported foreground %d/%d for sensor=%s", exported+1, count, sensorID)
		}

		last = snap.Timestamp
		exported++
	}

	if exported < count {
		log.Printf("[ExportSequence] foreground export ended early: got %d/%d snapshots for sensor=%s before timeout", exported, count, sensorID)
	}
}

// handleLidarSnapshots returns a JSON array of the last N lidar background snapshots for a sensor_id, with nonzero cell count for each.
// Query params:
//
//	sensor_id (required)
//	limit (optional, default 10)
func (ws *WebServer) handleLidarSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
		if limit <= 0 || limit > 100 {
			limit = 10
		}
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
		return
	}
	snaps, err := ws.db.ListRecentBgSnapshots(sensorID, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get recent snapshots: %v", err))
		return
	}
	type SnapSummary struct {
		SnapshotID        interface{} `json:"snapshot_id"`
		SensorID          string      `json:"sensor_id"`
		Taken             string      `json:"taken"`
		BlobBytes         int         `json:"blob_bytes"`
		ChangedCellsCount int         `json:"changed_cells_count"`
		SnapshotReason    string      `json:"snapshot_reason"`
		NonzeroCells      int         `json:"nonzero_cells"`
		TotalCells        int         `json:"total_cells"`
	}
	var summaries []SnapSummary
	for _, snap := range snaps {
		var snapIDVal interface{}
		if snap.SnapshotID != nil {
			snapIDVal = *snap.SnapshotID
		}
		nonzero := 0
		total := 0
		if len(snap.GridBlob) > 0 {
			gz, err := gzip.NewReader(bytes.NewReader(snap.GridBlob))
			if err == nil {
				var cells []lidar.BackgroundCell
				dec := gob.NewDecoder(gz)
				if err := dec.Decode(&cells); err == nil {
					total = len(cells)
					for _, c := range cells {
						if c.TimesSeenCount > 0 || c.AverageRangeMeters != 0 || c.RangeSpreadMeters != 0 {
							nonzero++
						}
					}
				}
				gz.Close()
			}
		}
		summaries = append(summaries, SnapSummary{
			SnapshotID:        snapIDVal,
			SensorID:          snap.SensorID,
			Taken:             time.Unix(0, snap.TakenUnixNanos).Format(time.RFC3339Nano),
			BlobBytes:         len(snap.GridBlob),
			ChangedCellsCount: snap.ChangedCellsCount,
			SnapshotReason:    snap.SnapshotReason,
			NonzeroCells:      nonzero,
			TotalCells:        total,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

func (ws *WebServer) handleLidarSnapshotsCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	// Parse form to get sensor_id if needed, or query param
	if err := r.ParseForm(); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid form data")
		return
	}
	sensorID := r.FormValue("sensor_id")
	if sensorID == "" {
		sensorID = r.URL.Query().Get("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured")
		return
	}

	count, err := ws.db.DeleteDuplicateBgSnapshots(sensorID)
	if err != nil {
		log.Printf("Failed to cleanup snapshots for %s: %v", sensorID, err)
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("cleanup failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sensor_id": sensorID,
		"deleted":   count,
	})
}

// handleHealth handles the health check endpoint
func (ws *WebServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "service": "lidar", "timestamp": "%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ws *WebServer) handleLidarStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ws.dataSourceMu.RLock()
	currentSource := ws.currentSource
	currentPCAPFile := ws.currentPCAPFile
	ws.dataSourceMu.RUnlock()

	ws.pcapMu.Lock()
	pcapInProgress := ws.pcapInProgress
	ws.pcapMu.Unlock()

	var statsSnapshot *StatsSnapshot
	if ws.stats != nil {
		statsSnapshot = ws.stats.GetLatestSnapshot()
	}

	uptime := ""
	if ws.stats != nil {
		uptime = ws.stats.GetUptime().Round(time.Second).String()
	}

	response := struct {
		Status           string         `json:"status"`
		SensorID         string         `json:"sensor_id"`
		UDPPort          int            `json:"udp_port"`
		Forwarding       bool           `json:"forwarding_enabled"`
		ForwardAddr      string         `json:"forward_addr,omitempty"`
		ForwardPort      int            `json:"forward_port,omitempty"`
		ParsingEnabled   bool           `json:"parsing_enabled"`
		DataSource       string         `json:"data_source"`
		PCAPFile         string         `json:"pcap_file,omitempty"`
		PCAPInProgress   bool           `json:"pcap_in_progress"`
		Uptime           string         `json:"uptime"`
		Stats            *StatsSnapshot `json:"stats,omitempty"`
		PCAPSafeDir      string         `json:"pcap_safe_dir,omitempty"`
		BackgroundSensor string         `json:"background_sensor_id,omitempty"`
	}{
		Status:           "ok",
		SensorID:         ws.sensorID,
		UDPPort:          ws.udpPort,
		Forwarding:       ws.forwardingEnabled,
		ForwardAddr:      ws.forwardAddr,
		ForwardPort:      ws.forwardPort,
		ParsingEnabled:   ws.parsingEnabled,
		DataSource:       string(currentSource),
		PCAPFile:         currentPCAPFile,
		PCAPInProgress:   pcapInProgress,
		Uptime:           uptime,
		Stats:            statsSnapshot,
		PCAPSafeDir:      ws.pcapSafeDir,
		BackgroundSensor: ws.sensorID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStatus handles the main status page endpoint
func (ws *WebServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/lidar/monitor" && r.URL.Path != "/" && r.URL.Path != "/api/lidar/monitor" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")

	// Determine forwarding status
	forwardingStatus := "disabled"
	if ws.forwardingEnabled {
		forwardingStatus = fmt.Sprintf("enabled (%s:%d)", ws.forwardAddr, ws.forwardPort)
	}

	// Determine parsing status
	parsingStatus := "enabled"
	if !ws.parsingEnabled {
		parsingStatus = "disabled"
	}

	ws.dataSourceMu.RLock()
	mode := "Live UDP"
	switch ws.currentSource {
	case DataSourcePCAP:
		mode = "PCAP Replay"
	case DataSourceLive:
		mode = "Live UDP"
	}
	currentPCAPFile := ws.currentPCAPFile
	ws.dataSourceMu.RUnlock()

	ws.pcapMu.Lock()
	pcapInProgress := ws.pcapInProgress
	pcapSpeedMode := ws.pcapSpeedMode
	pcapSpeedRatio := ws.pcapSpeedRatio
	ws.pcapMu.Unlock()

	// Get background manager to show current params
	var bgParams *lidar.BackgroundParams
	var bgParamsJSON string
	var bgParamDefs []ParamDef
	var bgParamsJSONLines int

	if mgr := lidar.GetBackgroundManager(ws.sensorID); mgr != nil {
		params := mgr.GetParams()
		bgParams = &params

		bgParamDefs = []ParamDef{
			{"noise_relative", "Noise Relative Fraction", params.NoiseRelativeFraction, "%.4f"},
			{"closeness_multiplier", "Closeness Sensitivity Multiplier", params.ClosenessSensitivityMultiplier, "%.2f"},
			{"neighbor_confirmation_count", "Neighbor Confirmation Count", params.NeighborConfirmationCount, ""},
			{"background_update_fraction", "Background Update Fraction", params.BackgroundUpdateFraction, "%.4f"},
			{"post_settle_update_fraction", "Post-Settle Update Fraction", params.PostSettleUpdateFraction, "%.4f"},
			{"warmup_duration_nanos", "Warmup Duration (ns)", params.WarmupDurationNanos, ""},
			{"warmup_min_frames", "Warmup Minimum Frames", params.WarmupMinFrames, ""},
			{"safety_margin_meters", "Safety Margin (meters)", params.SafetyMarginMeters, "%.2f"},
			{"seed_from_first", "Seed From First Observation", params.SeedFromFirstObservation, ""},
			{"foreground_min_cluster_points", "Foreground Min Cluster Points", params.ForegroundMinClusterPoints, ""},
			{"foreground_dbscan_eps", "Foreground DBSCAN Eps", params.ForegroundDBSCANEps, "%.3f"},
			{"enable_diagnostics", "Enable Diagnostics", mgr.EnableDiagnostics, ""},
		}

		// Create a map for JSON representation matching the API structure
		paramsMap := make(map[string]interface{})
		for _, def := range bgParamDefs {
			paramsMap[def.Key] = def.Value
		}

		if jsonBytes, err := json.MarshalIndent(paramsMap, "", "  "); err == nil {
			bgParamsJSON = string(jsonBytes)
			bgParamsJSONLines = strings.Count(bgParamsJSON, "\n") + 2
		}
	}

	// Refresh foreground snapshot counts for status rendering.
	ws.updateLatestFgCounts(ws.sensorID)

	// Load and parse the HTML template from embedded filesystem
	tmpl, err := template.ParseFS(StatusHTML, "status.html")
	if err != nil {
		http.Error(w, "Error loading template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Template data
	data := struct {
		Version           string
		GitSHA            string
		BuildTime         string
		UDPPort           int
		HTTPAddress       string
		ForwardingStatus  string
		ParsingStatus     string
		Mode              string
		PCAPSafeDir       string
		Uptime            string
		Stats             *StatsSnapshot
		SensorID          string
		BGParams          *lidar.BackgroundParams
		BGParamsJSON      string
		BGParamDefs       []ParamDef
		BGParamsJSONLines int
		PCAPFile          string
		PCAPInProgress    bool
		PCAPSpeedMode     string
		PCAPSpeedRatio    float64
		FgSnapshotCounts  map[string]int
	}{
		Version:           version.Version,
		GitSHA:            version.GitSHA,
		BuildTime:         version.BuildTime,
		UDPPort:           ws.udpPort,
		HTTPAddress:       ws.address,
		ForwardingStatus:  forwardingStatus,
		ParsingStatus:     parsingStatus,
		Mode:              mode,
		PCAPSafeDir:       ws.pcapSafeDir,
		Uptime:            ws.stats.GetUptime().Round(time.Second).String(),
		Stats:             ws.stats.GetLatestSnapshot(),
		SensorID:          ws.sensorID,
		BGParams:          bgParams,
		BGParamsJSON:      bgParamsJSON,
		BGParamDefs:       bgParamDefs,
		BGParamsJSONLines: bgParamsJSONLines,
		PCAPFile:          currentPCAPFile,
		PCAPInProgress:    pcapInProgress,
		PCAPSpeedMode:     pcapSpeedMode,
		PCAPSpeedRatio:    pcapSpeedRatio,
		FgSnapshotCounts:  ws.getLatestFgCounts(),
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Error executing template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleLidarPersist triggers manual persistence of a BackgroundGrid snapshot.
// Expects POST with form value or query param `sensor_id`.
func (ws *WebServer) handleLidarPersist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
		return
	}

	// Support both query params and form data for sensor_id
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}

	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	mgr := lidar.GetBackgroundManager(sensorID)
	if mgr == nil || mgr.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}

	// If a PersistCallback is set, build a minimal snapshot object and call it.
	if mgr.PersistCallback != nil {
		snap := &lidar.BgSnapshot{
			SensorID:          mgr.Grid.SensorID,
			TakenUnixNanos:    time.Now().UnixNano(),
			Rings:             mgr.Grid.Rings,
			AzimuthBins:       mgr.Grid.AzimuthBins,
			ParamsJSON:        "{}",
			GridBlob:          []byte("manual-trigger"),
			ChangedCellsCount: mgr.Grid.ChangesSinceSnapshot,
			SnapshotReason:    "manual_api",
		}
		if err := mgr.PersistCallback(snap); err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("persist error: %v", err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "sensor_id": sensorID})
		log.Printf("Successfully persisted snapshot for sensor '%s'", sensorID)
		return
	}

	ws.writeJSONError(w, http.StatusNotImplemented, "no persist callback configured for this sensor")
}

// handleLidarSnapshot returns a JSON summary for the latest lidar background snapshot for a sensor_id.
// Query params:
//
//	sensor_id (required)
//	db (optional) - path to sqlite DB (defaults to data/sensor_data.db)
func (ws *WebServer) handleLidarSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	// Use the configured DB instance. We no longer probe multiple DB files.
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
		return
	}
	snap, err := ws.db.GetLatestBgSnapshot(sensorID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get latest snapshot: %v", err))
		return
	}
	if snap == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no snapshot found for sensor")
		return
	}

	// helper for optional snapshot id
	var snapIDVal interface{}
	if snap.SnapshotID != nil {
		snapIDVal = *snap.SnapshotID
	}

	summary := map[string]interface{}{
		"snapshot_id":         snapIDVal,
		"sensor_id":           snap.SensorID,
		"taken":               time.Unix(0, snap.TakenUnixNanos).Format(time.RFC3339Nano),
		"rings":               snap.Rings,
		"azimuth_bins":        snap.AzimuthBins,
		"params_json":         snap.ParamsJSON,
		"blob_bytes":          len(snap.GridBlob),
		"changed_cells_count": snap.ChangedCellsCount,
		"snapshot_reason":     snap.SnapshotReason,
	}

	// quick hex prefix for inspection
	prefix := 64
	if len(snap.GridBlob) < prefix {
		prefix = len(snap.GridBlob)
	}
	summary["blob_hex_prefix"] = hex.EncodeToString(snap.GridBlob[:prefix])

	// Try to gunzip + gob decode
	if len(snap.GridBlob) == 0 {
		summary["total_cells"] = 0
		summary["non_empty_cells"] = 0
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
		return
	}

	gz, err := gzip.NewReader(bytes.NewReader(snap.GridBlob))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("gunzip: %v", err))
		return
	}
	defer gz.Close()

	var cells []lidar.BackgroundCell
	dec := gob.NewDecoder(gz)
	if err := dec.Decode(&cells); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("gob decode: %v", err))
		return
	}

	total := len(cells)
	nonZero := 0
	samples := make([]map[string]interface{}, 0, 10)
	maxSamples := 10
	for i, c := range cells {
		if c.TimesSeenCount > 0 || c.AverageRangeMeters != 0 || c.RangeSpreadMeters != 0 {
			nonZero++
			if len(samples) < maxSamples {
				ring := i / snap.AzimuthBins
				azbin := i % snap.AzimuthBins
				samples = append(samples, map[string]interface{}{
					"idx":          i,
					"ring":         ring,
					"azbin":        azbin,
					"avg_m":        c.AverageRangeMeters,
					"spread_m":     c.RangeSpreadMeters,
					"times_seen":   c.TimesSeenCount,
					"last_update":  time.Unix(0, c.LastUpdateUnixNanos).Format(time.RFC3339Nano),
					"frozen_until": time.Unix(0, c.FrozenUntilUnixNanos).Format(time.RFC3339Nano),
				})
			}
		}
	}

	summary["total_cells"] = total
	summary["non_empty_cells"] = nonZero
	summary["samples"] = samples

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleAcceptanceMetrics returns the range-bucketed acceptance/rejection metrics
// for a given sensor. Query params: sensor_id (required)
func (ws *WebServer) handleAcceptanceMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := lidar.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}
	metrics := mgr.GetAcceptanceMetrics()
	if metrics == nil {
		metrics = &lidar.AcceptanceMetrics{}
	}

	// Build richer response including totals and computed rates for convenience
	type RichAcceptance struct {
		BucketsMeters   []float64 `json:"BucketsMeters"`
		AcceptCounts    []int64   `json:"AcceptCounts"`
		RejectCounts    []int64   `json:"RejectCounts"`
		Totals          []int64   `json:"Totals"`
		AcceptanceRates []float64 `json:"AcceptanceRates"`
	}

	totals := make([]int64, len(metrics.BucketsMeters))
	rates := make([]float64, len(metrics.BucketsMeters))
	for i := range metrics.BucketsMeters {
		var a, rj int64
		if i < len(metrics.AcceptCounts) {
			a = metrics.AcceptCounts[i]
		}
		if i < len(metrics.RejectCounts) {
			rj = metrics.RejectCounts[i]
		}
		totals[i] = a + rj
		if totals[i] > 0 {
			rates[i] = float64(a) / float64(totals[i])
		} else {
			rates[i] = 0.0
		}
	}

	resp := RichAcceptance{
		BucketsMeters:   metrics.BucketsMeters,
		AcceptCounts:    metrics.AcceptCounts,
		RejectCounts:    metrics.RejectCounts,
		Totals:          totals,
		AcceptanceRates: rates,
	}

	// Log G: Debug mode returns verbose breakdown with active params
	debug := r.URL.Query().Get("debug") == "true"
	if debug {
		debugInfo := map[string]interface{}{
			"metrics":   resp,
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"sensor_id": sensorID,
		}
		// Include current params for context
		params := mgr.GetParams()
		debugInfo["params"] = map[string]interface{}{
			"noise_relative":        params.NoiseRelativeFraction,
			"closeness_multiplier":  params.ClosenessSensitivityMultiplier,
			"neighbor_confirmation": params.NeighborConfirmationCount,
			"seed_from_first":       params.SeedFromFirstObservation,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(debugInfo)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAcceptanceReset zeros the accept/reject counters for a given sensor_id.
// Method: POST. Query param: sensor_id (required)
func (ws *WebServer) handleAcceptanceReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := lidar.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}
	if err := mgr.ResetAcceptanceMetrics(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("reset error: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "sensor_id": sensorID})
}

// handlePCAPStart switches the data source to PCAP replay and starts ingestion.
// Method: POST. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	if sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unexpected sensor_id '%s'", sensorID))
		return
	}

	var pcapFile string
	var analysisMode bool
	var speedMode string
	var speedRatio float64 = 1.0
	var startSeconds float64 = 0
	var durationSeconds float64 = -1
	var debugRingMin, debugRingMax int
	var debugAzMin, debugAzMax float32
	var enableDebug bool
	var enablePlots bool

	// Accept both JSON and form data
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" || contentType == "application/json; charset=utf-8" {
		// Parse JSON body
		var req struct {
			PCAPFile        string  `json:"pcap_file"`
			AnalysisMode    bool    `json:"analysis_mode"`
			SpeedMode       string  `json:"speed_mode"`
			SpeedRatio      float64 `json:"speed_ratio"`
			StartSeconds    float64 `json:"start_seconds"`
			DurationSeconds float64 `json:"duration_seconds"`
			DebugRingMin    int     `json:"debug_ring_min"`
			DebugRingMax    int     `json:"debug_ring_max"`
			DebugAzMin      float32 `json:"debug_az_min"`
			DebugAzMax      float32 `json:"debug_az_max"`
			EnableDebug     bool    `json:"enable_debug"`
			EnablePlots     bool    `json:"enable_plots"`
		}
		// Set defaults
		req.DurationSeconds = -1
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				ws.writeJSONError(w, http.StatusBadRequest, "missing JSON body for PCAP request")
				return
			}
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			return
		}
		pcapFile = req.PCAPFile
		analysisMode = req.AnalysisMode
		speedMode = req.SpeedMode
		if req.SpeedRatio > 0 {
			speedRatio = req.SpeedRatio
		}
		startSeconds = req.StartSeconds
		durationSeconds = req.DurationSeconds
		debugRingMin = req.DebugRingMin
		debugRingMax = req.DebugRingMax
		debugAzMin = req.DebugAzMin
		debugAzMax = req.DebugAzMax
		enableDebug = req.EnableDebug
		enablePlots = req.EnablePlots
	} else {
		// Parse form data (default for HTML forms)
		if err := r.ParseForm(); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid form data: %v", err))
			return
		}
		pcapFile = r.FormValue("pcap_file")
		analysisMode = r.FormValue("analysis_mode") == "true" || r.FormValue("analysis_mode") == "1"
		speedMode = r.FormValue("speed_mode")
		if v := r.FormValue("speed_ratio"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
				speedRatio = f
			}
		}
		if v := r.FormValue("start_seconds"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
				startSeconds = f
			}
		}
		if v := r.FormValue("duration_seconds"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				durationSeconds = f
			}
		} else {
			durationSeconds = -1
		}
		if v := r.FormValue("debug_ring_min"); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i >= 0 {
				debugRingMin = i
			}
		}
		if v := r.FormValue("debug_ring_max"); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i >= 0 {
				debugRingMax = i
			}
		}
		if v := r.FormValue("debug_az_min"); v != "" {
			if f, err := strconv.ParseFloat(v, 32); err == nil && f >= 0 {
				debugAzMin = float32(f)
			}
		}
		if v := r.FormValue("debug_az_max"); v != "" {
			if f, err := strconv.ParseFloat(v, 32); err == nil && f >= 0 {
				debugAzMax = float32(f)
			}
		}
		enableDebug = r.FormValue("enable_debug") == "true" || r.FormValue("enable_debug") == "1"
		enablePlots = r.FormValue("enable_plots") == "true" || r.FormValue("enable_plots") == "1"
	}

	if speedMode == "" {
		speedMode = "fastest"
	}

	if pcapFile == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'pcap_file' in request body")
		return
	}

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	if ws.currentSource == DataSourcePCAP {
		ws.writeJSONError(w, http.StatusConflict, "PCAP replay already active")
		return
	}

	ws.stopLiveListenerLocked()

	if err := ws.resetAllState(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reset background grid: %v", err))
		if restartErr := ws.startLiveListenerLocked(); restartErr != nil {
			log.Printf("Failed to restart live listener after reset error: %v", restartErr)
			return
		}
		return
	}

	if err := ws.startPCAPLocked(pcapFile, speedMode, speedRatio, startSeconds, durationSeconds, debugRingMin, debugRingMax, debugAzMin, debugAzMax, enableDebug, enablePlots); err != nil {
		var sErr *switchError
		if errors.As(err, &sErr) {
			ws.writeJSONError(w, sErr.status, sErr.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		if restartErr := ws.startLiveListenerLocked(); restartErr != nil {
			log.Printf("Failed to restart live listener after PCAP error: %v", restartErr)
		}
		return
	}

	ws.pcapMu.Lock()
	ws.pcapAnalysisMode = analysisMode
	ws.pcapMu.Unlock()

	ws.currentSource = DataSourcePCAP
	currentFile := ws.currentPCAPFile

	mode := "replay"
	if analysisMode {
		mode = "analysis"
	}
	log.Printf("[DataSource] switched to PCAP %s mode for sensor=%s file=%s", mode, sensorID, currentFile)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "started",
		"sensor_id":      sensorID,
		"current_source": string(ws.currentSource),
		"pcap_file":      currentFile,
		"analysis_mode":  analysisMode,
	})
}

// handlePCAPStop cancels any active PCAP replay and returns to live UDP.
// Method: POST. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	if sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unexpected sensor_id '%s'", sensorID))
		return
	}

	// Acquire dataSourceMu first to maintain consistent lock ordering with handlePCAPStart
	// (always dataSourceMu → pcapMu) to prevent deadlock
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	if ws.currentSource != DataSourcePCAP && ws.currentSource != DataSourcePCAPAnalysis {
		ws.writeJSONError(w, http.StatusConflict, "system is not in PCAP mode")
		return
	}

	// Now acquire pcapMu while holding dataSourceMu (consistent ordering)
	ws.pcapMu.Lock()
	if !ws.pcapInProgress {
		ws.pcapMu.Unlock()
		ws.writeJSONError(w, http.StatusConflict, "no PCAP replay in progress")
		return
	}
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapMu.Unlock()

	// Release dataSourceMu before waiting for goroutine completion to avoid deadlock
	// (the PCAP goroutine needs dataSourceMu to finish)
	// NOTE: We must unlock manually here because we need to wait for done.
	// Since handlePCAPStop defers the release of dataSourceMu, we must re-lock before returning.
	ws.dataSourceMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	// Reacquire dataSourceMu for subsequent operations
	// This lock will be released by the deferred Unlock when function returns
	ws.dataSourceMu.Lock()

	// If in analysis mode, only reset grid if explicitly requested
	ws.pcapMu.Lock()
	analysisMode := ws.pcapAnalysisMode
	ws.pcapAnalysisMode = false // Clear flag when stopping
	ws.pcapMu.Unlock()

	if !analysisMode {
		// Normal mode: always reset all state when stopping
		if err := ws.resetAllState(); err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reset state: %v", err))
			return
		}
	} else {
		// Analysis mode: still reset frame builder to clear stale frames
		ws.resetFrameBuilder()
		log.Printf("[DataSource] preserving grid from PCAP analysis for sensor=%s", sensorID)
	}

	if err := ws.startLiveListenerLocked(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start live listener: %v", err))
		return
	}

	ws.currentSource = DataSourceLive
	ws.currentPCAPFile = ""

	log.Printf("[DataSource] switched to Live after PCAP stop for sensor=%s", sensorID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "stopped",
		"sensor_id":      sensorID,
		"current_source": string(ws.currentSource),
	})
}

// handlePCAPResumeLive switches from PCAP analysis mode back to Live while preserving the background grid.
// This allows overlaying live data on top of PCAP-analyzed background.
// Method: POST. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPResumeLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
		return
	}

	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	if sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unexpected sensor_id '%s'", sensorID))
		return
	}

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	if ws.currentSource != DataSourcePCAPAnalysis {
		ws.writeJSONError(w, http.StatusConflict, "system is not in PCAP analysis mode")
		return
	}

	// Start live listener without resetting the grid
	if err := ws.startLiveListenerLocked(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start live listener: %v", err))
		return
	}

	ws.currentSource = DataSourceLive
	ws.currentPCAPFile = ""

	ws.pcapMu.Lock()
	ws.pcapAnalysisMode = false
	ws.pcapMu.Unlock()

	log.Printf("[DataSource] resumed Live from PCAP analysis for sensor=%s (grid preserved)", sensorID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "resumed_live",
		"sensor_id":      sensorID,
		"current_source": string(ws.currentSource),
		"grid_preserved": true,
	})
}

// Close shuts down the web server
func (ws *WebServer) Close() error {
	ws.dataSourceMu.Lock()
	if ws.udpListener != nil {
		ws.stopLiveListenerLocked()
	}
	ws.dataSourceMu.Unlock()

	ws.pcapMu.Lock()
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	if ws.server != nil {
		return ws.server.Close()
	}
	return nil
}

// handleBackgroundGrid returns the full background grid cells.
// Query params: sensor_id (required)
func (ws *WebServer) handleBackgroundGrid(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	g := bm.Grid

	// Use the safe accessor from lidar package
	exportedCells := bm.GetGridCells()

	// Downsample grid for frontend visualization (top-down 2D view)
	// Combine points into spatial buckets to reduce point count from ~72k to ~7k
	const gridSize = 0.1 // 10cm grid resolution
	type gridKey struct {
		x, y int
	}
	type gridAccumulator struct {
		sumX, sumY, sumSpread float64
		maxTimesSeen          uint32
		count                 int
	}
	grid := make(map[gridKey]*gridAccumulator)

	for _, cell := range exportedCells {
		if cell.Range < 0.1 {
			continue
		}

		// Convert Polar to Cartesian
		angleRad := float64(cell.AzimuthDeg) * math.Pi / 180.0
		x := float64(cell.Range) * math.Cos(angleRad)
		y := float64(cell.Range) * math.Sin(angleRad)

		k := gridKey{
			x: int(math.Floor(x / gridSize)),
			y: int(math.Floor(y / gridSize)),
		}

		acc, exists := grid[k]
		if !exists {
			acc = &gridAccumulator{}
			grid[k] = acc
		}
		acc.sumX += x
		acc.sumY += y
		acc.sumSpread += float64(cell.Spread)
		if cell.TimesSeen > acc.maxTimesSeen {
			acc.maxTimesSeen = cell.TimesSeen
		}
		acc.count++
	}

	type APIBackgroundCell struct {
		X         float32 `json:"x"`
		Y         float32 `json:"y"`
		Spread    float32 `json:"range_spread_meters"`
		TimesSeen uint32  `json:"times_seen"`
	}

	cells := make([]APIBackgroundCell, 0, len(grid))
	for _, acc := range grid {
		cells = append(cells, APIBackgroundCell{
			X:         float32(acc.sumX / float64(acc.count)),
			Y:         float32(acc.sumY / float64(acc.count)),
			Spread:    float32(acc.sumSpread / float64(acc.count)),
			TimesSeen: acc.maxTimesSeen,
		})
	}

	resp := struct {
		SensorID    string              `json:"sensor_id"`
		Timestamp   string              `json:"timestamp"`
		Rings       int                 `json:"rings"`
		AzimuthBins int                 `json:"azimuth_bins"`
		Cells       []APIBackgroundCell `json:"cells"`
	}{
		SensorID:    g.SensorID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Rings:       g.Rings,
		AzimuthBins: g.AzimuthBins,
		Cells:       cells,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleBackgroundRegions returns region debug information for the background grid
func (ws *WebServer) handleBackgroundRegions(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := lidar.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	// Check if "include_cells" query parameter is set
	includeCells := r.URL.Query().Get("include_cells") == "true"

	info := bm.GetRegionDebugInfo(includeCells)
	if info == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "failed to get region debug info")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleBackgroundRegionsDashboard renders an HTML visualization of the background regions
func (ws *WebServer) handleBackgroundRegionsDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	escapedSensorID := html.EscapeString(sensorID)

	doc := fmt.Sprintf(regionsDashboardHTML, escapedSensorID, sensorID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(doc))
}
