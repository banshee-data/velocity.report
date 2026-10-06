package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

const goodSum = "2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a"

func runTool(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func newPack(t *testing.T, o evalfixture.NearFaceOptions) (string, string) {
	t.Helper()
	dir := t.TempDir()
	f, err := evalfixture.WriteNearFacePack(dir, o)
	if err != nil {
		t.Fatal(err)
	}
	return f.PackDir, dir
}

func TestDraftsAManifestFromWhereTheWarmUpEnds(t *testing.T) {
	pack, dir := newPack(t, evalfixture.NearFaceOptions{})
	out := filepath.Join(dir, "draft.json")
	code, stdout, stderr := runTool("-pack", pack, "-output", out, "-from-seconds", "0.2", "-note", "after warm-up", "-split", "tune", "-episode", "late")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"annotation revision 1, samples 2 to 5 of 6", "included 1 objects, 4 scored masks", "obj_car (car): 4 masks, samples 2 to 5", "pins annotation revision 1"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	m, err := annotation.LoadSplitManifest(out)
	if err != nil {
		t.Fatal(err)
	}
	if m.Episodes[0].EpisodeID != "late" || m.Episodes[0].FrameIntervals[0].FirstSample != 2 || m.Note != "after warm-up" || m.Splits[0].Name != "tune" {
		t.Fatalf("manifest %+v", m)
	}
	// The evaluator's own loader binds it to the pack.
	p, _ := annotation.OpenPack(pack)
	sc, _ := annotation.LoadSidecar(p)
	if err := m.ValidateAgainst(p, sc); err != nil {
		t.Fatal(err)
	}
	// It is never overwritten.
	if code, _, stderr := runTool("-pack", pack, "-output", out); code != 2 || !strings.Contains(stderr, "never overwritten") {
		t.Fatalf("overwrite: exit %d: %s", code, stderr)
	}
}

func TestDraftsFromWhereAnEvidenceDatabaseStarts(t *testing.T) {
	pack, dir := newPack(t, evalfixture.NearFaceOptions{})
	db := filepath.Join(dir, "e.db")
	// The first row is 0.3 ms after sample 0, so the first sample at or after it is 1.
	if err := evalfixture.WriteNearFaceDB(db, map[string]evalfixture.NearFaceBody{"only": {}}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "m.json")
	code, stdout, stderr := runTool("-pack", pack, "-output", out, "-from-evidence", db)
	if code != 0 || !strings.Contains(stdout, "samples 1 to 5 of 6") {
		t.Fatalf("exit %d: %s\n%s", code, stderr, stdout)
	}
	// An absent database, and two ways to say where to start.
	if code, _, stderr := runTool("-pack", pack, "-output", filepath.Join(dir, "m2.json"), "-from-evidence", filepath.Join(dir, "absent.db")); code != 1 || stderr == "" {
		t.Fatalf("absent: exit %d: %s", code, stderr)
	}
	if code, _, stderr := runTool("-pack", pack, "-output", filepath.Join(dir, "m3.json"), "-from-evidence", db, "-from-sample", "2"); code != 2 || !strings.Contains(stderr, "use one") {
		t.Fatalf("two starts: exit %d: %s", code, stderr)
	}
}

func TestWritesAFreezeDraftWithTheCaseAndItsCapture(t *testing.T) {
	pack, dir := newPack(t, evalfixture.NearFaceOptions{})
	out, draftPath := filepath.Join(dir, "m.json"), filepath.Join(dir, "freeze.json")
	code, stdout, stderr := runTool("-pack", pack, "-output", out, "-from-sample", "1", "-to-sample", "4",
		"-freeze-draft", draftPath, "-case", "kirk0", "-capture", "kirk0.pcapng=sha256:"+goodSum, "-capture", "other.pcapng="+goodSum)
	if code != 0 || strings.Contains(stderr, "warning") || !strings.Contains(stdout, "annotation-split freeze") {
		t.Fatalf("exit %d: %s\n%s", code, stderr, stdout)
	}
	var d annotation.SplitDraft
	b, _ := os.ReadFile(draftPath)
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if d.Schema != annotation.SplitDraftSchema || len(d.Cases) != 1 || d.Cases[0].CaseID != "kirk0" || d.Cases[0].Role != annotation.SplitRoleTuning ||
		len(d.Cases[0].Captures) != 2 || d.Cases[0].Captures[1].SHA256 != "sha256:"+goodSum ||
		len(d.Packs) != 1 || d.Packs[0].CaseID != "kirk0" || !filepath.IsAbs(d.Packs[0].Dir) || !filepath.IsAbs(d.Packs[0].SplitManifest) {
		t.Fatalf("draft %+v", d)
	}
	m, _ := annotation.LoadSplitManifest(out)
	if m.Episodes[0].FrameIntervals[0] != (annotation.FrameInterval{FirstSample: 1, LastSample: 4}) {
		t.Fatalf("interval %+v", m.Episodes[0].FrameIntervals)
	}
	// A draft with no capture warns, and the case role can differ from the split's.
	out2, draft2 := filepath.Join(dir, "m2.json"), filepath.Join(dir, "freeze2.json")
	code, _, stderr = runTool("-pack", pack, "-output", out2, "-freeze-draft", draft2, "-case", "kirk0", "-case-role", "screen")
	if code != 0 || !strings.Contains(stderr, "warning: no -capture") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	m3 := filepath.Join(dir, "m3.json")
	if code, _, stderr := runTool("-pack", pack, "-output", m3, "-freeze-draft", draftPath, "-case", "kirk0"); code != 1 || !strings.Contains(stderr, "never overwritten") {
		t.Fatalf("draft overwrite: exit %d: %s", code, stderr)
	}
	// The refusal comes before anything is written: no manifest is left to clear before a re-run.
	if _, err := os.Lstat(m3); err == nil {
		t.Fatalf("%s was written although the draft path was taken", m3)
	}
}

func TestUsageAndFailures(t *testing.T) {
	pack, dir := newPack(t, evalfixture.NearFaceOptions{})
	out := filepath.Join(dir, "o.json")
	cases := map[string]struct {
		args []string
		code int
		want string
	}{
		"no pack":              {[]string{"-output", out}, 2, "-pack and -output"},
		"no output":            {[]string{"-pack", pack}, 2, "-pack and -output"},
		"both starts":          {[]string{"-pack", pack, "-output", out, "-from-sample", "1", "-from-seconds", "1"}, 2, "use one"},
		"freeze without case":  {[]string{"-pack", pack, "-output", out, "-freeze-draft", filepath.Join(dir, "f.json")}, 2, "needs -case"},
		"case without freeze":  {[]string{"-pack", pack, "-output", out, "-case", "k"}, 2, "are for -freeze-draft"},
		"capture without it":   {[]string{"-pack", pack, "-output", out, "-capture", "a=" + goodSum}, 2, "are for -freeze-draft"},
		"stray argument":       {[]string{"-pack", pack, "-output", out, "x"}, 2, "unexpected arguments"},
		"unknown flag":         {[]string{"-nope"}, 2, ""},
		"bad capture":          {[]string{"-pack", pack, "-output", out, "-freeze-draft", filepath.Join(dir, "f.json"), "-case", "k", "-capture", "nodigest"}, 2, "want basename=sha256"},
		"short digest":         {[]string{"-pack", pack, "-output", out, "-freeze-draft", filepath.Join(dir, "f.json"), "-case", "k", "-capture", "a=abc"}, 2, "64-digit"},
		"missing pack":         {[]string{"-pack", filepath.Join(dir, "missing"), "-output", out}, 1, "open pack"},
		"warm-up past the end": {[]string{"-pack", pack, "-output", out, "-from-seconds", "100"}, 1, "ends"},
		"bad role":             {[]string{"-pack", pack, "-output", out, "-role", "screen"}, 1, "split role"},
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
	// An unwritable output is reported.
	if code, _, stderr := runTool("-pack", pack, "-output", filepath.Join(dir, "no", "such", "x.json")); code != 1 || !strings.Contains(stderr, "write") {
		t.Fatalf("unwritable: exit %d: %s", code, stderr)
	}
	// A pack with nothing reviewed reports what it left out.
	proposed, _ := newPack(t, evalfixture.NearFaceOptions{Proposed: true})
	code, stdout, stderr := runTool("-pack", proposed, "-output", filepath.Join(dir, "p.json"))
	if code != 1 || !strings.Contains(stderr, "no object qualifies") || !strings.Contains(stdout, "obj_car (car): object_not_reviewed") {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	var l listFlag
	l.Set("a")
	l.Set("b")
	if l.String() != "a,b" {
		t.Fatalf("listFlag %q", l.String())
	}
}
