package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/api"
	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// ParamDef defines a configuration parameter for display and editing
type ParamDef struct {
	Key    string      // JSON key
	Label  string      // Display label
	Value  interface{} // Current value
	Format string      // Printf format string (optional)
}

// Legacy embedded assets now live in l9endpoints/l10clients/.
var (
	dashboardHTML        = l9endpoints.LegacyDashboardHTML
	regionsDashboardHTML = l9endpoints.LegacyRegionsDashboardHTML
	sweepDashboardHTML   = l9endpoints.LegacySweepDashboardHTML
)

const echartsAssetsPrefix = "/assets/"

// DataSource, DataSourceLive, DataSourcePCAP, DataSourcePCAPAnalysis
// are now defined in datasource.go

type switchError struct {
	status int
	err    error
}

func (e *switchError) Error() string { return e.err.Error() }

func (e *switchError) Unwrap() error { return e.err }

// Server handles the HTTP interface for monitoring LiDAR statistics
// It provides endpoints for health checks and real-time status information
type Server struct {
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
	tuningConfigMu    sync.RWMutex
	tuningConfig      *cfgpkg.TuningConfig

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
	pcapMu                      sync.Mutex
	pcapInProgress              bool
	pcapCancel                  context.CancelFunc
	pcapDone                    chan struct{}
	pcapAnalysisMode            bool        // When true, preserve grid after PCAP completion
	pcapDisableRecording        bool        // When true, skip VRLOG recording during PCAP replay
	pcapBenchmarkMode           atomic.Bool // When true, enable pipeline performance tracing
	pcapDisableTrackPersistence atomic.Bool // When true, skip DB track/observation writes
	pcapSpeedMode               string
	pcapSpeedRatio              float64
	pcapLastRunID               string // Last analysis run ID from PCAP replay (protected by pcapMu)

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
	gridPlotter  *l9endpoints.GridPlotter
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
	onVRLogLoad       func(vrlogPath string) (string, error)
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

// Config contains configuration options for the web server
type Config struct {
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
	TuningConfig      *cfgpkg.TuningConfig

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
	OnVRLogLoad       func(vrlogPath string) (string, error)
	OnVRLogStop       func()
	GetPlaybackStatus func() *PlaybackStatusInfo
}

// NewServer creates a new web server with the provided configuration
func NewServer(config Config) *Server {
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

	ws := &Server{
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
		tuningConfig:      cloneTuningConfig(config.TuningConfig),
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

func cloneTuningConfig(cfg *cfgpkg.TuningConfig) *cfgpkg.TuningConfig {
	if cfg == nil {
		return nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		opsf("monitor: failed to marshal tuning config for clone: %v", err)
		return cfg
	}
	var cloned cfgpkg.TuningConfig
	if err := json.Unmarshal(data, &cloned); err != nil {
		opsf("monitor: failed to unmarshal tuning config for clone: %v", err)
		return cfg
	}
	return &cloned
}

func (ws *Server) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Start begins the HTTP server in a goroutine and handles graceful shutdown
func (ws *Server) Start(ctx context.Context) error {
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
		diagf("Starting HTTP server on %s", ws.address)
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			opsFatalf("failed to start server: %v", err)
		}
	}()

	// Wait for context cancellation to shut down server
	<-ctx.Done()
	diagf("shutting down HTTP server...")

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
		opsf("HTTP server shutdown error: %v, forcing close", err)
		_ = ws.server.Close()
	}

	diagf("HTTP server routine stopped")
	return nil
}

// Close shuts down the web server
func (ws *Server) Close() error {
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
