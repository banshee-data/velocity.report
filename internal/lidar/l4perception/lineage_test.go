package l4perception

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

// The lineage carrier must cost nothing when no tap is enabled: it sits in
// padding, and it must not appear in the reduced JSON observation records
// whose bytes the evidence oracle hashes.
func TestSourceOrdinalIsFreeAndInvisibleToJSON(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if got := unsafe.Sizeof(WorldPoint{}); got != 72 {
			t.Fatalf("WorldPoint is %d bytes, want 72: the lineage field must occupy padding", got)
		}
	}
	plain := WorldPoint{X: 1.5, Y: -2, Z: 0.25, Intensity: 9, Timestamp: time.Unix(0, 42).UTC(), SensorID: "s"}
	stamped := plain
	if !stamped.SetSourceOrdinal(0) {
		t.Fatal("ordinal zero is a real return and must be representable")
	}
	a, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(stamped)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("lineage leaked into JSON:\n%s\n%s", a, b)
	}
	const golden = `{"X":1.5,"Y":-2,"Z":0.25,"Intensity":9,"Timestamp":"1970-01-01T00:00:00.000000042Z","SensorID":"s"}`
	if string(a) != golden {
		t.Fatalf("WorldPoint JSON changed:\n got %s\nwant %s", a, golden)
	}
}

func TestSourceOrdinalPresenceIsDistinctFromZero(t *testing.T) {
	var p WorldPoint
	if _, ok := p.SourceOrdinal(); ok {
		t.Fatal("an unstamped point reported lineage")
	}
	p.SetSourceOrdinal(0)
	if ordinal, ok := p.SourceOrdinal(); !ok || ordinal != 0 {
		t.Fatalf("ordinal = %d, %v; want 0, true", ordinal, ok)
	}
	p.SetSourceOrdinal(69_277)
	if ordinal, ok := p.SourceOrdinal(); !ok || ordinal != 69_277 {
		t.Fatalf("ordinal = %d, %v", ordinal, ok)
	}
	for _, bad := range []int{-1, math.MaxUint32} {
		p.SetSourceOrdinal(5)
		if p.SetSourceOrdinal(bad) {
			t.Fatalf("accepted unrepresentable ordinal %d", bad)
		}
		if _, ok := p.SourceOrdinal(); ok {
			t.Fatalf("unrepresentable ordinal %d left stale lineage", bad)
		}
	}
}

// Every L4 stage copies WorldPoint by value, so lineage stamped before the
// height filter must arrive intact at the DBSCAN trace, including through the
// in-place height-band compaction, a voxel representative and the input cap.
func TestSourceOrdinalSurvivesEveryL4Stage(t *testing.T) {
	points := make([]WorldPoint, 0, 64)
	for i := 0; i < 60; i++ {
		// Two tight blobs plus a floor return and an overhead return.
		x := float64(i%30) * 0.02
		if i >= 30 {
			x += 5
		}
		points = append(points, WorldPoint{X: x, Y: float64(i%3) * 0.02, Z: -1})
	}
	points = append(points, WorldPoint{X: 2, Z: -3.5}, WorldPoint{X: 2, Z: 3})
	for i := range points {
		points[i].SetSourceOrdinal(1000 + i)
	}
	band := DefaultHeightBandFilter()
	kept := band.FilterVertical(points)
	if len(kept) != 60 {
		t.Fatalf("height band kept %d, want 60", len(kept))
	}
	voxels := VoxelGrid(kept, 0.05)
	params := DBSCANParams{Eps: 0.3, MinPts: 3, MaxInputPoints: 20, MaxClusterDiameter: 10, MinClusterDiameter: 0.01, MaxClusterAspectRatio: 100}
	clusters, trace := DBSCANWithTrace(voxels, params)
	if trace.InputPoints != len(voxels) || len(trace.Processed) != 20 || len(trace.Labels) != 20 {
		t.Fatalf("trace = input %d processed %d labels %d", trace.InputPoints, len(trace.Processed), len(trace.Labels))
	}
	seen := map[int]bool{}
	for _, p := range trace.Processed {
		ordinal, ok := p.SourceOrdinal()
		if !ok || ordinal < 1000 || ordinal >= 1060 || seen[ordinal] {
			t.Fatalf("processed point lost or duplicated lineage: %d, %v", ordinal, ok)
		}
		seen[ordinal] = true
	}
	members := map[int64]int{}
	for i, label := range trace.Labels {
		if label == 0 {
			t.Fatalf("processed point %d left unvisited", i)
		}
		if label > 0 {
			members[int64(label)]++
		}
	}
	for _, c := range clusters {
		if members[c.ClusterID] != c.PointsCount {
			t.Fatalf("cluster %d has %d labelled points, summary counts %d", c.ClusterID, members[c.ClusterID], c.PointsCount)
		}
	}
}

func TestDBSCANWithTraceMatchesDBSCAN(t *testing.T) {
	points := make([]WorldPoint, 0, 300)
	for i := 0; i < 300; i++ {
		points = append(points, WorldPoint{X: float64(i%50) * 0.05, Y: float64(i/50) * 0.05, Z: float64(i%7) * 0.1})
	}
	params := DefaultDBSCANParams()
	params.MaxInputPoints = 120
	want := DBSCAN(points, params)
	got, trace := DBSCANWithTrace(points, params)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("traced clustering differs from DBSCAN")
	}
	if trace.InputPoints != len(points) || len(trace.Processed) != 120 {
		t.Fatalf("capped trace = %d of %d", len(trace.Processed), trace.InputPoints)
	}
	if clusters, trace := DBSCANWithTrace(nil, params); clusters != nil || trace.InputPoints != 0 || trace.Processed != nil {
		t.Fatalf("empty input produced %v, %+v", clusters, trace)
	}
}
