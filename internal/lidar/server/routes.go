package server

import (
	"net/http"
	"os"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"tailscale.com/tsweb"
)

// route defines a single HTTP route with pattern and handler.
type route struct {
	pattern string
	handler http.HandlerFunc
}

// withDB wraps a handler and returns 503 Service Unavailable if the
// Server's database connection is nil. This replaces conditional route
// registration that checked ws.db != nil.
func (ws *Server) withDB(next http.HandlerFunc) http.HandlerFunc {
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
func (ws *Server) RegisterRoutes(mux *http.ServeMux) {
	assetsFS, err := l9endpoints.LegacyAssetsFS()
	if err != nil {
		opsf("failed to prepare echarts assets: %v", err)
		assetsFS = nil
	}

	// Core status and health routes
	coreRoutes := []route{
		{"/health", ws.handleHealth},
		{"/api/lidar/server", ws.handleStatus},
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
		{"GET /api/lidar/settling_eval", ws.handleSettlingEval},
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

	// Note: pprof endpoints (/debug/pprof/*) are registered by tsweb.Debugger()
	// via db.AttachAdminRoutes() on the main mux, and by setupRoutes() on
	// the lidar-only server mux.

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
		labelAPI := api.NewLidarLabelAPI(ws.db)
		labelAPI.RegisterRoutes(mux)
	}

	// Run track API routes (analysis run management and track labelling)
	mux.HandleFunc("/api/lidar/runs/", ws.withDB(ws.handleRunTrackAPI))

	// Scene API routes (scene management for track labelling and auto-tuning)
	mux.HandleFunc("/api/lidar/scenes", ws.withDB(ws.handleScenes))
	mux.HandleFunc("/api/lidar/scenes/", ws.withDB(ws.handleSceneByID))

}

// setupRoutes configures the HTTP routes and handlers for the lidar-only
// server. pprof and debug endpoints are registered via tsweb.Debugger(),
// matching the radar admin server's access control (tailscale-only).
func (ws *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleStatus)
	ws.RegisterRoutes(mux)

	// Register pprof and debug endpoints via tsweb.Debugger, which
	// restricts access to loopback and authenticated Tailscale peers
	// — same mechanism as the radar admin server.
	tsweb.Debugger(mux)

	return mux
}
