package serialmux

import (
	"bytes"
	"testing"
)

// testWriteCloser wraps a buffer with a Close method
type testWriteCloser struct {
	*bytes.Buffer
}

func (t *testWriteCloser) Close() error {
	return nil
}

func TestMockSerialPort_SyncClock(t *testing.T) {
	port := &MockSerialPort{}

	err := port.SyncClock()

	if err != nil {
		t.Errorf("SyncClock returned unexpected error: %v", err)
	}
}

func TestMockSerialPort_Write(t *testing.T) {
	// Create a buffer to capture writes
	buf := &testWriteCloser{Buffer: &bytes.Buffer{}}

	port := &MockSerialPort{
		WriteCloser: buf,
	}

	testData := []byte("test data")
	n, err := port.Write(testData)

	if err != nil {
		t.Errorf("Write returned unexpected error: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Write returned %d bytes, expected %d", n, len(testData))
	}

	// Verify data was written
	if buf.String() != string(testData) {
		t.Errorf("Written data = %q, expected %q", buf.String(), string(testData))
	}
}

func TestNewMockSerialMux(t *testing.T) {
	// Test creating a mock serial mux with test data
	testData := []byte("test line\n")
	mux := NewMockSerialMux(testData)

	if mux == nil {
		t.Fatal("NewMockSerialMux returned nil")
	}

	// Test basic operations on the mock mux
	id, ch := mux.Subscribe()
	if id == "" {
		t.Error("Subscribe returned empty ID")
	}
	if ch == nil {
		t.Error("Subscribe returned nil channel")
	}

	// Test SendCommand
	err := mux.SendCommand("TEST")
	if err != nil {
		t.Errorf("SendCommand returned error: %v", err)
	}

	// Test Initialise
	err = mux.Initialise()
	if err != nil {
		t.Errorf("Initialise returned error: %v", err)
	}

	// Test Unsubscribe
	mux.Unsubscribe(id)

	// Test Close
	err = mux.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}
