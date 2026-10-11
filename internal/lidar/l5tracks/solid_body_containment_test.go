package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// A box is scored against the points it was measured from: points inside the
// reported box, widened by the face tolerance, count; the rest do not.
func TestBoxContainmentCountsThePointsInsideTheReportedBox(t *testing.T) {
	// A 4 x 2 m body at 30 degrees, and a cloud on it plus three strays.
	psi := math.Pi / 6
	c, s := math.Cos(psi), math.Sin(psi)
	var points []l4perception.WorldPoint
	for i := 0; i < 20; i++ {
		along, across := -1.9+float64(i)*0.2, -0.9+float64(i%5)*0.45
		points = append(points, l4perception.WorldPoint{X: 10 + along*c - across*s, Y: 5 + along*s + across*c})
	}
	for _, off := range []float64{3.0, -3.5, 4.0} {
		points = append(points, l4perception.WorldPoint{X: 10 + off*c, Y: 5 + off*s})
	}
	inside := boxContainment(points, 10, 5, psi, 2+DefaultFaceToleranceMetres, 1+DefaultFaceToleranceMetres)
	if inside != 20 {
		t.Fatalf("got %d points inside, want the 20 on the body", inside)
	}
	// The same cloud against a box turned 90 degrees loses most of it.
	if turned := boxContainment(points, 10, 5, psi+math.Pi/2, 2, 1); turned >= 20 {
		t.Fatalf("a box across the body still held %d of 20 points", turned)
	}
}

// The observed spans are the cloud's trimmed extents along and across the
// reported axis, and nothing for too few points to trim.
func TestObservedSpansFollowTheReportedAxis(t *testing.T) {
	psi := 0.4
	c, s := math.Cos(psi), math.Sin(psi)
	var points []l4perception.WorldPoint
	for i := 0; i < 200; i++ {
		along, across := -2.0+float64(i)*(4.0/199), -0.75+float64(i%4)*0.5
		points = append(points, l4perception.WorldPoint{X: along*c - across*s, Y: along*s + across*c})
	}
	along, across := observedSpans(points, psi)
	if math.Abs(float64(along)-4.0) > 0.1 || math.Abs(float64(across)-1.5) > 0.1 {
		t.Fatalf("spans along %.2f across %.2f, want about 4.0 and 1.5", along, across)
	}
	if a, b := observedSpans(points[:DefaultMinFaceSupport-1], psi); a != 0 || b != 0 {
		t.Fatalf("too few points gave spans %.2f and %.2f", a, b)
	}
}

// The floor's angle search reads at most containmentFloorMaxPoints of a
// frame's returns, the same ones each time, and the floor it gives on a
// dense cloud is never below the whole cloud's window minimum and within
// two returns' spacing above it; a cloud under the cap is read whole.
func TestFloorPointsCapTheSearchAndKeepTheSpan(t *testing.T) {
	psi := 0.3
	c, s := math.Cos(psi), math.Sin(psi)
	var points []l4perception.WorldPoint
	for i := 0; i < 3000; i++ {
		along, across := -2.25+float64(i%60)*(4.5/59), -0.95+float64(i/60)*(1.9/49)
		points = append(points, l4perception.WorldPoint{X: 20 + along*c - across*s, Y: 4 + along*s + across*c})
	}
	sub := floorPoints(points)
	if len(sub) != containmentFloorMaxPoints {
		t.Fatalf("the floor read %d of %d points", len(sub), len(points))
	}
	for i, p := range floorPoints(points) {
		if p != sub[i] {
			t.Fatalf("the subset is not reproducible at %d", i)
		}
	}
	fullAlong, _ := minimumAxisSpan(points, float32(psi))
	fullAcross, _ := minimumAxisSpan(points, float32(psi+math.Pi/2))
	capAlong, capAcross := windowMinimumSpans(points, psi)
	if capAlong < fullAlong-1e-6 || capAcross < fullAcross-1e-6 {
		t.Fatalf("capped floor %.3f x %.3f is below the whole cloud's window minimum %.3f x %.3f", capAlong, capAcross, fullAlong, fullAcross)
	}
	if capAlong > fullAlong+0.15 || capAcross > fullAcross+0.15 {
		t.Fatalf("capped floor %.3f x %.3f overstates the whole cloud's window minimum %.3f x %.3f", capAlong, capAcross, fullAlong, fullAcross)
	}
	if small := floorPoints(points[:100]); len(small) != 100 || &small[0] != &points[0] {
		t.Fatalf("a cloud under the cap was copied or cut")
	}
}

// Through the whole tracker, every associated frame carries its containment
// and spans, and the spans read the frame against the belief: an end-on truck
// whose length stays the face depth has a box 8 m shorter than the points it
// sits on, and with the course heading and growth admission the shortfall
// closes to under a metre. The containment share is reported, not asserted:
// on this pass most returns are the front face, so a centre that lags the
// face by a decimetre halves it, which is what the measure is for.
func TestContainmentAndSpansReadTheFrameAgainstTheBelief(t *testing.T) {
	type reading struct {
		known, frames int
		worstShare    float32
		shortfallLast float32 // observed along-span minus believed length at the last known frame
		shortfallMax  float32
	}
	run := func(heading, grow bool) reading {
		cfg := solidBodyConfig()
		cfg.SolidBody.CourseAlignedFaces = true
		cfg.SolidBody.CourseHeading = heading
		cfg.SolidBody.ExtentGrowthAdmission = grow
		tracker := NewTracker(cfg)
		r := reading{worstShare: 1}
		for _, f := range syntheticPassFrames(t, endOnTruckPass()) {
			tracker.Update(f.clusters, f.at)
			sb, ok := mainTrack(t, tracker).SolidBody()
			if !ok {
				continue
			}
			r.frames++
			m := sb.Measurement
			if !m.ContainmentKnown {
				// The seed and a coast publish without a cluster, by design.
				if len(f.clusters) > 0 && m.FallbackReason != "no_association" && m.FallbackReason != "initialisation_window" &&
					sb.Estimate.Orientation.Provenance != ProvenanceNone {
					t.Fatalf("an associated frame with an orientation carries no containment (fallback %q)", m.FallbackReason)
				}
				continue
			}
			r.known++
			if m.ContainedPoints == 0 || m.ContainmentShare < 0 || m.ContainmentShare > 1 {
				t.Fatalf("containment %v of %d points is not a share", m.ContainmentShare, m.ContainedPoints)
			}
			if m.ObservedSpanAlongMetres <= 0 && m.ContainedPoints >= DefaultMinFaceSupport {
				t.Fatalf("a frame of %d points carries no observed span", m.ContainedPoints)
			}
			if sb.Estimate.Reference == ReferenceBodyCentre && m.ContainmentShare < r.worstShare {
				r.worstShare = m.ContainmentShare
			}
			short := m.ObservedSpanAlongMetres - sb.Estimate.Length.Metres
			r.shortfallLast = short
			if short > r.shortfallMax {
				r.shortfallMax = short
			}
		}
		return r
	}
	tracked, corrected := run(false, false), run(true, true)
	t.Logf("tracked axis: %d of %d frames known, worst body-centre share %.2f, shortfall max %.2f m last %.2f m",
		tracked.known, tracked.frames, tracked.worstShare, tracked.shortfallMax, tracked.shortfallLast)
	t.Logf("corrected: %d of %d frames known, worst body-centre share %.2f, shortfall max %.2f m last %.2f m",
		corrected.known, corrected.frames, corrected.worstShare, corrected.shortfallMax, corrected.shortfallLast)
	if tracked.known == 0 || corrected.known == 0 {
		t.Fatal("no frame carried a containment share")
	}
	if tracked.shortfallMax < 5 {
		t.Fatalf("the tracked arm's box is only %.2f m shorter than its points at worst; the face depth as a length should leave 8 m", tracked.shortfallMax)
	}
	if corrected.shortfallLast > 1 {
		t.Fatalf("the corrected arm's box is still %.2f m shorter than its points at the end", corrected.shortfallLast)
	}
}
