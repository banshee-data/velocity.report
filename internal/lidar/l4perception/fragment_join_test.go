package l4perception

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func bruteFragmentsNear(a, b []WorldPoint, gap float64) bool {
	for _, p := range a {
		for _, q := range b {
			dx, dy := p.X-q.X, p.Y-q.Y
			if dx*dx+dy*dy <= gap*gap {
				return true
			}
		}
	}
	return false
}

func TestFragmentGridMatchesExactDistance(t *testing.T) {
	rng := rand.New(rand.NewSource(731))
	for trial := 0; trial < 500; trial++ {
		a, b := make([]WorldPoint, rng.Intn(40)), make([]WorldPoint, rng.Intn(40))
		for _, points := range [][]WorldPoint{a, b} {
			for i := range points {
				points[i] = WorldPoint{X: rng.Float64()*20 - 10, Y: rng.Float64()*20 - 10}
			}
		}
		gap := .05 + rng.Float64()*2
		if got, want := fragmentsNear(a, b, gap), bruteFragmentsNear(a, b, gap); got != want {
			t.Fatalf("trial %d: grid=%v exact=%v", trial, got, want)
		}
	}
	// Include exact-distance matches across positive and negative cell edges,
	// and a diagonal neighbour that is outside the circular threshold.
	for _, x := range []float64{-2, -.5, 0, .5, 2} {
		a := []WorldPoint{{X: x, Y: -.5}}
		for _, q := range []WorldPoint{{X: x + .5, Y: -.5}, {X: x, Y: 0}, {X: x + .5, Y: 0}} {
			b := []WorldPoint{q}
			if fragmentsNear(a, b, .5) != bruteFragmentsNear(a, b, .5) {
				t.Fatalf("boundary mismatch: %v %v", a, b)
			}
		}
	}
}

func BenchmarkFragmentGapLargeMembership(b *testing.B) {
	a, other := make([]WorldPoint, 2048), make([]WorldPoint, 4096)
	for i := range a {
		a[i] = WorldPoint{X: float64(i%64) * .01, Y: float64(i/64) * .01}
	}
	for i := range other {
		other[i] = WorldPoint{X: 2 + float64(i%64)*.01, Y: float64(i/64) * .01}
	}
	for _, tc := range []struct {
		name string
		near func([]WorldPoint, []WorldPoint, float64) bool
	}{{"grid", fragmentsNear}, {"all-pairs", bruteFragmentsNear}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if tc.near(a, other, .5) {
					b.Fatal("separated fragments joined")
				}
			}
		})
	}
}

func fragmentFixture(t *testing.T) ([]WorldCluster, DBSCANTrace, DBSCANParams, FragmentHint) {
	t.Helper()
	p := DefaultDBSCANParams()
	p.Eps, p.MinPts, p.KeepMembers, p.MaxSamplePoints = .25, 2, true, 3
	var pts []WorldPoint
	for _, start := range []float64{0, 1.4} {
		for i := 0; i < 3; i++ {
			for j := 0; j < 2; j++ {
				point := WorldPoint{X: start + float64(i)*.1, Y: float64(j) * .1, Z: .8, SensorID: "join", Timestamp: time.Unix(1_700_000_000, 0)}
				point.SetSourceOrdinal(len(pts))
				pts = append(pts, point)
			}
		}
	}
	c, trace := DBSCANWithTrace(pts, p)
	if len(c) != 2 {
		t.Fatalf("fixture has %d clusters", len(c))
	}
	return c, trace, p, FragmentHint{ID: "vehicle", Rect: Rect{X: .8, Y: .05, Length: 2, Width: 1}}
}

func TestMotionJoinPreservesCompleteMembershipAndInput(t *testing.T) {
	c, trace, p, hint := fragmentFixture(t)
	labels := append([]int(nil), trace.Labels...)
	out, joinedTrace, joined := JoinMotionFragments(c, trace, p, []FragmentHint{hint}, 2, .15)
	if joined != 1 || len(out) != 1 || out[0].PointsCount != 12 || len(out[0].Members) != 12 || len(out[0].RetainedPoints) != 3 {
		t.Fatalf("joined=%d clusters=%+v", joined, out)
	}
	if !reflect.DeepEqual(trace.Labels, labels) {
		t.Fatal("joining mutated the original partition")
	}
	seen := map[int]bool{}
	for _, point := range out[0].Members {
		ordinal, ok := point.SourceOrdinal()
		if !ok || seen[ordinal] {
			t.Fatal("lost or duplicated return lineage")
		}
		seen[ordinal] = true
	}
	for _, label := range joinedTrace.Labels {
		if label != int(out[0].ClusterID) {
			t.Fatal("trace disagrees with merged detection")
		}
	}
	// Canonical component identity does not depend on the order clusters
	// were presented by a caller or on map traversal.
	c[0], c[1] = c[1], c[0]
	other, _, _ := JoinMotionFragments(c, trace, p, []FragmentHint{hint}, 2, .15)
	if len(other) != 1 || other[0].ClusterID != out[0].ClusterID {
		t.Fatal("input order changed joined identity")
	}
}

func TestMotionJoinRefusesAmbiguityAndPreservesRejectedGroups(t *testing.T) {
	for _, name := range []string{"ambiguous-neighbour", "outside-prior", "gap-too-long", "shape-filter", "missing-membership", "nonfinite-prior", "duplicate-prior", "no-prior", "invalid-id", "duplicate-id", "bad-label", "nonfinite-member"} {
		t.Run(name, func(t *testing.T) {
			c, tr, p, h := fragmentFixture(t)
			hints := []FragmentHint{h}
			gap := 2.0
			switch name {
			case "ambiguous-neighbour":
				hints = append(hints, FragmentHint{ID: "neighbour", Rect: Rect{X: 1.6, Y: .05, Length: .2, Width: .2}})
			case "outside-prior":
				hints[0].Rect.Length = .5
			case "gap-too-long":
				gap = .5
			case "shape-filter":
				p.MaxClusterDiameter = 1
			case "missing-membership":
				c[0].PointsCount++
			case "nonfinite-prior":
				hints[0].Rect.X = math.NaN()
			case "duplicate-prior":
				hints = append(hints, h)
			case "no-prior":
				hints = nil
			case "invalid-id":
				c[0].ClusterID = int64(len(tr.Processed) + 1)
			case "duplicate-id":
				c[1].ClusterID = c[0].ClusterID
			case "bad-label":
				tr.Labels = append([]int(nil), tr.Labels...)
				tr.Labels[0] = len(tr.Processed) + 1
			case "nonfinite-member":
				tr.Processed = append([]WorldPoint(nil), tr.Processed...)
				tr.Processed[0].Z = math.NaN()
			}
			out, trace, joined := JoinMotionFragments(c, tr, p, hints, gap, .15)
			if joined != 0 || !reflect.DeepEqual(out, c) || !reflect.DeepEqual(trace, tr) {
				t.Fatalf("unsafe join or dropped original components: %s", name)
			}
		})
	}
}

func TestMotionJoinDoesNotRecoverNoiseOrRejectedComponents(t *testing.T) {
	c, tr, p, h := fragmentFixture(t)
	noise := WorldPoint{X: .8, Y: .05, Z: .8}
	noise.SetSourceOrdinal(12)
	tr.Processed = append(append([]WorldPoint(nil), tr.Processed...), noise)
	tr.Labels = append(append([]int(nil), tr.Labels...), -1)
	out, next, joined := JoinMotionFragments(c, tr, p, []FragmentHint{h}, 2, .15)
	if joined != 1 || len(out) != 1 || out[0].PointsCount != 12 || next.Labels[12] != -1 {
		t.Fatal("joining turned noise into an observation")
	}
}

// One rejected group must not erase its original components while another
// valid group in the same frame joins. The high original component ID must
// survive even though the first trial partition no longer contains it.
func TestMotionJoinMixedShapeOutcomesPreserveEveryReturn(t *testing.T) {
	_, trace, params, hint := fragmentFixture(t)
	points := append([]WorldPoint(nil), trace.Processed...)
	for _, p := range trace.Processed {
		if p.X < 1 {
			p.X += 10
		} else {
			p.X += 10.7
		}
		p.SetSourceOrdinal(len(points))
		points = append(points, p)
	}
	params.MaxClusterDiameter = 2
	clusters, trace := DBSCANWithTrace(points, params)
	if len(clusters) != 4 {
		t.Fatalf("setup has %d clusters", len(clusters))
	}
	hints := []FragmentHint{hint, {ID: "second", Rect: Rect{X: 11.15, Y: .05, Length: 3, Width: 1}}}
	out, next, joined := JoinMotionFragments(clusters, trace, params, hints, 2, .15)
	if len(out) != 3 || joined != 1 {
		t.Fatalf("joined %d, components %d", joined, len(out))
	}
	returns := 0
	ids := map[int64]bool{}
	for _, c := range out {
		returns += c.PointsCount
		ids[c.ClusterID] = true
	}
	if returns != 24 || !ids[3] || !ids[4] {
		t.Fatal("rejected group lost original components or members")
	}
	for _, label := range next.Labels {
		if !ids[int64(label)] {
			t.Fatal("trace names a missing component")
		}
	}
}
