//go:build pcap && !race

package main

// Excluded from race builds for time, as the replayeval continuity test is:
// two kirk0 replays. main registers its flags on the process's flag set, so
// it runs once per test binary, and only here.

import (
	"encoding/json"
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

// The tool end to end on kirk0 as a one-case corpus, under a frozen split
// that gives kirk0 the tuning role: the summary and the replay manifest both
// name the split, and the case's role.
func TestMainRunsKirk0UnderAFrozenSplit(t *testing.T) {
	requireKirk0(t)
	dir := t.TempDir()
	split, splitPath := writeCaseSplit(t, annotation.SplitCase{CaseID: "kirk0", Role: annotation.SplitRoleTuning})
	out := filepath.Join(dir, "out")
	args := []string{
		"lidar-state-estimation-baseline",
		"-corpus", "testdata/kirk0-corpus.json", "-index", "testdata/kirk0-index.json",
		"-pcap-root", "../../../internal/lidar/perf", "-pcap-subdir", "pcap", "-warmup", "20", "-duration", "2",
		"-source-manifest", filepath.Join(dir, "source-manifest.json"), "-out", out,
		"-evidence-dir", filepath.Join(dir, "evidence"), "-evidence-per-case", "-discard-evidence",
		"-split-manifest", splitPath,
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
}
