package main

import (
	"math"
	"testing"
)

// The aspect angle is E1's independent variable and the nearest corner is one
// of its candidates. An error in either would not fail anything visibly — it
// would just produce a plausible table of wrong numbers — so both are pinned
// here.

func TestFoldAspectDegMapsGeometryToZeroNinety(t *testing.T) {
	deg := func(d float64) float64 { return d * math.Pi / 180 }

	for _, tc := range []struct {
		name string
		in   float64
		want float64
	}{
		// Looking along the body axis: end-on.
		{"aligned", deg(0), 0},
		// Perpendicular: broadside.
		{"perpendicular", deg(90), 90},
		{"perpendicular negative", deg(-90), 90},
		// A body axis is undirected, so a half turn is the same geometry.
		{"half turn is identical", deg(180), 0},
		{"minus half turn", deg(-180), 0},
		// And the two lateral faces are symmetric, so 100 degrees presents the
		// same aspect as 80.
		{"obtuse folds back", deg(100), 80},
		{"obtuse folds back negative", deg(-100), 80},
		{"just past a half turn", deg(190), 10},
		{"three quarter turn", deg(270), 90},
		{"full turn", deg(360), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := foldAspectDeg(tc.in)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("foldAspectDeg(%.1f deg) = %.4f, want %.4f", tc.in*180/math.Pi, got, tc.want)
			}
			if got < 0 || got > 90 {
				t.Errorf("result %.4f is outside [0, 90]", got)
			}
		})
	}
}

func TestFoldAspectDegAlwaysLandsInRange(t *testing.T) {
	// Swept rather than sampled: a branch-cut error at some unlucky angle
	// would corrupt one bin and be very hard to spot in a table.
	for d := -720.0; d <= 720.0; d += 0.25 {
		got := foldAspectDeg(d * math.Pi / 180)
		if got < 0 || got > 90 || math.IsNaN(got) {
			t.Fatalf("foldAspectDeg(%.2f deg) = %v, outside [0, 90]", d, got)
		}
	}
}

func TestNearestCornerPicksTheCornerFacingTheSensor(t *testing.T) {
	// The sensor is at the origin. For a box out along +X with no rotation,
	// the nearest corner is the one at smaller X and whichever Y is nearer.
	const cx, cy = 10, 0
	const length, width = 4.0, 2.0

	x, y, ok := nearestCorner(cx, cy, length, width, 0)
	if !ok {
		t.Fatal("no corner returned")
	}
	// Along-axis: the rear face, at cx - length/2.
	if math.Abs(float64(x)-(cx-length/2)) > 1e-5 {
		t.Errorf("corner X = %v, want %v", x, cx-length/2)
	}
	// Across-axis the two corners are equidistant at cy = 0, so either sign is
	// admissible; what matters is the magnitude.
	if math.Abs(math.Abs(float64(y))-width/2) > 1e-5 {
		t.Errorf("corner Y = %v, want magnitude %v", y, width/2)
	}

	// It must genuinely be the nearest of the four, not merely a corner.
	cos, sin := 1.0, 0.0
	var nearest float64 = math.Inf(1)
	for _, sl := range []float64{-1, 1} {
		for _, sw := range []float64{-1, 1} {
			px := cx + sl*length/2*cos + sw*width/2*(-sin)
			py := cy + sl*length/2*sin + sw*width/2*cos
			if d := math.Hypot(px, py); d < nearest {
				nearest = d
			}
		}
	}
	if got := math.Hypot(float64(x), float64(y)); math.Abs(got-nearest) > 1e-5 {
		t.Errorf("returned corner is at %v, nearest is %v", got, nearest)
	}
}

func TestNearestCornerRotatesWithHeading(t *testing.T) {
	// A box rotated a quarter turn swaps which extent lies along the line of
	// sight, so the nearest corner must move accordingly. If heading were
	// ignored, these two would agree.
	const cx, cy = 12, 0
	const length, width = 6.0, 1.5

	ax, ay, _ := nearestCorner(cx, cy, length, width, 0)
	bx, by, _ := nearestCorner(cx, cy, length, width, float32(math.Pi/2))

	if math.Abs(float64(ax-bx)) < 1e-3 && math.Abs(float64(ay-by)) < 1e-3 {
		t.Errorf("rotating the body did not move the nearest corner: (%v,%v) and (%v,%v)", ax, ay, bx, by)
	}
	// Unrotated, the long axis points at the sensor, so the corner is
	// length/2 nearer than the centre along X.
	if math.Abs(float64(ax)-(cx-length/2)) > 1e-5 {
		t.Errorf("unrotated corner X = %v, want %v", ax, cx-length/2)
	}
	// Rotated, the short axis does, so it is only width/2 nearer.
	if math.Abs(float64(bx)-(cx-width/2)) > 1e-5 {
		t.Errorf("rotated corner X = %v, want %v", bx, cx-width/2)
	}
}

func TestMeanAndStddevHandleDegenerateInput(t *testing.T) {
	if got := mean(nil); !math.IsNaN(got) {
		t.Errorf("mean of nothing = %v, want NaN rather than a misleading zero", got)
	}
	if got := stddev([]float64{1}); !math.IsNaN(got) {
		t.Errorf("stddev of one sample = %v, want NaN", got)
	}
	if got := mean([]float64{1, 2, 3}); math.Abs(got-2) > 1e-12 {
		t.Errorf("mean = %v, want 2", got)
	}
	// Sample standard deviation, not population: the divisor is n-1.
	if got := stddev([]float64{2, 4}); math.Abs(got-math.Sqrt2) > 1e-12 {
		t.Errorf("stddev = %v, want sqrt(2)", got)
	}
}

func TestReadManifestRejectsAnEmptyCaseList(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/manifest.json"
	if err := writeFile(path, `{"cases":[]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil {
		t.Error("accepted a manifest with no cases, which would silently analyse nothing")
	}

	if err := writeFile(path, `{"cases":[{"id":"a","source_id":"source/v1/abc"}]}`); err != nil {
		t.Fatal(err)
	}
	sites, err := readManifest(path)
	if err != nil {
		t.Fatalf("rejected a valid manifest: %v", err)
	}
	if sites["source/v1/abc"] != "a" {
		t.Errorf("mapping = %v, want source/v1/abc to a", sites)
	}
}
