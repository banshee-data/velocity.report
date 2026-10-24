package pipeline

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func guardTrack(id string, x, vx float32, misses int) *l5tracks.TrackedObject {
	return &l5tracks.TrackedObject{TrackID: id, X: x, Y: 10, LatestZ: 0.8, VX: vx,
		OBBLength: 4, OBBWidth: 2, OBBHeight: 1.6, Misses: misses}
}

func TestHoldBoxesHoldMovingTracksOnly(t *testing.T) {
	tracks := []*l5tracks.TrackedObject{
		guardTrack("moving", 0, 5, 0),
		guardTrack("coasting", 10, 3, 2),
		guardTrack("still", 20, 0.2, 0),
		{TrackID: "no box", X: 30, VX: 5},
	}
	boxes := holdBoxes(tracks, 1)
	if len(boxes) != 2 {
		t.Fatalf("%d held boxes, want the moving and the coasting track's", len(boxes))
	}
	if boxes[0].X != 0 || boxes[0].VX != 5 || boxes[0].Length != 4 || boxes[0].Z != float64(float32(0.8)+float32(1.6)/2) || boxes[1].X != 10 {
		t.Fatalf("held boxes %+v", boxes)
	}
	if n := len(holdBoxes(tracks, 0)); n != 3 {
		t.Fatalf("with no speed floor %d boxes held, want every track with a box (3)", n)
	}
}

func TestEdgeBoxesFollowObservedTracksAndTheirClusters(t *testing.T) {
	tracks := []*l5tracks.TrackedObject{guardTrack("seen", 0, 5, 0), guardTrack("coasting", 10, 5, 1)}
	obb := func(x float32) *l4perception.OrientedBoundingBox {
		return &l4perception.OrientedBoundingBox{CenterX: x, CenterY: 10, CenterZ: 0.8, Length: 4.5, Width: 2, Height: 1.6}
	}
	clusters := []l4perception.WorldCluster{
		{OBB: obb(0.3)}, // associated with the observed track
		{OBB: obb(10)},  // with the coasting one: not this frame's object
		{OBB: obb(20)},  // with none
		{},              // no box
	}
	boxes := edgeBoxes(tracks, clusters, []string{"seen", "coasting", "", "seen"})
	if len(boxes) != 2 {
		t.Fatalf("%d edge boxes, want the observed track's and its cluster's: %+v", len(boxes), boxes)
	}
	if boxes[0].X != 0 || boxes[1].X != float64(float32(0.3)) || boxes[1].Length != 4.5 {
		t.Fatalf("edge boxes %+v", boxes)
	}
	// Both stand on their base, 0.8 m: the centre is half their height above it.
	for _, b := range boxes {
		if math.Abs(b.Z-1.6) > 1e-6 {
			t.Fatalf("edge box centre z %.3f, want 1.6 (base 0.8 + height 1.6 / 2): %+v", b.Z, b)
		}
	}
	if n := len(edgeBoxes(tracks, clusters, nil)); n != 1 {
		t.Fatalf("with no associations %d edge boxes, want the observed track's alone", n)
	}
}

func TestSameBacking(t *testing.T) {
	a := []bool{true, false}
	if !sameBacking(a, a[:1]) || sameBacking(a, append([]bool(nil), a...)) || sameBacking(nil, a) {
		t.Fatal("sameBacking must tell a slice of the same array from a copy")
	}
}
