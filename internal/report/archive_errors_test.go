package report

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// readZipEntries decodes an in-memory archive into name -> contents.
func readZipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("reading archive: %v", err)
	}
	out := make(map[string][]byte, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// zipWithUnknownCompression builds an archive whose entry declares a
// compression method the standard reader has no decompressor for. Writing it
// needs a registered compressor; reading it back does not have one, so
// entry.Open fails with zip.ErrAlgorithm.
func zipWithUnknownCompression(t *testing.T, name string) []byte {
	t.Helper()
	const oddMethod uint16 = 99

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	w.RegisterCompressor(oddMethod, func(out io.Writer) (io.WriteCloser, error) {
		return nopWriteCloser{out}, nil
	})
	fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: oddMethod})
	if err != nil {
		t.Fatalf("creating entry: %v", err)
	}
	if _, err := fw.Write([]byte("payload")); err != nil {
		t.Fatalf("writing entry: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing archive: %v", err)
	}
	return buf.Bytes()
}

func TestAppendZipBytesReportsUnopenableEntry(t *testing.T) {
	original := zipWithUnknownCompression(t, "odd.txt")

	// Appending a different file means odd.txt is copied through, so the
	// merge has to open it — and cannot.
	if _, err := appendZipBytes(original, map[string][]byte{"new.txt": []byte("new")}); err == nil {
		t.Fatal("appendZipBytes over an unopenable entry succeeded, want an error")
	}
}

func TestAppendFilesToZipReportsUnreadableArchive(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.zip")

	if err := AppendFilesToZip(missing, map[string][]byte{"a.txt": []byte("x")}); err == nil {
		t.Fatal("AppendFilesToZip on a missing archive succeeded, want an error")
	}
}

func TestAppendFilesToZipReportsUnparseableArchive(t *testing.T) {
	// A file that exists but is not a ZIP must fail at the reader, not
	// silently produce a fresh archive that discards the original.
	path := filepath.Join(t.TempDir(), "not-a.zip")
	if err := os.WriteFile(path, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	if err := AppendFilesToZip(path, map[string][]byte{"a.txt": []byte("x")}); err == nil {
		t.Fatal("AppendFilesToZip on a non-ZIP file succeeded, want an error")
	}
}

func TestAppendZipBytesRejectsGarbage(t *testing.T) {
	if _, err := appendZipBytes([]byte("garbage"), nil); err == nil {
		t.Fatal("appendZipBytes on garbage succeeded, want an error")
	}
}

func TestAppendZipBytesRejectsEmptyInput(t *testing.T) {
	if _, err := appendZipBytes(nil, map[string][]byte{"a.txt": []byte("x")}); err == nil {
		t.Fatal("appendZipBytes on empty input succeeded, want an error")
	}
}

func TestAppendZipBytesReportsCorruptEntryPayload(t *testing.T) {
	// Build a valid archive, then corrupt the deflate stream of the entry
	// that will be copied through. The central directory still parses, so
	// the failure surfaces when the entry's contents are read.
	original, err := BuildZip(map[string][]byte{
		"keep.txt": bytes.Repeat([]byte("compressible payload "), 64),
	})
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}

	corrupted := append([]byte(nil), original...)
	// The local file header is 30 bytes plus the name; corrupt well inside
	// the compressed data but before the central directory.
	start := 30 + len("keep.txt") + 8
	end := len(corrupted) - 100
	if start >= end {
		t.Fatalf("archive too small to corrupt: %d bytes", len(corrupted))
	}
	for i := start; i < end; i++ {
		corrupted[i] ^= 0xFF
	}

	// Adding a different file means keep.txt is copied through rather than
	// replaced, so its payload is actually read.
	if _, err := appendZipBytes(corrupted, map[string][]byte{"new.txt": []byte("new")}); err == nil {
		t.Fatal("appendZipBytes over a corrupt payload succeeded, want an error")
	}
}

func TestAppendZipBytesReplacesEntryWithoutReadingIt(t *testing.T) {
	// When an incoming file has the same name as an existing entry, the old
	// payload is replaced outright — it is never decompressed, so even a
	// corrupt original entry is fine.
	original, err := BuildZip(map[string][]byte{"same.txt": []byte("old contents")})
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}

	merged, err := appendZipBytes(original, map[string][]byte{"same.txt": []byte("new contents")})
	if err != nil {
		t.Fatalf("appendZipBytes: %v", err)
	}

	files := readZipEntries(t, merged)
	if len(files) != 1 {
		t.Fatalf("got %d entries, want 1", len(files))
	}
	if got := string(files["same.txt"]); got != "new contents" {
		t.Errorf("same.txt = %q, want %q", got, "new contents")
	}
}
