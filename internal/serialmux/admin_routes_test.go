package serialmux

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// localHostRequest creates an httptest request that appears to come from localhost.
// This bypasses tsweb.AllowDebugAccess which checks for loopback IPs.
func localHostRequest(method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

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

			req := localHostRequest(tt.method, "/debug/send-command-api", body)
			if tt.formData != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}

			w := httptest.NewRecorder()
			httpMux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
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
	req := localHostRequest(http.MethodPost, "/debug/send-command-api", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	httpMux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestAttachAdminRoutes_Tail tests the tail endpoint registration and method handling.
// Note: SSE data streaming is tested in admin_routes_integration_test.go to avoid race conditions.
func TestAttachAdminRoutes_Tail(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	t.Run("POST method not allowed", func(t *testing.T) {
		req := localHostRequest(http.MethodPost, "/debug/tail", nil)
		w := httptest.NewRecorder()

		httpMux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", w.Code)
		}
	})
}

// TestAttachAdminRoutes_SendCommand tests the send-command HTML page
func TestAttachAdminRoutes_SendCommand(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	req := localHostRequest(http.MethodGet, "/debug/send-command", nil)
	w := httptest.NewRecorder()

	httpMux.ServeHTTP(w, req)

	// Should return 200 for valid GET request
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Check that the response looks like HTML
	if w.Code == http.StatusOK {
		body := w.Body.String()
		if !strings.Contains(body, "html") && !strings.Contains(body, "HTML") && !strings.Contains(body, "<") {
			t.Error("Response doesn't appear to be HTML")
		}
	}
}

// TestAttachAdminRoutes_TailJS tests the tail.js endpoint
func TestAttachAdminRoutes_TailJS(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	req := localHostRequest(http.MethodGet, "/debug/tail.js", nil)
	w := httptest.NewRecorder()

	httpMux.ServeHTTP(w, req)

	// Should return 200 for valid GET request
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Check that the response looks like JavaScript
	if w.Code == http.StatusOK {
		contentType := w.Header().Get("Content-Type")
		if !strings.Contains(contentType, "javascript") {
			t.Errorf("Expected Content-Type to contain 'javascript', got: %s", contentType)
		}
	}
}
