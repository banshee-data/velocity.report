package l4perception

import (
	"math"
	"reflect"
	"testing"
	"time"
)

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
	for _, name := range []string{"ambiguous-neighbour", "outside-prior", "gap-too-long", "shape-filter", "missing-membership", "nonfinite-prior", "duplicate-prior", "no-prior"} {
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
