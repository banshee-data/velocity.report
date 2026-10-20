package l8analytics

import "testing"

// MatchPerFrame must hand back exactly the matching the MOT16 tally counts:
// the continuity-preferred hypothesis on each scored reference, nothing on an
// ignored one, and the fragment that picked the object up after a gap.
func TestMatchPerFrameIsTheTalliedMatching(t *testing.T) {
	ref := []TrackSeries{straight("obj", 0, 6, 0, 0)}
	ref[0].Points[2].Ignore = true
	hyp := []TrackSeries{
		straight("h1", 0, 3, 0, 0),
		// h2 picks the object up at frame 3 after h1 ends.
		{ID: "h2", Points: straight("h2", 3, 3, 3, 0).Points},
	}
	got := MatchPerFrame(ref, hyp, FixedGate(0.5))
	want := []string{"h1", "h1", "", "h2", "h2", "h2"}
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d", len(got), len(want))
	}
	for i, c := range got {
		if c.TimestampNanos != int64(i)*mFrame {
			t.Errorf("frame %d at %d, want %d", i, c.TimestampNanos, int64(i)*mFrame)
		}
		h, matched := c.Matches["obj"]
		if want[i] == "" {
			if matched {
				t.Errorf("frame %d: ignored reference matched to %s", i, h)
			}
			continue
		}
		if h != want[i] {
			t.Errorf("frame %d: matched %q, want %q", i, h, want[i])
		}
	}
	m := ComputeTrackMetricsGated(ref, hyp, FixedGate(0.5))
	matches := 0
	for _, c := range got {
		matches += len(c.Matches)
	}
	if matches != m.Matches {
		t.Errorf("correspondences %d, tally %d", matches, m.Matches)
	}
}
