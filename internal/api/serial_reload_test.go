package api

import (
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

// TestSerialPortManager_Snapshot tests configuration snapshot
func TestSerialPortManager_Snapshot(t *testing.T) {
	snapshot := SerialConfigSnapshot{
		ConfigID: 42,
		Name:     "Test Config",
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
	if got.Name != "Test Config" {
		t.Errorf("Expected name 'Test Config', got '%s'", got.Name)
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
