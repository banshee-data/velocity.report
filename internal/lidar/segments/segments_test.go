package segments

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

const testBase int64 = 1788466680 * 1_000_000_000

func moving(id string, frames int, x0 float64, first int) []Point {
	out := make([]Point, 0, frames)
	for i := 0; i < frames; i++ {
		out = append(out, Point{Track: id, TimeNs: testBase + int64(first+i)*100_000_000, X: x0 + float64(i), VX: 10, MaxSpeed: 10})
	}
	return out
}

func rank(t *testing.T, points []Point, finder, role string) []Window {
	t.Helper()
	out, err := Find(points, finder, "source", role, DefaultParams(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFollowingScriptParity(t *testing.T) {
	points := append(moving("1", 30, 0, 0), moving("2", 30, 10, 0)...)
	w := rank(t, points, "following", "tuning")
	if len(w) != 1 {
		t.Fatalf("windows=%d", len(w))
	}
	if w[0].StartNs != testBase || w[0].PairFrames != 30 || w[0].PairSeconds != 3 || w[0].Pairs != 1 || w[0].Leaders != 1 || w[0].LeaderChanges != 0 || w[0].ClosestGapM != 10 {
		t.Fatalf("unexpected script parity: %+v", w[0])
	}
	// The same source, parameters and window must keep its identity across reads.
	if again := rank(t, points, "following", "tuning"); again[0].ID != w[0].ID {
		t.Fatal("unstable segment ID")
	}
}

func TestLeaderChangesAndExclusions(t *testing.T) {
	points := append(moving("f", 20, 0, 0), moving("a", 10, 10, 0)...)
	points = append(points, moving("b", 10, 20, 10)...)
	w := rank(t, points, "following", "tuning")
	if len(w) != 1 || w[0].Pairs != 2 || w[0].Leaders != 2 || w[0].LeaderChanges != 1 || !reflect.DeepEqual(w[0].LeaderIDs, []string{"a", "b"}) {
		t.Fatalf("leader parity: %+v", w)
	}
	l := rank(t, points, "leader_changes", "tuning")
	if len(l) != 1 || l[0].Score != 1 {
		t.Fatalf("changes: %+v", l)
	}
	// Off-lane, oncoming, stationary and too far ahead cannot form pairs.
	for _, mod := range []func(*Point){func(p *Point) { p.Y = 4 }, func(p *Point) { p.VX = -10 }, func(p *Point) { p.VX = 0 }, func(p *Point) { p.X += 60 }} {
		other := moving("other", 10, 10, 0)
		for i := range other {
			mod(&other[i])
		}
		if got := rank(t, append(moving("f", 10, 0, 0), other...), "following", "tuning"); len(got) != 0 {
			t.Fatalf("excluded pair was ranked: %+v", got)
		}
	}
}

func TestRankingUsesExactFramesAndCaptureOffset(t *testing.T) {
	points := []Point{}
	for _, v := range []struct {
		start int64
		n     int
	}{{0, 2}, {10_000_000_000, 3}} {
		for i := 0; i < v.n; i++ {
			ts := testBase + v.start + int64(i)*4_000_000
			points = append(points, Point{Track: "1", TimeNs: ts, VX: 10}, Point{Track: "2", TimeNs: ts, X: 10, VX: 10})
		}
	}
	w, err := Find(points, "following", "s", "tuning", DefaultParams(), []Capture{{Path: "capture.pcap", FirstNs: testBase - 1_000_000_000, LastNs: testBase + 20_000_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 2 || w[0].PairFrames != 3 || w[1].PairFrames != 2 || w[0].PairSeconds != 0.01 || w[1].PairSeconds != 0.01 || w[0].OffsetSeconds != 11 {
		t.Fatalf("ranking or offset: %+v", w)
	}
}

func TestLateralJumpScriptParityAndGap(t *testing.T) {
	points := []Point{}
	for i := 0; i < 5; i++ {
		y := 0.0
		if i == 2 {
			y = 1
		}
		points = append(points, Point{Track: "jump", TimeNs: int64(i) * 100_000_000, X: float64(i), Y: y, VX: 10, MaxSpeed: 10})
	}
	res, ok := lateralResidual(points)
	if !ok || math.Abs(res-0.8) > 1e-9 {
		t.Fatalf("residual=%v ok=%t", res, ok)
	}
	w := rank(t, points, "lateral_jump", "tuning")
	if len(w) != 1 || w[0].PeakNs != 200_000_000 || w[0].Score != 0.8 {
		t.Fatalf("jump=%+v", w)
	}
	points[3].TimeNs += 1_000_000_000
	points[4].TimeNs += 1_000_000_000
	if got := rank(t, points, "lateral_jump", "tuning"); len(got) != 0 {
		t.Fatalf("gap accepted: %+v", got)
	}
}

func TestRoleAndRandomSelection(t *testing.T) {
	points := append(moving("1", 5, 0, 0), moving("2", 5, 10, 0)...)
	for _, finder := range []string{"leader_changes", "lateral_jump", "split_flags"} {
		if _, err := Find(points, finder, "s", "held_out", DefaultParams(), nil); err == nil {
			t.Fatalf("held-out accepted %s", finder)
		}
	}
	for _, finder := range []string{"following", "exposure", "random"} {
		if _, err := Find(points, finder, "s", "held_out", DefaultParams(), nil); err != nil {
			t.Fatalf("held-out refused %s: %v", finder, err)
		}
	}
	a := rank(t, points, "random", "held_out")
	b := rank(t, points, "random", "held_out")
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(aa) != string(bb) {
		t.Fatal("seeded random ranking changed")
	}
}

func TestParameterAndRecordValidation(t *testing.T) {
	p := DefaultParams()
	p.WindowSeconds = 0
	if _, err := Find(nil, "following", "s", "tuning", p, nil); err == nil {
		t.Fatal("accepted zero window")
	}
	p = DefaultParams()
	p.MaxGap = math.NaN()
	if _, err := Find(nil, "following", "s", "tuning", p, nil); err == nil {
		t.Fatal("accepted NaN")
	}
	r := Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: "digest", Role: "held_out", Finder: "lateral_jump", FinderVersion: 1, Parameters: DefaultParams(), Segment: Window{StartNs: 1, EndNs: 2}}
	if err := r.Validate(); err == nil {
		t.Fatal("failure-chosen held-out pack")
	}
	r.Finder = "random"
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
