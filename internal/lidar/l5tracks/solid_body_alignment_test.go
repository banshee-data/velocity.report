package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The solid body's extent and heading from one face
// (lidar-solid-body-physical-alignment-plan.md). kirk0's physical references
// found an approaching truck held 0.6 m long and turned across itself, and a
// truck whose side came into view refused as a merge for twenty frames.

// strip samples a filled rectangle, x in [x0, x0+dx], y in [y0, y0+dy].
func strip(x0, dx, y0, dy float64, n int) []l4perception.WorldPoint {
	var points []l4perception.WorldPoint
	for i := 0; i <= n; i++ {
		for j := 0; j <= n; j++ {
			points = append(points, l4perception.WorldPoint{
				X: x0 + dx*float64(i)/float64(n), Y: y0 + dy*float64(j)/float64(n),
			})
		}
	}
	return points
}

func movingBody(vx, vy float32) *TrackedObject {
	track := &TrackedObject{}
	track.solidBody = solidBodyTrack{
		seeded:      true,
		state:       [4]float32{0, 0, vx, vy},
		estimation:  EstimationGeometryConverging,
		orientation: OrientationBelief{PsiRad: 0, Provenance: ProvenanceObserved},
	}
	return track
}

func TestCourseHeadingFollowsTravelAboveTheCourseSpeed(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.CourseHeading = true
	tracker := NewTracker(cfg)
	// Travelling along +Y at 6 m/s, with the tracked heading across it, as a
	// front face's PCA axis is.
	track := movingBody(0, 6)
	track.ObservationCount = 5
	track.OBBHeadingRad = 0
	track.HeadingSource = HeadingSourcePCA
	track.solidBody.p[2*4+2] = 0.09 // vx variance: across a +Y course
	track.solidBody.p[3*4+3] = 4    // vy variance: along it, no effect
	o := tracker.solidBodyOrientation(track, &track.solidBody)
	if math.Abs(float64(o.PsiRad)-math.Pi/2) > 1e-6 || !o.IsResolved() {
		t.Fatalf("orientation %+v, want the +Y course, resolved", o)
	}
	if want := float32(0.09 / 36); math.Abs(float64(o.VarianceRad2-want)) > 1e-7 {
		t.Errorf("variance %v, want the velocity's variance across the course over speed squared, %v", o.VarianceRad2, want)
	}

	slow := movingBody(0, 1.5)
	slow.ObservationCount = 5
	slow.OBBHeadingRad = 0.3
	slow.HeadingSource = HeadingSourcePCA
	if o := tracker.solidBodyOrientation(slow, &slow.solidBody); math.Abs(float64(o.PsiRad)-0.3) > 1e-6 {
		t.Errorf("below the course speed the orientation is %v, want the tracked heading 0.3", o.PsiRad)
	}
	plain := NewTracker(solidBodyConfig())
	if o := plain.solidBodyOrientation(track, &track.solidBody); o.PsiRad != 0 {
		t.Errorf("without the option the orientation is %v, want the tracked heading", o.PsiRad)
	}
}

func TestFacePlaneSpansTakeTheDimensionAFaceLiesAcross(t *testing.T) {
	// A truck coming along +X shows its front: a strip 2.5 m across the
	// course and 0.6 m deep.
	front := WorldCluster{RetainedPoints: strip(0, 0.6, -1.25, 2.5, 30)}
	frontOnly := EdgeMeasurementSet{Edges: []EdgeMeasurement{{Face: FaceFront}}}

	plain := NewTracker(solidBodyConfig())
	track := movingBody(6, 0)
	plain.admitSolidBodyExtents(track, front, frontOnly, 0, true)
	if l := track.solidBody.lengthBelief.Estimate(); l <= 0 || l > 0.7 {
		t.Fatalf("by default the front face gives its depth as the length; got %v", l)
	}

	cfg := solidBodyConfig()
	cfg.SolidBody.FacePlaneSpans = true
	planes := NewTracker(cfg)
	track = movingBody(6, 0)
	planes.admitSolidBodyExtents(track, front, frontOnly, 0, true)
	if track.solidBody.lengthBelief.Support != 0 {
		t.Errorf("a front face alone gave a length: %v", track.solidBody.lengthBelief.Estimate())
	}
	if w := track.solidBody.widthBelief.Estimate(); math.Abs(float64(w)-2.5) > 0.15 {
		t.Errorf("front face width %v, want the 2.5 m it spans", w)
	}

	// Broadside, a side strip 9 m along the course gives the length.
	side := WorldCluster{RetainedPoints: strip(-4.5, 9, 1.0, 0.4, 40)}
	track = movingBody(6, 0)
	planes.admitSolidBodyExtents(track, side, EdgeMeasurementSet{Edges: []EdgeMeasurement{{Face: FaceRight}}}, 0, true)
	if l := track.solidBody.lengthBelief.Estimate(); math.Abs(float64(l)-9) > 0.4 {
		t.Errorf("side face length %v, want the 9 m it spans", l)
	}
	if track.solidBody.widthBelief.Support != 0 {
		t.Errorf("a side face alone gave a width: %v", track.solidBody.widthBelief.Estimate())
	}
}

func TestExtentPriorFloorKeepsThePriorAboveAShortSpan(t *testing.T) {
	var short, long extentBelief
	for range 3 {
		short.Observe(0.6)
		long.Observe(9)
	}
	plain := NewTracker(solidBodyConfig())
	if d := plain.dimensionOf(short, 4.5, 1.5); math.Abs(float64(d.Metres)-0.6) > 0.15 {
		t.Fatalf("by default a short span replaces the prior; got %v", d.Metres)
	}
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentPriorFloor = true
	floor := NewTracker(cfg)
	if d := floor.dimensionOf(short, 4.5, 1.5); d.Metres != 4.5 || d.SigmaMetres != 1.5 || d.Provenance != ProvenanceClassPrior {
		t.Errorf("a span shorter than the prior gave %+v, want the prior", d)
	}
	if d := floor.dimensionOf(long, 4.5, 1.5); d.Provenance != ProvenanceAccumulated || d.Metres < 8.5 {
		t.Errorf("a span longer than the prior gave %+v, want it", d)
	}
}

func TestExtentGrowthAdmitsALengtheningViewButNotAWideningOne(t *testing.T) {
	set := EdgeMeasurementSet{Edges: []EdgeMeasurement{{Face: FaceFront}, {Face: FaceRight}}}
	// A truck's side in view: 9 m along the course, 2.6 m across.
	longer := WorldCluster{RetainedPoints: strip(-4.5, 9, -1.3, 2.6, 40)}
	// Two vehicles side by side: 4.5 m along, 5 m across.
	wider := WorldCluster{RetainedPoints: strip(-2.25, 4.5, -2.5, 5, 40)}

	plain := NewTracker(solidBodyConfig())
	track := movingBody(6, 0)
	track.MergeCandidate = true
	plain.admitSolidBodyExtents(track, longer, set, 0, true)
	if track.solidBody.lengthBelief.Support != 0 {
		t.Fatal("by default a merge candidate gives no extent")
	}

	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentGrowthAdmission = true
	growth := NewTracker(cfg)
	track = movingBody(6, 0)
	track.MergeCandidate = true
	growth.admitSolidBodyExtents(track, longer, set, 0, true)
	if l := track.solidBody.lengthBelief.Estimate(); l < 8.5 {
		t.Errorf("a lengthening view gave length %v, want about 9 m", l)
	}
	track = movingBody(6, 0)
	track.MergeCandidate = true
	growth.admitSolidBodyExtents(track, wider, set, 0, true)
	if track.solidBody.lengthBelief.Support != 0 || track.solidBody.widthBelief.Support != 0 {
		t.Error("a cluster wider than a body plus the margin still gave extents")
	}
}
