package main

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

type MockSerialPort struct {
	errorMessage string
	buf          []byte
	bytesWritten int
}

func (m *MockSerialPort) Read(p []byte) (n int, err error) {
	if m.errorMessage != "" {
		return 0, fmt.Errorf("error %q", m.errorMessage)
	} else {
		byteCount := copy(p, m.buf)

		m.bytesWritten += byteCount
		m.buf = m.buf[byteCount:] // remove read bytes

		// log.Printf("mockSerialPort Read: %d bytes", byteCount)
		return byteCount, nil
	}

}

func (m *MockSerialPort) SetMode(mode *serial.Mode) error                      { return nil }
func (m *MockSerialPort) Write(p []byte) (n int, err error)                    { return 0, nil }
func (m *MockSerialPort) Drain() error                                         { return nil }
func (m *MockSerialPort) ResetInputBuffer() error                              { return nil }
func (m *MockSerialPort) ResetOutputBuffer() error                             { return nil }
func (m *MockSerialPort) SetDTR(dtr bool) error                                { return nil }
func (m *MockSerialPort) SetRTS(rts bool) error                                { return nil }
func (m *MockSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) { return nil, nil }
func (m *MockSerialPort) SetReadTimeout(t time.Duration) error                 { return nil }
func (m *MockSerialPort) Close() error                                         { return nil }
func (m *MockSerialPort) Break(time.Duration) error                            { return nil }
