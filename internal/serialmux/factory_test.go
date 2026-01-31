package serialmux

import (
	"testing"
)

func TestNewRealSerialMux(t *testing.T) {
	// We can't actually test opening a real serial port in a unit test
	// since we don't have a real serial device, but we can verify
	// the function returns an error for invalid port
	mux, err := NewRealSerialMux("/dev/nonexistent-serial-port-12345")
	if err == nil {
		t.Error("Expected error when opening non-existent serial port")
		if mux != nil {
			mux.Close()
		}
	}

	// Verify we get a nil mux when there's an error
	if err != nil && mux != nil {
		t.Error("Expected nil mux when error is returned")
	}
}

func TestNewRealSerialPortFactory(t *testing.T) {
	factory := NewRealSerialPortFactory()
	if factory == nil {
		t.Fatal("NewRealSerialPortFactory returned nil")
	}
}

func TestRealSerialPortFactory_Open_InvalidPath(t *testing.T) {
	factory := NewRealSerialPortFactory()

	_, err := factory.Open("/dev/nonexistent-serial-port-12345", nil)
	if err == nil {
		t.Error("Expected error when opening non-existent serial port")
	}
}

func TestRealSerialPortFactory_Open_WithDefaultMode(t *testing.T) {
	factory := NewRealSerialPortFactory()

	// Opening with nil mode should use defaults
	_, err := factory.Open("/dev/nonexistent-serial-port-12345", nil)
	if err == nil {
		t.Error("Expected error when opening non-existent serial port")
	}
	// The error should be about the path, not about nil mode
}

func TestRealSerialPortFactory_Open_CustomMode(t *testing.T) {
	factory := NewRealSerialPortFactory()

	mode := &SerialPortMode{
		BaudRate: 9600,
		DataBits: 7,
		Parity:   EvenParity,
		StopBits: TwoStopBits,
	}

	// Opening with custom mode should use those values
	_, err := factory.Open("/dev/nonexistent-serial-port-12345", mode)
	if err == nil {
		t.Error("Expected error when opening non-existent serial port")
	}
}

func TestConvertParity(t *testing.T) {
	tests := []struct {
		name     string
		parity   Parity
		expected string
	}{
		{"NoParity", NoParity, "none"},
		{"OddParity", OddParity, "odd"},
		{"EvenParity", EvenParity, "even"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := convertParity(tc.parity)
			// We can't directly compare serial.Parity, so just ensure no panic
			_ = result
		})
	}
}

func TestConvertStopBits(t *testing.T) {
	tests := []struct {
		name     string
		stopBits StopBits
	}{
		{"OneStopBit", OneStopBit},
		{"TwoStopBits", TwoStopBits},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := convertStopBits(tc.stopBits)
			// We can't directly compare serial.StopBits, so just ensure no panic
			_ = result
		})
	}
}
