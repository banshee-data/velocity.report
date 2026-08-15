package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

// stubTailscale is a TailscaleController that records calls and returns
// canned results. It deliberately does NOT implement tailscaleStatusWaiter,
// so it also exercises the "controller cannot long-poll" branch.
type stubTailscale struct {
	mu           sync.Mutex
	status       tailscale.Status
	enableErr    error
	disableErr   error
	enableCalls  int
	disableCalls int
	statusCalls  int
}

func (s *stubTailscale) Status(context.Context) tailscale.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCalls++
	return s.status
}

func (s *stubTailscale) Enable(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enableCalls++
	return s.enableErr
}

func (s *stubTailscale) Disable(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableCalls++
	return s.disableErr
}

func (s *stubTailscale) counts() (enable, disable, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enableCalls, s.disableCalls, s.statusCalls
}

// waitingTailscale additionally implements tailscaleStatusWaiter so the
// long-poll branch of handleTailscaleStatus can be driven.
type waitingTailscale struct {
	stubTailscale
	mu        sync.Mutex
	waitCalls int
	gotSince  uint64
	gotWait   time.Duration
	// block, when non-nil, is waited on inside WaitForChange so a test can
	// assert the handler honours request cancellation.
	block chan struct{}
}

func (w *waitingTailscale) WaitForChange(ctx context.Context, since uint64, timeout time.Duration) uint64 {
	w.mu.Lock()
	w.waitCalls++
	w.gotSince = since
	w.gotWait = timeout
	block := w.block
	w.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
		}
	}
	return since + 1
}

func (w *waitingTailscale) observed() (calls int, since uint64, wait time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.waitCalls, w.gotSince, w.gotWait
}

// tailscaleTestServer returns an api Server with the given controller wired in.
// A nil controller leaves the integration unavailable.
func tailscaleTestServer(tc TailscaleController) *Server {
	s := NewServer(nil, nil, "mph", "UTC")
	if tc != nil {
		s.SetTailscaleController(tc)
	}
	return s
}

func TestTailscaleEndpointsUnavailableWithoutController(t *testing.T) {
	// With no controller wired (non-Pi builds), every endpoint must 503
	// rather than panic on a nil interface.
	tests := []struct {
		name    string
		method  string
		target  string
		handler func(*Server) http.HandlerFunc
	}{
		{"status", http.MethodGet, "/api/tailscale/status",
			func(s *Server) http.HandlerFunc { return s.handleTailscaleStatus }},
		{"enable", http.MethodPost, "/api/tailscale/enable",
			func(s *Server) http.HandlerFunc { return s.handleTailscaleEnable }},
		{"disable", http.MethodPost, "/api/tailscale/disable",
			func(s *Server) http.HandlerFunc { return s.handleTailscaleDisable }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tailscaleTestServer(nil)
			rec := httptest.NewRecorder()
			tc.handler(s)(rec, httptest.NewRequest(tc.method, tc.target, nil))

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding error body: %v", err)
			}
		})
	}
}

func TestTailscaleEndpointsRejectWrongMethod(t *testing.T) {
	// The availability check runs before the method check, so these need a
	// live controller to reach the 405.
	tests := []struct {
		name    string
		method  string
		target  string
		handler func(*Server) http.HandlerFunc
	}{
		{"status rejects POST", http.MethodPost, "/api/tailscale/status",
			func(s *Server) http.HandlerFunc { return s.handleTailscaleStatus }},
		{"enable rejects GET", http.MethodGet, "/api/tailscale/enable",
			func(s *Server) http.HandlerFunc { return s.handleTailscaleEnable }},
		{"disable rejects GET", http.MethodGet, "/api/tailscale/disable",
			func(s *Server) http.HandlerFunc { return s.handleTailscaleDisable }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubTailscale{}
			s := tailscaleTestServer(stub)
			rec := httptest.NewRecorder()
			tc.handler(s)(rec, httptest.NewRequest(tc.method, tc.target, nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			if enable, disable, status := stub.counts(); enable+disable+status != 0 {
				t.Errorf("controller called on rejected method: enable=%d disable=%d status=%d",
					enable, disable, status)
			}
		})
	}
}

func TestHandleTailscaleStatusReturnsControllerStatus(t *testing.T) {
	stub := &stubTailscale{status: tailscale.Status{
		DaemonRunning: true,
		BackendState:  "Running",
		Hostname:      "velocity",
		MagicDNS:      "velocity.tailfoo.ts.net",
		PeerCount:     3,
		SSHEnabled:    true,
	}}
	s := tailscaleTestServer(stub)

	rec := httptest.NewRecorder()
	s.handleTailscaleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/tailscale/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got tailscale.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding status: %v", err)
	}
	if !got.DaemonRunning || got.BackendState != "Running" || got.PeerCount != 3 {
		t.Errorf("status = %+v, want the controller's status", got)
	}
	if got.MagicDNS != "velocity.tailfoo.ts.net" {
		t.Errorf("MagicDNS = %q, want velocity.tailfoo.ts.net", got.MagicDNS)
	}
}

func TestHandleTailscaleStatusSkipsLongPollWithoutWaitParam(t *testing.T) {
	waiter := &waitingTailscale{}
	s := tailscaleTestServer(waiter)

	rec := httptest.NewRecorder()
	s.handleTailscaleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/tailscale/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calls, _, _ := waiter.observed(); calls != 0 {
		t.Errorf("WaitForChange calls = %d, want 0 without ?wait", calls)
	}
}

func TestHandleTailscaleStatusLongPollsWhenWaitRequested(t *testing.T) {
	waiter := &waitingTailscale{}
	s := tailscaleTestServer(waiter)

	rec := httptest.NewRecorder()
	s.handleTailscaleStatus(rec,
		httptest.NewRequest(http.MethodGet, "/api/tailscale/status?wait=5&v=42", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	calls, since, wait := waiter.observed()
	if calls != 1 {
		t.Fatalf("WaitForChange calls = %d, want 1", calls)
	}
	if since != 42 {
		t.Errorf("since = %d, want 42 from ?v", since)
	}
	if wait != 5*time.Second {
		t.Errorf("wait = %v, want 5s", wait)
	}
}

func TestHandleTailscaleStatusClampsWaitToMaximum(t *testing.T) {
	waiter := &waitingTailscale{}
	s := tailscaleTestServer(waiter)

	rec := httptest.NewRecorder()
	s.handleTailscaleStatus(rec,
		httptest.NewRequest(http.MethodGet, "/api/tailscale/status?wait=99999", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// A client must not be able to pin a connection open for longer than
	// the server's own cap.
	if _, _, wait := waiter.observed(); wait != maxTailscaleStatusWait {
		t.Errorf("wait = %v, want it clamped to %v", wait, maxTailscaleStatusWait)
	}
}

func TestHandleTailscaleStatusLongPollHonoursCancellation(t *testing.T) {
	// block is never closed, so WaitForChange only returns when the request
	// context is cancelled.
	waiter := &waitingTailscale{block: make(chan struct{})}
	s := tailscaleTestServer(waiter)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/tailscale/status?wait=30", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleTailscaleStatus(httptest.NewRecorder(), req)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after request cancellation")
	}
}

func TestHandleTailscaleStatusIgnoresLongPollForNonWaitingController(t *testing.T) {
	// stubTailscale does not implement tailscaleStatusWaiter, so ?wait must
	// degrade to an immediate read rather than failing.
	stub := &stubTailscale{status: tailscale.Status{BackendState: "NeedsLogin"}}
	s := tailscaleTestServer(stub)

	rec := httptest.NewRecorder()
	s.handleTailscaleStatus(rec,
		httptest.NewRequest(http.MethodGet, "/api/tailscale/status?wait=10&v=1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got tailscale.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding status: %v", err)
	}
	if got.BackendState != "NeedsLogin" {
		t.Errorf("BackendState = %q, want NeedsLogin", got.BackendState)
	}
}

func TestHandleTailscaleEnableSucceeds(t *testing.T) {
	stub := &stubTailscale{status: tailscale.Status{DaemonRunning: true, BackendState: "Starting"}}
	s := tailscaleTestServer(stub)

	rec := httptest.NewRecorder()
	s.handleTailscaleEnable(rec, httptest.NewRequest(http.MethodPost, "/api/tailscale/enable", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", rec.Code, rec.Body)
	}
	enable, _, status := stub.counts()
	if enable != 1 {
		t.Errorf("Enable calls = %d, want 1", enable)
	}
	// The handler reports fresh status after enabling, not the pre-call state.
	if status != 1 {
		t.Errorf("Status calls = %d, want 1 after enable", status)
	}
	var got tailscale.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding status: %v", err)
	}
	if got.BackendState != "Starting" {
		t.Errorf("BackendState = %q, want Starting", got.BackendState)
	}
}

func TestHandleTailscaleEnableReportsControllerError(t *testing.T) {
	stub := &stubTailscale{enableErr: errors.New("systemctl start failed")}
	s := tailscaleTestServer(stub)

	rec := httptest.NewRecorder()
	s.handleTailscaleEnable(rec, httptest.NewRequest(http.MethodPost, "/api/tailscale/enable", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	// Status must not be reported when the enable itself failed.
	if _, _, status := stub.counts(); status != 0 {
		t.Errorf("Status calls = %d, want 0 after a failed enable", status)
	}
}

func TestHandleTailscaleDisableSucceeds(t *testing.T) {
	stub := &stubTailscale{status: tailscale.Status{DaemonRunning: false}}
	s := tailscaleTestServer(stub)

	rec := httptest.NewRecorder()
	s.handleTailscaleDisable(rec, httptest.NewRequest(http.MethodPost, "/api/tailscale/disable", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", rec.Code, rec.Body)
	}
	_, disable, status := stub.counts()
	if disable != 1 {
		t.Errorf("Disable calls = %d, want 1", disable)
	}
	if status != 1 {
		t.Errorf("Status calls = %d, want 1 after disable", status)
	}
	var got tailscale.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding status: %v", err)
	}
	if got.DaemonRunning {
		t.Error("DaemonRunning = true, want false after disable")
	}
}

func TestHandleTailscaleDisableReportsControllerError(t *testing.T) {
	stub := &stubTailscale{disableErr: errors.New("daemon busy")}
	s := tailscaleTestServer(stub)

	rec := httptest.NewRecorder()
	s.handleTailscaleDisable(rec, httptest.NewRequest(http.MethodPost, "/api/tailscale/disable", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if _, _, status := stub.counts(); status != 0 {
		t.Errorf("Status calls = %d, want 0 after a failed disable", status)
	}
}

func TestParseWaitSeconds(t *testing.T) {
	const max = 30 * time.Second
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty disables long-poll", "", 0},
		{"non-numeric disables long-poll", "soon", 0},
		{"zero disables long-poll", "0", 0},
		{"negative disables long-poll", "-5", 0},
		{"positive seconds", "7", 7 * time.Second},
		{"exactly max", "30", max},
		{"above max clamps", "31", max},
		{"far above max clamps", "100000", max},
		// Overflow-ish input still parses as an int but clamps.
		{"float is rejected", "1.5", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseWaitSeconds(tc.in, max); got != tc.want {
				t.Errorf("parseWaitSeconds(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseUint64(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want uint64
	}{
		{"empty is zero", "", 0},
		{"plain number", "42", 42},
		{"zero", "0", 0},
		{"negative is zero", "-1", 0},
		{"non-numeric is zero", "abc", 0},
		// parseUint64 discards ParseUint's error, and ParseUint returns
		// MaxUint64 alongside ErrRange on overflow. The effect is that an
		// out-of-range ?v pins the long-poll's `since` at the maximum, so the
		// client waits the full window instead of getting an immediate read.
		{"overflow saturates rather than resetting to zero",
			"99999999999999999999999", math.MaxUint64},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseUint64(tc.in); got != tc.want {
				t.Errorf("parseUint64(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
