package report

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
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

func TestBuildZip_Deterministic(t *testing.T) {
	files := map[string][]byte{
		"z-last.txt":   []byte("z"),
		"a-first.txt":  []byte("a"),
		"m-middle.txt": []byte("m"),
		"sub/a.csv":    []byte("sub-a"),
		"sub/z.csv":    []byte("sub-z"),
	}

	first, err := BuildZip(files)
	if err != nil {
		t.Fatalf("BuildZip error: %v", err)
	}
	for i := range 10 {
		next, err := BuildZip(files)
		if err != nil {
			t.Fatalf("BuildZip error: %v", err)
		}
		if !bytes.Equal(first, next) {
			t.Fatalf("BuildZip output differs between runs (iteration %d)", i)
		}
	}

	r, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	want := []string{"a-first.txt", "m-middle.txt", "sub/a.csv", "sub/z.csv", "z-last.txt"}
	if len(r.File) != len(want) {
		t.Fatalf("got %d entries, want %d", len(r.File), len(want))
	}
	for i, f := range r.File {
		if f.Name != want[i] {
			t.Errorf("entry[%d] = %q, want %q", i, f.Name, want[i])
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

func TestAppendFilesToZip(t *testing.T) {
	original, err := BuildZip(map[string][]byte{
		"report.tex": []byte("go tex"),
		"chart.svg":  []byte("<svg />"),
	})
	if err != nil {
		t.Fatalf("BuildZip error: %v", err)
	}

	zipPath := filepath.Join(t.TempDir(), "report_sources.zip")
	if err := os.WriteFile(zipPath, original, 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	if err := AppendFilesToZip(zipPath, map[string][]byte{
		"report.tex":                   []byte("updated go tex"),
		"comparison/python/report.tex": []byte("python tex"),
	}); err != nil {
		t.Fatalf("AppendFilesToZip error: %v", err)
	}

	merged, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(merged), int64(len(merged)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	got := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			rc.Close()
			t.Fatalf("read %s: %v", f.Name, err)
		}
		rc.Close()
		got[f.Name] = buf.String()
	}

	if got["report.tex"] != "updated go tex" {
		t.Fatalf("report.tex = %q, want %q", got["report.tex"], "updated go tex")
	}
	if got["chart.svg"] != "<svg />" {
		t.Fatalf("chart.svg = %q, want %q", got["chart.svg"], "<svg />")
	}
	if got["comparison/python/report.tex"] != "python tex" {
		t.Fatalf("comparison/python/report.tex = %q, want %q", got["comparison/python/report.tex"], "python tex")
	}
}
