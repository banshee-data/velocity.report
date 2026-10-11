package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The truncated normal's closed-form moments agree with a numerical
// integration of the density over the interval.
func TestTruncatedNormalMomentsMatchNumericalIntegration(t *testing.T) {
	for _, c := range []struct{ mu, sigma, a, b float64 }{
		{0, 1, -1, 1}, {0, 1, 0.5, 3}, {0, 0.4, 1.9, 3.1}, {2, 1.5, -10, 2.2}, {0, 1, -0.2, 0.2},
	} {
		mean, variance, ok := truncatedNormalMoments(c.mu, c.sigma, c.a, c.b)
		if !ok {
			t.Fatalf("%+v: no moments", c)
		}
		// Riemann sums over the interval.
		const n = 20000
		var mass, m1, m2 float64
		h := (c.b - c.a) / n
		for i := 0; i < n; i++ {
			x := c.a + (float64(i)+0.5)*h
			d := math.Exp(-(x-c.mu)*(x-c.mu)/(2*c.sigma*c.sigma)) * h
			mass += d
			m1 += x * d
			m2 += x * x * d
		}
		wantMean := m1 / mass
		wantVar := m2/mass - wantMean*wantMean
		if math.Abs(mean-wantMean) > 1e-4 || math.Abs(variance-wantVar) > 1e-4 {
			t.Errorf("%+v: mean %.5f var %.5f, want %.5f %.5f", c, mean, variance, wantMean, wantVar)
		}
		if c.a <= c.mu && c.mu <= c.b && variance >= c.sigma*c.sigma {
			t.Errorf("%+v: truncation did not reduce the variance (%.4f against %.4f)", c, variance, c.sigma*c.sigma)
		}
	}
	if _, _, ok := truncatedNormalMoments(0, 1, 40, 41); ok {
		t.Error("an interval holding no density gave moments")
	}
}

// A centre outside the interval the points allow is moved into it, by at most
// the state's own uncertainty, and the variance along the axis falls; a
// belief that cannot hold the span leaves the state alone; a centre well
// inside the interval is not moved.
func TestTruncateAxisHoldsTheCentreInsideThePoints(t *testing.T) {
	// Points along +X from 1.0 to 4.0 m ahead of the current position.
	var points []l4perception.WorldPoint
	for i := 0; i < 120; i++ {
		points = append(points, l4perception.WorldPoint{X: 1.0 + 3.0*float64(i)/119, Y: 0.3 * float64(i%3)})
	}
	scratch := make([]float64, len(points))
	fresh := func(sigma2 float64) (*[4]float32, *[16]float32) {
		state := &[4]float32{0, 0, 5, 0}
		p := &[16]float32{}
		p[0], p[5], p[10], p[15] = float32(sigma2), float32(sigma2), 1, 1
		return state, p
	}
	// A 4 m belief with a 0.2 m sigma: the centre must lie in [4.0-2.1, 1.0+2.1] = [1.9, 3.1].
	state, p := fresh(1.0)
	move := truncateAxis(state, p, points, scratch, 1, 0, DimensionBelief{Metres: 4.0, SigmaMetres: 0.2}, 20)
	if move < 1.0 || move > 3.1 || float64(state[0]) != float64(move) {
		t.Fatalf("centre moved %.2f m to x=%.2f, want into [1.9, 3.1] by less than the full way", move, state[0])
	}
	if p[0] >= 1.0 || p[5] != 1.0 || p[10] != 1 {
		t.Fatalf("covariance after the move: along %.3f (want below 1), across %.3f, velocity %.3f", p[0], p[5], p[10])
	}
	// A belief shorter than the span: extent evidence, no move.
	state, p = fresh(1.0)
	if move := truncateAxis(state, p, points, scratch, 1, 0, DimensionBelief{Metres: 2.0, SigmaMetres: 0.2}, 20); move != 0 || state[0] != 0 {
		t.Fatalf("a belief that cannot hold the span moved the centre %.2f m", move)
	}
	// A centre already inside: the points span [-1.5, 1.5] about it.
	var centred []l4perception.WorldPoint
	for i := 0; i < 120; i++ {
		centred = append(centred, l4perception.WorldPoint{X: -1.5 + 3.0*float64(i)/119})
	}
	state, p = fresh(0.04)
	if move := truncateAxis(state, p, centred, scratch, 1, 0, DimensionBelief{Metres: 4.0, SigmaMetres: 0.2}, 20); math.Abs(float64(move)) > 1e-3 {
		t.Fatalf("a centre inside the interval moved %.3f m", move)
	}
}

// Through the whole tracker on the end-on truck, with the course heading and
// growth admission, containment keeps the body on its points: no worse
// across the body, a higher body-centre containment share, fewer frames
// whose box is shorter than its points, and the floor never below the span.
func TestContainmentHoldsAnEndOnTruckOnItsPoints(t *testing.T) {
	type reading struct {
		worstAcross float64
		medianShare float64
		shortFrames int
		lapses      int
		floorBroken int
		flooredRows int
		shiftedRows int
	}
	run := func(containment bool) reading {
		cfg := solidBodyConfig()
		cfg.SolidBody.CourseAlignedFaces = true
		cfg.SolidBody.CourseHeading = true
		cfg.SolidBody.ExtentGrowthAdmission = true
		cfg.SolidBody.Containment = containment
		tracker := NewTracker(cfg)
		var r reading
		var shares []float64
		for _, f := range syntheticPassFrames(t, endOnTruckPass()) {
			tracker.Update(f.clusters, f.at)
			sb, ok := mainTrack(t, tracker).SolidBody()
			if !ok {
				continue
			}
			m := sb.Measurement
			if m.FallbackReason == "body_centre_lapsed" {
				r.lapses++
			}
			if m.ExtentFloor != "" {
				r.flooredRows++
			}
			if m.ContainmentShiftAlongMetres != 0 || m.ContainmentShiftAcrossMetres != 0 {
				r.shiftedRows++
			}
			if sb.Estimate.Reference != ReferenceBodyCentre || !m.ContainmentKnown {
				continue
			}
			shares = append(shares, float64(m.ContainmentShare))
			r.worstAcross = math.Max(r.worstAcross, math.Abs(float64(sb.Estimate.Y)-f.truthY))
			if m.ObservedSpanAlongMetres-sb.Estimate.Length.Metres > 0.25 {
				r.shortFrames++
			}
			if containment && len(f.clusters) > 0 {
				along, across := windowMinimumSpans(nearEdgePoints(f.clusters[0]), float64(sb.Estimate.Orientation.PsiRad))
				if sb.Estimate.Length.Metres+1e-3 < along || sb.Estimate.Width.Metres+1e-3 < across {
					r.floorBroken++
				}
			}
		}
		if len(shares) > 0 {
			sorted := append([]float64(nil), shares...)
			for i := range sorted {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[j] < sorted[i] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			r.medianShare = sorted[len(sorted)/2]
		}
		return r
	}
	without, with := run(false), run(true)
	t.Logf("without containment: worst across %.2f m, median share %.2f, %d short frames, %d lapses",
		without.worstAcross, without.medianShare, without.shortFrames, without.lapses)
	t.Logf("with containment: worst across %.2f m, median share %.2f, %d short frames, %d lapses, %d floored, %d shifted, floor broken %d",
		with.worstAcross, with.medianShare, with.shortFrames, with.lapses, with.flooredRows, with.shiftedRows, with.floorBroken)
	if with.floorBroken > 0 {
		t.Fatalf("the reported box fell below the window-minimum span on %d frames", with.floorBroken)
	}
	if with.worstAcross > without.worstAcross+0.05 {
		t.Fatalf("containment made the across-body error worse: %.2f m against %.2f m", with.worstAcross, without.worstAcross)
	}
	if with.medianShare < without.medianShare {
		t.Fatalf("containment lowered the median containment share: %.2f against %.2f", with.medianShare, without.medianShare)
	}
	if with.shortFrames > without.shortFrames {
		t.Fatalf("containment left more frames short of their points: %d against %d", with.shortFrames, without.shortFrames)
	}
	if with.flooredRows == 0 && with.shiftedRows == 0 {
		t.Fatal("containment never acted on the end-on truck")
	}
}
