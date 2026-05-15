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
	authGate             *authGate // Capability-grant authorization (nil = allow all)
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

// SensorStatus is the per-sensor health snapshot included in the
// capabilities response. Each sensor in the radar/lidar maps carries
// one of these.
type SensorStatus struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"` // "disabled", "starting", "ready", "receiving", "stale", "error"
}

// LidarSensorStatus extends SensorStatus with lidar-specific fields.
type LidarSensorStatus struct {
	SensorStatus
	Sweep bool `json:"sweep"`
}

// Capabilities is the JSON shape returned by /api/capabilities.
// Top-level keys are sensor classes; values are named-object maps
// keyed by a stable, human-assigned sensor name (e.g. "default").
type Capabilities struct {
	Radar map[string]SensorStatus      `json:"radar"`
	Lidar map[string]LidarSensorStatus `json:"lidar"`
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

// SetAuthGate enables capability-grant authorization for the whole
// HTTP surface.  When mode is EnforcementOff (or tc is nil), every
// request is admin and no daemon calls are made.  When mode is
// EnforcementOn, the auth wrapper installed at Start() time gates
// every route that is not on the allowlist (see authAllowlist) at
// CapAdmin by default; explicitly registered View routes (see
// viewRoutes) require only CapView.  This default-deny shape means
// adding a new mutating endpoint cannot accidentally bypass auth.
func (s *Server) SetAuthGate(tc PeerAuthClient, mode CapEnforcement) {
	s.authGate = newAuthGate(tc, mode)
	// One-line arming record so operators can grep journald for the
	// transition.  The wording is referenced from
	// docs/platform/operations/tailscale-remote-access.md.
	if s.authGate.mode == EnforcementOn {
		log.Print("auth: capability enforcement armed (mode=on)")
	}
}

// viewRoutes is the allowlist of read-only endpoints that should
// be reachable by holders of velocity.report/cap/view.  Anything
// not listed here defaults to CapAdmin.  Match is by exact path
// or, when the entry ends in "/", by prefix.
var viewRoutes = map[string]struct{}{
	"/events":                {},
	"/api/commands":          {},
	"/api/events":            {},
	"/api/radar_stats":       {},
	"/api/config":            {},
	"/api/capabilities":      {},
	"/api/version":           {},
	"/api/timeline":          {},
	"/api/db_stats":          {},
	"/api/charts/timeseries": {},
	"/api/charts/histogram":  {},
	"/api/charts/comparison": {},
}

// viewRoutesGetOnly is the allowlist of paths where GET/HEAD/OPTIONS
// require CapView but write methods require CapAdmin.  Used for
// REST collections that mix read and write under one mux entry
// (e.g. /api/sites, /api/reports/).
var viewRoutesGetOnly = []string{
	"/api/sites",
	"/api/sites/",
	"/api/site_config_periods",
	"/api/reports",
	"/api/reports/",
}

// authAllowlist is the set of paths exempted from cap checks
// entirely.  Only paths whose unauthenticated reachability is
// either (a) a deliberate UX feature or (b) a recovery channel
// belong here.
var authAllowlist = []string{
	// SPA static assets — the page itself must be reachable so the
	// app can render an "unauthorized" state.  `/app` is listed in
	// addition to `/app/` because the auth wrapper runs before the
	// ServeMux trailing-slash redirect, so a bare `/app` would
	// otherwise hit default-deny instead of redirecting.
	"/",
	"/app",
	"/app/",
	"/favicon.ico",
	// Tailscale status read so an operator with a botched grant
	// policy can still see the daemon state and recover.  Note
	// that enable/disable are NOT on the allowlist.
	"/api/tailscale/status",
}

// classifyRoute reports the CapKind required for path+method.  Used
// by the wrapper installed at Start() time.  Allowlist hits return
// (false, _).
//
// Match rules for both authAllowlist and viewRoutesGetOnly:
//   - Exact path match.
//   - If the entry ends in "/", it matches any path *strictly under*
//     that prefix.  The root "/" is treated as exact-match only,
//     otherwise it would absorb every URL.
func classifyRoute(path, method string) (gated bool, required CapKind) {
	if pathMatchesAny(path, authAllowlist) {
		return false, 0
	}
	if _, ok := viewRoutes[path]; ok {
		return true, CapView
	}
	if pathMatchesAny(path, viewRoutesGetOnly) {
		switch method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return true, CapView
		default:
			return true, CapAdmin
		}
	}
	// Default-deny: anything not explicitly view-classified requires
	// admin.  This is the property that prevents a forgotten new
	// route from silently bypassing auth.
	return true, CapAdmin
}

// pathMatchesAny applies the exact-or-prefix rule documented on
// classifyRoute.  Note that "/" is exact-match only.
func pathMatchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if p == path {
			return true
		}
		if p != "/" && strings.HasSuffix(p, "/") && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (s *Server) ServeMux() *http.ServeMux {
	if s.mux != nil {
		return s.mux
	}
	s.mux = http.NewServeMux()

	// Note: pprof endpoints are provided by tailscale's tsweb via db.AttachAdminRoutes()
	// Usage: go tool pprof http://localhost:8081/debug/pprof/profile?seconds=30

	// Routes are NOT gated individually here. The auth wrapper
	// installed at Start() time gates the entire mux by classifying
	// each request's path against viewRoutes / viewRoutesGetOnly /
	// authAllowlist (see the maps above). This is intentional: a
	// new route added after this point, including routes attached
	// by external packages via mux.Handle, inherits the default-deny
	// policy automatically.

	s.mux.HandleFunc("/admin/radar/command", s.sendCommandHandler) // mutating device control: admin namespace (per cli-restructuring plan), not /api
	s.mux.HandleFunc("/api/commands", s.listCommandsHandler)       // OPS24x command catalogue (dashboard dropdown)
	s.mux.HandleFunc("/api/events", s.listEvents)                  // radar detection events (DB query, not SSE); renamed from /events to sit under /api
	s.mux.HandleFunc("/api/radar_stats", s.showRadarObjectStats)
	s.mux.HandleFunc("/api/config", s.showConfig)
	s.mux.HandleFunc("/api/capabilities", s.showCapabilities)
	s.mux.HandleFunc("/api/version", s.showVersion)
	s.mux.HandleFunc("/api/generate_report", s.generateReport)
	s.mux.HandleFunc("/api/sites", s.handleSites)
	s.mux.HandleFunc("/api/sites/", s.handleSites)
	s.mux.HandleFunc("/api/site_config_periods", s.handleSiteConfigPeriods)
	s.mux.HandleFunc("/api/timeline", s.handleTimeline)
	s.mux.HandleFunc("/api/reports/", s.handleReports)
	s.mux.HandleFunc("/api/transit_worker", s.handleTransitWorker)
	s.mux.HandleFunc("/api/db_stats", s.handleDatabaseStats)
	s.mux.HandleFunc("/api/charts/timeseries", s.handleChartTimeSeries)
	s.mux.HandleFunc("/api/charts/histogram", s.handleChartHistogram)
	s.mux.HandleFunc("/api/charts/comparison", s.handleChartComparison)
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
	return s.mux
}

// authWrapper wraps next with the configured auth gate, classifying
// each request via classifyRoute.  Returns next unchanged when the
// gate is disabled (mode=off or no PeerAuthClient).  Installed once
// at Start() time so it covers handlers attached after ServeMux()
// was called (e.g. radarSerial.AttachAdminRoutes, the LiDAR routes,
// the offline docs site).
func (s *Server) authWrapper(next http.Handler) http.Handler {
	if s.authGate == nil || s.authGate.mode == EnforcementOff {
		return next
	}
	gate := s.authGate
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gated, required := classifyRoute(r.URL.Path, r.Method)
		if !gated {
			next.ServeHTTP(w, r)
			return
		}
		gate.requireCap(required, next).ServeHTTP(w, r)
	})
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
	// auth, not command-string filtering. We match the two-character code so
	// parameterised commands (e.g. "R>0.25", "C=...") are recognised rather
	// than warned about. See internal/radar/commands.go.
	if command != "" && !radarcmd.IsKnownCommandCode(command) {
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

	// Order: logging on the outside (so 403s from auth are logged),
	// auth on the inside (so the cap check sees the real handler).
	server := &http.Server{Handler: LoggingMiddleware(s.authWrapper(mux))}

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
