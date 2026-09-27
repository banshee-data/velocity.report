package vrlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Only an object that does not exist may read as missing: a chain walk takes
// a missing generation as its end, so an I/O or permission failure classed as
// missing would silently truncate the evidence instead of failing the open.
func TestClassifyReadError(t *testing.T) {
	missing := classifyReadError(fmt.Errorf("open: %w", fs.ErrNotExist), "generations/00000003.gen", -1)
	if !isMissing(missing) {
		t.Fatalf("a missing object is not missing: %v", missing)
	}

	failed := classifyReadError(&os.PathError{Op: "read", Path: "x", Err: syscall.EIO}, "generations/00000003.gen", -1)
	var corrupt *CorruptionError
	if isMissing(failed) || errors.As(failed, &corrupt) {
		t.Fatalf("an I/O error reads as corruption: %v", failed)
	}
	if !errors.Is(failed, syscall.EIO) {
		t.Fatalf("the I/O error is lost: %v", failed)
	}

	located := classifyReadError(&CorruptionError{Kind: CorruptLength, Chunk: -1, Length: -1}, "current", 4)
	if !errors.As(located, &corrupt) || corrupt.Object != "current" || corrupt.Chunk != 4 || corrupt.Kind != CorruptLength {
		t.Fatalf("corruption not located: %v", located)
	}

	// A directory where an object belongs is damage, not an absent object.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "object"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := readBounded(filepath.Join(dir, "object"), 1<<10)
	notRegular := locateObject(err, "generations/00000001.gen")
	if isMissing(notRegular) || !errors.As(notRegular, &corrupt) || corrupt.Kind != CorruptStructure {
		t.Fatalf("a directory in an object's place = %v", notRegular)
	}
}
