package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	urlpath "path"
	"path/filepath"
	"strings"
	"time"

	radar "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/db"
	radarcmd "github.com/banshee-data/velocity.report/internal/radar"
	"github.com/banshee-data/velocity.report/internal/security"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

type Server struct {
	m                    serialmux.SerialMuxInterface
	db                   *db.DB
	units                string
	timezone             string
	debugMode            bool
	transitController    TransitController    // Interface for transit worker control
	capabilitiesProvider CapabilitiesProvider // Interface for sensor capability reporting
	tailscale            TailscaleController  // Interface for Tailscale enable/disable/status
	serialManager        *SerialPortManager
	// mux holds the HTTP handlers; storing it here ensures callers that
	// obtain the mux via ServeMux() and register additional admin routes
	// will have those routes preserved when Start uses the mux to run the
	// server.
	mux *http.ServeMux
}

// TransitController is an interface for controlling the transit worker.
// This allows the API server to toggle the worker without direct coupling.
type TransitController interface {
	IsEnabled() bool
	SetEnabled(enabled bool)
	TriggerManualRun()
	TriggerFullHistoryRun()
	GetStatus() db.TransitStatus
}

// LidarCapability describes the runtime state of the LiDAR subsystem.
type LidarCapability struct {
	Enabled bool   `json:"enabled"`
	State   string `json:"state"` // "disabled", "starting", "ready", "error"
}

// Capabilities is the JSON shape returned by /api/capabilities.
type Capabilities struct {
	Radar      bool            `json:"radar"`
	Lidar      LidarCapability `json:"lidar"`
	LidarSweep bool            `json:"lidar_sweep"`
}

// CapabilitiesProvider reports sensor availability at runtime.
// Implementations live outside the api package so the server carries
// no direct dependency on LiDAR internals.
type CapabilitiesProvider interface {
	Capabilities() Capabilities
}

func NewServer(m serialmux.SerialMuxInterface, db *db.DB, units string, timezone string) *Server {
	return &Server{
		m:        m,
		db:       db,
		units:    units,
		timezone: timezone,
	}
}

// SetTransitController sets the transit controller for the server.
// This allows the API to provide UI controls for the transit worker.
func (s *Server) SetTransitController(tc TransitController) {
	s.transitController = tc
}

// SetCapabilitiesProvider sets the provider that reports which sensors
// are active at runtime. When nil, the capabilities endpoint returns
// a radar-only default.
func (s *Server) SetCapabilitiesProvider(cp CapabilitiesProvider) {
	s.capabilitiesProvider = cp
}

// SetSerialManager installs the SerialPortManager that should be used to handle
// hot-reload requests. When not set (nil), the /api/serial/reload endpoint will
// return HTTP 503 Service Unavailable.
func (s *Server) SetSerialManager(manager *SerialPortManager) {
	s.serialManager = manager
}

func (s *Server) currentSerialMux() serialmux.SerialMuxInterface {
	if s.serialManager != nil {
		if mux := s.serialManager.CurrentMux(); mux != nil {
			return mux
		}
	}
	return s.m
}

func (s *Server) ServeMux() *http.ServeMux {
	if s.mux != nil {
		return s.mux
	}
	s.mux = http.NewServeMux()

	// Note: pprof endpoints are provided by tailscale's tsweb via db.AttachAdminRoutes()
	// Usage: go tool pprof http://localhost:8081/debug/pprof/profile?seconds=30

	s.mux.HandleFunc("/events", s.listEvents)
	s.mux.HandleFunc("/admin/radar/command", s.sendCommandHandler) // mutating device control: admin namespace (per cli-restructuring plan), not /api
	s.mux.HandleFunc("/api/commands", s.listCommandsHandler)       // OPS24x command catalogue (dashboard dropdown)
	s.mux.HandleFunc("/api/radar_stats", s.showRadarObjectStats)
	s.mux.HandleFunc("/api/config", s.showConfig)
	s.mux.HandleFunc("/api/capabilities", s.showCapabilities)
	s.mux.HandleFunc("/api/version", s.showVersion)
	s.mux.HandleFunc("/api/generate_report", s.generateReport)
	s.mux.HandleFunc("/api/sites", s.handleSites)
	s.mux.HandleFunc("/api/sites/", s.handleSites) // Note trailing slash to match /api/sites and /api/sites/*
	s.mux.HandleFunc("/api/site_config_periods", s.handleSiteConfigPeriods)
	s.mux.HandleFunc("/api/timeline", s.handleTimeline)
	s.mux.HandleFunc("/api/reports/", s.handleReports)                  // Report management endpoints
	s.mux.HandleFunc("/api/transit_worker", s.handleTransitWorker)      // Transit worker control
	s.mux.HandleFunc("/api/db_stats", s.handleDatabaseStats)            // Database table sizes and disk usage
	s.mux.HandleFunc("/api/charts/timeseries", s.handleChartTimeSeries) // SVG time-series chart
	s.mux.HandleFunc("/api/charts/histogram", s.handleChartHistogram)   // SVG histogram chart
	s.mux.HandleFunc("/api/charts/comparison", s.handleChartComparison) // SVG comparison chart
	s.mux.HandleFunc("/api/tailscale/status", s.handleTailscaleStatus)
	s.mux.HandleFunc("/api/tailscale/enable", s.handleTailscaleEnable)
	s.mux.HandleFunc("/api/tailscale/disable", s.handleTailscaleDisable)

	// Serial configuration endpoints
	s.mux.HandleFunc("/api/serial/configs", s.handleSerialConfigsOrCreate)
	s.mux.HandleFunc("/api/serial/configs/", s.handleSerialConfigByID)
	s.mux.HandleFunc("/api/serial/models", s.handleSensorModels)
	s.mux.HandleFunc("/api/serial/test", s.handleSerialTest)
	s.mux.HandleFunc("/api/serial/devices", s.handleSerialDevices)
	s.mux.HandleFunc("/api/serial/reload", s.handleSerialReload)

	// Map data proxy — fetches Overpass (OSM) data server-side so the browser
	// isn't blocked by the mirrors' User-Agent filtering and missing CORS.
	s.mux.HandleFunc("/api/map/overpass", s.handleMapOverpass)
	return s.mux
}

func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	command := r.FormValue("command")

	// Reject control characters before anything else. SendCommand writes the
	// string verbatim (only appending a trailing newline), so an embedded \n or
	// \r would smuggle multiple serial commands into a single request
	// (e.g. "AX\nOJ"), and a null byte could truncate it. One command per
	// request; control characters are never part of a valid OPS24x command.
	if strings.ContainsFunc(command, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		s.writeJSONError(w, http.StatusBadRequest, "Command must not contain control characters")
		return
	}

	// Advisory check only: the OPS24x command set is config/query-only with no
	// firmware-flash command, so we forward whatever is asked. An unknown
	// command is still sent, but we log a warning so a typo or undocumented
	// command is visible. Access control is localhost binding + planned API
	// auth, not command-string filtering. See internal/radar/commands.go.
	if command != "" && !radarcmd.IsKnownCommand(command) {
		log.Printf("warning: command %q is not in the known OPS24x command catalogue; forwarding anyway", command)
	}

	if err := s.currentSerialMux().SendCommand(command); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to send command")
		return
	}
	if _, err := io.WriteString(w, "Command sent successfully"); err != nil {
		log.Printf("failed to write command response: %v", err)
	}
}

// listCommandsHandler serves GET /api/commands, returning the catalogue of
// documented OPS24x commands as JSON. It backs a dashboard command dropdown.
// The catalogue is advisory (see sendCommandHandler): callers may still send
// commands that are not listed here.
func (s *Server) listCommandsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(radarcmd.KnownCommands); err != nil {
		log.Printf("failed to encode commands response: %v", err)
	}
}

func (s *Server) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("failed to encode json error response: %v", err)
	}
}

// handleSerialReload handles POST /api/serial/reload requests to reconfigure the
// serial port with settings from the database. This endpoint is only available when
// a SerialPortManager has been installed via SetSerialManager.
func (s *Server) handleSerialReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.serialManager == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "Serial reload not available on this instance")
		return
	}

	result, err := s.serialManager.ReloadConfig(r.Context())
	if err != nil {
		log.Printf("serial reload failed: %v", err)
		// "Not configured" cases (no factory/DB, e.g. fixture or disabled
		// modes) are availability conditions, not server faults, so report
		// 503 to match the serialManager == nil branch above. Genuine reload
		// failures (port open, invalid config, DB query) stay 500.
		status := http.StatusInternalServerError
		if errors.Is(err, ErrSerialFactoryNotConfigured) || errors.Is(err, ErrSerialDBNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		s.writeJSONError(w, status, fmt.Sprintf("Failed to reload serial configuration: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("Error encoding serial reload response: %v", err)
	}
}

// Note: Start retrieves the mux by calling s.ServeMux(). ServeMux() returns
// the Server's stored *http.ServeMux (creating and storing it on first
// call). Callers are therefore free to call s.ServeMux() and register
// additional admin/diagnostic routes before invoking Start — those routes
// will be preserved and served. This avoids losing preconfigured routes when
// starting the server.
func (s *Server) Start(ctx context.Context, listen string, devMode bool) error {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}

	return s.startWithListener(ctx, listener, devMode)
}

func (s *Server) startWithListener(ctx context.Context, listener net.Listener, devMode bool) error {
	closeListener := true
	defer func() {
		if closeListener {
			_ = listener.Close()
		}
	}()

	// Store debug mode for use in handlers
	s.debugMode = devMode

	mux := s.ServeMux()

	// read static files from the embedded filesystem in production or from
	// the local ./static in dev for easier iteration without restarting the
	// server
	var staticHandler http.Handler
	if devMode {
		staticHandler = http.FileServer(http.Dir("./static"))
	} else {
		staticHandler = http.FileServer(http.FS(radar.StaticFiles))
	}

	mux.Handle("/favicon.ico", staticHandler)

	// serve frontend app from /app route
	// In dev mode, check build directory exists
	if devMode {
		buildDir := "./web/build"
		if _, err := os.Stat(buildDir); os.IsNotExist(err) {
			return fmt.Errorf("build directory %s does not exist. Run 'cd web && pnpm run build' first.", buildDir)
		}
	}

	// Unified frontend handler that works for both dev and production
	mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /app prefix and normalize path
		path := strings.TrimPrefix(r.URL.Path, "/app")
		if path == "" || path == "/" {
			path = "/index.html"
		}

		// Redirect trailing slash URLs to non-trailing slash for consistent relative path resolution
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			redirectURL := strings.TrimSuffix(r.URL.Path, "/")
			if r.URL.RawQuery != "" {
				redirectURL += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		// Helper function to serve file content
		serveContent := func(content []byte, filename string) {
			http.ServeContent(w, r, filename, time.Time{}, strings.NewReader(string(content)))
		}

		// Helper function to try serving a file from filesystem or embedded FS
		tryServeFile := func(requestedPath string) bool {
			if devMode {
				// Dev mode: serve from filesystem
				buildDir, err := filepath.Abs("./web/build")
				if err != nil {
					log.Printf("Security: failed to resolve build directory: %v", err)
					return false
				}
				// Normalise requestedPath to a relative, cleaned path to ensure buildDir is honoured.
				// Use TrimLeft (not TrimPrefix) to strip all leading separators: a double-slash
				// path such as "//assets/x.js" would survive TrimPrefix and filepath.Clean would
				// preserve the leading separator, making filepath.Join discard buildDir entirely.
				relPath := strings.TrimLeft(requestedPath, "/")
				relPath = filepath.Clean(relPath)
				if filepath.IsAbs(relPath) {
					log.Printf("Security: rejected absolute relPath: %s", relPath)
					return false
				}

				joinedPath := filepath.Join(buildDir, relPath)
				fullPath, err := filepath.Abs(joinedPath)
				if err != nil {
					log.Printf("Security: failed to resolve absolute path: %v", err)
					return false
				}

				// Security: Validate path is within build directory to prevent path traversal
				if err := security.ValidatePathWithinDirectory(fullPath, buildDir); err != nil {
					log.Printf("Security: rejected path %s: %v", fullPath, err)
					return false
				}

				if _, err := os.Stat(fullPath); err == nil {
					http.ServeFile(w, r, fullPath)
					return true
				}
			} else {
				// Production mode: serve from embedded filesystem
				embedPath := filepath.Join("web/build", strings.TrimPrefix(requestedPath, "/"))
				if content, err := radar.WebBuildFiles.ReadFile(embedPath); err == nil {
					serveContent(content, requestedPath)
					return true
				}
			}
			return false
		}

		// Try to serve the requested file directly first
		if tryServeFile(path) {
			return
		}

		// File doesn't exist, try with .html extension for prerendered routes
		if !strings.HasSuffix(path, ".html") {
			htmlPath := path + ".html"
			if tryServeFile(htmlPath) {
				return
			}
		}

		if isFrontendAssetRequest(path) {
			http.NotFound(w, r)
			return
		}

		// Fall back to index.html for SPA routing
		if !tryServeFile("/index.html") {
			http.NotFound(w, r)
		}
	})

	// redirect root to /app
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/app/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	server := &http.Server{Handler: LoggingMiddleware(mux)}

	log.Printf("HTTP server listening on %s", listener.Addr())

	// Run server in background and wait for either context cancellation or error
	errCh := make(chan error, 1)
	closeListener = false
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down HTTP server...")

		// Create a shutdown context with a shorter timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
			// Force close the server if graceful shutdown fails
			if err := server.Close(); err != nil {
				log.Printf("HTTP server force close error: %v", err)
			}
		}

		log.Printf("HTTP server routine stopped")
		return nil
	case err := <-errCh:
		return err
	}
}

func isFrontendAssetRequest(path string) bool {
	if strings.Contains(path, "/_app/") {
		return true
	}

	switch strings.ToLower(urlpath.Ext(path)) {
	case ".css", ".gif", ".ico", ".jpg", ".jpeg", ".js", ".json", ".map", ".png", ".svg", ".txt", ".wasm", ".webmanifest", ".woff", ".woff2":
		return true
	default:
		return false
	}
}
