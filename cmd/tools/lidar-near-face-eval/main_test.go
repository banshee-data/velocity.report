package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

func runTool(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

type scene struct {
	pack, split, db string
}

func newScene(t *testing.T) scene {
	t.Helper()
	dir := t.TempDir()
	f, err := evalfixture.WriteNearFacePack(dir, evalfixture.NearFaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "e.db")
	if err := evalfixture.WriteNearFaceDB(db, map[string]evalfixture.NearFaceBody{
		"exact": {}, "sensorward": {DX: -0.3},
	}); err != nil {
		t.Fatal(err)
	}
	return scene{pack: f.PackDir, split: f.SplitManifestPath, db: db}
}

func (s scene) args(extra ...string) []string {
	base := []string{"-pack", s.pack, "-split-manifest", s.split, "-split", evalfixture.NearFaceSplit, "-allow-tuning-split",
		"-param-hash", "", "-arm", "exact=" + s.db}
	return append(base, extra...)
}

func TestScoresTwoArmsAndWritesBothCopies(t *testing.T) {
	s := newScene(t)
	dir := t.TempDir()
	jsonPath, mdPath := filepath.Join(dir, "faces.json"), filepath.Join(dir, "faces.md")
	// One database holds both versions, so each arm names its own parameter
	// hash; the shared flag is for databases that hold one.
	for _, tc := range []struct{ label, params string }{{"exact", "exact"}, {"sensorward", "sensorward"}} {
		code, _, errs := runTool("-pack", s.pack, "-split-manifest", s.split, "-split", evalfixture.NearFaceSplit, "-allow-tuning-split",
			"-arm", tc.label+"="+s.db, "-param-hash", tc.params, "-json", filepath.Join(dir, tc.label+".json"))
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", tc.label, code, errs)
		}
	}
	code, out, errs := runTool("-pack", s.pack, "-split-manifest", s.split, "-split", evalfixture.NearFaceSplit, "-allow-tuning-split",
		"-arm", "sensorward="+s.db, "-param-hash", "sensorward", "-instants", "-episodes", "ep, ",
		"-frame-tolerance-ms", "10", "-json", jsonPath, "-markdown", mdPath)
	if code != 0 || out != "" {
		t.Fatalf("exit %d, stdout %q: %s", code, out, errs)
	}
	for _, want := range []string{"labels: pack", "sensorward: 6 labelled, 6 matched, 6 scored", "end face mean +0.300 m", "caveat:"} {
		if !strings.Contains(errs, want) {
			t.Fatalf("stderr lacks %q:\n%s", want, errs)
		}
	}
	var report perframeeval.NearFaceReport
	b, _ := os.ReadFile(jsonPath)
	if err := json.Unmarshal(b, &report); err != nil || len(report.Arms) != 1 || len(report.Arms[0].Instants) != 6 {
		t.Fatalf("report %v: %s", err, b)
	}
	md, _ := os.ReadFile(mdPath)
	if !strings.Contains(string(md), "# Near-face residuals") {
		t.Fatalf("markdown %s", md)
	}
}

func TestPrintsTheReportToStdoutWithoutAJSONPath(t *testing.T) {
	s := newScene(t)
	code, out, errs := runTool(s.args("-param-hash", "exact", "-include-proposed", "-ignore-partial-masks")...)
	// Every mask is partial and ignored, so nothing is scored: refused, with the reason.
	if code != 1 || !strings.Contains(errs, "no scored mask") {
		t.Fatalf("exit %d, stdout %q, stderr %s", code, out, errs)
	}
	code, out, errs = runTool(s.args("-param-hash", "exact")...)
	if code != 0 || !strings.Contains(out, `"schema": "velocity.report/near-face-comparison"`) {
		t.Fatalf("exit %d, stdout %q, stderr %s", code, out, errs)
	}
}

func TestUsageAndFailures(t *testing.T) {
	s := newScene(t)
	cases := map[string]struct {
		args []string
		code int
		want string
	}{
		"no pack":         {[]string{"-split-manifest", s.split, "-split", "tune", "-arm", "a=" + s.db}, 2, "-pack is required"},
		"no manifest":     {[]string{"-pack", s.pack, "-split", "tune", "-arm", "a=" + s.db}, 2, "-split-manifest is required"},
		"no split":        {[]string{"-pack", s.pack, "-split-manifest", s.split, "-arm", "a=" + s.db}, 2, "-split is required"},
		"no arm":          {[]string{"-pack", s.pack, "-split-manifest", s.split, "-split", "tune"}, 2, "-arm is required"},
		"malformed arm":   {append(s.args(), "-arm", "nolabel"), 2, "want label=database"},
		"empty label":     {append(s.args(), "-arm", "=x.db"), 2, "want label=database"},
		"duplicate label": {append(s.args(), "-arm", "exact=other.db"), 2, "used twice"},
		"stray argument":  {append(s.args(), "extra"), 2, "unexpected arguments"},
		"unknown flag":    {[]string{"-nope"}, 2, ""},
		"bad option":      {s.args("-min-returns", "1"), 1, "min returns"},
		"unwritable json": {s.args("-param-hash", "exact", "-json", filepath.Join(s.pack, "no", "dir", "x.json")), 1, "write"},
		"unwritable md":   {s.args("-param-hash", "exact", "-json", filepath.Join(t.TempDir(), "x.json"), "-markdown", filepath.Join(s.pack, "no", "dir", "x.md")), 1, "write"},
		"missing db":      {[]string{"-pack", s.pack, "-split-manifest", s.split, "-split", "tune", "-allow-tuning-split", "-arm", "a=" + filepath.Join(t.TempDir(), "absent.db")}, 1, "arm a"},
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
	var l listFlag
	l.Set("a")
	l.Set("b")
	if l.String() != "a,b" {
		t.Fatalf("listFlag %q", l.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

func TestReportsAFailureToWriteTheReport(t *testing.T) {
	s := newScene(t)
	var errb bytes.Buffer
	if code := run(s.args("-param-hash", "exact"), failingWriter{}, &errb); code != 1 || !strings.Contains(errb.String(), "write report") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}
