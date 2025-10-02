package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	radar "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
	"github.com/banshee-data/velocity.report/internal/units"
)

// ANSI escape codes for cyan and reset
const colorCyan = "\033[36m"
const colorReset = "\033[0m"
const colorYellow = "\033[33m"
const colorBoldGreen = "\033[1;32m"
const colorBoldRed = "\033[1;31m"

// convertEventAPISpeed applies unit conversion to the Speed field of an EventAPI
func convertEventAPISpeed(event db.EventAPI, targetUnits string) db.EventAPI {
	if event.Speed != nil {
		convertedSpeed := units.ConvertSpeed(*event.Speed, targetUnits)
		event.Speed = &convertedSpeed
	}
	return event
}

type Server struct {
	m        serialmux.SerialMuxInterface
	db       *db.DB
	units    string
	timezone string
	// mux holds the HTTP handlers; storing it here ensures callers that
	// obtain the mux via ServeMux() and register additional admin routes
	// will have those routes preserved when Start uses the mux to run the
	// server.
	mux *http.ServeMux
}

func NewServer(m serialmux.SerialMuxInterface, db *db.DB, units string, timezone string) *Server {
	return &Server{
		m:        m,
		db:       db,
		units:    units,
		timezone: timezone,
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func statusCodeColor(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return colorBoldGreen + strconv.Itoa(statusCode) + colorReset
	case statusCode >= 300 && statusCode < 400:
		return colorYellow + strconv.Itoa(statusCode) + colorReset
	case statusCode >= 400 && statusCode < 500:
		return colorBoldRed + strconv.Itoa(statusCode) + colorReset
	case statusCode >= 500:
		return colorBoldRed + strconv.Itoa(statusCode) + colorReset
	default:
		return strconv.Itoa(statusCode)
	}
}

// LoggingMiddleware logs method, path, query, status, and duration
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{w, http.StatusOK}
		next.ServeHTTP(lrw, r)
		log.Printf(
			"[%s] %s %s%s%s %vms",
			statusCodeColor(lrw.statusCode), r.Method,
			colorCyan, r.RequestURI, colorReset,
			float64(time.Since(start).Nanoseconds())/1e6,
		)
	})
}

// start and end are expected as unix timestamps (seconds). group is a
// human-friendly code that maps to seconds (see supportedGroups below).

func (s *Server) ServeMux() *http.ServeMux {
	if s.mux != nil {
		return s.mux
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/events", s.listEvents)
	s.mux.HandleFunc("/command", s.sendCommandHandler)
	s.mux.HandleFunc("/api/radar_stats", s.showRadarObjectStats)
	s.mux.HandleFunc("/api/config", s.showConfig)
	return s.mux
}

func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	command := r.FormValue("command")

	if err := s.m.SendCommand(command); err != nil {
		http.Error(w, "Failed to send command", http.StatusInternalServerError)
		return
	}
	if _, err := io.WriteString(w, "Command sent successfully"); err != nil {
		log.Printf("failed to write command response: %v", err)
	}
}

func (s *Server) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("failed to encode json error response: %v", err)
	}
}

// supportedGroups is a mapping of allowed group tokens to seconds for radar stats grouping
var supportedGroups = map[string]int64{
	"15m": 15 * 60,
	"30m": 30 * 60,
	"1h":  60 * 60,
	"2h":  2 * 60 * 60,
	"3h":  3 * 60 * 60,
	"4h":  4 * 60 * 60,
	"6h":  6 * 60 * 60,
	"8h":  8 * 60 * 60,
	"12h": 12 * 60 * 60,
	"24h": 24 * 60 * 60,
	"2d":  2 * 24 * 60 * 60,
	"3d":  3 * 24 * 60 * 60,
	"7d":  7 * 24 * 60 * 60,
	"14d": 14 * 24 * 60 * 60,
	"28d": 28 * 24 * 60 * 60,
}

func (s *Server) showRadarObjectStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check for units override in query parameter
	displayUnits := s.units // default to CLI-set units
	if u := r.URL.Query().Get("units"); u != "" {
		if units.IsValid(u) {
			displayUnits = u
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'units' parameter. Must be one of: %s", units.GetValidUnitsString()))
			return
		}
	}

	// Check for timezone override in query parameter
	displayTimezone := s.timezone // default to CLI-set timezone
	if tz := r.URL.Query().Get("timezone"); tz != "" {
		if units.IsTimezoneValid(tz) {
			displayTimezone = tz
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'timezone' parameter. Must be one of: %s", units.GetValidTimezonesString()))
			return
		}
	}

	// Check for optional start/end/group parameters for time range + grouping
	// start and end are expected as unix timestamps (seconds). group is a
	// human-friendly code that maps to seconds (see supportedGroups below).
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	groupStr := r.URL.Query().Get("group")

	// All three params are required for range-grouped query
	if startStr == "" || endStr == "" {
		s.writeJSONError(w, http.StatusBadRequest, "'start' and 'end' must be provided for radar stats queries")
		return
	}
	startUnix, err1 := strconv.ParseInt(startStr, 10, 64)
	endUnix, err2 := strconv.ParseInt(endStr, 10, 64)
	if err1 != nil || err2 != nil || startUnix <= 0 || endUnix <= 0 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'start' or 'end' parameter; must be unix timestamps in seconds")
		return
	}

	// If group is not provided, default to the smallest group (15m)
	groupSeconds := supportedGroups["15m"]
	if groupStr != "" {
		var ok bool
		groupSeconds, ok = supportedGroups[groupStr]
		if !ok {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'group' parameter. Supported values: %v", keysOfMap(supportedGroups)))
			return
		}
	}

	// parse optional min_speed query parameter (in display units)
	// if provided, convert to mps before passing to DB
	minSpeedMPS := 0.0 // default: let DB use its internal default when 0
	if minSpeedStr := r.URL.Query().Get("min_speed"); minSpeedStr != "" {
		// parse as float in displayUnits
		if minSpeedValue, err := strconv.ParseFloat(minSpeedStr, 64); err == nil {
			minSpeedMPS = units.ConvertToMPS(minSpeedValue, displayUnits)
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'min_speed' parameter; must be a number")
			return
		}
	}

	// parse optional data source parameter (radar_objects or radar_data_transits)
	// default to radar_objects when empty
	dataSource := r.URL.Query().Get("source")
	if dataSource == "" {
		dataSource = "radar_objects"
	} else if dataSource != "radar_objects" && dataSource != "radar_data_transits" {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'source' parameter; must be 'radar_objects' or 'radar_data_transits'")
		return
	}

	stats, dbErr := s.db.RadarObjectRollupRange(startUnix, endUnix, groupSeconds, minSpeedMPS, dataSource)
	if dbErr != nil {
		s.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to retrieve radar stats: %v", dbErr))
		return
	}

	// Convert StartTime to requested timezone (Display timezone) and apply unit conversions.
	// RadarObjectRollupRow.StartTime is stored in UTC by the DB layer.
	for i := range stats {
		// convert timestamp to display timezone; if conversion fails, keep UTC value
		if displayTimezone != "" {
			if t, err := units.ConvertTime(stats[i].StartTime, displayTimezone); err == nil {
				stats[i].StartTime = t
			} else {
				// log and continue with UTC value
				log.Printf("failed to convert start time to timezone %s: %v", displayTimezone, err)
			}
		}

		stats[i].MaxSpeed = units.ConvertSpeed(stats[i].MaxSpeed, displayUnits)
		stats[i].P50Speed = units.ConvertSpeed(stats[i].P50Speed, displayUnits)
		stats[i].P85Speed = units.ConvertSpeed(stats[i].P85Speed, displayUnits)
		stats[i].P98Speed = units.ConvertSpeed(stats[i].P98Speed, displayUnits)
	}

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write radar stats")
		return
	}
}

// keysOfMap returns the keys of a string->int64 map as a sorted slice for error messages.
func keysOfMap(m map[string]int64) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (s *Server) showConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	config := map[string]interface{}{
		"units":    s.units,
		"timezone": s.timezone,
	}

	if err := json.NewEncoder(w).Encode(config); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write config")
		return
	}
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check for units override in query parameter
	displayUnits := s.units // default to CLI-set units
	if u := r.URL.Query().Get("units"); u != "" {
		if units.IsValid(u) {
			displayUnits = u
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'units' parameter. Must be one of: %s", units.GetValidUnitsString()))
			return
		}
	}

	// Check for timezone override in query parameter
	displayTimezone := s.timezone // default to CLI-set timezone
	if tz := r.URL.Query().Get("timezone"); tz != "" {
		if units.IsTimezoneValid(tz) {
			displayTimezone = tz
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'timezone' parameter. Must be one of: %s", units.GetValidTimezonesString()))
			return
		}
	}

	// TODO: Add timezone conversion for timestamps once database schema includes timestamps
	_ = displayTimezone // Silence unused variable warning for now

	events, err := s.db.Events()
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve events: %v", err))
		return
	}

	// without the EventAPI struct and EventToAPI function the response
	// would be a list of events with their raw fields (Float64, Valid).
	// we control the output format with the EventAPI struct.
	apiEvents := make([]db.EventAPI, len(events))
	for i, e := range events {
		apiEvents[i] = convertEventAPISpeed(db.EventToAPI(e), displayUnits)
	}

	if err := json.NewEncoder(w).Encode(apiEvents); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write events")
		return
	}
}

// Start launches the HTTP server and blocks until the provided context is done
// or the server returns an error. It installs the same static file and SPA
// handlers used previously in the cmd/radar binary.
//
// Note: Start retrieves the mux by calling s.ServeMux(). ServeMux() returns
// the Server's stored *http.ServeMux (creating and storing it on first
// call). Callers are therefore free to call s.ServeMux() and register
// additional admin/diagnostic routes before invoking Start — those routes
// will be preserved and served. This avoids losing preconfigured routes when
// starting the server.
func (s *Server) Start(ctx context.Context, listen string, devMode bool) error {
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
				fullPath := filepath.Join("./web/build", requestedPath)
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

	server := &http.Server{
		Addr:    listen,
		Handler: LoggingMiddleware(mux),
	}

	// Run server in background and wait for either context cancellation or error
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
