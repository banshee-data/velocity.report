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

func TestWriteStableLinksRefusesToReplaceNonSymlink(t *testing.T) {
	root := t.TempDir()
	linkDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("creating link dir: %v", err)
	}
	// A real binary sitting where the symlink belongs is almost certainly
	// the distro's own tailscale; clobbering it would be destructive.
	if err := os.WriteFile(filepath.Join(linkDir, "tailscale"), []byte("distro binary"), 0o755); err != nil {
		t.Fatalf("writing decoy binary: %v", err)
	}

	i := Installer{Root: root, LinkDir: linkDir}

	err := i.writeStableLinks()
	if err == nil {
		t.Fatal("writeStableLinks replaced a regular file, want a refusal")
	}
	if !strings.Contains(err.Error(), "refuse to replace non-symlink") {
		t.Errorf("error = %v, want the non-symlink refusal", err)
	}
	// The existing file must be untouched.
	got, readErr := os.ReadFile(filepath.Join(linkDir, "tailscale"))
	if readErr != nil {
		t.Fatalf("reading decoy binary back: %v", readErr)
	}
	if string(got) != "distro binary" {
		t.Errorf("decoy binary = %q, want it left alone", got)
	}
}

func TestWriteStableLinksReportsUncreatableLinkDir(t *testing.T) {
	root := t.TempDir()
	// A regular file where the link directory should be makes MkdirAll fail.
	blocker := filepath.Join(root, "bin")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}

	i := Installer{Root: root, LinkDir: filepath.Join(blocker, "nested")}

	err := i.writeStableLinks()
	if err == nil {
		t.Fatal("writeStableLinks with an uncreatable link dir succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "create Tailscale link directory") {
		t.Errorf("error = %v, want it to name the link directory", err)
	}
}

func TestWriteStableLinksReplacesExistingSymlink(t *testing.T) {
	root := t.TempDir()
	linkDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("creating link dir: %v", err)
	}
	// A stale symlink from a previous install must be refreshed, not refused.
	if err := os.Symlink("/nonexistent/old", filepath.Join(linkDir, "tailscale")); err != nil {
		t.Fatalf("creating stale symlink: %v", err)
	}

	i := Installer{Root: root, LinkDir: linkDir}
	if err := i.writeStableLinks(); err != nil {
		t.Fatalf("writeStableLinks: %v", err)
	}

	target, err := os.Readlink(filepath.Join(linkDir, "tailscale"))
	if err != nil {
		t.Fatalf("reading link: %v", err)
	}
	if want := filepath.Join(root, "current", "tailscale"); target != want {
		t.Errorf("link target = %q, want %q", target, want)
	}
}

func TestWriteServiceReportsUncreatableSystemdDir(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "systemd")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}

	i := Installer{Root: root, SystemdDir: filepath.Join(blocker, "nested")}

	err := i.writeService(Release{Version: Version})
	if err == nil {
		t.Fatal("writeService with an uncreatable systemd dir succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "create systemd directory") {
		t.Errorf("error = %v, want it to name the systemd directory", err)
	}
}

func TestWriteServiceReportsUnwritableUnitFile(t *testing.T) {
	root := t.TempDir()
	systemdDir := filepath.Join(root, "systemd")
	if err := os.MkdirAll(systemdDir, 0o755); err != nil {
		t.Fatalf("creating systemd dir: %v", err)
	}
	// A directory where the unit file belongs makes WriteFile fail.
	if err := os.MkdirAll(filepath.Join(systemdDir, "tailscaled.service"), 0o755); err != nil {
		t.Fatalf("creating blocking directory: %v", err)
	}

	i := Installer{Root: root, SystemdDir: systemdDir}

	err := i.writeService(Release{Version: Version})
	if err == nil {
		t.Fatal("writeService over a directory succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "write tailscaled service") {
		t.Errorf("error = %v, want it to name the unit file", err)
	}
}

func TestWriteServiceEmitsExpectedUnit(t *testing.T) {
	root := t.TempDir()
	systemdDir := filepath.Join(root, "systemd")
	i := Installer{Root: root, SystemdDir: systemdDir}

	if err := i.writeService(Release{Version: Version}); err != nil {
		t.Fatalf("writeService: %v", err)
	}

	unit, err := os.ReadFile(filepath.Join(systemdDir, "tailscaled.service"))
	if err != nil {
		t.Fatalf("reading unit file: %v", err)
	}
	// The unit must point at the version-stable "current" path, not a
	// versioned directory, so upgrades do not need the unit rewritten.
	wantExec := filepath.Join(root, "current", "tailscaled")
	if !strings.Contains(string(unit), "ExecStart="+wantExec) {
		t.Errorf("unit missing ExecStart=%s\n---\n%s", wantExec, unit)
	}
	if !strings.Contains(string(unit), "ExecStopPost="+wantExec+" --cleanup") {
		t.Errorf("unit missing the cleanup ExecStopPost\n---\n%s", unit)
	}
}

func TestRequireDebianFamilyReportsUnreadableOSRelease(t *testing.T) {
	// A host whose os-release cannot be read is not provably Debian-family,
	// so the install must stop rather than assume.
	i := Installer{OSRelease: filepath.Join(t.TempDir(), "absent-os-release")}

	err := i.requireDebianFamily()
	if err == nil {
		t.Fatal("requireDebianFamily with an unreadable os-release succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "read OS release") {
		t.Errorf("error = %v, want it to name the unreadable os-release", err)
	}
}

func TestRequireDebianFamilyAcceptsDebianDerivatives(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"debian", "ID=debian\n"},
		{"raspbian", "ID=raspbian\n"},
		// Ubuntu and friends declare their lineage via ID_LIKE.
		{"derivative via ID_LIKE", "ID=ubuntu\nID_LIKE=debian\n"},
		{"quoted values", "ID=\"debian\"\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "os-release")
			if err := os.WriteFile(path, []byte(tc.contents), 0o644); err != nil {
				t.Fatalf("writing os-release: %v", err)
			}
			if err := (Installer{OSRelease: path}).requireDebianFamily(); err != nil {
				t.Errorf("requireDebianFamily(%q) = %v, want nil", tc.contents, err)
			}
		})
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
