package main

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

type Server struct {
	port RadarPortInterface
	db   *DB
}

func NewServer(port RadarPortInterface, db *DB) *Server {
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
	mux.HandleFunc("/", s.homeHandler)
	mux.HandleFunc("/events", s.listEvents)

	mux.HandleFunc("/command", s.sendCommandHandler)
	return mux
}

func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	command := r.FormValue("command")

	if slices.Contains(allowedCommands, strings.TrimSpace(command)) {
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
