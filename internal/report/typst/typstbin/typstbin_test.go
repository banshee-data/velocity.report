package typstbin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func restoreTypstbinDeps(t *testing.T) {
	t.Helper()
	oldGetenv := osGetenv
	oldStat := osStat
	oldLookPath := execLookPath
	oldUserCacheDir := osUserCacheDir
	oldMkdirAll := osMkdirAll
	oldMkdirTemp := osMkdirTemp
	oldCreateTemp := osCreateTemp
	oldRename := osRename
	oldChmod := osChmod
	oldCacheDir := cacheDirFunc
	oldTypstTarget := typstTargetFunc
	oldHTTPDownload := httpDownloadFunc
	oldExtractTypst := extractTypstFunc
	oldCopyExecutable := copyExecutableFunc
	oldEmbeddedTypst := embeddedTypstFunc
	oldCachedDownload := cachedDownloadFunc
	t.Cleanup(func() {
		osGetenv = oldGetenv
		osStat = oldStat
		execLookPath = oldLookPath
		osUserCacheDir = oldUserCacheDir
		osMkdirAll = oldMkdirAll
		osMkdirTemp = oldMkdirTemp
		osCreateTemp = oldCreateTemp
		osRename = oldRename
		osChmod = oldChmod
		cacheDirFunc = oldCacheDir
		typstTargetFunc = oldTypstTarget
		httpDownloadFunc = oldHTTPDownload
		extractTypstFunc = oldExtractTypst
		copyExecutableFunc = oldCopyExecutable
		embeddedTypstFunc = oldEmbeddedTypst
		cachedDownloadFunc = oldCachedDownload
	})
}

func writeTestExecutable(t *testing.T, path string, body []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestResolveFromEnvPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typst")
	writeTestExecutable(t, path, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv(EnvPath, path)

	got, cleanup, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	cleanup()
	if got != path {
		t.Fatalf("Resolve path = %q, want %q", got, path)
	}
}

func TestResolveFromEnvPathErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvPath, dir)
	if _, _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Resolve directory error = %v, want not a regular file", err)
	}

	path := filepath.Join(t.TempDir(), "typst")
	writeTestExecutable(t, path, []byte("x"), 0o644)
	t.Setenv(EnvPath, path)
	if _, _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("Resolve non-executable error = %v, want not executable", err)
	}
}

func TestResolveEmbeddedPath(t *testing.T) {
	restoreTypstbinDeps(t)
	cacheRoot := t.TempDir()
	embeddedTypstFunc = func() ([]byte, bool) { return []byte("embedded-binary"), true }
	osUserCacheDir = func() (string, error) { return cacheRoot, nil }

	got, cleanup, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	cleanup()
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read embedded output: %v", err)
	}
	if string(data) != "embedded-binary" {
		t.Fatalf("embedded output = %q, want embedded-binary", data)
	}
}

func TestResolveFromPathAndDownloadFallbacks(t *testing.T) {
	restoreTypstbinDeps(t)
	embeddedTypstFunc = func() ([]byte, bool) { return nil, false }

	pathDir := t.TempDir()
	typstPath := filepath.Join(pathDir, "typst")
	writeTestExecutable(t, typstPath, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", pathDir)
	got, cleanup, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve PATH fallback: %v", err)
	}
	cleanup()
	if got != typstPath {
		t.Fatalf("Resolve path = %q, want %q", got, typstPath)
	}

	restoreTypstbinDeps(t)
	embeddedTypstFunc = func() ([]byte, bool) { return nil, false }
	execLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	cachedDownloadFunc = func() (string, error) { return "/tmp/downloaded-typst", nil }
	got, cleanup, err = Resolve()
	if err != nil {
		t.Fatalf("Resolve download fallback: %v", err)
	}
	cleanup()
	if got != "/tmp/downloaded-typst" {
		t.Fatalf("Resolve download path = %q, want /tmp/downloaded-typst", got)
	}

	restoreTypstbinDeps(t)
	embeddedTypstFunc = func() ([]byte, bool) { return nil, false }
	execLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	cachedDownloadFunc = func() (string, error) { return "", errors.New("boom") }
	if _, _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "auto-download failed") {
		t.Fatalf("Resolve download error = %v, want auto-download failed", err)
	}

	restoreTypstbinDeps(t)
	embeddedTypstFunc = func() ([]byte, bool) { return nil, false }
	execLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Setenv(EnvNoDownload, "1")
	if _, _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "typst executable not found") {
		t.Fatalf("Resolve final error = %v, want typst executable not found", err)
	}
}

func TestEmbeddedAndExecutableHelpers(t *testing.T) {
	if Embedded() {
		t.Fatal("Embedded should be false in the default build")
	}

	path := filepath.Join(t.TempDir(), "typst")
	writeTestExecutable(t, path, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	if err := validateExecutable(path); err != nil {
		t.Fatalf("validateExecutable: %v", err)
	}
	if err := validateExecutable(filepath.Dir(path)); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("validateExecutable directory error = %v, want not a regular file", err)
	}

	noExec := filepath.Join(t.TempDir(), "noexec")
	writeTestExecutable(t, noExec, []byte("x"), 0o644)
	if err := validateExecutable(noExec); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("validateExecutable non-executable error = %v, want not executable", err)
	}

	if exeSuffixFor("windows") != ".exe" || exeSuffixFor("darwin") != "" {
		t.Fatal("exeSuffixFor returned unexpected values")
	}
	if !isExecutableMode("windows", 0) {
		t.Fatal("windows should ignore execute mode bits")
	}
	if isExecutableMode("darwin", 0o644) {
		t.Fatal("darwin should require execute mode bits")
	}

	if !isUsableBinary(path, 17) {
		t.Fatal("isUsableBinary should accept matching executable")
	}
	if isUsableBinary(path, 18) {
		t.Fatal("isUsableBinary should reject the wrong size")
	}
	if !usableExecutable(path) {
		t.Fatal("usableExecutable should accept a non-empty executable")
	}
	empty := filepath.Join(t.TempDir(), "empty")
	writeTestExecutable(t, empty, nil, 0o755)
	if usableExecutable(empty) {
		t.Fatal("usableExecutable should reject an empty file")
	}
}

func TestCacheDirAndExtractCached(t *testing.T) {
	restoreTypstbinDeps(t)
	cacheRoot := t.TempDir()
	osUserCacheDir = func() (string, error) { return cacheRoot, nil }
	dir, err := cacheDir()
	if err != nil {
		t.Fatalf("cacheDir: %v", err)
	}
	if want := filepath.Join(cacheRoot, "velocity-report", "typst"); dir != want {
		t.Fatalf("cacheDir = %q, want %q", dir, want)
	}

	restoreTypstbinDeps(t)
	osUserCacheDir = func() (string, error) { return "", errors.New("no cache dir") }
	dir, err = cacheDir()
	if err != nil {
		t.Fatalf("cacheDir fallback: %v", err)
	}
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		t.Fatalf("cacheDir fallback dir missing: %v", statErr)
	}

	restoreTypstbinDeps(t)
	osUserCacheDir = func() (string, error) { return cacheRoot, nil }
	data := []byte("embedded-binary")
	first, err := extractCached(data)
	if err != nil {
		t.Fatalf("extractCached first: %v", err)
	}
	second, err := extractCached(data)
	if err != nil {
		t.Fatalf("extractCached second: %v", err)
	}
	if first != second {
		t.Fatalf("extractCached reused path = %q, want %q", second, first)
	}

	restoreTypstbinDeps(t)
	cacheRoot = t.TempDir()
	osUserCacheDir = func() (string, error) { return cacheRoot, nil }
	osRename = func(src, dst string) error {
		if data, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(dst, data, 0o755)
		}
		return os.ErrExist
	}
	if _, err := extractCached([]byte("rename-conflict")); err != nil {
		t.Fatalf("extractCached rename conflict: %v", err)
	}

	restoreTypstbinDeps(t)
	cacheRoot = t.TempDir()
	osUserCacheDir = func() (string, error) { return cacheRoot, nil }
	osRename = func(string, string) error { return errors.New("rename failed") }
	if _, err := extractCached([]byte("rename-error")); err == nil || !strings.Contains(err.Error(), "rename failed") {
		t.Fatalf("extractCached rename error = %v, want rename failed", err)
	}
}

func TestDownloadHelpers(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"yes", true},
		{"on", true},
		{"0", false},
		{"false", false},
		{"", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(EnvNoDownload, tc.value)
			if got := downloadDisabled(); got != tc.want {
				t.Fatalf("downloadDisabled(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		goos    string
		goarch  string
		target  string
		archive string
		wantErr bool
	}{
		{"linux", "arm64", "aarch64-unknown-linux-musl", "tar.xz", false},
		{"linux", "amd64", "x86_64-unknown-linux-musl", "tar.xz", false},
		{"darwin", "arm64", "aarch64-apple-darwin", "tar.xz", false},
		{"darwin", "amd64", "x86_64-apple-darwin", "tar.xz", false},
		{"windows", "amd64", "", "", true},
		{"plan9", "arm", "", "", true},
	} {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			target, archive, err := typstTargetFor(tc.goos, tc.goarch)
			if tc.wantErr {
				if err == nil {
					t.Fatal("typstTargetFor should have failed")
				}
				return
			}
			if err != nil {
				t.Fatalf("typstTargetFor: %v", err)
			}
			if target != tc.target || archive != tc.archive {
				t.Fatalf("typstTargetFor = %q %q, want %q %q", target, archive, tc.target, tc.archive)
			}
		})
	}
}

func TestHTTPDownloadAndArchiveHelpers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "payload.bin")
	if err := httpDownload(server.URL, dest); err != nil {
		t.Fatalf("httpDownload: %v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "payload" {
		t.Fatalf("downloaded payload = %q, err=%v", got, err)
	}

	serverFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer serverFail.Close()
	if err := httpDownload(serverFail.URL, filepath.Join(t.TempDir(), "fail.bin")); err == nil {
		t.Fatal("httpDownload should fail on non-200 status")
	}

	copySrc := filepath.Join(t.TempDir(), "src")
	copyDst := filepath.Join(t.TempDir(), "dst")
	writeTestExecutable(t, copySrc, []byte("copy-me"), 0o755)
	if err := copyExecutable(copySrc, copyDst); err != nil {
		t.Fatalf("copyExecutable: %v", err)
	}
	if got, err := os.ReadFile(copyDst); err != nil || string(got) != "copy-me" {
		t.Fatalf("copyExecutable output = %q, err=%v", got, err)
	}
}

func TestExtractTypst(t *testing.T) {
	if _, err := extractTypst(filepath.Join(t.TempDir(), "typst.zip"), "zip", "x86_64-pc-windows-msvc", t.TempDir()); err == nil {
		t.Fatal("extractTypst should reject unsupported archive types")
	}

	tarTarget := "aarch64-apple-darwin"
	archiveRoot := t.TempDir()
	typstPath := filepath.Join(archiveRoot, "typst-"+tarTarget, "typst")
	writeTestExecutable(t, typstPath, []byte("tar-typst"), 0o755)
	tarPath := filepath.Join(t.TempDir(), "typst.tar.xz")
	cmd := exec.Command("tar", "-cJf", tarPath, "-C", archiveRoot, "typst-"+tarTarget)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("tar with xz support unavailable: %v: %s", err, out)
	}
	tarDestDir := t.TempDir()
	tarDest, err := extractTypst(tarPath, "tar.xz", tarTarget, tarDestDir)
	if err != nil {
		t.Fatalf("extractTypst tar.xz: %v", err)
	}
	if got, err := os.ReadFile(tarDest); err != nil || string(got) != "tar-typst" {
		t.Fatalf("extractTypst tar output = %q, err=%v", got, err)
	}

	brokenTar := filepath.Join(t.TempDir(), "broken.tar.xz")
	writeTestExecutable(t, brokenTar, []byte("not-a-tar"), 0o644)
	if _, err := extractTypst(brokenTar, "tar.xz", tarTarget, t.TempDir()); err == nil {
		t.Fatal("extractTypst should fail for a broken tar.xz archive")
	}
}

func TestCachedDownload(t *testing.T) {
	restoreTypstbinDeps(t)
	base := t.TempDir()
	versionDir := filepath.Join(base, Version)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		t.Fatalf("mkdir version dir: %v", err)
	}
	dest := filepath.Join(versionDir, "typst"+exeSuffix())
	writeTestExecutable(t, dest, []byte("cached"), 0o755)
	cacheDirFunc = func() (string, error) { return base, nil }
	httpDownloadFunc = func(string, string) error {
		return errors.New("should not download on cache hit")
	}
	got, err := cachedDownload()
	if err != nil {
		t.Fatalf("cachedDownload cache hit: %v", err)
	}
	if got != dest {
		t.Fatalf("cachedDownload cache hit path = %q, want %q", got, dest)
	}

	restoreTypstbinDeps(t)
	base = t.TempDir()
	cacheDirFunc = func() (string, error) { return base, nil }
	typstTargetFunc = func() (string, string, error) { return "test-target", "zip", nil }
	httpDownloadFunc = func(url, archivePath string) error {
		if !strings.Contains(url, Version) || !strings.Contains(url, "test-target") {
			return fmt.Errorf("unexpected URL %q", url)
		}
		return os.WriteFile(archivePath, []byte("archive"), 0o644)
	}
	extractTypstFunc = func(string, string, string, string) (string, error) {
		bin := filepath.Join(t.TempDir(), "typst-bin")
		writeTestExecutable(t, bin, []byte("downloaded"), 0o755)
		return bin, nil
	}
	got, err = cachedDownload()
	if err != nil {
		t.Fatalf("cachedDownload success: %v", err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "downloaded" {
		t.Fatalf("cachedDownload output = %q, err=%v", data, err)
	}

	restoreTypstbinDeps(t)
	cacheDirFunc = func() (string, error) { return t.TempDir(), nil }
	typstTargetFunc = func() (string, string, error) { return "", "", errors.New("bad target") }
	if _, err := cachedDownload(); err == nil || !strings.Contains(err.Error(), "bad target") {
		t.Fatalf("cachedDownload target error = %v, want bad target", err)
	}

	restoreTypstbinDeps(t)
	cacheDirFunc = func() (string, error) { return t.TempDir(), nil }
	typstTargetFunc = func() (string, string, error) { return "test-target", "zip", nil }
	httpDownloadFunc = func(string, string) error { return errors.New("download failed") }
	if _, err := cachedDownload(); err == nil || !strings.Contains(err.Error(), "download typst") {
		t.Fatalf("cachedDownload download error = %v, want download typst", err)
	}

	restoreTypstbinDeps(t)
	cacheDirFunc = func() (string, error) { return t.TempDir(), nil }
	typstTargetFunc = func() (string, string, error) { return "test-target", "zip", nil }
	httpDownloadFunc = func(string, string) error { return nil }
	extractTypstFunc = func(string, string, string, string) (string, error) { return "", errors.New("extract failed") }
	if _, err := cachedDownload(); err == nil || !strings.Contains(err.Error(), "extract failed") {
		t.Fatalf("cachedDownload extract error = %v, want extract failed", err)
	}

	restoreTypstbinDeps(t)
	base = t.TempDir()
	cacheDirFunc = func() (string, error) { return base, nil }
	typstTargetFunc = func() (string, string, error) { return "test-target", "zip", nil }
	httpDownloadFunc = func(string, string) error { return nil }
	extractTypstFunc = func(string, string, string, string) (string, error) {
		bin := filepath.Join(t.TempDir(), "bin")
		writeTestExecutable(t, bin, []byte("rename-conflict"), 0o755)
		return bin, nil
	}
	osRename = func(src, dst string) error {
		data, _ := os.ReadFile(src)
		_ = os.WriteFile(dst, data, 0o755)
		return os.ErrExist
	}
	got, err = cachedDownload()
	if err != nil {
		t.Fatalf("cachedDownload rename conflict: %v", err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "rename-conflict" {
		t.Fatalf("cachedDownload rename conflict output = %q, err=%v", data, err)
	}

	restoreTypstbinDeps(t)
	base = t.TempDir()
	cacheDirFunc = func() (string, error) { return base, nil }
	typstTargetFunc = func() (string, string, error) { return "test-target", "zip", nil }
	httpDownloadFunc = func(string, string) error { return nil }
	extractTypstFunc = func(string, string, string, string) (string, error) {
		bin := filepath.Join(t.TempDir(), "bin")
		writeTestExecutable(t, bin, []byte("copy-fallback"), 0o755)
		return bin, nil
	}
	osRename = func(string, string) error { return errors.New("cross-device link") }
	copyExecutableFunc = func(src, dst string) error {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o755)
	}
	got, err = cachedDownload()
	if err != nil {
		t.Fatalf("cachedDownload copy fallback: %v", err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "copy-fallback" {
		t.Fatalf("cachedDownload copy fallback output = %q, err=%v", data, err)
	}
}

func TestRuntimeSpecificHelpersMatchCurrentPlatform(t *testing.T) {
	target, archive, err := typstTarget()
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		if err != nil || target != "aarch64-apple-darwin" || archive != "tar.xz" {
			t.Fatalf("typstTarget = %q %q %v, want aarch64-apple-darwin tar.xz", target, archive, err)
		}
	}
	if exeSuffix() != exeSuffixFor(runtime.GOOS) {
		t.Fatal("exeSuffix should delegate to exeSuffixFor")
	}
}
