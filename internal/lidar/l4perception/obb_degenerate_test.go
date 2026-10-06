package l4perception

import (
	"math"
	"testing"
)

// Gap P1 in data/maths/paper-implementation-gap-analysis.md: the degenerate
// covariance cases in the closed-form 2x2 eigendecomposition. The extents these
// produce feed physical endpoint uncertainty, so "no NaN" is the floor, not the
// requirement — the heading has to be reported honestly too.

func finiteOBB(t *testing.T, obb OrientedBoundingBox, context string) {
	t.Helper()
	for _, f := range []struct {
		name string
		v    float32
	}{
		{"CenterX", obb.CenterX}, {"CenterY", obb.CenterY}, {"CenterZ", obb.CenterZ},
		{"Length", obb.Length}, {"Width", obb.Width}, {"Height", obb.Height},
		{"HeadingRad", obb.HeadingRad},
	} {
		if math.IsNaN(float64(f.v)) || math.IsInf(float64(f.v), 0) {
			t.Errorf("%s: %s is not finite (%v)", context, f.name, f.v)
		}
	}
	if obb.Length < 0 || obb.Width < 0 || obb.Height < 0 {
		t.Errorf("%s: negative extent (%v, %v, %v)", context, obb.Length, obb.Width, obb.Height)
	}
}

func TestEstimateOBBFromClusterIdenticalPoints(t *testing.T) {
	// The case the gap names: every point at the same X, Y and Z, so the whole
	// covariance matrix is zero and both eigenvalues are zero.
	points := make([]WorldPoint, 5)
	for i := range points {
		points[i] = WorldPoint{X: 12.5, Y: -3.25, Z: 0.75}
	}

	obb := EstimateOBBFromCluster(points)
	finiteOBB(t, obb, "identical points")

	if obb.Length != 0 || obb.Width != 0 || obb.Height != 0 {
		t.Errorf("expected a zero-extent box, got length=%v width=%v height=%v",
			obb.Length, obb.Width, obb.Height)
	}
	if obb.CenterX != 12.5 || obb.CenterY != -3.25 || obb.CenterZ != 0.75 {
		t.Errorf("expected the centre at the shared point, got (%v, %v, %v)",
			obb.CenterX, obb.CenterY, obb.CenterZ)
	}
	// The heading is arbitrary — there is no axis of variation to recover — but
	// it must be deterministic, or a replay of the same capture would produce a
	// different heading for the same frame.
	repeat := EstimateOBBFromCluster(points)
	if repeat.HeadingRad != obb.HeadingRad {
		t.Errorf("degenerate heading is not deterministic: %v then %v", obb.HeadingRad, repeat.HeadingRad)
	}
	if obb.HeadingRad != 0 {
		t.Errorf("expected the documented X-axis fallback (0 rad), got %v", obb.HeadingRad)
	}
}

func TestEstimateOBBFromClusterVerticalColumn(t *testing.T) {
	// Reachable in a way the all-identical case is not: a narrow vertical
	// object whose returns share an X and Y but span Z. The XY covariance is
	// degenerate while the box still has a real height.
	points := []WorldPoint{
		{X: 8.0, Y: 2.0, Z: 0.0},
		{X: 8.0, Y: 2.0, Z: 0.5},
		{X: 8.0, Y: 2.0, Z: 1.0},
		{X: 8.0, Y: 2.0, Z: 1.5},
		{X: 8.0, Y: 2.0, Z: 2.0},
	}

	obb := EstimateOBBFromCluster(points)
	finiteOBB(t, obb, "vertical column")

	if obb.Length != 0 || obb.Width != 0 {
		t.Errorf("expected zero XY extents, got length=%v width=%v", obb.Length, obb.Width)
	}
	if math.Abs(float64(obb.Height)-2.0) > 1e-6 {
		t.Errorf("height = %v, want 2.0", obb.Height)
	}
	// CenterZ is the cluster floor by design, so the wireframe box sits on the
	// lowest return rather than floating at the volumetric centre.
	if obb.CenterZ != 0 {
		t.Errorf("CenterZ = %v, want the minimum Z (0)", obb.CenterZ)
	}
}

func TestEstimateOBBFromClusterCollinear(t *testing.T) {
	// Collinear points have one zero eigenvalue. The recovered axis has to be
	// the line's own direction, and the perpendicular extent has to be zero
	// rather than a rounding artefact.
	for _, tc := range []struct {
		name        string
		dx, dy      float64
		wantHeading float64
	}{
		{"along X", 1, 0, 0},
		{"along Y", 0, 1, math.Pi / 2},
		{"at 45 degrees", 1, 1, math.Pi / 4},
		{"at 135 degrees", -1, 1, 3 * math.Pi / 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var points []WorldPoint
			for i := -2; i <= 2; i++ {
				s := float64(i)
				points = append(points, WorldPoint{X: 20 + s*tc.dx, Y: 5 + s*tc.dy, Z: 1})
			}

			obb := EstimateOBBFromCluster(points)
			finiteOBB(t, obb, tc.name)

			// PCA recovers an axis, not a direction: a heading 180 degrees
			// opposed describes the same line and is equally correct.
			got := float64(obb.HeadingRad)
			diff := math.Abs(math.Mod(math.Abs(got-tc.wantHeading), math.Pi))
			if diff > 1e-5 && math.Abs(diff-math.Pi) > 1e-5 {
				t.Errorf("heading = %v rad, want %v rad up to a half turn", got, tc.wantHeading)
			}
			if obb.Width > 1e-6 {
				t.Errorf("perpendicular extent = %v, want 0 for collinear points", obb.Width)
			}
			wantLength := 4 * math.Hypot(tc.dx, tc.dy)
			if math.Abs(float64(obb.Length)-wantLength) > 1e-5 {
				t.Errorf("length = %v, want %v", obb.Length, wantLength)
			}
		})
	}
}

func TestEstimateOBBFromClusterNeverReturnsNonFiniteGeometry(t *testing.T) {
	// Inputs that would have made the old compact discriminant form cancel: a
	// nearly circular cluster at long range, and large coordinates with a tiny
	// spread. They are kept after the switch to the non-cancelling form so a
	// regression back to trace² - 4·det shows up here, along with the ordinary
	// pathologies.
	cases := map[string][]WorldPoint{
		"two identical points": {
			{X: 1, Y: 1, Z: 0}, {X: 1, Y: 1, Z: 0},
		},
		"nearly circular at long range": {
			{X: 150, Y: 150, Z: 1}, {X: 150.0000001, Y: 150, Z: 1},
			{X: 150, Y: 150.0000001, Z: 1}, {X: 149.9999999, Y: 150, Z: 1},
			{X: 150, Y: 149.9999999, Z: 1},
		},
		"large coordinates tiny spread": {
			{X: 1e6, Y: 1e6, Z: 0}, {X: 1e6 + 1e-9, Y: 1e6, Z: 0},
			{X: 1e6, Y: 1e6 + 1e-9, Z: 0},
		},
		"one outlier among duplicates": {
			{X: 3, Y: 3, Z: 0}, {X: 3, Y: 3, Z: 0}, {X: 3, Y: 3, Z: 0},
			{X: 3, Y: 3, Z: 0}, {X: 3.001, Y: 3, Z: 0},
		},
		"negative quadrant collinear": {
			{X: -10, Y: -10, Z: 0}, {X: -11, Y: -11, Z: 0}, {X: -12, Y: -12, Z: 0},
		},
	}

	for name, points := range cases {
		t.Run(name, func(t *testing.T) {
			obb := EstimateOBBFromCluster(points)
			finiteOBB(t, obb, name)

			// The heading must be a usable angle, since it is smoothed and
			// compared against other headings downstream.
			if h := float64(obb.HeadingRad); h < -math.Pi-1e-6 || h > math.Pi+1e-6 {
				t.Errorf("heading %v is outside [-pi, pi]", h)
			}
			// Extents must bound the input: a box smaller than its own points
			// would understate physical size.
			var spanX, spanY float64
			minX, maxX := math.MaxFloat64, -math.MaxFloat64
			minY, maxY := math.MaxFloat64, -math.MaxFloat64
			for _, p := range points {
				minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
				minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
			}
			spanX, spanY = maxX-minX, maxY-minY
			diagonal := math.Hypot(spanX, spanY)
			boxDiagonal := math.Hypot(float64(obb.Length), float64(obb.Width))
			if boxDiagonal+1e-6 < diagonal {
				t.Errorf("box diagonal %v does not cover the point span %v", boxDiagonal, diagonal)
			}
		})
	}
}

func TestEstimateOBBFromClusterPrincipalAxisWhenPerpendicularVarianceDominates(t *testing.T) {
	// Gap P2: the old negative-discriminant fallback used lambda1 = c00, which
	// is the wrong eigenvalue when c11 > c00. The discriminant is now computed
	// in a form that cannot go negative, so the branch is gone — but the case
	// it was meant to cover still has to give the right answer. A cluster with
	// more spread across Y than X, and a real covariance between them, must
	// pick the Y-leaning axis.
	points := []WorldPoint{
		{X: 0.00, Y: -3.0, Z: 0},
		{X: 0.05, Y: -1.5, Z: 0},
		{X: 0.10, Y: 0.0, Z: 0},
		{X: 0.15, Y: 1.5, Z: 0},
		{X: 0.20, Y: 3.0, Z: 0},
	}

	obb := EstimateOBBFromCluster(points)
	finiteOBB(t, obb, "perpendicular-dominant")

	// The principal axis is the long one, so the heading must be closer to the
	// Y axis than the X axis.
	h := math.Abs(float64(obb.HeadingRad))
	if h > math.Pi/2 {
		h = math.Pi - h // fold to [0, pi/2]: PCA recovers an axis, not a direction
	}
	if h < math.Pi/4 {
		t.Errorf("heading %v rad leans toward X, but the variance is dominated by Y", obb.HeadingRad)
	}
	// And the box must be longer than it is wide, given a 6 m span against 0.2 m.
	if obb.Length <= obb.Width {
		t.Errorf("length %v is not greater than width %v for a Y-elongated cluster", obb.Length, obb.Width)
	}
	if math.Abs(float64(obb.Length)-6.0) > 0.01 {
		t.Errorf("length = %v, want approximately the 6 m Y span", obb.Length)
	}
}

func TestOBBDiscriminantFormsAgreeWhereTheCompactOneIsWellConditioned(t *testing.T) {
	// The replacement is an algebraic identity, so it must agree with the
	// compact form wherever that form is trustworthy. This pins the equivalence
	// rather than asserting it in a comment.
	for _, tc := range []struct{ c00, c01, c11 float64 }{
		{1, 0, 1},
		{2, 0.5, 1},
		{1, -0.5, 2},
		{0.001, 0.0002, 0.003},
		{5, 2, 5},
		{0, 0, 0},
	} {
		compact := (tc.c00+tc.c11)*(tc.c00+tc.c11) - 4*(tc.c00*tc.c11-tc.c01*tc.c01)
		expanded := (tc.c00-tc.c11)*(tc.c00-tc.c11) + 4*tc.c01*tc.c01

		if expanded < 0 {
			t.Errorf("expanded discriminant went negative for %+v: %v", tc, expanded)
		}
		scale := math.Max(1, math.Abs(compact))
		if math.Abs(compact-expanded)/scale > 1e-12 {
			t.Errorf("forms disagree for %+v: compact=%v expanded=%v", tc, compact, expanded)
		}
	}
}
