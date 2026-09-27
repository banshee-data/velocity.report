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

func TestFollowingPeakIsTheBusiestPairFrame(t *testing.T) {
	first := testBase
	second := first + 100_000_000
	points := []Point{
		{Track: "a", TimeNs: first, VX: 10}, {Track: "b", TimeNs: first, X: 10, VX: 10},
		{Track: "a", TimeNs: second, VX: 10}, {Track: "b", TimeNs: second, X: 10, VX: 10},
		{Track: "c", TimeNs: second, X: 20, VX: 10},
	}
	windows := rank(t, points, "following", "tuning")
	if len(windows) != 1 || windows[0].PeakNs != second {
		t.Fatalf("following peak: %+v", windows)
	}
}

// laneWithLeaderChange is one follower whose nearest leader changes at the
// given frame, in a lane far enough from the others to pair with nobody else.
func laneWithLeaderChange(prefix string, y float64, change int) []Point {
	points := moving(prefix+"-follower", 20, 0, 0)
	points = append(points, moving(prefix+"-first", change, 10, 0)...)
	points = append(points, moving(prefix+"-second", 20-change, float64(change)+10, change)...)
	for i := range points {
		points[i].Y = y
	}
	return points
}

func TestLeaderChangePeakDoesNotDependOnMapOrder(t *testing.T) {
	points := append(laneWithLeaderChange("near", 0, 10), laneWithLeaderChange("far", 20, 5)...)
	earliest := testBase + 5*100_000_000
	// Go randomises map iteration, so one agreeing read proves nothing.
	for attempt := 0; attempt < 64; attempt++ {
		w := rank(t, points, "leader_changes", "tuning")
		if len(w) != 1 || w[0].LeaderChanges != 2 {
			t.Fatalf("leader changes: %+v", w)
		}
		if w[0].PeakNs != earliest {
			t.Fatalf("attempt %d: peak %d, want the earliest change %d", attempt, w[0].PeakNs, earliest)
		}
	}
	// Three followers change at the later frame and one at the earlier: the
	// busiest change frame is the peak, not the first.
	busy := laneWithLeaderChange("a", 0, 10)
	busy = append(busy, laneWithLeaderChange("b", 20, 10)...)
	busy = append(busy, laneWithLeaderChange("c", 40, 10)...)
	busy = append(busy, laneWithLeaderChange("d", 60, 5)...)
	for attempt := 0; attempt < 64; attempt++ {
		w := rank(t, busy, "leader_changes", "tuning")
		if len(w) != 1 || w[0].LeaderChanges != 4 || w[0].PeakNs != testBase+10*100_000_000 {
			t.Fatalf("attempt %d: busiest change frame: %+v", attempt, w)
		}
	}
}

func TestFollowingPeakIgnoresLeaderChanges(t *testing.T) {
	// A third vehicle joins the far lane at frame 12, so that frame has the
	// most pairs. Leader changes at frames 5 and 10 must not move the peak.
	points := append(laneWithLeaderChange("near", 0, 10), laneWithLeaderChange("far", 20, 5)...)
	extra := moving("far-third", 1, 32, 12)
	extra[0].Y = 20
	points = append(points, extra...)
	busiest := testBase + 12*100_000_000
	for attempt := 0; attempt < 64; attempt++ {
		w := rank(t, points, "following", "tuning")
		if len(w) != 1 || w[0].LeaderChanges != 2 || w[0].PeakNs != busiest {
			t.Fatalf("attempt %d: following peak: %+v", attempt, w)
		}
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
	for _, finder := range []string{"following", "exposure"} {
		if _, err := Find(points, finder, "s", "held_out", DefaultParams(), nil); err != nil {
			t.Fatalf("held-out refused %s: %v", finder, err)
		}
	}
	if _, err := Find(points, "random", "s", "held_out", DefaultParams(), nil); err == nil {
		t.Fatal("held-out random selection used tracker observations without a capture timeline")
	}
	captures := []Capture{{Path: "a.pcap", FirstNs: testBase, LastNs: testBase + 80_000_000_000}, {Path: "b.pcap", FirstNs: testBase + 100_000_000_000, LastNs: testBase + 180_000_000_000}}
	a, err := Find(nil, "random", "s", "held_out", DefaultParams(), captures)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Find(nil, "random", "s", "held_out", DefaultParams(), captures)
	if err != nil {
		t.Fatal(err)
	}
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(aa) != string(bb) {
		t.Fatal("seeded random ranking changed")
	}
	if len(a) != 2 || a[0].Capture == a[1].Capture {
		t.Fatalf("want one random window per capture: %+v", a)
	}
	for _, w := range a {
		if w.OffsetSeconds < 35 || w.EndNs-w.StartNs != 10_000_000_000 {
			t.Fatalf("unsettled or incomplete random window: %+v", w)
		}
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
	r.Segment = Window{ID: Identity("random", "s", "held_out", r.Parameters, 1), Finder: "random", Version: Version, Source: "s", Role: "held_out", StartNs: 1, EndNs: 10_000_000_001}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Parameters.RandomSeed++
	if err := r.Validate(); err == nil {
		t.Fatal("accepted a segment with changed finder parameters")
	}
}

func TestFinderRejectsAmbiguousInputsAndKeepsNegativeTimeWindows(t *testing.T) {
	if _, err := Find(nil, "unknown", "s", "tuning", DefaultParams(), nil); err == nil {
		t.Fatal("accepted an unknown finder")
	}
	if _, err := Find(nil, "following", "s", "unknown", DefaultParams(), nil); err == nil {
		t.Fatal("accepted an unknown pack role")
	}
	if _, err := Find(nil, "following", "", "tuning", DefaultParams(), nil); err == nil {
		t.Fatal("accepted an unnamed source")
	}
	badCapture := []Capture{{Path: "bad.pcap", FirstNs: 2, LastNs: 1}}
	if _, err := Find(nil, "random", "s", "held_out", DefaultParams(), badCapture); err == nil {
		t.Fatal("accepted an inverted capture extent")
	}
	points := []Point{{Track: "one", TimeNs: -1, VX: 10}}
	windows, err := Find(points, "exposure", "s", "tuning", DefaultParams(), nil)
	if err != nil || len(windows) != 1 || windows[0].StartNs != -10_000_000_000 {
		t.Fatalf("negative timestamp floor: %+v, %v", windows, err)
	}
	flat := []Point{{}, {}, {}, {}, {}}
	if _, ok := lateralResidual(flat); ok {
		t.Fatal("fit accepted zero time span")
	}
	for i := range flat {
		flat[i].TimeNs = int64(i) * 100_000_000
	}
	if _, ok := lateralResidual(flat); ok {
		t.Fatal("fit accepted a stationary path")
	}
}

func TestFindersExcludeInvalidEvidenceAndKeepTiesChronological(t *testing.T) {
	invalid := []Point{{Track: "", TimeNs: testBase, VX: 10}, {Track: "bad", TimeNs: testBase, X: math.NaN(), VX: 10}}
	if got := rank(t, invalid, "exposure", "tuning"); len(got) != 0 {
		t.Fatalf("invalid observations were counted: %+v", got)
	}
	points := []Point{{Track: "one", TimeNs: testBase, VX: 0}, {Track: "two", TimeNs: testBase, VX: 10}}
	if got := rank(t, points, "split_flags", "tuning"); len(got) != 0 {
		t.Fatalf("unflagged tracks became split candidates: %+v", got)
	}
	if got := rank(t, points, "exposure", "tuning"); len(got) != 1 || got[0].Events != 1 {
		t.Fatalf("stationary exposure was counted: %+v", got)
	}
	if got := rank(t, append(moving("a", 5, 0, 0), moving("b", 5, 10, 0)...), "leader_changes", "tuning"); len(got) != 0 {
		t.Fatalf("stable leader was ranked as a split: %+v", got)
	}
	if got := rank(t, nil, "random", "tuning"); len(got) != 0 {
		t.Fatalf("empty tuning evidence produced a random window: %+v", got)
	}
	straight := moving("a", 5, 0, 0)
	if got := rank(t, straight, "lateral_jump", "tuning"); len(got) != 0 {
		t.Fatalf("straight track was ranked as a jump: %+v", got)
	}
	tied := []Point{{Track: "a", TimeNs: testBase, VX: 10}, {Track: "a", TimeNs: testBase + 10_000_000_000, VX: 10}}
	if got := rank(t, tied, "exposure", "tuning"); len(got) != 2 || got[0].StartNs >= got[1].StartNs {
		t.Fatalf("tied windows lost time order: %+v", got)
	}
}
