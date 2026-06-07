//go:build !linux

package ctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwapCurrentNonLinuxRemoveError(t *testing.T) {
	linkPath := filepath.Join(t.TempDir(), "current")
	if err := os.MkdirAll(filepath.Join(linkPath, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := swapCurrent(linkPath, "versions/0.5.2"); err == nil {
		t.Fatal("expected remove error for non-empty directory")
	}
}
