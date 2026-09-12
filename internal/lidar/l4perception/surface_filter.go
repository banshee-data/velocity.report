package l4perception

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// SurfaceHeightFilter removes points by height above a fitted local surface,
// rather than by absolute sensor-frame Z. Floor and Ceiling are metres above
// that surface.
type SurfaceHeightFilter struct {
	Surface l3grid.GroundSurface
	Floor   float64
	Ceiling float64
}

// Filter separates kept points from lower-surface rejections. Only lower
// rejections are returned because they can indicate a cluster clipped at its
// contact patch; overhead rejections carry no such geometry implication.
func (f SurfaceHeightFilter) Filter(points []WorldPoint) (kept, lowerRejected []WorldPoint) {
	kept = make([]WorldPoint, 0, len(points))
	for _, point := range points {
		height := point.Z - f.Surface.HeightAt(point.X, point.Y)
		if height < f.Floor {
			lowerRejected = append(lowerRejected, point)
			continue
		}
		if f.Ceiling > 0 && height > f.Ceiling {
			continue
		}
		kept = append(kept, point)
	}
	return kept, lowerRejected
}

// MarkGroundClipped marks a cluster when a lower-filtered point fell within
// its conservative XY footprint. The point was removed before DBSCAN, so a
// direct member identity is unavailable; this spatial criterion preserves the
// useful fact without pretending it is a segmentation assignment.
func MarkGroundClipped(clusters []WorldCluster, lowerRejected []WorldPoint) {
	for i := range clusters {
		cluster := &clusters[i]
		radius := math.Max(float64(cluster.BoundingBoxLength), float64(cluster.BoundingBoxWidth))/2 + .25
		for _, point := range lowerRejected {
			if math.Hypot(point.X-float64(cluster.CentroidX), point.Y-float64(cluster.CentroidY)) <= radius {
				cluster.GroundClipped = true
				break
			}
		}
	}
}
