package l5tracks

import (
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// staticView is one frame of a stationary box seen from the sensor at the
// origin: the synthetic generator's ray cast, with the box centred at (x, y)
// and its length along +X.
func staticView(t *testing.T, length, width, height, x, y float64) WorldCluster {
	t.Helper()
	pass := l4perception.SyntheticPass{
		Sensor: l4perception.DefaultSyntheticSensor(),
		Vehicle: l4perception.SyntheticVehicle{
			Length: length, Width: width, Height: height, LateralOffsetMetres: y, StartXMetres: x,
		},
		Frames: 1, Interval: 100 * time.Millisecond, SensorID: "synthetic-pandar40p",
	}
	frames := l4perception.GenerateSyntheticPass(pass)
	if len(frames) != 1 || len(frames[0].Points) == 0 {
		t.Fatalf("no returns from a %.1f by %.1f m box at (%.1f, %.1f)", length, width, x, y)
	}
	return WorldCluster{Members: frames[0].Points, PointsCount: len(frames[0].Points)}
}

// profiledBody is a body-centre solid body at (x, y) with its axis along +X,
// believed within half a degree.
func profiledBody(x, y float32) *solidBodyTrack {
	return &solidBodyTrack{
		seeded:      true,
		state:       [4]float32{x, y, 0, 0},
		reference:   ReferenceBodyCentre,
		estimation:  EstimationGeometryConverging,
		orientation: OrientationBelief{PsiRad: 0, VarianceRad2: 1e-4, Provenance: ProvenanceObserved},
	}
}

// stepStatic runs n frames of one view through a profile.
func stepStatic(p *extentProfile, sb *solidBodyTrack, cluster WorldCluster, n int, at *int64) ExtentProfileReading {
	var r ExtentProfileReading
	for i := 0; i < n; i++ {
		*at += int64(100 * time.Millisecond)
		frame := newProfileFrame([]WorldCluster{cluster}, 0, 0)
		r = p.step(sb, cluster, frame, *at)
	}
	return r
}

// A 9.6 by 2.5 m truck seen end on, receding, shows only its rear face: the
// profile is a strip at the rear, the rear end observed (the rays reach the
// face across free cells) and the front censored (every cell beyond the face
// is behind it). Seen side on, the same body's length is all there, with
// both ends bounded by rays that pass them; the width the end-on frames
// locked is kept, since a side view cannot see across the body.
func TestExtentProfileLongVehicleEndOnThenSideOn(t *testing.T) {
	const length, width = 9.6, 2.5
	p := &extentProfile{}
	at := int64(1_700_000_000_000_000_000)

	sb := profiledBody(30, 0)
	end := stepStatic(p, sb, staticView(t, length, width, 3.2, 30, 0), 6, &at)
	if !end.Known {
		t.Fatalf("end on: no lock after six frames: %+v", end)
	}
	if end.LengthMetres > 0.5 {
		t.Errorf("end on: length %.2f m from a rear face alone", end.LengthMetres)
	}
	if math.Abs(float64(end.AlongLo)+length/2) > 0.15 {
		t.Errorf("end on: rear at %.2f m, want %.2f", end.AlongLo, -length/2)
	}
	if !end.RearObserved || end.FrontObserved {
		t.Errorf("end on: rear observed %v (want true), front observed %v (want false)", end.RearObserved, end.FrontObserved)
	}
	if math.Abs(float64(end.WidthMetres)-width) > 0.25 {
		t.Errorf("end on: width %.2f m, want %.2f", end.WidthMetres, width)
	}

	sb = profiledBody(0, 10)
	side := stepStatic(p, sb, staticView(t, length, width, 3.2, 0, 10), 6, &at)
	if math.Abs(float64(side.LengthMetres)-length) > 0.25 {
		t.Errorf("side on: length %.2f m, want %.2f", side.LengthMetres, length)
	}
	if !side.RearObserved || !side.FrontObserved {
		t.Errorf("side on: ends observed rear %v front %v, want both", side.RearObserved, side.FrontObserved)
	}
	if math.Abs(float64(side.WidthMetres)-width) > 0.25 {
		t.Errorf("side on: width %.2f m, want the end-on %.2f kept", side.WidthMetres, width)
	}
	if side.LockedUnixNanos != end.LockedUnixNanos || side.LockedUnixNanos == 0 {
		t.Errorf("lock-on time moved: %d then %d", end.LockedUnixNanos, side.LockedUnixNanos)
	}
}

// A 4.5 by 1.8 m car quartering towards the sensor shows its front and its
// near side, and both ends of each are seen: length and width are whole. The
// front and the near side are observed, since rays reach them across free
// cells. The rear and the far side are censored: every ray that would cross
// the body's strip beyond them meets the near side or the front first.
func TestExtentProfileCarQuartering(t *testing.T) {
	const length, width = 4.5, 1.8
	p := &extentProfile{}
	at := int64(1_700_000_000_000_000_000)
	sb := profiledBody(-6, 8)
	r := stepStatic(p, sb, staticView(t, length, width, 1.5, -6, 8), 8, &at)
	if !r.Known {
		t.Fatalf("no lock: %+v", r)
	}
	if math.Abs(float64(r.LengthMetres)-length) > 0.25 {
		t.Errorf("length %.2f m, want %.2f", r.LengthMetres, length)
	}
	if math.Abs(float64(r.WidthMetres)-width) > 0.25 {
		t.Errorf("width %.2f m, want %.2f", r.WidthMetres, width)
	}
	if !r.FrontObserved || r.RearObserved || !r.RightObserved || r.LeftObserved {
		t.Errorf("ends: rear %v front %v right %v left %v; want censored, observed, observed, censored",
			r.RearObserved, r.FrontObserved, r.RightObserved, r.LeftObserved)
	}
}

// Every fifth admitted frame is withheld and changes no cell; a frame whose
// orientation is not believed within 5 degrees is refused and counted; a body
// not yet referenced to its centre is refused without counting as a heading
// refusal.
func TestExtentProfileWithholdsAndRefuses(t *testing.T) {
	p := &extentProfile{}
	at := int64(1_700_000_000_000_000_000)
	cluster := staticView(t, 4.5, 1.8, 1.5, -6, 8)
	sb := profiledBody(-6, 8)
	var withheld []int
	for i := 1; i <= 10; i++ {
		before := *p
		r := stepStatic(p, sb, cluster, 1, &at)
		if r.Withheld {
			withheld = append(withheld, i)
			if p.along != before.along || p.across != before.across {
				t.Errorf("frame %d was withheld and changed the grids", i)
			}
		}
	}
	if len(withheld) != 2 || withheld[0] != 5 || withheld[1] != 10 {
		t.Errorf("withheld frames %v, want [5 10]", withheld)
	}

	sb.orientation.VarianceRad2 = float32(math.Pow(6*math.Pi/180, 2))
	before := *p
	r := stepStatic(p, sb, cluster, 1, &at)
	if r.Admitted || r.RefusedFrames != 1 || r.HeadingRefusedFrames != 1 || p.along != before.along {
		t.Errorf("6 degrees: admitted %v, refused %d, heading refused %d", r.Admitted, r.RefusedFrames, r.HeadingRefusedFrames)
	}
	sb.orientation.VarianceRad2 = 1e-4
	sb.reference = ReferenceClusterMedoid
	r = stepStatic(p, sb, cluster, 1, &at)
	if r.Admitted || r.RefusedFrames != 2 || r.HeadingRefusedFrames != 1 {
		t.Errorf("medoid-referenced: admitted %v, refused %d, heading refused %d", r.Admitted, r.RefusedFrames, r.HeadingRefusedFrames)
	}
	if r.AdmittedFrames != 10 || r.WithheldFrames != 2 {
		t.Errorf("admitted %d, withheld %d; want 10 and 2", r.AdmittedFrames, r.WithheldFrames)
	}
}

// A heading that flips by pi is the same axis: the profile does not mirror.
func TestExtentProfileKeepsItsAxisSense(t *testing.T) {
	p := &extentProfile{}
	at := int64(1_700_000_000_000_000_000)
	cluster := staticView(t, 9.6, 2.5, 3.2, 30, 0)
	sb := profiledBody(30, 0)
	first := stepStatic(p, sb, cluster, 4, &at)
	sb.orientation.PsiRad = math.Pi
	flipped := stepStatic(p, sb, cluster, 2, &at)
	if flipped.AlongLo != first.AlongLo || flipped.AlongHi != first.AlongHi {
		t.Errorf("after a pi flip the along extent moved from [%.2f, %.2f] to [%.2f, %.2f]",
			first.AlongLo, first.AlongHi, flipped.AlongLo, flipped.AlongHi)
	}
	if math.Abs(float64(flipped.PsiRad)) > 1e-6 {
		t.Errorf("profile axis %.3f rad, want the first sense, 0", flipped.PsiRad)
	}
}

// A return of another cluster nearer the sensor in the same column shadows
// the cell behind it: a vehicle in front of this one's end leaves that end
// censored.
func TestExtentProfileOtherClusterShadows(t *testing.T) {
	own := staticView(t, 9.6, 2.5, 3.2, 30, 0)
	front := staticView(t, 4.5, 2.6, 1.5, 22, 0)
	frame := newProfileFrame([]WorldCluster{own, front}, 0, 0)
	b := bearingsOf(nearEdgePoints(own), 0, 0)
	if frame.crossed(24.9, 0, b) {
		t.Error("a point between the car and the truck's rear read as crossed")
	}
	if !frame.crossed(15, 0, b) {
		t.Error("a point in front of the car did not read as crossed")
	}
	if !frame.crossed(30, 8, b) {
		t.Error("a point in an empty column outside both bodies did not read as crossed")
	}
}

// With the option on, the tracks and every bit the solid body publishes are
// the off digests: the profile is a shadow. The course-heading scenario is
// the one in which frames are admitted, so the profile runs; its digest was
// taken with the option off.
func TestExtentProfileChangesNoTrack(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	scenarios := append(extentProfileFingerprintScenarios(), solidBodyFingerprintScenario{
		name: "profile_course_heading_through_truck",
		pass: throughTruckPass,
		config: func() TrackerConfig {
			cfg := faceStateConfig()
			cfg.SolidBody.CourseHeading = true
			return cfg
		},
		golden: profileCourseHeadingGolden,
	})
	admitted := 0
	for _, s := range scenarios {
		cfg := s.config()
		cfg.SolidBody.ExtentProfile = true
		frames := syntheticPassFrames(t, s.pass())
		if got := solidBodyFingerprint(t, cfg, frames); got != s.golden {
			t.Errorf("%s with the profile on: digest %s, want %s", s.name, got, s.golden)
		}
		tracker := NewTracker(cfg)
		for _, f := range frames {
			tracker.Update(f.clusters, f.at)
			for _, tr := range tracker.GetActiveTracks() {
				if r, ok := tr.SolidBody(); ok && r.Profile.Admitted {
					admitted++
				}
			}
		}
	}
	if admitted == 0 {
		t.Error("no scenario admitted a frame, so the profile never ran")
	}
}

// The believed axis: the orientation when it is settled and the course does
// not contradict it; otherwise the course at speed when the course is
// settled; otherwise none, counted as a heading refusal.
func TestExtentProfileBelievedAxis(t *testing.T) {
	sb := profiledBody(0, 10)
	sb.state[2] = 5 // 5 m/s along +X
	sb.p[2*4+2], sb.p[3*4+3] = 0.01, 0.01
	psi, ok, _ := profileAxis(sb)
	if !ok || psi != 0 {
		t.Errorf("settled orientation along the course: axis %.3f ok %v", psi, ok)
	}
	sb.orientation.PsiRad = 0.5 // 29 degrees off the course
	if psi, ok, _ = profileAxis(sb); !ok || math.Abs(float64(psi)) > 1e-6 {
		t.Errorf("orientation the course contradicts: axis %.3f ok %v, want the course, 0", psi, ok)
	}
	sb.orientation.VarianceRad2 = 1 // unsettled
	if psi, ok, _ = profileAxis(sb); !ok || math.Abs(float64(psi)) > 1e-6 {
		t.Errorf("unsettled orientation: axis %.3f ok %v, want the course, 0", psi, ok)
	}
	sb.p[3*4+3] = 1 // the course's sigma 0.2 rad across
	if _, ok, heading := profileAxis(sb); ok || !heading {
		t.Errorf("neither settled: ok %v heading-only %v, want a heading refusal", ok, heading)
	}
	sb.state[2] = 0
	if _, ok, heading := profileAxis(sb); ok || !heading {
		t.Errorf("at rest, unsettled: ok %v heading-only %v, want a heading refusal", ok, heading)
	}
}
