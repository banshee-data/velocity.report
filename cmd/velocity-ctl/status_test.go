package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

type cmdFailRunner struct {
	err error
}

func (r cmdFailRunner) Run(string, ...string) error {
	return r.err
}

func TestRunStatusReturnsRunnerError(t *testing.T) {
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
	ctlManager = ctl.NewManager(cfg, nil, cmdFailRunner{err: errors.New("boom")}, &out, &out)
	defer func() { ctlManager = old }()

	err := runStatus([]string{})
	if err == nil || !strings.Contains(err.Error(), "running systemctl") {
		t.Fatalf("expected wrapped error, got: %v", err)
	}
}
