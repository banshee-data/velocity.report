package serialmux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSerialPort implements SerialPorter for testing SerialMux operations
type TestSerialPort struct {
	readData    []byte
	readIndex   int
	writtenData bytes.Buffer
	writeErr    error
	closeErr    error
	closed      bool
	mu          sync.Mutex
}

func NewTestSerialPort(data string) *TestSerialPort {
	return &TestSerialPort{
		readData: []byte(data),
	}
}

func (p *TestSerialPort) Read(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.EOF
	}
	if p.readIndex >= len(p.readData) {
		// Block until closed to simulate waiting for more data
		time.Sleep(10 * time.Millisecond)
		if p.closed {
			return 0, io.EOF
		}
		return 0, nil
	}
	n := copy(buf, p.readData[p.readIndex:])
	p.readIndex += n
	return n, nil
}

func (p *TestSerialPort) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.writeErr != nil {
		return 0, p.writeErr
	}
	return p.writtenData.Write(data)
}

func (p *TestSerialPort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.closeErr
}

func (p *TestSerialPort) SetWriteError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeErr = err
}

func (p *TestSerialPort) WrittenData() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.writtenData.String()
}

// TestNewSerialMux tests creation of a new SerialMux
func TestNewSerialMux(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	if mux == nil {
		t.Fatal("NewSerialMux returned nil")
	}
	if mux.port != port {
		t.Error("SerialMux port not set correctly")
	}
	if mux.subscribers == nil {
		t.Error("SerialMux subscribers map not initialized")
	}
}

// TestSerialMux_Subscribe tests subscribing to the serial mux
func TestSerialMux_Subscribe(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	id1, ch1 := mux.Subscribe()
	id2, ch2 := mux.Subscribe()

	if id1 == "" {
		t.Error("First subscription returned empty ID")
	}
	if id2 == "" {
		t.Error("Second subscription returned empty ID")
	}
	if id1 == id2 {
		t.Error("Subscription IDs should be unique")
	}
	if ch1 == nil {
		t.Error("First subscription returned nil channel")
	}
	if ch2 == nil {
		t.Error("Second subscription returned nil channel")
	}

	// Verify both are in subscribers map
	mux.subscriberMu.Lock()
	if len(mux.subscribers) != 2 {
		t.Errorf("Expected 2 subscribers, got %d", len(mux.subscribers))
	}
	mux.subscriberMu.Unlock()
}

// TestSerialMux_Unsubscribe tests unsubscribing from the serial mux
func TestSerialMux_Unsubscribe(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	id, ch := mux.Subscribe()

	// Start a goroutine to detect channel closure
	done := make(chan bool)
	go func() {
		_, ok := <-ch
		if ok {
			t.Error("Expected channel to be closed")
		}
		done <- true
	}()

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	mux.Unsubscribe(id)

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for channel closure")
	}

	// Verify removed from map
	mux.subscriberMu.Lock()
	if len(mux.subscribers) != 0 {
		t.Errorf("Expected 0 subscribers, got %d", len(mux.subscribers))
	}
	mux.subscriberMu.Unlock()
}

// TestSerialMux_Unsubscribe_NonExistent tests unsubscribing with invalid ID
func TestSerialMux_Unsubscribe_NonExistent(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	// Should not panic
	mux.Unsubscribe("non-existent-id")
}

// TestSerialMux_SendCommand tests sending commands to the serial port
func TestSerialMux_SendCommand(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	tests := []struct {
		name        string
		command     string
		expectedEnd string
	}{
		{"command without newline", "OJ", "OJ\n"},
		{"command with newline", "OS\n", "OS\n"},
		{"query command", "??", "??\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mux.SendCommand(tt.command)
			if err != nil {
				t.Errorf("SendCommand returned error: %v", err)
			}
		})
	}

	// Verify all commands were written
	written := port.WrittenData()
	if !strings.Contains(written, "OJ\n") {
		t.Error("Expected OJ command to be written")
	}
	if !strings.Contains(written, "OS\n") {
		t.Error("Expected OS command to be written")
	}
}

// TestSerialMux_SendCommand_WriteError tests error handling in SendCommand
func TestSerialMux_SendCommand_WriteError(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	port.SetWriteError(errors.New("write failed"))

	err := mux.SendCommand("OJ")
	if err == nil {
		t.Error("Expected error when write fails")
	}
}

// TestSerialMux_Initialise tests the Initialise method
func TestSerialMux_Initialise(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	err := mux.Initialise()
	if err != nil {
		t.Errorf("Initialise returned error: %v", err)
	}

	// Verify commands were sent
	written := port.WrittenData()
	expectedCommands := []string{"C=", "CZ", "AX", "OJ", "OS", "oD", "OM", "oM", "OH", "OC"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(written, cmd) {
			t.Errorf("Expected command %s to be written during initialization", cmd)
		}
	}
}

// TestSerialMux_Initialise_WriteError tests Initialise with write failure
func TestSerialMux_Initialise_WriteError(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	port.SetWriteError(errors.New("write failed"))

	err := mux.Initialise()
	if err == nil {
		t.Error("Expected error when write fails during initialization")
	}
}

// TestSerialMux_Close tests closing the serial mux
func TestSerialMux_Close(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	id1, ch1 := mux.Subscribe()
	_, ch2 := mux.Subscribe()

	// Start goroutines to detect channel closure
	done1 := make(chan bool)
	done2 := make(chan bool)

	go func() {
		_, ok := <-ch1
		if ok {
			t.Error("Expected channel 1 to be closed")
		}
		done1 <- true
	}()

	go func() {
		_, ok := <-ch2
		if ok {
			t.Error("Expected channel 2 to be closed")
		}
		done2 <- true
	}()

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	err := mux.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	select {
	case <-done1:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for channel 1 closure")
	}

	select {
	case <-done2:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for channel 2 closure")
	}

	// Verify subscribers map is empty
	mux.subscriberMu.Lock()
	if len(mux.subscribers) != 0 {
		t.Errorf("Expected 0 subscribers after close, got %d", len(mux.subscribers))
	}
	mux.subscriberMu.Unlock()

	// Verify closing flag is set
	mux.closingMu.Lock()
	if !mux.closing {
		t.Error("Expected closing flag to be true after Close")
	}
	mux.closingMu.Unlock()

	// Unsubscribing after close should be safe
	mux.Unsubscribe(id1)
}

// TestSerialMux_Monitor tests the Monitor method with context cancellation
func TestSerialMux_Monitor(t *testing.T) {
	// Create a port with some test data
	port := NewTestSerialPort("line1\nline2\nline3\n")
	mux := NewSerialMux(port)

	_, ch := mux.Subscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start monitoring in background
	done := make(chan error, 1)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	// Read lines from subscriber channel
	received := make([]string, 0)
	timeout := time.After(200 * time.Millisecond)

loop:
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				break loop
			}
			received = append(received, line)
		case <-timeout:
			break loop
		}
	}

	// Wait for monitor to complete
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Logf("Monitor returned: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Log("Monitor still running")
	}
}

// TestSerialMux_Monitor_ScanError tests Monitor with scanner error
func TestSerialMux_Monitor_ScanError(t *testing.T) {
	port := &ErrorReadPort{errAfter: 2}
	mux := NewSerialMux(port)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := mux.Monitor(ctx)
	// Should get either the read error or context timeout
	if err != nil {
		t.Logf("Monitor returned error (expected): %v", err)
	}
}

// TestSerialMux_Monitor_CloseDuringRead tests closing while Monitor is reading
func TestSerialMux_Monitor_CloseDuringRead(t *testing.T) {
	port := NewTestSerialPort("line1\nline2\nline3\nline4\n")
	mux := NewSerialMux(port)

	_, ch := mux.Subscribe()

	ctx := context.Background()

	// Start monitoring in background
	done := make(chan error, 1)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	// Read a line to ensure monitor is running
	select {
	case <-ch:
		// Got a line
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for first line")
	}

	// Now close the mux
	if err := mux.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Monitor should exit
	select {
	case err := <-done:
		if err != nil {
			t.Logf("Monitor returned: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Monitor did not exit after Close")
	}
}

// ErrorReadPort simulates a port that returns an error after N reads
type ErrorReadPort struct {
	readCount int
	errAfter  int
	closed    bool
}

func (p *ErrorReadPort) Read(buf []byte) (int, error) {
	if p.closed {
		return 0, io.EOF
	}
	p.readCount++
	if p.readCount > p.errAfter {
		return 0, errors.New("simulated read error")
	}
	// Return a newline to simulate a line
	if len(buf) > 0 {
		buf[0] = '\n'
		return 1, nil
	}
	return 0, nil
}

func (p *ErrorReadPort) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *ErrorReadPort) Close() error {
	p.closed = true
	return nil
}

// TestSerialMux_AttachAdminRoutes tests the admin routes attachment
func TestSerialMux_AttachAdminRoutes(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	// Debug routes are protected by tailscale auth, so they return 403 when not authorized
	// We test that the routes are registered and respond (even if with 403)

	// Test send-command-api endpoint - should be registered (returns 403 unauthorized)
	t.Run("send-command-api_registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/debug/send-command-api", strings.NewReader("command=OJ"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		httpMux.ServeHTTP(w, req)

		// The route is registered - it will return 403 (forbidden) because of tailscale auth
		// or return 200/400/etc if auth passes. Either is fine.
		if w.Code == http.StatusNotFound {
			t.Errorf("Route /debug/send-command-api should be registered, got 404")
		}
	})

	// Test tail.js endpoint - should be registered
	t.Run("tail.js_registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/tail.js", nil)
		w := httptest.NewRecorder()
		httpMux.ServeHTTP(w, req)

		// Should be registered, returns 403 for unauthorized access
		if w.Code == http.StatusNotFound {
			t.Errorf("Route /debug/tail.js should be registered, got 404")
		}
	})

	// Test tail endpoint - should be registered
	t.Run("tail_registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/tail", nil)
		w := httptest.NewRecorder()
		httpMux.ServeHTTP(w, req)

		// Should be registered, returns 403 for unauthorized access
		if w.Code == http.StatusNotFound {
			t.Errorf("Route /debug/tail should be registered, got 404")
		}
	})

	// Test send-command - should be registered
	t.Run("send-command_registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/send-command", nil)
		w := httptest.NewRecorder()
		httpMux.ServeHTTP(w, req)

		// Should be registered
		if w.Code == http.StatusNotFound {
			t.Errorf("Route /debug/send-command should be registered, got 404")
		}
	})
}

// TestRandomID tests the randomID helper function
func TestRandomID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := randomID()
		if len(id) != 16 { // 8 bytes hex encoded = 16 chars
			t.Errorf("Expected ID length 16, got %d", len(id))
		}
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

// TestErrWriteFailed tests the error constant
func TestErrWriteFailed(t *testing.T) {
	if ErrWriteFailed == nil {
		t.Error("ErrWriteFailed should not be nil")
	}
	if ErrWriteFailed.Error() == "" {
		t.Error("ErrWriteFailed should have error message")
	}
}

// TestSerialMux_SendCommand_PartialWrite tests handling of partial writes
func TestSerialMux_SendCommand_PartialWrite(t *testing.T) {
	port := &PartialWritePort{maxWrite: 1}
	mux := NewSerialMux(port)

	err := mux.SendCommand("OJ")
	if !errors.Is(err, ErrWriteFailed) {
		t.Errorf("Expected ErrWriteFailed for partial write, got %v", err)
	}
}

// PartialWritePort is a test port that only writes a limited number of bytes
type PartialWritePort struct {
	maxWrite int
	written  []byte
	closed   bool
}

func (p *PartialWritePort) Read(buf []byte) (int, error) {
	return 0, io.EOF
}

func (p *PartialWritePort) Write(data []byte) (int, error) {
	if p.maxWrite > 0 && len(data) > p.maxWrite {
		p.written = append(p.written, data[:p.maxWrite]...)
		return p.maxWrite, nil
	}
	p.written = append(p.written, data...)
	return len(data), nil
}

func (p *PartialWritePort) Close() error {
	p.closed = true
	return nil
}
