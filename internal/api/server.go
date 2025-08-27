package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

// ANSI escape codes for cyan and reset
const colorCyan = "\033[36m"
const colorReset = "\033[0m"
const colorYellow = "\033[33m"
const colorBoldGreen = "\033[1;32m"
const colorBoldRed = "\033[1;31m"

type Server struct {
	m  serialmux.SerialMuxInterface
	db *db.DB
}

func NewServer(m serialmux.SerialMuxInterface, db *db.DB) *Server {
	return &Server{
		m:  m,
		db: db,
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

func (s *Server) ServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.listEvents)
	mux.HandleFunc("/command", s.sendCommandHandler)
	mux.HandleFunc("/api/radar_stats", s.showRadarObjectStats)
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

	stats, err := s.db.RadarObjectRollup(days)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to retrieve radar stats: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write radar stats")
		return
	}
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

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
		apiEvents[i] = db.EventToAPI(e)
	}

	if err := json.NewEncoder(w).Encode(apiEvents); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write events")
		return
	}
}
