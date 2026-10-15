package labelset

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// setJSONField rewrites one top-level field of a JSON file, as a writer
// from a newer build would leave it.
func setJSONField(t *testing.T, path, field, value string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.Replace(b, []byte("{\n"), []byte("{\n  \""+field+"\": "+value+",\n"), 1)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMinReaderAboveThisBuildIsRefused(t *testing.T) {
	dir, _ := newSet(t)
	above := strconv.Itoa(annotation.ReaderLevel + 1)
	setJSONField(t, filepath.Join(dir, ManifestFile), "min_reader", above)
	_, err := Open(dir)
	if err == nil || !strings.Contains(err.Error(), "needs reader level "+above) {
		t.Fatalf("a set needing a newer reader opened: %v", err)
	}

	packDir := filepath.Join(t.TempDir(), "pack")
	writeTestPack(t, packDir, []uint64{1}, []testFrame{{0, 1, []uint64{5}}}, "run-a")
	setJSONField(t, filepath.Join(packDir, "manifest.json"), "min_reader", above)
	if _, err := annotation.OpenPack(packDir); err == nil || !strings.Contains(err.Error(), "needs reader level "+above) {
		t.Fatalf("a pack needing a newer reader opened: %v", err)
	}
}

// A file at this build's level opens, and a save keeps its min_reader: a
// writer never lowers what a file says it needs.
func TestMinReaderAtThisLevelIsKept(t *testing.T) {
	dir, _ := newSet(t)
	if b, _ := os.ReadFile(filepath.Join(dir, ManifestFile)); bytes.Contains(b, []byte("min_reader")) {
		t.Fatalf("a level 1 set writes min_reader:\n%s", b)
	}
	setJSONField(t, filepath.Join(dir, ManifestFile), "min_reader", "1")
	ls, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	addMember(t, ls, 40)
	if err := ls.Save(Change{Author: "dd", Operation: "edit"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	if !bytes.Contains(b, []byte(`"min_reader": 1`)) {
		t.Fatalf("the save dropped min_reader:\n%s", b)
	}
}
