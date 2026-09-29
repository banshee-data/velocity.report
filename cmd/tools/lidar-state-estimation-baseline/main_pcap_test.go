//go:build pcap && !race

package main

// Excluded from race builds for time, as the replayeval continuity test is:
// two whole kirk0 replays, about a minute without the race detector. main
// registers its flags on the process's flag set, so it runs once per test
// binary, and only here.

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// kirk0Capture is the in-repo reference capture, relative to this package.
const kirk0Capture = "../../../internal/lidar/perf/pcap/kirk0.pcapng"

// requireKirk0 skips when the capture is absent or still a Git LFS pointer.
func requireKirk0(t *testing.T) {
	t.Helper()
	info, err := os.Stat(kirk0Capture)
	if err != nil {
		t.Skipf("reference capture not available: %v", err)
	}
	if info.Size() < 1<<20 {
		t.Skipf("reference capture is a %d-byte LFS pointer; run git lfs pull", info.Size())
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// committedCoverage is the declaration set the corpus tool loads, which
// holds kirk0's surveyed declaration.
const committedCoverage = "../../../tools/s2-archive/continuity-coverage.json"

// The tool end to end on kirk0 as a one-case corpus, the whole capture after
// a 20 s warm-up, as kirk0's coverage was first read by hand:
//
//   - under a frozen split that gives kirk0 the tuning role, the summary and
//     the replay manifest both name the split and the case's role;
//   - the coverage survey reproduces the hand-made declaration's range, the
//     online estimates' 91.7 m maximum (p90 42.8 m) rounded up to 92 m, and
//     the committed declaration, field for field.
func TestMainRunsKirk0UnderAFrozenSplit(t *testing.T) {
	requireKirk0(t)
	dir := t.TempDir()
	kirk0SHA, err := fileSHA256(kirk0Capture)
	if err != nil {
		t.Fatal(err)
	}
	split, splitPath := writeCaseSplit(t, annotation.SplitCase{CaseID: "kirk0", Role: annotation.SplitRoleTuning,
		Captures: []annotation.CaseCapture{{Basename: "kirk0.pcapng", SHA256: kirk0SHA}}})
	out := filepath.Join(dir, "out")
	surveyPath := filepath.Join(dir, "continuity-coverage.json")
	args := []string{
		"lidar-state-estimation-baseline",
		"-corpus", "testdata/kirk0-corpus.json", "-index", "testdata/kirk0-index.json",
		"-pcap-root", "../../../internal/lidar/perf", "-pcap-subdir", "pcap", "-warmup", "20",
		"-source-manifest", filepath.Join(dir, "source-manifest.json"), "-out", out,
		"-evidence-dir", filepath.Join(dir, "evidence"), "-evidence-per-case", "-discard-evidence",
		"-split-manifest", splitPath, "-survey-coverage", surveyPath,
	}
	saved := os.Args
	os.Args = args
	defer func() { os.Args = saved }()
	main()

	var summary struct {
		Split *splitRecord  `json:"split"`
		Cases []caseSummary `json:"cases"`
	}
	readJSON(t, filepath.Join(out, "phase0-summary.json"), &summary)
	if summary.Split == nil || summary.Split.Digest != split.SplitDigest || summary.Split.HeldOut {
		t.Fatalf("summary split %+v, want %s", summary.Split, split.SplitDigest)
	}
	if len(summary.Cases) != 1 || summary.Cases[0].SplitRole != "tuning" || !summary.Cases[0].BaselineEqual {
		t.Fatalf("summary cases %+v", summary.Cases)
	}
	var manifest struct {
		Split *replayeval.SplitUse `json:"split"`
	}
	readJSON(t, filepath.Join(out, "kirk0", "first", "replay_manifest.json"), &manifest)
	want := replayeval.SplitUse{SplitDigest: split.SplitDigest, Revision: 1, CaseID: "kirk0", Role: "tuning"}
	if manifest.Split == nil || *manifest.Split != want {
		t.Fatalf("replay manifest split %+v, want %+v", manifest.Split, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "evidence", "kirk0.db")); !os.IsNotExist(err) {
		t.Fatalf("-discard-evidence left the case database: %v", err)
	}

	surveyed := summary.Cases[0].CoverageSurvey
	if surveyed == nil {
		t.Fatal("the summary carries no coverage survey")
	}
	d, st := surveyed.Declaration, surveyed.Stats
	t.Logf("kirk0 survey: %+v; range %+v; azimuth %+v", d, st.RangeMetres, st.Azimuth)
	if math.Round(st.RangeMetres.Max*10)/10 != 91.7 || math.Round(st.RangeMetres.P90*10)/10 != 42.8 || d.MaxRangeMetres != 92 {
		t.Fatalf("range max %.2f m, p90 %.2f m, declared %g m; the hand-made declaration read 91.7 m (p90 42.8 m) and declared 92 m",
			st.RangeMetres.Max, st.RangeMetres.P90, d.MaxRangeMetres)
	}
	set, err := replayeval.LoadContinuityCoverageSet(surveyPath)
	if err != nil || set["kirk0"] != d {
		t.Fatalf("surveyed set %+v, %v; want the summary's declaration", set, err)
	}
	committed, err := replayeval.LoadContinuityCoverageSet(committedCoverage)
	if err != nil {
		t.Fatal(err)
	}
	wantCoverage := committed["kirk0"]
	if !st.BuildStamped {
		// The committed declaration was surveyed by an unstamped build, as
		// this test's is, so even its source is reproduced.
		if d != wantCoverage {
			t.Fatalf("surveyed %+v\ncommitted %+v", d, wantCoverage)
		}
	}
	d.Source, wantCoverage.Source = "", ""
	if d != wantCoverage {
		t.Fatalf("surveyed %+v\ncommitted %+v", d, wantCoverage)
	}
}
