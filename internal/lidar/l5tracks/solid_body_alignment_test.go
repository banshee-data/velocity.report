package l5tracks

import (
	"math"
	"testing"
	"time"

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

	// A tracked axis within the span window of the course is kept, pointed
	// the way the body travels: 5 degrees off a +Y course, given backwards.
	near := movingBody(0, 6)
	near.ObservationCount = 5
	near.OBBHeadingRad = float32(-math.Pi/2 + 5*math.Pi/180)
	near.HeadingSource = HeadingSourcePCA
	o = tracker.solidBodyOrientation(near, &near.solidBody)
	if want := math.Pi/2 + 5*math.Pi/180; math.Abs(float64(o.PsiRad)-want) > 1e-5 || !o.IsResolved() {
		t.Errorf("an axis 5 degrees off the course gave %v rad (resolved %v), want %v resolved", o.PsiRad, o.IsResolved(), want)
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
	if d := plain.dimensionOf(short, 4.5, 1.5, 2.4); math.Abs(float64(d.Metres)-0.6) > 0.15 {
		t.Fatalf("by default a short span replaces the prior; got %v", d.Metres)
	}
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentPriorFloor = true
	floor := NewTracker(cfg)
	if d := floor.dimensionOf(short, 4.5, 1.5, 2.4); d.Metres != 4.5 || d.SigmaMetres != 1.5 || d.Provenance != ProvenanceClassPrior {
		t.Errorf("a span shorter than the prior gave %+v, want the prior", d)
	}
	if d := floor.dimensionOf(long, 4.5, 1.5, 2.4); d.Provenance != ProvenanceAccumulated || d.Metres < 8.5 {
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
	if track.solidBody.widthBelief.Support != 0 {
		t.Errorf("a merge candidate gave a width, %v: a merge widens a body", track.solidBody.widthBelief.Estimate())
	}
	track = movingBody(6, 0)
	track.MergeCandidate = true
	growth.admitSolidBodyExtents(track, wider, set, 0, true)
	if track.solidBody.lengthBelief.Support != 0 || track.solidBody.widthBelief.Support != 0 {
		t.Error("a cluster wider than a body plus the margin still gave extents")
	}
}

func TestVehicleExtentFloorRaisesAPartialViewToTheSmallestCar(t *testing.T) {
	var short, small extentBelief
	for range 3 {
		short.Observe(1.1)
		small.Observe(3.6)
	}
	cfg := solidBodyConfig()
	cfg.SolidBody.VehicleExtentFloor = true
	floor := NewTracker(cfg)
	vehicle := dimensionPriorFor(MotionRigidVehicle)
	d := floor.dimensionOf(short, vehicle.widthMetres, vehicle.sigmaMetres, vehicle.minWidthMetres)
	if d.Metres != vehicle.minWidthMetres || d.Provenance != ProvenanceAccumulated {
		t.Errorf("a 1.1 m width gave %+v, want the smallest car's %v, still accumulated", d, vehicle.minWidthMetres)
	}
	if d := floor.dimensionOf(small, vehicle.lengthMetres, vehicle.sigmaMetres, vehicle.minLengthMetres); math.Abs(float64(d.Metres)-3.6) > 0.15 {
		t.Errorf("a small car seen whole at 3.6 m became %v", d.Metres)
	}
	if p := dimensionPriorFor(MotionPedestrian); p.minLengthMetres != 0 || p.minWidthMetres != 0 {
		t.Error("a class whose smallest member is not known has a floor")
	}
}

// endOnTruckPass is a 9.6 by 2.5 m truck coming straight at the sensor from
// 50 m along +X, 4 m to one side, at 7 m/s: kirk0's truck 2. From the
// sensor's 2.3 m it is seen almost end on for the whole pass.
func endOnTruckPass() l4perception.SyntheticPass {
	sensor := l4perception.DefaultSyntheticSensor()
	sensor.HeightMetres = 2.3
	return l4perception.SyntheticPass{
		Sensor: sensor,
		Vehicle: l4perception.SyntheticVehicle{
			Length: 9.6, Width: 2.5, Height: 3.2, LateralOffsetMetres: 4,
			SpeedMps: 7, StartXMetres: -50,
		},
		Frames: 60, Interval: 100 * time.Millisecond, SensorID: "synthetic-pandar40p",
	}
}

func runEndOnTruck(t *testing.T, cfg TrackerConfig) SolidBodyReading {
	t.Helper()
	tracker := NewTracker(cfg)
	for _, f := range syntheticPassFrames(t, endOnTruckPass()) {
		tracker.Update(f.clusters, f.at)
	}
	r, ok := mainTrack(t, tracker).SolidBody()
	if !ok {
		t.Fatal("the truck has no solid body")
	}
	return r
}

func TestAnEndOnTruckIsHeadedAlongItsCourse(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.CourseAlignedFaces = true
	plain := runEndOnTruck(t, cfg)
	cfg.SolidBody.CourseHeading = true
	cfg.SolidBody.ExtentGrowthAdmission = true
	course := runEndOnTruck(t, cfg)
	headingOff := func(r SolidBodyReading) float64 {
		return FoldAxisAngleDeg(float64(r.Estimate.Orientation.PsiRad))
	}
	t.Logf("tracked heading %.1f deg off the course, length %.2f m; with the course heading %.1f deg, %.2f m",
		headingOff(plain), plain.Estimate.Length.Metres, headingOff(course), course.Estimate.Length.Metres)
	if off := headingOff(course); off > spanSearchHalfWindowDeg {
		t.Errorf("with the course heading the truck is %.1f degrees off its course", off)
	}
	// Lower-bound evidence: no more than a bin over the true 9.6 m, and once
	// the side has been seen, most of it.
	if l := course.Estimate.Length.Metres; l < 7.5 || l > 9.6+extentBeliefBinMetres {
		t.Errorf("with growth admission the 9.6 m truck is %.2f m long", l)
	}
}

// BenchmarkSolidBodyEndOnTruck times the tracker over the end-on truck pass,
// with and without the course heading and growth admission: growth admission
// adds a span search on merge-candidate frames, and extent admission is
// already most of a solid body's update cost.
func BenchmarkSolidBodyEndOnTruck(b *testing.B) {
	frames := syntheticPassFrames(&testing.T{}, endOnTruckPass())
	for _, c := range []struct {
		name          string
		heading, grow bool
	}{{"tracked", false, false}, {"course_heading", true, false}, {"extent_growth", false, true}, {"both", true, true}} {
		b.Run(c.name, func(b *testing.B) {
			cfg := solidBodyConfig()
			cfg.SolidBody.CourseAlignedFaces = true
			cfg.SolidBody.CourseHeading = c.heading
			cfg.SolidBody.ExtentGrowthAdmission = c.grow
			for range b.N {
				tracker := NewTracker(cfg)
				for _, f := range frames {
					tracker.Update(f.clusters, f.at)
				}
			}
		})
	}
}
