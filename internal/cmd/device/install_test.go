package device

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestRunInstallWithRootWritesComponent(t *testing.T) {
	tmp := t.TempDir()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runInstallWithRoot([]string{"wifi"}, tmp)

	_ = w.Close()
	os.Stdout = old
	var out bytes.Buffer
	_, _ = out.ReadFrom(r)

	if err != nil {
		t.Fatalf("runInstallWithRoot failed: %v", err)
	}
	dest := filepath.Join(tmp, installComponents["wifi"].dest)
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("expected installed file at %s: %v", dest, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("installed mode = %o, want 0600", info.Mode().Perm())
	}
	if !strings.Contains(out.String(), "installed wifi") {
		t.Fatalf("expected install confirmation, got %q", out.String())
	}
}

func TestRunInstallUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--bogus"},
		{"network", "extra"},
		{"bogus"},
	} {
		if err := runInstallWithRoot(args, t.TempDir()); err == nil {
			t.Fatalf("runInstallWithRoot(%v) returned nil, want error", args)
		}
	}
}

func TestRunInstallHelpReturnsNil(t *testing.T) {
	if err := runInstallWithRoot([]string{"--help"}, t.TempDir()); err != nil {
		t.Fatalf("runInstallWithRoot --help returned error: %v", err)
	}
}

func TestRunInstallReturnsWriteError(t *testing.T) {
	tmp := t.TempDir()
	blockingFile := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blockingFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runInstallWithRoot([]string{"network"}, blockingFile); err == nil {
		t.Fatal("expected write error when root path is a file")
	}
}

func TestWriteComponentWithReportsStepErrors(t *testing.T) {
	c := installComponent{data: []byte("x"), dest: "/etc/example", mode: 0o644}
	boom := errors.New("boom")
	ok := componentWriter{
		mkdirAll:  func(string, os.FileMode) error { return nil },
		writeFile: func(string, []byte, os.FileMode) error { return nil },
		chmod:     func(string, os.FileMode) error { return nil },
	}

	cases := []struct {
		name   string
		writer componentWriter
		want   string
	}{
		{
			name: "mkdir",
			writer: componentWriter{
				mkdirAll:  func(string, os.FileMode) error { return boom },
				writeFile: ok.writeFile,
				chmod:     ok.chmod,
			},
			want: "creating",
		},
		{
			name: "write",
			writer: componentWriter{
				mkdirAll:  ok.mkdirAll,
				writeFile: func(string, []byte, os.FileMode) error { return boom },
				chmod:     ok.chmod,
			},
			want: "writing",
		},
		{
			name: "chmod",
			writer: componentWriter{
				mkdirAll:  ok.mkdirAll,
				writeFile: ok.writeFile,
				chmod:     func(string, os.FileMode) error { return boom },
			},
			want: "chmod",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeComponentWith(c, t.TempDir(), tc.writer)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}

	if err := writeComponentWith(c, t.TempDir(), ok); err != nil {
		t.Fatalf("writeComponentWith success path failed: %v", err)
	}
}
