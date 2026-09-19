package l4perception

import (
	"math"
	"testing"
)

// Gap D6 in data/maths/paper-implementation-gap-analysis.md: uniform
// subsampling to the input cap thins every neighbourhood by f = M/N while
// MinPts stays put, so the effective density threshold on a capped frame is
// MinPts/f. These tests pin the arithmetic and show a cluster of exactly the
// shipped density surviving a frame padded to twice the cap when the option
// that scales MinPts with f is on.

func TestEffectiveMinPtsScalesWithTheSubsampleFraction(t *testing.T) {
	cases := []struct {
		minPts, kept, total, want int
	}{
		{5, 8000, 8000, 5},  // not subsampled: unchanged
		{5, 8000, 6000, 5},  // kept >= total: unchanged
		{5, 8000, 12000, 3}, // f = 2/3: 3.33 -> 3
		{5, 8000, 16000, 3}, // f = 1/2: 2.5 -> 3 (round half away from zero)
		{5, 8000, 40000, 2}, // f = 1/5: 1 -> floor of 2
		{8, 200, 400, 4},    // the survival test below
		{0, 8000, 16000, 0}, // degenerate MinPts left alone
	}
	for _, c := range cases {
		if got := effectiveMinPts(c.minPts, c.kept, c.total); got != c.want {
			t.Errorf("effectiveMinPts(%d, %d, %d) = %d, want %d", c.minPts, c.kept, c.total, got, c.want)
		}
	}
}

// densityTestFrame is a 12-point cluster within one eps of the origin, padded
// with isolated clutter (one point per metre of a grid, none within eps of
// another) to twice the cap. At MinPts 8 the cluster is comfortably core in
// the full frame; after a one-in-two subsample it keeps six points on
// average, below 8 but above the scaled threshold of 4.
func densityTestFrame() ([]WorldPoint, DBSCANParams) {
	var points []WorldPoint
	for i := 0; i < 12; i++ {
		a := 2 * math.Pi * float64(i) / 12
		points = append(points, WorldPoint{X: 0.3 * math.Cos(a), Y: 0.3 * math.Sin(a), Z: 1})
	}
	for gx := 0; gx < 20; gx++ {
		for gy := 0; gy < 20; gy++ {
			if len(points) >= 400 {
				break
			}
			points = append(points, WorldPoint{X: 20 + float64(gx), Y: 20 + float64(gy), Z: 1})
		}
	}
	params := DBSCANParams{
		Eps:                   0.8,
		MinPts:                8,
		MaxInputPoints:        200,
		MaxClusterDiameter:    50,
		MinClusterDiameter:    0,
		MaxClusterAspectRatio: 100,
	}
	return points, params
}

func clusterNearOrigin(clusters []WorldCluster) (WorldCluster, bool) {
	for _, c := range clusters {
		if math.Hypot(float64(c.CentroidX), float64(c.CentroidY)) < 0.5 {
			return c, true
		}
	}
	return WorldCluster{}, false
}

func TestDensityPreservingCapKeepsAThresholdDensityCluster(t *testing.T) {
	points, params := densityTestFrame()

	full := DBSCAN(points, DBSCANParams{Eps: params.Eps, MinPts: params.MinPts,
		MaxClusterDiameter: 50, MaxClusterAspectRatio: 100})
	if c, ok := clusterNearOrigin(full); !ok || c.PointsCount != 12 {
		t.Fatalf("without a cap the 12-point cluster should be found whole; clusters = %d", len(full))
	}

	capped := params
	capped.ScaleMinPtsWhenSubsampled = false
	if _, ok := clusterNearOrigin(DBSCAN(points, capped)); ok {
		// Not a failure: the subsample is deterministic per point set, and
		// this one happened to keep 8 of the 12. The scaled run below is the
		// property under test.
		t.Log("the plain cap kept the cluster in this subsample; the effective threshold was still MinPts/f = 16")
	}

	scaled := params
	scaled.ScaleMinPtsWhenSubsampled = true
	got := DBSCAN(points, scaled)
	c, ok := clusterNearOrigin(got)
	if !ok {
		t.Fatalf("with MinPts scaled to %d the threshold-density cluster was lost on a frame at twice the cap; clusters = %d",
			effectiveMinPts(params.MinPts, params.MaxInputPoints, len(points)), len(got))
	}
	if c.PointsCount < 4 || c.PointsCount > 12 {
		t.Fatalf("cluster kept %d points, want between the scaled threshold 4 and the original 12", c.PointsCount)
	}
	for _, other := range got {
		if math.Hypot(float64(other.CentroidX), float64(other.CentroidY)) > 1 {
			t.Fatalf("isolated clutter formed a cluster at (%.1f, %.1f) under the scaled threshold", other.CentroidX, other.CentroidY)
		}
	}
}

func TestDensityPreservingCapOffByDefault(t *testing.T) {
	if DefaultDBSCANParams().ScaleMinPtsWhenSubsampled {
		t.Fatal("ScaleMinPtsWhenSubsampled defaults on; it must be measured before it ships")
	}
}
