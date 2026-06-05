package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteComponentWritesEmbeddedFile(t *testing.T) {
	tmp := t.TempDir()

	for name, c := range installComponents {
		if len(c.data) == 0 {
			t.Errorf("component %q has no embedded data", name)
		}
		if err := writeComponent(c, tmp); err != nil {
			t.Fatalf("writeComponent(%q): %v", name, err)
		}
		dest := filepath.Join(tmp, c.dest)
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("reading %q: %v", dest, err)
		}
		if string(got) != string(c.data) {
			t.Errorf("component %q content mismatch", name)
		}
		info, err := os.Stat(dest)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != c.mode {
			t.Errorf("component %q mode = %o, want %o", name, info.Mode().Perm(), c.mode)
		}
	}
}

func TestWriteComponentIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	c := installComponents["udev"]
	if err := writeComponent(c, tmp); err != nil {
		t.Fatal(err)
	}
	// A second write must succeed and re-apply the mode.
	if err := writeComponent(c, tmp); err != nil {
		t.Fatalf("second writeComponent: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmp, c.dest))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != c.mode {
		t.Errorf("mode after re-run = %o, want %o", info.Mode().Perm(), c.mode)
	}
}
