package l4perception

import (
	"math"
	"runtime"
	"testing"
)

// denseBlob returns n points packed inside a disc small enough that every point
// is within eps of every other, so DBSCAN treats all of them as core points of
// a single cluster. This is the worst case for cluster expansion: each core
// point reports the whole neighbourhood.
func denseBlob(n int, radius float64) []WorldPoint {
	points := make([]WorldPoint, n)
	// A Fermat spiral fills the disc evenly, so the density is uniform rather
	// than concentrated at the centre.
	golden := math.Pi * (3 - math.Sqrt(5))
	for i := range points {
		r := radius * math.Sqrt(float64(i)/float64(n))
		theta := float64(i) * golden
		points[i] = WorldPoint{
			X:        r * math.Cos(theta),
			Y:        r * math.Sin(theta),
			Z:        0,
			SensorID: "test",
		}
	}
	return points
}

// TestDBSCANExpansionAllocationBounded pins expansion cost to the number of
// points, not to the number of core points times their degree.
//
// Appending every queried neighbourhood onto the seed list unfiltered made a
// dense cluster's seed list grow quadratically in its size, and querying with a
// freshly allocated slice each time compounded it. Between them they accounted
// for 99% of the 62 GB a full kirk0 replay allocated.
func TestDBSCANExpansionAllocationBounded(t *testing.T) {
	const n = 1500
	points := denseBlob(n, 0.4)
	params := testDBSCANParams(1.0, 5)

	// Warm the code paths so first-call costs are not counted.
	DBSCAN(denseBlob(50, 0.4), params)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	clusters := DBSCAN(points, params)

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if len(clusters) != 1 {
		t.Fatalf("expected the blob to form 1 cluster, got %d", len(clusters))
	}

	allocated := int64(after.TotalAlloc - before.TotalAlloc)

	// Index slices are 8 bytes an entry. A linear expansion touches each point
	// a small number of times; a quadratic one costs n times more. The budget
	// sits far above the former and far below the latter.
	budget := int64(n) * 8 * 64

	if allocated > budget {
		t.Errorf("clustering %d dense points allocated %d bytes, budget %d: "+
			"expansion is growing with core points rather than with points to visit",
			n, allocated, budget)
	}
}

// TestDBSCANBorderPointsJoinCluster covers the branch the expansion filter
// touches: a point that is reachable from a core point but is not itself dense
// enough to be one. It must end up in the cluster, whether it was unvisited or
// had already been marked as noise.
func TestDBSCANBorderPointsJoinCluster(t *testing.T) {
	points := []WorldPoint{}

	// A dense core of 12 points inside a 0.2 m box.
	for i := 0; i < 12; i++ {
		points = append(points, WorldPoint{
			X:        0.05 * float64(i%4),
			Y:        0.05 * float64(i/4),
			SensorID: "test",
		})
	}

	// Two border points: within eps of the core, but with too few neighbours of
	// their own to be core points.
	border := []WorldPoint{
		{X: 0.55, Y: 0.0, SensorID: "test"},
		{X: -0.55, Y: 0.0, SensorID: "test"},
	}
	points = append(points, border...)

	params := testDBSCANParams(0.6, 6)
	clusters := DBSCAN(points, params)

	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if got := clusters[0].PointsCount; got != len(points) {
		t.Errorf("expected all %d points in the cluster, got %d: border points were dropped as noise",
			len(points), got)
	}
}

// TestDBSCANChainedClustersStayWhole checks that expansion still walks a chain
// of overlapping neighbourhoods end to end. Filtering what goes onto the seed
// list must not stop the walk short of points only reachable through several
// hops.
func TestDBSCANChainedClustersStayWhole(t *testing.T) {
	// A 60-point chain, each point 0.2 m from the next. With eps = 0.5 no single
	// neighbourhood spans the chain, so reaching the far end takes many hops.
	const n = 60
	points := make([]WorldPoint, n)
	for i := range points {
		points[i] = WorldPoint{X: 0.2 * float64(i), Y: 0, SensorID: "test"}
	}

	params := testDBSCANParams(0.5, 3)
	params.MaxClusterDiameter = 100.0
	clusters := DBSCAN(points, params)

	if len(clusters) != 1 {
		t.Fatalf("expected the chain to form 1 cluster, got %d", len(clusters))
	}
	if got := clusters[0].PointsCount; got != n {
		t.Errorf("expected all %d chained points in the cluster, got %d", n, got)
	}
}
