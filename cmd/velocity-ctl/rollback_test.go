package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

func TestRunRollbackNoBackups(t *testing.T) {
	tmp := t.TempDir()
	cfg := ctl.Config{
		BinaryName:      "velocity-report",
		BinaryPath:      filepath.Join(tmp, "bin", "velocity-report"),
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
	if err := os.MkdirAll(cfg.BackupDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := runRollback([]string{})
	if err == nil || !strings.Contains(err.Error(), "no backups found") {
		t.Fatalf("expected no backups error, got: %v", err)
	}
}
