package main

import (
	"bytes"
	"errors"
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
		_, _ = w.Write([]byte(`{"stable":{"version":"0.5.2","linux_arm64":{"url":"https://example.com/bin","sha256":""}}}`))
	}))
	defer server.Close()

	cfg := ctl.Config{
		ReleasesMetaURL: server.URL,
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

func TestLoadIncludePrereleasesMissingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "missing.json")

	include, err := loadIncludePrereleases(path)
	if err != nil {
		t.Fatalf("loadIncludePrereleases failed: %v", err)
	}
	if include {
		t.Fatal("expected include_prereleases=false when config file is missing")
	}
}

func TestLoadIncludePrereleasesReadError(t *testing.T) {
	tmp := t.TempDir()

	_, err := loadIncludePrereleases(tmp)
	if err == nil {
		t.Fatal("expected read error when config path points to a directory")
	}
}

func TestLoadIncludePrereleasesInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "velocity-ctl.json")
	if err := os.WriteFile(path, []byte(`{"include_prereleases":`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadIncludePrereleases(path)
	if err == nil {
		t.Fatal("expected parse error for invalid config JSON")
	}
}

func TestLoadIncludePrereleasesUsesDefaultHomePath(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".velocity-ctl.json")
	if err := os.WriteFile(path, []byte(`{"include_prereleases": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	old := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = old }()

	include, err := loadIncludePrereleases("")
	if err != nil {
		t.Fatalf("loadIncludePrereleases failed: %v", err)
	}
	if !include {
		t.Fatal("expected include_prereleases=true from default home config")
	}
}

func TestLoadIncludePrereleasesHomeDirError(t *testing.T) {
	old := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("boom") }
	defer func() { userHomeDir = old }()

	include, err := loadIncludePrereleases("")
	if err != nil {
		t.Fatalf("expected graceful fallback on home-dir error, got: %v", err)
	}
	if include {
		t.Fatal("expected include_prereleases=false when home lookup fails")
	}
}

func TestRunUpgradeCheckOnlyIncludePrereleases(t *testing.T) {
	tmp := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stable":{"version":"0.5.2","linux_arm64":{"url":"https://example.com/stable","sha256":""}},"prerelease":{"version":"0.6.0-rc1","linux_arm64":{"url":"https://example.com/rc","sha256":""}}}`))
	}))
	defer server.Close()

	cfg := ctl.Config{
		ReleasesMetaURL: server.URL,
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

func TestRunUpgradeCheckOnlyIncludePrereleasesFromConfig(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "velocity-ctl.json")
	if err := os.WriteFile(configPath, []byte(`{"include_prereleases": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stable":{"version":"0.5.2","linux_arm64":{"url":"https://example.com/stable","sha256":""}},"prerelease":{"version":"0.6.0-rc1","linux_arm64":{"url":"https://example.com/rc","sha256":""}}}`))
	}))
	defer server.Close()

	cfg := ctl.Config{
		ReleasesMetaURL: server.URL,
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

	if err := runUpgrade([]string{"--check", "--config", configPath}); err != nil {
		t.Fatalf("runUpgrade failed: %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Latest:  v0.6.0-rc1")) {
		t.Fatalf("expected prerelease latest from config opt-in, got: %s", out.String())
	}
}

func TestRunUpgradeReturnsConfigParseError(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "velocity-ctl.json")
	if err := os.WriteFile(configPath, []byte(`{"include_prereleases":`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runUpgrade([]string{"--check", "--config", configPath})
	if err == nil {
		t.Fatal("expected config parse error")
	}
}

func TestRunUpgradeReturnsFlagParseError(t *testing.T) {
	err := runUpgrade([]string{"--not-a-real-flag"})
	if err == nil {
		t.Fatal("expected flag parse error")
	}
}
