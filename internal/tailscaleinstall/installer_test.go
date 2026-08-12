package tailscaleinstall

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func testArchive(t *testing.T) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"tailscale_1.102.2_arm64/tailscale", "tailscale_1.102.2_arm64/tailscaled"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: 4}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("test")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func TestInstallerInstallAndIdempotence(t *testing.T) {
	payload := testArchive(t)
	hash := sha256.Sum256(payload)
	original := releases["arm64"]
	releases["arm64"] = Release{Version: Version, Architecture: "arm64", URL: "https://example.invalid/tailscale.tgz", SHA256: hex.EncodeToString(hash[:])}
	t.Cleanup(func() { releases["arm64"] = original })
	root := t.TempDir()
	osRelease := filepath.Join(root, "os-release")
	if err := os.WriteFile(osRelease, []byte("ID=debian\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fetches, runs := 0, 0
	i := Installer{
		Root: root, SystemdDir: filepath.Join(root, "systemd"), LinkDir: filepath.Join(root, "bin"), OSRelease: osRelease,
		Architecture: func() string { return "arm64" },
		Fetch: func(context.Context, string) (io.ReadCloser, error) {
			fetches++
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
		Run: func(context.Context, string, ...string) error { runs++; return nil },
	}
	if err := i.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fetches != 1 || runs != 1 {
		t.Fatalf("first install fetches/runs = %d/%d, want 1/1", fetches, runs)
	}
	for _, name := range []string{"tailscale", "tailscaled"} {
		if info, err := os.Stat(filepath.Join(root, "current", name)); err != nil || info.Mode()&0111 == 0 {
			t.Fatalf("installed %s is not executable: %v", name, err)
		}
	}
	if target, err := os.Readlink(filepath.Join(root, "bin", "tailscale")); err != nil || target != filepath.Join(root, "current", "tailscale") {
		t.Fatalf("tailscale link target = %q, %v", target, err)
	}
	if err := i.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fetches != 1 || runs != 1 {
		t.Fatalf("idempotent install fetches/runs = %d/%d, want 1/1", fetches, runs)
	}
}

func TestInstallerRejectsUnsupportedHost(t *testing.T) {
	root := t.TempDir()
	osRelease := filepath.Join(root, "os-release")
	if err := os.WriteFile(osRelease, []byte("ID=fedora\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := (Installer{OSRelease: osRelease}).requireDebianFamily()
	if err == nil {
		t.Fatal("expected unsupported distribution error")
	}
}
