package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// DataSource, DataSourceLive, DataSourceCapture, DataSourceCaptureAnalysis
// are now defined in datasource.go

type switchError struct {
	status int
	err    error
}

func (e *switchError) Error() string { return e.err.Error() }

func (e *switchError) Unwrap() error { return e.err }

// Server holds the LiDAR pipeline's data sources and serves its HTTP API.
// It has no listener of its own: RegisterRoutes mounts the API on the main
// server's mux, and Start runs the live UDP listener and capture jobs.
type Server struct {
	listenFallback    bool
	stats             *PacketStats
	onReady           func() // run by Start once it is running; see SetOnReady
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
	// datasetDir is the published survey dataset clone that staging writes
	// into. Empty disables staging.
	datasetDir string
	// captureRoots are the capture volumes the operator configured. They come
	// from process configuration and never from a request: the safe-directory
	// boundary is only a boundary while the set of readable roots is fixed
	// outside the API. The UI selects among these; it cannot add one.
	captureRoots []string
	// captureRunner drains the capture job queue. Nil until StartCaptureJobs
	// runs, and left nil when the server has no database to queue work in.
	captureRunner *capjobs.Runner
	// backgrounds holds the settled backgrounds the map alignment asked for.
	backgrounds     backgroundCache
	vrlogSafeDir    string // Safe directory for VRLOG file access
	packetForwarder *network.PacketForwarder
	tuningConfigMu  sync.RWMutex
	tuningConfig    *cfgpkg.TuningConfig
	// tuningConfigDir is Config.TuningConfigDir made absolute; empty means
	// no listing. activeTuningPath and activeTuningFingerprint name the
	// config file last applied by a replay start, relative to the
	// directory, and its fingerprint; empty until one is. Guarded by
	// tuningConfigMu with the config they describe.
	tuningConfigDir         string
	activeTuningPath        string
	activeTuningFingerprint string

	// UDP listener lifecycle (live data source)
	udpListenerConfig network.UDPListenerConfig
	dataSourceMu      sync.RWMutex
	udpListener       *network.UDPListener
	udpListenerCancel context.CancelFunc
	udpListenerDone   chan struct{}
	baseCtxMu         sync.RWMutex
	baseCtx           context.Context

	// state is the authoritative pipeline state: which source is driving the
	// pipeline, whether a replay is running, and whether a VRLOG is being
	// recorded. See pipeline_state.go for the locking contract — stateMu
	// guards only the value and is never held across a call into another
	// subsystem.
	stateMu sync.RWMutex
	state   PipelineState
	// replayActiveFlag mirrors state.ReplayActive for the tracking pipeline's
	// per-frame hot path, which cannot take stateMu. Written only by
	// mutateState; see ReplayActiveFlag.
	replayActiveFlag atomic.Bool
	// analysisModeFlag mirrors state.AnalysisMode() for the same per-frame hot
	// path; see AnalysisModeFlag.
	analysisModeFlag atomic.Bool

	// PCAP replay lifecycle. pcapMu guards only the cancellation handles;
	// everything an observer can see lives in state above.
	pcapMu     sync.Mutex
	pcapCancel context.CancelFunc
	// parkedLiveWatchCancel stops the watcher that hands a parked replay over to
	// live input. Guarded by pcapMu with the other replay-lifecycle fields.
	parkedLiveWatchCancel context.CancelFunc
	// sourceBeforeReplayClaim holds the source a replay claim displaced, so a
	// start that fails after taking the slot can put it back. Guarded by
	// stateMu with the state it describes.
	sourceBeforeReplayClaim     SourceMode
	pcapDone                    chan struct{}
	pcapBenchmarkMode           atomic.Bool // When true, enable pipeline performance tracing
	pcapDisableTrackPersistence atomic.Bool // When true, skip DB track/observation writes
	// clusterMembers is the tuning config's solid_body.full_members, read
	// by the pipeline every frame (KeepClusterMembersRuntime) so a config
	// applied at runtime hands the tracker the members it asks for.
	clusterMembers atomic.Bool

	// Track API for tracking endpoints
	trackAPI *TrackAPI

	// In-memory tracker for real-time config access (optional)
	tracker *l5tracks.Tracker
	// Optional classifier reference for live threshold updates.
	classifier *l6objects.TrackClassifier

	// Analysis run manager for PCAP analysis mode
	runManager *sqlite.RunManager

	// Grid plotter for visualization during PCAP replay
	gridPlotter  *l9endpoints.GridPlotter
	plotsBaseDir string // Base directory for plot output (e.g., "plots")

	// annotationPacksDir is where a run's annotation pack is written when
	// requested through the API. Separate from vrlogSafeDir: one is a read
	// boundary for existing recordings, the other is where new pack
	// directories are created, and conflating them would let a pack write
	// land anywhere a VRLOG could be read from.
	annotationPacksDir string

	// segmentSelectors is the catalogue the segments API ranks with. Nil
	// reads the default one: the repository's file, or the binary's copy.
	segmentSelectors *segments.Catalogue

	// packListings and segmentRecordings keep what the Segments page reads
	// on every load, read again only when the files behind it change. See
	// segment_read_cache.go.
	packListings      packListingCache
	segmentRecordings recordingSeriesCache

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
	// onTuningChange runs once a runtime tuning patch has validated and
	// before any of it is applied; see Config.OnTuningChange.
	onTuningChange func()

	// Recording lifecycle callbacks
	onRecordingStart func(runID string) string
	// recordingAvailable probes whether onRecordingStart can record; nil = yes.
	recordingAvailable func() bool
	onRecordingStop    func(runID string) string

	// Playback control callbacks
	onPlaybackPause func()
	onPlaybackPlay  func()
	onPlaybackSeek  func(timestampNs int64) error
	onPlaybackRate  func(rate float32)
	onVRLogLoad     func(vrlogPath string) (string, error)
	onVRLogStop     func()
	playbackProbe   PlaybackProbe

	// Sweep runner for web-triggered parameter sweeps
	sweepRunner SweepRunner

	// Auto-tune runner for web-triggered auto-tuning
	autoTuneRunner AutoTuneRunner

	// HINT runner for human-in-the-loop parameter tuning
	hintRunner HINTRunner

	// Sweep store for persisting sweep results
	sweepStore *sqlite.SweepStore
}

// PlaybackPosition is the fast-moving replay position owned by the streaming
// layer. Pause/Play/Seek/SetRate arrive as gRPC calls and never pass through
// HTTP, so mirroring this into the monitor server would recreate the very
// divergence PipelineState exists to remove: it is pulled on demand instead.
//
// It deliberately carries no mode field. Mode has exactly one owner — the
// monitor server — and leaving it out is what keeps that enforceable.
type PlaybackPosition struct {
	Paused       bool
	Rate         float32
	Seekable     bool
	CurrentFrame uint64
	TotalFrames  uint64
	TimestampNs  int64
	LogStartNs   int64
	LogEndNs     int64
	ReplayEpoch  uint64
}

// PlaybackProbe is implemented by the visualiser gRPC server.
type PlaybackProbe interface {
	PlaybackPosition() PlaybackPosition
}

// PlaybackStatusInfo represents the current playback state for API responses.
//
// Mode uses the canonical source-mode vocabulary (live, pcap, pcap_analysis,
// vrlog) shared with /api/lidar/data_source, rather than a second near-synonym
// field beside it.
type PlaybackStatusInfo struct {
	Mode         string  `json:"mode"`
	Paused       bool    `json:"paused"`
	Rate         float32 `json:"rate"`
	Seekable     bool    `json:"seekable"`
	CurrentFrame uint64  `json:"current_frame"`
	TotalFrames  uint64  `json:"total_frames"`
	TimestampNs  int64   `json:"timestamp_ns"`
	LogStartNs   int64   `json:"log_start_ns"`
	LogEndNs     int64   `json:"log_end_ns"`
	VRLogPath    string  `json:"vrlog_path,omitempty"`

	ReplayActive      bool       `json:"replay_active"`
	ReplayPass        ReplayPass `json:"replay_pass"`
	ReplayTotalPasses int        `json:"replay_total_passes"`
	GridPreserved     bool       `json:"grid_preserved"`
	ReplayEpoch       uint64     `json:"replay_epoch"`

	Recording       bool   `json:"recording"`
	RecordingPath   string `json:"recording_path,omitempty"`
	RecordingRunID  string `json:"recording_run_id,omitempty"`
	RecordingFrames uint64 `json:"recording_frames"`
}

// Config contains configuration options for the web server
type Config struct {
	// ListenFallback lets Start carry on without the live UDP listener when
	// another process on the host already receives the sensor, serving
	// replays only, instead of failing.
	ListenFallback    bool
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
	// CaptureRoots are additional capture volumes to index. PCAPSafeDir is
	// always treated as a root, so leaving this empty keeps existing
	// deployments unchanged.
	CaptureRoots      []string
	VRLogSafeDir      string // Safe directory for VRLOG file access (restricts path traversal)
	PacketForwarder   *network.PacketForwarder
	UDPListenerConfig network.UDPListenerConfig
	PlotsBaseDir      string // Base directory for plot output (e.g., "plots")
	// AnnotationPacksDir is where POST .../annotation-export writes new pack
	// directories. Empty disables the endpoint rather than falling back to
	// vrlogSafeDir or plotsBaseDir: those directories have their own
	// contents, and a pack write landing among them would be a surprise
	// either way it went.
	AnnotationPacksDir string
	TuningConfig       *cfgpkg.TuningConfig
	// TuningConfigDir is the directory whose .json files GET /api/lidar/configs
	// lists and a replay may start from, by the tuning_config field of
	// POST /api/lidar/pcap/start. Empty disables both.
	TuningConfigDir string
	// SegmentSelectors is the catalogue the segments API ranks with, read
	// once at startup. Nil reads the default one.
	SegmentSelectors *segments.Catalogue
	// DatasetDir is the published survey dataset clone (manifest.json,
	// raw/lidar/) that POST /api/lidar/clips/{id}/stage writes into. It must
	// sit under PCAPSafeDir, as the scene publisher replays from it. Empty
	// disables staging.
	DatasetDir string

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

	// OnTuningChange is called when a runtime tuning patch has validated,
	// before its first value is applied, so anything that has bound evidence
	// to the current tuning (a live observation capture names its extractor
	// by it) can end first. Optional.
	OnTuningChange func()

	// OnPCAPProgress is called periodically during PCAP replay with the
	// current and total packet counts, enabling progress/seek in the UI.
	OnPCAPProgress func(currentPacket, totalPackets uint64)

	// OnPCAPTimestamps is called after PCAP pre-counting with the first and
	// last capture timestamps, enabling timeline display in the UI.
	OnPCAPTimestamps func(startNs, endNs int64)

	// OnRecordingStart is called when VRLOG recording starts for an analysis run.
	// The callback receives the run ID, starts the recorder, and returns the
	// path it is writing to (empty if recording could not start) so the server
	// can report recording state instead of leaving it in a closure local.
	OnRecordingStart func(runID string) string

	// RecordingAvailable reports, without starting anything, whether
	// OnRecordingStart has what it needs. A replay that will record checks it
	// before its first pass, so one that cannot record fails before the
	// settling pass reads the capture rather than after it. Nil means
	// available, and OnRecordingStart's empty path remains the final word.
	RecordingAvailable func() bool

	// OnRecordingStop is called when VRLOG recording stops.
	// The callback receives the run ID and should return the path to the recorded VRLOG.
	OnRecordingStop func(runID string) string

	// Playback control callbacks
	OnPlaybackPause func()
	OnPlaybackPlay  func()
	OnPlaybackSeek  func(timestampNs int64) error
	OnPlaybackRate  func(rate float32)
	OnVRLogLoad     func(vrlogPath string) (string, error)
	OnVRLogStop     func()
	// PlaybackProbe supplies replay position for GET /api/lidar/playback/status.
	// A nil probe yields a zero position; mode and recording still report
	// truthfully from the server's own state.
	PlaybackProbe PlaybackProbe
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
	// Unlike vrlogSafeDir, an empty AnnotationPacksDir stays empty rather than
	// defaulting: it disables the export endpoint, which is the right
	// behaviour for a deployment that never opted in, rather than silently
	// writing packs under a directory meant for something else.
	annotationPacksDir := config.AnnotationPacksDir
	if annotationPacksDir != "" {
		if absDir, err := filepath.Abs(annotationPacksDir); err == nil {
			annotationPacksDir = absDir
		}
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

	tuningConfigDir := config.TuningConfigDir
	if tuningConfigDir != "" {
		if absDir, err := filepath.Abs(tuningConfigDir); err == nil {
			tuningConfigDir = absDir
		}
	}

	ws := &Server{
		listenFallback:     config.ListenFallback,
		stats:              config.Stats,
		forwardingEnabled:  config.ForwardingEnabled,
		forwardAddr:        config.ForwardAddr,
		forwardPort:        config.ForwardPort,
		parsingEnabled:     config.ParsingEnabled,
		udpPort:            config.UDPPort,
		db:                 config.DB,
		sensorID:           config.SensorID,
		parser:             config.Parser,
		frameBuilder:       config.FrameBuilder,
		classifier:         config.Classifier,
		pcapSafeDir:        config.PCAPSafeDir,
		datasetDir:         config.DatasetDir,
		captureRoots:       normaliseCaptureRoots(config.PCAPSafeDir, config.CaptureRoots),
		vrlogSafeDir:       vrlogSafeDir,
		packetForwarder:    config.PacketForwarder,
		tuningConfig:       cloneTuningConfig(config.TuningConfig),
		tuningConfigDir:    tuningConfigDir,
		udpListenerConfig:  listenerConfig,
		state:              newPipelineState(),
		latestFgCounts:     make(map[string]int),
		plotsBaseDir:       config.PlotsBaseDir,
		annotationPacksDir: annotationPacksDir,
		segmentSelectors:   config.SegmentSelectors,
		onPCAPStarted:      config.OnPCAPStarted,
		onPCAPStopped:      config.OnPCAPStopped,
		onTuningChange:     config.OnTuningChange,
		onPCAPProgress:     config.OnPCAPProgress,
		onPCAPTimestamps:   config.OnPCAPTimestamps,
		onRecordingStart:   config.OnRecordingStart,
		recordingAvailable: config.RecordingAvailable,
		onRecordingStop:    config.OnRecordingStop,
		onPlaybackPause:    config.OnPlaybackPause,
		onPlaybackPlay:     config.OnPlaybackPlay,
		onPlaybackSeek:     config.OnPlaybackSeek,
		onPlaybackRate:     config.OnPlaybackRate,
		onVRLogLoad:        config.OnVRLogLoad,
		onVRLogStop:        config.OnVRLogStop,
		playbackProbe:      config.PlaybackProbe,
	}

	ws.clusterMembers.Store(tuningFullMembers(config.TuningConfig))

	// Initialize DataSourceManager - use provided one or create RealDataSourceManager
	if config.DataSourceManager != nil {
		ws.dataSourceManager = config.DataSourceManager
	} else {
		ws.dataSourceManager = NewRealDataSourceManager(ws)
	}

	// Initialise TrackAPI if database is configured
	if config.DB != nil {
		ws.trackAPI = NewTrackAPI(config.DB.DB, config.SensorID)
		// Initialize RunManager for PCAP analysis runs
		ws.runManager = sqlite.NewRunManager(config.DB, config.SensorID)
		sqlite.RegisterRunManager(config.SensorID, ws.runManager)
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

// SetOnReady registers fn to run once Start is running: in live mode, once
// its UDP listener is bound. Call it before Start.
func (ws *Server) SetOnReady(fn func()) {
	ws.onReady = fn
}

// Start runs the live UDP listener and the capture job queue until ctx is
// cancelled, then stops any replay. A live listener it cannot bind is
// returned as an error, so the caller can report the LiDAR subsystem as
// failed, unless ListenFallback lets it serve replays only. The HTTP API is
// served by the main server, on which RegisterRoutes mounts it.
func (ws *Server) Start(ctx context.Context) error {
	ws.setBaseContext(ctx)

	ws.dataSourceMu.Lock()
	if ws.PipelineState().Source == SourceModeLive && ws.udpListener == nil {
		if err := ws.startLiveListenerLocked(); err != nil {
			// The live port is the sensor's and cannot move. A server that
			// tolerates another on the host serves replays without it, the
			// same way a parked replay does when the listener cannot restart.
			if !ws.listenFallback {
				ws.dataSourceMu.Unlock()
				return err
			}
			opsf("monitor: no live listener (%v); serving replays only. Another server is likely receiving the sensor", err)
		}
	}
	ws.dataSourceMu.Unlock()

	// Drain the capture job queue for as long as the server runs. A motion
	// pass reads gigabytes and takes minutes, so it cannot run inside a
	// request; this is where that work actually happens.
	if err := ws.StartCaptureJobs(ctx); err != nil {
		opsf("Warning: capture job runner did not start: %v", err)
	}

	if ws.onReady != nil {
		ws.onReady()
	}

	<-ctx.Done()
	diagf("shutting down the LiDAR pipeline...")

	ws.dataSourceMu.Lock()
	if ws.udpListener != nil {
		ws.stopLiveListenerLocked()
	}
	ws.dataSourceMu.Unlock()

	ws.pcapMu.Lock()
	pcapCancel := ws.pcapCancel
	pcapDone := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapMu.Unlock()

	if pcapCancel != nil {
		pcapCancel()
	}
	if pcapDone != nil {
		<-pcapDone
	}

	diagf("LiDAR pipeline stopped")
	return nil
}

// Close stops the live listener and any replay.
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
	ws.pcapMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}
