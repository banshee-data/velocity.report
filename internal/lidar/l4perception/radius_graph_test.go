package l4perception

import (
	"math/rand"
	"sort"
	"testing"
)

func chain(x0, y, z float64, n int, step float64) []WorldPoint {
	out := make([]WorldPoint, n)
	for i := range out {
		out[i] = WorldPoint{X: x0 + step*float64(i), Y: y, Z: z}
	}
	return out
}

func graphParams(zGap float64) DBSCANParams {
	return DBSCANParams{Eps: 0.8, MinPts: 5, MaxClusterDiameter: 40, MaxClusterAspectRatio: 100, RadiusGraph: true, RadiusGraphZGap: zGap}
}

func memberCounts(cs []WorldCluster) []int {
	var out []int
	for _, c := range cs {
		out = append(out, c.PointsCount)
	}
	sort.Ints(out)
	return out
}

// A sparse chain has no DBSCAN core point at minPts 5 but is one radius-graph component.
func TestRadiusGraphJoinsASparseChainDBSCANRejects(t *testing.T) {
	pts := chain(0, 0, 0.5, 12, 0.7)
	if got := DBSCAN(pts, DBSCANParams{Eps: 0.8, MinPts: 5, MaxClusterDiameter: 40, MaxClusterAspectRatio: 100}); len(got) != 0 {
		t.Fatalf("DBSCAN found %d clusters in a chain with two neighbours a point", len(got))
	}
	got := DBSCAN(pts, graphParams(0))
	if len(got) != 1 || got[0].PointsCount != 12 {
		t.Fatalf("radius graph: %+v", memberCounts(got))
	}
}

func TestRadiusGraphComponentFloor(t *testing.T) {
	pts := append(chain(0, 0, 0.5, 4, 0.5), chain(20, 0, 0.5, 6, 0.5)...)
	got := DBSCAN(pts, graphParams(0))
	if c := memberCounts(got); len(c) != 1 || c[0] != 6 {
		t.Fatalf("components under the floor kept: %v", c)
	}
}

// Points at equal XY and different heights are one component without the condition and two with it.
func TestRadiusGraphHeightCondition(t *testing.T) {
	low := chain(0, 0, 0.2, 6, 0.4)
	high := chain(0, 0, 2.5, 6, 0.4)
	pts := append(append([]WorldPoint{}, low...), high...)
	if c := memberCounts(DBSCAN(pts, graphParams(0))); len(c) != 1 || c[0] != 12 {
		t.Fatalf("without the height condition: %v", c)
	}
	if c := memberCounts(DBSCAN(pts, graphParams(0.3))); len(c) != 2 || c[0] != 6 || c[1] != 6 {
		t.Fatalf("with the height condition: %v", c)
	}
}

// A bridge chain between two objects merges them, which is the harm the experiment measures.
func TestRadiusGraphBridgeMergesObjects(t *testing.T) {
	a := chain(0, 0, 0.5, 8, 0.3)
	bridge := chain(2.4, 0, 0.5, 10, 0.6)
	b := chain(8.4, 0, 0.5, 8, 0.3)
	pts := append(append(append([]WorldPoint{}, a...), bridge...), b...)
	if c := memberCounts(DBSCAN(pts, graphParams(0))); len(c) != 1 {
		t.Fatalf("bridge did not merge: %v", c)
	}
}

func TestRadiusGraphInputOrderIndependent(t *testing.T) {
	pts := append(chain(0, 0, 0.5, 9, 0.5), chain(30, 5, 0.5, 7, 0.5)...)
	want := memberCounts(DBSCAN(pts, graphParams(0.3)))
	rng := rand.New(rand.NewSource(3))
	shuf := append([]WorldPoint{}, pts...)
	rng.Shuffle(len(shuf), func(i, j int) { shuf[i], shuf[j] = shuf[j], shuf[i] })
	got := memberCounts(DBSCAN(shuf, graphParams(0.3)))
	if len(got) != len(want) || got[0] != want[0] || got[len(got)-1] != want[len(want)-1] {
		t.Fatalf("order dependent: %v vs %v", got, want)
	}
}

// With the mode off nothing changes.
func TestRadiusGraphOffIsDBSCAN(t *testing.T) {
	pts := chain(0, 0, 0.5, 30, 0.1)
	a := memberCounts(DBSCAN(pts, DBSCANParams{Eps: 0.8, MinPts: 5, MaxClusterDiameter: 40, MaxClusterAspectRatio: 100}))
	b := memberCounts(DBSCAN(pts, DBSCANParams{Eps: 0.8, MinPts: 5, MaxClusterDiameter: 40, MaxClusterAspectRatio: 100, RadiusGraph: false, RadiusGraphZGap: 0.3}))
	if len(a) != len(b) || (len(a) > 0 && a[0] != b[0]) {
		t.Fatalf("%v vs %v", a, b)
	}
}
