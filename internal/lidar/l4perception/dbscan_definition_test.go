package l4perception

import (
	"math"
	"math/rand"
	"testing"
)

// Gaps D1, D2 and D3 in data/maths/paper-implementation-gap-analysis.md, pinned
// against Ester et al. (1996) now that the paper is in hand.

func definitionParams(eps float64, minPts int) DBSCANParams {
	return DBSCANParams{
		Eps:                   eps,
		MinPts:                minPts,
		MaxClusterDiameter:    100,
		MinClusterDiameter:    0,
		MaxClusterAspectRatio: 1000,
	}
}

// D2: Definition 1's neighbourhood contains the point itself, so a core point
// needs MinPts points *including* itself. Exactly MinPts points within eps of
// one another form one cluster; MinPts-1 are noise.
func TestDBSCAN_MinPtsCountsTheQueryPoint(t *testing.T) {
	const minPts = 5
	ring := func(n int) []WorldPoint {
		pts := make([]WorldPoint, 0, n)
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			pts = append(pts, WorldPoint{X: 0.2 * math.Cos(a), Y: 0.2 * math.Sin(a)})
		}
		return pts
	}
	if got := DBSCAN(ring(minPts), definitionParams(0.8, minPts)); len(got) != 1 || got[0].PointsCount != minPts {
		t.Fatalf("exactly MinPts points within eps: clusters = %d, want one cluster of %d points", len(got), minPts)
	}
	if got := DBSCAN(ring(minPts-1), definitionParams(0.8, minPts)); len(got) != 0 {
		t.Fatalf("MinPts-1 points within eps: clusters = %d, want noise only", len(got))
	}
}

// D1: a border point reachable from a core point of two different clusters
// goes to whichever cluster is discovered first (Ester §4, deliberately
// order-dependent). Whatever the order, both clusters must survive, the border
// point must land in exactly one of them, and no point may be lost.
func TestDBSCAN_SharedBorderPointJoinsExactlyOneCluster(t *testing.T) {
	// MinPts 6: each cluster's six points lie within eps of one another, so
	// all are core; the border point sees only the nearest column of each,
	// two points a side plus itself, five, so it is not core and cannot
	// bridge the clusters.
	const eps, minPts = 0.5, 6
	var pts []WorldPoint
	// Cluster A: 6 core-capable points around (0,0).
	for i := 0; i < 6; i++ {
		pts = append(pts, WorldPoint{X: 0.1 * float64(i%3), Y: 0.1 * float64(i/3)})
	}
	// Cluster B: 6 core-capable points around (1.1,0), so its nearest core
	// point to A is 0.9 away: more than eps, the clusters are distinct.
	for i := 0; i < 6; i++ {
		pts = append(pts, WorldPoint{X: 1.1 + 0.1*float64(i%3), Y: 0.1 * float64(i/3)})
	}
	// The border point at (0.65, 0.05): 0.453 from A's (0.2,0) and (0.2,0.1)
	// and from B's (1.1,0) and (1.1,0.1); the next columns are 0.552 away.
	border := WorldPoint{X: 0.65, Y: 0.05}
	pts = append(pts, border)

	rng := rand.New(rand.NewSource(7)) //nolint:gosec // deterministic permutations
	assignedTo := map[int]int{}
	for perm := 0; perm < 8; perm++ {
		shuffled := append([]WorldPoint(nil), pts...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		clusters := DBSCAN(shuffled, definitionParams(eps, minPts))
		if len(clusters) != 2 {
			t.Fatalf("permutation %d: clusters = %d, want 2 (the border point must not merge them)", perm, len(clusters))
		}
		total := clusters[0].PointsCount + clusters[1].PointsCount
		if total != len(pts) {
			t.Fatalf("permutation %d: %d points clustered of %d; the border point was lost or duplicated", perm, total, len(pts))
		}
		// Which cluster took it: the one with 7 points, identified by side.
		for _, c := range clusters {
			if c.PointsCount == 7 {
				side := 0
				if c.CentroidX > 0.65 {
					side = 1
				}
				assignedTo[side]++
			}
		}
	}
	t.Logf("shared border point joined cluster A %d times and cluster B %d times over 8 input orders (order-dependent by the paper's definition)",
		assignedTo[0], assignedTo[1])
}

// D3: two parallel lines of points closer than eps chain into one cluster; at
// more than eps apart they stay two. At the shipped eps of 0.8 m the first
// case is a pedestrian beside a parked car or vehicles in adjacent lanes.
func TestDBSCAN_ParallelLinesMergeOnlyWithinEps(t *testing.T) {
	const eps, minPts = 0.8, 5
	lines := func(separation float64) []WorldPoint {
		var pts []WorldPoint
		for i := 0; i < 30; i++ {
			pts = append(pts, WorldPoint{X: 0.2 * float64(i), Y: 0})
			pts = append(pts, WorldPoint{X: 0.2 * float64(i), Y: separation})
		}
		return pts
	}
	if got := DBSCAN(lines(1.5*eps), definitionParams(eps, minPts)); len(got) != 2 {
		t.Fatalf("lines %.2f m apart (1.5 eps): clusters = %d, want 2", 1.5*eps, len(got))
	}
	if got := DBSCAN(lines(0.8*eps), definitionParams(eps, minPts)); len(got) != 1 {
		t.Fatalf("lines %.2f m apart (0.8 eps): clusters = %d, want 1 (the merge pathology D3 describes)", 0.8*eps, len(got))
	}
}
