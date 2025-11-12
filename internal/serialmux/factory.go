package serialmux

import (
	"go.bug.st/serial"
)

// NewRealSerialMux creates a SerialMux instance backed by a real serial port at the
// given path using the provided serial options.
func NewRealSerialMux(path string, opts PortOptions) (*SerialMux[serial.Port], error) {
	mode, err := opts.SerialMode()
	if err != nil {
		return nil, err
	}

	port, err := serial.Open(path, mode)
	if err != nil {
		return nil, err
	}

	return NewSerialMux[serial.Port](port), nil
}
