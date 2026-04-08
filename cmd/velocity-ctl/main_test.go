package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainUsage(t *testing.T) {
	if usage == "" {
		t.Fatal("usage string should not be empty")
	}
}

func TestRunVersion(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runVersion()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output == "" {
		t.Fatal("runVersion produced no output")
	}
	if !strings.Contains(output, "velocity-ctl") {
		t.Errorf("expected 'velocity-ctl' in output, got: %s", output)
	}
}
