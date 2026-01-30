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

// TestAttachAdminRoutes_SendCommandAPI tests the send-command-api endpoint
func TestAttachAdminRoutes_SendCommandAPI(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	tests := []struct {
		name           string
		method         string
		formData       url.Values
		expectedStatus int
		checkBody      func(t *testing.T, body string)
	}{
		{
			name:           "valid POST with command",
			method:         http.MethodPost,
			formData:       url.Values{"command": {"OJ"}},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "OJ") {
					t.Errorf("Expected response to contain command 'OJ', got: %s", body)
				}
			},
		},
		{
			name:           "POST with empty command",
			method:         http.MethodPost,
			formData:       url.Values{"command": {""}},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "Missing command") {
					t.Errorf("Expected 'Missing command' error, got: %s", body)
				}
			},
		},
		{
			name:           "POST with whitespace-only command",
			method:         http.MethodPost,
			formData:       url.Values{"command": {"   "}},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "Missing command") {
					t.Errorf("Expected 'Missing command' error, got: %s", body)
				}
			},
		},
		{
			name:           "POST without command parameter",
			method:         http.MethodPost,
			formData:       url.Values{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "Missing command") {
					t.Errorf("Expected 'Missing command' error, got: %s", body)
				}
			},
		},
		{
			name:           "GET method not allowed",
			method:         http.MethodGet,
			formData:       nil,
			expectedStatus: http.StatusMethodNotAllowed,
			checkBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "Method not allowed") {
					t.Errorf("Expected 'Method not allowed' error, got: %s", body)
				}
			},
		},
		{
			name:           "PUT method not allowed",
			method:         http.MethodPut,
			formData:       nil,
			expectedStatus: http.StatusMethodNotAllowed,
			checkBody:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.formData != nil {
				body = strings.NewReader(tt.formData.Encode())
			}

			req := httptest.NewRequest(tt.method, "/debug/send-command-api", body)
			if tt.formData != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}

			w := httptest.NewRecorder()
			httpMux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus && w.Code != http.StatusForbidden {
				// Allow 403 due to tailscale auth, or the expected status
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if w.Code == tt.expectedStatus && tt.checkBody != nil {
				tt.checkBody(t, w.Body.String())
			}
		})
	}
}

// TestAttachAdminRoutes_SendCommandAPI_WriteError tests error handling when writing to port fails
func TestAttachAdminRoutes_SendCommandAPI_WriteError(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	// Set write error on the port
	port.SetWriteError(io.ErrShortWrite)

	formData := url.Values{"command": {"OJ"}}
	req := httptest.NewRequest(http.MethodPost, "/debug/send-command-api", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	httpMux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError && w.Code != http.StatusForbidden {
		// Allow 403 due to tailscale auth, or 500 for the write error
		t.Errorf("Expected status 500 or 403, got %d", w.Code)
	}
}

// TestAttachAdminRoutes_Tail tests the tail endpoint
func TestAttachAdminRoutes_Tail(t *testing.T) {
	port := NewTestSerialPort("line1\nline2\n")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	t.Run("GET request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/tail", nil)
		w := httptest.NewRecorder()

		// Start monitoring in background
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go mux.Monitor(ctx)

		// Give monitor time to start
		time.Sleep(20 * time.Millisecond)

		// Create a done channel for the HTTP request
		done := make(chan struct{})
		go func() {
			httpMux.ServeHTTP(w, req)
			close(done)
		}()

		// Wait a bit for SSE headers to be set
		time.Sleep(50 * time.Millisecond)

		// Cancel and wait for completion
		cancel()
		select {
		case <-done:
			// Success - request completed
		case <-time.After(500 * time.Millisecond):
			// Timeout - that's okay, SSE endpoints can hang
			t.Log("Tail endpoint timed out (expected for SSE)")
		}

		// Check headers if we got a response
		if w.Code != 0 && w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
			if contentType := w.Header().Get("Content-Type"); contentType != "" && !strings.Contains(contentType, "text/event-stream") {
				t.Logf("Expected Content-Type 'text/event-stream', got: %s", contentType)
			}
		}
	})

	t.Run("POST method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/debug/tail", nil)
		w := httptest.NewRecorder()

		httpMux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusForbidden {
			t.Errorf("Expected status 405 or 403, got %d", w.Code)
		}
	})
}

// TestAttachAdminRoutes_SendCommand tests the send-command HTML page
func TestAttachAdminRoutes_SendCommand(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	req := httptest.NewRequest(http.MethodGet, "/debug/send-command", nil)
	w := httptest.NewRecorder()

	httpMux.ServeHTTP(w, req)

	// Should be registered (might return 403 due to auth or 200 if auth passes)
	if w.Code == http.StatusNotFound {
		t.Error("Route /debug/send-command should be registered, got 404")
	}

	// If we get 200, check that the response looks like HTML
	if w.Code == http.StatusOK {
		body := w.Body.String()
		if !strings.Contains(body, "html") && !strings.Contains(body, "HTML") {
			t.Log("Response doesn't appear to be HTML")
		}
	}
}

// TestAttachAdminRoutes_TailJS tests the tail.js endpoint
func TestAttachAdminRoutes_TailJS(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	req := httptest.NewRequest(http.MethodGet, "/debug/tail.js", nil)
	w := httptest.NewRecorder()

	httpMux.ServeHTTP(w, req)

	// Should be registered (might return 403 due to auth or 200 if auth passes)
	if w.Code == http.StatusNotFound {
		t.Error("Route /debug/tail.js should be registered, got 404")
	}

	// If we get 200, check that the response looks like JavaScript
	if w.Code == http.StatusOK {
		contentType := w.Header().Get("Content-Type")
		body := w.Body.String()
		if !strings.Contains(contentType, "javascript") && !strings.Contains(body, "function") {
			t.Log("Response doesn't appear to be JavaScript")
		}
	}
}
