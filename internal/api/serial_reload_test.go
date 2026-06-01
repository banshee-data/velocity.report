package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/serialmux"
)

// TestSerialPortManager_Subscribe tests that Subscribe returns persistent channels
func TestSerialPortManager_Subscribe(t *testing.T) {
	mockMux := serialmux.NewMockSerialMux([]byte(""))
	snapshot := SerialConfigSnapshot{
		PortPath: "/dev/test",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
		Source:   "test",
	}

	manager := NewSerialPortManager(nil, mockMux, snapshot, nil)
	defer manager.Close()

	// Subscribe should return a valid channel
	id, ch := manager.Subscribe()
	if id == "" {
		t.Error("Expected non-empty subscriber ID")
	}
	if ch == nil {
		t.Fatal("Expected non-nil channel")
	}

	// Verify channel is open
	select {
	case <-ch:
		t.Error("Channel should not be closed immediately")
	case <-time.After(10 * time.Millisecond):
		// Expected: channel is open and empty
	}

	// Unsubscribe should close the channel
	manager.Unsubscribe(id)

	// Verify channel is closed after unsubscribe
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should be closed immediately after unsubscribe")
	}
}

// TestSerialPortManager_SendCommand tests command delegation
func TestSerialPortManager_SendCommand(t *testing.T) {
	mockMux := serialmux.NewMockSerialMux([]byte(""))
	snapshot := SerialConfigSnapshot{
		PortPath: "/dev/test",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
		Source:   "test",
	}

	manager := NewSerialPortManager(nil, mockMux, snapshot, nil)
	defer manager.Close()

	// SendCommand should delegate to the current mux
	err := manager.SendCommand("??")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestSerialPortManager_CloseAndSendCommand tests that SendCommand fails after Close
func TestSerialPortManager_CloseAndSendCommand(t *testing.T) {
	mockMux := serialmux.NewMockSerialMux([]byte(""))
	snapshot := SerialConfigSnapshot{
		PortPath: "/dev/test",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
		Source:   "test",
	}

	manager := NewSerialPortManager(nil, mockMux, snapshot, nil)
	manager.Close()

	// SendCommand should fail after Close
	err := manager.SendCommand("??")
	if err == nil {
		t.Error("Expected error after Close, got nil")
	}
}

// TestSerialPortManager_CloseIsIdempotent guards against the original
// double-close panic: close(m.eventFanoutCh) without a guard panics on a
// second invocation. sync.Once makes Close() safe to call repeatedly,
// which matters for shutdown paths that may run via both signal handler
// and explicit teardown.
func TestSerialPortManager_CloseIsIdempotent(t *testing.T) {
	manager := NewSerialPortManager(nil, serialmux.NewMockSerialMux([]byte("")), SerialConfigSnapshot{}, nil)

	if err := manager.Close(); err != nil {
		t.Fatalf("first Close: unexpected error: %v", err)
	}
	// Second call must not panic and must not return an error.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("second Close panicked: %v", r)
		}
	}()
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close: unexpected error: %v", err)
	}
}

// TestSerialPortManager_AdminRoutesFollowReload guards against the original
// AttachAdminRoutes(mux *http.ServeMux) bug where the admin handlers closed
// over the concrete mux at registration time. After a hot-reload swapped
// m.current, the /debug/send-command-api handler kept writing to the OLD
// (possibly closed) mux. The fix routes admin handlers through the manager
// itself so each request resolves m.current at call time.
//
// We verify by:
//  1. registering admin routes on a manager backed by port1
//  2. POSTing a command and checking port1 received it
//  3. swapping the manager's current mux to one backed by port2
//  4. POSTing a different command and checking port2 (not port1) received it
func TestSerialPortManager_AdminRoutesFollowReload(t *testing.T) {
	port1 := serialmux.NewTestableSerialPort()
	mux1 := serialmux.NewSerialMux(port1)
	mgr := NewSerialPortManager(nil, mux1, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	httpMux := http.NewServeMux()
	mgr.AttachAdminRoutes(httpMux)

	post := func(cmd string) int {
		body := strings.NewReader(url.Values{"command": {cmd}}.Encode())
		req := httptest.NewRequest(http.MethodPost, "/debug/send-command-api", body)
		req.RemoteAddr = "127.0.0.1:12345" // bypass tsweb loopback check
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		httpMux.ServeHTTP(w, req)
		return w.Code
	}

	if code := post("FIRST"); code != http.StatusOK {
		t.Fatalf("first POST: expected 200, got %d", code)
	}
	if !strings.Contains(string(port1.GetWrittenData()), "FIRST") {
		t.Fatalf("port1 should have received FIRST, got %q", port1.GetWrittenData())
	}

	// Swap the underlying mux. In production this happens via ReloadConfig;
	// for the regression we exercise the same code path by mutating
	// m.current directly so we don't need a real DB + factory.
	port2 := serialmux.NewTestableSerialPort()
	mux2 := serialmux.NewSerialMux(port2)
	mgr.mu.Lock()
	mgr.current = mux2
	mgr.mu.Unlock()

	if code := post("SECOND"); code != http.StatusOK {
		t.Fatalf("second POST: expected 200, got %d", code)
	}
	if !strings.Contains(string(port2.GetWrittenData()), "SECOND") {
		t.Fatalf("port2 should have received SECOND after swap, got %q (port1 saw %q)",
			port2.GetWrittenData(), port1.GetWrittenData())
	}
	if strings.Contains(string(port1.GetWrittenData()), "SECOND") {
		t.Fatalf("port1 must NOT see SECOND after the swap; got %q", port1.GetWrittenData())
	}
}

// TestSerialPortManager_Snapshot tests configuration snapshot
func TestSerialPortManager_Snapshot(t *testing.T) {
	snapshot := SerialConfigSnapshot{
		ConfigID: 42,
		PortPath: "/dev/ttyUSB0",
		Source:   "database",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}

	manager := NewSerialPortManager(nil, nil, snapshot, nil)
	defer manager.Close()

	got := manager.Snapshot()
	if got.ConfigID != 42 {
		t.Errorf("Expected config ID 42, got %d", got.ConfigID)
	}
	if got.PortPath != "/dev/ttyUSB0" {
		t.Errorf("Expected port '/dev/ttyUSB0', got '%s'", got.PortPath)
	}
}

// TestSerialPortManager_EmptySnapshot tests empty snapshot when no config applied
func TestSerialPortManager_EmptySnapshot(t *testing.T) {
	manager := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer manager.Close()

	got := manager.Snapshot()
	if got.PortPath != "" {
		t.Errorf("Expected empty port path, got '%s'", got.PortPath)
	}
}

// TestSerialPortManager_SubscribeAfterClose tests that Subscribe returns closed channel after Close
func TestSerialPortManager_SubscribeAfterClose(t *testing.T) {
	manager := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	manager.Close()

	// Allow fanout to shut down
	time.Sleep(50 * time.Millisecond)

	id, ch := manager.Subscribe()
	if id != "" {
		t.Errorf("Expected empty ID after close, got %q", id)
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after manager is closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should be closed immediately")
	}
}
