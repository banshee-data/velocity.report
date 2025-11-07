package serialmux

import (
	"fmt"
	"strings"

	"go.bug.st/serial"
)

// PortOptions describes the serial connection parameters used when opening a real
// serial port. The fields intentionally mirror the database configuration used by
// the API layer so that the options can be passed through without additional
// translation.
type PortOptions struct {
	BaudRate int    `json:"baud_rate"`
	DataBits int    `json:"data_bits"`
	StopBits int    `json:"stop_bits"`
	Parity   string `json:"parity"`
}

// Normalize validates the options and applies defaults for any unset values.
func (o PortOptions) Normalize() (PortOptions, error) {
	opts := o

	if opts.BaudRate <= 0 {
		opts.BaudRate = 19200
	}

	if opts.DataBits == 0 {
		opts.DataBits = 8
	}
	if opts.DataBits < 5 || opts.DataBits > 8 {
		return opts, fmt.Errorf("invalid data bits %d: must be between 5 and 8", opts.DataBits)
	}

	if opts.StopBits == 0 {
		opts.StopBits = 1
	}
	if opts.StopBits != 1 && opts.StopBits != 2 {
		return opts, fmt.Errorf("invalid stop bits %d: supported values are 1 or 2", opts.StopBits)
	}

	parity := strings.TrimSpace(strings.ToUpper(opts.Parity))
	if parity == "" {
		parity = "N"
	}

	switch parity {
	case "N", "NONE":
		parity = "N"
	case "E", "EVEN":
		parity = "E"
	case "O", "ODD":
		parity = "O"
	default:
		return opts, fmt.Errorf("unsupported parity %q: expected N, E, or O", opts.Parity)
	}

	opts.Parity = parity
	return opts, nil
}

// Equal reports whether two PortOptions describe the same serial configuration.
func (o PortOptions) Equal(other PortOptions) bool {
	normalizedA, errA := o.Normalize()
	normalizedB, errB := other.Normalize()
	if errA != nil || errB != nil {
		return false
	}

	return normalizedA.BaudRate == normalizedB.BaudRate &&
		normalizedA.DataBits == normalizedB.DataBits &&
		normalizedA.StopBits == normalizedB.StopBits &&
		normalizedA.Parity == normalizedB.Parity
}

// SerialMode converts the port options into the serial.Mode structure required by
// go.bug.st/serial when opening a port.
func (o PortOptions) SerialMode() (*serial.Mode, error) {
	opts, err := o.Normalize()
	if err != nil {
		return nil, err
	}

	mode := &serial.Mode{
		BaudRate: opts.BaudRate,
		DataBits: opts.DataBits,
		StopBits: serial.StopBits(opts.StopBits),
	}

	switch opts.Parity {
	case "N":
		mode.Parity = serial.NoParity
	case "E":
		mode.Parity = serial.EvenParity
	case "O":
		mode.Parity = serial.OddParity
	default:
		return nil, fmt.Errorf("unsupported parity %q", opts.Parity)
	}

	return mode, nil
}
