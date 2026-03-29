package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

type cmdFakeRunner struct{}

func (cmdFakeRunner) Run(string, ...string) error { return nil }

func TestRunBackupDelegatesToManager(t *testing.T) {
	tmp := t.TempDir()
	binaryPath := filepath.Join(tmp, "bin", "velocity-report")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := ctl.Config{
		BinaryName:      "velocity-report",
		BinaryPath:      binaryPath,
		BackupDir:       filepath.Join(tmp, "backups"),
		DBPath:          filepath.Join(tmp, "sensor_data.db"),
		CurrentVersion:  "0.5.1",
		GOOS:            "linux",
		GOARCH:          "arm64",
		RequestTimeout:  time.Second,
		DownloadTimeout: time.Second,
	}

	var out bytes.Buffer
	old := ctlManager
	ctlManager = ctl.NewManager(cfg, nil, cmdFakeRunner{}, &out, &out)
	defer func() { ctlManager = old }()

	if err := runBackup([]string{"--output", cfg.BackupDir}); err != nil {
		t.Fatalf("runBackup failed: %v", err)
	}

	entries, err := os.ReadDir(cfg.BackupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one backup directory, got %d", len(entries))
	}
}
