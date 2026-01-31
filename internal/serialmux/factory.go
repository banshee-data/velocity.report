package serialmux

import (
	"go.bug.st/serial"
)

// RealSerialPortFactory implements SerialPortFactory using the go.bug.st/serial library.
type RealSerialPortFactory struct{}

// NewRealSerialPortFactory creates a new RealSerialPortFactory.
func NewRealSerialPortFactory() *RealSerialPortFactory {
	return &RealSerialPortFactory{}
}

// Open opens a real serial port at the specified path.
func (f *RealSerialPortFactory) Open(path string, mode *SerialPortMode) (SerialPorter, error) {
	if mode == nil {
		mode = DefaultSerialPortMode()
	}

	serialMode := &serial.Mode{
		BaudRate: mode.BaudRate,
		DataBits: mode.DataBits,
		Parity:   convertParity(mode.Parity),
		StopBits: convertStopBits(mode.StopBits),
	}

	return serial.Open(path, serialMode)
}

// convertParity converts our Parity type to serial.Parity.
func convertParity(p Parity) serial.Parity {
	switch p {
	case OddParity:
		return serial.OddParity
	case EvenParity:
		return serial.EvenParity
	default:
		return serial.NoParity
	}
}

// convertStopBits converts our StopBits type to serial.StopBits.
func convertStopBits(sb StopBits) serial.StopBits {
	switch sb {
	case TwoStopBits:
		return serial.TwoStopBits
	default:
		return serial.OneStopBit
	}
}

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
