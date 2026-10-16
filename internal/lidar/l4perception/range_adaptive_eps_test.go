package l4perception

import (
	"math"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
)

// Experiment l4clus0001 (decision D-28): a DBSCAN neighbourhood that grows
// with range at the beam spacing, never below the tuned eps. These tests pin
// the law's constants to the sensor, show it joins a far object a fixed eps
// splits while keeping 2 m apart, and show that off means unchanged.

// rangeAdaptiveTestParams are the shipped L4 gates at eps0 0.8 with filters
// wide enough that only DBSCAN's connectivity decides the clusters.
func rangeAdaptiveTestParams() DBSCANParams {
	return DBSCANParams{
		Eps:                   0.8,
		MinPts:                5,
		MaxClusterDiameter:    50,
		MinClusterDiameter:    0,
		MaxClusterAspectRatio: 1000,
	}
}

// clump is n points in a line along y at x, from y0 at 0.1 m spacing.
func clump(x, y0 float64, n int) []WorldPoint {
	points := make([]WorldPoint, n)
	for i := range points {
		points[i] = WorldPoint{X: x, Y: y0 + 0.1*float64(i), Z: -1}
	}
	return points
}

func withRangeAdaptive(p DBSCANParams) DBSCANParams {
	law := DefaultRangeAdaptiveEps()
	p.RangeAdaptive = &law
	return p
}

// The fine-band spacing the law uses is the embedded Pandar40P table's, and
// it is the larger of the two steps at 10 Hz. If the table changes, so must
// the constant.
func TestRangeAdaptiveEpsSpacingMatchesThePandar40PTable(t *testing.T) {
	cfg, err := parse.LoadEmbeddedPandar40PConfig()
	if err != nil {
		t.Fatal(err)
	}
	elev := parse.ElevationsFromConfig(cfg)
	// Channels 6 to 30 are indices 5 to 29.
	mean := (elev[5] - elev[29]) / 24
	if math.Abs(mean-pandar40PFineBandSpacingDeg) > 0.0005 {
		t.Fatalf("fine-band mean spacing %.5f°, constant %.3f°", mean, pandar40PFineBandSpacingDeg)
	}
	for ch := 5; ch < 29; ch++ {
		if gap := elev[ch] - elev[ch+1]; gap < 0.33 || gap > 0.345 {
			t.Errorf("channel %d to %d spacing %.4f° is outside the fine band", ch+1, ch+2, gap)
		}
	}
	law := DefaultRangeAdaptiveEps()
	if want := pandar40PFineBandSpacingDeg * math.Pi / 180; math.Abs(law.AngularStepRad-want) > 1e-15 {
		t.Errorf("default step %.6g rad, want the fine band's %.6g rad", law.AngularStepRad, want)
	}
	if law.Lambda != 1.9 || law.MaxRange != 200 || law.OriginX != 0 || law.OriginY != 0 {
		t.Errorf("default law %+v", law)
	}
}

// The law's values at the W3 range bands, and its floor and ceiling.
func TestRangeAdaptiveEpsLaw(t *testing.T) {
	law := DefaultRangeAdaptiveEps()
	for _, c := range []struct{ r, want float64 }{
		{0, 0.8}, {10, 0.804}, {20, 0.816}, {35, 0.849}, {50, 0.897}, {200, 1.807}, {1000, 1.807},
	} {
		if got := math.Sqrt(law.SquaredEpsAt(0.8, c.r)); math.Abs(got-c.want) > 0.0015 {
			t.Errorf("eps(%g m) = %.4f, want %.3f", c.r, got, c.want)
		}
	}
	for _, r := range []float64{0, 1, 5, 20, 80, 150, 199.9, 200, 5000} {
		if got := law.SquaredEpsAt(0.8, r); got < 0.64 {
			t.Errorf("eps(%g m)² = %g is below eps0²", r, got)
		}
	}
	// A degenerate law is the floor, never NaN or below it. The floor is the
	// runtime product, as the fixed-eps query computes it, not the constant
	// 0.64 the compiler would fold 0.8*0.8 to.
	eps0 := 0.8
	for _, bad := range []RangeAdaptiveEps{{}, {Lambda: -1, AngularStepRad: 0.01}, {Lambda: math.NaN(), AngularStepRad: 0.01}} {
		if got := bad.SquaredEpsAt(eps0, 50); got != eps0*eps0 {
			t.Errorf("%+v: eps(50 m)² = %g, want the floor", bad, got)
		}
	}
}

// Two clumps 0.85 m apart at 50 m: a fixed 0.8 m eps splits them, and the
// range-adaptive eps (0.897 m there) joins them.
func TestRangeAdaptiveEpsJoinsAFarObjectAFixedEpsSplits(t *testing.T) {
	points := append(clump(50, 0, 6), clump(50, 0.5+0.85, 6)...)

	fixed := DBSCAN(points, rangeAdaptiveTestParams())
	if len(fixed) != 2 {
		t.Fatalf("fixed eps: %d clusters, want the object split in 2", len(fixed))
	}
	adaptive := DBSCAN(points, withRangeAdaptive(rangeAdaptiveTestParams()))
	if len(adaptive) != 1 || adaptive[0].PointsCount != 12 {
		t.Fatalf("range-adaptive eps: %d clusters, want 1 of 12 points", len(adaptive))
	}
}

// Two objects 2 m apart stay apart at short range, at the far W3 band, and at
// the law's ceiling: eps never reaches 2 m.
func TestRangeAdaptiveEpsKeepsObjectsTwoMetresApart(t *testing.T) {
	for _, x := range []float64{5, 10, 50, 150, 300} {
		points := append(clump(x, 0, 6), clump(x, 0.5+2, 6)...)
		got := DBSCAN(points, withRangeAdaptive(rangeAdaptiveTestParams()))
		if len(got) != 2 {
			t.Errorf("at %g m: %d clusters, want 2", x, len(got))
		}
	}
}

// The pair rule is the smaller of the two tolerances (the nearer point's, for
// the shipped law). A pair 0.9 m apart, inside the far point's 1.0 m but
// outside the near point's 0.8 m, is not a neighbour pair from either side,
// so the relation stays symmetric.
func TestRangeAdaptiveEpsPairRuleIsTheSmallerTolerance(t *testing.T) {
	tol2 := []float64{0.64, 1.0}
	points := []WorldPoint{{X: 10, Y: 0}, {X: 10.9, Y: 0}}
	si := NewSpatialIndex(0.8)
	si.Build(points)
	for i := range points {
		got := si.regionQueryAdaptiveInto(points, i, tol2, nil)
		if len(got) != 1 || got[0] != i {
			t.Errorf("query %d found %v; a pair 0.9 m apart is outside the smaller 0.8 m", i, got)
		}
	}
	tol2[0] = 0.82 // 0.906 m: now inside both
	for i := range points {
		if got := si.regionQueryAdaptiveInto(points, i, tol2, nil); len(got) != 2 {
			t.Errorf("query %d found %v, want both once both tolerances admit the pair", i, got)
		}
	}
}

// The index keeps eps0-wide cells; a neighbour two cells away within the
// adaptive eps must still be found. Here it is from both sides.
func TestRangeAdaptiveEpsSearchesBeyondTheThreeByThreeCells(t *testing.T) {
	a := WorldPoint{X: 50.401, Y: 0} // cell 63 starts at 50.4
	b := WorldPoint{X: 50.401 - 0.85, Y: 0}
	points := []WorldPoint{a, b}
	si := NewSpatialIndex(0.8)
	si.Build(points)
	if ca, cb := math.Floor(a.X/0.8), math.Floor(b.X/0.8); ca-cb != 2 {
		t.Fatalf("fixture cells %v and %v are not two apart", ca, cb)
	}
	tol2 := DefaultRangeAdaptiveEps().squaredTolerances(points, 0.8)
	for i := range points {
		if got := si.regionQueryAdaptiveInto(points, i, tol2, nil); len(got) != 2 {
			t.Errorf("query %d found %v, want both points", i, got)
		}
	}
}

// The widened grid search finds exactly the pairs a brute-force scan of the
// pair rule finds, over a cloud spread from the sensor out past MaxRange.
func TestRangeAdaptiveEpsQueryMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(28)) //nolint:gosec // fixture
	var points []WorldPoint
	for i := 0; i < 1500; i++ {
		r := 1 + 260*rng.Float64()
		a := 2 * math.Pi * rng.Float64()
		points = append(points, WorldPoint{X: r * math.Cos(a), Y: r * math.Sin(a)})
		// A few close companions, so neighbourhoods are not all empty.
		for j := 0; j < 3; j++ {
			points = append(points, WorldPoint{X: points[len(points)-1].X + 1.2*(rng.Float64()-0.5),
				Y: points[len(points)-1].Y + 1.2*(rng.Float64()-0.5)})
		}
	}
	law := DefaultRangeAdaptiveEps()
	law.OriginX, law.OriginY = 3, -2 // the origin is honoured, not assumed
	tol2 := law.squaredTolerances(points, 0.8)
	si := NewSpatialIndex(0.8)
	si.Build(points)
	for i := range points {
		got := si.regionQueryAdaptiveInto(points, i, tol2, nil)
		sort.Ints(got)
		var want []int
		for k := range points {
			dx, dy := points[k].X-points[i].X, points[k].Y-points[i].Y
			if dx*dx+dy*dy <= math.Min(tol2[i], tol2[k]) {
				want = append(want, k)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("query %d: got %v, want %v", i, got, want)
		}
	}
}

// frameLikeCloud is a deterministic scene: vehicle-sized clumps at ranges
// from 3 m to 120 m, some split by gaps near eps, plus scattered noise.
func frameLikeCloud(seed int64) []WorldPoint {
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // fixture
	var points []WorldPoint
	for k := 0; k < 40; k++ {
		r := 3 + 117*rng.Float64()
		a := math.Pi * rng.Float64()
		cx, cy := r*math.Cos(a), r*math.Sin(a)
		n := 6 + rng.Intn(40)
		gap := 0.6 + 0.5*rng.Float64()
		for i := 0; i < n; i++ {
			along := 4.5 * rng.Float64()
			if along > 2.2 {
				along += gap
			}
			points = append(points, WorldPoint{X: cx + along*math.Cos(a+1.3), Y: cy + along*math.Sin(a+1.3) + 0.3*rng.Float64(),
				Z: -1 + rng.Float64(), Intensity: uint8(rng.Intn(255))})
		}
	}
	for i := 0; i < 300; i++ {
		points = append(points, WorldPoint{X: 240*rng.Float64() - 120, Y: 120 * rng.Float64(), Z: -1})
	}
	return points
}

// Off is unchanged: the nil option takes the fixed-eps query. And the
// adaptive machinery at λ = 0 reduces to that query exactly, label for label.
func TestRangeAdaptiveEpsOffGivesIdenticalClusters(t *testing.T) {
	points := frameLikeCloud(41)
	params := rangeAdaptiveTestParams()
	params.MaxSamplePoints = 64

	off, offTrace := DBSCANWithTrace(points, params)
	again, againTrace := DBSCANWithTrace(points, params)
	if !reflect.DeepEqual(off, again) || !reflect.DeepEqual(offTrace.Labels, againTrace.Labels) {
		t.Fatal("the fixed-eps path is not repeatable")
	}

	flat := RangeAdaptiveEps{Lambda: 0, AngularStepRad: DefaultRangeAdaptiveEps().AngularStepRad, MaxRange: 200}
	params.RangeAdaptive = &flat
	zero, zeroTrace := DBSCANWithTrace(points, params)
	if !reflect.DeepEqual(offTrace.Labels, zeroTrace.Labels) {
		t.Fatal("the adaptive query at λ = 0 labelled the points differently from the fixed eps")
	}
	if !reflect.DeepEqual(off, zero) {
		t.Fatal("the adaptive query at λ = 0 built different clusters from the fixed eps")
	}

	// And the shipped law does change this scene, or the test proves nothing.
	law := DefaultRangeAdaptiveEps()
	params.RangeAdaptive = &law
	on, _ := DBSCANWithTrace(points, params)
	if len(on) >= len(off) {
		t.Errorf("range-adaptive eps left %d clusters from %d; the fixture should have far splits to join", len(on), len(off))
	}
}

// Determinism: the same input in the same order gives the same clusters and
// labels, with the option on; through the clusterer's sort too.
func TestRangeAdaptiveEpsIsDeterministic(t *testing.T) {
	points := frameLikeCloud(7)
	params := withRangeAdaptive(rangeAdaptiveTestParams())
	params.MaxSamplePoints = 64
	params.MaxInputPoints = 600 // exercise the content-seeded subsample too

	first, firstTrace := DBSCANWithTrace(points, params)
	for run := 0; run < 3; run++ {
		copied := append([]WorldPoint(nil), points...)
		got, trace := DBSCANWithTrace(copied, params)
		if !reflect.DeepEqual(first, got) || !reflect.DeepEqual(firstTrace.Labels, trace.Labels) {
			t.Fatalf("run %d differed", run+1)
		}
	}
	if len(first) == 0 {
		t.Fatal("fixture produced no clusters")
	}
}
