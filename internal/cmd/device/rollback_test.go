package device

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

func TestRunRollbackNoPrevious(t *testing.T) {
	tmp := t.TempDir()
	cfg := ctl.Config{
		InstallRoot:     filepath.Join(tmp, "opt"),
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
	if err := os.MkdirAll(cfg.InstallRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// With no `previous` symlink, rollback must refuse.
	err := runRollback([]string{})
	if err == nil || !strings.Contains(err.Error(), "no previous version") {
		t.Fatalf("expected no-previous error, got: %v", err)
	}
}
