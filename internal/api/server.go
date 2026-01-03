package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	radar "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/security"
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
	m                 serialmux.SerialMuxInterface
	db                *db.DB
	units             string
	timezone          string
	debugMode         bool
	transitController TransitController // Interface for transit worker control
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
	GetStatus() db.TransitStatus
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

		// Include the listener port (if present) ahead of the path for clarity across multiple servers.
		portPrefix := ""
		if host := r.Host; host != "" {
			if h, p, err := net.SplitHostPort(host); err == nil {
				_ = h // host not used currently
				portPrefix = ":" + p
			}
		}
		if portPrefix == "" {
			if p := r.URL.Port(); p != "" {
				portPrefix = ":" + p
			}
		}
		requestTarget := fmt.Sprintf("%s%s", portPrefix, r.RequestURI)
		log.Printf(
			"[%s] %s %s%s%s %vms",
			statusCodeColor(lrw.statusCode), r.Method,
			colorCyan, requestTarget, colorReset,
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
	s.mux.HandleFunc("/api/generate_report", s.generateReport)
	s.mux.HandleFunc("/api/sites", s.handleSites)
	s.mux.HandleFunc("/api/sites/", s.handleSites)                 // Note trailing slash to match /api/sites and /api/sites/*
	s.mux.HandleFunc("/api/reports/", s.handleReports)             // Report management endpoints
	s.mux.HandleFunc("/api/transit_worker", s.handleTransitWorker) // Transit worker control
	s.mux.HandleFunc("/api/db_stats", s.handleDatabaseStats)       // Database table sizes and disk usage
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
	// special grouping that aggregates all values into a single bucket
	// the server will pass 0 to the DB which treats it as 'all'
	"all": 0,
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

	modelVersion := ""
	if dataSource == "radar_data_transits" {
		modelVersion = r.URL.Query().Get("model_version")
		if modelVersion == "" {
			modelVersion = "rebuild-full"
		}
	}

	// Optional histogram computation parameters
	computeHist := false
	if ch := r.URL.Query().Get("compute_histogram"); ch != "" {
		computeHist = (ch == "1" || strings.ToLower(ch) == "true")
	}
	histBucketSize := 0.0
	if hbs := r.URL.Query().Get("hist_bucket_size"); hbs != "" {
		if v, err := strconv.ParseFloat(hbs, 64); err == nil {
			histBucketSize = v
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'hist_bucket_size' parameter; must be a number")
			return
		}
	}
	histMax := 0.0
	if hm := r.URL.Query().Get("hist_max"); hm != "" {
		if v, err := strconv.ParseFloat(hm, 64); err == nil {
			histMax = v
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'hist_max' parameter; must be a number")
			return
		}
	}

	// Convert histogram params from display units to mps for DB call
	bucketSizeMPS := 0.0
	maxMPS := 0.0
	if computeHist && histBucketSize > 0 {
		bucketSizeMPS = units.ConvertToMPS(histBucketSize, displayUnits)
	}
	if computeHist && histMax > 0 {
		maxMPS = units.ConvertToMPS(histMax, displayUnits)
	}

	result, dbErr := s.db.RadarObjectRollupRange(startUnix, endUnix, groupSeconds, minSpeedMPS, dataSource, modelVersion, bucketSizeMPS, maxMPS)
	if dbErr != nil {
		s.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to retrieve radar stats: %v", dbErr))
		return
	}

	// Convert StartTime to requested timezone (Display timezone) and apply unit conversions.
	// RadarObjectRollupRow.StartTime is stored in UTC by the DB layer.
	for i := range result.Metrics {
		// convert timestamp to display timezone; if conversion fails, keep UTC value
		if displayTimezone != "" {
			if t, err := units.ConvertTime(result.Metrics[i].StartTime, displayTimezone); err == nil {
				result.Metrics[i].StartTime = t
			} else {
				// log and continue with UTC value
				log.Printf("failed to convert start time to timezone %s: %v", displayTimezone, err)
			}
		}

		result.Metrics[i].MaxSpeed = units.ConvertSpeed(result.Metrics[i].MaxSpeed, displayUnits)
		result.Metrics[i].P50Speed = units.ConvertSpeed(result.Metrics[i].P50Speed, displayUnits)
		result.Metrics[i].P85Speed = units.ConvertSpeed(result.Metrics[i].P85Speed, displayUnits)
		result.Metrics[i].P98Speed = units.ConvertSpeed(result.Metrics[i].P98Speed, displayUnits)
	}

	// Build response
	respObj := map[string]interface{}{
		"metrics": result.Metrics,
	}
	if len(result.Histogram) > 0 {
		// convert histogram keys to strings for JSON stability
		histOut := make(map[string]int64, len(result.Histogram))
		for k, v := range result.Histogram {
			// convert bucket start (which is in mps) back to the requested display units
			conv := units.ConvertSpeed(k, displayUnits)
			key := fmt.Sprintf("%.2f", conv)
			// accumulate in case multiple mps bins map to the same display-unit label
			histOut[key] += v
		}
		respObj["histogram"] = histOut
	}

	if err := json.NewEncoder(w).Encode(respObj); err != nil {
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

// handleSites routes site-related requests to appropriate handlers
func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse the path to extract ID if present
	// URL format: /api/sites or /api/sites/123
	path := strings.TrimPrefix(r.URL.Path, "/api/sites")
	path = strings.Trim(path, "/")

	// List or Create
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			s.listSites(w, r)
		case http.MethodPost:
			s.createSite(w, r)
		default:
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Get, Update, or Delete by ID
	id, err := strconv.Atoi(path)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getSite(w, r, id)
	case http.MethodPut:
		s.updateSite(w, r, id)
	case http.MethodDelete:
		s.deleteSite(w, r, id)
	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	_ = r
	sites, err := s.db.GetAllSites()
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve sites: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(sites); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode sites")
		return
	}
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request, id int) {
	_ = r
	site, err := s.db.GetSite(id)
	if err != nil {
		if err.Error() == "site not found" {
			s.writeJSONError(w, http.StatusNotFound, "Site not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve site: %v", err))
		}
		return
	}

	if err := json.NewEncoder(w).Encode(site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site")
		return
	}
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var site db.Site
	if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate required fields
	if site.Name == "" {
		s.writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if site.Location == "" {
		s.writeJSONError(w, http.StatusBadRequest, "location is required")
		return
	}
	if site.CosineErrorAngle == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "cosine_error_angle is required")
		return
	}

	if err := s.db.CreateSite(&site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create site: %v", err))
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site")
		return
	}
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request, id int) {
	var site db.Site
	if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	site.ID = id

	// Validate required fields
	if site.Name == "" {
		s.writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if site.Location == "" {
		s.writeJSONError(w, http.StatusBadRequest, "location is required")
		return
	}
	if site.CosineErrorAngle == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "cosine_error_angle is required")
		return
	}

	if err := s.db.UpdateSite(&site); err != nil {
		if err.Error() == "site not found" {
			s.writeJSONError(w, http.StatusNotFound, "Site not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update site: %v", err))
		}
		return
	}

	if err := json.NewEncoder(w).Encode(site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site")
		return
	}
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request, id int) {
	_ = r
	if err := s.db.DeleteSite(id); err != nil {
		if err.Error() == "site not found" {
			s.writeJSONError(w, http.StatusNotFound, "Site not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete site: %v", err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReportRequest represents the JSON payload for report generation
type ReportRequest struct {
	SiteID         *int    `json:"site_id"`          // Optional: use site configuration
	StartDate      string  `json:"start_date"`       // YYYY-MM-DD format
	EndDate        string  `json:"end_date"`         // YYYY-MM-DD format
	Timezone       string  `json:"timezone"`         // e.g., "US/Pacific"
	Units          string  `json:"units"`            // "mph" or "kph"
	Group          string  `json:"group"`            // e.g., "1h", "4h"
	Source         string  `json:"source"`           // "radar_objects" or "radar_data_transits"
	MinSpeed       float64 `json:"min_speed"`        // minimum speed filter
	Histogram      bool    `json:"histogram"`        // whether to generate histogram
	HistBucketSize float64 `json:"hist_bucket_size"` // histogram bucket size
	HistMax        float64 `json:"hist_max"`         // histogram max value

	// These can be overridden if site_id is not provided
	Location         string  `json:"location"`           // site location
	Surveyor         string  `json:"surveyor"`           // surveyor name
	Contact          string  `json:"contact"`            // contact info
	SpeedLimit       int     `json:"speed_limit"`        // posted speed limit
	SiteDescription  string  `json:"site_description"`   // site description
	CosineErrorAngle float64 `json:"cosine_error_angle"` // radar mounting angle
}

func (s *Server) generateReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse the JSON request body
	var req ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate required fields
	if req.StartDate == "" || req.EndDate == "" {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "start_date and end_date are required")
		return
	}

	// Load site data if site_id is provided
	var site *db.Site
	if req.SiteID != nil {
		var err error
		site, err = s.db.GetSite(*req.SiteID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to load site: %v", err))
			return
		}
	}

	// Set defaults from site or fallback values
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if req.Units == "" {
		req.Units = "mph"
	}
	if req.Group == "" {
		req.Group = "1h"
	}
	if req.Source == "" {
		req.Source = "radar_data_transits"
	}
	if req.HistBucketSize == 0 {
		req.HistBucketSize = 5.0
	}

	// Use site data if available, otherwise use request data or defaults
	location := req.Location
	surveyor := req.Surveyor
	contact := req.Contact
	speedLimit := req.SpeedLimit
	siteDescription := req.SiteDescription
	speedLimitNote := ""
	cosineErrorAngle := req.CosineErrorAngle

	if site != nil {
		location = site.Location
		surveyor = site.Surveyor
		contact = site.Contact
		speedLimit = site.SpeedLimit
		if site.SiteDescription != nil {
			siteDescription = *site.SiteDescription
		}
		if site.SpeedLimitNote != nil {
			speedLimitNote = *site.SpeedLimitNote
		}
		cosineErrorAngle = site.CosineErrorAngle
	}

	// Apply final defaults if still empty
	if location == "" {
		location = "Survey Location"
	}
	if surveyor == "" {
		surveyor = "Surveyor"
	}
	if contact == "" {
		contact = "contact@example.com"
	}
	if speedLimit == 0 {
		speedLimit = 25
	}
	if cosineErrorAngle == 0 {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "cosine_error_angle is required (either from site or in request)")
		return
	}

	// Create unique run ID for organized output folders
	// Include nanoseconds to ensure uniqueness under concurrent load
	now := time.Now()
	runID := fmt.Sprintf("%s-%d", now.Format("20060102-150405"), now.Nanosecond())
	outputDir := fmt.Sprintf("output/%s", runID)

	// Create a config JSON for the PDF generator
	// Note: Not setting file_prefix - let Python auto-generate from source + date range
	config := map[string]interface{}{
		"query": map[string]interface{}{
			"start_date":       req.StartDate,
			"end_date":         req.EndDate,
			"timezone":         req.Timezone,
			"group":            req.Group,
			"units":            req.Units,
			"source":           req.Source,
			"min_speed":        req.MinSpeed,
			"histogram":        req.Histogram,
			"hist_bucket_size": req.HistBucketSize,
			"hist_max":         req.HistMax,
		},
		"site": map[string]interface{}{
			"location":         location,
			"surveyor":         surveyor,
			"contact":          contact,
			"speed_limit":      speedLimit,
			"site_description": siteDescription,
			"speed_limit_note": speedLimitNote,
		},
		"radar": map[string]interface{}{
			"cosine_error_angle": cosineErrorAngle,
		},
		"output": map[string]interface{}{
			"output_dir": outputDir,
			"debug":      s.debugMode,
		},
	}

	// Write config to a temporary file
	// Use nanoseconds to ensure unique filename under concurrent requests
	configFile := filepath.Join(os.TempDir(), fmt.Sprintf("report_config_%d.json", now.UnixNano()))
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to marshal config: %v", err))
		return
	}

	// Validate temp file path (should always pass since we control the temp dir)
	if err := security.ValidateExportPath(configFile); err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Invalid config file path: %v", err))
		return
	}

	if err := os.WriteFile(configFile, configData, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write config file: %v", err))
		return
	}
	// Log the config file and speed_limit_note so we can inspect what is passed to the
	// Python generator in production/debug runs. For tests we preserve the file when
	// PDF_GENERATOR_PYTHON is set so the test can inspect the JSON the server wrote.
	log.Printf("Report config written: %s (site.speed_limit_note=%q)", configFile, speedLimitNote)
	if os.Getenv("PDF_GENERATOR_PYTHON") == "" {
		defer os.Remove(configFile) // Clean up after execution in normal runs
	} else {
		log.Printf("Preserving config file for test inspection: %s", configFile)
	}

	// Get the PDF generator directory
	pdfDir, err := getPDFGeneratorDir()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine PDF generator directory: %v", err))
		return
	}

	// Path to Python binary - allow overriding via PDF_GENERATOR_PYTHON
	// Check locations in priority order:
	// 1. Deployed venv: /opt/velocity-report/.venv/bin/python
	// 2. Development venv: ./.venv/bin/python
	// 3. System python3
	pythonBin := os.Getenv("PDF_GENERATOR_PYTHON")
	if pythonBin == "" {
		deployedPython := "/opt/velocity-report/.venv/bin/python"
		if _, err := os.Stat(deployedPython); err == nil {
			pythonBin = deployedPython
			log.Printf("Using deployed PDF generator python: %s", pythonBin)
		} else {
			repoRoot, _ := os.Getwd()
			defaultPythonBin := filepath.Join(repoRoot, ".venv", "bin", "python")
			if _, err := os.Stat(defaultPythonBin); err == nil {
				pythonBin = defaultPythonBin
				log.Printf("Using development PDF generator python: %s", pythonBin)
			} else {
				pythonBin = "python3"
				log.Printf("PDF generator venv not found, using system python3")
			}
		}
	} else {
		log.Printf("Using overridden PDF generator python: %s", pythonBin)
	}

	// Execute the PDF generator
	cmd := exec.Command(
		pythonBin,
		"-m", "pdf_generator.cli.main",
		configFile,
	)
	cmd.Dir = pdfDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("PDF generation failed: %v\nOutput: %s", err, string(output))
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("PDF generation failed: %v", err))
		return
	}

	// Python auto-generates filename as: velocity.report_{source}_{start_date}_to_{end_date}_report.pdf
	pdfFilename := fmt.Sprintf("velocity.report_%s_%s_to_%s_report.pdf", req.Source, req.StartDate, req.EndDate)

	// Python also generates a ZIP with sources: velocity.report_{source}_{start_date}_to_{end_date}_sources.zip
	zipFilename := fmt.Sprintf("velocity.report_%s_%s_to_%s_sources.zip", req.Source, req.StartDate, req.EndDate)

	// Store relative paths from pdf-generator directory
	relativePdfPath := filepath.Join(outputDir, pdfFilename)
	relativeZipPath := filepath.Join(outputDir, zipFilename)

	// Create report record in database
	siteID := 0
	if req.SiteID != nil {
		siteID = *req.SiteID
	}

	report := &db.SiteReport{
		SiteID:      siteID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Filepath:    relativePdfPath,
		Filename:    pdfFilename,
		ZipFilepath: &relativeZipPath,
		ZipFilename: &zipFilename,
		RunID:       runID,
		Timezone:    req.Timezone,
		Units:       req.Units,
		Source:      req.Source,
	}

	if err := s.db.CreateSiteReport(report); err != nil {
		log.Printf("Failed to create report record: %v", err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create report record")
		return
	}

	// Return report ID instead of streaming file
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":   true,
		"report_id": report.ID,
		"message":   "Report generated successfully",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}

	log.Printf("Successfully generated PDF report (ID: %d): %s", report.ID, pdfFilename)
}

// handleReports routes report-related requests to appropriate handlers
func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse the path to extract ID and action
	// URL formats:
	//   /api/reports - list all recent reports
	//   /api/reports/123 - get report metadata
	//   /api/reports/123/download - download PDF file (legacy, with query param)
	//   /api/reports/123/download/filename.pdf - download file with filename in URL
	//   /api/reports/site/456 - list reports for site 456
	path := strings.TrimPrefix(r.URL.Path, "/api/reports")
	path = strings.Trim(path, "/")

	// List all recent reports
	if path == "" && r.Method == http.MethodGet {
		s.listAllReports(w, r)
		return
	}

	// Handle /api/reports/site/{siteID}
	if strings.HasPrefix(path, "site/") {
		siteIDStr := strings.TrimPrefix(path, "site/")
		siteID, err := strconv.Atoi(siteIDStr)
		if err != nil {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid site ID")
			return
		}
		if r.Method == http.MethodGet {
			s.listSiteReports(w, r, siteID)
		} else {
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Parse report ID and action
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request path")
		return
	}

	reportID, err := strconv.Atoi(parts[0])
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid report ID")
		return
	}

	// Handle download action with optional file type (pdf or zip)
	// Supports both:
	//   /api/reports/123/download?file_type=pdf (legacy with query param)
	//   /api/reports/123/download/velocity.report_*.pdf (new with filename in path)
	if len(parts) >= 2 && parts[1] == "download" {
		if r.Method == http.MethodGet {
			// New format: filename in URL path
			if len(parts) == 3 {
				// Extract file type from filename extension
				filename := parts[2]
				fileType := "pdf"
				if strings.HasSuffix(filename, ".zip") {
					fileType = "zip"
				}
				s.downloadReport(w, r, reportID, fileType)
				return
			}
			// Legacy format: file_type query parameter (defaults to "pdf")
			fileType := r.URL.Query().Get("file_type")
			if fileType == "" {
				fileType = "pdf"
			}
			s.downloadReport(w, r, reportID, fileType)
		} else {
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Get report metadata or delete
	switch r.Method {
	case http.MethodGet:
		s.getReport(w, r, reportID)
	case http.MethodDelete:
		s.deleteReport(w, r, reportID)
	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listAllReports(w http.ResponseWriter, r *http.Request) {
	_ = r
	reports, err := s.db.GetRecentReportsAllSites(15)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve reports: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(reports); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode reports")
		return
	}
}

func (s *Server) listSiteReports(w http.ResponseWriter, r *http.Request, siteID int) {
	_ = r
	reports, err := s.db.GetRecentReportsForSite(siteID, 5)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve reports: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(reports); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode reports")
		return
	}
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request, reportID int) {
	_ = r
	report, err := s.db.GetSiteReport(reportID)
	if err != nil {
		if err.Error() == "report not found" {
			s.writeJSONError(w, http.StatusNotFound, "Report not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve report: %v", err))
		}
		return
	}

	if err := json.NewEncoder(w).Encode(report); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode report")
		return
	}
}

func (s *Server) downloadReport(w http.ResponseWriter, r *http.Request, reportID int, fileType string) {
	_ = r
	// Validate file type
	if fileType != "pdf" && fileType != "zip" {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "Invalid file_type parameter. Must be 'pdf' or 'zip'")
		return
	}

	// Get report metadata from database
	report, err := s.db.GetSiteReport(reportID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "report not found" {
			s.writeJSONError(w, http.StatusNotFound, "Report not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve report: %v", err))
		}
		return
	}

	// Get the PDF generator directory
	pdfDir, err := getPDFGeneratorDir()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine PDF generator directory: %v", err))
		return
	}

	// Determine which file to serve based on file_type
	var filePath, filename, contentType string

	if fileType == "zip" {
		// Check if ZIP file exists
		if report.ZipFilepath == nil || *report.ZipFilepath == "" {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusNotFound, "ZIP file not available for this report")
			return
		}
		filePath = filepath.Join(pdfDir, *report.ZipFilepath)
		filename = *report.ZipFilename
		contentType = "application/zip"
	} else {
		// Default to PDF
		filePath = filepath.Join(pdfDir, report.Filepath)
		filename = report.Filename
		contentType = "application/pdf"
	}

	// Validate path is within pdf-generator directory (security check)
	if err := security.ValidatePathWithinDirectory(filePath, pdfDir); err != nil {
		log.Printf("Security: rejected download path %s: %v", filePath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("File not found at path: %s", filePath)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("%s file not found", strings.ToUpper(fileType)))
		return
	}

	// Read the file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Failed to read file: %v", err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read %s file", fileType))
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileData)))

	// Stream the file to the client
	if _, err := w.Write(fileData); err != nil {
		log.Printf("Failed to write file to response: %v", err)
		return
	}

	log.Printf("Successfully downloaded %s file (ID: %d): %s", strings.ToUpper(fileType), reportID, filename)
}

// getPDFGeneratorDir determines the PDF generator directory.
// Can be overridden via PDF_GENERATOR_DIR env var.
// Default to /opt/velocity-report/tools/pdf-generator for deployed systems,
// or tools/pdf-generator relative to current directory for development.
func getPDFGeneratorDir() (string, error) {
	pdfDir := os.Getenv("PDF_GENERATOR_DIR")
	if pdfDir != "" {
		return pdfDir, nil
	}

	// Check if deployed location exists
	deployedPath := "/opt/velocity-report/tools/pdf-generator"
	if _, err := os.Stat(deployedPath); err == nil {
		return deployedPath, nil
	}

	// Fall back to development location
	repoRoot, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(repoRoot, "tools", "pdf-generator"), nil
}

func (s *Server) deleteReport(w http.ResponseWriter, r *http.Request, reportID int) {
	_ = r
	if err := s.db.DeleteSiteReport(reportID); err != nil {
		if err.Error() == "report not found" {
			s.writeJSONError(w, http.StatusNotFound, "Report not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete report: %v", err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Start launches the HTTP server and blocks until the provided context is done
// or the server returns an error. It installs the same static file and SPA
// handlers used previously in the cmd/radar binary.
// handleDatabaseStats returns database table sizes and disk usage statistics.
// GET: returns { total_size_mb: float, tables: [{name, row_count, size_mb}, ...] }
func (s *Server) handleDatabaseStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if s.db == nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	stats, err := s.db.GetDatabaseStats()
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get database stats: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}
}

// handleTransitWorker provides API endpoints for controlling the transit worker.
// GET: returns current state { enabled: bool }
// POST: with { enabled: bool } to update state, optionally { trigger: true } for manual run
func (s *Server) handleTransitWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if transit controller is available
	if s.transitController == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "Transit worker not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Return current status including last run time and error
		status := s.transitController.GetStatus()
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("failed to encode transit worker status: %v", err)
		}

	case http.MethodPost:
		// Update state or trigger manual run
		var req struct {
			Enabled *bool `json:"enabled"`
			Trigger bool  `json:"trigger"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Update enabled state if provided
		if req.Enabled != nil {
			s.transitController.SetEnabled(*req.Enabled)
			status := "disabled"
			if *req.Enabled {
				status = "enabled"
			}
			log.Printf("Transit worker %s via API", status)
		}

		// Trigger manual run if requested
		if req.Trigger {
			s.transitController.TriggerManualRun()
			log.Printf("Transit worker manual run triggered via API")
		}

		// Return updated status
		status := s.transitController.GetStatus()
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("failed to encode transit worker response: %v", err)
		}

	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// Note: Start retrieves the mux by calling s.ServeMux(). ServeMux() returns
// the Server's stored *http.ServeMux (creating and storing it on first
// call). Callers are therefore free to call s.ServeMux() and register
// additional admin/diagnostic routes before invoking Start — those routes
// will be preserved and served. This avoids losing preconfigured routes when
// starting the server.
func (s *Server) Start(ctx context.Context, listen string, devMode bool) error {
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
				fullPath := filepath.Join(buildDir, requestedPath)

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
