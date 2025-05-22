package serialmux

import (
	"io"
	"time"
)

// MockSerialPort implements SerialPorter for testing
type MockSerialPort struct {
	ReadData      []byte
	WrittenData   []byte
	ReadError     error
	WriteError    error
	CloseError    error
	Closed        bool
	ReadDelay     time.Duration
	ReadCallCount int
}

func (m *MockSerialPort) Read(p []byte) (n int, err error) {
	if m.ReadError != nil {
		return 0, m.ReadError
	}

	if m.ReadDelay > 0 {
		time.Sleep(m.ReadDelay)
	}

	m.ReadCallCount++

	if len(m.ReadData) == 0 {
		return 0, io.EOF
	}

	n = copy(p, m.ReadData)
	m.ReadData = m.ReadData[n:]
	return n, nil
}

func (m *MockSerialPort) Write(p []byte) (n int, err error) {
	if m.WriteError != nil {
		return 0, m.WriteError
	}
	m.WrittenData = append(m.WrittenData, p...)
	return len(p), nil
}

func (m *MockSerialPort) Close() error {
	m.Closed = true
	return m.CloseError
}

// NewMockSerialMux creates a SerialMux instance backed by a mock serial port
func NewMockSerialMux(mockData []byte) *SerialMux[*MockSerialPort] {
	mockPort := &MockSerialPort{
		ReadData: mockData,
	}
	return NewSerialMux[*MockSerialPort](mockPort)
}
