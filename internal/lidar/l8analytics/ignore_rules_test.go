package l8analytics

import "testing"

// The ignore rules, pinned case by case.
//
// MOT16's distractor rule says an uncertifiable annotation neither penalises
// nor rewards a tracker: results are matched first, and anything matched to an
// ignored annotation is then removed. What that has to mean for each metric,
// and what it must not be allowed to mean, is below. Every case is small
// enough to count by hand; the counts in each row are that hand count.
//
// The rules:
//
//  1. A hypothesis matched to an ignored point is neither a true nor a false
//     positive, in CLEAR MOT, HOTA and the identity family alike. Nor can an
//     ignored point hold on to a hypothesis by continuity when a scored
//     object nearer to it needs it: that would charge the tracker a miss.
//  2. An ignored point is never a false negative.
//  3. Ignore absorbs only what matches it: a hypothesis outside the ignored
//     point's gate is still a false positive.
//  4. An ignored stretch does not interrupt a trajectory, so it cannot create a
//     fragmentation; the same stretch scored and missed does.
//  5. A hypothesis absorbed during an ignored stretch never becomes the
//     reference's last assignment, so it cannot cause an identity switch.
//  6. Continuity carries across an ignored stretch, so a stray hypothesis
//     cannot take the object from the one that tracked it through. This is the
//     one place the implementation departs from MOT16's devkit, which drops
//     the correspondence; see perframe.go.
//  7. A real change of hypothesis across an ignored stretch is still a switch:
//     MOT16 compares with the last known assignment, and that failure is the
//     one reassociation work has to be measured on.
//  8. Ignore is decided per frame, per point: one object can be scored in one
//     frame and ignored in the next.
//  9. Matching precedes discarding: a hypothesis nearer an ignored point than a
//     scored one is absorbed, and the scored one is missed.

// span is frames from..to (inclusive) of an object moving along +x at one
// metre per frame, so frame k sits at x = k.
func span(from, to int, y float32) []SeriesPoint {
	out := make([]SeriesPoint, 0, to-from+1)
	for k := from; k <= to; k++ {
		out = append(out, SeriesPoint{TimestampNanos: int64(k) * mFrame, X: float32(k), Y: y})
	}
	return out
}

func track(id string, parts ...[]SeriesPoint) TrackSeries {
	s := TrackSeries{ID: id}
	for _, p := range parts {
		s.Points = append(s.Points, p...)
	}
	return s
}

// ignoring marks the listed frames of a reference as ignored.
func ignoring(s TrackSeries, frames ...int) TrackSeries {
	marked := make(map[int64]bool, len(frames))
	for _, f := range frames {
		marked[int64(f)*mFrame] = true
	}
	out := TrackSeries{ID: s.ID, Points: append([]SeriesPoint(nil), s.Points...)}
	for i := range out.Points {
		if marked[out.Points[i].TimestampNanos] {
			out.Points[i].Ignore = true
		}
	}
	return out
}

type ignoreCounts struct {
	NumGT, Matches, FN, FP, IDSW, FM, Ignored int
}

func TestIgnoreRulesMOT16(t *testing.T) {
	gate := FixedGate(1.0)
	type identityCounts struct{ IDTP, IDFP, IDFN int }
	type hotaCounts struct{ TP, FN, FP int }

	cases := []struct {
		rule     string
		ref, hyp []TrackSeries
		want     ignoreCounts
		identity *identityCounts
		// hota is checked at the lowest alpha, 0.05, where every exact or
		// near-exact pairing clears the threshold.
		hota *hotaCounts
	}{
		{
			rule: "1: a hypothesis on an ignored point is neither TP nor FP",
			ref: []TrackSeries{
				track("A", span(0, 4, 0)),
				ignoring(track("Q", span(0, 4, 30)), 0, 1, 2, 3, 4),
			},
			hyp:      []TrackSeries{track("hA", span(0, 4, 0)), track("hQ", span(0, 4, 30))},
			want:     ignoreCounts{NumGT: 5, Matches: 5, Ignored: 5},
			identity: &identityCounts{IDTP: 5},
			hota:     &hotaCounts{TP: 5},
		},
		{
			rule: "1: an ignored point does not keep its hypothesis by continuity",
			// h1 tracks R while it is scored, then R becomes uncertifiable
			// just as S appears 0.8 m away, and h1 is exactly on S. S is
			// scored and must not be missed for R's sake.
			ref: []TrackSeries{
				ignoring(track("R", span(0, 4, 0)), 2, 3, 4),
				track("S", span(2, 4, 0.8)),
			},
			hyp:      []TrackSeries{track("h1", span(0, 1, 0), span(2, 4, 0.8))},
			want:     ignoreCounts{NumGT: 5, Matches: 5},
			identity: &identityCounts{IDTP: 3, IDFP: 2, IDFN: 2},
		},
		{
			rule: "2: an ignored point nobody tracked is not missed",
			ref: []TrackSeries{
				track("A", span(0, 4, 0)),
				ignoring(track("Q", span(0, 4, 30)), 0, 1, 2, 3, 4),
			},
			hyp:      []TrackSeries{track("hA", span(0, 4, 0))},
			want:     ignoreCounts{NumGT: 5, Matches: 5},
			identity: &identityCounts{IDTP: 5},
			hota:     &hotaCounts{TP: 5},
		},
		{
			rule:     "3: a hypothesis outside an ignored point's gate is still false",
			ref:      []TrackSeries{ignoring(track("Q", span(0, 4, 30)), 0, 1, 2, 3, 4)},
			hyp:      []TrackSeries{track("h", span(0, 4, 32))},
			want:     ignoreCounts{FP: 5},
			identity: &identityCounts{IDFP: 5},
			hota:     &hotaCounts{FP: 5},
		},
		{
			rule:     "4: an ignored stretch does not fragment a trajectory",
			ref:      []TrackSeries{ignoring(track("R", span(0, 9, 0)), 3, 4, 5)},
			hyp:      []TrackSeries{track("h1", span(0, 2, 0), span(6, 9, 0))},
			want:     ignoreCounts{NumGT: 7, Matches: 7},
			identity: &identityCounts{IDTP: 7},
		},
		{
			rule: "4 (control): the same stretch scored and missed is one fragmentation",
			ref:  []TrackSeries{track("R", span(0, 9, 0))},
			hyp:  []TrackSeries{track("h1", span(0, 2, 0), span(6, 9, 0))},
			want: ignoreCounts{NumGT: 10, Matches: 7, FN: 3, FM: 1},
		},
		{
			rule: "5: a hypothesis absorbed in an ignored stretch is not the last assignment",
			ref:  []TrackSeries{ignoring(track("R", span(0, 9, 0)), 3, 4, 5)},
			hyp: []TrackSeries{
				track("h1", span(0, 2, 0), span(6, 9, 0)),
				track("h2", span(3, 5, 0)),
			},
			want: ignoreCounts{NumGT: 7, Matches: 7, Ignored: 3},
			// h2's three detections were absorbed, so they are not identity
			// false positives either.
			identity: &identityCounts{IDTP: 7},
			hota:     &hotaCounts{TP: 7},
		},
		{
			rule: "6: continuity carries across an ignored stretch",
			// h1 tracks R exactly, through the ignored frames 3 and 4, then
			// sits 0.5 m off. A stray h2 appears 0.1 m off from frame 5.
			// Continuity keeps h1; h2 is the false positive.
			ref: []TrackSeries{ignoring(track("R", span(0, 9, 0)), 3, 4)},
			hyp: []TrackSeries{
				track("h1", span(0, 4, 0), span(5, 9, 0.5)),
				track("h2", span(5, 9, -0.1)),
			},
			want: ignoreCounts{NumGT: 8, Matches: 8, FP: 5, Ignored: 2},
		},
		{
			rule: "6 (control): the same stretch scored scores the same identity",
			ref:  []TrackSeries{track("R", span(0, 9, 0))},
			hyp: []TrackSeries{
				track("h1", span(0, 4, 0), span(5, 9, 0.5)),
				track("h2", span(5, 9, -0.1)),
			},
			want: ignoreCounts{NumGT: 10, Matches: 10, FP: 5},
		},
		{
			rule:     "7: a real change of hypothesis across an ignored stretch is a switch",
			ref:      []TrackSeries{ignoring(track("R", span(0, 9, 0)), 3, 4, 5)},
			hyp:      []TrackSeries{track("h1", span(0, 2, 0)), track("h2", span(6, 9, 0))},
			want:     ignoreCounts{NumGT: 7, Matches: 7, IDSW: 1},
			identity: &identityCounts{IDTP: 4, IDFP: 3, IDFN: 3},
		},
		{
			rule:     "8: ignore is per frame (tracked throughout)",
			ref:      []TrackSeries{ignoring(track("R", span(0, 9, 0)), 5, 6, 7, 8, 9)},
			hyp:      []TrackSeries{track("h", span(0, 9, 0))},
			want:     ignoreCounts{NumGT: 5, Matches: 5, Ignored: 5},
			identity: &identityCounts{IDTP: 5},
			hota:     &hotaCounts{TP: 5},
		},
		{
			rule:     "8: ignore is per frame (missed only where scored)",
			ref:      []TrackSeries{ignoring(track("R", span(0, 9, 0)), 5, 6, 7, 8, 9)},
			hyp:      nil,
			want:     ignoreCounts{NumGT: 5, FN: 5},
			identity: &identityCounts{IDFN: 5},
			hota:     &hotaCounts{FN: 5},
		},
		{
			rule: "9: matching precedes discarding",
			// h is 0.9 m from scored A and 0.6 m from ignored Q. The optimal
			// assignment gives it to Q, where it is absorbed, and A is missed.
			ref: []TrackSeries{
				track("A", span(0, 4, 0)),
				ignoring(track("Q", span(0, 4, 1.5)), 0, 1, 2, 3, 4),
			},
			hyp:      []TrackSeries{track("h", span(0, 4, 0.9))},
			want:     ignoreCounts{NumGT: 5, FN: 5, Ignored: 5},
			identity: &identityCounts{IDFN: 5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			m := ComputeTrackMetricsGated(tc.ref, tc.hyp, gate)
			got := ignoreCounts{
				NumGT: m.NumGT, Matches: m.Matches, FN: m.FN, FP: m.FP,
				IDSW: m.IDSwitches, FM: m.Fragmentations, Ignored: m.IgnoredHypotheses,
			}
			if got != tc.want {
				t.Errorf("CLEAR MOT counts = %+v, want %+v", got, tc.want)
			}

			if tc.identity != nil {
				id := ComputeIdentityMetrics(tc.ref, tc.hyp, gate)
				if got := (identityCounts{id.IDTP, id.IDFP, id.IDFN}); got != *tc.identity {
					t.Errorf("identity counts = %+v, want %+v", got, *tc.identity)
				}
				// The identity totals and the CLEAR MOT totals count the same
				// populations, so they must reconcile.
				if id.IDTP+id.IDFN != m.NumGT {
					t.Errorf("IDTP+IDFN = %d, want NumGT %d", id.IDTP+id.IDFN, m.NumGT)
				}
				if id.IDTP+id.IDFP != m.Matches+m.FP {
					t.Errorf("IDTP+IDFP = %d, want scored hypotheses %d", id.IDTP+id.IDFP, m.Matches+m.FP)
				}
			}

			if tc.hota != nil {
				h := ComputeHOTAGated(tc.ref, tc.hyp, gate, nil)
				row := h.PerAlpha[0]
				if got := (hotaCounts{row.TP, row.FN, row.FP}); got != *tc.hota {
					t.Errorf("HOTA counts at alpha %.2f = %+v, want %+v", row.Alpha, got, *tc.hota)
				}
			}

			// One pass must agree with the separate entry points.
			all := EvaluatePerFrame(tc.ref, tc.hyp, gate, nil)
			if all.CLEARMOT != m {
				t.Errorf("EvaluatePerFrame CLEAR MOT = %+v, want %+v", all.CLEARMOT, m)
			}
		})
	}
}

// Two guards on rule 6. The carried correspondence gives way when a scored
// match claims its hypothesis, so the continuity map stays one-to-one; and an
// ignored point never keeps a hypothesis by continuity, so it cannot take one
// from a scored object beside it.
func TestCarriedCorrespondenceYieldsToAScoredClaim(t *testing.T) {
	// R is ignored at frames 2 and 3. S is scored from frame 2, 0.8 m from R,
	// inside the gate of both. h1 tracks R, then moves onto S exactly. h2
	// picks R up when it is scored again at frame 4.
	ref := []TrackSeries{
		ignoring(track("R", span(0, 5, 0)), 2, 3),
		track("S", span(2, 5, 0.8)),
	}
	hyp := []TrackSeries{
		track("h1", span(0, 1, 0), span(2, 5, 0.8)),
		track("h2", span(4, 5, 0)),
	}
	m := ComputeTrackMetricsGated(ref, hyp, FixedGate(1.0))
	// R: frames 0, 1 by h1; frames 4, 5 by h2 (a real change: one switch).
	// S: frames 2 to 5 by h1, never missed. Had R kept h1 by continuity at
	// frame 2, S would have been missed; had R's carried h1 not yielded to S,
	// both would claim h1 at frame 4 and h2 would be a false positive.
	want := ignoreCounts{NumGT: 8, Matches: 8, IDSW: 1}
	got := ignoreCounts{
		NumGT: m.NumGT, Matches: m.Matches, FN: m.FN, FP: m.FP,
		IDSW: m.IDSwitches, FM: m.Fragmentations, Ignored: m.IgnoredHypotheses,
	}
	if got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
}
