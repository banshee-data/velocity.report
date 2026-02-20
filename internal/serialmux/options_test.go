package serialmux

import (
	"testing"

	"go.bug.st/serial"
)

func TestPortOptions_Normalise_Defaults(t *testing.T) {
	// Zero-value options should get defaults applied
	opts := PortOptions{}
	got, err := opts.Normalise()
	if err != nil {
		t.Fatalf("Normalise() error = %v", err)
	}
	if got.BaudRate != 19200 {
		t.Errorf("BaudRate = %d, want 19200", got.BaudRate)
	}
	if got.DataBits != 8 {
		t.Errorf("DataBits = %d, want 8", got.DataBits)
	}
	if got.StopBits != 1 {
		t.Errorf("StopBits = %d, want 1", got.StopBits)
	}
	if got.Parity != "N" {
		t.Errorf("Parity = %q, want %q", got.Parity, "N")
	}
}

func TestPortOptions_Normalise_ExplicitValues(t *testing.T) {
	opts := PortOptions{BaudRate: 9600, DataBits: 7, StopBits: 2, Parity: "E"}
	got, err := opts.Normalise()
	if err != nil {
		t.Fatalf("Normalise() error = %v", err)
	}
	if got.BaudRate != 9600 {
		t.Errorf("BaudRate = %d, want 9600", got.BaudRate)
	}
	if got.DataBits != 7 {
		t.Errorf("DataBits = %d, want 7", got.DataBits)
	}
	if got.StopBits != 2 {
		t.Errorf("StopBits = %d, want 2", got.StopBits)
	}
	if got.Parity != "E" {
		t.Errorf("Parity = %q, want %q", got.Parity, "E")
	}
}

func TestPortOptions_Normalise_NegativeBaudRate(t *testing.T) {
	opts := PortOptions{BaudRate: -5}
	got, err := opts.Normalise()
	if err != nil {
		t.Fatalf("Normalise() error = %v", err)
	}
	if got.BaudRate != 19200 {
		t.Errorf("negative baud rate should default to 19200, got %d", got.BaudRate)
	}
}

func TestPortOptions_Normalise_InvalidBaudRate(t *testing.T) {
	opts := PortOptions{BaudRate: 12345}
	_, err := opts.Normalise()
	if err == nil {
		t.Error("expected error for invalid baud rate, got nil")
	}
}

func TestPortOptions_Normalise_AllStandardBaudRates(t *testing.T) {
	rates := []int{110, 300, 600, 1200, 2400, 4800, 9600, 14400, 19200, 28800, 38400, 57600, 115200, 128000, 256000}
	for _, rate := range rates {
		opts := PortOptions{BaudRate: rate}
		got, err := opts.Normalise()
		if err != nil {
			t.Errorf("Normalise() with baud %d: unexpected error %v", rate, err)
		}
		if got.BaudRate != rate {
			t.Errorf("Normalise() with baud %d: got %d", rate, got.BaudRate)
		}
	}
}

func TestPortOptions_Normalise_InvalidDataBits(t *testing.T) {
	tests := []struct {
		name     string
		dataBits int
	}{
		{"too low", 4},
		{"too high", 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := PortOptions{DataBits: tc.dataBits}
			_, err := opts.Normalise()
			if err == nil {
				t.Errorf("expected error for data bits %d, got nil", tc.dataBits)
			}
		})
	}
}

func TestPortOptions_Normalise_ValidDataBits(t *testing.T) {
	for bits := 5; bits <= 8; bits++ {
		opts := PortOptions{DataBits: bits}
		got, err := opts.Normalise()
		if err != nil {
			t.Errorf("Normalise() with data bits %d: unexpected error %v", bits, err)
		}
		if got.DataBits != bits {
			t.Errorf("Normalise() with data bits %d: got %d", bits, got.DataBits)
		}
	}
}

func TestPortOptions_Normalise_InvalidStopBits(t *testing.T) {
	opts := PortOptions{StopBits: 3}
	_, err := opts.Normalise()
	if err == nil {
		t.Error("expected error for stop bits 3, got nil")
	}
}

func TestPortOptions_Normalise_ParityVariations(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"N", "N"},
		{"n", "N"},
		{"NONE", "N"},
		{"none", "N"},
		{"E", "E"},
		{"e", "E"},
		{"EVEN", "E"},
		{"even", "E"},
		{"O", "O"},
		{"o", "O"},
		{"ODD", "O"},
		{"odd", "O"},
		{"  N  ", "N"}, // whitespace
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			opts := PortOptions{Parity: tc.input}
			got, err := opts.Normalise()
			if err != nil {
				t.Fatalf("Normalise() with parity %q: unexpected error %v", tc.input, err)
			}
			if got.Parity != tc.want {
				t.Errorf("Normalise() with parity %q: got %q, want %q", tc.input, got.Parity, tc.want)
			}
		})
	}
}

func TestPortOptions_Normalise_InvalidParity(t *testing.T) {
	opts := PortOptions{Parity: "X"}
	_, err := opts.Normalise()
	if err == nil {
		t.Error("expected error for parity X, got nil")
	}
}

func TestPortOptions_Equal(t *testing.T) {
	a := PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"}
	b := PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"}

	eq, err := a.Equal(b)
	if err != nil {
		t.Fatalf("Equal() error = %v", err)
	}
	if !eq {
		t.Error("expected equal, got not equal")
	}
}

func TestPortOptions_Equal_DefaultsMatch(t *testing.T) {
	// Zero values should normalise to the same defaults
	a := PortOptions{}
	b := PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"}

	eq, err := a.Equal(b)
	if err != nil {
		t.Fatalf("Equal() error = %v", err)
	}
	if !eq {
		t.Error("default options should equal explicit defaults")
	}
}

func TestPortOptions_Equal_DifferentBaudRate(t *testing.T) {
	a := PortOptions{BaudRate: 9600}
	b := PortOptions{BaudRate: 19200}

	eq, err := a.Equal(b)
	if err != nil {
		t.Fatalf("Equal() error = %v", err)
	}
	if eq {
		t.Error("expected not equal")
	}
}

func TestPortOptions_Equal_DifferentParity(t *testing.T) {
	a := PortOptions{Parity: "E"}
	b := PortOptions{Parity: "O"}

	eq, err := a.Equal(b)
	if err != nil {
		t.Fatalf("Equal() error = %v", err)
	}
	if eq {
		t.Error("expected not equal")
	}
}

func TestPortOptions_Equal_InvalidFirst(t *testing.T) {
	a := PortOptions{BaudRate: 12345}
	b := PortOptions{}

	_, err := a.Equal(b)
	if err == nil {
		t.Error("expected error for invalid first options, got nil")
	}
}

func TestPortOptions_Equal_InvalidSecond(t *testing.T) {
	a := PortOptions{}
	b := PortOptions{BaudRate: 12345}

	_, err := a.Equal(b)
	if err == nil {
		t.Error("expected error for invalid second options, got nil")
	}
}

func TestPortOptions_SerialMode_Default(t *testing.T) {
	opts := PortOptions{}
	mode, err := opts.SerialMode()
	if err != nil {
		t.Fatalf("SerialMode() error = %v", err)
	}
	if mode.BaudRate != 19200 {
		t.Errorf("BaudRate = %d, want 19200", mode.BaudRate)
	}
	if mode.DataBits != 8 {
		t.Errorf("DataBits = %d, want 8", mode.DataBits)
	}
	// StopBits default is 1, which maps to serial.StopBits(1)
	expectedStopBits := serial.StopBits(1)
	if mode.StopBits != expectedStopBits {
		t.Errorf("StopBits = %v, want %v", mode.StopBits, expectedStopBits)
	}
	if mode.Parity != serial.NoParity {
		t.Errorf("Parity = %v, want NoParity", mode.Parity)
	}
}

func TestPortOptions_SerialMode_EvenParity(t *testing.T) {
	opts := PortOptions{Parity: "E"}
	mode, err := opts.SerialMode()
	if err != nil {
		t.Fatalf("SerialMode() error = %v", err)
	}
	if mode.Parity != serial.EvenParity {
		t.Errorf("Parity = %v, want EvenParity", mode.Parity)
	}
}

func TestPortOptions_SerialMode_OddParity(t *testing.T) {
	opts := PortOptions{Parity: "O"}
	mode, err := opts.SerialMode()
	if err != nil {
		t.Fatalf("SerialMode() error = %v", err)
	}
	if mode.Parity != serial.OddParity {
		t.Errorf("Parity = %v, want OddParity", mode.Parity)
	}
}

func TestPortOptions_SerialMode_TwoStopBits(t *testing.T) {
	opts := PortOptions{StopBits: 2}
	mode, err := opts.SerialMode()
	if err != nil {
		t.Fatalf("SerialMode() error = %v", err)
	}
	expectedStopBits := serial.StopBits(2)
	if mode.StopBits != expectedStopBits {
		t.Errorf("StopBits = %v, want %v", mode.StopBits, expectedStopBits)
	}
}

func TestPortOptions_SerialMode_InvalidOptions(t *testing.T) {
	opts := PortOptions{BaudRate: 12345}
	_, err := opts.SerialMode()
	if err == nil {
		t.Error("expected error for invalid options, got nil")
	}
}
