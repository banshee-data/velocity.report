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
	"html/template"
	"io"
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
	"github.com/banshee-data/velocity.report/internal/security"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

//go:embed status.html
var StatusHTML embed.FS

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

	// Track API for tracking endpoints
	trackAPI *TrackAPI
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
	}

	// Initialize TrackAPI if database is configured
	if config.DB != nil {
		ws.trackAPI = NewTrackAPI(config.DB.DB, config.SensorID)
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

func (ws *WebServer) startPCAPLocked(pcapFile string) error {
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
	ws.pcapMu.Unlock()

	ws.currentPCAPFile = resolvedPath

	go func(path string, ctx context.Context, finished chan struct{}) {
		defer close(finished)
		log.Printf("Starting PCAP replay from file: %s (sensor: %s)", path, ws.sensorID)

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

		if err := network.ReadPCAPFile(ctx, path, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("PCAP replay error: %v", err)
		} else {
			log.Printf("PCAP replay completed: %s", path)
		}

		ws.pcapMu.Lock()
		ws.pcapInProgress = false
		ws.pcapCancel = nil
		ws.pcapDone = nil
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
				// Normal mode: reset grid and return to live
				if err := ws.resetBackgroundGrid(); err != nil {
					log.Printf("Failed to reset background grid after PCAP: %v", err)
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
	}(resolvedPath, ctx, done)

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
	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/api/lidar/monitor", ws.handleStatus)
	mux.HandleFunc("/api/lidar/status", ws.handleLidarStatus)
	mux.HandleFunc("/api/lidar/persist", ws.handleLidarPersist)
	mux.HandleFunc("/api/lidar/snapshot", ws.handleLidarSnapshot)
	mux.HandleFunc("/api/lidar/snapshots", ws.handleLidarSnapshots)
	mux.HandleFunc("/api/lidar/export_snapshot", ws.handleExportSnapshotASC)
	mux.HandleFunc("/api/lidar/export_next_frame", ws.handleExportNextFrameASC)
	mux.HandleFunc("/api/lidar/acceptance", ws.handleAcceptanceMetrics)
	mux.HandleFunc("/api/lidar/acceptance/reset", ws.handleAcceptanceReset)
	mux.HandleFunc("/api/lidar/params", ws.handleBackgroundParams)
	mux.HandleFunc("/api/lidar/grid_status", ws.handleGridStatus)
	mux.HandleFunc("/api/lidar/grid_reset", ws.handleGridReset)
	mux.HandleFunc("/api/lidar/grid_heatmap", ws.handleGridHeatmap)
	mux.HandleFunc("/api/lidar/background/grid", ws.handleBackgroundGrid) // Full background grid
	mux.HandleFunc("/debug/lidar", ws.handleLidarDebugDashboard)
	mux.HandleFunc("/debug/lidar/background/polar", ws.handleBackgroundGridPolar)
	mux.HandleFunc("/debug/lidar/background/heatmap", ws.handleBackgroundGridHeatmapChart)
	mux.HandleFunc("/debug/lidar/foreground", ws.handleForegroundFrameChart)
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
			"noise_relative":              params.NoiseRelativeFraction,
			"enable_diagnostics":          bm.EnableDiagnostics,
			"closeness_multiplier":        params.ClosenessSensitivityMultiplier,
			"neighbor_confirmation_count": params.NeighborConfirmationCount,
			"seed_from_first":             params.SeedFromFirstObservation,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	case http.MethodPost:
		var body struct {
			NoiseRelative        *float64 `json:"noise_relative"`
			EnableDiagnostics    *bool    `json:"enable_diagnostics"`
			ClosenessMultiplier  *float64 `json:"closeness_multiplier"`
			NeighborConfirmation *int     `json:"neighbor_confirmation_count"`
			SeedFromFirst        *bool    `json:"seed_from_first"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
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

		// Read back current params for confirmation
		cur := bm.GetParams()
		// Emit an info log so operators can see applied changes in the app logs
		log.Printf("[Monitor] Applied background params for sensor=%s: noise_relative=%.6f, enable_diagnostics=%v", sensorID, cur.NoiseRelativeFraction, bm.EnableDiagnostics)

		// Log D: API call timing for params with all active settings
		timestamp := time.Now().UnixNano()
		log.Printf("[API:params] sensor=%s noise_rel=%.6f closeness=%.3f neighbors=%d seed_from_first=%v timestamp=%d",
			sensorID, cur.NoiseRelativeFraction, cur.ClosenessSensitivityMultiplier,
			cur.NeighborConfirmationCount, cur.SeedFromFirstObservation, timestamp)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "noise_relative": cur.NoiseRelativeFraction, "enable_diagnostics": bm.EnableDiagnostics})
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

// handleGridReset zeros the BackgroundGrid stats (times seen, averages, spreads)
// and acceptance counters. This is intended only for testing A/B sweeps.
// Method: POST. Query params: sensor_id (required)
func (ws *WebServer) handleGridReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
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
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Background (Polar->XY)", Theme: "dark", Width: "900px", Height: "900px"}),
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
	qs := ""
	if sensorID != "" {
		qs = "?sensor_id=" + url.QueryEscape(sensorID)
	}

	doc := fmt.Sprintf(`<!DOCTYPE html>
	<html>
	<head>
		<title>LiDAR Debug Dashboard - %s</title>
		<style>
			html, body { height: 100%%; }
			body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 12px auto; max-width: 1800px; background: #f5f7fb; color: #0f172a; }
			h1 { margin: 0 0 6px 0; }
			p { margin: 0 0 16px 0; color: #475569; }
			.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(600px, 1fr)); gap: 14px; align-items: stretch; }
			.panel { display: flex; flex-direction: column; border: 1px solid #e2e8f0; border-radius: 8px; background: #ffffff; box-shadow: 0 6px 18px rgba(15, 23, 42, 0.08); overflow: hidden; min-height: 700px; }
			.panel h2 { font-size: 15px; font-weight: 600; margin: 0; padding: 12px 14px; border-bottom: 1px solid #e2e8f0; background: #f8fafc; }
			iframe { width: 100%%; border: 0; flex: 1; min-height: 700px; height: 100%%; background: #111827; }
			@media (max-width: 1100px) { .grid { grid-template-columns: 1fr; } }
		</style>
	</head>
	<body>
		<h1>LiDAR Debug Dashboard</h1>
		<p>Sensor: %s</p>
		<div class="grid">
			<div class="panel"><h2>Background Polar (XY)</h2><iframe src="/debug/lidar/background/polar%s" title="Background Polar"></iframe></div>
			<div class="panel"><h2>Background Heatmap</h2><iframe src="/debug/lidar/background/heatmap%s" title="Background Heatmap"></iframe></div>
			<div class="panel"><h2>Foreground Frame</h2><iframe src="/debug/lidar/foreground%s" title="Foreground Frame"></iframe></div>
			<div class="panel"><h2>Clusters</h2><iframe src="/debug/lidar/clusters%s" title="Clusters"></iframe></div>
			<div class="panel"><h2>Tracks</h2><iframe src="/debug/lidar/tracks%s" title="Tracks"></iframe></div>
		</div>
	</body>
	</html>`, sensorID, sensorID, qs, qs, qs, qs, qs)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(doc))
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
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
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
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Background Heatmap", Theme: "dark", Width: "900px", Height: "900px"}),
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
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Clusters", Theme: "dark", Width: "900px", Height: "900px"}),
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
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Tracks", Theme: "dark", Width: "900px", Height: "900px"}),
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
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Foreground Frame", Theme: "dark", Width: "900px", Height: "900px"}),
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
// Query params: sensor_id (required), snapshot_id (optional), out (optional file path)
func (ws *WebServer) handleExportSnapshotASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	outPath := r.URL.Query().Get("out")
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

	// Validate and sanitize output path
	if outPath == "" {
		outPath = filepath.Join(os.TempDir(), fmt.Sprintf("bg_snapshot_%s_%d.asc", sensorID, snap.TakenUnixNanos))
	} else {
		// If user provides a path, ensure it's within temp directory or current working directory
		absOutPath, err := filepath.Abs(outPath)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid output path: %v", err))
			return
		}

		// Validate path is within allowed directories (temp or cwd)
		if err := security.ValidateExportPath(absOutPath); err != nil {
			ws.writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		outPath = absOutPath
	}

	if err := lidar.ExportBgSnapshotToASC(snap, outPath, elevs); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("export error: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "out": outPath})
}

// handleExportNextFrameASC triggers an export to ASC for the next completed LiDARFrame for a sensor.
// Query params: sensor_id (required), out (optional file path)
func (ws *WebServer) handleExportNextFrameASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	outPath := r.URL.Query().Get("out")
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

	// Validate and sanitize output path if provided
	if outPath != "" {
		absOutPath, err := filepath.Abs(outPath)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid output path: %v", err))
			return
		}

		// Validate path is within allowed directories (temp or cwd)
		if err := security.ValidateExportPath(absOutPath); err != nil {
			ws.writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		outPath = absOutPath
	}

	// Set flag to export next frame
	fb.RequestExportNextFrameASC(outPath)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "Will export next completed frame", "out": outPath})
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
	ws.pcapMu.Unlock()

	// Get background manager to show current params
	var bgParams *lidar.BackgroundParams
	if mgr := lidar.GetBackgroundManager(ws.sensorID); mgr != nil {
		params := mgr.GetParams()
		bgParams = &params
	}

	// Load and parse the HTML template from embedded filesystem
	tmpl, err := template.ParseFS(StatusHTML, "status.html")
	if err != nil {
		http.Error(w, "Error loading template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Template data
	data := struct {
		UDPPort          int
		HTTPAddress      string
		ForwardingStatus string
		ParsingStatus    string
		Mode             string
		PCAPSafeDir      string
		Uptime           string
		Stats            *StatsSnapshot
		SensorID         string
		BGParams         *lidar.BackgroundParams
		PCAPFile         string
		PCAPInProgress   bool
	}{
		UDPPort:          ws.udpPort,
		HTTPAddress:      ws.address,
		ForwardingStatus: forwardingStatus,
		ParsingStatus:    parsingStatus,
		Mode:             mode,
		PCAPSafeDir:      ws.pcapSafeDir,
		Uptime:           ws.stats.GetUptime().Round(time.Second).String(),
		Stats:            ws.stats.GetLatestSnapshot(),
		SensorID:         ws.sensorID,
		BGParams:         bgParams,
		PCAPFile:         currentPCAPFile,
		PCAPInProgress:   pcapInProgress,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Error executing template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleLidarPersist triggers manual persistence of a BackgroundGrid snapshot.
// Expects POST with form value or query param `sensor_id`.
func (ws *WebServer) handleLidarPersist(w http.ResponseWriter, r *http.Request) {
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
// Method: POST (or GET for convenience). Query param: sensor_id (required)
func (ws *WebServer) handleAcceptanceReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use POST")
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

	// Accept both JSON and form data
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" || contentType == "application/json; charset=utf-8" {
		// Parse JSON body
		var req struct {
			PCAPFile     string `json:"pcap_file"`
			AnalysisMode bool   `json:"analysis_mode"`
		}
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
	} else {
		// Parse form data (default for HTML forms)
		if err := r.ParseForm(); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid form data: %v", err))
			return
		}
		pcapFile = r.FormValue("pcap_file")
		analysisMode = r.FormValue("analysis_mode") == "true" || r.FormValue("analysis_mode") == "1"
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

	if err := ws.resetBackgroundGrid(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reset background grid: %v", err))
		if restartErr := ws.startLiveListenerLocked(); restartErr != nil {
			log.Printf("Failed to restart live listener after reset error: %v", restartErr)
			return
		}
		return
	}

	if err := ws.startPCAPLocked(pcapFile); err != nil {
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
// Method: GET. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use GET")
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

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	// If in analysis mode, only reset grid if explicitly requested
	ws.pcapMu.Lock()
	analysisMode := ws.pcapAnalysisMode
	ws.pcapAnalysisMode = false // Clear flag when stopping
	ws.pcapMu.Unlock()

	if !analysisMode {
		// Normal mode: always reset grid when stopping
		if err := ws.resetBackgroundGrid(); err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reset background grid: %v", err))
			return
		}
	} else {
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
// Method: GET. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPResumeLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed; use GET")
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
