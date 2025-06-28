package serialmux

import (
	"io"
)

// SerialPorter defines the minimal interface needed for a serial port
type SerialPorter interface {
	io.ReadWriter
	io.Closer
}
