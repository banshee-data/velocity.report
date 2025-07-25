package serialmux

import (
	"go.bug.st/serial"
)

// NewRealSerialMux creates a SerialMux instance backed by a real serial port at the
// given path.
func NewRealSerialMux(path string) (*SerialMux[serial.Port], error) {
	mode := &serial.Mode{
		BaudRate: 19200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(path, mode)
	if err != nil {
		return nil, err
	}

	return NewSerialMux[serial.Port](port), nil
}
