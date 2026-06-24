//go:build pcap
// +build pcap

package pcapsplit

import (
	"os"
	"path/filepath"
	"testing"
)

func corruptPCAP(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "junk.pcap")
	if err := os.WriteFile(p, []byte("not a pcap file"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAnalyse_CorruptPCAP(t *testing.T) {
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = corruptPCAP(t)
	if _, err := Analyse(cfg); err == nil {
		t.Error("Analyse: expected error for corrupt pcap")
	}
}

func TestRun_CorruptPCAP(t *testing.T) {
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = corruptPCAP(t)
	cfg.OutputDir = t.TempDir()
	if err := Run(cfg); err == nil {
		t.Error("Run: expected error for corrupt pcap")
	}
}
