package main

import "testing"

func TestRunStatusNoSystemd(t *testing.T) {
	// On macOS/CI where systemctl is not available, runStatus should
	// return an error rather than panic.
	err := runStatus([]string{})
	if err == nil {
		t.Skip("systemctl available — test passed on Linux")
	}
	// Expected: "running systemctl" error on non-Linux.
}
