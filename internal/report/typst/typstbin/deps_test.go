package typstbin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultCreateTempExecCreatesAWritableFile(t *testing.T) {
	dir := t.TempDir()

	f, err := defaultCreateTempExec(dir, "typst-*.tmp")
	if err != nil {
		t.Fatalf("defaultCreateTempExec: %v", err)
	}
	t.Cleanup(func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	})

	name := f.Name()
	if got := filepath.Dir(name); got != dir {
		t.Errorf("temp file created in %q, want %q", got, dir)
	}
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "typst-") || !strings.HasSuffix(base, ".tmp") {
		t.Errorf("temp file name = %q, want it to match the typst-*.tmp pattern", base)
	}

	// The returned handle must satisfy the tempExecutable contract the
	// download path relies on: write, chmod, then close.
	if _, err := f.Write([]byte("payload")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Chmod(0o755); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	info, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, want the executable bits set", info.Mode().Perm())
	}
}

func TestDefaultCreateTempExecFailsInMissingDirectory(t *testing.T) {
	_, err := defaultCreateTempExec(filepath.Join(t.TempDir(), "absent"), "typst-*.tmp")
	if err == nil {
		t.Fatal("defaultCreateTempExec in a missing directory succeeded, want an error")
	}
}
