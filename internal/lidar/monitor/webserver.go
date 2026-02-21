package monitor

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/version"
)

// ParamDef defines a configuration parameter for display and editing
type ParamDef struct {
	Key    string      // JSON key
	Label  string      // Display label
	Value  interface{} // Current value
	Format string      // Printf format string (optional)
}

//go:embed assets/*
var EchartsAssets embed.FS

//go:embed html/status.html
var StatusHTML embed.FS

//go:embed html/dashboard.html
var dashboardHTML string

//go:embed html/regions_dashboard.html
var regionsDashboardHTML string

//go:embed html/sweep_dashboard.html
var sweepDashboardHTML string

const echartsAssetsPrefix = "/assets/"

// DataSource, DataSourceLive, DataSourcePCAP, DataSourcePCAPAnalysis
// are now defined in datasource.go

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
	vrlogSafeDir      string // Safe directory for VRLOG file access
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
	pcapMu               sync.Mutex
	pcapInProgress       bool
	pcapCancel           context.CancelFunc
	pcapDone             chan struct{}
	pcapAnalysisMode     bool // When true, preserve grid after PCAP completion
	pcapDisableRecording bool // When true, skip VRLOG recording during PCAP replay
	pcapSpeedMode        string
	pcapSpeedRatio       float64
	pcapLastRunID        string // Last analysis run ID from PCAP replay (protected by pcapMu)

	// PCAP progress tracking (protected by pcapMu)
	pcapCurrentPacket uint64 // 0-based index of current packet
	pcapTotalPackets  uint64 // Total packets in current PCAP file

	// Track API for tracking endpoints
	trackAPI *TrackAPI

	// In-memory tracker for real-time config access (optional)
	tracker *l5tracks.Tracker
	// Optional classifier reference for live threshold updates.
	classifier *l6objects.TrackClassifier

	// Analysis run manager for PCAP analysis mode
	analysisRunManager *sqlite.AnalysisRunManager

	// Grid plotter for visualization during PCAP replay
	gridPlotter  *GridPlotter
	plotsBaseDir string // Base directory for plot output (e.g., "plots")
	plotsEnabled bool   // Whether plots are enabled for current run

	// latestFgCounts holds counts from the most recent foreground snapshot for status UI.
	fgCountsMu     sync.RWMutex
	latestFgCounts map[string]int

	// dataSourceManager manages data source lifecycle (live UDP, PCAP replay).
	// This is always initialized - either from config or created internally.
	dataSourceManager DataSourceManager

	// PCAP lifecycle callbacks for notifying external components (e.g. visualiser gRPC server)
	onPCAPStarted    func()
	onPCAPStopped    func()
	onPCAPProgress   func(currentPacket, totalPackets uint64)
	onPCAPTimestamps func(startNs, endNs int64)

	// Recording lifecycle callbacks
	onRecordingStart func(runID string)
	onRecordingStop  func(runID string) string

	// Playback control callbacks
	onPlaybackPause   func()
	onPlaybackPlay    func()
	onPlaybackSeek    func(timestampNs int64) error
	onPlaybackRate    func(rate float32)
	onVRLogLoad       func(vrlogPath string) error
	onVRLogStop       func()
	getPlaybackStatus func() *PlaybackStatusInfo

	// Sweep runner for web-triggered parameter sweeps
	sweepRunner SweepRunner

	// Auto-tune runner for web-triggered auto-tuning
	autoTuneRunner AutoTuneRunner

	// HINT runner for human-in-the-loop parameter tuning
	hintRunner HINTRunner

	// Sweep store for persisting sweep results
	sweepStore *sqlite.SweepStore
}

// PlaybackStatusInfo represents the current playback state for API responses.
type PlaybackStatusInfo struct {
	Mode         string  `json:"mode"` // "live", "pcap", "vrlog"
	Paused       bool    `json:"paused"`
	Rate         float32 `json:"rate"`
	Seekable     bool    `json:"seekable"`
	CurrentFrame uint64  `json:"current_frame"`
	TotalFrames  uint64  `json:"total_frames"`
	TimestampNs  int64   `json:"timestamp_ns"`
	LogStartNs   int64   `json:"log_start_ns"`
	LogEndNs     int64   `json:"log_end_ns"`
	VRLogPath    string  `json:"vrlog_path,omitempty"`
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
	Classifier        *l6objects.TrackClassifier
	PCAPSafeDir       string // Safe directory for PCAP file access (restricts path traversal)
	VRLogSafeDir      string // Safe directory for VRLOG file access (restricts path traversal)
	PacketForwarder   *network.PacketForwarder
	UDPListenerConfig network.UDPListenerConfig
	PlotsBaseDir      string // Base directory for plot output (e.g., "plots")

	// DataSourceManager allows injecting a custom data source manager.
	// If nil, a RealDataSourceManager is created automatically.
	// Inject a MockDataSourceManager for testing.
	DataSourceManager DataSourceManager

	// OnPCAPStarted is called when a PCAP replay starts successfully.
	// Used to notify the visualiser gRPC server to switch to replay mode.
	OnPCAPStarted func()

	// OnPCAPStopped is called when a PCAP replay stops and the system
	// returns to live mode. Used to notify the visualiser gRPC server.
	OnPCAPStopped func()

	// OnPCAPProgress is called periodically during PCAP replay with the
	// current and total packet counts, enabling progress/seek in the UI.
	OnPCAPProgress func(currentPacket, totalPackets uint64)

	// OnPCAPTimestamps is called after PCAP pre-counting with the first and
	// last capture timestamps, enabling timeline display in the UI.
	OnPCAPTimestamps func(startNs, endNs int64)

	// OnRecordingStart is called when VRLOG recording starts for an analysis run.
	// The callback receives the run ID and should start the recorder.
	OnRecordingStart func(runID string)

	// OnRecordingStop is called when VRLOG recording stops.
	// The callback receives the run ID and should return the path to the recorded VRLOG.
	OnRecordingStop func(runID string) string

	// Playback control callbacks
	OnPlaybackPause   func()
	OnPlaybackPlay    func()
	OnPlaybackSeek    func(timestampNs int64) error
	OnPlaybackRate    func(rate float32)
	OnVRLogLoad       func(vrlogPath string) error
	OnVRLogStop       func()
	GetPlaybackStatus func() *PlaybackStatusInfo
}

// NewWebServer creates a new web server with the provided configuration
func NewWebServer(config WebServerConfig) *WebServer {
	listenerConfig := config.UDPListenerConfig
	vrlogSafeDir := config.VRLogSafeDir
	if vrlogSafeDir == "" {
		vrlogSafeDir = "/var/lib/velocity-report"
	}
	if absDir, err := filepath.Abs(vrlogSafeDir); err == nil {
		vrlogSafeDir = absDir
	}
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
		classifier:        config.Classifier,
		pcapSafeDir:       config.PCAPSafeDir,
		vrlogSafeDir:      vrlogSafeDir,
		packetForwarder:   config.PacketForwarder,
		udpListenerConfig: listenerConfig,
		currentSource:     DataSourceLive,
		latestFgCounts:    make(map[string]int),
		plotsBaseDir:      config.PlotsBaseDir,
		onPCAPStarted:     config.OnPCAPStarted,
		onPCAPStopped:     config.OnPCAPStopped,
		onPCAPProgress:    config.OnPCAPProgress,
		onPCAPTimestamps:  config.OnPCAPTimestamps,
		onRecordingStart:  config.OnRecordingStart,
		onRecordingStop:   config.OnRecordingStop,
		onPlaybackPause:   config.OnPlaybackPause,
		onPlaybackPlay:    config.OnPlaybackPlay,
		onPlaybackSeek:    config.OnPlaybackSeek,
		onPlaybackRate:    config.OnPlaybackRate,
		onVRLogLoad:       config.OnVRLogLoad,
		onVRLogStop:       config.OnVRLogStop,
		getPlaybackStatus: config.GetPlaybackStatus,
	}

	// Initialize DataSourceManager - use provided one or create RealDataSourceManager
	if config.DataSourceManager != nil {
		ws.dataSourceManager = config.DataSourceManager
	} else {
		ws.dataSourceManager = NewRealDataSourceManager(ws)
	}

	// Initialise TrackAPI if database is configured
	if config.DB != nil {
		ws.trackAPI = NewTrackAPI(config.DB.DB, config.SensorID)
		// Initialize AnalysisRunManager for PCAP analysis runs
		ws.analysisRunManager = sqlite.NewAnalysisRunManager(config.DB.DB, config.SensorID)
		sqlite.RegisterAnalysisRunManager(config.SensorID, ws.analysisRunManager)
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

// SetTracker sets the tracker reference for direct config access via /api/lidar/params.
// Also propagates to trackAPI if available.
func (ws *WebServer) SetTracker(tracker *l5tracks.Tracker) {
	ws.tracker = tracker
	if ws.trackAPI != nil {
		ws.trackAPI.SetTracker(tracker)
	}
}

// SetClassifier sets the classifier reference used by the tracking pipeline.
// This allows live updates of classification thresholds through /api/lidar/params.
func (ws *WebServer) SetClassifier(classifier *l6objects.TrackClassifier) {
	ws.classifier = classifier
}

// SetSweepRunner sets the sweep runner for web-triggered parameter sweeps.
func (ws *WebServer) SetSweepRunner(runner SweepRunner) {
	ws.sweepRunner = runner
}

// SetAutoTuneRunner sets the auto-tune runner for web-triggered auto-tuning.
func (ws *WebServer) SetAutoTuneRunner(runner AutoTuneRunner) {
	ws.autoTuneRunner = runner
}

// SetHINTRunner sets the HINT runner for human-in-the-loop parameter tuning.
func (ws *WebServer) SetHINTRunner(runner HINTRunner) {
	ws.hintRunner = runner
}

// SetSweepStore sets the sweep store for persisting sweep results.
func (ws *WebServer) SetSweepStore(store *sqlite.SweepStore) {
	ws.sweepStore = store
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

	snap := l3grid.GetForegroundSnapshot(sensorID)
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

func (ws *WebServer) resetBackgroundGrid() error {
	mgr := l3grid.GetBackgroundManager(ws.sensorID)
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
	fb := l2frames.GetFrameBuilder(ws.sensorID)
	if fb != nil {
		fb.Reset()
	}
}

// resetAllState performs a comprehensive reset of all processing state
// when switching data sources. This includes the background grid, frame
// builder, tracker, and any other stateful components.
func (ws *WebServer) resetAllState() error {
	// Reset frame builder first to discard any in-flight frames
	ws.resetFrameBuilder()

	// Reset background grid
	if err := ws.resetBackgroundGrid(); err != nil {
		return err
	}

	// Reset tracker to clear Kalman filter state and restart track IDs from 1
	if ws.tracker != nil {
		ws.tracker.Reset()
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

// route defines a single HTTP route with pattern and handler.
type route struct {
	pattern string
	handler http.HandlerFunc
}

// withDB wraps a handler and returns 503 Service Unavailable if the
// WebServer's database connection is nil. This replaces conditional route
// registration that checked ws.db != nil.
func (ws *WebServer) withDB(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws.db == nil {
			ws.writeJSONError(w, http.StatusServiceUnavailable, "database not available")
			return
		}
		next(w, r)
	}
}

// featureGate wraps a handler and returns 404 Not Found unless the specified
// environment variable is set to "1". Use for destructive or experimental
// endpoints that should only be available during development.
func featureGate(envVar string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv(envVar) != "1" {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

// RegisterRoutes registers all Lidar monitor routes on the provided mux
func (ws *WebServer) RegisterRoutes(mux *http.ServeMux) {
	assetsFS, err := fs.Sub(EchartsAssets, "assets")
	if err != nil {
		log.Printf("failed to prepare echarts assets: %v", err)
		assetsFS = nil
	}

	// Core status and health routes
	coreRoutes := []route{
		{"/health", ws.handleHealth},
		{"/api/lidar/monitor", ws.handleStatus},
		{"GET /api/lidar/status", ws.handleLidarStatus},
		{"POST /api/lidar/persist", ws.handleLidarPersist},
	}

	// Snapshot and export routes
	snapshotRoutes := []route{
		{"GET /api/lidar/snapshot", ws.handleLidarSnapshot},
		{"GET /api/lidar/snapshots", ws.handleLidarSnapshots},
		{"POST /api/lidar/snapshots/cleanup", ws.handleLidarSnapshotsCleanup},
		{"/api/lidar/export_snapshot", ws.handleExportSnapshotASC},
		{"/api/lidar/export_next_frame", ws.handleExportNextFrameASC},
		{"/api/lidar/export_frame_sequence", ws.handleExportFrameSequenceASC},
		{"/api/lidar/export_foreground", ws.handleExportForegroundASC},
	}

	// Traffic and acceptance metrics routes
	metricsRoutes := []route{
		{"GET /api/lidar/traffic", ws.handleTrafficStats},
		{"GET /api/lidar/acceptance", ws.handleAcceptanceMetrics},
		{"POST /api/lidar/acceptance/reset", ws.handleAcceptanceReset},
		{"/api/lidar/params", ws.handleTuningParams},
	}

	// Sweep and auto-tune routes
	sweepRoutes := []route{
		{"POST /api/lidar/sweep/start", ws.handleSweepStart},
		{"GET /api/lidar/sweep/status", ws.handleSweepStatus},
		{"POST /api/lidar/sweep/stop", ws.handleSweepStop},
		{"/api/lidar/sweep/auto", ws.handleAutoTune},
		{"POST /api/lidar/sweep/auto/stop", ws.handleAutoTuneStop},
		{"POST /api/lidar/sweep/auto/suspend", ws.handleAutoTuneSuspend},
		{"POST /api/lidar/sweep/auto/resume", ws.handleAutoTuneResume},
		{"GET /api/lidar/sweep/auto/suspended", ws.handleAutoTuneSuspended},
		{"POST /api/lidar/sweep/hint/continue", ws.handleHINTContinue},
		{"POST /api/lidar/sweep/hint/stop", ws.handleHINTStop},
		{"/api/lidar/sweep/hint", ws.handleHINT},
		{"GET /api/lidar/sweep/explain/", ws.handleSweepExplain},
		{"PUT /api/lidar/sweeps/charts", ws.handleSweepCharts},
		{"GET /api/lidar/sweeps/", ws.handleGetSweep},
		{"GET /api/lidar/sweeps", ws.handleListSweeps},
	}

	// Background grid and region routes
	gridRoutes := []route{
		{"GET /api/lidar/grid_status", ws.handleGridStatus},
		{"POST /api/lidar/grid_reset", ws.handleGridReset},
		{"GET /api/lidar/grid_heatmap", ws.handleGridHeatmap},
		{"/api/lidar/background/grid", ws.handleBackgroundGrid},
	}

	// Data source and PCAP replay routes
	pcapRoutes := []route{
		{"GET /api/lidar/data_source", ws.handleDataSource},
		{"POST /api/lidar/pcap/start", ws.handlePCAPStart},
		{"POST /api/lidar/pcap/stop", ws.handlePCAPStop},
		{"POST /api/lidar/pcap/resume_live", ws.handlePCAPResumeLive},
		{"GET /api/lidar/pcap/files", ws.handleListPCAPFiles},
	}

	// Chart API routes (structured JSON data for frontend charts)
	chartRoutes := []route{
		{"/api/lidar/chart/polar", ws.handleChartPolarJSON},
		{"/api/lidar/chart/heatmap", ws.handleChartHeatmapJSON},
		{"/api/lidar/chart/foreground", ws.handleChartForegroundJSON},
		{"/api/lidar/chart/clusters", ws.handleChartClustersJSON},
		{"/api/lidar/chart/traffic", ws.handleChartTrafficJSON},
	}

	// Debug dashboard and visualisation routes
	debugRoutes := []route{
		{"/debug/lidar/sweep", ws.handleSweepDashboard},
		{"/debug/lidar/background/regions", ws.handleBackgroundRegions},
		{"/debug/lidar/background/regions/dashboard", ws.handleBackgroundRegionsDashboard},
		{"/debug/lidar", ws.handleLidarDebugDashboard},
		{"/debug/lidar/background/polar", ws.handleBackgroundGridPolar},
		{"/debug/lidar/background/heatmap", ws.handleBackgroundGridHeatmapChart},
		{"/debug/lidar/foreground", ws.handleForegroundFrameChart},
		{"/debug/lidar/traffic", ws.handleTrafficChart},
		{"/debug/lidar/clusters", ws.handleClustersChart},
		{"/debug/lidar/tracks", ws.handleTracksChart},
	}

	// Playback API routes (VRLOG replay control)
	playbackRoutes := []route{
		{"GET /api/lidar/playback/status", ws.handlePlaybackStatus},
		{"POST /api/lidar/playback/pause", ws.handlePlaybackPause},
		{"POST /api/lidar/playback/play", ws.handlePlaybackPlay},
		{"POST /api/lidar/playback/seek", ws.handlePlaybackSeek},
		{"POST /api/lidar/playback/rate", ws.handlePlaybackRate},
		{"POST /api/lidar/vrlog/load", ws.handleVRLogLoad},
		{"POST /api/lidar/vrlog/stop", ws.handleVRLogStop},
	}

	// Register all route groups
	for _, group := range [][]route{
		coreRoutes, snapshotRoutes, metricsRoutes, sweepRoutes,
		gridRoutes, pcapRoutes, chartRoutes, debugRoutes, playbackRoutes,
	} {
		for _, r := range group {
			mux.HandleFunc(r.pattern, r.handler)
		}
	}

	// ECharts assets (static file serving)
	if assetsFS != nil {
		mux.Handle(echartsAssetsPrefix, http.StripPrefix(echartsAssetsPrefix, http.FileServer(http.FS(assetsFS))))
	}

	// Track API routes (delegate to TrackAPI handlers)
	if ws.trackAPI != nil {
		trackRoutes := []route{
			{"/api/lidar/tracks", ws.trackAPI.handleListTracks},
			{"/api/lidar/tracks/history", ws.trackAPI.handleListTracks},
			{"/api/lidar/tracks/active", ws.trackAPI.handleActiveTracks},
			{"/api/lidar/tracks/metrics", ws.trackAPI.handleTrackingMetrics},
			{"/api/lidar/tracks/", ws.trackAPI.handleTrackByID},
			{"/api/lidar/tracks/summary", ws.trackAPI.handleTrackSummary},
			{"/api/lidar/clusters", ws.trackAPI.handleListClusters},
			{"/api/lidar/observations", ws.trackAPI.handleListObservations},
			{"/api/lidar/tracks/clear", ws.trackAPI.handleClearTracks},
		}
		for _, r := range trackRoutes {
			mux.HandleFunc(r.pattern, r.handler)
		}

		// Highly destructive endpoint: only register when explicitly enabled for development/debug use.
		mux.HandleFunc("/api/lidar/runs/clear", featureGate("VELOCITY_REPORT_ENABLE_DESTRUCTIVE_LIDAR_API", ws.trackAPI.handleClearRuns))
	}

	// Label API routes (delegate to LidarLabelAPI handlers)
	if ws.db != nil {
		labelAPI := api.NewLidarLabelAPI(ws.db.DB)
		labelAPI.RegisterRoutes(mux)
	}

	// Run track API routes (analysis run management and track labelling)
	mux.HandleFunc("/api/lidar/runs/", ws.withDB(ws.handleRunTrackAPI))

	// Scene API routes (scene management for track labelling and auto-tuning)
	mux.HandleFunc("/api/lidar/scenes", ws.withDB(ws.handleScenes))
	mux.HandleFunc("/api/lidar/scenes/", ws.withDB(ws.handleSceneByID))

}

// setupRoutes configures the HTTP routes and handlers
func (ws *WebServer) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleStatus)
	ws.RegisterRoutes(mux)
	return mux
}

// handleTuningParams is the unified LIDAR configuration endpoint for all
// tuning parameters including background subtraction, frame builder, and tracker configuration.
//
// Query params: sensor_id (required)
//
// GET: Returns all configuration parameters including:
//   - Background params: noise_relative, closeness_multiplier, neighbor_confirmation_count, etc.
//   - Frame builder params: buffer_timeout, min_frame_points
//   - Flush params: flush_interval, flush_disable
//   - Tracker params (if tracker available): gating_distance_squared, process_noise_pos, etc.
//
// POST: Accepts partial JSON updates. All fields are optional; only non-nil fields are applied.
func (ws *WebServer) handleTuningParams(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	bm := l3grid.GetBackgroundManager(sensorID)
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
			"background_update_fraction":    params.BackgroundUpdateFraction,
			"safety_margin_meters":          params.SafetyMarginMeters,
		}

		// Include tracker config if tracker is available
		if ws.tracker != nil {
			cfg := ws.tracker.Config
			resp["gating_distance_squared"] = cfg.GatingDistanceSquared
			resp["process_noise_pos"] = cfg.ProcessNoisePos
			resp["process_noise_vel"] = cfg.ProcessNoiseVel
			resp["measurement_noise"] = cfg.MeasurementNoise
			resp["occlusion_cov_inflation"] = cfg.OcclusionCovInflation
			resp["hits_to_confirm"] = cfg.HitsToConfirm
			resp["max_misses"] = cfg.MaxMisses
			resp["max_misses_confirmed"] = cfg.MaxMissesConfirmed
			resp["max_tracks"] = cfg.MaxTracks
			resp["max_reasonable_speed_mps"] = cfg.MaxReasonableSpeedMps
			resp["max_position_jump_meters"] = cfg.MaxPositionJumpMeters
			resp["max_predict_dt"] = cfg.MaxPredictDt
			resp["max_covariance_diag"] = cfg.MaxCovarianceDiag
			resp["min_points_for_pca"] = cfg.MinPointsForPCA
			resp["obb_heading_smoothing_alpha"] = cfg.OBBHeadingSmoothingAlpha
			resp["obb_aspect_ratio_lock_threshold"] = cfg.OBBAspectRatioLockThreshold
			resp["max_track_history_length"] = cfg.MaxTrackHistoryLength
			resp["max_speed_history_length"] = cfg.MaxSpeedHistoryLength
			resp["merge_size_ratio"] = cfg.MergeSizeRatio
			resp["split_size_ratio"] = cfg.SplitSizeRatio
			resp["deleted_track_grace_period"] = cfg.DeletedTrackGracePeriod.String()
			resp["min_observations_for_classification"] = cfg.MinObservationsForClassification
		}

		if r.URL.Query().Get("format") == "pretty" {
			w.Header().Set("Content-Type", "application/json")
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			if err := enc.Encode(resp); err != nil {
				log.Printf("failed to encode response: %v", err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	case http.MethodPost:
		// @TODO(config-parity): align this POST body with the canonical tuning schema
		// (internal/config/tuning.go:TuningConfig and config/tuning.defaults.json)
		// and keep fields in canonical key order.
		// Missing keys: buffer_timeout, min_frame_points, flush_interval, background_flush,
		// height_band_floor, height_band_ceiling, remove_ground,
		// max_cluster_diameter, min_cluster_diameter, max_cluster_aspect_ratio.
		var body struct {
			// Background params
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
			BackgroundUpdateFraction   *float64 `json:"background_update_fraction"`
			SafetyMarginMeters         *float64 `json:"safety_margin_meters"`
			// Tracker params
			GatingDistanceSquared *float64 `json:"gating_distance_squared"`
			ProcessNoisePos       *float64 `json:"process_noise_pos"`
			ProcessNoiseVel       *float64 `json:"process_noise_vel"`
			MeasurementNoise      *float64 `json:"measurement_noise"`
			OcclusionCovInflation *float64 `json:"occlusion_cov_inflation"`
			HitsToConfirm         *int     `json:"hits_to_confirm"`
			MaxMisses             *int     `json:"max_misses"`
			MaxMissesConfirmed    *int     `json:"max_misses_confirmed"`
			MaxTracks             *int     `json:"max_tracks"`
			// Extended tracker params
			MaxReasonableSpeedMps            *float64 `json:"max_reasonable_speed_mps"`
			MaxPositionJumpMeters            *float64 `json:"max_position_jump_meters"`
			MaxPredictDt                     *float64 `json:"max_predict_dt"`
			MaxCovarianceDiag                *float64 `json:"max_covariance_diag"`
			MinPointsForPCA                  *int     `json:"min_points_for_pca"`
			OBBHeadingSmoothingAlpha         *float64 `json:"obb_heading_smoothing_alpha"`
			OBBAspectRatioLockThreshold      *float64 `json:"obb_aspect_ratio_lock_threshold"`
			MaxTrackHistoryLength            *int     `json:"max_track_history_length"`
			MaxSpeedHistoryLength            *int     `json:"max_speed_history_length"`
			MergeSizeRatio                   *float64 `json:"merge_size_ratio"`
			SplitSizeRatio                   *float64 `json:"split_size_ratio"`
			DeletedTrackGracePeriod          *string  `json:"deleted_track_grace_period"`
			MinObservationsForClassification *int     `json:"min_observations_for_classification"`
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
		if body.BackgroundUpdateFraction != nil {
			p := bm.GetParams()
			p.BackgroundUpdateFraction = float32(*body.BackgroundUpdateFraction)
			if err := bm.SetParams(p); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.SafetyMarginMeters != nil {
			p := bm.GetParams()
			p.SafetyMarginMeters = float32(*body.SafetyMarginMeters)
			if err := bm.SetParams(p); err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Apply tracker config changes if tracker is available
		if ws.tracker != nil {
			if body.GatingDistanceSquared != nil {
				ws.tracker.Config.GatingDistanceSquared = float32(*body.GatingDistanceSquared)
			}
			if body.ProcessNoisePos != nil {
				ws.tracker.Config.ProcessNoisePos = float32(*body.ProcessNoisePos)
			}
			if body.ProcessNoiseVel != nil {
				ws.tracker.Config.ProcessNoiseVel = float32(*body.ProcessNoiseVel)
			}
			if body.MeasurementNoise != nil {
				ws.tracker.Config.MeasurementNoise = float32(*body.MeasurementNoise)
			}
			if body.OcclusionCovInflation != nil {
				ws.tracker.Config.OcclusionCovInflation = float32(*body.OcclusionCovInflation)
			}
			if body.HitsToConfirm != nil {
				ws.tracker.Config.HitsToConfirm = *body.HitsToConfirm
			}
			if body.MaxMisses != nil {
				ws.tracker.Config.MaxMisses = *body.MaxMisses
			}
			if body.MaxMissesConfirmed != nil {
				ws.tracker.Config.MaxMissesConfirmed = *body.MaxMissesConfirmed
			}
			if body.MaxTracks != nil {
				ws.tracker.Config.MaxTracks = *body.MaxTracks
			}
			if body.MaxReasonableSpeedMps != nil {
				ws.tracker.Config.MaxReasonableSpeedMps = float32(*body.MaxReasonableSpeedMps)
			}
			if body.MaxPositionJumpMeters != nil {
				ws.tracker.Config.MaxPositionJumpMeters = float32(*body.MaxPositionJumpMeters)
			}
			if body.MaxPredictDt != nil {
				ws.tracker.Config.MaxPredictDt = float32(*body.MaxPredictDt)
			}
			if body.MaxCovarianceDiag != nil {
				ws.tracker.Config.MaxCovarianceDiag = float32(*body.MaxCovarianceDiag)
			}
			if body.MinPointsForPCA != nil {
				ws.tracker.Config.MinPointsForPCA = *body.MinPointsForPCA
			}
			if body.OBBHeadingSmoothingAlpha != nil {
				ws.tracker.Config.OBBHeadingSmoothingAlpha = float32(*body.OBBHeadingSmoothingAlpha)
			}
			if body.OBBAspectRatioLockThreshold != nil {
				ws.tracker.Config.OBBAspectRatioLockThreshold = float32(*body.OBBAspectRatioLockThreshold)
			}
			if body.MaxTrackHistoryLength != nil {
				ws.tracker.Config.MaxTrackHistoryLength = *body.MaxTrackHistoryLength
			}
			if body.MaxSpeedHistoryLength != nil {
				ws.tracker.Config.MaxSpeedHistoryLength = *body.MaxSpeedHistoryLength
			}
			if body.MergeSizeRatio != nil {
				ws.tracker.Config.MergeSizeRatio = float32(*body.MergeSizeRatio)
			}
			if body.SplitSizeRatio != nil {
				ws.tracker.Config.SplitSizeRatio = float32(*body.SplitSizeRatio)
			}
			if body.DeletedTrackGracePeriod != nil {
				d, err := time.ParseDuration(*body.DeletedTrackGracePeriod)
				if err != nil {
					ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid deleted_track_grace_period: %v", err))
					return
				}
				ws.tracker.Config.DeletedTrackGracePeriod = d
			}
			if body.MinObservationsForClassification != nil {
				ws.tracker.Config.MinObservationsForClassification = *body.MinObservationsForClassification
				if ws.classifier != nil {
					ws.classifier.MinObservations = *body.MinObservationsForClassification
				}
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

		resp := map[string]interface{}{
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
			"background_update_fraction":    cur.BackgroundUpdateFraction,
			"safety_margin_meters":          cur.SafetyMarginMeters,
		}

		// Include tracker config in response if tracker is available
		if ws.tracker != nil {
			cfg := ws.tracker.Config
			resp["gating_distance_squared"] = cfg.GatingDistanceSquared
			resp["process_noise_pos"] = cfg.ProcessNoisePos
			resp["process_noise_vel"] = cfg.ProcessNoiseVel
			resp["measurement_noise"] = cfg.MeasurementNoise
			resp["occlusion_cov_inflation"] = cfg.OcclusionCovInflation
			resp["hits_to_confirm"] = cfg.HitsToConfirm
			resp["max_misses"] = cfg.MaxMisses
			resp["max_misses_confirmed"] = cfg.MaxMissesConfirmed
			resp["max_tracks"] = cfg.MaxTracks
			resp["max_reasonable_speed_mps"] = cfg.MaxReasonableSpeedMps
			resp["max_position_jump_meters"] = cfg.MaxPositionJumpMeters
			resp["max_predict_dt"] = cfg.MaxPredictDt
			resp["max_covariance_diag"] = cfg.MaxCovarianceDiag
			resp["min_points_for_pca"] = cfg.MinPointsForPCA
			resp["obb_heading_smoothing_alpha"] = cfg.OBBHeadingSmoothingAlpha
			resp["obb_aspect_ratio_lock_threshold"] = cfg.OBBAspectRatioLockThreshold
			resp["max_track_history_length"] = cfg.MaxTrackHistoryLength
			resp["max_speed_history_length"] = cfg.MaxSpeedHistoryLength
			resp["merge_size_ratio"] = cfg.MergeSizeRatio
			resp["split_size_ratio"] = cfg.SplitSizeRatio
			resp["deleted_track_grace_period"] = cfg.DeletedTrackGracePeriod.String()
			resp["min_observations_for_classification"] = cfg.MinObservationsForClassification
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
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
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
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
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}

	// Log C: API call timing for grid_reset
	beforeNanos := time.Now().UnixNano()

	// Reset frame builder to clear any buffered frames
	fb := l2frames.GetFrameBuilder(sensorID)
	if fb != nil {
		fb.Reset()
	}

	if err := mgr.ResetGrid(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("reset error: %v", err))
		return
	}

	// Reset tracker to clear Kalman filter state between sweep permutations
	if ws.tracker != nil {
		ws.tracker.Reset()
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
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	bm := l3grid.GetBackgroundManager(sensorID)
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
	// Long-poll support: if wait_for_done=true and PCAP is in progress,
	// block until PCAP completes or the request context is cancelled.
	// This replaces the 500ms polling loop in Client.WaitForPCAPComplete.
	if r.URL.Query().Get("wait_for_done") == "true" {
		ws.pcapMu.Lock()
		done := ws.pcapDone
		inProgress := ws.pcapInProgress
		ws.pcapMu.Unlock()

		if inProgress && done != nil {
			select {
			case <-done:
				// PCAP finished — fall through to return current state
			case <-r.Context().Done():
				// Client disconnected or request timeout
				return
			}
		}
	}

	ws.dataSourceMu.RLock()
	currentSource := ws.currentSource
	currentPCAPFile := ws.currentPCAPFile
	ws.dataSourceMu.RUnlock()

	ws.pcapMu.Lock()
	pcapInProgress := ws.pcapInProgress
	analysisMode := ws.pcapAnalysisMode
	lastRunID := ws.pcapLastRunID
	ws.pcapMu.Unlock()

	response := map[string]interface{}{
		"status":           "ok",
		"data_source":      string(currentSource),
		"pcap_file":        currentPCAPFile,
		"pcap_in_progress": pcapInProgress,
		"analysis_mode":    analysisMode,
		"last_run_id":      lastRunID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleHealth handles the health check endpoint
func (ws *WebServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "service": "lidar", "timestamp": "%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ws *WebServer) handleLidarStatus(w http.ResponseWriter, r *http.Request) {
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
	var bgParams *l3grid.BackgroundParams
	var bgParamsJSON string
	var bgParamDefs []ParamDef
	var bgParamsJSONLines int

	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		params := mgr.GetParams()
		bgParams = &params

		bgParamDefs = []ParamDef{
			// Background subtraction params
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

		// Add tracker params if tracker is available
		if ws.tracker != nil {
			cfg := ws.tracker.Config
			bgParamDefs = append(bgParamDefs,
				ParamDef{"gating_distance_squared", "Gating Distance Squared", cfg.GatingDistanceSquared, "%.2f"},
				ParamDef{"process_noise_pos", "Process Noise Position", cfg.ProcessNoisePos, "%.4f"},
				ParamDef{"process_noise_vel", "Process Noise Velocity", cfg.ProcessNoiseVel, "%.4f"},
				ParamDef{"measurement_noise", "Measurement Noise", cfg.MeasurementNoise, "%.4f"},
				ParamDef{"occlusion_cov_inflation", "Occlusion Covariance Inflation", cfg.OcclusionCovInflation, "%.2f"},
				ParamDef{"hits_to_confirm", "Hits to Confirm Track", cfg.HitsToConfirm, ""},
				ParamDef{"max_misses", "Max Misses (tentative)", cfg.MaxMisses, ""},
				ParamDef{"max_misses_confirmed", "Max Misses (confirmed)", cfg.MaxMissesConfirmed, ""},
				ParamDef{"max_tracks", "Max Tracks", cfg.MaxTracks, ""},
			)
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
	tmpl, err := template.ParseFS(StatusHTML, "html/status.html")
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
		BGParams          *l3grid.BackgroundParams
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
	// Support both query params and form data for sensor_id
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}

	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil || mgr.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}

	// If a PersistCallback is set, build a minimal snapshot object and call it.
	if mgr.PersistCallback != nil {
		snap := &l3grid.BgSnapshot{
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

// handleAcceptanceMetrics returns the range-bucketed acceptance/rejection metrics
// for a given sensor. Query params: sensor_id (required)
func (ws *WebServer) handleAcceptanceMetrics(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background manager for sensor '%s'", sensorID))
		return
	}
	metrics := mgr.GetAcceptanceMetrics()
	if metrics == nil {
		metrics = &l3grid.AcceptanceMetrics{}
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
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
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
	bm := l3grid.GetBackgroundManager(sensorID)
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

	bm := l3grid.GetBackgroundManager(sensorID)
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
