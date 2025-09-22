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
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
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

	return mux
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
		Uptime           string
		Stats            *StatsSnapshot
		SensorID         string
	}{
		UDPPort:          ws.udpPort,
		HTTPAddress:      ws.address,
		ForwardingStatus: forwardingStatus,
		ParsingStatus:    parsingStatus,
		Uptime:           ws.stats.GetUptime().Round(time.Second).String(),
		Stats:            ws.stats.GetLatestSnapshot(),
		SensorID:         ws.sensorID,
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

// Close shuts down the web server
func (ws *WebServer) Close() error {
	if ws.server != nil {
		return ws.server.Close()
	}
	return nil
}
