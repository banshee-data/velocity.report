package serialmux

import (
	"bytes"
	"errors"
	"testing"
	"time"
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

func TestTestableSerialPort_ReadWrite(t *testing.T) {
	port := NewTestableSerialPort()

	// Add data to read buffer
	testData := []byte("test data")
	port.AddReadData(testData)

	// Read data
	buf := make([]byte, 100)
	n, err := port.Read(buf)
	if err != nil {
		t.Errorf("Read returned error: %v", err)
	}
	if string(buf[:n]) != string(testData) {
		t.Errorf("Read returned %q, expected %q", string(buf[:n]), string(testData))
	}
	if port.ReadCalls != 1 {
		t.Errorf("Expected 1 read call, got %d", port.ReadCalls)
	}

	// Write data
	writeData := []byte("write data")
	n, err = port.Write(writeData)
	if err != nil {
		t.Errorf("Write returned error: %v", err)
	}
	if n != len(writeData) {
		t.Errorf("Write returned %d, expected %d", n, len(writeData))
	}
	if port.WriteCalls != 1 {
		t.Errorf("Expected 1 write call, got %d", port.WriteCalls)
	}

	// Verify written data
	if string(port.GetWrittenData()) != string(writeData) {
		t.Errorf("Written data = %q, expected %q", string(port.GetWrittenData()), string(writeData))
	}
}

func TestTestableSerialPort_Errors(t *testing.T) {
	port := NewTestableSerialPort()

	// Test read error
	port.ReadError = errors.New("read error")
	_, err := port.Read(make([]byte, 10))
	if err == nil || err.Error() != "read error" {
		t.Errorf("Expected 'read error', got: %v", err)
	}
	// Error should be cleared
	port.AddReadData([]byte("x"))
	_, err = port.Read(make([]byte, 10))
	if err != nil {
		t.Errorf("Expected no error after error cleared, got: %v", err)
	}

	// Test write error
	port.WriteError = errors.New("write error")
	_, err = port.Write([]byte("test"))
	if err == nil || err.Error() != "write error" {
		t.Errorf("Expected 'write error', got: %v", err)
	}

	// Test close error
	port.CloseError = errors.New("close error")
	err = port.Close()
	if err == nil || err.Error() != "close error" {
		t.Errorf("Expected 'close error', got: %v", err)
	}
}

func TestTestableSerialPort_Closed(t *testing.T) {
	port := NewTestableSerialPort()

	// Close the port
	port.Close()

	if !port.Closed {
		t.Error("Expected port to be closed")
	}

	// Read should fail
	_, err := port.Read(make([]byte, 10))
	if err == nil {
		t.Error("Expected error reading from closed port")
	}

	// Write should fail
	_, err = port.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error writing to closed port")
	}
}

func TestTestableSerialPort_Latency(t *testing.T) {
	port := NewTestableSerialPort()
	port.ReadLatency = 50 * time.Millisecond
	port.WriteLatency = 50 * time.Millisecond

	port.AddReadData([]byte("test"))

	// Measure read time
	start := time.Now()
	port.Read(make([]byte, 10))
	readDuration := time.Since(start)
	if readDuration < 40*time.Millisecond {
		t.Errorf("Read was too fast: %v", readDuration)
	}

	// Measure write time
	start = time.Now()
	port.Write([]byte("test"))
	writeDuration := time.Since(start)
	if writeDuration < 40*time.Millisecond {
		t.Errorf("Write was too fast: %v", writeDuration)
	}
}

func TestTestableSerialPort_SetReadTimeout(t *testing.T) {
	port := NewTestableSerialPort()

	err := port.SetReadTimeout(100 * time.Millisecond)
	if err != nil {
		t.Errorf("SetReadTimeout returned error: %v", err)
	}
	if port.ReadTimeout != 100*time.Millisecond {
		t.Errorf("Expected timeout 100ms, got %v", port.ReadTimeout)
	}
}

func TestTestableSerialPort_Reset(t *testing.T) {
	port := NewTestableSerialPort()

	// Set up state
	port.AddReadData([]byte("test"))
	port.Write([]byte("write"))
	port.ReadError = errors.New("error")
	port.WriteError = errors.New("error")
	port.ReadLatency = time.Second
	port.Close()

	// Reset
	port.Reset()

	// Verify reset state
	if port.ReadCalls != 0 {
		t.Errorf("Expected ReadCalls 0, got %d", port.ReadCalls)
	}
	if port.WriteCalls != 0 {
		t.Errorf("Expected WriteCalls 0, got %d", port.WriteCalls)
	}
	if port.Closed {
		t.Error("Expected port not closed")
	}
	if port.ReadError != nil || port.WriteError != nil {
		t.Error("Expected errors to be nil")
	}
	if port.ReadLatency != 0 {
		t.Error("Expected latency to be 0")
	}
	if port.ReadBuffer.Len() != 0 {
		t.Error("Expected ReadBuffer to be empty")
	}
	if port.WriteBuffer.Len() != 0 {
		t.Error("Expected WriteBuffer to be empty")
	}
}

func TestMockSerialPortFactory(t *testing.T) {
	port := NewTestableSerialPort()
	factory := NewMockSerialPortFactory(port)

	mode := &SerialPortMode{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   NoParity,
		StopBits: OneStopBit,
	}

	result, err := factory.Open("/dev/ttyUSB0", mode)
	if err != nil {
		t.Errorf("Open returned error: %v", err)
	}
	if result != port {
		t.Error("Expected returned port to match configured port")
	}

	// Verify call was recorded
	if len(factory.OpenCalls) != 1 {
		t.Fatalf("Expected 1 open call, got %d", len(factory.OpenCalls))
	}
	if factory.OpenCalls[0].Path != "/dev/ttyUSB0" {
		t.Errorf("Expected path '/dev/ttyUSB0', got '%s'", factory.OpenCalls[0].Path)
	}
	if factory.OpenCalls[0].Mode.BaudRate != 9600 {
		t.Errorf("Expected baud rate 9600, got %d", factory.OpenCalls[0].Mode.BaudRate)
	}
}

func TestMockSerialPortFactory_Error(t *testing.T) {
	factory := NewMockSerialPortFactory(nil)
	factory.Error = errors.New("open error")

	_, err := factory.Open("/dev/ttyUSB0", nil)
	if err == nil || err.Error() != "open error" {
		t.Errorf("Expected 'open error', got: %v", err)
	}
}

func TestMockSerialPortFactory_LastCall(t *testing.T) {
	port := NewTestableSerialPort()
	factory := NewMockSerialPortFactory(port)

	// No calls yet
	if factory.LastCall() != nil {
		t.Error("Expected nil when no calls")
	}

	factory.Open("/dev/tty1", nil)
	factory.Open("/dev/tty2", nil)

	last := factory.LastCall()
	if last == nil {
		t.Fatal("Expected non-nil last call")
	}
	if last.Path != "/dev/tty2" {
		t.Errorf("Expected '/dev/tty2', got '%s'", last.Path)
	}
}

func TestMockSerialPortFactory_Reset(t *testing.T) {
	port := NewTestableSerialPort()
	factory := NewMockSerialPortFactory(port)
	factory.Open("/dev/tty1", nil)
	factory.Error = errors.New("error")

	factory.Reset()

	if len(factory.OpenCalls) != 0 {
		t.Errorf("Expected 0 calls after reset, got %d", len(factory.OpenCalls))
	}
	if factory.Error != nil {
		t.Error("Expected nil error after reset")
	}
}

func TestDefaultSerialPortMode(t *testing.T) {
	mode := DefaultSerialPortMode()

	if mode.BaudRate != 19200 {
		t.Errorf("Expected baud rate 19200, got %d", mode.BaudRate)
	}
	if mode.DataBits != 8 {
		t.Errorf("Expected data bits 8, got %d", mode.DataBits)
	}
	if mode.Parity != NoParity {
		t.Errorf("Expected NoParity, got %v", mode.Parity)
	}
	if mode.StopBits != OneStopBit {
		t.Errorf("Expected OneStopBit, got %v", mode.StopBits)
	}
}
