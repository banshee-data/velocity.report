package device

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
	installRoot := filepath.Join(tmp, "opt")
	verBin := filepath.Join(installRoot, "versions", "0.5.1", "velocity")
	if err := os.MkdirAll(filepath.Dir(verBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verBin, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("versions/0.5.1", filepath.Join(installRoot, "current")); err != nil {
		t.Fatal(err)
	}

	cfg := ctl.Config{
		InstallRoot:     installRoot,
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
