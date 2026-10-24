package l5tracks

import (
	"math"
	"testing"
)

func TestPublishedBoxFallsBackToTheOBB(t *testing.T) {
	tr := &TrackedObject{X: 1, Y: 2, LatestZ: 0.8, OBBLength: 4, OBBWidth: 2, OBBHeight: 1.6, OBBHeadingRad: 0.5}
	got := tr.PublishedBox()
	want := PublishedBox{X: 1, Y: 2, Z: 0.8, Length: 4, Width: 2, Height: 1.6, HeadingRad: 0.5}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPublishedBoxPrefersTheBoxModel(t *testing.T) {
	tr := &TrackedObject{
		X: 1, Y: 2, LatestZ: 0.8, OBBLength: 4, OBBWidth: 2, OBBHeight: 1.6, OBBHeadingRad: 0.5,
		BoxX: 1.2, BoxY: 2.1, BoxLength: 4.5, BoxWidth: 1.9, BoxHeadingRad: 0.4,
	}
	got := tr.PublishedBox()
	want := PublishedBox{X: 1.2, Y: 2.1, Z: 0.8, Length: 4.5, Width: 1.9, Height: 1.6, HeadingRad: 0.4}
	if got != want {
		t.Fatalf("plan from the box model: got %+v, want %+v", got, want)
	}
	// A fitted height stands the box on its base: Z is the middle.
	tr.BoxZ, tr.BoxHeight = -0.2, 1.5
	got = tr.PublishedBox()
	if got.Height != 1.5 || math.Abs(float64(got.Z)-0.55) > 1e-6 {
		t.Fatalf("height from the box model: got %+v, want height 1.5, z 0.55", got)
	}
}

func TestPublishedBoxContains(t *testing.T) {
	// A 4 x 2 x 2 m box at the origin, turned 90 degrees: its length runs along y.
	b := PublishedBox{Length: 4, Width: 2, Height: 2, HeadingRad: math.Pi / 2}
	cases := []struct {
		name            string
		x, y, z         float64
		marginXY, margZ float64
		want            bool
	}{
		{"centre", 0, 0, 0, 0, 0, true},
		{"along its length", 0, 1.9, 0, 0, 0, true},
		{"past its end", 0, 2.1, 0, 0, 0, false},
		{"past its side", 1.1, 0, 0, 0, 0, false},
		{"past its side, inside the margin", 1.1, 0, 0, 0.2, 0, true},
		{"above its top", 0, 0, 1.1, 0, 0, false},
		{"above its top, inside the margin", 0, 0, 1.1, 0, 0.2, true},
		{"below its base, never grown down", 0, 0, -1.1, 0.5, 0.5, false},
	}
	for _, c := range cases {
		if got := b.Contains(c.x, c.y, c.z, c.marginXY, c.margZ); got != c.want {
			t.Errorf("%s: Contains(%g, %g, %g) = %v, want %v", c.name, c.x, c.y, c.z, got, c.want)
		}
	}
}
