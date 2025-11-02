package monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
)

//go:embed status.html
var StatusHTML embed.FS

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
	pcapMode          bool   // True if running in PCAP mode (no UDP listener)

	// PCAP replay state
	pcapMu         sync.Mutex
	pcapInProgress bool
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
	PCAPMode          bool   // True if running in PCAP mode (no UDP listener)
}

// NewWebServer creates a new web server with the provided configuration
func NewWebServer(config WebServerConfig) *WebServer {
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
		pcapMode:          config.PCAPMode,
	}

	ws.server = &http.Server{
		Addr:    ws.address,
		Handler: ws.setupRoutes(),
	}

	return ws
}

func (ws *WebServer) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Start begins the HTTP server in a goroutine and handles graceful shutdown
func (ws *WebServer) Start(ctx context.Context) error {
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

// setupRoutes configures the HTTP routes and handlers
func (ws *WebServer) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/", ws.handleStatus)
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
	mux.HandleFunc("/api/lidar/pcap/start", ws.handlePCAPStart)

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

	if outPath == "" {
		outPath = filepath.Join(os.TempDir(), fmt.Sprintf("bg_snapshot_%s_%d.asc", sensorID, snap.TakenUnixNanos))
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

// handleStatus handles the main status page endpoint
func (ws *WebServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
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

	// Determine mode (PCAP vs Live)
	mode := "Live UDP"
	if ws.pcapMode {
		mode = "PCAP Replay"
	}

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

// handlePCAPStart triggers PCAP file reading in a background goroutine
// Method: POST. Query param: sensor_id (required). Body: {"pcap_file": "/path/to/file.pcap"}
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

	var req struct {
		PCAPFile string `json:"pcap_file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.PCAPFile == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'pcap_file' in request body")
		return
	}

	// Safe directory validation to prevent path traversal attacks
	// Convert safe directory to absolute path
	safeDirAbs, err := filepath.Abs(ws.pcapSafeDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("invalid PCAP safe directory configuration: %v", err))
		return
	}

	// Join user input with safe directory (handles both filenames and relative paths)
	// This prevents absolute paths from bypassing the safe directory
	candidatePath := filepath.Join(safeDirAbs, req.PCAPFile)

	// Resolve to absolute path and clean it
	resolvedPath, err := filepath.Abs(candidatePath)
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid pcap_file path: %v", err))
		return
	}

	// Resolve all symlinks to get the canonical path
	// This prevents symlink-based attacks where a symlink points outside the safe directory
	canonicalPath, err := filepath.EvalSymlinks(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			ws.writeJSONError(w, http.StatusNotFound, "PCAP file not found")
		} else {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("cannot resolve PCAP file path: %v", err))
		}
		return
	}

	// Verify the canonical path is still within the safe directory
	// This prevents traversal attacks like "../../../etc/passwd" and symlink escapes
	if !strings.HasPrefix(canonicalPath, safeDirAbs+string(filepath.Separator)) && canonicalPath != safeDirAbs {
		ws.writeJSONError(w, http.StatusForbidden, fmt.Sprintf("access denied: pcap_file must be within safe directory (%s)", ws.pcapSafeDir))
		return
	}

	// Verify file exists and is a regular file (not a directory or device)
	fileInfo, err := os.Stat(canonicalPath)
	if err != nil {
		if os.IsNotExist(err) {
			ws.writeJSONError(w, http.StatusNotFound, "PCAP file not found")
		} else {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("cannot access PCAP file: %v", err))
		}
		return
	}

	// Ensure it's a regular file, not a directory or device
	if !fileInfo.Mode().IsRegular() {
		ws.writeJSONError(w, http.StatusBadRequest, "pcap_file must be a regular file")
		return
	}

	// Verify file extension (pcap, pcapng)
	ext := filepath.Ext(canonicalPath)
	if ext != ".pcap" && ext != ".pcapng" {
		ws.writeJSONError(w, http.StatusBadRequest, "pcap_file must have .pcap or .pcapng extension")
		return
	}

	// Check if another PCAP replay is already in progress
	ws.pcapMu.Lock()
	if ws.pcapInProgress {
		ws.pcapMu.Unlock()
		ws.writeJSONError(w, http.StatusServiceUnavailable, "PCAP replay already in progress")
		return
	}
	ws.pcapInProgress = true
	ws.pcapMu.Unlock()

	// Start PCAP reading in background goroutine
	go func() {
		defer func() {
			ws.pcapMu.Lock()
			ws.pcapInProgress = false
			ws.pcapMu.Unlock()
		}()

		ctx := context.Background() // TODO: Consider using a cancelable context for stop functionality
		log.Printf("Starting PCAP replay from file: %s (sensor: %s)", canonicalPath, sensorID)

		if err := network.ReadPCAPFile(ctx, canonicalPath, ws.udpPort, ws.parser, ws.frameBuilder, ws.stats); err != nil {
			log.Printf("PCAP replay error: %v", err)
		} else {
			log.Printf("PCAP replay completed successfully: %s", canonicalPath)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "started",
		"sensor_id": sensorID,
		"pcap_file": canonicalPath,
	})
}

// Close shuts down the web server
func (ws *WebServer) Close() error {
	if ws.server != nil {
		return ws.server.Close()
	}
	return nil
}
