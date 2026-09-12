package l4perception

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestSurfaceHeightFilterFollowsGradeAndReportsLowerEvidence(t *testing.T) {
	filter := SurfaceHeightFilter{Surface: l3grid.GroundSurface{A: .1, C: -3}, Floor: .2, Ceiling: 4}
	kept, clipped := filter.Filter([]WorldPoint{
		{X: 0, Z: -2.7},  // 0.3 m above ground
		{X: 10, Z: -1.7}, // same 0.3 m above a 10% grade
		{X: 10, Z: -2.2}, // below the graded surface threshold
		{X: 0, Z: 2},     // overhead
	})
	if len(kept) != 2 || len(clipped) != 1 || clipped[0].X != 10 || clipped[0].Z != -2.2 {
		t.Fatalf("kept=%+v clipped=%+v", kept, clipped)
	}
}

func TestMarkGroundClippedMarksOnlyAffectedClusters(t *testing.T) {
	clusters := []WorldCluster{
		{CentroidX: 0, CentroidY: 0, BoundingBoxLength: 2, BoundingBoxWidth: 1},
		{CentroidX: 10, CentroidY: 0, BoundingBoxLength: 2, BoundingBoxWidth: 1},
	}
	MarkGroundClipped(clusters, []WorldPoint{{X: .9, Y: 0}, {X: 50, Y: 50}})
	if !clusters[0].GroundClipped || clusters[1].GroundClipped {
		t.Fatalf("ground clipping flags = %+v", clusters)
	}
}
