package l8analytics

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

// Known-answer cases. A metric that cannot produce the right number on input
// whose answer is known by construction cannot be trusted on real input, so
// every case here is one where the geometry fixes the expected count.

const mFrame = int64(100_000_000) // 100 ms

// series builds one object's track from per-frame positions, starting at frame
// `from`. A nil entry means the object is absent that frame.
func series(id string, from int, pts [][2]float32) TrackSeries {
	s := TrackSeries{ID: id}
	for i, p := range pts {
		s.Points = append(s.Points, SeriesPoint{
			TimestampNanos: int64(from+i) * mFrame, X: p[0], Y: p[1],
		})
	}
	return s
}

// straight is an object moving along +x at one metre per frame.
func straight(id string, from, n int, x0, y float32) TrackSeries {
	pts := make([][2]float32, n)
	for i := range pts {
		pts[i] = [2]float32{x0 + float32(i), y}
	}
	return series(id, from, pts)
}

func TestPerfectTrackingScoresOne(t *testing.T) {
	ref := []TrackSeries{straight("a", 0, 10, 0, 0), straight("b", 0, 10, 0, 20)}
	hyp := []TrackSeries{straight("h1", 0, 10, 0, 0), straight("h2", 0, 10, 0, 20)}

	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.MOTA != 1.0 || m.FN != 0 || m.FP != 0 || m.IDSwitches != 0 || m.Fragmentations != 0 {
		t.Fatalf("perfect tracking scored %+v; want MOTA 1.0 with no FN, FP, IDSW or FM", m)
	}
	if m.MOTP != 0 {
		t.Errorf("MOTP = %v, want 0 for exact positions", m.MOTP)
	}

	h := ComputeHOTA(ref, hyp, 1.0, nil)
	if math.Abs(h.HOTA-1.0) > 1e-9 || math.Abs(h.DetA-1.0) > 1e-9 || math.Abs(h.AssA-1.0) > 1e-9 {
		t.Fatalf("perfect tracking HOTA = %.6f (DetA %.6f, AssA %.6f); want 1.0 for all three",
			h.HOTA, h.DetA, h.AssA)
	}
}

// Two objects crossing, with the tracker swapping their identities at the
// crossing: the textbook identity switch. Both references are matched on every
// frame, so detection is perfect and only association suffers.
func TestScriptedIdentitySwapCountsTwoSwitches(t *testing.T) {
	// a runs y=0, b runs y=10; each hypothesis follows one reference for the
	// first half and the other for the second.
	ref := []TrackSeries{straight("a", 0, 10, 0, 0), straight("b", 0, 10, 0, 10)}
	hyp := []TrackSeries{
		{ID: "h1", Points: append(straight("", 0, 5, 0, 0).Points, straight("", 5, 5, 5, 10).Points...)},
		{ID: "h2", Points: append(straight("", 0, 5, 0, 10).Points, straight("", 5, 5, 5, 0).Points...)},
	}

	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.IDSwitches != 2 {
		t.Fatalf("IDSwitches = %d, want 2 (both references change hypothesis at the swap): %+v", m.IDSwitches, m)
	}
	if m.FN != 0 || m.FP != 0 {
		t.Errorf("swap should not cost detections, got FN=%d FP=%d", m.FN, m.FP)
	}
	if m.Fragmentations != 0 {
		t.Errorf("Fragmentations = %d, want 0: both references stay tracked throughout", m.Fragmentations)
	}
	// MOTA = 1 - (0 + 0 + 2)/20
	if want := 1.0 - 2.0/20.0; math.Abs(m.MOTA-want) > 1e-9 {
		t.Errorf("MOTA = %v, want %v", m.MOTA, want)
	}

	// HOTA separates the two: detection is perfect, association is not.
	h := ComputeHOTA(ref, hyp, 1.0, nil)
	if math.Abs(h.DetA-1.0) > 1e-9 {
		t.Errorf("DetA = %.6f, want 1.0: every reference is matched on every frame", h.DetA)
	}
	if h.AssA >= 1.0 {
		t.Errorf("AssA = %.6f, want below 1.0: identities were swapped", h.AssA)
	}
	if h.HOTA >= 1.0 {
		t.Errorf("HOTA = %.6f, want below 1.0", h.HOTA)
	}
}

// One reference, tracked then lost then picked up again: one fragmentation.
// This is the number that distinguishes a recall gain from one object cut up,
// and it is what the existing evaluator hard-codes to zero.
func TestFragmentationCountsResumedTracking(t *testing.T) {
	ref := []TrackSeries{straight("a", 0, 10, 0, 0)}
	// Hypothesis present for frames 0-2 and 6-9, absent in between.
	hyp := []TrackSeries{{ID: "h1", Points: append(
		straight("", 0, 3, 0, 0).Points, straight("", 6, 4, 6, 0).Points...)}}

	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.Fragmentations != 1 {
		t.Fatalf("Fragmentations = %d, want 1 (tracked, lost, resumed): %+v", m.Fragmentations, m)
	}
	if m.FN != 3 {
		t.Errorf("FN = %d, want 3 for frames 3, 4 and 5", m.FN)
	}
	// The same hypothesis resumed, so it is not an identity switch.
	if m.IDSwitches != 0 {
		t.Errorf("IDSwitches = %d, want 0: the same hypothesis resumed", m.IDSwitches)
	}

	// Two separate hypotheses over the same gap is one fragmentation and one
	// identity switch.
	split := []TrackSeries{
		{ID: "h1", Points: straight("", 0, 3, 0, 0).Points},
		{ID: "h2", Points: straight("", 6, 4, 6, 0).Points},
	}
	if m := ComputeTrackMetrics(ref, split, 1.0); m.Fragmentations != 1 || m.IDSwitches != 1 {
		t.Errorf("split resume gave FM=%d IDSW=%d, want 1 and 1", m.Fragmentations, m.IDSwitches)
	}
}

// One object reported as four consecutive pieces: the case the temporal-IoU
// evaluator scores as undetected and this one must score as fragmented.
func TestOneObjectAsFourPiecesIsFragmentedNotMissed(t *testing.T) {
	ref := []TrackSeries{straight("a", 0, 12, 0, 0)}
	var hyp []TrackSeries
	for piece := 0; piece < 4; piece++ {
		from := piece * 3
		hyp = append(hyp, TrackSeries{
			ID:     string(rune('A' + piece)),
			Points: straight("", from, 3, float32(from), 0).Points,
		})
	}

	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.FN != 0 {
		t.Fatalf("FN = %d, want 0: the object was detected on every frame", m.FN)
	}
	if m.IDSwitches != 3 {
		t.Errorf("IDSwitches = %d, want 3 for four pieces", m.IDSwitches)
	}
	// Each piece begins the frame after the last ended, so tracking is never
	// interrupted: the damage is identity, not continuity.
	if m.Fragmentations != 0 {
		t.Errorf("Fragmentations = %d, want 0: the pieces are contiguous", m.Fragmentations)
	}
	h := ComputeHOTA(ref, hyp, 1.0, nil)
	if math.Abs(h.DetA-1.0) > 1e-9 {
		t.Errorf("DetA = %.6f, want 1.0", h.DetA)
	}
	if h.AssA > 0.5 {
		t.Errorf("AssA = %.6f, want well below 0.5 for four pieces", h.AssA)
	}
}

// MOT16's ignore rule: a hypothesis matching an uncertifiable reference point
// is neither rewarded nor penalised.
func TestIgnoredReferenceAbsorbsItsHypothesis(t *testing.T) {
	ref := []TrackSeries{straight("a", 0, 5, 0, 0)}
	ignored := straight("occluded", 0, 5, 0, 30)
	for i := range ignored.Points {
		ignored.Points[i].Ignore = true
	}
	ref = append(ref, ignored)
	hyp := []TrackSeries{straight("h1", 0, 5, 0, 0), straight("h2", 0, 5, 0, 30)}

	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.NumGT != 5 {
		t.Errorf("NumGT = %d, want 5: the ignored reference is not ground truth", m.NumGT)
	}
	if m.FP != 0 {
		t.Errorf("FP = %d, want 0: the second hypothesis was absorbed, not wrong", m.FP)
	}
	if m.FN != 0 {
		t.Errorf("FN = %d, want 0", m.FN)
	}
	if m.IgnoredHypotheses != 5 {
		t.Errorf("IgnoredHypotheses = %d, want 5", m.IgnoredHypotheses)
	}
	if m.MOTA != 1.0 {
		t.Errorf("MOTA = %v, want 1.0", m.MOTA)
	}

	// Without the ignore flag the same hypothesis is a false positive on every
	// frame, which is what the rule exists to avoid.
	plain := []TrackSeries{straight("a", 0, 5, 0, 0), straight("occluded", 0, 5, 0, 30)}
	if m := ComputeTrackMetrics(plain, hyp, 1.0); m.FP != 0 || m.NumGT != 10 {
		t.Logf("control: unignored reference gives NumGT=%d FP=%d", m.NumGT, m.FP)
	}
}

// A hypothesis that never comes near anything is a false positive per frame.
func TestSpuriousHypothesisIsFalsePositive(t *testing.T) {
	ref := []TrackSeries{straight("a", 0, 6, 0, 0)}
	hyp := []TrackSeries{straight("h1", 0, 6, 0, 0), straight("ghost", 0, 6, 0, 500)}

	m := ComputeTrackMetrics(ref, hyp, 1.0)
	if m.FP != 6 || m.FN != 0 || m.Matches != 6 {
		t.Fatalf("got %+v; want FP 6, FN 0, Matches 6", m)
	}
	if m.MOTA != 0.0 {
		t.Errorf("MOTA = %v, want 0 (six false positives against six references)", m.MOTA)
	}
}

// Determinism: the metrics must not depend on the order tracks or points are
// supplied in, because the scorecard's whole contract is a reproducible digest.
func TestMetricsAreIndependentOfInputOrder(t *testing.T) {
	var ref, hyp []TrackSeries
	for i := 0; i < 6; i++ {
		ref = append(ref, straight(string(rune('a'+i)), i, 10, float32(i), float32(i*4)))
		hyp = append(hyp, straight(string(rune('A'+i)), i, 10, float32(i)+0.1, float32(i*4)))
	}
	encode := func(r, h []TrackSeries) string {
		m := ComputeTrackMetrics(r, h, 1.0)
		o := ComputeHOTA(r, h, 1.0, nil)
		b, err := json.Marshal(struct {
			M TrackMetrics
			H HOTAResult
		}{m, o})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	want := encode(ref, hyp)
	rng := rand.New(rand.NewSource(11))
	for trial := 0; trial < 5; trial++ {
		r := append([]TrackSeries(nil), ref...)
		h := append([]TrackSeries(nil), hyp...)
		rng.Shuffle(len(r), func(i, j int) { r[i], r[j] = r[j], r[i] })
		rng.Shuffle(len(h), func(i, j int) { h[i], h[j] = h[j], h[i] })
		for k := range r {
			pts := append([]SeriesPoint(nil), r[k].Points...)
			rng.Shuffle(len(pts), func(i, j int) { pts[i], pts[j] = pts[j], pts[i] })
			r[k].Points = pts
		}
		if got := encode(r, h); got != want {
			t.Fatalf("trial %d: metrics changed with input order", trial)
		}
	}
}

func TestEmptyInputsAreSafe(t *testing.T) {
	if m := ComputeTrackMetrics(nil, nil, 1.0); m.MOTA != 0 || m.NumFrames != 0 {
		t.Errorf("empty input gave %+v", m)
	}
	h := ComputeHOTA(nil, nil, 1.0, nil)
	if h.HOTA != 0 || len(h.PerAlpha) != len(DefaultHOTAAlphas()) {
		t.Errorf("empty input gave HOTA %v with %d alpha rows", h.HOTA, len(h.PerAlpha))
	}
	if _, err := json.Marshal(h); err != nil {
		t.Fatal(err)
	}
}
