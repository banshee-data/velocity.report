package api

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/banshee-data/radar/db"
	"github.com/banshee-data/radar/radar"
)

type Server struct {
	port radar.RadarPortInterface
	db   *db.DB
}

func NewServer(port radar.RadarPortInterface, db *db.DB) *Server {
	return &Server{
		port: port,
		db:   db,
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
	mux.HandleFunc("/", s.homeHandler)
	return mux
}

func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	command := r.FormValue("command")

	if slices.Contains(radar.AllowedCommands, strings.TrimSpace(command)) {
		s.port.SendCommand(command)
		w.Write([]byte("Command sent successfully"))
	} else {
		http.Error(w, "Invalid command", http.StatusBadRequest)
	}

	io.WriteString(w, "Command sent successfully")
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
