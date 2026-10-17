package l4perception

import (
	"math"
	"testing"
)

func TestRectOverlapShare(t *testing.T) {
	a := Rect{Length: 4, Width: 2}
	for _, c := range []struct {
		name string
		b    Rect
		want float64
	}{
		{"the same rectangle", a, 1},
		{"half along its length", Rect{X: 2, Length: 4, Width: 2}, 0.5},
		{"apart", Rect{X: 5, Length: 4, Width: 2}, 0},
		{"a small rectangle inside", Rect{X: 0.5, HeadingRad: 0.3, Length: 1, Width: 0.5}, 1},
		{"a quarter turn across", Rect{HeadingRad: math.Pi / 2, Length: 2, Width: 2}, 1},
		{"a quarter turn of itself", Rect{HeadingRad: math.Pi / 2, Length: 4, Width: 2}, 0.5},
		{"no area", Rect{Length: 4}, 0},
	} {
		if got := RectOverlapShare(a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: %.4f, want %.4f", c.name, got, c.want)
		}
		if got := RectOverlapShare(c.b, a); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s, the other way: %.4f, want %.4f", c.name, got, c.want)
		}
	}
	if got := RectIntersectionArea(a, Rect{HeadingRad: math.Pi / 4, Length: 4, Width: 2}); got <= 4 || got >= 8 {
		t.Errorf("an eighth turn of itself shares %.3f m², want between 4 and 8", got)
	}
}
