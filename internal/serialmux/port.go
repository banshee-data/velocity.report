package serialmux

import (
	"io"
	"time"
)

// SerialPorter defines the minimal interface needed for a serial port.
// This abstraction enables unit testing without real serial hardware.
type SerialPorter interface {
	io.ReadWriter
	io.Closer
}

// SerialPortMode defines serial port configuration parameters.
type SerialPortMode struct {
	BaudRate int
	DataBits int
	Parity   Parity
	StopBits StopBits
}

// Parity defines serial port parity options.
type Parity int

const (
	NoParity Parity = iota
	OddParity
	EvenParity
)

// StopBits defines serial port stop bit options.
type StopBits int

const (
	OneStopBit StopBits = iota
	TwoStopBits
)

// DefaultSerialPortMode returns the default mode for radar sensors.
func DefaultSerialPortMode() *SerialPortMode {
	return &SerialPortMode{
		BaudRate: 19200,
		DataBits: 8,
		Parity:   NoParity,
		StopBits: OneStopBit,
	}
}

// SerialPortFactory defines an interface for creating serial ports.
// This abstraction enables dependency injection of serial port creation.
type SerialPortFactory interface {
	// Open opens a serial port at the specified path with the given mode.
	Open(path string, mode *SerialPortMode) (SerialPorter, error)
}

// SerialPortOpener is a function type for opening serial ports.
// This allows for easier testing by replacing the opener function.
type SerialPortOpener func(path string, mode *SerialPortMode) (SerialPorter, error)

// TimeoutSerialPorter extends SerialPorter with timeout capabilities.
// This is an optional interface that serial ports may implement.
type TimeoutSerialPorter interface {
	SerialPorter
	// SetReadTimeout sets the read timeout for the serial port.
	SetReadTimeout(timeout time.Duration) error
}
