package scene

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestModeOfGroupsClassifierLabels(t *testing.T) {
	for label, want := range map[string]string{
		"car":           "vehicle",
		"bus":           "vehicle",
		"truck":         "vehicle",
		"pedestrian":    "person",
		"cyclist":       "cycle",
		"motorcyclist":  "cycle",
		"bird":          "other",
		"dynamic":       "other",
		"noise":         "other",
		"":              "other",
		"something-new": "other",
	} {
		if got := modeOf(label); got != want {
			t.Errorf("modeOf(%q) = %q, want %q", label, got, want)
		}
	}
}

// Counts are means, not totals, so a bucket's height means the same thing
// whatever the stride or the number of frames that landed in it.
func TestTimelineCountsAreMeanConcurrent(t *testing.T) {
	acc := newTimelineAccumulator(5)
	for i := 0; i < 10; i++ {
		acc.observe(Frame{
			TimeUs: int64(i) * 100_000,
			Tracks: []TrackJSON{{Class: "car", Speed: 5}, {Class: "car", Speed: 9}},
		})
	}
	s := acc.summary()
	if len(s.Buckets) != 1 {
		t.Fatalf("got %d buckets, want 1", len(s.Buckets))
	}
	if got := s.Buckets[0].Vehicle; got != 2 {
		t.Errorf("vehicle = %v, want 2 (mean concurrent, not 20 total)", got)
	}
	if got := s.Buckets[0].MaxSpeed; got != 9 {
		t.Errorf("max speed = %v, want 9", got)
	}
	if s.Buckets[0].Frames != 10 {
		t.Errorf("frames = %d, want 10", s.Buckets[0].Frames)
	}
}

func TestTimelineSplitsModes(t *testing.T) {
	acc := newTimelineAccumulator(5)
	acc.observe(Frame{TimeUs: 0, Tracks: []TrackJSON{
		{Class: "car"}, {Class: "bus"},
		{Class: "pedestrian"}, {Class: "pedestrian"}, {Class: "pedestrian"},
		{Class: "cyclist"},
		{Class: "bird"}, {Class: ""},
	}})
	b := acc.summary().Buckets[0]
	if b.Vehicle != 2 || b.Person != 3 || b.Cycle != 1 || b.Other != 2 {
		t.Errorf("veh/ped/cyc/oth = %v/%v/%v/%v, want 2/3/1/2",
			b.Vehicle, b.Person, b.Cycle, b.Other)
	}
}

// A quiet stretch must still occupy its slot, or the strip would compress time
// and stop lining up with the scrubber.
func TestTimelineKeepsEmptyBuckets(t *testing.T) {
	acc := newTimelineAccumulator(5)
	acc.observe(Frame{TimeUs: 0, Tracks: []TrackJSON{{Class: "car"}}})
	acc.observe(Frame{TimeUs: 20_000_000, Tracks: []TrackJSON{{Class: "car"}}})

	s := acc.summary()
	if len(s.Buckets) != 5 {
		t.Fatalf("got %d buckets, want 5 covering 0-20 s at 5 s each", len(s.Buckets))
	}
	for i, b := range s.Buckets {
		if want := float64(i) * 5; b.StartSec != want {
			t.Errorf("bucket %d starts at %v, want %v", i, b.StartSec, want)
		}
	}
	if s.Buckets[1].Frames != 0 || s.Buckets[1].Vehicle != 0 {
		t.Errorf("the quiet bucket should be empty, got %+v", s.Buckets[1])
	}
}

func TestTimelineReportsPeaks(t *testing.T) {
	acc := newTimelineAccumulator(5)
	acc.observe(Frame{TimeUs: 0, Tracks: []TrackJSON{{Class: "car", Speed: 4}}})
	acc.observe(Frame{TimeUs: 10_000_000, Tracks: []TrackJSON{
		{Class: "car", Speed: 17.8}, {Class: "pedestrian"}, {Class: "cyclist"},
	}})
	s := acc.summary()
	if s.MaxSpeed != 17.8 {
		t.Errorf("max_speed = %v, want 17.8", s.MaxSpeed)
	}
	if s.MaxTotal != 3 {
		t.Errorf("max_total = %v, want 3", s.MaxTotal)
	}
}

// The summary is written beside the frames it describes, so a viewer can fetch
// it without knowing how the chunks are laid out.
func TestExportWritesTimelineSummary(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(60))
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out, BucketSeconds: 2}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(out, "timeline.json"))
	if err != nil {
		t.Fatalf("timeline.json missing: %v", err)
	}
	var s TimelineSummary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.BucketSeconds != 2 {
		t.Errorf("bucket_seconds = %v, want 2", s.BucketSeconds)
	}
	if len(s.Buckets) == 0 {
		t.Fatal("summary has no buckets")
	}
	if got := s.Buckets[0].Vehicle; got != 3 {
		t.Errorf("first bucket vehicle = %v, want 3 (the fixture writes three cars)", got)
	}
}
