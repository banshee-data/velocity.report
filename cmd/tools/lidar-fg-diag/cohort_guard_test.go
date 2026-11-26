//go:build pcap

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
)

func TestReplayRefusesQualificationBeforeCreatingOutput(t *testing.T) {
	capture, _ := fx.Capture(t, t.TempDir(), "qualification.pcap", "synthetic capture")
	fx.ConfigureCLI(t, capture, "qualification")
	renamed := filepath.Join(t.TempDir(), "development.pcap")
	if err := os.Rename(capture, renamed); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "dump")
	err := runReplay([]string{"-pcap", renamed, "-out", out})
	if err == nil || !strings.Contains(err.Error(), "not eligible") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output created: %v", err)
	}
}
