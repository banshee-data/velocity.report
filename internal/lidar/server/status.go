package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/version"
)

// handleGridStatus returns simple statistics about the in-memory BackgroundGrid
// for a sensor: distribution of TimesSeenCount, number of frozen cells, and totals.
// Query params: sensor_id (required)
func (ws *Server) handleGridStatus(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
		return
	}
	status := mgr.GridStatus()
	if status == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "could not compute grid status: check background manager is initialised")
		return
	}
	resp := status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleSettlingEval returns convergence metrics for the background grid's
// settling process. This powers the settling-eval CLI tool and provides
// data-driven guidance for WarmupMinFrames tuning.
// Query params: sensor_id (required)
func (ws *Server) handleSettlingEval(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
		return
	}
	// Use the grid's current frame count as frame number.
	status := mgr.GridStatus()
	frameNumber := 0
	if status != nil {
		if bg, ok := status["background_count"].(int64); ok {
			frameNumber = int(bg)
		}
	}
	metrics := mgr.EvaluateSettling(frameNumber)
	thresholds := l3grid.DefaultSettlingThresholds()
	converged := metrics.IsConverged(thresholds)

	resp := map[string]interface{}{
		"sensor_id":         sensorID,
		"metrics":           metrics,
		"thresholds":        thresholds,
		"converged":         converged,
		"settling_complete": mgr.IsSettlingComplete(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleTrafficStats returns the latest packet/point throughput snapshot.
// Query params: sensor_id (optional; defaults to configured sensor)
func (ws *Server) handleTrafficStats(w http.ResponseWriter, r *http.Request) {
	if ws.stats == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no packet stats available: check the UDP port is receiving data from the sensor")
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
func (ws *Server) handleGridReset(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
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
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not reset grid: %v", err))
		return
	}

	// Reset tracker to clear Kalman filter state between sweep permutations
	if ws.tracker != nil {
		ws.tracker.Reset()
	}

	afterNanos := time.Now().UnixNano()
	elapsedMs := float64(afterNanos-beforeNanos) / 1e6

	diagf("[API:grid_reset] sensor=%s reset_duration_ms=%.3f timestamp=%d",
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
func (ws *Server) handleGridHeatmap(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}

	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
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
		ws.writeJSONError(w, http.StatusInternalServerError, "could not generate heatmap: check grid data is populated")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(heatmap)
}

func (ws *Server) handleDataSource(w http.ResponseWriter, r *http.Request) {
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
				// PCAP finished: fall through to return current state
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
func (ws *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "service": "lidar", "timestamp": "%s"}`, time.Now().UTC().Format(time.RFC3339))
}

func (ws *Server) handleLidarStatus(w http.ResponseWriter, r *http.Request) {
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
func (ws *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/lidar/server" && r.URL.Path != "/" && r.URL.Path != "/api/lidar/server" && r.URL.Path != "/api/lidar/monitor" {
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
	var bgParamsJSONLines int

	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		params := mgr.GetParams()
		bgParams = &params

		cfg := ws.runtimeTuningConfig(mgr)
		if jsonBytes, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			bgParamsJSON = string(jsonBytes)
			bgParamsJSONLines = strings.Count(bgParamsJSON, "\n") + 2
		}
	}

	// Refresh foreground snapshot counts for status rendering.
	ws.updateLatestFgCounts(ws.sensorID)

	// Load and parse the HTML template from embedded filesystem
	statusFS, statusFSErr := l9endpoints.LegacyStatusFS()
	if statusFSErr != nil {
		http.Error(w, "could not load status assets: "+statusFSErr.Error(), http.StatusInternalServerError)
		return
	}
	tmpl, err := template.ParseFS(statusFS, "status.html")
	if err != nil {
		http.Error(w, "could not load status template: "+err.Error(), http.StatusInternalServerError)
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
		BGParamsJSONLines: bgParamsJSONLines,
		PCAPFile:          currentPCAPFile,
		PCAPInProgress:    pcapInProgress,
		PCAPSpeedMode:     pcapSpeedMode,
		PCAPSpeedRatio:    pcapSpeedRatio,
		FgSnapshotCounts:  ws.getLatestFgCounts(),
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "could not render status page: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleLidarPersist triggers manual persistence of a BackgroundGrid snapshot.
// Expects POST with form value or query param `sensor_id`.
func (ws *Server) handleLidarPersist(w http.ResponseWriter, r *http.Request) {
	// Support both query params and form data for sensor_id
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}

	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}

	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil || mgr.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
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
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not persist snapshot: %v", err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "sensor_id": sensorID})
		diagf("Successfully persisted snapshot for sensor '%s'", sensorID)
		return
	}

	ws.writeJSONError(w, http.StatusNotImplemented, "no persist callback configured for this sensor: check server startup configuration")
}

// handleAcceptanceMetrics returns the range-bucketed acceptance/rejection metrics
// for a given sensor. Query params: sensor_id (required)
func (ws *Server) handleAcceptanceMetrics(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
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
			"noise_relative":         params.NoiseRelativeFraction,
			"closeness_multiplier":   params.ClosenessSensitivityMultiplier,
			"neighbour_confirmation": params.NeighbourConfirmationCount,
			"seed_from_first":        params.SeedFromFirstObservation,
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
func (ws *Server) handleAcceptanceReset(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}
	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
		return
	}
	if err := mgr.ResetAcceptanceMetrics(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not reset acceptance metrics: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "sensor_id": sensorID})
}

// handleBackgroundGrid returns the full background grid cells.
// Query params: sensor_id (required)
func (ws *Server) handleBackgroundGrid(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "the sensor_id parameter is required")
		return
	}
	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no background data available for sensor '%s': check it is connected and active", sensorID))
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
func (ws *Server) handleBackgroundRegions(w http.ResponseWriter, r *http.Request) {
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
		ws.writeJSONError(w, http.StatusInternalServerError, "could not get region debug info: check region manager is initialised")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
