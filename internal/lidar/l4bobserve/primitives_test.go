package l4bobserve

import (
	"math"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestExtractPrimitivesFindsStablePlaneAndEdge(t *testing.T) {
	points := []l4perception.WorldPoint{
		{X: 0, Y: 0, Z: 1}, {X: 1, Y: 0, Z: 1}, {X: 2, Y: 0, Z: 1},
		{X: 0, Y: 1, Z: 1}, {X: 1, Y: 1, Z: 1}, {X: 2, Y: 1, Z: 1},
	}
	got := ExtractPrimitives(l4perception.WorldCluster{RetainedPoints: points})
	if len(got.Planes) != 1 || len(got.Edges) != 1 {
		t.Fatalf("primitives = %+v", got)
	}
	plane := got.Planes[0]
	if math.Abs(math.Abs(plane.Normal[2])-1) > 1e-9 || math.Abs(plane.Offset+1) > 1e-9 || plane.Support != len(points) {
		t.Fatalf("plane = %+v, want z=1 with complete support", plane)
	}
	if got.Edges[0].Support != 3 || math.Abs(got.Edges[0].Start[1]-got.Edges[0].End[1]) > 0.02 {
		t.Fatalf("edge = %+v, want a supported horizontal row", got.Edges[0])
	}
	if again := ExtractPrimitives(l4perception.WorldCluster{RetainedPoints: points}); !reflect.DeepEqual(again, got) {
		t.Fatalf("primitive fit changed across equal replays: %+v then %+v", got, again)
	}
}

func TestExtractPrimitivesNeedsRetainedGeometry(t *testing.T) {
	if got := ExtractPrimitives(l4perception.WorldCluster{}); len(got.Planes) != 0 || len(got.Edges) != 0 {
		t.Fatalf("empty evidence produced primitives: %+v", got)
	}
	if got := ExtractPrimitives(l4perception.WorldCluster{RetainedPoints: []l4perception.WorldPoint{{}, {}}}); len(got.Planes) != 0 || len(got.Edges) != 0 {
		t.Fatalf("coincident points produced primitives: %+v", got)
	}
	if got := ExtractPrimitives(l4perception.WorldCluster{RetainedPoints: []l4perception.WorldPoint{{X: 0}, {Y: 1}, {Z: 1}}}); len(got.Edges) != 0 {
		t.Fatalf("three non-collinear points produced an unsupported edge: %+v", got)
	}
}
