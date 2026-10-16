package main

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
)

// box is an axis-aligned solid in the before window's sensor frame.
type box struct{ lo, hi vec3 }

// scene is a street seen from a sensor 2.3 m above the road: the road, two
// facades either side, end walls, two parked cars and two poles. Its surfaces
// face along all three axes and sit all round the sensor, so every axis of a
// rigid motion is observable.
type scene struct {
	groundZ float64 // NaN: no ground
	boxes   []box
}

func streetScene() scene {
	return scene{groundZ: -2.3, boxes: []box{
		{vec3{12, -80, -2.3}, vec3{20, 80, 15}},   // east facade
		{vec3{-20, -80, -2.3}, vec3{-11, 80, 12}}, // west facade
		{vec3{-11, 60, -2.3}, vec3{12, 70, 10}},   // north end
		{vec3{-11, -58, -2.3}, vec3{12, -52, 8}},  // south end
		{vec3{6, 8, -2.3}, vec3{7.8, 12.5, -0.8}}, // parked car, azimuth 26-44
		{vec3{-7.5, -15, -2.3}, vec3{-5.7, -10.5, -0.9}},
		{vec3{3, 25, -2.3}, vec3{3.3, 25.3, 4}},
		{vec3{-4, -30, -2.3}, vec3{-3.7, -29.7, 4}},
	}}
}

// rayBox is the slab test: the entry distance of ray o + s d into b, if any.
func rayBox(o, d vec3, b box) (float64, bool) {
	tmin, tmax := math.Inf(-1), math.Inf(1)
	for i := 0; i < 3; i++ {
		if math.Abs(d[i]) < 1e-12 {
			if o[i] < b.lo[i] || o[i] > b.hi[i] {
				return 0, false
			}
			continue
		}
		t1, t2 := (b.lo[i]-o[i])/d[i], (b.hi[i]-o[i])/d[i]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tmin, tmax = math.Max(tmin, t1), math.Min(tmax, t2)
	}
	if tmax < math.Max(tmin, 0) || tmin <= 0 {
		return 0, false
	}
	return tmin, true
}

func (s scene) cast(o, d vec3) (float64, bool) {
	best, hit := math.Inf(1), false
	if !math.IsNaN(s.groundZ) && d[2] < 0 {
		if r := (s.groundZ - o[2]) / d[2]; r > 0 {
			best, hit = r, true
		}
	}
	for _, b := range s.boxes {
		if r, ok := rayBox(o, d, b); ok && r < best {
			best, hit = r, true
		}
	}
	return best, hit && best <= 120
}

// render is the cell summary a window would give for a sensor whose frame
// maps into the scene's by pose: x_scene = pose.R x_sensor + pose.T. Each
// median carries independent Gaussian noise of sigma metres.
func render(t *testing.T, g *geometry, s scene, pose rigid, sigma float64, rng *rand.Rand) []cellStat {
	t.Helper()
	cells := make([]cellStat, numCells)
	for c := range cells {
		r, ok := s.cast(pose.T, pose.R.mul(g.dirs[c]))
		if !ok || r < 2.5 {
			continue
		}
		if sigma > 0 {
			r += sigma * rng.NormFloat64()
		}
		cells[c] = cellStat{Median: r, Coverage: 1, Samples: 100, Static: true}
	}
	return cells
}

func testGeometry(t *testing.T) *geometry {
	t.Helper()
	cfg, err := parse.LoadEmbeddedPandar40PConfig()
	if err != nil {
		t.Fatal(err)
	}
	g, err := newGeometry(parse.ElevationsFromConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func deg(x float64) float64 { return x * math.Pi / 180 }

func TestIdenticalWindowsGiveTheNullTransform(t *testing.T) {
	g := testGeometry(t)
	rng := rand.New(rand.NewPCG(1, 2))
	a := render(t, g, streetScene(), identityRigid(), 0.005, rng)
	b := render(t, g, streetScene(), identityRigid(), 0.005, rng)
	r := comparePair(g, a, b, defaultFitParams())
	if r.Abstain {
		t.Fatalf("abstained on a well-supported scene: %v", r.Reasons)
	}
	if r.Transform.RotationMagDeg > 0.005 || r.Transform.TranslationMagM > 0.003 {
		t.Fatalf("null pair fitted %.4f deg, %.4f m", r.Transform.RotationMagDeg, r.Transform.TranslationMagM)
	}
	if r.Before.ChangedShare > 0.001 || r.After.ChangedShare > 0.001 {
		t.Fatalf("null pair changed share %.4f -> %.4f", r.Before.ChangedShare, r.After.ChangedShare)
	}
	// The noise alone sets the residual; the fit must not invent a reduction.
	if r.After.MedianAbs > 1.05*r.Before.MedianAbs {
		t.Fatalf("fit raised the residual: %.4f -> %.4f", r.Before.MedianAbs, r.After.MedianAbs)
	}
	for i := 0; i < 3; i++ {
		if r.Transform.RotationSEDeg[i] <= 0 || r.Transform.TranslationSEM[i] <= 0 {
			t.Fatalf("missing standard errors: %+v", r.Transform)
		}
	}
	if r.Transform.JackknifeFits != 12 {
		t.Fatalf("jackknife used %d sectors, want 12", r.Transform.JackknifeFits)
	}

	// Without noise the two windows are the same numbers and the fit is exact.
	exact := render(t, g, streetScene(), identityRigid(), 0, nil)
	r = comparePair(g, exact, exact, defaultFitParams())
	if r.Transform.RotationMagDeg > 1e-9 || r.Transform.TranslationMagM > 1e-9 || r.After.MedianAbs != 0 {
		t.Fatalf("identical windows: %+v after %.3g", r.Transform, r.After.MedianAbs)
	}
}

func TestKnownPoseStepIsRecovered(t *testing.T) {
	g := testGeometry(t)
	rng := rand.New(rand.NewPCG(3, 4))
	want := vec3{deg(0.2), 0, deg(0.3)} // 0.2 deg pitch about x, 0.3 deg yaw
	pose := rigid{R: expSO3(want), T: vec3{0.03, 0, -0.04}}
	a := render(t, g, streetScene(), identityRigid(), 0.005, rng)
	b := render(t, g, streetScene(), pose, 0.005, rng)
	r := comparePair(g, a, b, defaultFitParams())
	if r.Abstain {
		t.Fatalf("abstained: %v", r.Reasons)
	}
	for i := 0; i < 3; i++ {
		if got := deg(r.Transform.RotationDeg[i]); math.Abs(got-want[i]) > deg(0.01) {
			t.Errorf("rotation[%d] = %.4f deg, want %.4f", i, r.Transform.RotationDeg[i], want[i]*180/math.Pi)
		}
		if got := r.Transform.TranslationM[i]; math.Abs(got-pose.T[i]) > 0.005 {
			t.Errorf("translation[%d] = %.4f m, want %.4f", i, got, pose.T[i])
		}
	}
	if math.Abs(r.Transform.TranslationMagM-0.05) > 0.005 {
		t.Errorf("translation magnitude %.4f, want 0.05", r.Transform.TranslationMagM)
	}
	// The step moves the residual well above the noise; the fit brings it
	// back to the noise, everywhere, and so does every held-out sector.
	if r.Before.MedianAbs < 0.03 {
		t.Fatalf("step too small to test: before %.4f m", r.Before.MedianAbs)
	}
	if r.After.MedianAbs > 0.01 {
		t.Errorf("after the fit the median |delta range| is %.4f m", r.After.MedianAbs)
	}
	if r.HeldOutAfter.MedianAbs > 0.012 {
		t.Errorf("held-out median |delta range| %.4f m", r.HeldOutAfter.MedianAbs)
	}
	globalBefore := 0
	for _, s := range r.PerSector {
		if s.Before.Cells == 0 {
			continue
		}
		if s.Before.MedianAbs > 0.02 {
			globalBefore++
		}
		if s.After.MedianAbs > 0.012 {
			t.Errorf("sector %d still at %.4f m after the fit", s.Sector, s.After.MedianAbs)
		}
	}
	if globalBefore < 8 {
		t.Errorf("a pose step should change most sectors; %d did", globalBefore)
	}
}

func TestLocalChangeLeavesTheTransformNearNull(t *testing.T) {
	g := testGeometry(t)
	rng := rand.New(rand.NewPCG(5, 6))
	before := streetScene()
	after := streetScene()
	after.boxes[4] = box{vec3{6, 9, -2.3}, vec3{7.8, 13.5, -0.8}} // the parked car moves 1 m
	a := render(t, g, before, identityRigid(), 0.005, rng)
	b := render(t, g, after, identityRigid(), 0.005, rng)
	r := comparePair(g, a, b, defaultFitParams())
	if r.Abstain {
		t.Fatalf("abstained: %v", r.Reasons)
	}
	if r.Transform.RotationMagDeg > 0.01 || r.Transform.TranslationMagM > 0.005 {
		t.Fatalf("a moved car pulled the fit to %.4f deg, %.4f m", r.Transform.RotationMagDeg, r.Transform.TranslationMagM)
	}
	// The car spans azimuth 24-45 degrees before and after: sectors 0 and 1.
	carSectors := map[int]bool{0: true, 1: true}
	for _, s := range r.PerSector {
		if carSectors[s.Sector] {
			if s.After.ChangedShare < 0.005 {
				t.Errorf("the car's sector %d shows no residual change: %.4f", s.Sector, s.After.ChangedShare)
			}
			continue
		}
		if s.After.ChangedShare > 0.001 {
			t.Errorf("sector %d changed share %.4f; the change should stay with the car", s.Sector, s.After.ChangedShare)
		}
	}
}

func TestOnePlaneAbstains(t *testing.T) {
	g := testGeometry(t)
	rng := rand.New(rand.NewPCG(7, 8))
	ground := scene{groundZ: -2.3}
	a := render(t, g, ground, identityRigid(), 0.005, rng)
	b := render(t, g, ground, identityRigid(), 0.005, rng)
	r := comparePair(g, a, b, defaultFitParams())
	if !r.Abstain {
		t.Fatalf("a road with nothing else on it did not abstain: %+v", r.Support)
	}
	if !anyReason(r.Reasons, "plane") && !anyReason(r.Reasons, "observed") && !anyReason(r.Reasons, "singular") {
		t.Fatalf("abstained for the wrong reason: %v", r.Reasons)
	}
}

func TestOneSectorAbstains(t *testing.T) {
	g := testGeometry(t)
	rng := rand.New(rand.NewPCG(9, 10))
	a := render(t, g, streetScene(), identityRigid(), 0.005, rng)
	b := render(t, g, streetScene(), identityRigid(), 0.005, rng)
	// Only 60-90 degrees stays static in the after window, as if traffic hid
	// everything else.
	for c := range b {
		if s := (c % numAzBins) * 12 / numAzBins; s != 2 {
			b[c].Static = false
		}
	}
	r := comparePair(g, a, b, defaultFitParams())
	if !r.Abstain || !anyReason(r.Reasons, "sector") {
		t.Fatalf("support in one sector did not abstain on it: %v (max share %.2f)", r.Reasons, r.Support.MaxSectorShare)
	}
}

func anyReason(reasons []string, word string) bool {
	for _, r := range reasons {
		if strings.Contains(r, word) {
			return true
		}
	}
	return false
}

func TestDeltaRangeIsTheRangeDifferenceUnderTheIdentity(t *testing.T) {
	c := candidate{D: vec3{0, 1, 0}, Q: vec3{0, 10, 0}, N: vec3{0, -1, 0}, PB: vec3{0, 10.2, 0}, RA: 10, RB: 10.2}
	if d := identityRigid().deltaRange(c); math.Abs(d-0.2) > 1e-12 {
		t.Fatalf("deltaRange = %v, want 0.2", d)
	}
	// Moving the sensor 0.2 m towards the wall explains the change.
	moved := rigid{R: identity3(), T: vec3{0, -0.2, 0}}
	if d := moved.deltaRange(c); math.Abs(d) > 1e-12 {
		t.Fatalf("deltaRange after the move = %v, want 0", d)
	}
}

func TestRotationRoundTrip(t *testing.T) {
	w := vec3{0.01, -0.02, 0.005}
	got := logSO3(expSO3(w))
	if got.sub(w).norm() > 1e-12 {
		t.Fatalf("log(exp(w)) = %v, want %v", got, w)
	}
	vals, vecs := symEigen([][]float64{{2, 1, 0}, {1, 2, 0}, {0, 0, 5}})
	if math.Abs(vals[0]-1) > 1e-12 || math.Abs(vals[1]-3) > 1e-12 || math.Abs(vals[2]-5) > 1e-12 {
		t.Fatalf("eigenvalues %v", vals)
	}
	if math.Abs(math.Abs(vecs[0][0])-math.Sqrt(0.5)) > 1e-9 || math.Abs(vecs[0][0]+vecs[0][1]) > 1e-9 {
		t.Fatalf("eigenvector for 1: %v", vecs[0])
	}
}

func TestMedianSelectsTheMiddle(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12))
	for n := 1; n < 60; n++ {
		xs := make([]float64, n)
		for i := range xs {
			xs[i] = math.Round(10 * rng.NormFloat64()) // ties included
		}
		sorted := append([]float64(nil), xs...)
		slices.Sort(sorted)
		if got, want := median(xs), sortedQuantile(sorted, 0.5); got != want {
			t.Fatalf("n=%d: median %v, want %v", n, got, want)
		}
	}
}
