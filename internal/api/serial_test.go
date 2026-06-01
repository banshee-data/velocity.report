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

	"github.com/banshee-data/velocity.report/internal/db"
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
				fakeDirEntry{name: "tty0"}, // plain tty0 — no pattern matches
				fakeDirEntry{name: "ttyUSB0"},
			}, nil
		case "/dev/serial/by-id":
			return []os.DirEntry{fakeDirEntry{name: "usb-ops243-a"}}, nil
		case "/dev/serial/by-path", "/dev/serial":
			return nil, os.ErrNotExist
		default:
			return nil, os.ErrNotExist
		}
	}

	got := listSupplementalSerialPorts(readDir)
	// ttyUSB0 now matches the broadened pattern set; tty0 still does not.
	want := []string{"/dev/serial/by-id/usb-ops243-a", "/dev/ttySC1", "/dev/ttyUSB0"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// Once the supplemental scan grew beyond ttySC*, we want explicit assertions
// that every "primary fallback" pattern still matches what it claims to —
// and that obviously-not-a-serial-port names stay out.
func TestListSupplementalSerialPorts_BroadPatterns(t *testing.T) {
	readDir := func(path string) ([]os.DirEntry, error) {
		switch path {
		case "/dev":
			return []os.DirEntry{
				fakeDirEntry{name: "ttySC0"},
				fakeDirEntry{name: "ttyS0"},
				fakeDirEntry{name: "ttyAMA10"},
				fakeDirEntry{name: "ttyACM0"},
				fakeDirEntry{name: "ttyUSB0"},
				fakeDirEntry{name: "ttySCfoo"}, // letters after prefix — must NOT match
				fakeDirEntry{name: "ttyXYZ0"},  // unknown family — must NOT match
				fakeDirEntry{name: "random.txt"},
				fakeDirEntry{name: "console"},
			}, nil
		default:
			return nil, os.ErrNotExist
		}
	}

	got := listSupplementalSerialPorts(readDir)
	want := []string{
		"/dev/ttyACM0",
		"/dev/ttyAMA10",
		"/dev/ttyS0",
		"/dev/ttySC0",
		"/dev/ttyUSB0",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// /dev/serial/ (the root, not the by-id/by-path subdirs) is where the Pi's
// primary UART symlinks live as serial0/serial1. Before this scan was
// added, those symlinks were invisible to the supplemental fallback.
func TestListSupplementalSerialPorts_DevSerialRoot(t *testing.T) {
	readDir := func(path string) ([]os.DirEntry, error) {
		switch path {
		case "/dev":
			return nil, nil
		case "/dev/serial":
			return []os.DirEntry{
				fakeDirEntry{name: "serial0"},
				fakeDirEntry{name: "serial1"},
				fakeDirEntry{name: "by-id", isDir: true},
				fakeDirEntry{name: "by-path", isDir: true},
			}, nil
		default:
			return nil, os.ErrNotExist
		}
	}

	got := listSupplementalSerialPorts(readDir)
	want := []string{"/dev/serial/serial0", "/dev/serial/serial1"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// scanSupplementalSerialPorts powers the diagnostic envelope. It must
// surface the raw /dev/ entries (so callers can spot ports missing from
// the regex set) and report scan errors that are not "directory does not
// exist" (which is the normal case on macOS dev hosts).
func TestScanSupplementalSerialPorts_DiagnosticFields(t *testing.T) {
	readDir := func(path string) ([]os.DirEntry, error) {
		switch path {
		case "/dev":
			return []os.DirEntry{
				fakeDirEntry{name: "ttyS0"},
				fakeDirEntry{name: "ttyXYZ0"}, // raw-only
				fakeDirEntry{name: "serial0"}, // raw-only (matched by symlink dir)
				fakeDirEntry{name: "random.txt"},
			}, nil
		case "/dev/serial":
			return nil, os.ErrPermission // real error — must be reported
		case "/dev/serial/by-id":
			return nil, os.ErrNotExist // missing — must NOT be reported
		case "/dev/serial/by-path":
			return nil, os.ErrNotExist
		default:
			return nil, os.ErrNotExist
		}
	}

	got := scanSupplementalSerialPorts(readDir)

	wantPorts := []string{"/dev/ttyS0"}
	if !reflect.DeepEqual(got.Ports, wantPorts) {
		t.Errorf("Ports: expected %v, got %v", wantPorts, got.Ports)
	}

	wantDev := []string{"serial0", "ttyS0", "ttyXYZ0"}
	if !reflect.DeepEqual(got.DevTTY, wantDev) {
		t.Errorf("DevTTY: expected %v, got %v", wantDev, got.DevTTY)
	}

	if len(got.ScanError) != 1 || !strings.Contains(got.ScanError[0], "/dev/serial") {
		t.Errorf("ScanError: expected one error mentioning /dev/serial, got %v", got.ScanError)
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

// Diagnostic mode is opt-in via ?diagnostic=true. Without it, /api/serial/devices
// must return the legacy []SerialDeviceInfo array so the existing frontend
// keeps working. With it, callers (the serial-harness CLI in particular) get
// the structured envelope with enumeration sources, configured ports, raw
// /dev/ contents, and any scan errors.
func TestHandleSerialDevices_DiagnosticEnvelopeOptIn(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_devices_*.db")
	if err != nil {
		t.Fatalf("create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	defer database.Close()

	server := NewServer(serialmux.NewDisabledSerialMux(), database, "mph", "UTC")

	// Default: plain array shape (UI contract).
	{
		req := httptest.NewRequest(http.MethodGet, "/api/serial/devices", nil)
		w := httptest.NewRecorder()
		server.handleSerialDevices(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("default: expected 200, got %d (body=%s)", w.Code, w.Body.String())
		}
		var devices []SerialDeviceInfo
		if err := json.Unmarshal(w.Body.Bytes(), &devices); err != nil {
			t.Fatalf("default: response is not a []SerialDeviceInfo array: %v (body=%s)", err, w.Body.String())
		}
		// We don't assert on the contents — what's in /dev varies per host —
		// only that the shape is the legacy array. An accidental envelope
		// regression would surface here as a JSON unmarshal failure.
	}

	// ?diagnostic=true: envelope shape.
	{
		req := httptest.NewRequest(http.MethodGet, "/api/serial/devices?diagnostic=true", nil)
		w := httptest.NewRecorder()
		server.handleSerialDevices(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("diagnostic: expected 200, got %d (body=%s)", w.Code, w.Body.String())
		}
		var resp SerialDevicesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("diagnostic: decode envelope: %v (body=%s)", err, w.Body.String())
		}
		if resp.Diagnostic == nil {
			t.Fatalf("diagnostic: envelope.diagnostic must be present, got nil")
		}
		if resp.Diagnostic.EnumerationSource != "go.bug.st/serial" {
			t.Errorf("diagnostic: enumeration_source = %q, want %q", resp.Diagnostic.EnumerationSource, "go.bug.st/serial")
		}
		// Devices field must always be a non-nil slice (even if empty),
		// so the harness can index into it without a nil check.
		if resp.Devices == nil {
			t.Errorf("diagnostic: devices field is nil (should be [] when no devices)")
		}
		// ConfiguredPorts must be present (even empty) — that's how the
		// harness shows which ports are already "owned" and therefore
		// filtered out of the merged list.
		if resp.Diagnostic.ConfiguredPorts == nil {
			t.Errorf("diagnostic: configured_ports field is nil (should be [] when no configs)")
		}
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
