//go:build pcap
// +build pcap

package lidar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/analysis"
)

func TestTrackingBaselineDisplayAndBandLabels(t *testing.T) {
	dir := t.TempDir()
	silence(t, func() int { printTrackingBaseline(dir); return 0 })
	for _, data := range []string{
		"invalid", `{}`,
		`{"schema_version":2,"population":"scoring_window_including_terminated_tracks","residual_bands":[{"speed_floor_mps":0,"count":1},{"speed_floor_mps":15,"count":2,"decomposed":2}],"association_bands":[{"speed_floor_mps":15,"matched":2,"missed":1,"rate":0.6667}]}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, "tracking_baseline.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		silence(t, func() int { printTrackingBaseline(dir); return 0 })
	}
	for floor, want := range map[float32]string{0: "0-2", 2: "2-5", 5: "5-10", 10: "10-15", 15: "15+", 7: "7"} {
		if got := bandLabel(floor); got != want {
			t.Fatalf("%g: %s, want %s", floor, got, want)
		}
	}
}

func TestReplayEvalFlagHandling(t *testing.T) {
	cases := map[string]struct {
		args []string
		want int
	}{
		"help":       {[]string{"-h"}, 0},
		"bad flag":   {[]string{"-nope"}, 2},
		"no pcap":    {[]string{"--output", t.TempDir()}, 2},
		"no output":  {[]string{"--pcap", "capture.pcap"}, 2},
		"no capture": {[]string{"--pcap", filepath.Join(t.TempDir(), "absent.pcap"), "--output", filepath.Join(t.TempDir(), "o")}, 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if code := silence(t, func() int { return ReplayEvalMain(c.args) }); code != c.want {
				t.Errorf("exited %d, want %d", code, c.want)
			}
		})
	}
}

// An invalid replay window is refused before any work is done, so a mistyped
// flag does not cost a full pass over the capture.
func TestReplayEvalRejectsAnImpossibleWindow(t *testing.T) {
	src := truncatedCapture(t, 400)
	code := silence(t, func() int {
		return ReplayEvalMain([]string{
			"--pcap", src, "--output", filepath.Join(t.TempDir(), "out"),
			"--start-seconds", "1", "--warmup-seconds", "5",
		})
	})
	if code != 1 {
		t.Fatalf("exited %d, want 1: warm-up cannot precede the capture", code)
	}
}

func TestReplayEvalRecordsAndAnalyses(t *testing.T) {
	src := truncatedCapture(t, 4000)
	out := filepath.Join(t.TempDir(), "run")

	code := silence(t, func() int {
		return ReplayEvalMain([]string{
			"--pcap", src, "--output", out, "--progress-frames", "0",
			"--include-debug",
		})
	})
	if code != 0 {
		t.Fatalf("replay exited %d, want 0", code)
	}
	if _, _, err := analysis.GenerateReport(out); err != nil {
		t.Fatalf("the run did not produce an analysable recording: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "replay_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		IncludeDebug bool `json:"include_debug"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil || !manifest.IncludeDebug {
		t.Fatalf("debug flag lost: %v", err)
	}

	// A second run compared against the first exercises the comparison path.
	second := filepath.Join(t.TempDir(), "run2")
	code = silence(t, func() int {
		return ReplayEvalMain([]string{
			"--pcap", src, "--output", second, "--progress-frames", "0",
			"--compare-to", out,
		})
	})
	if code != 0 {
		t.Fatalf("compared replay exited %d, want 0", code)
	}
}

func TestReplayEvalReportsAMissingBaseline(t *testing.T) {
	src := truncatedCapture(t, 4000)
	code := silence(t, func() int {
		return ReplayEvalMain([]string{
			"--pcap", src, "--output", filepath.Join(t.TempDir(), "run"),
			"--progress-frames", "0", "--compare-to", filepath.Join(t.TempDir(), "absent"),
		})
	})
	if code != 1 {
		t.Fatalf("exited %d, want 1", code)
	}
}

func TestReplayEvalSkipsAnalysisWhenAsked(t *testing.T) {
	src := truncatedCapture(t, 4000)
	code := silence(t, func() int {
		return ReplayEvalMain([]string{
			"--pcap", src, "--output", filepath.Join(t.TempDir(), "run"),
			"--progress-frames", "0", "--analyse=false",
		})
	})
	if code != 0 {
		t.Fatalf("exited %d, want 0", code)
	}
}

// The summary is what a person reads to judge a change, so its formatting is
// exercised directly rather than only through a full replay.
func TestPrintReplaySummaryHandlesMissingEvidence(t *testing.T) {
	p50 := 42.3
	full := &analysis.AnalysisReport{
		FrameSummary: analysis.FrameSummary{
			CoLocation: &analysis.CoLocationSummary{ScoredFrames: 200, PairFrames: 93, OverlapFrames: 68},
		},
		TrackSummary: analysis.TrackSummary{
			TotalTracks: 41, ConfirmedTracks: 4,
			Alignment: &analysis.AlignmentSummary{
				CourseAlignmentTracks: 11,
				CourseAlignmentP50Deg: &analysis.DistStats{P50: &p50, Avg: 39.8, Max: 78.1},
			},
			HeadingLock: &analysis.HeadingLockSummary{
				AcceptedFrames: 948, HeldFrames: 259, AcceptanceRatio: 0.785,
				SourceFrames: map[string]int{"locked": 259},
				Tracks:       23,
			},
		},
	}
	silence(t, func() int { printReplaySummary(full); return 0 })

	// A report with no alignment or lock evidence must still print rather than
	// dereference its way into a panic.
	silence(t, func() int { printReplaySummary(&analysis.AnalysisReport{}); return 0 })
}

// The breakdown partitions the held frames. A source missing from the table
// would make the percentages quietly wrong, so it warns instead.
func TestPrintAbstentionReasonsWarnsOnAShortfall(t *testing.T) {
	silence(t, func() int {
		printAbstentionReasons(map[string]int{"locked": 10}, 10)
		printAbstentionReasons(map[string]int{"locked": 4}, 10) // 6 unaccounted
		printAbstentionReasons(nil, 0)                          // nothing held
		return 0
	})
}

func TestDerefFloat(t *testing.T) {
	v := 1.5
	if got := derefFloat(&v); got != 1.5 {
		t.Fatalf("got %v", got)
	}
	if got := derefFloat(nil); got != 0 {
		t.Fatalf("nil deref = %v, want 0", got)
	}
}
