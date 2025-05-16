package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.bug.st/serial"
)

// MockSerialPort is a mock implementation of serial.Port for testing
type MockSerialPort struct {
	readData      []byte
	writtenData   []byte
	readError     error
	writeError    error
	closeError    error
	closed        bool
	readDelay     time.Duration
	readCallCount int
}

func (m *MockSerialPort) Break(time.Duration) error                            { return nil }
func (m *MockSerialPort) Drain() error                                         { return nil }
func (m *MockSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) { return nil, nil }
func (m *MockSerialPort) ResetInputBuffer() error                              { return nil }
func (m *MockSerialPort) ResetOutputBuffer() error                             { return nil }
func (m *MockSerialPort) SetDTR(dtr bool) error                                { return nil }
func (m *MockSerialPort) SetMode(mode *serial.Mode) error                      { return nil }
func (m *MockSerialPort) SetReadTimeout(t time.Duration) error                 { return nil }
func (m *MockSerialPort) SetRTS(rts bool) error                                { return nil }

func (m *MockSerialPort) Read(p []byte) (int, error) {
	if m.readError != nil {
		return 0, m.readError
	}

	m.readCallCount++
	if m.readDelay > 0 {
		time.Sleep(m.readDelay)
	}

	if len(m.readData) == 0 {
		// Block the read operation if no data is available
		time.Sleep(10 * time.Millisecond)
		return 0, nil
	}

	n := copy(p, m.readData)
	m.readData = m.readData[n:]
	return n, nil
}

func (m *MockSerialPort) Write(p []byte) (int, error) {
	if m.writeError != nil {
		return 0, m.writeError
	}
	m.writtenData = append(m.writtenData, p...)
	return len(p), nil
}

func (m *MockSerialPort) Close() error {
	m.closed = true
	return m.closeError
}

func TestMonitor_ReadLine(t *testing.T) {
	// Setup mock port with test data
	mockPort := &MockSerialPort{
		readData: []byte("test line 1\ntest line 2\n"),
	}

	// Create RadarPort with the mock
	radarPort := &RadarPort{
		Port:     mockPort,
		events:   make(chan string, 10),
		commands: make(chan string, 10),
	}

	// Setup context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start monitoring in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- radarPort.Monitor(ctx)
	}()

	// Read first event
	select {
	case event := <-radarPort.events:
		if event != "test line 1" {
			t.Errorf("Expected 'test line 1', got '%s'", event)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for first event")
	}

	// Read second event
	select {
	case event := <-radarPort.events:
		if event != "test line 2" {
			t.Errorf("Expected 'test line 2', got '%s'", event)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for second event")
	}

	// Cancel context to stop monitoring
	cancel()

	// Check for errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for monitor to stop")
	}

	if !mockPort.closed {
		t.Error("Port was not closed")
	}
}

func TestMonitor_SendCommand(t *testing.T) {
	mockPort := &MockSerialPort{}

	radarPort := &RadarPort{
		Port:     mockPort,
		events:   make(chan string, 10),
		commands: make(chan string, 10),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- radarPort.Monitor(ctx)
	}()

	// Send command
	radarPort.SendCommand("test command")

	// Give some time for command to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify command was written
	expected := "test command"
	if string(mockPort.writtenData) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(mockPort.writtenData))
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for monitor to stop")
	}
}

func TestMonitor_ContextCancellation(t *testing.T) {
	mockPort := &MockSerialPort{
		readDelay: 200 * time.Millisecond, // Add delay to ensure context cancellation is tested
	}

	radarPort := &RadarPort{
		Port:     mockPort,
		events:   make(chan string, 10),
		commands: make(chan string, 10),
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- radarPort.Monitor(ctx)
	}()

	// Cancel immediately
	cancel()

	// Check that monitor stops
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for monitor to stop after context cancellation")
	}

	if !mockPort.closed {
		t.Error("Port was not closed after context cancellation")
	}
}

func TestMonitor_ScanError(t *testing.T) {
	expectedErr := errors.New("read error")
	mockPort := &MockSerialPort{
		readError: expectedErr,
	}

	radarPort := &RadarPort{
		Port:     mockPort,
		events:   make(chan string, 10),
		commands: make(chan string, 10),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := radarPort.Monitor(ctx)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}
