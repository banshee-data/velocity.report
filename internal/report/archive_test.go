package report

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestBuildZip(t *testing.T) {
	files := map[string][]byte{
		"hello.txt":    []byte("hello world"),
		"sub/data.csv": []byte("a,b,c\n1,2,3"),
	}

	data, err := BuildZip(files)
	if err != nil {
		t.Fatalf("BuildZip error: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	found := make(map[string]bool)
	for _, f := range r.File {
		found[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		buf.ReadFrom(rc)
		rc.Close()

		expected := files[f.Name]
		if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("%s: got %q, want %q", f.Name, buf.String(), string(expected))
		}
	}

	for name := range files {
		if !found[name] {
			t.Errorf("missing file in zip: %s", name)
		}
	}
}

func TestBuildZip_Empty(t *testing.T) {
	data, err := BuildZip(map[string][]byte{})
	if err != nil {
		t.Fatalf("BuildZip error: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if len(r.File) != 0 {
		t.Errorf("expected empty zip, got %d files", len(r.File))
	}
}
