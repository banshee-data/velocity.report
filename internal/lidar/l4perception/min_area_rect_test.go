package l4perception

import (
	"math"
	"testing"
)

func rectOutline(cx, cy, psi, length, width float64) []WorldPoint {
	c, s := math.Cos(psi), math.Sin(psi)
	var out []WorldPoint
	add := func(a, n float64) { out = append(out, WorldPoint{X: cx + a*c - n*s, Y: cy + a*s + n*c}) }
	for i := 0; i <= 40; i++ {
		a := -length/2 + length*float64(i)/40
		add(a, -width/2)
		add(a, width/2)
	}
	for i := 1; i < 16; i++ {
		n := -width/2 + width*float64(i)/16
		add(-length/2, n)
		add(length/2, n)
	}
	return out
}

func TestMinAreaRectangleRecoversAnOutline(t *testing.T) {
	for _, psi := range []float64{0, 0.4, 1.2, 2.0, -0.7} {
		r, ok := MinAreaRectangle(rectOutline(12, -3, psi, 4.5, 1.8))
		if !ok {
			t.Fatalf("psi %.1f: no rectangle", psi)
		}
		if FoldAxisRad(r.AxisRad, psi)*180/math.Pi > 0.1 {
			t.Fatalf("psi %.1f: axis %.3f", psi, r.AxisRad)
		}
		long, short := math.Max(r.Span1, r.Span2), math.Min(r.Span1, r.Span2)
		if math.Abs(long-4.5) > 1e-6 || math.Abs(short-1.8) > 1e-6 || r.AxisRad < 0 || r.AxisRad >= math.Pi/2 {
			t.Fatalf("psi %.1f: %+v", psi, r)
		}
	}
}

func TestMinAreaRectangleSpansFollowTheFold(t *testing.T) {
	// A body along 100°: the folded axis is 10°, across the length.
	r, _ := MinAreaRectangle(rectOutline(0, 8, 100*math.Pi/180, 4.5, 1.8))
	if math.Abs(r.AxisRad-10*math.Pi/180) > 1e-6 || math.Abs(r.Span1-1.8) > 1e-6 || math.Abs(r.Span2-4.5) > 1e-6 {
		t.Fatalf("got %+v", r)
	}
}

func TestMinAreaRectangleRefusesTooFewPoints(t *testing.T) {
	if _, ok := MinAreaRectangle([]WorldPoint{{X: 1}, {X: 2}}); ok {
		t.Fatal("two points made a rectangle")
	}
	if _, ok := MinAreaRectangle([]WorldPoint{{X: 1}, {X: 2}, {X: 3}}); ok {
		t.Fatal("collinear points made a rectangle")
	}
}
