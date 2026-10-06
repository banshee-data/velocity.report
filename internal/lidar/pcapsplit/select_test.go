package pcapsplit

import "testing"

func labelledSegment(kind string, id int, prefix string) Segment {
	return Segment{Type: kind, ID: id,
		Filename: prefix + "-" + kind + "-" + string(rune('0'+id)) + ".pcap"}
}

func TestSegmentLabel(t *testing.T) {
	tests := []struct {
		filename string
		kind     string
		want     string
	}{
		{"broadway_columbus-static-5.pcap", "static", "static-5"},
		{"broadway_columbus-motion-0.pcap", "motion", "motion-0"},
		// A prefix that itself contains the type word must not confuse the split.
		{"static-site-static-2.pcap", "static", "static-2"},
		// A filename with no recognisable label falls back to the whole stem.
		{"odd.pcap", "static", "odd"},
	}
	for _, tc := range tests {
		got := Segment{Type: tc.kind, Filename: tc.filename}.SegmentLabel()
		if got != tc.want {
			t.Errorf("SegmentLabel(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestSelectSegmentsFiltersByLabel(t *testing.T) {
	segs := []Segment{
		labelledSegment("static", 0, "bc"), labelledSegment("motion", 0, "bc"),
		labelledSegment("static", 1, "bc"), labelledSegment("motion", 1, "bc"),
	}

	selected, unmatched := SelectSegments(segs, []string{"static-1"})
	if len(unmatched) != 0 {
		t.Errorf("unmatched = %v, want none", unmatched)
	}
	if len(selected) != 1 || selected[0].Filename != "bc-static-1.pcap" {
		t.Fatalf("selected = %+v, want just bc-static-1.pcap", selected)
	}
}

func TestSelectSegmentsPreservesOrder(t *testing.T) {
	segs := []Segment{labelledSegment("static", 0, "bc"), labelledSegment("motion", 0, "bc"), labelledSegment("static", 1, "bc")}
	selected, _ := SelectSegments(segs, []string{"static-1", "static-0"})
	if len(selected) != 2 {
		t.Fatalf("selected %d, want 2", len(selected))
	}
	// Order follows the segments, not the request: writing is a single pass
	// over the captures in time order.
	if selected[0].Filename != "bc-static-0.pcap" {
		t.Errorf("first selected = %q, want bc-static-0.pcap", selected[0].Filename)
	}
}

func TestSelectSegmentsEmptyRequestSelectsAll(t *testing.T) {
	segs := []Segment{labelledSegment("static", 0, "bc"), labelledSegment("motion", 0, "bc")}
	selected, unmatched := SelectSegments(segs, nil)
	if len(selected) != 2 || len(unmatched) != 0 {
		t.Errorf("selected %d, unmatched %v; want all and none", len(selected), unmatched)
	}
}

func TestSelectSegmentsReportsUnmatchedRequests(t *testing.T) {
	// Silently writing nothing for a typo is the worst outcome: the operator
	// waits for a long pass and gets an empty directory.
	segs := []Segment{labelledSegment("static", 0, "bc")}
	selected, unmatched := SelectSegments(segs, []string{"static-0", "static-9", "motion-3"})
	if len(selected) != 1 {
		t.Errorf("selected %d, want 1", len(selected))
	}
	if len(unmatched) != 2 {
		t.Fatalf("unmatched = %v, want the two absent labels", unmatched)
	}
}

func TestSelectSegmentsTrimsWhitespace(t *testing.T) {
	segs := []Segment{labelledSegment("static", 0, "bc")}
	selected, unmatched := SelectSegments(segs, []string{" static-0 "})
	if len(selected) != 1 || len(unmatched) != 0 {
		t.Errorf("selected %d, unmatched %v; want 1 and none", len(selected), unmatched)
	}
}

func TestInputFiles(t *testing.T) {
	tests := []struct {
		name string
		cfg  SplitConfig
		want int
	}{
		{"single file", SplitConfig{PCAPFile: "a.pcap"}, 1},
		{"sequence", SplitConfig{PCAPFile: "a.pcap", PCAPFiles: []string{"a.pcap", "b.pcap"}}, 2},
		{"nothing configured", SplitConfig{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(tc.cfg.InputFiles()); got != tc.want {
				t.Errorf("InputFiles() returned %d, want %d", got, tc.want)
			}
		})
	}
}

// TestSelectionMaskRoutesOverTheWholeTimeline guards the failure that a
// filtered write hit in practice.
//
// segmentIndexForTime clamps by design so no packet is dropped: anything before
// the first segment routes to the first, anything after the last routes to the
// last. Handing it only the selected segments therefore made every packet in
// the run land in the one surviving segment — a seven-minute extract came out
// as the whole forty-minute session. The selection has to be applied after
// routing, against the complete list.
func TestSelectionMaskRoutesOverTheWholeTimeline(t *testing.T) {
	segs := []Segment{
		labelledSegment("static", 0, "bc"),
		labelledSegment("motion", 0, "bc"),
		labelledSegment("static", 1, "bc"),
	}

	mask := selectionMask(segs, []string{"static-1"})
	if len(mask) != len(segs) {
		t.Fatalf("mask covers %d segments, want %d — it must be parallel to the "+
			"full list routing sees", len(mask), len(segs))
	}
	want := []bool{false, false, true}
	for i := range want {
		if mask[i] != want[i] {
			t.Errorf("mask[%d] = %v, want %v", i, mask[i], want[i])
		}
	}
}

func TestSelectionMaskEmptySelectionWritesEverything(t *testing.T) {
	segs := []Segment{labelledSegment("static", 0, "bc"), labelledSegment("motion", 0, "bc")}
	mask := selectionMask(segs, nil)
	for i, on := range mask {
		if !on {
			t.Errorf("mask[%d] is off; an empty selection must write every segment", i)
		}
	}
}

func TestSelectionMaskIgnoresUnknownLabels(t *testing.T) {
	// Run rejects unmatched labels before this point; the mask must simply not
	// select anything rather than panic or select by position.
	segs := []Segment{labelledSegment("static", 0, "bc")}
	for i, on := range selectionMask(segs, []string{"static-9"}) {
		if on {
			t.Errorf("mask[%d] selected a segment for an unknown label", i)
		}
	}
}
