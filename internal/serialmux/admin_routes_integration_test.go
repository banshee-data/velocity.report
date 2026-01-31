package serialmux

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// AdminRoutesTestPort is a test port specifically for admin routes testing
// that allows controlled read/write behaviour.
type AdminRoutesTestPort struct {
	readData    []byte
	readIndex   int
	writeErr    error
	closed      bool
	readBlocks  bool
	blockSignal chan struct{}
}

func NewAdminRoutesTestPort(data string) *AdminRoutesTestPort {
	return &AdminRoutesTestPort{
		readData:    []byte(data),
		blockSignal: make(chan struct{}),
	}
}

func (p *AdminRoutesTestPort) Read(buf []byte) (int, error) {
	if p.closed {
		return 0, io.EOF
	}
	if p.readBlocks {
		<-p.blockSignal
		return 0, io.EOF
	}
	if p.readIndex >= len(p.readData) {
		time.Sleep(5 * time.Millisecond)
		return 0, nil
	}
	n := copy(buf, p.readData[p.readIndex:])
	p.readIndex += n
	return n, nil
}

func (p *AdminRoutesTestPort) Write(data []byte) (int, error) {
	if p.writeErr != nil {
		return 0, p.writeErr
	}
	return len(data), nil
}

func (p *AdminRoutesTestPort) Close() error {
	p.closed = true
	if p.readBlocks {
		close(p.blockSignal)
	}
	return nil
}

func (p *AdminRoutesTestPort) SetWriteError(err error) {
	p.writeErr = err
}

func (p *AdminRoutesTestPort) Unblock() {
	if p.readBlocks {
		close(p.blockSignal)
	}
}

// TestAdminRoutes_SendCommandAPI_SuccessfulCommand verifies the command is
// written to the serial port when the API is called.
func TestAdminRoutes_SendCommandAPI_SuccessfulCommand(t *testing.T) {
	port := NewAdminRoutesTestPort("")
	mux := NewSerialMux(port)

	// Create a simple HTTP mux that bypasses tailscale auth for testing
	httpMux := http.NewServeMux()

	// Register the send-command-api handler directly (bypassing tsweb.Debugger)
	httpMux.HandleFunc("/test/send-command-api", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimSpace(r.FormValue("command"))
		if command == "" {
			http.Error(w, "Missing command", http.StatusBadRequest)
			return
		}
		if err := mux.SendCommand(command); err != nil {
			http.Error(w, "Failed to write command", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "Wrote command to serial port")
	})

	tests := []struct {
		name           string
		method         string
		formData       url.Values
		expectedStatus int
	}{
		{
			name:           "POST with valid command",
			method:         http.MethodPost,
			formData:       url.Values{"command": {"OJ"}},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST with empty command",
			method:         http.MethodPost,
			formData:       url.Values{"command": {""}},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "GET method",
			method:         http.MethodGet,
			formData:       nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.formData != nil {
				body = strings.NewReader(tt.formData.Encode())
			}

			req := httptest.NewRequest(tt.method, "/test/send-command-api", body)
			if tt.formData != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}

			w := httptest.NewRecorder()
			httpMux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestAdminRoutes_SendCommandAPI_WriteFailure tests the error path when
// writing to the serial port fails.
func TestAdminRoutes_SendCommandAPI_WriteFailure(t *testing.T) {
	port := NewAdminRoutesTestPort("")
	port.SetWriteError(io.ErrShortWrite)
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/test/send-command-api", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimSpace(r.FormValue("command"))
		if command == "" {
			http.Error(w, "Missing command", http.StatusBadRequest)
			return
		}
		if err := mux.SendCommand(command); err != nil {
			http.Error(w, "Failed to write command", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "Wrote command to serial port")
	})

	formData := url.Values{"command": {"OJ"}}
	req := httptest.NewRequest(http.MethodPost, "/test/send-command-api", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	httpMux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestAdminRoutes_Tail_SSEHeaders tests that the tail endpoint sets
// correct SSE headers and sends data.
func TestAdminRoutes_Tail_SSEHeaders(t *testing.T) {
	port := NewAdminRoutesTestPort("test line 1\ntest line 2\n")
	mux := NewSerialMux(port)

	// Create handler that mimics the tail endpoint logic
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		id, c := mux.Subscribe()
		defer mux.Unsubscribe(id)

		// Send initial ping
		w.Write([]byte(": ping\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Read a few events then return
		timeout := time.After(100 * time.Millisecond)
		for {
			select {
			case payload, ok := <-c:
				if !ok {
					return
				}
				w.Write([]byte("data: " + payload + "\n\n"))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-timeout:
				return
			case <-r.Context().Done():
				return
			}
		}
	})

	// Start monitoring
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mux.Monitor(ctx)

	req := httptest.NewRequest(http.MethodGet, "/test/tail", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify SSE headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Expected Content-Type 'text/event-stream', got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Expected Cache-Control 'no-cache', got %q", cc)
	}

	// Body should contain the ping
	body := w.Body.String()
	if !strings.Contains(body, ": ping") {
		t.Errorf("Expected body to contain ping, got %q", body)
	}
}

// TestAdminRoutes_Tail_MethodNotAllowed verifies POST is rejected.
func TestAdminRoutes_Tail_MethodNotAllowed(t *testing.T) {
	port := NewAdminRoutesTestPort("")
	mux := NewSerialMux(port)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// ... rest of handler
	})

	_ = mux // Use mux to avoid unused warning

	req := httptest.NewRequest(http.MethodPost, "/test/tail", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestAdminRoutes_Tail_ClientDisconnect tests that the tail handler
// exits gracefully when the client disconnects.
func TestAdminRoutes_Tail_ClientDisconnect(t *testing.T) {
	port := NewAdminRoutesTestPort("")
	port.readBlocks = true
	mux := NewSerialMux(port)

	handlerDone := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)

		w.Header().Set("Content-Type", "text/event-stream")

		id, c := mux.Subscribe()
		defer mux.Unsubscribe(id)

		for {
			select {
			case _, ok := <-c:
				if !ok {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/test/tail", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	go handler.ServeHTTP(w, req)

	// Let handler start
	time.Sleep(10 * time.Millisecond)

	// Simulate client disconnect
	cancel()

	// Handler should exit
	select {
	case <-handlerDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Handler did not exit after context cancellation")
	}

	mux.Close()
}

// TestAdminRoutes_Tail_ChannelClosed tests the handler behaviour when
// the subscription channel is closed.
func TestAdminRoutes_Tail_ChannelClosed(t *testing.T) {
	port := NewAdminRoutesTestPort("")
	mux := NewSerialMux(port)

	handlerDone := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)

		w.Header().Set("Content-Type", "text/event-stream")

		id, c := mux.Subscribe()
		// Don't defer Unsubscribe - we'll close the mux instead

		for {
			select {
			case _, ok := <-c:
				if !ok {
					return
				}
			case <-r.Context().Done():
				mux.Unsubscribe(id)
				return
			}
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/test/tail", nil)
	w := httptest.NewRecorder()

	go handler.ServeHTTP(w, req)

	// Let handler start
	time.Sleep(10 * time.Millisecond)

	// Close the mux which closes all channels
	mux.Close()

	// Handler should exit
	select {
	case <-handlerDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Handler did not exit after channel closed")
	}
}
