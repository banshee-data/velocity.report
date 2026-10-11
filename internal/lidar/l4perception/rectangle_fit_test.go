package l4perception

import (
	"math"
	"math/rand"
	"testing"
)

// rectangleCloud samples points on the visible faces of a box of the given
// size at the given axis, centred at (cx, cy): the near end face (across)
// and, when side is true, one side along its whole length, with Gaussian
// noise perpendicular to each face.
func rectangleCloud(rng *rand.Rand, length, width, axis, cx, cy float64, side bool, n int, noise float64) []WorldPoint {
	c, s := math.Cos(axis), math.Sin(axis)
	var pts []WorldPoint
	add := func(along, across float64) {
		pts = append(pts, WorldPoint{X: cx + along*c - across*s, Y: cy + along*s + across*c})
	}
	for i := 0; i < n; i++ {
		// The front face: along = +L/2, across uniform over the width.
		add(length/2+rng.NormFloat64()*noise, (rng.Float64()-0.5)*width)
		if side {
			// The left side: across = +W/2, along uniform over the length.
			add((rng.Float64()-0.5)*length, width/2+rng.NormFloat64()*noise)
		}
	}
	return pts
}

func axisErrDeg(a, b float64) float64 { return FoldAxisRad(a, b) * 180 / math.Pi }

// A corner view fixes the axis to within a degree; an end-on strip,
// whose single edge runs across the body, to within two; and a reversing
// body (the axis plus pi) gives the same axis.
func TestFitRectangleFindsTheAxisFromAnLOrAStrip(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, c := range []struct {
		name  string
		axis  float64
		side  bool
		n     int
		worst float64
	}{
		{"corner at 30 degrees", 30 * math.Pi / 180, true, 120, 1.0},
		{"corner at 100 degrees (reversed)", 100 * math.Pi / 180, true, 120, 1.0},
		{"end-on strip at 10 degrees", 10 * math.Pi / 180, false, 60, 2.0},
		{"end-on strip at 75 degrees", 75 * math.Pi / 180, false, 60, 2.0},
		{"corner at 89.5 degrees, across the wrap", 89.5 * math.Pi / 180, true, 120, 1.0},
	} {
		pts := rectangleCloud(rng, 4.5, 1.9, c.axis, 12, -3, c.side, c.n, 0.02)
		fit := FitRectangle(pts)
		if !fit.Known() {
			t.Errorf("%s: abstained %q (sigma %.1f deg, plateau %.1f deg)", c.name, fit.Abstain, fit.SigmaRad*180/math.Pi, fit.PlateauRad*180/math.Pi)
			continue
		}
		if e := axisErrDeg(fit.AxisRad, c.axis); e > c.worst {
			t.Errorf("%s: axis %.2f deg off (sigma %.2f deg)", c.name, e, fit.SigmaRad*180/math.Pi)
		}
		if fit.AxisRad < 0 || fit.AxisRad >= math.Pi/2 {
			t.Errorf("%s: axis %.3f rad is outside [0, pi/2)", c.name, fit.AxisRad)
		}
	}
}

// The spans and supports read the faces: a corner view spans the length
// along one axis and the width across the other, each edge supported.
func TestFitRectangleReportsSpansAndSupport(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	pts := rectangleCloud(rng, 4.5, 1.9, 0.4, 0, 0, true, 100, 0.02)
	fit := FitRectangle(pts)
	if !fit.Known() {
		t.Fatalf("abstained %q", fit.Abstain)
	}
	long, short := fit.Span1, fit.Span2
	if long < short {
		long, short = short, long
	}
	if math.Abs(long-4.5) > 0.2 || math.Abs(short-1.9) > 0.2 {
		t.Fatalf("spans %.2f and %.2f, want about 4.5 and 1.9", fit.Span1, fit.Span2)
	}
	if fit.Support1 < 50 || fit.Support2 < 50 {
		t.Fatalf("edge support %d and %d, want both faces supported", fit.Support1, fit.Support2)
	}
	if fit.Points != len(pts) {
		t.Fatalf("read %d of %d points", fit.Points, len(pts))
	}
}

// The variance follows the edges: a long well-supported edge gives a sharp
// axis, a short sparse one a wide sigma, and the shape floor holds under
// both.
func TestFitRectangleSigmaFollowsTheEdges(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	sharp := FitRectangle(rectangleCloud(rng, 9.6, 2.5, 0.2, 0, 0, true, 120, 0.02))
	strip := FitRectangle(rectangleCloud(rng, 4.5, 1.9, 0.2, 0, 0, false, 12, 0.03))
	if sharp.SigmaRad < RectangleSigmaFloorRad-1e-9 {
		t.Fatalf("sigma %.3f rad is below the shape floor", sharp.SigmaRad)
	}
	if strip.SigmaRad <= sharp.SigmaRad {
		t.Fatalf("a 12-point strip (sigma %.2f deg) is no wider than a 240-point corner (%.2f deg)", strip.SigmaRad*180/math.Pi, sharp.SigmaRad*180/math.Pi)
	}
	// Twelve returns on one 1.9 m edge: 12 * 0.05^2 / (12 * 1.9^2) under the
	// unit scale is about 0.026 rad, 1.5 degrees, which the floor lifts.
	if strip.SigmaRad != RectangleSigmaFloorRad && strip.SigmaRad < RectangleSigmaFloorRad {
		t.Fatalf("strip sigma %.3f rad", strip.SigmaRad)
	}
}

// Too few points, or a cloud with no edges to speak of, is no axis.
func TestFitRectangleAbstainsWithoutEdges(t *testing.T) {
	if fit := FitRectangle(make([]WorldPoint, RectangleFitMinPoints-1)); fit.Known() || fit.Abstain != "too_few_points" {
		t.Fatalf("seven points gave %+v", fit)
	}
	// A filled disc: every orientation scores alike.
	rng := rand.New(rand.NewSource(5))
	var disc []WorldPoint
	for len(disc) < 200 {
		x, y := rng.Float64()*2-1, rng.Float64()*2-1
		if x*x+y*y <= 1 {
			disc = append(disc, WorldPoint{X: x, Y: y})
		}
	}
	fit := FitRectangle(disc)
	if fit.Known() {
		t.Fatalf("a disc gave an axis: %+v", fit)
	}
	if fit.Abstain != "wide_plateau" && fit.Abstain != "wide_sigma" && fit.Abstain != "weak_edges" {
		t.Fatalf("a disc abstained for %q", fit.Abstain)
	}
}

// More points than the cap are read at a stride, and the axis does not
// depend on which ones.
func TestFitRectangleCapsThePointsItReads(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	pts := rectangleCloud(rng, 4.5, 1.9, 0.7, 0, 0, true, 1500, 0.02)
	fit := FitRectangle(pts)
	if fit.Points > RectangleFitMaxPoints || fit.Points < RectangleFitMaxPoints/2 {
		t.Fatalf("read %d points of %d", fit.Points, len(pts))
	}
	if e := axisErrDeg(fit.AxisRad, 0.7); e > 0.5 {
		t.Fatalf("capped fit %.2f deg off", e)
	}
}

// The closeness loop compares in place of math.Min and math.Max; on finite
// projections the two are the same function, so the score is the same to
// the bit, at every angle, as the form the criterion was calibrated with.
func TestRectangleClosenessMatchesTheMinMaxForm(t *testing.T) {
	reference := func(pts [][2]float64, theta float64) float64 {
		ct, st := math.Cos(theta), math.Sin(theta)
		c1, c2 := make([]float64, len(pts)), make([]float64, len(pts))
		min1, max1 := math.Inf(1), math.Inf(-1)
		min2, max2 := math.Inf(1), math.Inf(-1)
		for i, p := range pts {
			a, b := p[0]*ct+p[1]*st, -p[0]*st+p[1]*ct
			c1[i], c2[i] = a, b
			min1, max1 = math.Min(min1, a), math.Max(max1, a)
			min2, max2 = math.Min(min2, b), math.Max(max2, b)
		}
		score := 0.0
		for i := range pts {
			d1 := math.Min(max1-c1[i], c1[i]-min1)
			d2 := math.Min(max2-c2[i], c2[i]-min2)
			score += 1 / math.Max(math.Min(d1, d2), rectangleClosenessFloorMetres)
		}
		return score
	}
	rng := rand.New(rand.NewSource(13))
	for trial := 0; trial < 20; trial++ {
		pts := rectanglePoints(rectangleCloud(rng, 2+rng.Float64()*8, 1+rng.Float64()*1.5, rng.Float64()*math.Pi, 30*rng.Float64(), -10, trial%2 == 0, 20+rng.Intn(200), 0.03))
		c1, c2 := make([]float64, len(pts)), make([]float64, len(pts))
		for deg := 0.0; deg < 90; deg += 0.25 {
			theta := deg * math.Pi / 180
			if got, want := rectangleCloseness(pts, c1, c2, theta), reference(pts, theta); got != want {
				t.Fatalf("trial %d at %.2f degrees: %v against %v", trial, deg, got, want)
			}
		}
	}
}

func TestFoldAxisRad(t *testing.T) {
	for _, c := range []struct{ a, b, want float64 }{
		{0.1, 0.1, 0}, {0.1, 0.1 + math.Pi/2, 0}, {0.1, 0.1 + math.Pi, 0}, {0, math.Pi / 4, math.Pi / 4},
		{0.02, math.Pi/2 - 0.02, 0.04}, {1.0, 1.3, 0.3},
	} {
		if got := FoldAxisRad(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("fold(%.3f, %.3f) = %.4f, want %.4f", c.a, c.b, got, c.want)
		}
	}
}

func BenchmarkFitRectangle(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	for _, n := range []int{64, 256, 2000} {
		pts := rectangleCloud(rng, 4.5, 1.9, 0.6, 20, 5, true, n/2, 0.02)
		b.Run("points="+itoa(len(pts)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				FitRectangle(pts)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
