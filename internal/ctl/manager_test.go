package ctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	calls []string
	fails map[string]error
}

type errorGetter struct {
	err error
}

func (g errorGetter) Get(_ string) (*http.Response, error) {
	return nil, g.err
}

func (f *fakeRunner) Run(name string, args ...string) error {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	for prefix, err := range f.fails {
		if strings.HasPrefix(call, prefix) {
			return err
		}
	}
	return nil
}

func testConfig(tmp string) Config {
	return Config{
		ReleasesAPI:     "",
		BinaryName:      "velocity-report",
		BinaryPath:      filepath.Join(tmp, "bin", "velocity-report"),
		ServiceName:     "velocity-report.service",
		BackupDir:       filepath.Join(tmp, "backups"),
		DBPath:          filepath.Join(tmp, "sensor_data.db"),
		RequestTimeout:  2 * time.Second,
		DownloadTimeout: 2 * time.Second,
		VerifyDelay:     0,
		CurrentVersion:  "0.5.1",
		GOOS:            "linux",
		GOARCH:          "arm64",
	}
}

func TestRunBackupCreatesSnapshot(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	if err := os.MkdirAll(filepath.Dir(cfg.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BinaryPath, []byte("binary-data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.DBPath, []byte("db-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	m.now = func() time.Time { return time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC) }

	backupPath, err := m.RunBackup(cfg.BackupDir)
	if err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}

	want := filepath.Join(cfg.BackupDir, "20260329-120000")
	if backupPath != want {
		t.Fatalf("backup path mismatch: got %s, want %s", backupPath, want)
	}

	if _, err := os.Stat(filepath.Join(want, cfg.BinaryName)); err != nil {
		t.Fatalf("missing backup binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(want, "sensor_data.db")); err != nil {
		t.Fatalf("missing backup database: %v", err)
	}
}

func TestDefaultConfigAndManager(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ServiceName == "" || cfg.BinaryPath == "" {
		t.Fatal("default config should set paths and service name")
	}

	m := NewDefaultManager()
	if m.ServiceName() == "" {
		t.Fatal("default manager should have service name")
	}
}

func TestExecRunnerMissingCommand(t *testing.T) {
	r := ExecRunner{Stdout: io.Discard, Stderr: io.Discard}
	err := r.Run("this-command-does-not-exist-ctl-test")
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestRunRollbackNoBackups(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	if err := os.MkdirAll(cfg.BackupDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	err := m.RunRollback()
	if err == nil || !strings.Contains(err.Error(), "no backups found") {
		t.Fatalf("expected no-backups error, got: %v", err)
	}
}

func TestRunRollbackUsesLatestBackup(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	runner := &fakeRunner{}

	if err := os.MkdirAll(filepath.Dir(cfg.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BinaryPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	older := filepath.Join(cfg.BackupDir, "20260328-120000")
	latest := filepath.Join(cfg.BackupDir, "20260329-120000")
	for _, d := range []string{older, latest} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, cfg.BinaryName), []byte("newer"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	if err := m.RunRollback(); err != nil {
		t.Fatalf("RunRollback failed: %v", err)
	}

	installed, err := os.ReadFile(cfg.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "newer" {
		t.Fatalf("binary not restored from latest backup: %q", string(installed))
	}
}

func TestRunUpgradeCheckOnly(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.2","assets":[{"name":"velocity-report-0.5.2-linux-arm64","browser_download_url":"https://example.com/bin"}]}`))
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	if err := m.RunUpgrade(true, ""); err != nil {
		t.Fatalf("RunUpgrade check-only failed: %v", err)
	}

	if !strings.Contains(out.String(), "Latest:") {
		t.Fatalf("expected latest version output, got: %s", out.String())
	}
}

func TestRunUpgradePreventDowngrade(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	// Current is 0.5.1-pre3, stable latest is 0.5.0 → should refuse.
	cfg.CurrentVersion = "0.5.1-pre3"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.0","assets":[{"name":"velocity-report-0.5.0-linux-arm64","browser_download_url":"https://example.com/bin"}]}`))
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)

	if err := m.RunUpgrade(false, ""); err != nil {
		t.Fatalf("RunUpgrade should not return error on downgrade prevention: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "newer than the latest stable") {
		t.Fatalf("expected downgrade prevention message, got: %s", output)
	}
	if !strings.Contains(output, "--prerelease") {
		t.Fatalf("expected --prerelease suggestion for prerelease user, got: %s", output)
	}
}

func TestRunUpgradePrereleaseSuggestsFlag(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	// Current is 0.5.1 (stable), latest stable is 0.5.0 → downgrade blocked, no --prerelease hint.
	cfg.CurrentVersion = "0.5.1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.0","assets":[]}`))
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)

	if err := m.RunUpgrade(false, ""); err != nil {
		t.Fatalf("RunUpgrade should not error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "newer than the latest stable") {
		t.Fatalf("expected downgrade prevention message, got: %s", output)
	}
	// Stable user should NOT get the --prerelease suggestion.
	if strings.Contains(output, "--prerelease") {
		t.Fatalf("stable user should not see --prerelease suggestion, got: %s", output)
	}
}

func TestRunUpgradeAllowsLegitimateUpgrade(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	// Current is 0.5.0, latest is 0.5.2 → should proceed to check-only output.
	cfg.CurrentVersion = "0.5.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.2","assets":[{"name":"velocity-report-0.5.2-linux-arm64","browser_download_url":"https://example.com/bin"}]}`))
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)

	if err := m.RunUpgradeWithOptions(true, "", UpgradeOptions{}); err != nil {
		t.Fatalf("RunUpgrade check-only failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Latest:  v0.5.2") {
		t.Fatalf("expected upgrade available output, got: %s", output)
	}
}

func TestRunUpgradeLocalBinaryHappyPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	runner := &fakeRunner{}

	if err := os.MkdirAll(filepath.Dir(cfg.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BinaryPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	newBinary := filepath.Join(tmp, "new-binary")
	if err := os.WriteFile(newBinary, []byte("new-binary-data"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	m.sleep = func(time.Duration) {}

	if err := m.RunUpgrade(false, newBinary); err != nil {
		t.Fatalf("RunUpgrade failed: %v", err)
	}

	installed, err := os.ReadFile(cfg.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "new-binary-data" {
		t.Fatalf("unexpected installed binary: %q", string(installed))
	}

	if len(runner.calls) == 0 {
		t.Fatal("expected runner calls")
	}
}

func TestRunUpgradeNoMatchingAsset(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.2","assets":[{"name":"other","browser_download_url":"https://example.com/bin"}]}`))
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	err := m.RunUpgrade(false, "")
	if err == nil || !strings.Contains(err.Error(), "no binary asset found") {
		t.Fatalf("expected no-asset error, got: %v", err)
	}
}

func TestFetchLatestReleaseBadJSON(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":`))
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	_, err := m.fetchLatestRelease(false)
	if err == nil || !strings.Contains(err.Error(), "parsing release JSON") {
		t.Fatalf("expected JSON parse error, got: %v", err)
	}
}

func TestFetchLatestReleaseHTTPError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	_, err := m.fetchLatestRelease(false)
	if err == nil || !strings.Contains(err.Error(), "GitHub API returned") {
		t.Fatalf("expected HTTP status error, got: %v", err)
	}
}

func TestFetchLatestReleaseGetError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	m := NewManager(cfg, errorGetter{err: errors.New("network down")}, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := m.fetchLatestRelease(false)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected getter error, got: %v", err)
	}
}

func TestFindAssetURLVersionedName(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	url, err := m.findAssetURL(&githubRelease{
		TagName: "v0.5.2",
		Assets: []githubAsset{
			{Name: "velocity-report-0.5.2-linux-arm64", BrowserDownloadURL: "https://example.com/versioned"},
		},
	})
	if err != nil {
		t.Fatalf("findAssetURL failed: %v", err)
	}
	if url != "https://example.com/versioned" {
		t.Fatalf("unexpected asset URL: %s", url)
	}
}

func TestFindAssetURLNoMatchForUnversioned(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := m.findAssetURL(&githubRelease{
		TagName: "v0.5.2",
		Assets: []githubAsset{
			{Name: "velocity-report-linux-arm64", BrowserDownloadURL: "https://example.com/old"},
		},
	})
	if err == nil {
		t.Fatal("expected error for unversioned asset name")
	}
}

func TestApplyLocalBinaryDirectoryError(t *testing.T) {
	tmp := t.TempDir()
	m := NewManager(testConfig(tmp), nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	err := m.applyLocalBinary(tmp)
	if err == nil || !strings.Contains(err.Error(), "expected a file") {
		t.Fatalf("expected directory error, got: %v", err)
	}
}

func TestInstallBinaryFailsForMissingSource(t *testing.T) {
	tmp := t.TempDir()
	m := NewManager(testConfig(tmp), nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	err := m.installBinary(filepath.Join(tmp, "missing"), filepath.Join(tmp, "dst"))
	if err == nil {
		t.Fatal("expected installBinary error")
	}
}

func TestCopyFilePreservesContentsAndMode(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.WriteFile(src, []byte("abc"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abc" {
		t.Fatalf("unexpected copied data: %q", string(data))
	}
}

func TestDownloadToTempSuccess(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	path, err := m.downloadToTemp(server.URL)
	if err != nil {
		t.Fatalf("downloadToTemp failed: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected downloaded data: %q", string(data))
	}
}

func TestRestoreBackupMissingBinary(t *testing.T) {
	tmp := t.TempDir()
	m := NewManager(testConfig(tmp), nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	err := m.restoreBackup(filepath.Join(tmp, "backup"))
	if err == nil || !strings.Contains(err.Error(), "backup binary not found") {
		t.Fatalf("expected missing backup binary error, got: %v", err)
	}
}

func TestApplyUpgradeInstallFailureTriggersRestart(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	runner := &fakeRunner{}

	if err := os.MkdirAll(filepath.Dir(cfg.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BinaryPath, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}

	badInput := filepath.Join(tmp, "missing-new-binary")
	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	err := m.applyUpgrade(badInput)
	if err == nil || !strings.Contains(err.Error(), "installing binary") {
		t.Fatalf("expected install failure, got: %v", err)
	}

	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "systemctl start "+cfg.ServiceName) {
		t.Fatalf("expected restart attempt after failed install, calls: %s", joined)
	}
}

func TestFetchLatestReleaseDecodeRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	want := githubRelease{
		TagName: "v0.5.2",
		Assets:  []githubAsset{{Name: "velocity-report-0.5.2-linux-arm64", BrowserDownloadURL: "https://example.com/bin"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer server.Close()

	cfg.ReleasesAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	got, err := m.fetchLatestRelease(false)
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	if got.TagName != want.TagName || len(got.Assets) != 1 {
		t.Fatalf("unexpected release payload: %+v", got)
	}
}

func TestFetchLatestReleaseIncludingPrereleasesUsesListEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/releases":
			_, _ = w.Write([]byte(`[
				{"tag_name":"v0.6.0-rc1","prerelease":true,"draft":false,"assets":[{"name":"velocity-report-0.6.0-rc1-linux-arm64","browser_download_url":"https://example.com/rc"}]},
				{"tag_name":"v0.5.2","prerelease":false,"draft":false,"assets":[{"name":"velocity-report-0.5.2-linux-arm64","browser_download_url":"https://example.com/stable"}]}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg.ReleasesListAPI = server.URL + "/releases"
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	got, err := m.fetchLatestRelease(true)
	if err != nil {
		t.Fatalf("fetchLatestRelease(true) failed: %v", err)
	}
	if got.TagName != "v0.6.0-rc1" {
		t.Fatalf("expected prerelease from list endpoint, got: %+v", got)
	}
}

func TestFetchLatestReleaseIncludingPrereleasesSkipsDrafts(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name":"v0.6.0-rc1","prerelease":true,"draft":true,"assets":[]},
			{"tag_name":"v0.5.2","prerelease":false,"draft":false,"assets":[{"name":"velocity-report-0.5.2-linux-arm64","browser_download_url":"https://example.com/stable"}]}
		]`))
	}))
	defer server.Close()

	cfg.ReleasesListAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	got, err := m.fetchLatestRelease(true)
	if err != nil {
		t.Fatalf("fetchLatestRelease(true) failed: %v", err)
	}
	if got.TagName != "v0.5.2" {
		t.Fatalf("expected first non-draft release, got: %+v", got)
	}
}

func TestFetchLatestReleaseIncludingPrereleasesNoNonDraft(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name":"v0.6.0-rc1","prerelease":true,"draft":true,"assets":[]}
		]`))
	}))
	defer server.Close()

	cfg.ReleasesListAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := m.fetchLatestRelease(true)
	if err == nil || !strings.Contains(err.Error(), "no non-draft releases found") {
		t.Fatalf("expected no-non-draft error, got: %v", err)
	}
}

func TestFetchLatestReleaseIncludingPrereleasesBadJSON(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[`))
	}))
	defer server.Close()

	cfg.ReleasesListAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := m.fetchLatestRelease(true)
	if err == nil || !strings.Contains(err.Error(), "parsing release JSON") {
		t.Fatalf("expected JSON parse error, got: %v", err)
	}
}

func TestFetchLatestReleaseIncludingPrereleasesHTTPError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg.ReleasesListAPI = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := m.fetchLatestRelease(true)
	if err == nil || !strings.Contains(err.Error(), "GitHub API returned") {
		t.Fatalf("expected HTTP status error, got: %v", err)
	}
}

func TestFetchLatestReleaseIncludingPrereleasesGetError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	m := NewManager(cfg, errorGetter{err: errors.New("network down")}, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := m.fetchLatestRelease(true)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected getter error, got: %v", err)
	}
}

func TestRunStatusHandlesExitError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	var errOut bytes.Buffer
	runner := &fakeRunner{fails: map[string]error{"systemctl status": &execExitError{code: 3}}}
	m := NewManager(cfg, nil, runner, &bytes.Buffer{}, &errOut)

	if err := m.RunStatus(); err != nil {
		t.Fatalf("RunStatus should swallow exit error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Service is not running") {
		t.Fatalf("expected service not running message, got: %s", errOut.String())
	}
}

func TestRunStatusReturnsRunnerError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	runner := &fakeRunner{fails: map[string]error{"systemctl status": errors.New("boom")}}
	m := NewManager(cfg, nil, runner, &bytes.Buffer{}, &bytes.Buffer{})
	err := m.RunStatus()
	if err == nil || !strings.Contains(err.Error(), "running systemctl") {
		t.Fatalf("expected wrapped runner error, got: %v", err)
	}
}

func TestDownloadToTempHTTPError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	_, err := m.downloadToTemp(server.URL)
	if err == nil || !strings.Contains(err.Error(), "download returned") {
		t.Fatalf("expected HTTP error, got: %v", err)
	}
}

// execExitError mimics exec.ExitError enough for errors.As checks in tests.
type execExitError struct {
	code int
}

func (e *execExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func (e *execExitError) ExitCode() int {
	return e.code
}
