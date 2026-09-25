package l8analytics

import (
	"math"
	"testing"
)

// Known answers for the identity family. Each is small enough to count: IDTP
// is the best one-to-one pairing of whole trajectories by frames shared inside
// the gate, and everything else is an identity miss or false alarm.

func TestIdentityKnownAnswers(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-12 }
	gate := FixedGate(1.0)

	t.Run("perfect tracking", func(t *testing.T) {
		ref := []TrackSeries{straight("a", 0, 10, 0, 0), straight("b", 0, 10, 0, 20)}
		hyp := []TrackSeries{straight("h1", 0, 10, 0, 0), straight("h2", 0, 10, 0, 20)}
		id := ComputeIdentityMetrics(ref, hyp, gate)
		if id.IDTP != 20 || id.IDFP != 0 || id.IDFN != 0 || id.IDF1 != 1 || id.IDP != 1 || id.IDR != 1 {
			t.Fatalf("got %+v, want IDF1 = IDP = IDR = 1 with IDTP 20", id)
		}
	})

	t.Run("swap halfway", func(t *testing.T) {
		// Each hypothesis covers half of each reference, so the best pairing
		// keeps half of every trajectory: IDTP 10 of 20 either way.
		ref := []TrackSeries{straight("a", 0, 10, 0, 0), straight("b", 0, 10, 0, 10)}
		hyp := []TrackSeries{
			{ID: "h1", Points: append(straight("", 0, 5, 0, 0).Points, straight("", 5, 5, 5, 10).Points...)},
			{ID: "h2", Points: append(straight("", 0, 5, 0, 10).Points, straight("", 5, 5, 5, 0).Points...)},
		}
		id := ComputeIdentityMetrics(ref, hyp, gate)
		if id.IDTP != 10 || id.IDFP != 10 || id.IDFN != 10 || !near(id.IDF1, 0.5) {
			t.Fatalf("got %+v, want IDTP 10, IDFP 10, IDFN 10, IDF1 0.5", id)
		}
	})

	t.Run("four pieces", func(t *testing.T) {
		ref := []TrackSeries{straight("a", 0, 12, 0, 0)}
		var hyp []TrackSeries
		for piece := 0; piece < 4; piece++ {
			hyp = append(hyp, TrackSeries{
				ID: string(rune('A' + piece)), Points: straight("", piece*3, 3, float32(piece*3), 0).Points,
			})
		}
		id := ComputeIdentityMetrics(ref, hyp, gate)
		if id.IDTP != 3 || id.IDFN != 9 || id.IDFP != 9 || !near(id.IDF1, 0.25) || !near(id.IDR, 0.25) {
			t.Fatalf("got %+v, want IDTP 3 of 12 and IDF1 0.25", id)
		}
	})

	t.Run("a long wrong identity costs more than a short one", func(t *testing.T) {
		// Both runs switch identity once, so IDSW cannot tell them apart.
		ref := []TrackSeries{straight("a", 0, 10, 0, 0)}
		brief := []TrackSeries{
			{ID: "h1", Points: straight("", 0, 8, 0, 0).Points},
			{ID: "h2", Points: straight("", 8, 2, 8, 0).Points},
		}
		long := []TrackSeries{
			{ID: "h1", Points: straight("", 0, 5, 0, 0).Points},
			{ID: "h2", Points: straight("", 5, 5, 5, 0).Points},
		}
		mb, ml := ComputeTrackMetrics(ref, brief, 1), ComputeTrackMetrics(ref, long, 1)
		if mb.IDSwitches != 1 || ml.IDSwitches != 1 {
			t.Fatalf("IDSW %d and %d, want 1 and 1", mb.IDSwitches, ml.IDSwitches)
		}
		ib, il := ComputeIdentityMetrics(ref, brief, gate), ComputeIdentityMetrics(ref, long, gate)
		// brief: IDTP 8, IDFN 2, IDFP 2: 16/20. long: IDTP 5: 10/20.
		if !near(ib.IDF1, 0.8) || !near(il.IDF1, 0.5) {
			t.Fatalf("IDF1 brief %.4f long %.4f, want 0.8 and 0.5", ib.IDF1, il.IDF1)
		}
	})

	t.Run("assignment is global, not greedy", func(t *testing.T) {
		// a is present in frames 0-9, b in frames 10-19. h1 covers all of a
		// and nine frames of b; h2 covers nine frames of a. Greedy takes the
		// largest pair, a-h1 (10), and leaves b nothing: 10. The optimum is
		// a-h2 plus b-h1: 18.
		ref := []TrackSeries{straight("a", 0, 10, 0, 0), straight("b", 10, 10, 10, 0)}
		hyp := []TrackSeries{
			{ID: "h1", Points: append(straight("", 0, 10, 0, 0).Points, straight("", 10, 9, 10, 0).Points...)},
			{ID: "h2", Points: straight("", 1, 9, 1, 0.2).Points},
		}
		id := ComputeIdentityMetrics(ref, hyp, gate)
		if id.IDTP != 18 {
			t.Fatalf("IDTP = %d, want 18 from the global assignment: %+v", id.IDTP, id)
		}
		// 20 reference detections, 28 hypothesis detections.
		if id.IDFN != 2 || id.IDFP != 10 || !near(id.IDF1, 36.0/48.0) {
			t.Fatalf("got %+v, want IDFN 2, IDFP 10, IDF1 0.75", id)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if id := ComputeIdentityMetrics(nil, nil, gate); id != (IdentityMetrics{}) {
			t.Fatalf("empty input gave %+v", id)
		}
	})
}

// Pooling separate sequences must give what scoring them as one would, when
// they share no identities and no frames: counts add, ratios are recomputed.
func TestCombiningEpisodesEqualsScoringThemTogether(t *testing.T) {
	gate := FixedGate(1.0)
	first := [2][]TrackSeries{
		{straight("a", 0, 10, 0, 0), straight("b", 0, 10, 0, 10)},
		{
			{ID: "h1", Points: append(straight("", 0, 5, 0, 0).Points, straight("", 5, 5, 5, 10).Points...)},
			{ID: "h2", Points: append(straight("", 0, 5, 0, 10.3).Points, straight("", 5, 5, 5, 0).Points...)},
		},
	}
	second := [2][]TrackSeries{
		{straight("c", 100, 12, 0, 0)},
		{
			{ID: "h3", Points: straight("", 100, 3, 0, 0.4).Points},
			{ID: "h4", Points: straight("", 106, 6, 6, 0).Points},
			{ID: "ghost", Points: straight("", 100, 4, 0, 40).Points},
		},
	}
	p1 := EvaluatePerFrame(first[0], first[1], gate, nil)
	p2 := EvaluatePerFrame(second[0], second[1], gate, nil)
	whole := EvaluatePerFrame(append(append([]TrackSeries(nil), first[0]...), second[0]...),
		append(append([]TrackSeries(nil), first[1]...), second[1]...), gate, nil)

	clear := CombineTrackMetrics([]TrackMetrics{p1.CLEARMOT, p2.CLEARMOT})
	w := whole.CLEARMOT
	if clear.NumGT != w.NumGT || clear.FN != w.FN || clear.FP != w.FP || clear.IDSwitches != w.IDSwitches ||
		clear.Fragmentations != w.Fragmentations || clear.Matches != w.Matches || clear.NumFrames != w.NumFrames ||
		math.Abs(clear.MOTA-w.MOTA) > 1e-12 || math.Abs(clear.MOTP-w.MOTP) > 1e-9 {
		t.Fatalf("combined CLEAR MOT %+v, scored together %+v", clear, w)
	}
	if id := CombineIdentity([]IdentityMetrics{p1.Identity, p2.Identity}); id != whole.Identity {
		t.Fatalf("combined identity %+v, scored together %+v", id, whole.Identity)
	}
	hota, err := CombineHOTA([]HOTAResult{p1.HOTA, p2.HOTA})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(hota.HOTA-whole.HOTA.HOTA) > 1e-9 || math.Abs(hota.DetA-whole.HOTA.DetA) > 1e-9 ||
		math.Abs(hota.AssA-whole.HOTA.AssA) > 1e-9 {
		t.Fatalf("combined HOTA %.9f/%.9f/%.9f, scored together %.9f/%.9f/%.9f",
			hota.HOTA, hota.DetA, hota.AssA, whole.HOTA.HOTA, whole.HOTA.DetA, whole.HOTA.AssA)
	}

	if _, err := CombineHOTA([]HOTAResult{p1.HOTA, ComputeHOTA(nil, nil, 1, []float64{0.5})}); err == nil {
		t.Fatal("CombineHOTA accepted parts swept over different alphas")
	}
	if h, err := CombineHOTA(nil); err != nil || h.HOTA != 0 {
		t.Fatalf("CombineHOTA(nil) = %+v, %v", h, err)
	}
}
