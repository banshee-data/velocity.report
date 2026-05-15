package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/serialmux"
)

func TestHandleSerialTest_UsesActiveConnectionForMatchingConfig(t *testing.T) {
	server := NewServer(nil, nil, "mph", "UTC")
	manager := NewSerialPortManager(nil, serialmux.NewDisabledSerialMux(), SerialConfigSnapshot{
		PortPath: "/dev/ttySC1",
		Source:   "database",
		Options: serialmux.PortOptions{
			BaudRate: 19200,
			DataBits: 8,
			StopBits: 1,
			Parity:   "N",
		},
	}, nil)
	defer manager.Close()
	server.SetSerialManager(manager)

	body, err := json.Marshal(SerialTestRequest{PortPath: "/dev/ttySC1", BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/serial/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSerialTest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SerialTestResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "already active") {
		t.Fatalf("expected active-connection message, got %q", resp.Message)
	}
	if !strings.Contains(resp.Suggestion, "no separate test open") {
		t.Fatalf("expected suggestion about avoiding a second open, got %q", resp.Suggestion)
	}
}

func TestHandleSerialTest_RejectsDifferentSettingsOnActivePort(t *testing.T) {
	server := NewServer(nil, nil, "mph", "UTC")
	manager := NewSerialPortManager(nil, serialmux.NewDisabledSerialMux(), SerialConfigSnapshot{
		PortPath: "/dev/ttySC1",
		Source:   "database",
		Options: serialmux.PortOptions{
			BaudRate: 19200,
			DataBits: 8,
			StopBits: 1,
			Parity:   "N",
		},
	}, nil)
	defer manager.Close()
	server.SetSerialManager(manager)

	body, err := json.Marshal(SerialTestRequest{PortPath: "/dev/ttySC1", BaudRate: 115200, DataBits: 8, StopBits: 1, Parity: "N"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/serial/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSerialTest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SerialTestResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Success {
		t.Fatalf("expected failure response, got %+v", resp)
	}
	if !strings.Contains(resp.Error, "currently in use") {
		t.Fatalf("expected in-use error, got %q", resp.Error)
	}
	if !strings.Contains(resp.Suggestion, "Save/apply the new settings") {
		t.Fatalf("expected suggestion about active connection settings, got %q", resp.Suggestion)
	}
}
