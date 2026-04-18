package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

// TailscaleController is the surface the api server depends on for
// Tailscale lifecycle operations.  Defined as an interface so tests and
// non-Pi builds can substitute a stub.
type TailscaleController interface {
	Status(ctx context.Context) tailscale.Status
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
}

// SetTailscaleController wires a Tailscale manager into the api server.
// When nil, the /api/tailscale/* endpoints return 503.
func (s *Server) SetTailscaleController(tc TailscaleController) {
	s.tailscale = tc
}

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "tailscale integration not available on this build")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Cap status calls so a wedged daemon does not block the HTTP
	// handler indefinitely.
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	st := s.tailscale.Status(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(st); err != nil {
		log.Printf("tailscale status: encode error: %v", err)
	}
}

func (s *Server) handleTailscaleEnable(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "tailscale integration not available on this build")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Enable touches systemd and waits for the daemon to come up; give
	// it a generous budget but do not let a stuck daemon hold the
	// connection forever.
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := s.tailscale.Enable(ctx); err != nil {
		log.Printf("tailscale enable: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st := s.tailscale.Status(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(st); err != nil {
		log.Printf("tailscale enable: encode error: %v", err)
	}
}

func (s *Server) handleTailscaleDisable(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "tailscale integration not available on this build")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.tailscale.Disable(ctx); err != nil {
		log.Printf("tailscale disable: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st := s.tailscale.Status(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(st); err != nil {
		log.Printf("tailscale disable: encode error: %v", err)
	}
}
