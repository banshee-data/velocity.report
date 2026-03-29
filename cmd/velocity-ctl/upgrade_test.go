package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
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
		ReleasesListAPI: server.URL,
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

func TestLoadIncludePrereleasesFromConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "velocity-ctl.json")
	if err := os.WriteFile(path, []byte(`{"include_prereleases": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	include, err := loadIncludePrereleases(path)
	if err != nil {
		t.Fatalf("loadIncludePrereleases failed: %v", err)
	}
	if !include {
		t.Fatal("expected include_prereleases=true from config")
	}
}

func TestRunUpgradeCheckOnlyIncludePrereleases(t *testing.T) {
	tmp := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v0.5.2","assets":[{"name":"velocity-report-linux-arm64","browser_download_url":"https://example.com/stable"}]}`))
		case "/releases":
			_, _ = w.Write([]byte(`[
				{"tag_name":"v0.6.0-rc1","prerelease":true,"draft":false,"assets":[{"name":"velocity-report-linux-arm64","browser_download_url":"https://example.com/rc"}]},
				{"tag_name":"v0.5.2","prerelease":false,"draft":false,"assets":[{"name":"velocity-report-linux-arm64","browser_download_url":"https://example.com/stable"}]}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := ctl.Config{
		ReleasesAPI:     server.URL + "/latest",
		ReleasesListAPI: server.URL + "/releases",
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

	if err := runUpgrade([]string{"--check", "--include-prereleases"}); err != nil {
		t.Fatalf("runUpgrade failed: %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Latest:  v0.6.0-rc1")) {
		t.Fatalf("expected prerelease latest in output, got: %s", out.String())
	}
}
