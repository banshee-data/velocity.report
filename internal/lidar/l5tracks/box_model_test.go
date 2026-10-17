package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// visibleFaces returns points on the faces of an L×W rectangle centred at
// (cx, cy) with heading psi that face a sensor at the origin: an end, a side,
// or both, as a roadside sensor sees a car.
func visibleFaces(cx, cy, psi, length, width float64, perMetre int) []l4perception.WorldPoint {
	c, s := math.Cos(psi), math.Sin(psi)
	type face struct{ ca, cn, da, dn, span float64 }
	faces := []face{
		{length / 2, 0, 0, 1, width}, {-length / 2, 0, 0, 1, width},
		{0, width / 2, 1, 0, length}, {0, -width / 2, 1, 0, length},
	}
	var out []l4perception.WorldPoint
	for _, f := range faces {
		fx, fy := cx+f.ca*c-f.cn*s, cy+f.ca*s+f.cn*c
		// The outward normal of the face is along its centre offset.
		nx, ny := f.ca*c-f.cn*s, f.ca*s+f.cn*c
		if -fx*nx-fy*ny <= 0 {
			continue
		}
		n := int(f.span * float64(perMetre))
		for i := 0; i <= n; i++ {
			t := -f.span/2 + f.span*float64(i)/float64(n)
			a, b := f.ca+t*f.da, f.cn+t*f.dn
			out = append(out, l4perception.WorldPoint{X: cx + a*c - b*s, Y: cy + a*s + b*c})
		}
	}
	return out
}

// medoid is the member nearest the members' mean, as the tracker measures.
func medoid(pts []l4perception.WorldPoint) (float64, float64) {
	var mx, my float64
	for _, p := range pts {
		mx += p.X
		my += p.Y
	}
	mx /= float64(len(pts))
	my /= float64(len(pts))
	best, bx, by := math.Inf(1), 0.0, 0.0
	for _, p := range pts {
		if d := math.Hypot(p.X-mx, p.Y-my); d < best {
			best, bx, by = d, p.X, p.Y
		}
	}
	return bx, by
}

func boxInput(nanos int64, pts []l4perception.WorldPoint, vx, vy float64) BoxInput {
	x, y := medoid(pts)
	return BoxInput{Nanos: nanos, X: x, Y: y, VX: vx, VY: vy, VelCov: [3]float64{0.02, 0, 0.02},
		Cluster: &WorldCluster{Members: pts}}
}

// assertContains fails when any point lies outside the box by more than
// a millimetre.
func assertContains(t *testing.T, frame int, b BoxState, pts []l4perception.WorldPoint) {
	t.Helper()
	c, s := math.Cos(b.HeadingRad), math.Sin(b.HeadingRad)
	for _, p := range pts {
		dx, dy := p.X-b.X, p.Y-b.Y
		a, n := dx*c+dy*s, -dx*s+dy*c
		if math.Abs(a) > b.Length/2+1e-3 || math.Abs(n) > b.Width/2+1e-3 {
			t.Fatalf("frame %d: point (%.2f, %.2f) outside box %+v", frame, p.X, p.Y, b)
		}
	}
}

func axisErrorDeg(a, b float64) float64 { return math.Abs(foldHalf(a-b)) * 180 / math.Pi }

func enabledBoxConfig() BoxModelConfig {
	cfg := DefaultBoxModelConfig()
	cfg.Enabled = true
	return cfg
}

func TestBoxModelStraightPassHoldsPointsAndHeading(t *testing.T) {
	e := NewBoxEstimator(enabledBoxConfig())
	psi := 30 * math.Pi / 180
	speed := 8.0
	for i := 0; i < 40; i++ {
		d := -15 + speed*0.1*float64(i)
		cx, cy := 12+d*math.Cos(psi), -6+d*math.Sin(psi)
		pts := visibleFaces(cx, cy, psi, 4.5, 1.8, 20)
		b := e.Observe(boxInput(int64(i)*1e8, pts, speed*math.Cos(psi), speed*math.Sin(psi)))
		assertContains(t, i, b, pts)
		if i >= 3 {
			if got := axisErrorDeg(b.HeadingRad, psi); got > 3 {
				t.Fatalf("frame %d: box axis %.1f° off the body", i, got)
			}
			if !b.Labelled {
				t.Fatalf("frame %d: a car at 8 m/s is not labelled", i)
			}
		}
	}
	if b := e.State(); b.Length < 4.4 || b.Length > 4.9 || b.Width < 1.7 || b.Width > 2.1 {
		t.Fatalf("extents %.2f × %.2f, want the 4.5 × 1.8 body", b.Length, b.Width)
	}
}

func TestBoxModelDoesNotTurnAtRest(t *testing.T) {
	// A parked, near-square view whose axis observation wobbles by ±6° and
	// whose span labels swap: the drawn box may correct its axis slowly but
	// must not spin.
	cfg := enabledBoxConfig()
	e := NewBoxEstimator(cfg)
	base := 70 * math.Pi / 180
	prev := math.NaN()
	for i := 0; i < 60; i++ {
		wobble := 6 * math.Pi / 180 * math.Sin(float64(i))
		pts := visibleFaces(-9, 14, base+wobble, 2.3, 2.0, 25)
		b := e.Observe(boxInput(int64(i)*1e8, pts, 0, 0))
		assertContains(t, i, b, pts)
		if !math.IsNaN(prev) {
			if turn := math.Abs(foldHalf(b.HeadingRad-prev)) * 180 / math.Pi; turn > cfg.UnlabelledYawRate*0.1*180/math.Pi+1e-6 {
				t.Fatalf("frame %d: box turned %.1f° at rest", i, turn)
			}
		}
		prev = b.HeadingRad
	}
}

func TestBoxModelFollowsAnArcWithinTheYawBound(t *testing.T) {
	cfg := enabledBoxConfig()
	e := NewBoxEstimator(cfg)
	const radius, speed, dt = 10.0, 5.0, 0.1
	omega := speed / radius
	prev := math.NaN()
	for i := 0; i < 50; i++ {
		theta := -math.Pi/2 + omega*dt*float64(i)
		cx, cy := 15+radius*math.Cos(theta), 5+radius*math.Sin(theta)
		psi := theta + math.Pi/2
		pts := visibleFaces(cx, cy, psi, 4.5, 1.8, 20)
		b := e.Observe(boxInput(int64(i)*1e8, pts, speed*math.Cos(psi), speed*math.Sin(psi)))
		assertContains(t, i, b, pts)
		if !math.IsNaN(prev) {
			bound := (cfg.MaxCurvature*speed + cfg.MinYawRate) * dt
			if !b.Labelled {
				bound = (cfg.MaxCurvature*speed + cfg.UnlabelledYawRate) * dt
			}
			if turn := math.Abs(foldHalf(b.HeadingRad - prev)); turn > bound+1e-9 && e.Stats.Relabels == 0 {
				t.Fatalf("frame %d: turned %.1f° against a bound of %.1f°", i, turn*180/math.Pi, bound*180/math.Pi)
			}
		}
		prev = b.HeadingRad
		if i >= 10 {
			if got := axisErrorDeg(b.HeadingRad, psi); got > 6 {
				t.Fatalf("frame %d: box %.1f° behind the turn", i, got)
			}
		}
	}
}

func TestBoxModelBoundsAnOutlierAxisAtLowSpeed(t *testing.T) {
	// Labelled at speed, then slowing to 1.2 m/s while the fit reports an
	// axis 40° away: the box may not swing faster than κ·v + ω₀.
	cfg := enabledBoxConfig()
	cfg.CourseMinSpeedMps = 2 // the slow frames carry no course
	e := NewBoxEstimator(cfg)
	psi := 0.0
	x := 0.0
	for i := 0; i < 10; i++ {
		x += 0.8
		pts := visibleFaces(x+5, 8, psi, 4.5, 1.8, 20)
		e.Observe(boxInput(int64(i)*1e8, pts, 8, 0))
	}
	prev := e.State().HeadingRad
	for i := 10; i < 30; i++ {
		x += 0.12
		pts := visibleFaces(x+5, 8, psi+40*math.Pi/180, 4.5, 1.8, 20)
		b := e.Observe(boxInput(int64(i)*1e8, pts, 1.2, 0))
		assertContains(t, i, b, pts)
		bound := (cfg.MaxCurvature*1.2 + cfg.MinYawRate) * 0.1
		if turn := math.Abs(foldHalf(b.HeadingRad - prev)); turn > bound+1e-9 {
			t.Fatalf("frame %d: turned %.2f° against %.2f°", i, turn*180/math.Pi, bound*180/math.Pi)
		}
		prev = b.HeadingRad
	}
	if e.Stats.Clamps == 0 {
		t.Fatal("the yaw-rate bound never acted")
	}
}

func TestBoxModelCoastsWithTheTrack(t *testing.T) {
	e := NewBoxEstimator(enabledBoxConfig())
	psi := -20 * math.Pi / 180
	var in BoxInput
	for i := 0; i < 10; i++ {
		d := float64(i) * 0.6
		pts := visibleFaces(10+d*math.Cos(psi), 10+d*math.Sin(psi), psi, 4.5, 1.8, 20)
		in = boxInput(int64(i)*1e8, pts, 6*math.Cos(psi), 6*math.Sin(psi))
		e.Observe(in)
	}
	before := e.State()
	in.Nanos += 1e8
	in.X += 0.6 * math.Cos(psi)
	in.Y += 0.6 * math.Sin(psi)
	in.Cluster = nil
	b := e.Observe(in)
	if moved := math.Hypot(b.X-before.X, b.Y-before.Y); math.Abs(moved-0.6) > 0.05 {
		t.Fatalf("coasted box moved %.2f m, the track 0.6 m", moved)
	}
	if b.Length != before.Length || b.Width != before.Width {
		t.Fatalf("coasting changed the extents: %+v then %+v", before, b)
	}
	if turn := axisErrorDeg(b.HeadingRad, before.HeadingRad); turn > 1 {
		t.Fatalf("coasting turned the box %.1f°", turn)
	}
}

func TestBoxModelRelabelsAnEndOnViewOnce(t *testing.T) {
	// A car driving straight at the sensor shows its front: a strip across
	// the body, longer than it is deep. Unlabelled, the box takes the strip's
	// long side as its length; the course then turns it by a quarter, once,
	// and the box still holds the points.
	e := NewBoxEstimator(enabledBoxConfig())
	psi := math.Pi // driving toward the origin along -x
	for i := 0; i < 20; i++ {
		x := 30 - 0.7*float64(i)
		pts := visibleFaces(x, 0.5, psi, 4.5, 1.8, 30)
		vx := 0.0
		if i >= 5 {
			vx = -7
		}
		b := e.Observe(boxInput(int64(i)*1e8, pts, vx, 0))
		assertContains(t, i, b, pts)
		if i >= 6 && axisErrorDeg(b.HeadingRad, psi) > 3 {
			t.Fatalf("frame %d: labelled box %.1f° off the body", i, axisErrorDeg(b.HeadingRad, psi))
		}
	}
	if e.Stats.Relabels != 1 {
		t.Fatalf("relabels = %d, want 1", e.Stats.Relabels)
	}
}

func TestWindowBeliefCorroboratesAndForgets(t *testing.T) {
	b, life := windowBelief{window: 50}, windowBelief{}
	if b.estimate() != 0 || life.estimate() != 0 {
		t.Fatal("empty belief is not zero")
	}
	b.observe(4)
	if b.estimate() != 4 {
		t.Fatalf("one span: %v", b.estimate())
	}
	life.observe(4)
	for _, v := range []float64{4.2, 9.5, 4.4, 4.3} {
		b.observe(v)
		life.observe(v)
	}
	if got := b.estimate(); got != 4.3 {
		t.Fatalf("third largest of 4, 4.2, 9.5, 4.4, 4.3 = %v, want 4.3", got)
	}
	if got := life.estimate(); got != 4.3 {
		t.Fatalf("whole life: third largest = %v, want 4.3", got)
	}
	for i := 0; i < b.window; i++ {
		b.observe(2)
		life.observe(2)
	}
	if got := b.estimate(); got != 2 {
		t.Fatalf("after a window of 2 m spans: %v", got)
	}
	if got := life.estimate(); got != 4.3 || len(life.spans) != boxBeliefCorroboration {
		t.Fatalf("whole life forgot: %v over %d spans", got, len(life.spans))
	}
}

func TestBoxModelExtentMemoryKeepsATruckWhole(t *testing.T) {
	// A 10 m truck leaves the sensor along x at 5 m/s, seen whole for a
	// second and then only its rear 4 m, the rest hidden behind a parked van.
	run := func(window int) float64 {
		cfg := enabledBoxConfig()
		cfg.ExtentWindowSpans = window
		e := NewBoxEstimator(cfg)
		var b BoxState
		for i := 0; i < 100; i++ {
			x := 8 + 0.5*float64(i)
			pts := visibleFaces(x, 3, 0, 10, 2.5, 20)
			if i >= 10 {
				kept := pts[:0]
				for _, p := range pts {
					if p.X <= x-1 {
						kept = append(kept, p)
					}
				}
				pts = kept
			}
			b = e.Observe(boxInput(int64(i)*1e8, pts, 5, 0))
			assertContains(t, i, b, pts)
		}
		return b.Length
	}
	if got := run(50); got > 6 {
		t.Fatalf("a 50-span window kept %.1f m; the test needs it to forget", got)
	}
	if got := run(0); got < 9.5 {
		t.Fatalf("whole-life memory drew the truck %.1f m long, want 10", got)
	}
}

func TestBoxModelTallFloorDrawsAnEndOnTruckLonger(t *testing.T) {
	// A body driving straight at the sensor shows only its front face, a
	// 2.5 m strip; a truck's is 3.2 m tall, a car's 1.5 m.
	run := func(height float64, cfg BoxModelConfig) float64 {
		e := NewBoxEstimator(cfg)
		var b BoxState
		for i := 0; i < 30; i++ {
			pts := visibleFaces(40-0.5*float64(i), 0, math.Pi, 10, 2.5, 20)
			in := boxInput(int64(i)*1e8, pts, -5, 0)
			in.Cluster.BoundingBoxHeight = float32(height)
			b = e.Observe(in)
			assertContains(t, i, b, pts)
		}
		return b.Length
	}
	plain, tall := enabledBoxConfig(), enabledBoxConfig()
	tall.TallHeightMetres, tall.TallLengthFloorMetres = 2.6, 6
	if got := run(3.2, plain); math.Abs(got-3.8) > 1e-9 {
		t.Fatalf("without the tall floor the truck is %.2f m, want the 3.8 m vehicle floor", got)
	}
	if got := run(3.2, tall); math.Abs(got-6) > 1e-9 {
		t.Fatalf("tall truck drawn %.2f m, want 6", got)
	}
	if got := run(1.5, tall); math.Abs(got-3.8) > 1e-9 {
		t.Fatalf("a car under the tall floor drawn %.2f m, want 3.8", got)
	}
}

// carCluster is a WorldCluster of a car's visible faces, with its members,
// its PCA box and its medoid as the tracker measures it.
func carCluster(id int64, cx, cy, psi float64) WorldCluster {
	pts := visibleFaces(cx, cy, psi, 4.5, 1.8, 20)
	obb := l4perception.EstimateOBBFromCluster(pts)
	x, y := medoid(pts)
	return WorldCluster{ClusterID: id, SensorID: "box-test", CentroidX: float32(x), CentroidY: float32(y),
		BoundingBoxLength: obb.Length, BoundingBoxWidth: obb.Width, BoundingBoxHeight: 1.5,
		PointsCount: len(pts), OBB: &obb, Members: pts}
}

func TestTrackerPublishesABoxThatHoldsItsCluster(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.BoxModel = enabledBoxConfig()
	tracker := NewTracker(cfg)
	psi := 25 * math.Pi / 180
	start := time.Unix(1_700_000_000, 0)
	var last WorldCluster
	for i := 0; i < 25; i++ {
		d := float64(i) * 0.8
		last = carCluster(int64(i+1), 8+d*math.Cos(psi), -12+d*math.Sin(psi), psi)
		tracker.Update([]WorldCluster{last}, start.Add(time.Duration(i)*100*time.Millisecond))
	}
	tracks := tracker.GetConfirmedTracks()
	if len(tracks) != 1 {
		t.Fatalf("%d confirmed tracks, want 1", len(tracks))
	}
	tr := tracks[0]
	if tr.BoxLength <= 0 {
		t.Fatal("the box model published no box")
	}
	b := BoxState{X: float64(tr.BoxX), Y: float64(tr.BoxY), HeadingRad: float64(tr.BoxHeadingRad),
		Length: float64(tr.BoxLength), Width: float64(tr.BoxWidth)}
	assertContains(t, 24, BoxState{X: b.X, Y: b.Y, HeadingRad: b.HeadingRad, Length: b.Length + 1e-3, Width: b.Width + 1e-3}, last.Members)
	if got := axisErrorDeg(b.HeadingRad, psi); got > 3 {
		t.Fatalf("published box %.1f° off the car", got)
	}

	// Switched off at run time, the box is dropped rather than frozen.
	tracker.UpdateConfig(func(c *TrackerConfig) { c.BoxModel = BoxModelConfig{} })
	tracker.Update([]WorldCluster{carCluster(99, 8+20*math.Cos(psi), -12+20*math.Sin(psi), psi)}, start.Add(2500*time.Millisecond))
	for _, tr := range tracker.GetActiveTracks() {
		if tr.BoxLength != 0 {
			t.Fatalf("box model off, but track %s still publishes a box", tr.TrackID)
		}
	}
}

func TestTrackerWithoutTheBoxModelPublishesNoBox(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.BoxModel = BoxModelConfig{}
	tracker := NewTracker(cfg)
	start := time.Unix(1_700_000_000, 0)
	for i := 0; i < 8; i++ {
		tracker.Update([]WorldCluster{carCluster(int64(i+1), 10+float64(i)*0.8, 3, 0)}, start.Add(time.Duration(i)*100*time.Millisecond))
	}
	for _, tr := range tracker.GetActiveTracks() {
		if tr.BoxLength != 0 || tr.box != nil {
			t.Fatalf("default tracker published a box: %+v", tr.BoxLength)
		}
	}
}

func TestBoxModelTuningRoundTrips(t *testing.T) {
	block := &config.L5BoxModel{Enabled: true, AxisSource: "rectangle_fit", CourseMinSpeedMps: 1,
		MaxCurvaturePerMetre: 0.2, MinYawRateDegPerSec: 3, UnlabelledYawRateDegPerSec: 45,
		YawAccelSigma: 0.4, AxisSigmaScale: 1.5, CentrePull: 0.3, VehicleLengthFloorMetres: 3.8,
		CourseLeadSec: 0.4, CourseRateTauSec: 0.2, DisplacementLabelMetres: 1.5,
		ExtentWindowSpans: 150, TallHeightMetres: 2.6, TallLengthFloorMetres: 6}
	cfg := BoxModelConfigFromTuning(block)
	if !cfg.Enabled || math.Abs(cfg.MinYawRate-3*math.Pi/180) > 1e-12 || cfg.VehicleLengthFloor != 3.8 ||
		cfg.CourseLeadSec != 0.4 || cfg.CourseRateTauSec != 0.2 || cfg.DisplacementLabelMetres != 1.5 ||
		cfg.ExtentWindowSpans != 150 || cfg.TallHeightMetres != 2.6 || cfg.TallLengthFloorMetres != 6 {
		t.Fatalf("block did not reach the tracker: %+v", cfg)
	}
	back := BoxModelTuningFromTracker(TrackerConfig{BoxModel: cfg})
	if back == nil || *back != *block {
		t.Fatalf("read-back %+v, want %+v", back, block)
	}
	if BoxModelConfigFromTuning(nil).Enabled || BoxModelTuningFromTracker(TrackerConfig{}) != nil {
		t.Fatal("no block is the model off, both ways")
	}
}

// arcLag drives a car round a 10 m arc at 6 m/s, with the velocity a
// constant-velocity filter would report (lagging the tangent by lagFrames),
// and returns the box's mean lag behind the body over the turn, in degrees.
func arcLag(t *testing.T, cfg BoxModelConfig, lagFrames int) float64 {
	t.Helper()
	e := NewBoxEstimator(cfg)
	const radius, speed, dt = 10.0, 6.0, 0.1
	omega := speed / radius
	sum, n := 0.0, 0
	for i := 0; i < 40; i++ {
		theta := -math.Pi/2 + omega*dt*float64(i)
		psi := theta + math.Pi/2
		vel := psi - omega*dt*float64(lagFrames)
		pts := visibleFaces(15+radius*math.Cos(theta), 5+radius*math.Sin(theta), psi, 4.5, 1.8, 20)
		b := e.Observe(boxInput(int64(i)*1e8, pts, speed*math.Cos(vel), speed*math.Sin(vel)))
		assertContains(t, i, b, pts)
		if i >= 15 {
			sum += foldHalf(psi-b.HeadingRad) * 180 / math.Pi
			n++
		}
	}
	return sum / float64(n)
}

func TestBoxModelCourseLeadCutsTheLagInATurn(t *testing.T) {
	base := enabledBoxConfig()
	lagged := arcLag(t, base, 4)
	lead := base
	lead.CourseLeadSec = 0.4
	tau := base
	tau.CourseRateTauSec = 0.4
	if l := arcLag(t, lead, 4); l >= lagged-1 {
		t.Fatalf("course lead: lag %.2f° against %.2f° without", l, lagged)
	}
	if l := arcLag(t, tau, 4); l >= lagged {
		t.Fatalf("rate-weighted course: lag %.2f° against %.2f° without", l, lagged)
	}
}

func TestBoxModelLabelsACreepingCarFromItsDisplacement(t *testing.T) {
	// A car creeping toward the sensor at 0.8 m/s shows its front, a strip
	// across the body, and never reaches the course's speed: without a
	// displacement label its length is drawn across it.
	run := func(cfg BoxModelConfig) float64 {
		e := NewBoxEstimator(cfg)
		var b BoxState
		for i := 0; i < 40; i++ {
			x := 30 - 0.08*float64(i)
			pts := visibleFaces(x, 0.5, math.Pi, 4.5, 1.8, 30)
			b = e.Observe(boxInput(int64(i)*1e8, pts, -0.8, 0))
			assertContains(t, i, b, pts)
		}
		return axisErrorDeg(b.HeadingRad, math.Pi)
	}
	without, with := enabledBoxConfig(), enabledBoxConfig()
	with.DisplacementLabelMetres = 1.0
	if got := run(without); got < 80 {
		t.Fatalf("without the displacement label the creeping car is %.1f° off; the test needs it wrong", got)
	}
	if got := run(with); got > 3 {
		t.Fatalf("displacement-labelled box %.1f° off the body", got)
	}
}

func TestBoxModelMinAreaAxisHoldsTheBody(t *testing.T) {
	cfg := enabledBoxConfig()
	cfg.AxisSource = BoxAxisMinArea
	e := NewBoxEstimator(cfg)
	psi := 35 * math.Pi / 180
	for i := 0; i < 30; i++ {
		d := float64(i) * 0.7
		pts := visibleFaces(14+d*math.Cos(psi), -9+d*math.Sin(psi), psi, 4.5, 1.8, 20)
		b := e.Observe(boxInput(int64(i)*1e8, pts, 7*math.Cos(psi), 7*math.Sin(psi)))
		assertContains(t, i, b, pts)
		// An exact L's hull ties the body's rectangle with the one on its
		// hypotenuse, so the axis wanders within the course's pull.
		if i >= 5 && axisErrorDeg(b.HeadingRad, psi) > 6 {
			t.Fatalf("frame %d: %.1f° off", i, axisErrorDeg(b.HeadingRad, psi))
		}
	}
}
