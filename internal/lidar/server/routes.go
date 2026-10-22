package server

import (
	"net/http"
	"os"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/compatibility"
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
	// Core status and health routes
	coreRoutes := []route{
		{"/health", ws.handleHealth},
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
		{"GET /api/lidar/background/regions", ws.handleBackgroundRegions},
		// SVG beside the report charts on /api/charts/*; the web's LiDAR
		// debug page embeds it.
		{"GET /api/charts/grid", ws.handleGridChart},
	}

	// Data source and PCAP replay routes
	pcapRoutes := []route{
		{"GET /api/lidar/data_source", ws.handleDataSource},
		{"POST /api/lidar/pcap/start", ws.handlePCAPStart},
		// One stop for every replay kind. The two older paths are aliases of
		// this handler, kept so existing clients and docs keep working; error
		// messages name only the canonical route.
		{"POST /api/lidar/replay/stop", ws.handleReplayStop},
		{compatibility.AliasRoute("POST /api/lidar/pcap/stop", "POST /api/lidar/replay/stop"), ws.handleReplayStop},
		{"POST /api/lidar/pcap/resume_live", ws.handlePCAPResumeLive},
		{"GET /api/lidar/pcap/files", ws.handleListPCAPFiles},
		{"GET /api/lidar/configs", ws.handleListTuningConfigs},
	}

	// Capture index routes: what is on the configured volumes, what changed,
	// and which files form a continuous session.
	captureRoutes := []route{
		{"GET /api/lidar/capture/roots", ws.handleCaptureRoots},
		{"POST /api/lidar/capture/scan", ws.handleCaptureScan},
		{"GET /api/lidar/capture/sessions", ws.handleCaptureSessions},
		{"GET /api/lidar/capture/files", ws.handleCaptureFiles},
		{"POST /api/lidar/capture/session/label", ws.handleCaptureSessionLabel},
		{"POST /api/lidar/capture/motion-pass", ws.handleCaptureMotionPass},
		{"POST /api/lidar/capture/motion-pass/missing", ws.handleCaptureMissingMotionPasses},
		{"GET /api/lidar/capture/periods", ws.handleCapturePeriods},
		{"GET /api/lidar/capture/jobs", ws.handleCaptureJobs},
		{"POST /api/lidar/capture/jobs/cancel", ws.handleCaptureJobCancel},
		{"GET /api/lidar/clip-locations", ws.handleClipLocations},
		{compatibility.AliasRoute("GET /api/lidar/scene-map", "GET /api/lidar/clip-locations"), ws.handleClipLocations},
	}

	// Chart API routes (structured JSON data for frontend charts)
	chartRoutes := []route{
		{"/api/lidar/chart/polar", ws.handleChartPolarJSON},
		{"/api/lidar/chart/heatmap", ws.handleChartHeatmapJSON},
		{"/api/lidar/chart/foreground", ws.handleChartForegroundJSON},
		{"/api/lidar/chart/clusters", ws.handleChartClustersJSON},
		{"/api/lidar/chart/traffic", ws.handleChartTrafficJSON},
	}

	// Note: pprof endpoints (/debug/pprof/*) are registered by tsweb.Debugger()
	// via db.AttachAdminRoutes() on the main mux.

	// Playback API routes (VRLOG replay control)
	playbackRoutes := []route{
		{"GET /api/lidar/playback/status", ws.handlePlaybackStatus},
		{"POST /api/lidar/playback/pause", ws.handlePlaybackPause},
		{"POST /api/lidar/playback/play", ws.handlePlaybackPlay},
		{"POST /api/lidar/playback/seek", ws.handlePlaybackSeek},
		{"POST /api/lidar/playback/rate", ws.handlePlaybackRate},
		{"POST /api/lidar/vrlog/load", ws.handleVRLogLoad},
		{compatibility.AliasRoute("POST /api/lidar/vrlog/stop", "POST /api/lidar/replay/stop"), ws.handleReplayStop},
	}

	// Register all route groups
	for _, group := range [][]route{
		coreRoutes, snapshotRoutes, metricsRoutes, sweepRoutes,
		gridRoutes, pcapRoutes, captureRoutes, chartRoutes, playbackRoutes,
	} {
		for _, r := range group {
			mux.HandleFunc(r.pattern, r.handler)
		}
	}

	// Track API routes (delegate to TrackAPI handlers)
	if ws.trackAPI != nil {
		trackRoutes := []route{
			{"/api/lidar/tracks", ws.trackAPI.handleListTracks},
			{compatibility.AliasRoute("/api/lidar/tracks/history", "/api/lidar/tracks"), ws.trackAPI.handleListTracks},
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

	// Clip API routes (evaluation windows for track labelling and auto-tuning)
	mux.HandleFunc("/api/lidar/clips", ws.withDB(ws.handleClips))
	mux.HandleFunc("/api/lidar/clips/", ws.withDB(ws.handleClipByID))
	mux.HandleFunc("/api/lidar/segments/finders", ws.withDB(ws.handleSegmentFinders))
	mux.HandleFunc("/api/lidar/segments/selectors", ws.withDB(ws.handleSegmentSelectors))
	mux.HandleFunc("/api/lidar/segments/strip", ws.withDB(ws.handleSegmentStrip))
	mux.HandleFunc("/api/lidar/segments/", ws.withDB(ws.handleSegmentByID))
	mux.HandleFunc("/api/lidar/segments", ws.withDB(ws.handleSegments))
	mux.HandleFunc("/api/lidar/annotations/packs", ws.handleAnnotationPacks)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/packs", "/api/lidar/annotations/packs"), ws.handleAnnotationPacks)
	mux.HandleFunc("/api/lidar/annotations/physical", ws.handlePhysicalReferences)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical", "/api/lidar/annotations/physical"), ws.handlePhysicalReferences)
	mux.HandleFunc("/api/lidar/annotations/features", ws.handleFeatureAnnotations)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/features", "/api/lidar/annotations/features"), ws.handleFeatureAnnotations)
	mux.HandleFunc("/api/lidar/annotations/features/pose-proposal", ws.handleFacetPoseProposal)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/features/pose-proposal", "/api/lidar/annotations/features/pose-proposal"), ws.handleFacetPoseProposal)
	mux.HandleFunc("/api/lidar/annotations/physical/validate", ws.handlePhysicalEdit(false))
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical/validate", "/api/lidar/annotations/physical/validate"), ws.handlePhysicalEdit(false))
	mux.HandleFunc("/api/lidar/annotations/physical/save", ws.handlePhysicalEdit(true))
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical/save", "/api/lidar/annotations/physical/save"), ws.handlePhysicalEdit(true))
	mux.HandleFunc("/api/lidar/annotations/physical/fit", ws.handlePhysicalFit)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical/fit", "/api/lidar/annotations/physical/fit"), ws.handlePhysicalFit)
	mux.HandleFunc("/api/lidar/annotations/physical/review", ws.handlePhysicalReview)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical/review", "/api/lidar/annotations/physical/review"), ws.handlePhysicalReview)
	mux.HandleFunc("/api/lidar/annotations/physical/history", ws.handlePhysicalHistory)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical/history", "/api/lidar/annotations/physical/history"), ws.handlePhysicalHistory)
	mux.HandleFunc("/api/lidar/annotations/physical/restore", ws.handlePhysicalRestore)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/physical/restore", "/api/lidar/annotations/physical/restore"), ws.handlePhysicalRestore)
	mux.HandleFunc("/api/lidar/annotations/split/preview", ws.handleSplitPreview)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/split/preview", "/api/lidar/annotations/split/preview"), ws.handleSplitPreview)
	mux.HandleFunc("/api/lidar/annotations/split/freeze", ws.handleSplitFreeze)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/split/freeze", "/api/lidar/annotations/split/freeze"), ws.handleSplitFreeze)
	mux.HandleFunc("/api/lidar/annotations/splits", ws.handleSplits)
	mux.HandleFunc(compatibility.AliasRoute("/api/annotations/splits", "/api/lidar/annotations/splits"), ws.handleSplits)

	// Site API routes (canonical pose for a located site, e.g. a surveyed
	// intersection midpoint) — distinct from a case's own sensor pose.
	mux.HandleFunc("/api/lidar/sites/", ws.withDB(ws.handleSiteByToken))

}
