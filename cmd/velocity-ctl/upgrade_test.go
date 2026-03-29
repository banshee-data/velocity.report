package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

func TestRunUpgradeCheckOnly(t *testing.T) {
	tmp := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.2","assets":[{"name":"velocity-report-linux-arm64","browser_download_url":"https://example.com/bin"}]}`))
	}))
	defer server.Close()

	cfg := ctl.Config{
		ReleasesAPI:     server.URL,
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

	if err := runUpgrade([]string{"--check"}); err != nil {
		t.Fatalf("runUpgrade failed: %v", err)
	}
}
