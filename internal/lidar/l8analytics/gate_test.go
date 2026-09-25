package l8analytics

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

// The fixed gate is what every caller before the footprint gate used, and its
// numbers must not move: a footprint on the reference is invisible to it.
func TestFixedGateIgnoresFootprintAndMatchesTheOldEntryPoints(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var ref, hyp []TrackSeries
	for i := 0; i < 5; i++ {
		r := straight(string(rune('a'+i)), i, 12, float32(i), float32(i*3))
		for k := range r.Points {
			r.Points[k].FootprintDiagonalMetres = float32(1 + rng.Float64()*5)
			r.Points[k].Ignore = rng.Intn(9) == 0
		}
		ref = append(ref, r)
		h := straight(string(rune('A'+i)), i+rng.Intn(3), 10, float32(i), float32(i*3))
		for k := range h.Points {
			h.Points[k].X += float32(rng.NormFloat64() * 0.4)
			h.Points[k].Y += float32(rng.NormFloat64() * 0.4)
		}
		hyp = append(hyp, h)
	}
	bare := make([]TrackSeries, len(ref))
	for i, r := range ref {
		bare[i] = TrackSeries{ID: r.ID, Points: append([]SeriesPoint(nil), r.Points...)}
		for k := range bare[i].Points {
			bare[i].Points[k].FootprintDiagonalMetres = 0
		}
	}

	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, d := range []float64{0.5, 1, 2} {
		old := encode(ComputeTrackMetrics(ref, hyp, d))
		if got := encode(ComputeTrackMetricsGated(ref, hyp, FixedGate(d))); got != old {
			t.Fatalf("gate %v: gated CLEAR MOT %s differs from %s", d, got, old)
		}
		if got := encode(ComputeTrackMetrics(bare, hyp, d)); got != old {
			t.Fatalf("gate %v: a footprint changed the fixed-gate result", d)
		}
		oldH := encode(ComputeHOTA(ref, hyp, d, nil))
		if got := encode(ComputeHOTAGated(ref, hyp, FixedGate(d), nil)); got != oldH {
			t.Fatalf("gate %v: gated HOTA differs from ComputeHOTA", d)
		}
	}
}

// The D2 A/B gate: one metre plus half the object's own footprint diagonal. A
// bus 3 m off its centre is still the bus; a pedestrian 2 m off is not.
func TestFootprintGateScalesWithTheObject(t *testing.T) {
	bus := straight("bus", 0, 5, 0, 0)
	ped := straight("ped", 0, 5, 0, 50)
	for k := range bus.Points {
		bus.Points[k].FootprintDiagonalMetres = 6 // gate 1 + 3 = 4 m
		ped.Points[k].FootprintDiagonalMetres = 0.5
	}
	ref := []TrackSeries{bus, ped}
	hyp := []TrackSeries{straight("hb", 0, 5, 0, 3), straight("hp", 0, 5, 0, 52)}

	footprint := ComputeTrackMetricsGated(ref, hyp, FootprintGate(1))
	if footprint.Matches != 5 || footprint.FN != 5 || footprint.FP != 5 {
		t.Fatalf("footprint gate: %+v; want the bus matched on all 5 frames and the pedestrian missed", footprint)
	}
	fixed := ComputeTrackMetricsGated(ref, hyp, FixedGate(1))
	if fixed.Matches != 0 || fixed.FN != 10 {
		t.Fatalf("fixed 1 m gate: %+v; want nothing matched", fixed)
	}

	// HOTA's similarity reaches zero at the object's own gate: the bus pair
	// is 3 m apart under a 4 m gate, similarity 0.25.
	h := ComputeHOTAGated([]TrackSeries{bus}, []TrackSeries{hyp[0]}, FootprintGate(1), []float64{0.2, 0.3})
	if h.PerAlpha[0].TP != 5 || h.PerAlpha[1].TP != 0 || h.PerAlpha[1].FN != 5 {
		t.Fatalf("footprint HOTA rows %+v; want matched at alpha 0.2 and not at 0.3", h.PerAlpha)
	}

	// A reference with no recorded footprint gets the slack alone.
	plain := straight("plain", 0, 5, 0, 0)
	if m := ComputeTrackMetricsGated([]TrackSeries{plain}, []TrackSeries{straight("h", 0, 5, 0, 1.5)}, FootprintGate(1)); m.Matches != 0 {
		t.Fatalf("no footprint, 1.5 m off, 1 m slack: %+v; want no match", m)
	}
}

func TestMatchGateValidate(t *testing.T) {
	for _, tc := range []struct {
		gate MatchGate
		ok   bool
	}{
		{FixedGate(1), true},
		{FixedGate(0), false},
		{FixedGate(-1), false},
		{FootprintGate(1), true},
		{FootprintGate(0), true},
		{FootprintGate(-0.5), false},
		{MatchGate{Kind: "elliptical", Metres: 1}, false},
		{FixedGate(math.NaN()), false},
		{FootprintGate(math.Inf(1)), false},
	} {
		if err := tc.gate.Validate(); (err == nil) != tc.ok {
			t.Errorf("%+v: Validate() = %v, want ok=%v", tc.gate, err, tc.ok)
		}
	}
	if got := FootprintGate(1).String(); got != "1 m + half footprint diagonal" {
		t.Errorf("FootprintGate(1).String() = %q", got)
	}
	if got := FixedGate(2).String(); got != "2 m fixed" {
		t.Errorf("FixedGate(2).String() = %q", got)
	}
}
