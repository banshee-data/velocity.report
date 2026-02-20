package serialmux

import (
	"fmt"
	"strings"

	"go.bug.st/serial"
)

// standardBaudRates defines the set of commonly supported baud rates.
// Reference: https://en.wikipedia.org/wiki/Serial_port#Common_baud_rates
var standardBaudRates = map[int]struct{}{
	110: {}, 300: {}, 600: {}, 1200: {}, 2400: {}, 4800: {}, 9600: {},
	14400: {}, 19200: {}, 28800: {}, 38400: {}, 57600: {}, 115200: {}, 128000: {}, 256000: {},
}

// supportedBaudRatesStr is the string representation of supported baud rates for error messages.
const supportedBaudRatesStr = "110, 300, 600, 1200, 2400, 4800, 9600, 14400, 19200, 28800, 38400, 57600, 115200, 128000, 256000"

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

// Normalise validates the options and applies defaults for any unset values.
func (o PortOptions) Normalise() (PortOptions, error) {
	opts := o

	if opts.BaudRate <= 0 {
		opts.BaudRate = 19200
	}

	// Validate against the standard baud rates map
	if _, ok := standardBaudRates[opts.BaudRate]; !ok {
		return opts, fmt.Errorf("invalid baud rate %d: supported values are %s", opts.BaudRate, supportedBaudRatesStr)
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
// If either configuration is invalid (i.e., Normalise returns an error), Equal returns false and the error.
func (o PortOptions) Equal(other PortOptions) (bool, error) {
	normalisedA, errA := o.Normalise()
	normalisedB, errB := other.Normalise()
	if errA != nil {
		return false, fmt.Errorf("first PortOptions invalid: %w", errA)
	}
	if errB != nil {
		return false, fmt.Errorf("second PortOptions invalid: %w", errB)
	}

	equal := normalisedA.BaudRate == normalisedB.BaudRate &&
		normalisedA.DataBits == normalisedB.DataBits &&
		normalisedA.StopBits == normalisedB.StopBits &&
		normalisedA.Parity == normalisedB.Parity
	return equal, nil
}

// SerialMode converts the port options into the serial.Mode structure required by
// go.bug.st/serial when opening a port.
func (o PortOptions) SerialMode() (*serial.Mode, error) {
	opts, err := o.Normalise()
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
	}

	return mode, nil
}
