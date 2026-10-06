package ctl

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPickAsset(t *testing.T) {
	ch := releasesChannel{
		LinuxArm64: releaseAsset{Version: "0.5.2", URL: "https://example.invalid/linux"},
		MacArm64:   releaseAsset{Version: "0.5.2", URL: "https://example.invalid/mac"},
	}

	tests := []struct {
		name    string
		goos    string
		goarch  string
		wantURL string
		wantErr bool
	}{
		{name: "pi", goos: "linux", goarch: "arm64", wantURL: "https://example.invalid/linux"},
		{name: "apple silicon", goos: "darwin", goarch: "arm64", wantURL: "https://example.invalid/mac"},
		// The device manager only upgrades the server binary, and it ships
		// for arm64 only; anything else must be refused rather than silently
		// installing the wrong artefact.
		{name: "linux amd64", goos: "linux", goarch: "amd64", wantErr: true},
		{name: "intel mac", goos: "darwin", goarch: "amd64", wantErr: true},
		{name: "windows", goos: "windows", goarch: "arm64", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickAsset(ch, tc.goos, tc.goarch)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("pickAsset(%s/%s) succeeded, want an error", tc.goos, tc.goarch)
				}
				if !strings.Contains(err.Error(), "unsupported platform") {
					t.Errorf("error = %v, want it to name the unsupported platform", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("pickAsset(%s/%s): %v", tc.goos, tc.goarch, err)
			}
			if got.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tc.wantURL)
			}
		})
	}
}

func TestReleaseAssetIsNewer(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		want      bool
	}{
		// No current version means anything is an upgrade — this is the
		// first-install case.
		{"empty current accepts anything", "0.0.1", "", true},
		{"higher patch", "0.5.2", "0.5.1", true},
		{"higher minor", "0.6.0", "0.5.9", true},
		{"higher major", "1.0.0", "0.9.9", true},
		{"equal is not newer", "0.5.1", "0.5.1", false},
		{"lower is not newer", "0.5.0", "0.5.1", false},
		// Double-digit components must compare numerically, not as strings,
		// or 0.5.10 would look older than 0.5.9.
		{"double-digit patch", "0.5.10", "0.5.9", true},
		{"double-digit minor", "0.10.0", "0.9.0", true},
		// A parseable version beats an unparseable one regardless of order.
		{"parseable beats garbage", "0.5.1", "not-a-version", true},
		{"garbage loses to parseable", "not-a-version", "0.5.1", false},
		// With neither parseable there is nothing better than a string compare.
		{"both unparseable falls back to string order", "beta", "alpha", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := releaseAssetIsNewer(
				releaseAsset{Version: tc.candidate},
				releaseAsset{Version: tc.current},
			)
			if got != tc.want {
				t.Errorf("releaseAssetIsNewer(%q, %q) = %v, want %v",
					tc.candidate, tc.current, got, tc.want)
			}
		})
	}
}

func TestNewManagerFillsEveryDefault(t *testing.T) {
	// A zero Config is what the CLI hands over when no flags are set, so
	// every field has to acquire its production default here — a missed one
	// means the device manager acts on an empty path or a zero timeout.
	var out bytes.Buffer
	m := NewManager(Config{}, nil, &fakeRunner{}, &out, &out)

	if got := m.cfg.ReleaseMetaURL; got != defaultReleaseMetaURL {
		t.Errorf("ReleaseMetaURL = %q, want %q", got, defaultReleaseMetaURL)
	}
	if got := m.cfg.InstallRoot; got != defaultInstallRoot {
		t.Errorf("InstallRoot = %q, want %q", got, defaultInstallRoot)
	}
	if got := m.cfg.BinaryName; got != defaultBinaryName {
		t.Errorf("BinaryName = %q, want %q", got, defaultBinaryName)
	}
	if got := m.cfg.ServiceName; got != defaultServiceName {
		t.Errorf("ServiceName = %q, want %q", got, defaultServiceName)
	}
	if got := m.cfg.BackupDir; got != defaultBackupDir {
		t.Errorf("BackupDir = %q, want %q", got, defaultBackupDir)
	}
	if got := m.cfg.DBPath; got != defaultDBPath {
		t.Errorf("DBPath = %q, want %q", got, defaultDBPath)
	}
	if got := m.cfg.VersionURL; got != defaultVersionURL {
		t.Errorf("VersionURL = %q, want %q", got, defaultVersionURL)
	}
	if got := m.cfg.Retain; got != defaultRetain {
		t.Errorf("Retain = %d, want %d", got, defaultRetain)
	}
	// Zero timeouts would mean "no timeout" and could hang the device.
	if m.cfg.RequestTimeout <= 0 {
		t.Errorf("RequestTimeout = %v, want a positive default", m.cfg.RequestTimeout)
	}
	if m.cfg.DownloadTimeout <= 0 {
		t.Errorf("DownloadTimeout = %v, want a positive default", m.cfg.DownloadTimeout)
	}
	if m.cfg.VerifyDelay <= 0 {
		t.Errorf("VerifyDelay = %v, want a positive default", m.cfg.VerifyDelay)
	}
	if m.cfg.CurrentVersion == "" {
		t.Error("CurrentVersion is empty, want the compiled-in version")
	}
	if m.cfg.GOOS == "" || m.cfg.GOARCH == "" {
		t.Errorf("platform = %s/%s, want the runtime values", m.cfg.GOOS, m.cfg.GOARCH)
	}
}

func TestNewManagerPreservesSuppliedConfig(t *testing.T) {
	// Anything explicitly set must survive: the defaults only fill gaps.
	var out bytes.Buffer
	cfg := Config{
		ReleaseMetaURL: "https://example.invalid/release.json",
		InstallRoot:    "/custom/root",
		BinaryName:     "custom-binary",
		ServiceName:    "custom.service",
		BackupDir:      "/custom/backups",
		DBPath:         "/custom/db.sqlite",
		VersionURL:     "http://127.0.0.1:9999/api/version",
		Retain:         7,
		CurrentVersion: "9.9.9",
		GOOS:           "linux",
		GOARCH:         "arm64",
	}

	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)

	if m.cfg.ReleaseMetaURL != cfg.ReleaseMetaURL ||
		m.cfg.InstallRoot != cfg.InstallRoot ||
		m.cfg.BinaryName != cfg.BinaryName ||
		m.cfg.ServiceName != cfg.ServiceName ||
		m.cfg.BackupDir != cfg.BackupDir ||
		m.cfg.DBPath != cfg.DBPath ||
		m.cfg.VersionURL != cfg.VersionURL ||
		m.cfg.Retain != cfg.Retain ||
		m.cfg.CurrentVersion != cfg.CurrentVersion ||
		m.cfg.GOOS != cfg.GOOS ||
		m.cfg.GOARCH != cfg.GOARCH {
		t.Errorf("NewManager overwrote supplied config:\n got %+v\nwant %+v", m.cfg, cfg)
	}
}

func TestRunCheckIsReadOnly(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stableJSON("0.5.2")))
	}))
	defer server.Close()
	cfg.ReleaseMetaURL = server.URL

	var out bytes.Buffer
	runner := &fakeRunner{}
	m := NewManager(cfg, nil, runner, &out, &out)

	if err := m.RunCheck(UpgradeOptions{}); err != nil {
		t.Fatalf("RunCheck: %v", err)
	}

	if !strings.Contains(out.String(), "Latest:") {
		t.Errorf("output missing the latest version\n---\n%s", out.String())
	}
	// "Read-only" is the whole point: no service restart, no install.
	if len(runner.calls) != 0 {
		t.Errorf("RunCheck ran %d commands, want none: %v", len(runner.calls), runner.calls)
	}
}

func TestRunCheckReportsMetadataFailure(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	cfg.ReleaseMetaURL = server.URL

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)

	if err := m.RunCheck(UpgradeOptions{}); err == nil {
		t.Fatal("RunCheck against a failing metadata endpoint succeeded, want an error")
	}
}
