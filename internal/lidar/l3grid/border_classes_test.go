package l3grid

import (
	"math/rand"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

func ring(ch int, ranges []float64) []l2frames.PointPolar {
	out := make([]l2frames.PointPolar, len(ranges))
	for i, r := range ranges {
		out[i] = l2frames.PointPolar{Channel: ch, Azimuth: 10 + 0.2*float64(i), Distance: r}
	}
	return out
}

func TestBorderTraitsVeilObstacleShadow(t *testing.T) {
	// a body at 10 m, one mixed return at 20 m, a wall at 30 m
	pts := ring(5, []float64{10, 10, 10, 20, 30, 30, 30})
	tr := BorderTraits(pts, DefaultBorderParams())
	if tr[3]&BorderVeil == 0 {
		t.Fatalf("mixed return not veil: %08b", tr[3])
	}
	if tr[2]&BorderObstacleRight == 0 || tr[2]&BorderVeil != 0 {
		t.Fatalf("near edge: %08b", tr[2])
	}
	if tr[4]&BorderShadowLeft == 0 || tr[4]&BorderVeil != 0 {
		t.Fatalf("far side: %08b", tr[4])
	}
	if tr[0] != 0 || tr[6] != 0 {
		t.Fatalf("interior carries traits: %08b %08b", tr[0], tr[6])
	}
}

func TestBorderTraitsThinObjectIsNotVeil(t *testing.T) {
	// a pole: one near return between two far ones is an obstacle on both sides
	tr := BorderTraits(ring(1, []float64{30, 30, 12, 30, 30}), DefaultBorderParams())
	if tr[2]&BorderVeil != 0 || tr[2]&(BorderObstacleLeft|BorderObstacleRight) != BorderObstacleLeft|BorderObstacleRight {
		t.Fatalf("pole: %08b", tr[2])
	}
}

func TestBorderTraitsGapIsUnknown(t *testing.T) {
	pts := ring(1, []float64{10, 10, 20, 30, 30})
	pts[3].Azimuth += 2 // the right neighbour of the middle return is 2 degrees away
	pts[4].Azimuth += 2
	tr := BorderTraits(pts, DefaultBorderParams())
	if tr[2]&BorderVeil != 0 {
		t.Fatalf("veil across a gap: %08b", tr[2])
	}
}

func TestBorderTraitsMissingReturnsAndOrder(t *testing.T) {
	pts := ring(2, []float64{10, 10, 10, 20, 30, 30, 0, 30})
	want := BorderTraits(pts, DefaultBorderParams())
	rng := rand.New(rand.NewSource(1))
	perm := rng.Perm(len(pts))
	shuf := make([]l2frames.PointPolar, len(pts))
	for i, j := range perm {
		shuf[i] = pts[j]
	}
	got := BorderTraits(shuf, DefaultBorderParams())
	for i, j := range perm {
		if got[i] != want[j] {
			t.Fatalf("order dependent at %d: %08b vs %08b", j, got[i], want[j])
		}
	}
	if want[6] != 0 {
		t.Fatalf("missing return took traits")
	}
}

func TestWithholdVeilClearsOnlyForeground(t *testing.T) {
	pts := ring(3, []float64{10, 10, 10, 20, 30, 30, 30})
	mask := []bool{true, true, true, true, true, false, false}
	out, n := WithholdVeil(pts, mask, DefaultBorderParams())
	if n != 1 || out[3] || !out[2] || !mask[3] {
		t.Fatalf("withheld %d out=%v mask=%v", n, out, mask)
	}
	bg := []bool{false, false, false, false, false, false, false}
	if _, n := WithholdVeil(pts, bg, DefaultBorderParams()); n != 0 {
		t.Fatal("background return withheld")
	}
}
