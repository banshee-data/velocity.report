package ctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

// stableJSON returns a minimal release.json payload for tests.
// The linux_arm64 asset is populated so pickAsset returns a URL.
func stableJSON(version string) string {
	return `{"stable":` + channelJSON(version) + `}`
}

// channelsJSON returns a release.json with both channels populated.
func channelsJSON(stable, prerelease string) string {
	return `{"stable":` + channelJSON(stable) + `,"prerelease":` + channelJSON(prerelease) + `}`
}

// channelJSON returns one channel object with linux_arm64 populated
// using the given asset version (empty version means the asset is
// unpublished — useful for testing the prerelease fallback).
func channelJSON(version string) string {
	if version == "" {
		return `{"linux_arm64":{"version":"","url":"","sha256":""}}`
	}
	url := "https://example.com/v" + version + "/velocity-" + version + "-linux-arm64"
	return `{"linux_arm64":{"version":"` + version + `","url":"` + url + `","sha256":""}}`
}

func testConfig(tmp string) Config {
	return Config{
		ReleaseMetaURL:  "",
		InstallRoot:     filepath.Join(tmp, "opt"),
		BinaryName:      "velocity",
		ServiceName:     "velocity-report.service",
		BackupDir:       filepath.Join(tmp, "backups"),
		DBPath:          filepath.Join(tmp, "sensor_data.db"),
		Retain:          3,
		RequestTimeout:  2 * time.Second,
		DownloadTimeout: 2 * time.Second,
		VerifyDelay:     0,
		VerifyTimeout:   0, // skip the /api/version poll in tests
		CurrentVersion:  "0.5.1",
		GOOS:            "linux",
		GOARCH:          "arm64",
	}
}

// seedVersion stages a binary at versions/<v>/<BinaryName> with the given
// content. It does not touch the current/previous symlinks.
func seedVersion(t *testing.T, cfg Config, v, content string) string {
	t.Helper()
	vbin := filepath.Join(cfg.InstallRoot, versionsSubdir, v, cfg.BinaryName)
	if err := os.MkdirAll(filepath.Dir(vbin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vbin, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return vbin
}

// linkVersion points name (current/previous) at versions/<v>.
func linkVersion(t *testing.T, cfg Config, name, v string) {
	t.Helper()
	link := filepath.Join(cfg.InstallRoot, name)
	_ = os.Remove(link)
	if err := os.Symlink(filepath.Join(versionsSubdir, v), link); err != nil {
		t.Fatal(err)
	}
}

// currentVersion reads which version current resolves to.
func currentVersion(t *testing.T, cfg Config) string {
	t.Helper()
	target, err := os.Readlink(filepath.Join(cfg.InstallRoot, currentName))
	if err != nil {
		t.Fatalf("reading current symlink: %v", err)
	}
	return filepath.Base(target)
}

func TestRunBackupCreatesSnapshot(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	seedVersion(t, cfg, "0.5.1", "binary-data")
	linkVersion(t, cfg, currentName, "0.5.1")
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
	if cfg.ServiceName == "" || cfg.InstallRoot == "" {
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

func TestRunRollbackNoPrevious(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	if err := os.MkdirAll(cfg.InstallRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	err := m.RunRollback()
	if err == nil || !strings.Contains(err.Error(), "no previous version") {
		t.Fatalf("expected no-previous error, got: %v", err)
	}
}

func TestRunRollbackSwapsToPrevious(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	runner := &fakeRunner{}

	// current -> 0.5.2 (active), previous -> 0.5.1 (rollback target).
	seedVersion(t, cfg, "0.5.1", "old")
	seedVersion(t, cfg, "0.5.2", "new")
	linkVersion(t, cfg, currentName, "0.5.2")
	linkVersion(t, cfg, previousName, "0.5.1")

	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	if err := m.RunRollback(); err != nil {
		t.Fatalf("RunRollback failed: %v", err)
	}

	if got := currentVersion(t, cfg); got != "0.5.1" {
		t.Fatalf("current should be 0.5.1 after rollback, got %s", got)
	}
	// previous now points at what current used to be.
	prev, err := os.Readlink(filepath.Join(cfg.InstallRoot, previousName))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(prev) != "0.5.2" {
		t.Fatalf("previous should be 0.5.2 after rollback, got %s", filepath.Base(prev))
	}
	if !strings.Contains(strings.Join(runner.calls, "\n"), "systemctl restart "+cfg.ServiceName) {
		t.Fatalf("expected service restart, calls: %#v", runner.calls)
	}
}

func TestRunUpgradeCheckOnly(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stableJSON("0.5.2")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
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
		_, _ = w.Write([]byte(stableJSON("0.5.0")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
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
		_, _ = w.Write([]byte(stableJSON("0.5.0")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
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
		_, _ = w.Write([]byte(stableJSON("0.5.2")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
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

	// Existing active version so the swap records a previous.
	seedVersion(t, cfg, "0.5.1", "old-binary")
	linkVersion(t, cfg, currentName, "0.5.1")

	newBinary := filepath.Join(tmp, "new-binary")
	if err := os.WriteFile(newBinary, []byte("new-binary-data"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	m.sleep = func(time.Duration) {}
	m.now = func() time.Time { return time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC) }

	if err := m.RunUpgrade(false, newBinary); err != nil {
		t.Fatalf("RunUpgrade failed: %v", err)
	}

	wantVersion := "local-20260329-120000"
	if got := currentVersion(t, cfg); got != wantVersion {
		t.Fatalf("current should be %s, got %s", wantVersion, got)
	}

	newBin := filepath.Join(cfg.InstallRoot, versionsSubdir, wantVersion, cfg.BinaryName)
	installed, err := os.ReadFile(newBin)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "new-binary-data" {
		t.Fatalf("unexpected installed binary: %q", string(installed))
	}

	prev, err := os.Readlink(filepath.Join(cfg.InstallRoot, previousName))
	if err != nil {
		t.Fatalf("previous symlink: %v", err)
	}
	if filepath.Base(prev) != "0.5.1" {
		t.Fatalf("previous should be 0.5.1, got %s", filepath.Base(prev))
	}

	joined := strings.Join(runner.calls, "\n")
	wantMigrationCall := newBin + " data migrate up --db-path " + cfg.DBPath
	if !strings.Contains(joined, wantMigrationCall) {
		t.Fatalf("expected migration call %q, got calls: %#v", wantMigrationCall, runner.calls)
	}
	if !strings.Contains(joined, "systemctl restart "+cfg.ServiceName) {
		t.Fatalf("expected service restart, got calls: %#v", runner.calls)
	}
}

func TestRunUpgradeNoVersionInJSON(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stable":{"version":""}}`))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	err := m.RunUpgrade(false, "")
	if err == nil || !strings.Contains(err.Error(), "no version found") {
		t.Fatalf("expected no-version error, got: %v", err)
	}
}

func TestFetchLatestReleaseBadJSON(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stable":`))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	_, _, _, err := m.fetchLatestRelease(false)
	if err == nil || !strings.Contains(err.Error(), "parsing release.json") {
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

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	_, _, _, err := m.fetchLatestRelease(false)
	if err == nil || !strings.Contains(err.Error(), "releases metadata returned") {
		t.Fatalf("expected HTTP status error, got: %v", err)
	}
}

func TestFetchLatestReleaseGetError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	m := NewManager(cfg, errorGetter{err: errors.New("network down")}, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, _, _, err := m.fetchLatestRelease(false)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected getter error, got: %v", err)
	}
}

func TestFetchLatestReleaseURLConstruction(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp) // GOOS=linux, GOARCH=arm64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stableJSON("0.5.2")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	ver, url, _, err := m.fetchLatestRelease(false)
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	if ver != "0.5.2" {
		t.Fatalf("unexpected version: %s", ver)
	}
	wantSuffix := "/v0.5.2/velocity-0.5.2-linux-arm64"
	if !strings.HasSuffix(url, wantSuffix) {
		t.Fatalf("unexpected download URL: %s", url)
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

// sha256Hex returns the hex-encoded SHA-256 digest of s.
func sha256Hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func TestDownloadToTempSuccess(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	const payload = "payload"
	const binaryName = "velocity-report-binary"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	binaryURL := server.URL + "/v0.5.2/" + binaryName
	tmpPath, err := m.downloadToTemp(binaryURL, sha256Hex(payload))
	if err != nil {
		t.Fatalf("downloadToTemp failed: %v", err)
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != payload {
		t.Fatalf("unexpected downloaded data: %q", string(data))
	}
	if !strings.Contains(out.String(), "SHA-256 verified") {
		t.Fatalf("expected verification confirmation in output, got: %s", out.String())
	}
}

func TestDownloadToTempChecksumMismatch(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	const binaryName = "velocity-report-binary"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	binaryURL := server.URL + "/v0.5.2/" + binaryName
	wrongSHA := strings.Repeat("a", 64)
	_, err := m.downloadToTemp(binaryURL, wrongSHA)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected mismatch in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), wrongSHA) {
		t.Fatalf("expected expected hash in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), sha256Hex("payload")) {
		t.Fatalf("expected computed hash in error, got: %v", err)
	}
}

func TestDownloadToTempEmptySHA(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	const binaryName = "velocity-report-binary"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &stdout, &stderr)
	binaryURL := server.URL + "/v0.5.2/" + binaryName
	tmpPath, err := m.downloadToTemp(binaryURL, "")
	if err != nil {
		t.Fatalf("downloadToTemp should proceed when expectedSHA is empty, got: %v", err)
	}
	defer os.Remove(tmpPath)

	if !strings.Contains(stderr.String(), "no expected SHA-256") {
		t.Fatalf("expected warning about missing SHA, got: %s", stderr.String())
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected downloaded data: %q", string(data))
	}
}

func TestApplyVersionedUpgradeInstallFailure(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	runner := &fakeRunner{}

	// A non-existent source binary makes the install step fail before any
	// activation or service action.
	badInput := filepath.Join(tmp, "missing-new-binary")
	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	err := m.applyVersionedUpgrade("0.5.2", badInput)
	if err == nil || !strings.Contains(err.Error(), "installing binary") {
		t.Fatalf("expected install failure, got: %v", err)
	}

	if _, statErr := os.Lstat(filepath.Join(cfg.InstallRoot, currentName)); statErr == nil {
		t.Fatal("current symlink must not exist after a failed install")
	}
	if joined := strings.Join(runner.calls, "\n"); strings.Contains(joined, "systemctl") {
		t.Fatalf("expected no systemctl calls on failed install, got: %#v", runner.calls)
	}
}

func TestPruneRetainsNewestAndProtected(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	cfg.Retain = 3
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	for _, v := range []string{"0.5.0", "0.5.1", "0.5.2", "0.5.3", "0.5.4"} {
		seedVersion(t, cfg, v, v)
	}
	// current=0.5.4, previous=0.5.3 (both protected). Retain=3 keeps one more
	// newest non-protected (0.5.2); 0.5.1 and 0.5.0 are pruned.
	linkVersion(t, cfg, currentName, "0.5.4")
	linkVersion(t, cfg, previousName, "0.5.3")

	if err := m.prune(); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	got := map[string]bool{}
	entries, _ := os.ReadDir(filepath.Join(cfg.InstallRoot, versionsSubdir))
	for _, e := range entries {
		got[e.Name()] = true
	}
	for _, keep := range []string{"0.5.4", "0.5.3", "0.5.2"} {
		if !got[keep] {
			t.Errorf("expected %s retained", keep)
		}
	}
	for _, gone := range []string{"0.5.1", "0.5.0"} {
		if got[gone] {
			t.Errorf("expected %s pruned", gone)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected exactly 3 versions retained, got %d: %v", len(got), got)
	}
}

func TestFetchLatestReleaseStableChannel(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(channelsJSON("0.5.2", "0.6.0-pre1")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	ver, _, _, err := m.fetchLatestRelease(false)
	if err != nil {
		t.Fatalf("fetchLatestRelease(false) failed: %v", err)
	}
	if ver != "0.5.2" {
		t.Fatalf("expected stable version 0.5.2, got: %s", ver)
	}
}

func TestFetchLatestReleasePrereleaseChannel(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(channelsJSON("0.5.2", "0.6.0-pre1")))
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	ver, _, _, err := m.fetchLatestRelease(true)
	if err != nil {
		t.Fatalf("fetchLatestRelease(true) failed: %v", err)
	}
	if ver != "0.6.0-pre1" {
		t.Fatalf("expected prerelease version 0.6.0-pre1, got: %s", ver)
	}
}

func TestFetchLatestReleasePrereleaseEmptyFallsBackToStable(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stableJSON("0.5.2"))) // no prerelease channel
	}))
	defer server.Close()

	cfg.ReleaseMetaURL = server.URL
	m := NewManager(cfg, nil, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})
	ver, _, _, err := m.fetchLatestRelease(true)
	if err != nil {
		t.Fatalf("fetchLatestRelease(true) with empty prerelease failed: %v", err)
	}
	if ver != "0.5.2" {
		t.Fatalf("expected fallback to stable 0.5.2, got: %s", ver)
	}
}

func TestFetchLatestReleasePrereleaseGetError(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	m := NewManager(cfg, errorGetter{err: errors.New("network down")}, &fakeRunner{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, _, _, err := m.fetchLatestRelease(true)
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
	_, err := m.downloadToTemp(server.URL, "")
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
