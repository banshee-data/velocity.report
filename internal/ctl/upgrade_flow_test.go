package ctl

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

// stubVersionGetter serves a canned GET /api/version body for verify tests.
type stubVersionGetter struct {
	body   string
	status int
}

func (g stubVersionGetter) Get(_ string) (*http.Response, error) {
	st := g.status
	if st == 0 {
		st = http.StatusOK
	}
	return &http.Response{
		StatusCode: st,
		Body:       io.NopCloser(strings.NewReader(g.body)),
		Header:     make(http.Header),
	}, nil
}

// TestRunUpgradeDownloadHappyPath exercises the full release path end to end:
// fetch release.json, download the asset, verify its SHA-256, stage it, migrate
// with the new binary, swap current, set previous, and restart.
func TestRunUpgradeDownloadHappyPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	cfg.CurrentVersion = "0.5.1"

	binaryBytes := []byte("downloaded-velocity-binary-bytes")
	sum := sha256.Sum256(binaryBytes)
	sha := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(binaryBytes)
	})
	var srv *httptest.Server
	mux.HandleFunc("/release.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"stable":{"linux_arm64":{"version":"0.5.2","url":"%s/bin","sha256":"%s"}}}`, srv.URL, sha)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	cfg.ReleaseMetaURL = srv.URL + "/release.json"

	seedVersion(t, cfg, "0.5.1", "old-binary")
	linkVersion(t, cfg, currentName, "0.5.1")

	runner := &fakeRunner{}
	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)
	m.sleep = func(time.Duration) {}

	if err := m.RunUpgrade(false, ""); err != nil {
		t.Fatalf("RunUpgrade failed: %v\nout=%s", err, out.String())
	}

	if got := currentVersion(t, cfg); got != "0.5.2" {
		t.Fatalf("current = %s, want 0.5.2", got)
	}
	newBin := filepath.Join(cfg.InstallRoot, versionsSubdir, "0.5.2", cfg.BinaryName)
	if got, _ := os.ReadFile(newBin); string(got) != string(binaryBytes) {
		t.Fatalf("installed binary content mismatch")
	}
	if prev, _ := os.Readlink(filepath.Join(cfg.InstallRoot, previousName)); filepath.Base(prev) != "0.5.1" {
		t.Fatalf("previous = %s, want 0.5.1", filepath.Base(prev))
	}
	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, newBin+" data migrate up --db-path "+cfg.DBPath) {
		t.Fatalf("missing migration call from the new binary: %#v", runner.calls)
	}
	if !strings.Contains(joined, "systemctl restart "+cfg.ServiceName) {
		t.Fatalf("missing service restart: %#v", runner.calls)
	}
	if !strings.Contains(out.String(), "SHA-256 verified") {
		t.Fatalf("expected SHA-256 verification in output: %s", out.String())
	}
}

// TestRunUpgradeDownloadSHAMismatchAborts confirms a tampered/wrong asset is
// rejected before anything is activated.
func TestRunUpgradeDownloadSHAMismatchAborts(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	cfg.CurrentVersion = "0.5.1"

	mux := http.NewServeMux()
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("actual-bytes"))
	})
	var srv *httptest.Server
	mux.HandleFunc("/release.json", func(w http.ResponseWriter, _ *http.Request) {
		// SHA for some other content — will not match the served bytes.
		fmt.Fprintf(w, `{"stable":{"linux_arm64":{"version":"0.5.2","url":"%s/bin","sha256":"%s"}}}`, srv.URL, strings.Repeat("a", 64))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	cfg.ReleaseMetaURL = srv.URL + "/release.json"

	seedVersion(t, cfg, "0.5.1", "old-binary")
	linkVersion(t, cfg, currentName, "0.5.1")

	runner := &fakeRunner{}
	var out bytes.Buffer
	m := NewManager(cfg, nil, runner, &out, &out)

	err := m.RunUpgrade(false, "")
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected SHA-256 mismatch error, got: %v", err)
	}
	if got := currentVersion(t, cfg); got != "0.5.1" {
		t.Fatalf("current must stay 0.5.1 after a rejected download, got %s", got)
	}
	if strings.Contains(strings.Join(runner.calls, "\n"), "systemctl") {
		t.Fatalf("no service action should run on a rejected download: %#v", runner.calls)
	}
}

// TestSequentialUpgradesRetainThreeVersions runs four upgrades and asserts the
// versions/ directory is capped at Retain (3) while current/previous resolve
// correctly and the oldest is pruned.
func TestSequentialUpgradesRetainThreeVersions(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp) // Retain = 3
	src := filepath.Join(tmp, "bin")
	if err := os.WriteFile(src, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	m.sleep = func(time.Duration) {}

	for _, v := range []string{"0.5.1", "0.5.2", "0.5.3", "0.5.4"} {
		if err := m.applyVersionedUpgrade(v, src); err != nil {
			t.Fatalf("upgrade to %s failed: %v", v, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(cfg.InstallRoot, versionsSubdir))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = true
	}
	if len(got) != 3 {
		t.Fatalf("expected exactly 3 retained versions, got %d: %v", len(got), got)
	}
	for _, keep := range []string{"0.5.2", "0.5.3", "0.5.4"} {
		if !got[keep] {
			t.Errorf("expected %s retained", keep)
		}
	}
	if got["0.5.1"] {
		t.Errorf("expected oldest 0.5.1 pruned")
	}
	if cur := currentVersion(t, cfg); cur != "0.5.4" {
		t.Fatalf("current = %s, want 0.5.4", cur)
	}
	if prev, _ := os.Readlink(filepath.Join(cfg.InstallRoot, previousName)); filepath.Base(prev) != "0.5.3" {
		t.Fatalf("previous = %s, want 0.5.3", filepath.Base(prev))
	}
}

func TestVerifyRunningVersionMatch(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	cfg.VerifyTimeout = time.Second

	var out bytes.Buffer
	m := NewManager(cfg, stubVersionGetter{body: `{"version":"0.5.2"}`}, &fakeRunner{}, &out, &out)
	m.sleep = func(time.Duration) {}

	m.verifyRunningVersion("0.5.2")
	if !strings.Contains(out.String(), "Service is running v0.5.2") {
		t.Fatalf("expected running-version confirmation, got: %s", out.String())
	}
}

func TestVerifyRunningVersionTimeoutWarns(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	cfg.VerifyTimeout = time.Second

	var out bytes.Buffer
	// Reports a different version forever → never matches → must time out.
	m := NewManager(cfg, stubVersionGetter{body: `{"version":"0.5.1"}`}, &fakeRunner{}, &out, &out)
	m.sleep = func(time.Duration) {}
	// Advance the clock past the deadline so the loop terminates deterministically.
	base := time.Unix(0, 0)
	n := 0
	m.now = func() time.Time {
		n++
		return base.Add(time.Duration(n) * cfg.VerifyTimeout)
	}

	m.verifyRunningVersion("0.5.2")
	if !strings.Contains(out.String(), "could not confirm the running version is v0.5.2") {
		t.Fatalf("expected timeout warning, got: %s", out.String())
	}
}

func TestVerifyRunningVersionSkippedWhenTimeoutZero(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp) // VerifyTimeout = 0
	var out bytes.Buffer
	// A getter that would error if called — proves the poll is skipped entirely.
	m := NewManager(cfg, errorGetter{err: fmt.Errorf("should not be called")}, &fakeRunner{}, &out, &out)
	m.verifyRunningVersion("0.5.2")
	if out.Len() != 0 {
		t.Fatalf("expected no output when verify is disabled, got: %s", out.String())
	}
}

// TestCreateBackupConsistentWALSnapshot proves the manager takes a consistent
// VACUUM INTO snapshot of a live WAL database (data still in the -wal), not a
// stale raw copy.
func TestCreateBackupConsistentWALSnapshot(t *testing.T) {
	tmp := t.TempDir()
	cfg := testConfig(tmp)
	seedVersion(t, cfg, "0.5.1", "bin")
	linkVersion(t, cfg, currentName, "0.5.1")

	writer, err := sql.Open("sqlite", "file:"+cfg.DBPath+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer writer.Close()
	if _, err := writer.Exec(`CREATE TABLE t (id INTEGER); INSERT INTO t VALUES (1),(2),(3),(4)`); err != nil {
		t.Fatalf("seed db: %v", err)
	}

	var out bytes.Buffer
	m := NewManager(cfg, nil, &fakeRunner{}, &out, &out)
	m.now = func() time.Time { return time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC) }

	backupPath, err := m.RunBackup(cfg.BackupDir)
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if strings.Contains(out.String(), "falling back to a raw file copy") {
		t.Fatalf("real SQLite DB should use VACUUM INTO, not the raw-copy fallback: %s", out.String())
	}

	bk, err := sql.Open("sqlite", "file:"+filepath.Join(backupPath, "sensor_data.db")+"?mode=ro")
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer bk.Close()
	var count int
	if err := bk.QueryRow(`SELECT count(*) FROM t`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 4 {
		t.Fatalf("backup row count = %d, want 4 (WAL data lost in backup)", count)
	}
}
