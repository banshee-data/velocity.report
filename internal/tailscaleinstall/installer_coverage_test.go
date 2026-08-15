package tailscaleinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerPathDefaults(t *testing.T) {
	// An Installer with no overrides falls back to the production layout.
	// The overrides exist so tests can run without root; the defaults are
	// what the appliance actually uses.
	var i Installer

	if got := i.root(); got != rootDir {
		t.Errorf("root() = %q, want %q", got, rootDir)
	}
	if got, want := i.systemdDir(), "/etc/systemd/system"; got != want {
		t.Errorf("systemdDir() = %q, want %q", got, want)
	}
	if got, want := i.linkDir(), "/usr/local/bin"; got != want {
		t.Errorf("linkDir() = %q, want %q", got, want)
	}
	if got := i.architecture(); got != runtime.GOARCH {
		t.Errorf("architecture() = %q, want %q", got, runtime.GOARCH)
	}
}

func TestInstallerPathOverrides(t *testing.T) {
	i := Installer{
		Root:         "/custom/root",
		SystemdDir:   "/custom/systemd",
		LinkDir:      "/custom/bin",
		Architecture: func() string { return "riscv64" },
	}

	if got := i.root(); got != "/custom/root" {
		t.Errorf("root() = %q, want the override", got)
	}
	if got := i.systemdDir(); got != "/custom/systemd" {
		t.Errorf("systemdDir() = %q, want the override", got)
	}
	if got := i.linkDir(); got != "/custom/bin" {
		t.Errorf("linkDir() = %q, want the override", got)
	}
	if got := i.architecture(); got != "riscv64" {
		t.Errorf("architecture() = %q, want the override", got)
	}
}

// debianInstaller returns an installer rooted in a temp dir on a host that
// passes the Debian-family check, with the given fetch behaviour.
func debianInstaller(t *testing.T, fetch func(context.Context, string) (io.ReadCloser, error)) Installer {
	t.Helper()
	root := t.TempDir()
	osRelease := filepath.Join(root, "os-release")
	if err := os.WriteFile(osRelease, []byte("ID=debian\n"), 0o644); err != nil {
		t.Fatalf("writing os-release: %v", err)
	}
	return Installer{
		Root:         root,
		SystemdDir:   filepath.Join(root, "systemd"),
		LinkDir:      filepath.Join(root, "bin"),
		OSRelease:    osRelease,
		Architecture: func() string { return "arm64" },
		Fetch:        fetch,
		Run:          func(context.Context, string, ...string) error { return nil },
	}
}

// pinRelease points the arm64 release at a payload with a given checksum.
func pinRelease(t *testing.T, sha string) {
	t.Helper()
	original := releases["arm64"]
	releases["arm64"] = Release{
		Version:      Version,
		Architecture: "arm64",
		URL:          "https://example.invalid/tailscale.tgz",
		SHA256:       sha,
	}
	t.Cleanup(func() { releases["arm64"] = original })
}

func TestInstallRejectsUnsupportedArchitecture(t *testing.T) {
	i := debianInstaller(t, nil)
	i.Architecture = func() string { return "riscv64" }

	err := i.Install(context.Background())
	if err == nil {
		t.Fatal("Install on an unsupported architecture succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "unsupported Tailscale architecture") {
		t.Errorf("error = %v, want it to name the unsupported architecture", err)
	}
}

func TestInstallReportsDownloadFailure(t *testing.T) {
	pinRelease(t, strings.Repeat("0", 64))
	i := debianInstaller(t, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("network unreachable")
	})

	err := i.Install(context.Background())
	if err == nil {
		t.Fatal("Install with a failing download succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "download Tailscale") {
		t.Errorf("error = %v, want it to name the download failure", err)
	}
}

func TestInstallRejectsChecksumMismatch(t *testing.T) {
	payload := testArchive(t)
	// Pin a checksum that cannot match the payload we serve.
	pinRelease(t, strings.Repeat("a", 64))

	i := debianInstaller(t, func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	})

	err := i.Install(context.Background())
	if err == nil {
		t.Fatal("Install with a mismatched checksum succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want it to report the checksum mismatch", err)
	}

	// A failed verification must not leave binaries behind.
	if _, statErr := os.Stat(filepath.Join(i.Root, "tailscaled")); statErr == nil {
		t.Error("a checksum failure still installed the tailscaled binary")
	}
}

func TestInstallReportsMidDownloadFailure(t *testing.T) {
	pinRelease(t, strings.Repeat("0", 64))
	// A body that errors partway through exercises the copy failure rather
	// than the request failure.
	i := debianInstaller(t, func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(io.MultiReader(
			bytes.NewReader([]byte("partial")),
			errReader{errors.New("connection reset")},
		)), nil
	})

	err := i.Install(context.Background())
	if err == nil {
		t.Fatal("Install with a truncated download succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "write Tailscale download") {
		t.Errorf("error = %v, want it to report the download write failure", err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestInstallReportsExtractionFailure(t *testing.T) {
	// A payload that is not a gzip stream passes the checksum but fails to
	// extract, so the error surfaces from extract rather than the download.
	payload := []byte("this is not a gzip archive")
	sum := sha256.Sum256(payload)
	pinRelease(t, hex.EncodeToString(sum[:]))

	i := debianInstaller(t, func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	})

	err := i.Install(context.Background())
	if err == nil {
		t.Fatal("Install with a non-gzip payload succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "open Tailscale archive") {
		t.Errorf("error = %v, want it to report the archive open failure", err)
	}
}

func TestFetchWithoutOverrideBuildsRequest(t *testing.T) {
	// With no Fetch override the installer performs a real HTTP request. A
	// malformed URL fails during request construction, before any network
	// access, which is the one part of that path testable in a unit test.
	var i Installer

	_, err := i.fetch(context.Background(), "://not a url")
	if err == nil {
		t.Fatal("fetch with a malformed URL succeeded, want an error")
	}
}
