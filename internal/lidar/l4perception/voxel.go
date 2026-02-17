package l4perception

import "math"

// VoxelGrid performs 3D voxel grid downsampling on world-frame points.
// Each occupied voxel retains a single representative point (the one closest
// to the voxel centroid), reducing point density while preserving spatial
// structure better than uniform stride decimation.
//
// leafSize is the side-length (metres) of each cubic voxel.
// Typical value: 0.05–0.15 m for street-level LiDAR.
func VoxelGrid(points []WorldPoint, leafSize float64) []WorldPoint {
	if len(points) == 0 || leafSize <= 0 {
		return points
	}

	invLeaf := 1.0 / leafSize

	// Each voxel accumulates sum(X,Y,Z) and count so we can compute the
	// centroid, then we pick the original point closest to that centroid.
	type voxelAccum struct {
		sumX, sumY, sumZ float64
		count            int
		bestIdx          int     // index of point closest to centroid (resolved lazily)
		bestDist2        float64 // squared distance to centroid
	}

	voxels := make(map[[3]int64]*voxelAccum, len(points)/4)

	// Pass 1: accumulate per-voxel statistics.
	for i, p := range points {
		key := [3]int64{
			int64(math.Floor(p.X * invLeaf)),
			int64(math.Floor(p.Y * invLeaf)),
			int64(math.Floor(p.Z * invLeaf)),
		}
		acc, exists := voxels[key]
		if !exists {
			acc = &voxelAccum{bestIdx: i, bestDist2: math.MaxFloat64}
			voxels[key] = acc
		}
		acc.sumX += p.X
		acc.sumY += p.Y
		acc.sumZ += p.Z
		acc.count++
	}

	// Pass 2: for each voxel, compute centroid and pick closest point.
	// We iterate points again but only for occupied voxels.
	for i, p := range points {
		key := [3]int64{
			int64(math.Floor(p.X * invLeaf)),
			int64(math.Floor(p.Y * invLeaf)),
			int64(math.Floor(p.Z * invLeaf)),
		}
		acc := voxels[key]
		cx := acc.sumX / float64(acc.count)
		cy := acc.sumY / float64(acc.count)
		cz := acc.sumZ / float64(acc.count)
		dx := p.X - cx
		dy := p.Y - cy
		dz := p.Z - cz
		d2 := dx*dx + dy*dy + dz*dz
		if d2 < acc.bestDist2 {
			acc.bestDist2 = d2
			acc.bestIdx = i
		}
	}

	// Collect survivors.
	result := make([]WorldPoint, 0, len(voxels))
	for _, acc := range voxels {
		result = append(result, points[acc.bestIdx])
	}

	return result
}
