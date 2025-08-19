package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

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

func (s *Server) homeHandler(w http.ResponseWriter, r *http.Request) {
	// Handle the home page
	w.Write([]byte("Welcome to the Radar Server!"))
}

func (s *Server) ServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.listEvents)
	mux.HandleFunc("/command", s.sendCommandHandler)
	mux.HandleFunc("/radar_stats", s.showRadarObjectStats)
	mux.HandleFunc("/", s.homeHandler)
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

func (s *Server) showRadarObjectStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	days := 1 // default value
	if d := r.URL.Query().Get("days"); d != "" {
		fmt.Sscanf(d, "%d", &days)
	}

	stats, err := s.db.RadarObjectRollup(days)
	if err != nil {
		s := fmt.Sprintf("Failed to retrieve radar stats: %v", err)
		http.Error(w, s, http.StatusInternalServerError)
		return
	}

	for _, stat := range stats {
		_, err := w.Write([]byte(stat.String() + "\n"))
		if err != nil {
			http.Error(w, "Failed to write radar stat", http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	events, err := s.db.Events()
	if err != nil {
		s := fmt.Sprintf("Failed to retrieve events: %v", err)
		http.Error(w, s, http.StatusInternalServerError)
		return
	}

	for _, event := range events {
		_, err := w.Write([]byte(event.String() + "\n"))
		if err != nil {
			http.Error(w, "Failed to write event", http.StatusInternalServerError)
			return
		}
	}
}
