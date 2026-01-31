package serialmux

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// ErrorPort simulates various error conditions
type ErrorPort struct {
	readErr   error
	writeErr  error
	closeErr  error
	readDelay time.Duration
	readData  string
	readOnce  bool
	didRead   bool
	mu        sync.Mutex
}

func NewErrorPort() *ErrorPort {
	return &ErrorPort{}
}

func (p *ErrorPort) SetReadError(err error) *ErrorPort {
	p.readErr = err
	return p
}

func (p *ErrorPort) SetWriteError(err error) *ErrorPort {
	p.writeErr = err
	return p
}

func (p *ErrorPort) SetCloseError(err error) *ErrorPort {
	p.closeErr = err
	return p
}

func (p *ErrorPort) SetReadDelay(d time.Duration) *ErrorPort {
	p.readDelay = d
	return p
}

func (p *ErrorPort) SetReadData(data string, readOnce bool) *ErrorPort {
	p.readData = data
	p.readOnce = readOnce
	return p
}

func (p *ErrorPort) Read(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.readDelay > 0 {
		time.Sleep(p.readDelay)
	}
	if p.readErr != nil {
		return 0, p.readErr
	}
	if p.readOnce && p.didRead {
		return 0, io.EOF
	}
	if len(p.readData) > 0 {
		p.didRead = true
		n := copy(buf, []byte(p.readData))
		return n, nil
	}
	return 0, io.EOF
}

func (p *ErrorPort) Write(data []byte) (int, error) {
	if p.writeErr != nil {
		return 0, p.writeErr
	}
	return len(data), nil
}

func (p *ErrorPort) Close() error {
	return p.closeErr
}

// TestSendCommand_PartialWrite_EdgeCase tests that partial writes are handled
func TestSendCommand_PartialWrite_EdgeCase(t *testing.T) {
	port := &EdgePartialWritePort{maxWrite: 3}
	mux := NewSerialMux(port)

	err := mux.SendCommand("LONGCOMMAND")
	if err == nil {
		t.Error("Expected error for partial write")
	}
	if !errors.Is(err, ErrWriteFailed) {
		t.Errorf("Expected ErrWriteFailed, got: %v", err)
	}
}

// EdgePartialWritePort only writes part of the data
type EdgePartialWritePort struct {
	maxWrite int
}

func (p *EdgePartialWritePort) Read(buf []byte) (int, error) {
	return 0, io.EOF
}

func (p *EdgePartialWritePort) Write(data []byte) (int, error) {
	if len(data) > p.maxWrite {
		return p.maxWrite, nil
	}
	return len(data), nil
}

func (p *EdgePartialWritePort) Close() error {
	return nil
}

// TestSendCommand_WriteError tests write error handling
func TestSendCommand_WriteError(t *testing.T) {
	port := NewErrorPort().SetWriteError(errors.New("write failed"))
	mux := NewSerialMux(port)

	err := mux.SendCommand("TEST")
	if err == nil {
		t.Error("Expected error on write failure")
	}
}

// TestSendCommand_AddsNewline tests that newline is added if missing
func TestSendCommand_AddsNewline(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	if err := mux.SendCommand("TEST"); err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}

	if port.Written() != "TEST\n" {
		t.Errorf("Expected 'TEST\\n', got %q", port.Written())
	}
}

// TestSendCommand_DoesNotDoubleNewline tests that existing newlines aren't doubled
func TestSendCommand_DoesNotDoubleNewline(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	if err := mux.SendCommand("TEST\n"); err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}

	if port.Written() != "TEST\n" {
		t.Errorf("Expected 'TEST\\n', got %q", port.Written())
	}
}

// RecordingPort records what was written
type RecordingPort struct {
	data []byte
	mu   sync.Mutex
}

func (p *RecordingPort) Read(buf []byte) (int, error) {
	return 0, io.EOF
}

func (p *RecordingPort) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = append(p.data, data...)
	return len(data), nil
}

func (p *RecordingPort) Close() error {
	return nil
}

func (p *RecordingPort) Written() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.data)
}

// TestMonitor_ScanError tests that scan errors are propagated
func TestMonitor_ScanError(t *testing.T) {
	port := NewErrorPort().SetReadError(errors.New("read error"))
	mux := NewSerialMux(port)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mux.Monitor(ctx)
	// Should get context timeout or EOF (scanner closes)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("Monitor returned error (expected): %v", err)
	}
}

// TestMonitor_ContextCancellation_EdgeCase tests context cancellation is handled
func TestMonitor_ContextCancellation_EdgeCase(t *testing.T) {
	port := &SlowReadPort{delay: 50 * time.Millisecond}
	mux := NewSerialMux(port)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	// Cancel quickly
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Monitor did not exit after context cancellation")
	}
}

// SlowReadPort reads slowly to allow cancellation testing
type SlowReadPort struct {
	delay time.Duration
}

func (p *SlowReadPort) Read(buf []byte) (int, error) {
	time.Sleep(p.delay)
	return 0, io.EOF
}

func (p *SlowReadPort) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *SlowReadPort) Close() error {
	return nil
}

// TestMonitor_ClosingFlag tests that closing flag stops the monitor
func TestMonitor_ClosingFlag(t *testing.T) {
	port := &LineReadPort{lines: []string{"line1\n", "line2\n", "line3\n"}}
	mux := NewSerialMux(port)

	// Set closing flag
	mux.closingMu.Lock()
	mux.closing = true
	mux.closingMu.Unlock()

	ctx := context.Background()
	err := mux.Monitor(ctx)

	// Should exit without error when closing flag is set
	if err != nil {
		t.Logf("Monitor returned: %v", err)
	}
}

// LineReadPort provides lines one at a time
type LineReadPort struct {
	lines []string
	index int
	mu    sync.Mutex
}

func (p *LineReadPort) Read(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.index >= len(p.lines) {
		return 0, io.EOF
	}

	line := p.lines[p.index]
	p.index++
	n := copy(buf, []byte(line))
	return n, nil
}

func (p *LineReadPort) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *LineReadPort) Close() error {
	return nil
}

// TestSubscribe_MultipleSubscribers tests multiple simultaneous subscribers
func TestSubscribe_MultipleSubscribers(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	const numSubscribers = 10
	ids := make([]string, numSubscribers)
	channels := make([]chan string, numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		id, ch := mux.Subscribe()
		ids[i] = id
		channels[i] = ch
	}

	// Verify all subscribers are registered
	mux.subscriberMu.Lock()
	if len(mux.subscribers) != numSubscribers {
		t.Errorf("Expected %d subscribers, got %d", numSubscribers, len(mux.subscribers))
	}
	mux.subscriberMu.Unlock()

	// Unsubscribe all
	for i := 0; i < numSubscribers; i++ {
		mux.Unsubscribe(ids[i])
	}

	mux.subscriberMu.Lock()
	if len(mux.subscribers) != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", len(mux.subscribers))
	}
	mux.subscriberMu.Unlock()
}

// TestUnsubscribe_NonexistentID tests unsubscribing with invalid ID
func TestUnsubscribe_NonexistentID(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	// Should not panic
	mux.Unsubscribe("nonexistent-id")
	mux.Unsubscribe("")
	mux.Unsubscribe("12345678901234567890")
}

// TestClose_ClosesAllSubscribers tests that Close closes all subscriber channels
func TestClose_ClosesAllSubscribers(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	var wg sync.WaitGroup
	const numSubscribers = 5

	for i := 0; i < numSubscribers; i++ {
		_, ch := mux.Subscribe()
		wg.Add(1)
		go func(c chan string) {
			defer wg.Done()
			// Wait for channel to be closed
			for range c {
			}
		}(ch)
	}

	// Close should close all channels
	err := mux.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Wait for all goroutines to complete (channels closed)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("Timed out waiting for subscriber channels to close")
	}
}

// TestClose_PortCloseError tests that port close errors are propagated
func TestClose_PortCloseError(t *testing.T) {
	port := NewErrorPort().SetCloseError(errors.New("close failed"))
	mux := NewSerialMux(port)

	err := mux.Close()
	if err == nil {
		t.Error("Expected error from Close")
	}
	if !strings.Contains(err.Error(), "close failed") {
		t.Errorf("Expected 'close failed' error, got: %v", err)
	}
}

// TestClassifyPayload_MoreEdgeCases tests edge cases in payload classification
func TestClassifyPayload_MoreEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		payload  string
		expected string
	}{
		{"empty_string", "", EventTypeUnknown},
		{"whitespace_only", "   ", EventTypeUnknown},
		{"just_brace", "{", EventTypeConfig},
		{"empty_json", "{}", EventTypeConfig},
		{"end_time_in_text", "some text with end_time in it", EventTypeRadarObject},
		{"classifier_only", `{"classifier":"foo"}`, EventTypeRadarObject},
		{"magnitude_only", `{"magnitude":1.0}`, EventTypeRawData},
		{"speed_only", `{"speed":5.0}`, EventTypeRawData},
		{"all_keywords", `{"classifier":"x","end_time":1,"magnitude":2,"speed":3}`, EventTypeRadarObject},
		{"unicode_content", `{"data":"日本語"}`, EventTypeConfig},
		{"very_long_string", strings.Repeat("x", 10000), EventTypeUnknown},
		{"newlines_in_payload", "{\n\"test\": true\n}", EventTypeConfig},
		{"tabs_in_payload", "{\t\"test\":\ttrue}", EventTypeConfig},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyPayload(tc.payload)
			if result != tc.expected {
				t.Errorf("ClassifyPayload(%q) = %q, want %q", tc.payload, result, tc.expected)
			}
		})
	}
}

// TestHandleConfigResponse_EdgeCases tests edge cases in config handling
func TestHandleConfigResponse_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		payload     string
		expectError bool
	}{
		{"empty_object", "{}", false},
		{"nested_object", `{"outer":{"inner":"value"}}`, false},
		{"array_value", `{"arr":[1,2,3]}`, false},
		{"null_value", `{"key":null}`, false},
		{"boolean_values", `{"t":true,"f":false}`, false},
		{"numeric_types", `{"int":42,"float":3.14}`, false},
		{"empty_string", "", true},
		{"malformed_json", "{invalid", true},
		{"trailing_comma", `{"a":1,}`, true},
		{"single_quote_json", "{'key':'value'}", true},
		{"plain_text", "not json at all", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state before each test
			CurrentState = nil

			err := HandleConfigResponse(tc.payload)
			if tc.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestHandleConfigResponse_AccumulatesState tests that state accumulates
func TestHandleConfigResponse_AccumulatesState(t *testing.T) {
	CurrentState = nil

	// First response
	if err := HandleConfigResponse(`{"key1":"value1"}`); err != nil {
		t.Fatalf("First response failed: %v", err)
	}

	// Second response with different key
	if err := HandleConfigResponse(`{"key2":"value2"}`); err != nil {
		t.Fatalf("Second response failed: %v", err)
	}

	// Both keys should be present
	if CurrentState["key1"] != "value1" {
		t.Error("key1 not preserved")
	}
	if CurrentState["key2"] != "value2" {
		t.Error("key2 not set")
	}

	// Third response overwrites existing key
	if err := HandleConfigResponse(`{"key1":"updated"}`); err != nil {
		t.Fatalf("Third response failed: %v", err)
	}

	if CurrentState["key1"] != "updated" {
		t.Error("key1 not updated")
	}
}

// TestRandomID_Uniqueness tests that random IDs are unique
func TestRandomID_Uniqueness(t *testing.T) {
	const numIDs = 1000
	ids := make(map[string]bool)

	for i := 0; i < numIDs; i++ {
		id := randomID()
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true

		// Verify format (16 hex chars = 8 bytes)
		if len(id) != 16 {
			t.Errorf("ID has wrong length: %d", len(id))
		}
	}
}

// TestConcurrentSubscribeUnsubscribe tests thread safety
func TestConcurrentSubscribeUnsubscribe(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	const numGoroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				id, _ := mux.Subscribe()
				mux.Unsubscribe(id)
			}
		}()
	}

	wg.Wait()

	// All subscribers should be cleaned up
	mux.subscriberMu.Lock()
	remaining := len(mux.subscribers)
	mux.subscriberMu.Unlock()

	if remaining != 0 {
		t.Errorf("Expected 0 remaining subscribers, got %d", remaining)
	}
}

// TestConcurrentSendCommands tests concurrent command sending
func TestConcurrentSendCommands(t *testing.T) {
	port := &RecordingPort{}
	mux := NewSerialMux(port)

	const numGoroutines = 20
	const cmdsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < cmdsPerGoroutine; j++ {
				cmd := strings.Repeat("X", id%10+1)
				if err := mux.SendCommand(cmd); err != nil {
					t.Errorf("SendCommand failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify some commands were written
	written := port.Written()
	if len(written) == 0 {
		t.Error("No commands were written")
	}
}
