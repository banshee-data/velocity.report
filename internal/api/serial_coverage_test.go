package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/serialmux"
)

func TestApplySerialTestDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   SerialTestRequest
		want SerialTestRequest
	}{
		{
			name: "empty request gets the OPS243 defaults",
			in:   SerialTestRequest{PortPath: "/dev/ttyUSB0"},
			want: SerialTestRequest{
				PortPath: "/dev/ttyUSB0", BaudRate: 19200, DataBits: 8,
				StopBits: 1, Parity: "N", TimeoutSeconds: 5,
			},
		},
		{
			name: "explicit values are preserved",
			in: SerialTestRequest{
				PortPath: "/dev/ttyUSB0", BaudRate: 115200, DataBits: 7,
				StopBits: 2, Parity: "E", TimeoutSeconds: 30,
			},
			want: SerialTestRequest{
				PortPath: "/dev/ttyUSB0", BaudRate: 115200, DataBits: 7,
				StopBits: 2, Parity: "E", TimeoutSeconds: 30,
			},
		},
		{
			name: "partial request fills only the zero fields",
			in:   SerialTestRequest{PortPath: "/dev/ttyUSB0", BaudRate: 9600, Parity: "O"},
			want: SerialTestRequest{
				PortPath: "/dev/ttyUSB0", BaudRate: 9600, DataBits: 8,
				StopBits: 1, Parity: "O", TimeoutSeconds: 5,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in
			applySerialTestDefaults(&got)
			if got != tc.want {
				t.Errorf("applySerialTestDefaults() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGetSuggestionForError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "missing device points at /dev",
			err:  errors.New("open /dev/ttyUSB9: no such file or directory"),
			want: "Check that the device is connected and appears in /dev/",
		},
		{
			name: "not found also points at /dev",
			err:  errors.New("device not found"),
			want: "Check that the device is connected and appears in /dev/",
		},
		{
			name: "permission denied suggests the dialout group",
			err:  errors.New("open /dev/ttyUSB0: permission denied"),
			want: "Run: sudo usermod -a -G dialout $USER && sudo reboot",
		},
		{
			name: "resource busy suggests stopping other users",
			err:  errors.New("open /dev/ttyUSB0: resource busy"),
			want: "Another process may be using the port. Stop other applications using this serial port.",
		},
		{
			name: "device busy suggests stopping other users",
			err:  errors.New("device busy"),
			want: "Another process may be using the port. Stop other applications using this serial port.",
		},
		{
			name: "unrecognised error falls back to the generic hint",
			err:  errors.New("something else entirely"),
			want: "Check device connection and permissions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getSuggestionForError(tc.err); got != tc.want {
				t.Errorf("getSuggestionForError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestTestSerialPortRejectsInvalidConfiguration(t *testing.T) {
	// A non-standard baud rate fails PortOptions.Normalise before any device
	// is touched, so this runs without hardware. Note a non-positive rate
	// would NOT do: Normalise silently rewrites those to the 19200 default.
	got := testSerialPort(SerialTestRequest{
		PortPath: "/dev/ttyUSB0",
		BaudRate: 12345,
		DataBits: 8,
		StopBits: 1,
		Parity:   "N",
	})

	if got.Success {
		t.Fatal("testSerialPort succeeded with an invalid baud rate, want failure")
	}
	if !strings.Contains(got.Error, "Invalid serial configuration") {
		t.Errorf("Error = %q, want it to name the invalid configuration", got.Error)
	}
	if got.Suggestion != "Check baud rate, data bits, stop bits, and parity settings" {
		t.Errorf("Suggestion = %q, want the settings hint", got.Suggestion)
	}
	if got.PortPath != "/dev/ttyUSB0" {
		t.Errorf("PortPath = %q, want it echoed back", got.PortPath)
	}
}

func TestTestSerialPortRejectsInvalidParity(t *testing.T) {
	got := testSerialPort(SerialTestRequest{
		PortPath: "/dev/ttyUSB0",
		BaudRate: 19200,
		DataBits: 8,
		StopBits: 1,
		Parity:   "Z", // not a recognised parity mode
	})

	if got.Success {
		t.Fatal("testSerialPort succeeded with an invalid parity, want failure")
	}
	if got.Message != "Serial port test failed" {
		t.Errorf("Message = %q, want %q", got.Message, "Serial port test failed")
	}
}

func TestTestSerialPortReportsOpenFailure(t *testing.T) {
	// A well-formed configuration pointing at a device that does not exist
	// exercises the serial.Open failure path and its suggestion lookup.
	got := testSerialPort(SerialTestRequest{
		PortPath:       "/dev/ttyVELOCITYDOESNOTEXIST",
		BaudRate:       19200,
		DataBits:       8,
		StopBits:       1,
		Parity:         "N",
		TimeoutSeconds: 1,
	})

	if got.Success {
		t.Fatal("testSerialPort succeeded on a nonexistent device, want failure")
	}
	if !strings.Contains(got.Error, "Failed to open port") {
		t.Errorf("Error = %q, want it to report the failed open", got.Error)
	}
	if got.Suggestion == "" {
		t.Error("Suggestion is empty, want a diagnostic hint")
	}
	if got.BaudRate != 19200 {
		t.Errorf("BaudRate = %d, want the normalised 19200", got.BaudRate)
	}
}

func TestActiveSerialTestResultNotHandledWithoutManager(t *testing.T) {
	s := NewServer(nil, nil, "mph", "UTC")

	// With no serial manager wired the caller must fall through to a real
	// port test rather than getting a synthesised answer.
	if _, handled := s.activeSerialTestResult(SerialTestRequest{PortPath: "/dev/ttyUSB0"}); handled {
		t.Error("activeSerialTestResult handled the request without a serial manager")
	}
}

// newActiveSerialServer wires a server whose serial manager reports the given
// active snapshot, using a disabled mux so no hardware is touched.
func newActiveSerialServer(t *testing.T, snap SerialConfigSnapshot, mux serialmux.SerialMuxInterface) *Server {
	t.Helper()
	s := NewServer(mux, nil, "mph", "UTC")
	s.SetSerialManager(NewSerialPortManager(nil, mux, snap, nil))
	return s
}

func TestActiveSerialTestResultIgnoresDifferentPort(t *testing.T) {
	s := newActiveSerialServer(t, SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}, serialmux.NewDisabledSerialMux())

	// A test against some other port is none of the active connection's
	// business, so it must not be short-circuited.
	if _, handled := s.activeSerialTestResult(SerialTestRequest{
		PortPath: "/dev/ttyUSB1", BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	}); handled {
		t.Error("activeSerialTestResult handled a request for a different port")
	}
}

func TestActiveSerialTestResultRejectsInvalidRequestOptions(t *testing.T) {
	s := newActiveSerialServer(t, SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}, serialmux.NewDisabledSerialMux())

	got, handled := s.activeSerialTestResult(SerialTestRequest{
		PortPath: "/dev/ttyUSB0", BaudRate: 12345, DataBits: 8, StopBits: 1, Parity: "N",
	})
	if !handled {
		t.Fatal("activeSerialTestResult did not handle a request for the active port")
	}
	if got.Success {
		t.Error("Success = true, want false for an invalid configuration")
	}
	if !strings.Contains(got.Error, "Invalid serial configuration") {
		t.Errorf("Error = %q, want it to name the invalid configuration", got.Error)
	}
}

func TestActiveSerialTestResultSucceedsWhenSettingsMatch(t *testing.T) {
	opts := serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"}
	s := newActiveSerialServer(t, SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0", Options: opts,
	}, serialmux.NewDisabledSerialMux())

	got, handled := s.activeSerialTestResult(SerialTestRequest{
		PortPath: "/dev/ttyUSB0", BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	if !handled {
		t.Fatal("activeSerialTestResult did not handle a request for the active port")
	}
	if !got.Success {
		t.Fatalf("Success = false, want true; error was %q", got.Error)
	}
	// No port is opened in this path, so the reported duration stays zero.
	if got.TestDurationMS != 0 {
		t.Errorf("TestDurationMS = %d, want 0 (no test open was attempted)", got.TestDurationMS)
	}
	if !strings.Contains(got.Message, "already active") {
		t.Errorf("Message = %q, want it to explain the port is already owned", got.Message)
	}
}

func TestActiveSerialTestResultReportsMissingLiveMux(t *testing.T) {
	opts := serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"}
	// A snapshot that claims an active port but no live mux is the
	// "configured but not connected" state.
	s := newActiveSerialServer(t, SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0", Options: opts,
	}, nil)

	got, handled := s.activeSerialTestResult(SerialTestRequest{
		PortPath: "/dev/ttyUSB0", BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	if !handled {
		t.Fatal("activeSerialTestResult did not handle a request for the active port")
	}
	if got.Success {
		t.Error("Success = true, want false when no live connection exists")
	}
	if !strings.Contains(got.Error, "no live serial connection") {
		t.Errorf("Error = %q, want it to report the missing live connection", got.Error)
	}
}

func TestHandleSerialTestRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		body     string
		wantCode int
		wantBody string
	}{
		{
			name:     "GET is not allowed",
			method:   http.MethodGet,
			body:     "",
			wantCode: http.StatusMethodNotAllowed,
			wantBody: "Method not allowed",
		},
		{
			name:     "malformed JSON",
			method:   http.MethodPost,
			body:     "{not json",
			wantCode: http.StatusBadRequest,
			wantBody: "Invalid request body",
		},
		{
			name:     "missing port path",
			method:   http.MethodPost,
			body:     `{"baud_rate":19200}`,
			wantCode: http.StatusBadRequest,
			wantBody: "Port path is required",
		},
		{
			name:     "port path outside /dev is rejected",
			method:   http.MethodPost,
			body:     `{"port_path":"/etc/passwd"}`,
			wantCode: http.StatusBadRequest,
			wantBody: "Invalid port path",
		},
		{
			name:     "path traversal is rejected",
			method:   http.MethodPost,
			body:     `{"port_path":"/dev/../etc/passwd"}`,
			wantCode: http.StatusBadRequest,
			wantBody: "Invalid port path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(nil, nil, "mph", "UTC")
			req := httptest.NewRequest(tc.method, "/api/serial/test", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()

			s.handleSerialTest(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantCode, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body, tc.wantBody)
			}
		})
	}
}

func TestHandleSerialTestReturns200ForDeviceFailure(t *testing.T) {
	// A failing device test is a result, not an API error, so the handler
	// still answers 200 with the failure encoded in the body.
	s := NewServer(nil, nil, "mph", "UTC")
	body := `{"port_path":"/dev/ttyVELOCITYDOESNOTEXIST","timeout_seconds":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/serial/test", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	s.handleSerialTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got SerialTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Success {
		t.Error("Success = true, want false for a nonexistent device")
	}
	// Defaults were applied before the test ran.
	if got.BaudRate != 19200 {
		t.Errorf("BaudRate = %d, want the default 19200", got.BaudRate)
	}
}
