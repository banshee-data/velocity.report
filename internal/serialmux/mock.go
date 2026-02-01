package serialmux

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// MockSerialPort implements SerialPorter for testing
type MockSerialPort struct {
	io.Reader
	io.WriteCloser
}

func (m *MockSerialPort) SyncClock() error {
	// Mock implementation does nothing
	return nil
}

func (m *MockSerialPort) Write(p []byte) (n int, err error) {
	return m.WriteCloser.Write(p)
}

// NewMockSerialMux creates a SerialMux instance backed by a mock serial port
func NewMockSerialMux(mockLine []byte) *SerialMux[*MockSerialPort] {
	r, w := io.Pipe()
	f, err := os.CreateTemp(".", "mock_serial_port")
	if err != nil {
		panic("failed to create temp file for mock serial port: " + err.Error())
	}
	log.Printf("Writing mock serial port received input at %s", f.Name())

	mockPort := &MockSerialPort{
		Reader:      r,
		WriteCloser: f,
	}

	// generate data periodically to simulate serial port input
	go func() {
		defer w.Close()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			w.Write(mockLine)
		}
	}()

	return NewSerialMux(mockPort)
}

// TestableSerialPort implements SerialPorter with configurable behaviour for testing.
// It provides fine-grained control over reads, writes, errors, and latency.
type TestableSerialPort struct {
	mu sync.Mutex

	// ReadBuffer holds data to be returned by Read calls
	ReadBuffer *bytes.Buffer

	// WriteBuffer captures data written to the port
	WriteBuffer *bytes.Buffer

	// ReadLatency adds a delay to each Read call
	ReadLatency time.Duration

	// WriteLatency adds a delay to each Write call
	WriteLatency time.Duration

	// ReadError is returned by the next Read call if set
	ReadError error

	// WriteError is returned by the next Write call if set
	WriteError error

	// CloseError is returned by Close if set
	CloseError error

	// Closed indicates whether Close was called
	Closed bool

	// ReadCalls records the number of Read calls
	ReadCalls int

	// WriteCalls records the number of Write calls
	WriteCalls int

	// ReadTimeout is the current read timeout
	ReadTimeout time.Duration

	// BlockReads causes Read to block until data is added or Close is called
	BlockReads bool

	// readCond is used to signal blocked readers
	readCond *sync.Cond
}

// NewTestableSerialPort creates a new TestableSerialPort for testing.
func NewTestableSerialPort() *TestableSerialPort {
	tsp := &TestableSerialPort{
		ReadBuffer:  bytes.NewBuffer(nil),
		WriteBuffer: bytes.NewBuffer(nil),
	}
	tsp.readCond = sync.NewCond(&tsp.mu)
	return tsp
}

// Read reads from the read buffer, optionally simulating latency and errors.
func (t *TestableSerialPort) Read(p []byte) (n int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ReadCalls++

	if t.Closed {
		return 0, errors.New("serial port closed")
	}

	if t.ReadError != nil {
		err := t.ReadError
		t.ReadError = nil
		return 0, err
	}

	if t.ReadLatency > 0 {
		t.mu.Unlock()
		time.Sleep(t.ReadLatency)
		t.mu.Lock()
	}

	// If blocking reads are enabled and buffer is empty, wait for data
	if t.BlockReads && t.ReadBuffer.Len() == 0 {
		for !t.Closed && t.ReadBuffer.Len() == 0 {
			t.readCond.Wait()
		}
		if t.Closed {
			return 0, errors.New("serial port closed")
		}
	}

	return t.ReadBuffer.Read(p)
}

// Write writes to the write buffer, optionally simulating latency and errors.
func (t *TestableSerialPort) Write(p []byte) (n int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.WriteCalls++

	if t.Closed {
		return 0, errors.New("serial port closed")
	}

	if t.WriteError != nil {
		err := t.WriteError
		t.WriteError = nil
		return 0, err
	}

	if t.WriteLatency > 0 {
		t.mu.Unlock()
		time.Sleep(t.WriteLatency)
		t.mu.Lock()
	}

	return t.WriteBuffer.Write(p)
}

// Close marks the port as closed.
func (t *TestableSerialPort) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Closed = true
	t.readCond.Broadcast() // Wake up any blocked readers

	return t.CloseError
}

// SetReadTimeout implements TimeoutSerialPorter.
func (t *TestableSerialPort) SetReadTimeout(timeout time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ReadTimeout = timeout
	return nil
}

// AddReadData adds data to be returned by subsequent Read calls.
func (t *TestableSerialPort) AddReadData(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ReadBuffer.Write(data)
	t.readCond.Signal() // Wake up a blocked reader
}

// GetWrittenData returns all data written to the port.
func (t *TestableSerialPort) GetWrittenData() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.WriteBuffer.Bytes()
}

// Reset clears all buffers and resets state.
func (t *TestableSerialPort) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ReadBuffer.Reset()
	t.WriteBuffer.Reset()
	t.ReadCalls = 0
	t.WriteCalls = 0
	t.Closed = false
	t.ReadError = nil
	t.WriteError = nil
	t.CloseError = nil
	t.ReadLatency = 0
	t.WriteLatency = 0
}

// MockSerialPortFactory implements SerialPortFactory for testing.
type MockSerialPortFactory struct {
	mu sync.Mutex

	// Port is the port to return from Open
	Port SerialPorter

	// Error is returned by Open if set
	Error error

	// OpenCalls records all Open calls
	OpenCalls []MockOpenCall
}

// MockOpenCall records details of an Open call.
type MockOpenCall struct {
	Path string
	Mode *SerialPortMode
}

// NewMockSerialPortFactory creates a new MockSerialPortFactory.
func NewMockSerialPortFactory(port SerialPorter) *MockSerialPortFactory {
	return &MockSerialPortFactory{Port: port}
}

// Open returns the configured port or error.
func (f *MockSerialPortFactory) Open(path string, mode *SerialPortMode) (SerialPorter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.OpenCalls = append(f.OpenCalls, MockOpenCall{
		Path: path,
		Mode: mode,
	})

	if f.Error != nil {
		return nil, f.Error
	}

	return f.Port, nil
}

// LastCall returns the most recent Open call, or nil if none.
func (f *MockSerialPortFactory) LastCall() *MockOpenCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.OpenCalls) == 0 {
		return nil
	}
	return &f.OpenCalls[len(f.OpenCalls)-1]
}

// Reset clears all recorded calls.
func (f *MockSerialPortFactory) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.OpenCalls = nil
	f.Error = nil
}
