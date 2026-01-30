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
