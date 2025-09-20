package monitor

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"
	lidar "github.com/banshee-data/velocity.report/internal/lidar"

)

//go:embed status.html
var statusHTML embed.FS

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
	}

	ws.server = &http.Server{
		Addr:    ws.address,
		Handler: ws.setupRoutes(),
	}

	return ws
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

	// Health check endpoint
	mux.HandleFunc("/health", ws.handleHealth)

	// Status page endpoint
	mux.HandleFunc("/", ws.handleStatus)
	mux.HandleFunc("/api/lidar/persist", ws.handleLidarPersist)


	return mux
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
	tmpl, err := template.ParseFS(statusHTML, "status.html")
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
	}{
		UDPPort:          ws.udpPort,
		HTTPAddress:      ws.address,
		ForwardingStatus: forwardingStatus,
		ParsingStatus:    parsingStatus,
		Uptime:           ws.stats.GetUptime().Round(time.Second).String(),
		Stats:            ws.stats.GetLatestSnapshot(),
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

// Close shuts down the web server
func (ws *WebServer) Close() error {
	if ws.server != nil {
		return ws.server.Close()
	}
	return nil
}
