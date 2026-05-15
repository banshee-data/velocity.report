package api

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/serialmux"
)

type fakeDirEntry struct {
	name  string
	isDir bool
}

func (e fakeDirEntry) Name() string { return e.name }
func (e fakeDirEntry) IsDir() bool  { return e.isDir }
func (e fakeDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestSerialTestCommands_DefaultProbeUsesQueryOnly(t *testing.T) {
	got := serialTestCommands(false)
	want := []string{"??"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSerialTestCommands_AutoCorrectAddsBaudQuery(t *testing.T) {
	got := serialTestCommands(true)
	want := []string{"??", "I?"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestListSupplementalSerialPorts_FindsTTYSCAndSerialSymlinks(t *testing.T) {
	readDir := func(path string) ([]os.DirEntry, error) {
		switch path {
		case "/dev":
			return []os.DirEntry{
				fakeDirEntry{name: "ttySC1"},
				fakeDirEntry{name: "tty0"},
				fakeDirEntry{name: "ttyUSB0"},
			}, nil
		case "/dev/serial/by-id":
			return []os.DirEntry{fakeDirEntry{name: "usb-ops243-a"}}, nil
		case "/dev/serial/by-path":
			return nil, os.ErrNotExist
		default:
			return nil, os.ErrNotExist
		}
	}

	got := listSupplementalSerialPorts(readDir)
	want := []string{"/dev/serial/by-id/usb-ops243-a", "/dev/ttySC1"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// Regression for https://… field report: a Pi 5 + SC16IS762 HAT exposes
// both /dev/ttySC0 and /dev/ttySC1; the original test only covered ttySC1
// so a deployed binary that silently dropped ttySC0 went unnoticed.
func TestListSupplementalSerialPorts_FindsAllTTYSCNumbers(t *testing.T) {
	readDir := func(path string) ([]os.DirEntry, error) {
		switch path {
		case "/dev":
			return []os.DirEntry{
				fakeDirEntry{name: "ttySC0"},
				fakeDirEntry{name: "ttySC1"},
				fakeDirEntry{name: "ttySC10"},
				fakeDirEntry{name: "ttySC100"},
				fakeDirEntry{name: "ttySC1234"}, // 4 digits — out of range, must NOT match
				fakeDirEntry{name: "ttySC"},     // no digits — must NOT match
			}, nil
		default:
			return nil, os.ErrNotExist
		}
	}

	got := listSupplementalSerialPorts(readDir)
	want := []string{"/dev/ttySC0", "/dev/ttySC1", "/dev/ttySC10", "/dev/ttySC100"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildSerialDeviceList_IncludesSupplementalPorts(t *testing.T) {
	devices := buildSerialDeviceList(map[string]bool{}, []string{"/dev/ttyUSB0"}, []string{"/dev/ttySC1"}, 123)

	got := make([]string, 0, len(devices))
	for _, device := range devices {
		got = append(got, device.PortPath)
	}

	want := []string{"/dev/ttySC1", "/dev/ttyUSB0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if devices[0].FriendlyName != "SC16IS762 HAT (ttySC1)" {
		t.Fatalf("unexpected friendly name for default hat: %q", devices[0].FriendlyName)
	}
}

func TestBuildSerialDeviceList_SkipsConfiguredSupplementalPort(t *testing.T) {
	devices := buildSerialDeviceList(map[string]bool{"/dev/ttySC1": true}, []string{"/dev/ttyUSB0"}, []string{"/dev/ttySC1"}, 123)

	got := make([]string, 0, len(devices))
	for _, device := range devices {
		got = append(got, device.PortPath)
	}

	want := []string{"/dev/ttyUSB0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

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
