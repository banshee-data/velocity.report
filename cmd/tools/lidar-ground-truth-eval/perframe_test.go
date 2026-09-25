package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// The command end to end, against the synthetic fixture: a pack, its review
// sidecar, a split manifest and an evidence database written by evalfixture,
// whose scene and hand counts are described there and in perframeeval's tests.

func perFrameFixture(t *testing.T) *evalfixture.Fixture {
	t.Helper()
	f, err := evalfixture.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func perFrameArgs(f *evalfixture.Fixture, extra ...string) []string {
	return append([]string{
		"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.SplitHeldOut,
		"-a-db", f.DBPath, "-a-param-hash", evalfixture.ParamsA,
		"-b-db", f.DBPath, "-b-param-hash", evalfixture.ParamsB,
	}, extra...)
}

func runPerFrameCapture(t *testing.T, args []string) (int, perframeeval.Comparison, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runPerFrame(args, &stdout, &stderr)
	var c perframeeval.Comparison
	if code == 0 {
		if err := json.Unmarshal(stdout.Bytes(), &c); err != nil {
			t.Fatalf("output is not a comparison: %v\n%s", err, stdout.String())
		}
	}
	return code, c, stderr.String()
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Reviewed-only held-out scoring, and the known identity-switch and
// fragmentation scenario: arm B loses the held-out car for one scored frame
// and resumes it under a new identity.
func TestPerFrameKnownScenario(t *testing.T) {
	f := perFrameFixture(t)
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "comparison.md")
	jsonPath := filepath.Join(dir, "comparison.json")
	var stdout, stderr bytes.Buffer
	if code := runPerFrame(perFrameArgs(f, "-json", jsonPath, "-markdown", mdPath), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var c perframeeval.Comparison
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}

	if c.Schema != perframeeval.ComparisonSchema || !c.Reference.HeldOut || c.Reference.Policy.Status != "reviewed_only" ||
		c.Reference.PackDigest != f.PackDigest || c.Gate.Kind != "footprint" || c.Gate.Metres != 1 ||
		c.FrameToleranceNanos != 10_000_000 {
		t.Fatalf("comparison header %+v / gate %+v", c.Reference, c.Gate)
	}
	a, b := c.Total.A, c.Total.B
	if a.NumGT != 9 || a.MOTA != 1 || a.IDSwitches != 0 || a.Fragmentations != 0 || !approx(a.IDF1, 1) {
		t.Fatalf("arm A %+v", a)
	}
	// MOTA = 1 - (FN 1 + FP 2 + IDSW 1) / 9; IDF1 = 2*4 / (2*4 + 6 + 5).
	if b.NumGT != 9 || b.FN != 1 || b.FP != 2 || b.IDSwitches != 1 || b.Fragmentations != 1 ||
		!approx(b.MOTA, 5.0/9) || !approx(b.IDF1, 8.0/19) || !approx(b.HOTA, math.Sqrt(4.0/11)) {
		t.Fatalf("arm B %+v", b)
	}
	if c.Total.Delta.IDSwitches != 1 || c.Total.Delta.Fragmentations != 1 {
		t.Fatalf("delta %+v", c.Total.Delta)
	}
	if c.A.Arm.Stage != "final" || c.A.Arm.DeclaredBaseline || c.A.Arm.ParamHash != evalfixture.ParamsA {
		t.Fatalf("arm A identity %+v", c.A.Arm)
	}
	// Ignore mapping, as the reference recorded it: the occluded frame, the
	// proposed pedestrian, and the two objects the episode does not score.
	ignored := c.A.Episodes[0].Reference.IgnoredByReason
	if ignored["visibility"] != 1 || ignored["unreviewed"] != 10 || ignored["outside_episode"] != 20 {
		t.Fatalf("ignored %+v", ignored)
	}

	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "| ID switches | 0 | 1 | +1 |") || !strings.Contains(string(md), "## Caveats") {
		t.Fatalf("markdown:\n%s", md)
	}
	if !strings.Contains(stderr.String(), "held_out=true") || !strings.Contains(stderr.String(), "B: MOTA 0.5556  IDSW 1  FM 1") {
		t.Fatalf("summary on stderr:\n%s", stderr.String())
	}
}

func TestPerFrameIncludeProposedIsRecorded(t *testing.T) {
	f := perFrameFixture(t)
	code, c, stderr := runPerFrameCapture(t, perFrameArgs(f, "-include-proposed"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if c.Reference.Policy.Status != "reviewed_and_proposed" || c.Total.A.NumGT != 19 {
		t.Fatalf("policy %q, reference points %d; want reviewed_and_proposed and 19", c.Reference.Policy.Status, c.Total.A.NumGT)
	}
}

func TestPerFrameRefusesTheTuningSplit(t *testing.T) {
	f := perFrameFixture(t)
	args := perFrameArgs(f)
	for i := range args {
		if args[i] == evalfixture.SplitHeldOut {
			args[i] = evalfixture.SplitTuning
		}
	}
	code, _, stderr := runPerFrameCapture(t, args)
	if code != 1 || !strings.Contains(stderr, "split is not held out") {
		t.Fatalf("exit %d, stderr %q; want a held-out refusal", code, stderr)
	}
	code, c, stderr := runPerFrameCapture(t, append(args, "-allow-tuning-split"))
	if code != 0 || c.Reference.HeldOut || !strings.Contains(stderr, "not a held-out result") {
		t.Fatalf("exit %d, held_out %v, stderr %q", code, c.Reference.HeldOut, stderr)
	}
}

func TestPerFrameRefusesAManifestForAnotherPack(t *testing.T) {
	f := perFrameFixture(t)
	m := f.Manifest()
	m.PackDigest = "sha256:" + strings.Repeat("f", 64)
	path := filepath.Join(t.TempDir(), "other.json")
	if err := evalfixture.WriteSplitManifest(path, m); err != nil {
		t.Fatal(err)
	}
	args := perFrameArgs(f)
	for i := range args {
		if args[i] == f.SplitManifestPath {
			args[i] = path
		}
	}
	code, _, stderr := runPerFrameCapture(t, args)
	if code != 1 || !strings.Contains(stderr, "frozen against pack") {
		t.Fatalf("exit %d, stderr %q; want a digest refusal", code, stderr)
	}
}

func TestPerFrameRequiresFinalEstimatesUnlessDeclared(t *testing.T) {
	f := perFrameFixture(t)
	code, _, stderr := runPerFrameCapture(t, perFrameArgs(f, "-a-stage", "online"))
	if code != 1 || !strings.Contains(stderr, "declare the arm a baseline") {
		t.Fatalf("exit %d, stderr %q; want a stage refusal", code, stderr)
	}
	code, c, stderr := runPerFrameCapture(t, perFrameArgs(f, "-a-stage", "online", "-a-declared-baseline"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if c.A.Arm.Stage != "online" || !c.A.Arm.DeclaredBaseline || !strings.Contains(strings.Join(c.Caveats, " "), "declared baseline") {
		t.Fatalf("arm A %+v, caveats %q", c.A.Arm, c.Caveats)
	}
}

func TestPerFrameAnalysisRuns(t *testing.T) {
	f := perFrameFixture(t)
	args := []string{
		"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.SplitHeldOut,
		"-a-db", f.DBPath, "-a-run-id", evalfixture.RunA, "-a-declared-baseline",
		"-b-db", f.DBPath, "-b-run-id", evalfixture.RunB, "-b-declared-baseline",
	}
	code, c, stderr := runPerFrameCapture(t, args)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if c.A.Arm.Kind != "analysis_run" || c.Total.B.IDSwitches != 1 || c.Total.B.Fragmentations != 1 {
		t.Fatalf("run comparison %+v / %+v", c.A.Arm, c.Total.B)
	}
}

func TestPerFrameUsageErrors(t *testing.T) {
	f := perFrameFixture(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"nothing", nil},
		{"no split", []string{"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-a-db", f.DBPath, "-b-db", f.DBPath}},
		{"unknown position", perFrameArgs(f, "-reference-position", "pose")},
		{"unknown gate", perFrameArgs(f, "-gate", "elliptical")},
		{"zero fixed gate", perFrameArgs(f, "-gate", "fixed", "-gate-metres", "0")},
		{"negative tolerance", perFrameArgs(f, "-frame-tolerance-ms", "-1")},
		{"stray argument", perFrameArgs(f, "extra")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code, _, _ := runPerFrameCapture(t, tc.args); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	if code := runPerFrame([]string{"-h"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "-a-declared-baseline") {
		t.Fatalf("help: exit %d\n%s", code, stderr.String())
	}
}

// The subcommand is reached through run, and the default mode is unchanged.
func TestRunDispatchesPerFrame(t *testing.T) {
	if code := run([]string{"perframe"}); code != 2 {
		t.Fatalf("run perframe with no flags = %d, want 2", code)
	}
}
