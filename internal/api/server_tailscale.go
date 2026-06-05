package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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

// tailscaleStatusWaiter is the optional long-poll surface a controller may
// expose: block until the status version moves past `since`, the request is
// cancelled, or the timeout elapses.  Asserted at runtime so test stubs and
// non-Pi builds need not implement it (they answer immediately).
type tailscaleStatusWaiter interface {
	WaitForChange(ctx context.Context, since uint64, timeout time.Duration) uint64
}

// maxTailscaleStatusWait caps how long the server holds a long-poll so a
// client, proxy, or load balancer never blocks for an unreasonable time.
const maxTailscaleStatusWait = 30 * time.Second

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	if s.tailscale == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "tailscale integration not available on this build")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Long-poll: with ?wait=<secs>, hold the request until the status
	// version moves past ?v=<n> (or the wait elapses), so the UI gets
	// near-instant updates with ~one request per wait window instead of a
	// 2-second timer.  Both params are optional — without them this is a
	// plain immediate read.
	if wait := parseWaitSeconds(r.URL.Query().Get("wait"), maxTailscaleStatusWait); wait > 0 {
		if waiter, ok := s.tailscale.(tailscaleStatusWaiter); ok {
			since := parseUint64(r.URL.Query().Get("v"))
			waiter.WaitForChange(r.Context(), since, wait)
		}
	}

	// Cap status calls so a wedged daemon does not block the HTTP handler
	// indefinitely.
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	st := s.tailscale.Status(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(st); err != nil {
		log.Printf("tailscale status: encode error: %v", err)
	}
}

// parseWaitSeconds parses a positive integer seconds value, clamped to max.
// Returns 0 on empty/invalid/non-positive input (i.e. no long-poll).
func parseWaitSeconds(s string, max time.Duration) time.Duration {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	d := time.Duration(n) * time.Second
	if d > max {
		d = max
	}
	return d
}

func parseUint64(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
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
