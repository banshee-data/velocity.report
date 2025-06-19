package serialmux

import (
	"io"
	"log"
	"os"
	"time"
)

// MockSerialPort implements SerialPorter for testing
type MockSerialPort struct {
	io.Reader
	io.WriteCloser
	o io.Writer
}

func (m *MockSerialPort) SyncClock() error {
	// Mock implementation does nothing
	return nil
}

func (m *MockSerialPort) Write(p []byte) (n int, err error) {
	n, err = m.WriteCloser.Write(p)
	m.o.Write(p)
	return n, err
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
		r,
		f,
		w,
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

	return NewSerialMux[*MockSerialPort](mockPort)
}
