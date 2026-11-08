package l5tracks

import (
	"math"
	"math/rand"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for BoxModelConfig.BoxExtentFromSolidBody. What carries the weight: a
// truck whose spans never show more than a few metres of it keeps a short
// windowBelief, so the shipped box draws it at the vehicle floor; with the
// option the box takes the solid body's extent state, which the two end faces
// fix, and its length converges on the truck's while still holding every
// frame's points. A frame with no extent state to read, or one measured along
// another axis, is the shipped box.

// partialTruckCluster is the returns a sensor at the origin sees of a box of
// the given length and width, axis +X, centred at (cx, cy) with cy > 0, when
// all but the near end of its side is hidden: the end face toward the sensor
// and the 2.5 m of side behind it on approach and departure, and abeam only
// the 3 m of side nearest the sensor. Its spans are never more than 3 m
// along it, the F0 report's short-belief case made extreme. Each return is
// jittered by 2 cm.
func partialTruckCluster(rng *rand.Rand, cx, cy, length, width float64) WorldCluster {
	const sideSeen = 2.5
	hl, hw := length/2, width/2
	var pts []l4perception.WorldPoint
	add := func(x, y float64) {
		pts = append(pts, l4perception.WorldPoint{X: x + 0.02*rng.NormFloat64(), Y: y + 0.02*rng.NormFloat64()})
	}
	side := func(lo, hi float64) {
		for i := 0; i < 15; i++ {
			add(lo+(hi-lo)*float64(i)/14, cy-hw)
		}
	}
	end := func(x float64) {
		for i := 0; i < 20; i++ {
			add(x, cy-hw+width*float64(i)/19)
		}
	}
	switch front, rear := cx+hl, cx-hl; {
	case front < 0:
		end(front)
		side(front-sideSeen, front)
	case rear > 0:
		end(rear)
		side(rear, rear+sideSeen)
	default:
		side(math.Max(rear, -1.5), math.Min(front, 1.5))
	}
	var sx, sy float64
	for _, p := range pts {
		sx += p.X
		sy += p.Y
	}
	n := float64(len(pts))
	return WorldCluster{CentroidX: float32(sx / n), CentroidY: float32(sy / n), PointsCount: len(pts), Members: pts}
}

// partialTruckResult is one arm of the partial truck pass.
type partialTruckResult struct {
	lengths      []float64 // the drawn box's length per frame
	finalLength  float64
	finalSigma   float64
	beliefLength float64 // windowBelief's length at the end
	stats        BoxStats
}

// runPartialTruck drives the solid body's state machine (ExtentState, the
// axis known to a degree, as runBoxPass does) and a box estimator side by
// side over a 10 by 2.5 m truck passing 6 m from the sensor at 7 m/s, from
// 30 m before it to 30 m after. The box reads the body's extent state after
// each frame's step, as updateBox does after step 3c.
func runPartialTruck(t *testing.T, boxCfg BoxModelConfig) partialTruckResult {
	t.Helper()
	const length, width, cy, speed, dt = 10.0, 2.5, 6.0, 7.0, 0.1
	tracker := NewTracker(solidBodyConfig())
	tracker.Config.SolidBody.ExtentState = true
	start := -30.0
	track := boxPassBody(float32(start), cy, speed)
	track.ObjectClass, track.ObjectConfidence = "truck", 1
	sb := &track.solidBody
	prior := dimensionPriorFor(MotionRigidVehicle)
	seedSolidBodyExtent(sb, prior)
	// Re-referenced from a medoid on the front corner, with the medoid's
	// bias along and across the body in the position covariance, as a
	// first fix leaves it: not at the true centre, where one face would
	// read the length off at once.
	first := partialTruckCluster(rand.New(rand.NewSource(9)), start, cy, length, width)
	mx, my := medoid(first.Members)
	sb.state[0], sb.state[1] = float32(mx), float32(my)
	sb.p[0*4+0] += (prior.lengthMetres / 2) * (prior.lengthMetres / 2)
	sb.p[1*4+1] += (prior.widthMetres / 2) * (prior.widthMetres / 2)
	box := NewBoxEstimator(boxCfg)
	rng := rand.New(rand.NewSource(3))
	var r partialTruckResult
	for k := 1; cx(start, speed, dt, k) <= 30; k++ {
		x := cx(start, speed, dt, k)
		tracker.predictSolidBodyStep(track, dt)
		cluster := partialTruckCluster(rng, x, cy, length, width)
		f := tracker.measureNearEdgeFrame(sb, cluster, prior, 100, false)
		tracker.stepNearEdge(sb, cluster, prior, f)
		if !sb.seeded {
			t.Fatalf("frame %d: the body was dropped", k)
		}
		mx, my := medoid(cluster.Members)
		b := box.Observe(BoxInput{
			Nanos: int64(k) * 1e8, X: mx, Y: my, VX: speed, VelCov: [3]float64{0.02, 0, 0.02},
			Cluster: &cluster, Body: boxBodyExtent(track),
		})
		assertContains(t, k, b, cluster.Members)
		r.lengths = append(r.lengths, b.Length)
		r.finalLength, r.finalSigma = b.Length, b.LengthSigma
	}
	r.beliefLength = box.length.estimate()
	r.stats = box.Stats
	return r
}

func cx(start, speed, dt float64, k int) float64 { return start + speed*dt*float64(k) }

func TestBoxExtentFromSolidBodyConvergesOnAShortBeliefTruck(t *testing.T) {
	shipped := runPartialTruck(t, enabledBoxConfig())
	cfg := enabledBoxConfig()
	cfg.BoxExtentFromSolidBody = true
	on := runPartialTruck(t, cfg)
	n := len(on.lengths)
	last := func(v []float64) []float64 { return v[n-n/5:] }
	shippedErr := meanAbsError(last(shipped.lengths), 10)
	onErr := meanAbsError(last(on.lengths), 10)
	t.Logf("shipped: final box length %.2f m (windowBelief %.2f m), last-fifth mean |L - 10| %.2f m; stats %+v",
		shipped.finalLength, shipped.beliefLength, shippedErr, shipped.stats)
	t.Logf("from the solid body: final box length %.2f ± %.2f m (windowBelief %.2f m), last-fifth mean |L - 10| %.2f m; stats %+v",
		on.finalLength, on.finalSigma, on.beliefLength, onErr, on.stats)
	for _, k := range []int{n / 5, 2 * n / 5, 3 * n / 5, 4 * n / 5, n - 1} {
		t.Logf("frame %3d: shipped %.2f m, from the solid body %.2f m", k+1, shipped.lengths[k], on.lengths[k])
	}

	// The shipped box sees at most 3 m of the truck and draws it at the
	// vehicle floor; the belief is the spans'.
	if shipped.beliefLength > 3.2 || shippedErr < 5 {
		t.Fatalf("the pass does not keep the shipped belief short: belief %.2f m, error %.2f m", shipped.beliefLength, shippedErr)
	}
	// With the option the length converges toward the truck's once both
	// ends have been seen, and the belief it replaced is the same short one.
	// It stays short of 10 m by what the 1.5 m prior still holds against a
	// gap of five seconds between the two end faces.
	if onErr > 1.25 || onErr > shippedErr/4 || math.Abs(on.finalLength-10) > 1.25 {
		t.Errorf("box length from the solid body: final %.2f m, last-fifth error %.2f m; want within 1.25 m of 10 m and under a quarter of the shipped %.2f m",
			on.finalLength, onErr, shippedErr)
	}
	if limit := boxBodyExtentPriorSigmaShare * float64(dimensionPriorFor(MotionRigidVehicle).sigmaMetres); on.finalSigma <= 0 || on.finalSigma > limit {
		t.Errorf("final length sigma %.3f m, want the state's, in (0, %.2f]", on.finalSigma, limit)
	}
	if math.Abs(on.beliefLength-shipped.beliefLength) > 1e-9 {
		t.Errorf("windowBelief %.3f m with the option, %.3f m without: it must still observe every span", on.beliefLength, shipped.beliefLength)
	}
	if on.stats.BodyExtentFrames == 0 || on.stats.BodyExtentUnsettled == 0 {
		t.Errorf("stats %+v: want frames that took the state and frames, before it settled, that did not", on.stats)
	}
	// Before the state narrows, the box is the shipped one.
	for k := 0; k < n; k++ {
		if on.lengths[k] != shipped.lengths[k] {
			if k == 0 {
				t.Fatal("the first frame already differs from the shipped box")
			}
			t.Logf("first frame the option changed the box: %d of %d", k+1, n)
			break
		}
	}
}

func meanAbsError(v []float64, truth float64) float64 {
	s := 0.0
	for _, x := range v {
		s += math.Abs(x - truth)
	}
	return s / float64(len(v))
}

// A frame with no state to read, or one whose axis is across the box's, is
// the shipped box exactly: the option changes nothing it cannot stand on.
func TestBoxExtentFromSolidBodyFallsBackToTheShippedBox(t *testing.T) {
	cfg := enabledBoxConfig()
	shipped := NewBoxEstimator(cfg)
	cfg.BoxExtentFromSolidBody = true
	unknown, across := NewBoxEstimator(cfg), NewBoxEstimator(cfg)
	acrossBody := BoxBodyExtent{Known: true, AxisRad: math.Pi / 2, Length: 12, Width: 2.4,
		LengthSigma: 0.05, WidthSigma: 0.05, LengthEvidence: true, WidthEvidence: true, PriorSigma: 1.5}
	for i := 0; i < 30; i++ {
		x := -12 + 0.8*float64(i)
		pts := visibleFaces(x, 5, 0, 4.5, 1.8, 20)
		in := boxInput(int64(i)*1e8, pts, 8, 0)
		want := shipped.Observe(in)
		got := unknown.Observe(in)
		in.Body = acrossBody
		gotAcross := across.Observe(in)
		if got != want || gotAcross != want {
			t.Fatalf("frame %d: no state %+v, an axis across the box %+v, want the shipped %+v", i, got, gotAcross, want)
		}
	}
	if s := unknown.Stats; s.BodyExtentUnknown == 0 || s.BodyExtentFrames != 0 {
		t.Errorf("no state: stats %+v", s)
	}
	if s := across.Stats; s.BodyExtentAxisRefusals == 0 || s.BodyExtentFrames != 0 {
		t.Errorf("an axis across the box: stats %+v", s)
	}
}

// boxBodyExtent reads only a body-centre body with an extent state.
func TestBoxBodyExtentNeedsABodyCentreExtentState(t *testing.T) {
	track := boxPassBody(0, 5, 7)
	sb := &track.solidBody
	if boxBodyExtent(track).Known {
		t.Fatal("a body without ExtentState has an extent to read")
	}
	seedSolidBodyExtent(sb, dimensionPriorFor(MotionRigidVehicle))
	got := boxBodyExtent(track)
	if !got.Known || got.Length != 4.5 || got.LengthEvidence || got.LengthSigma != 1.5 {
		t.Fatalf("seeded at the prior: %+v, want 4.5 ± 1.5 m with no evidence", got)
	}
	// The prior the share is taken of is the body's current class's.
	if got.PriorSigma != float64(dimensionPriorFor(MotionUnknown).sigmaMetres) {
		t.Fatalf("an unclassified body's prior sigma %.2f m, want the unknown class's", got.PriorSigma)
	}
	track.ObjectClass, track.ObjectConfidence = "car", 0.9
	if got := boxBodyExtent(track); got.PriorSigma != float64(dimensionPriorFor(MotionRigidVehicle).sigmaMetres) {
		t.Fatalf("a car's prior sigma %.2f m, want the rigid vehicle's", got.PriorSigma)
	}
	sb.ext.updates[0] = 3
	if !boxBodyExtent(track).LengthEvidence {
		t.Fatal("a half-extent updated by a face is not evidence")
	}
	sb.reference = ReferenceClusterMedoid
	if boxBodyExtent(track).Known {
		t.Fatal("a medoid-referenced body has an extent to read")
	}
}

// Through the tracker: the option reads the solid body updated in the same
// frame, and with no solid body it is the shipped box.
func TestTrackerBoxExtentFromSolidBodyReadsTheSolidBody(t *testing.T) {
	frames := syntheticPassFrames(t, throughTruckPass())
	run := func(cfg TrackerConfig) (*TrackedObject, BoxStats) {
		tracker := NewTracker(cfg)
		for _, f := range frames {
			tracker.Update(f.clusters, f.at)
		}
		tr := mainTrack(t, tracker)
		s, ok := tr.BoxModelStats()
		if !ok {
			t.Fatal("the truck has no box")
		}
		return tr, s
	}
	cfg := faceStateConfig()
	cfg.BoxModel.BoxExtentFromSolidBody = true
	_, s := run(cfg)
	if s.BodyExtentFrames == 0 {
		t.Errorf("no frame took the solid body's extent: %+v", s)
	}
	t.Logf("through-truck pass with the face state: %+v", s)

	plain := DefaultTrackerConfig()
	without, _ := run(plain)
	plain.BoxModel.BoxExtentFromSolidBody = true
	with, s := run(plain)
	if with.BoxLength != without.BoxLength || with.BoxWidth != without.BoxWidth || with.BoxX != without.BoxX {
		t.Errorf("without a solid body the option drew %v × %v at %v, the shipped box %v × %v at %v",
			with.BoxLength, with.BoxWidth, with.BoxX, without.BoxLength, without.BoxWidth, without.BoxX)
	}
	if s.BodyExtentFrames != 0 || s.BodyExtentUnknown == 0 {
		t.Errorf("without a solid body: %+v", s)
	}
}
