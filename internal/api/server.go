package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

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
	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.listEvents)
	mux.HandleFunc("/command", s.sendCommandHandler)
	mux.HandleFunc("/api/radar_stats", s.showRadarObjectStats)
	mux.HandleFunc("/api/config", s.showConfig)
	return mux
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
	io.WriteString(w, "Command sent successfully")
}

func (s *Server) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) showRadarObjectStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	days := 1 // default value
	if d := r.URL.Query().Get("days"); d != "" {
		parsedDays, err := strconv.Atoi(d)
		if err != nil || parsedDays < 1 {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'days' parameter")
			return
		}
		days = parsedDays
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

	// Check for optional start/end/group parameters for time range + grouping
	// start and end are expected as unix timestamps (seconds). group is a
	// human-friendly code that maps to seconds (see supportedGroups below).
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	groupStr := r.URL.Query().Get("group")

	// mapping of allowed group tokens to seconds
	supportedGroups := map[string]int64{
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

	var stats []db.RadarObjectsRollupRow
	var dbErr error

	if startStr != "" || endStr != "" || groupStr != "" {
		// all three params are required for range-grouped query
		if startStr == "" || endStr == "" || groupStr == "" {
			s.writeJSONError(w, http.StatusBadRequest, "'start', 'end', and 'group' must all be provided for grouped range queries")
			return
		}
		startUnix, err1 := strconv.ParseInt(startStr, 10, 64)
		endUnix, err2 := strconv.ParseInt(endStr, 10, 64)
		if err1 != nil || err2 != nil || startUnix <= 0 || endUnix <= 0 {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'start' or 'end' parameter; must be unix timestamps in seconds")
			return
		}
		groupSeconds, ok := supportedGroups[groupStr]
		if !ok {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'group' parameter. Supported values: %v", keysOfMap(supportedGroups)))
			return
		}

		stats, dbErr = s.db.RadarObjectRollupRange(startUnix, endUnix, groupSeconds)
	} else {
		stats, dbErr = s.db.RadarObjectRollup(days)
	}
	if dbErr != nil {
		s.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to retrieve radar stats: %v", dbErr))
		return
	}

	// Apply unit conversion to all speed values using the determined units
	for i := range stats {
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
