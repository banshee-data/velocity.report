package experiments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateTemp(t *testing.T) {
	dir := t.TempDir()
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		f, err := CreateTemp(dir, ".unit-*.tmp")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		name := filepath.Base(f.Name())
		if !strings.HasPrefix(name, ".unit-") || !strings.HasSuffix(name, ".tmp") || seen[name] {
			t.Fatalf("bad or repeated name %q", name)
		}
		seen[name] = true
		if filepath.Dir(f.Name()) != dir {
			t.Fatalf("%s is outside %s", f.Name(), dir)
		}
	}
	if _, err := CreateTemp(filepath.Join(dir, "missing"), "x*"); err == nil {
		t.Fatal("a missing directory must fail")
	}
	if e, _ := os.ReadDir(dir); len(e) != 50 {
		t.Fatalf("%d files", len(e))
	}
}
