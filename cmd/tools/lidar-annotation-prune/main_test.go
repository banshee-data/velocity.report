package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture writes a pack directory holding what a prune reads: a manifest, a
// head and n archived revisions twenty minutes apart.
func fixture(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	hist := filepath.Join(dir, "annotation-revisions")
	if err := os.MkdirAll(hist, 0o700); err != nil {
		t.Fatal(err)
	}
	epoch := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	write := func(path, body string, at time.Time) {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "manifest.json"), `{"pack_digest":"sha256:pack-a"}`, epoch)
	for i := 1; i <= n; i++ {
		write(filepath.Join(hist, fmt.Sprintf("%010d.json", i)), strings.Repeat("x", 100*i), epoch.Add(time.Duration(i-1)*20*time.Minute))
	}
	write(filepath.Join(dir, "annotations.json"), `{"revision":99}`, epoch)
	return dir
}

func runTool(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestDryRunPlansAndWritesNothing(t *testing.T) {
	dir := fixture(t, 12)
	frozen := filepath.Join(t.TempDir(), "frozen.json")
	os.WriteFile(frozen, []byte(`{"packs":[{"pack_digest":"sha256:pack-a","sidecar_revision":5}]}`), 0o600)
	jsonPath := filepath.Join(t.TempDir(), "plan.json")
	code, out, errs := runTool("-pack", dir, "-keep-last", "2", "-keep-every", "1h", "-split", frozen, "-protect", "9", "-json", jsonPath)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	for _, want := range []string{"retained: 12 revisions", "keep:", "prune:", "protected revision 5: pinned by frozen.json", "protected revision 9: named by -protect", "dry run: nothing was written"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "annotation-revisions"))
	if len(entries) != 12 {
		t.Fatalf("a dry run removed files: %d left", len(entries))
	}
	var doc struct {
		Plan struct {
			Pruned int `json:"pruned"`
		} `json:"plan"`
		Report any `json:"report"`
	}
	b, err := os.ReadFile(jsonPath)
	if err != nil || json.Unmarshal(b, &doc) != nil || doc.Plan.Pruned == 0 || doc.Report != nil {
		t.Fatalf("json %s, %v", b, err)
	}
}

func TestApplyBacksUpAndRemoves(t *testing.T) {
	dir := fixture(t, 12)
	os.WriteFile(filepath.Join(dir, "annotation-revisions", "notes.txt"), []byte("not an archive"), 0o600)
	backup := filepath.Join(t.TempDir(), "backup.tar.gz")
	jsonPath := filepath.Join(t.TempDir(), "run.json")
	code, out, errs := runTool("-pack", dir, "-keep-last", "2", "-keep-every", "0", "-apply", "-backup", backup, "-json", jsonPath)
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errs, out)
	}
	for _, want := range []string{"backup verified", "backup: " + backup, "removed: 9 revisions", "ignored (not archive files): notes.txt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "annotation-revisions"))
	if len(entries) != 4 { // the oldest, the newest two and the note
		t.Fatalf("%d entries left", len(entries))
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatal(err)
	}
	// With nothing left to prune a second run writes no backup.
	again := filepath.Join(t.TempDir(), "again.tar.gz")
	if code, out, errs := runTool("-pack", dir, "-keep-last", "5", "-apply", "-backup", again); code != 0 || !strings.Contains(out, "removed: 0 revisions") {
		t.Fatalf("second run exit %d: %s\n%s", code, errs, out)
	}
	if _, err := os.Stat(again); !os.IsNotExist(err) {
		t.Fatalf("a backup was written for nothing: %v", err)
	}
	b, _ := os.ReadFile(jsonPath)
	if !strings.Contains(string(b), `"report"`) || !strings.Contains(string(b), `"removed": 9`) {
		t.Fatalf("json lacks the report: %s", b)
	}
}

func TestApplyFailureIsReported(t *testing.T) {
	dir := fixture(t, 6)
	backup := filepath.Join(t.TempDir(), "backup.tar.gz")
	os.WriteFile(backup, []byte("taken"), 0o600)
	code, _, errs := runTool("-pack", dir, "-keep-last", "1", "-apply", "-backup", backup)
	if code != 1 || !strings.Contains(errs, "already exists") {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if b, _ := os.ReadFile(backup); string(b) != "taken" {
		t.Fatalf("the backup was overwritten: %q", b)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "annotation-revisions"))
	if len(entries) != 6 {
		t.Fatalf("%d revisions left after a refused prune", len(entries))
	}
}

func TestUsageErrors(t *testing.T) {
	dir := fixture(t, 4)
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o600)
	cases := map[string]struct {
		args []string
		code int
		want string
	}{
		"no pack":              {nil, 2, "-pack is required"},
		"stray argument":       {[]string{"-pack", dir, "extra"}, 2, "takes no other arguments"},
		"apply without backup": {[]string{"-pack", dir, "-apply"}, 2, "-apply needs -backup"},
		"backup without apply": {[]string{"-pack", dir, "-backup", "x.tar.gz"}, 2, "only used with -apply"},
		"bad protect":          {[]string{"-pack", dir, "-protect", "zero"}, 2, "want a revision number"},
		"protect below one":    {[]string{"-pack", dir, "-protect", "0"}, 2, "want a revision number"},
		"unknown flag":         {[]string{"-nope"}, 2, ""},
		"keep none":            {[]string{"-pack", dir, "-keep-last", "0"}, 1, "keep at least"},
		"unreadable split":     {[]string{"-pack", dir, "-split", bad}, 1, "parse split"},
		"missing pack":         {[]string{"-pack", filepath.Join(dir, "missing")}, 1, "pack manifest"},
		"unwritable json":      {[]string{"-pack", dir, "-json", filepath.Join(dir, "no", "such", "dir", "p.json")}, 1, "write"},
	}
	for name, c := range cases {
		code, _, errs := runTool(c.args...)
		if code != c.code || !strings.Contains(errs, c.want) {
			t.Fatalf("%s: exit %d, stderr %q (want %d, %q)", name, code, errs, c.code, c.want)
		}
	}
	if code, _, _ := runTool("-h"); code != 0 {
		t.Fatalf("-h exits %d", code)
	}
}

func TestPrintHelpers(t *testing.T) {
	for in, want := range map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 kB", 1500: "1.5 kB", 38_000_000_000: "38.0 GB", 25_555_183: "25.6 MB"} {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
	var l listFlag
	l.Set("a")
	l.Set("b")
	if l.String() != "a,b" {
		t.Fatalf("listFlag %q", l.String())
	}
}
