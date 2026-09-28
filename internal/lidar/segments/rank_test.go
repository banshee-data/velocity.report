package segments

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

// A standard selector is its finder, exactly. The golden file is what the
// finders return on their own, so any difference is a window whose identity,
// score or place in the ranking a selector has moved.
func TestStandardSelectorsRankExactlyAsTheirFinders(t *testing.T) {
	c := shippedCatalogue(t)
	points, captures := goldenSeries()
	got := map[string][]Window{}
	for name, run := range goldenRuns(captures) {
		windows, err := Rank(points, selectorFrom(t, c, run.finder), "golden", run.role, run.captures)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got[name] = windows
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(goldenBytes(t, got), want) {
		t.Fatalf("a standard selector ranks differently from its finder; compare with %s", goldenPath)
	}
}

// The table says which finders compute each measure. Checked against the
// finders themselves: a measure a finder is said to compute is not zero in
// every window, and one it is not said to compute is zero in all of them.
func TestTheMeasureTableSaysWhatEachFinderComputes(t *testing.T) {
	points, captures := goldenSeries()
	for _, f := range Finders() {
		windows, err := Find(points, f.Name, "golden", "tuning", DefaultParams(), captures)
		if err != nil {
			t.Fatal(err)
		}
		for name, m := range measures {
			listed := slices.Contains(m.finders, f.Name)
			found := false
			for _, w := range windows {
				found = found || m.read(w) != 0
			}
			if listed && !found {
				t.Errorf("%s is said to compute %s, and it is zero in every window", f.Name, name)
			}
			// These two read the finder's own score, which every finder sets;
			// only the table decides which finder may be ranked by them.
			if !listed && found && name != "max_residual_m" && name != "draw" {
				t.Errorf("%s computes %s, and the table does not say so", f.Name, name)
			}
		}
		if err := (Selector{Finder: f.Name}).computes(nativeMeasure[f.Name]); err != nil {
			t.Errorf("%s's own measure: %v", f.Name, err)
		}
	}
}

func TestEveryMeasureReadsItsOwnField(t *testing.T) {
	w := Window{PairFrames: 1, PairSeconds: 2, Pairs: 3, Followers: 4, Leaders: 5, LeaderChanges: 6, ClosestGapM: 7,
		TrackIDs: []string{"a", "b", "c", "d", "e", "f", "g", "h"}, Events: 9, Score: 10}
	want := map[string]float64{"pair_frames": 1, "pair_seconds": 2, "pairs": 3, "followers": 4, "leaders": 5, "leader_changes": 6,
		"closest_gap_m": 7, "tracks": 8, "events": 9, "max_residual_m": 10, "draw": 10}
	if len(measures) != len(want) {
		t.Fatalf("%d measures, %d expected", len(measures), len(want))
	}
	for name, value := range want {
		if got := measures[name].read(w); got != value {
			t.Errorf("%s read %v, want %v", name, got, value)
		}
	}
}

func TestRequirementsAndOrderChooseTheWindows(t *testing.T) {
	c := shippedCatalogue(t)
	points, captures := goldenSeries()
	ranked := func(s Selector, points []Point, captures []Capture) []Window {
		t.Helper()
		windows, err := Rank(points, s, "golden", "tuning", captures)
		if err != nil {
			t.Fatal(err)
		}
		return windows
	}
	seconds := func(windows []Window) []float64 {
		out := []float64{}
		for _, w := range windows {
			out = append(out, float64(w.StartNs-testBase)/1e9, w.Score)
		}
		return out
	}
	closer := selectorFrom(t, c, "close_following")
	// Three seconds of following at 20 s, and exactly two at 0 s, which the
	// inclusive bound keeps.
	if got := seconds(ranked(closer, points, captures)); !slices.Equal(got, []float64{20, 3, 0, 2}) {
		t.Fatalf("close following: %v", got)
	}
	upper := 2.0
	higher := 2.5
	for name, tc := range map[string]struct {
		change func(*Selector)
		want   []float64
	}{
		"a higher minimum":  {func(s *Selector) { s.Require[0].Min = &higher }, []float64{20, 3}},
		"a maximum instead": {func(s *Selector) { s.Require[0].Min, s.Require[0].Max = nil, &upper }, []float64{0, 2}},
		"the least first":   {func(s *Selector) { s.Score.Order = "ascending" }, []float64{0, 2, 20, 3}},
	} {
		s := closer
		s.Require = []Requirement{{Measure: closer.Require[0].Measure, Min: closer.Require[0].Min, Max: closer.Require[0].Max}}
		tc.change(&s)
		if got := seconds(ranked(s, points, captures)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}

	// Among equal scores the earlier window comes first, whichever way the
	// measure is ordered.
	byTracks := Selector{ID: "moving_tracks", Label: "Moving tracks", Category: "Traffic", Description: "Tracks seen moving.",
		Finder: "exposure", Parameters: DefaultParams(), Score: Score{"tracks", "descending"}}
	tied := []Point{{Track: "a", TimeNs: testBase + 10_000_000_000, VX: 10}, {Track: "a", TimeNs: testBase, VX: 10}}
	for _, order := range []string{"descending", "ascending"} {
		s := byTracks
		s.Score.Order = order
		if got := ranked(s, tied, nil); len(got) != 2 || got[0].StartNs != testBase {
			t.Errorf("%s ties lost time order: %+v", order, got)
		}
	}

	// A bound reads the window as its finder scored it: the lateral jump's
	// residual, even though the window is then ranked by its track count.
	wide := Selector{ID: "wide_jumps", Label: "Wide jumps", Category: "Tracker failure", Description: "Jumps of a metre or more.",
		Finder: "lateral_jump", Parameters: DefaultParams(), Score: Score{"tracks", "descending"}}
	metre, half := 1.0, 0.5
	wide.Require = []Requirement{{Measure: "max_residual_m", Min: &metre}}
	if got := ranked(wide, points, captures); len(got) != 0 {
		t.Fatalf("a 0.8 m jump met a 1 m minimum: %+v", got)
	}
	wide.Require = []Requirement{{Measure: "max_residual_m", Min: &half}}
	if got := ranked(wide, points, captures); len(got) != 1 || got[0].Score != 1 {
		t.Fatalf("the jump's window, scored by its one track: %+v", got)
	}
}

func TestRankRefusesHeldOutWindowsItCannotChoose(t *testing.T) {
	c := shippedCatalogue(t)
	points, captures := goldenSeries()
	following := selectorFrom(t, c, "following")
	wider := following
	wider.Parameters.WindowSeconds = 20
	for _, s := range []Selector{selectorFrom(t, c, "close_following"), selectorFrom(t, c, "leader_changes"), wider} {
		if _, err := Rank(points, s, "golden", "held_out", captures); err == nil || !strings.Contains(err.Error(), "cannot choose a held_out window") {
			t.Errorf("%s at %v chose held-out windows: %v", s.ID, s.Parameters, err)
		}
	}
	// What Find refuses, Rank refuses with Find's reason.
	if _, err := Rank(points, following, "", "tuning", captures); err == nil || !strings.Contains(err.Error(), "source is required") {
		t.Fatalf("no source: %v", err)
	}
	if _, err := Rank(points, selectorFrom(t, c, "random"), "golden", "held_out", nil); err == nil || !strings.Contains(err.Error(), "requires indexed captures") {
		t.Fatalf("held-out random without captures: %v", err)
	}
}

// With a min_gap of 0, a leader abreast of its follower is a real gap of 0.
// It was taken for "no gap yet" and replaced by the next pair's.
func TestAFollowingGapOfZeroIsTheClosest(t *testing.T) {
	p := DefaultParams()
	p.MinGap = 0
	next := testBase + 100_000_000
	points := []Point{
		{Track: "f", TimeNs: testBase, VX: 10}, {Track: "l", TimeNs: testBase, Y: 1, VX: 10},
		{Track: "f", TimeNs: next, X: 1, VX: 10}, {Track: "l", TimeNs: next, X: 6, Y: 1, VX: 10},
	}
	w, err := Find(points, "following", "s", "tuning", p, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Abreast, each is the other's leader at 0 m; then l is 5 m ahead.
	if len(w) != 1 || w[0].PairFrames != 3 || w[0].ClosestGapM != 0 {
		t.Fatalf("closest gap: %+v", w)
	}
}
