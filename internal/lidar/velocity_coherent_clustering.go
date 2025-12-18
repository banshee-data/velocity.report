package lidar

import (
	"math"
	"sort"
)

// =============================================================================
// Phase 2: Velocity-Coherent Clustering (6D DBSCAN)
// =============================================================================

// Clustering6DConfig contains parameters for 6D (position + velocity) DBSCAN.
type Clustering6DConfig struct {
	// Spatial clustering parameters
	PositionEps float64 // meters, default 0.6
	VelocityEps float64 // m/s, default 1.0

	// REDUCED minimum points (from 12 to 3) for velocity-coherent clustering
	MinPts int // default 3

	// Weights for 6D distance metric
	PositionWeight float64 // default 1.0
	VelocityWeight float64 // default 2.0 (velocity more important)

	// Velocity confidence filter
	MinVelocityConfidence float32 // default 0.3
}

// DefaultClustering6DConfig returns default 6D clustering parameters.
func DefaultClustering6DConfig() Clustering6DConfig {
	return Clustering6DConfig{
		PositionEps:           0.6,
		VelocityEps:           1.0,
		MinPts:                3, // Reduced from 12 for velocity-coherent tracking
		PositionWeight:        1.0,
		VelocityWeight:        2.0,
		MinVelocityConfidence: 0.3,
	}
}

// VelocityCoherentCluster represents a group of points moving together.
type VelocityCoherentCluster struct {
	ClusterID int64

	// Centroid position (world frame)
	CentroidX, CentroidY, CentroidZ float64

	// Average velocity (world frame, m/s)
	VelocityX, VelocityY, VelocityZ float64

	// Cluster statistics
	PointCount         int
	VelocityConfidence float32 // Average confidence across points

	// Point indices in the original array
	PointIndices []int

	// Bounding box and features
	BoundingBoxLength float32
	BoundingBoxWidth  float32
	BoundingBoxHeight float32
	HeightP95         float32
	IntensityMean     float32

	// Timestamp (from first point)
	TSUnixNanos int64
	SensorID    string
}

// SpatialIndex6D provides efficient nearest neighbor queries in 6D space.
type SpatialIndex6D struct {
	PositionCellSize float64
	VelocityCellSize float64
	Grid             map[int64][]int // Cell ID → point indices
}

// NewSpatialIndex6D creates a new 6D spatial index.
func NewSpatialIndex6D(positionCellSize, velocityCellSize float64) *SpatialIndex6D {
	return &SpatialIndex6D{
		PositionCellSize: positionCellSize,
		VelocityCellSize: velocityCellSize,
		Grid:             make(map[int64][]int),
	}
}

// Build populates the 6D spatial index from velocity points.
func (si *SpatialIndex6D) Build(points []PointVelocity) {
	si.Grid = make(map[int64][]int, len(points)/4)

	for i, p := range points {
		cellID := si.getCellID4D(p.X, p.Y, p.VX, p.VY)
		si.Grid[cellID] = append(si.Grid[cellID], i)
	}
}

// getCellID4D computes a unique cell ID for 4D space (x, y, vx, vy).
// We use 4D (ignoring Z and VZ) for spatial indexing efficiency while
// maintaining velocity coherence. The full 6D distance check is done
// during the actual neighbor validation.
func (si *SpatialIndex6D) getCellID4D(x, y, vx, vy float64) int64 {
	// Discretize position
	cellX := int64(math.Floor(x / si.PositionCellSize))
	cellY := int64(math.Floor(y / si.PositionCellSize))

	// Discretize velocity
	cellVX := int64(math.Floor(vx / si.VelocityCellSize))
	cellVY := int64(math.Floor(vy / si.VelocityCellSize))

	// Combine into a single hash using polynomial rolling
	// This provides good distribution for grid-based spatial indexing
	const prime = 31
	h := cellX
	h = h*prime + cellY
	h = h*prime + cellVX
	h = h*prime + cellVY
	return h
}

// RegionQuery6D returns indices of points within eps distance in 6D space.
func (si *SpatialIndex6D) RegionQuery6D(
	points []PointVelocity,
	idx int,
	positionEps, velocityEps float64,
	positionWeight, velocityWeight float64,
) []int {
	p := points[idx]
	neighbors := []int{}

	// Search neighboring cells in 4D grid (3^4 = 81 cells)
	// We search all 81 neighboring cells and validate with full 6D distance
	posEps2 := positionEps * positionEps
	velEps2 := velocityEps * velocityEps

	cellX := int64(math.Floor(p.X / si.PositionCellSize))
	cellY := int64(math.Floor(p.Y / si.PositionCellSize))
	cellVX := int64(math.Floor(p.VX / si.VelocityCellSize))
	cellVY := int64(math.Floor(p.VY / si.VelocityCellSize))

	// Search 3x3x3x3 neighborhood
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for dvx := int64(-1); dvx <= 1; dvx++ {
				for dvy := int64(-1); dvy <= 1; dvy++ {
					neighborCellID := si.getCellID4D(
						float64(cellX+dx)*si.PositionCellSize,
						float64(cellY+dy)*si.PositionCellSize,
						float64(cellVX+dvx)*si.VelocityCellSize,
						float64(cellVY+dvy)*si.VelocityCellSize,
					)

					for _, candidateIdx := range si.Grid[neighborCellID] {
						candidate := points[candidateIdx]

						// Compute full 6D distance (including Z and VZ)
						posDist2 := math.Pow(candidate.X-p.X, 2) +
							math.Pow(candidate.Y-p.Y, 2) +
							math.Pow(candidate.Z-p.Z, 2)
						velDist2 := math.Pow(candidate.VX-p.VX, 2) +
							math.Pow(candidate.VY-p.VY, 2) +
							math.Pow(candidate.VZ-p.VZ, 2)

						// Check if within epsilon for both dimensions
						if posDist2 <= posEps2 && velDist2 <= velEps2 {
							neighbors = append(neighbors, candidateIdx)
						}
					}
				}
			}
		}
	}

	return neighbors
}

// DBSCAN6D performs density-based clustering in 6D (position + velocity) space.
// Returns velocity-coherent clusters with reduced MinPts (3 instead of 12).
func DBSCAN6D(points []PointVelocity, config Clustering6DConfig) []VelocityCoherentCluster {
	if len(points) == 0 {
		return nil
	}

	// Filter points by velocity confidence
	validPoints := make([]PointVelocity, 0, len(points))
	validIndices := make([]int, 0, len(points))
	for i, p := range points {
		if p.VelocityConfidence >= config.MinVelocityConfidence {
			validPoints = append(validPoints, p)
			validIndices = append(validIndices, i)
		}
	}

	if len(validPoints) == 0 {
		return nil
	}

	n := len(validPoints)
	labels := make([]int, n) // 0=unvisited, -1=noise, >0=clusterID
	clusterID := 0

	// Build 6D spatial index
	spatialIndex := NewSpatialIndex6D(config.PositionEps, config.VelocityEps)
	spatialIndex.Build(validPoints)

	for i := 0; i < n; i++ {
		if labels[i] != 0 {
			continue // Already processed
		}

		// Region query in 6D space
		neighbors := spatialIndex.RegionQuery6D(
			validPoints, i,
			config.PositionEps, config.VelocityEps,
			config.PositionWeight, config.VelocityWeight,
		)

		// Use REDUCED minimum points (3 instead of 12)
		if len(neighbors) < config.MinPts {
			labels[i] = -1 // Mark as noise
			continue
		}

		clusterID++
		expandCluster6D(validPoints, spatialIndex, labels, i, neighbors, clusterID, config)
	}

	return buildVelocityClusters(validPoints, validIndices, labels, clusterID)
}

// expandCluster6D expands a cluster from a core point in 6D space.
func expandCluster6D(
	points []PointVelocity,
	si *SpatialIndex6D,
	labels []int,
	seedIdx int,
	neighbors []int,
	clusterID int,
	config Clustering6DConfig,
) {
	labels[seedIdx] = clusterID

	// Use a queue-based approach for expansion
	for j := 0; j < len(neighbors); j++ {
		idx := neighbors[j]

		if labels[idx] == -1 {
			labels[idx] = clusterID // Noise becomes border point
		}

		if labels[idx] != 0 {
			continue // Already processed
		}

		labels[idx] = clusterID
		newNeighbors := si.RegionQuery6D(
			points, idx,
			config.PositionEps, config.VelocityEps,
			config.PositionWeight, config.VelocityWeight,
		)

		if len(newNeighbors) >= config.MinPts {
			// Core point - add its neighbors to the queue
			neighbors = append(neighbors, newNeighbors...)
		}
	}
}

// buildVelocityClusters creates VelocityCoherentCluster objects from clustering results.
func buildVelocityClusters(
	points []PointVelocity,
	originalIndices []int,
	labels []int,
	maxClusterID int,
) []VelocityCoherentCluster {
	clusters := make([]VelocityCoherentCluster, 0, maxClusterID)

	for cid := 1; cid <= maxClusterID; cid++ {
		// Collect points for this cluster
		var clusterPoints []PointVelocity
		var pointIndices []int

		for i, label := range labels {
			if label == cid {
				clusterPoints = append(clusterPoints, points[i])
				pointIndices = append(pointIndices, originalIndices[i])
			}
		}

		if len(clusterPoints) == 0 {
			continue
		}

		cluster := computeVelocityClusterMetrics(clusterPoints, pointIndices, int64(cid))
		clusters = append(clusters, cluster)
	}

	return clusters
}

// computeVelocityClusterMetrics computes metrics for a velocity-coherent cluster.
func computeVelocityClusterMetrics(
	points []PointVelocity,
	indices []int,
	clusterID int64,
) VelocityCoherentCluster {
	n := float64(len(points))

	// Compute centroid and average velocity
	var sumX, sumY, sumZ float64
	var sumVX, sumVY, sumVZ float64
	var sumConfidence float32
	var sumIntensity uint64

	for _, p := range points {
		sumX += p.X
		sumY += p.Y
		sumZ += p.Z
		sumVX += p.VX
		sumVY += p.VY
		sumVZ += p.VZ
		sumConfidence += p.VelocityConfidence
		sumIntensity += uint64(p.Intensity)
	}

	// Compute bounding box
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	minZ, maxZ := points[0].Z, points[0].Z
	heights := make([]float64, len(points))

	for i, p := range points {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
		if p.Z < minZ {
			minZ = p.Z
		}
		if p.Z > maxZ {
			maxZ = p.Z
		}
		heights[i] = p.Z
	}

	// Compute P95 height
	sort.Float64s(heights)
	p95Idx := int(0.95 * float64(len(heights)))
	if p95Idx >= len(heights) {
		p95Idx = len(heights) - 1
	}

	// Get timestamp and sensor ID from first point
	var tsUnixNanos int64
	var sensorID string
	if len(points) > 0 {
		tsUnixNanos = points[0].TimestampNanos
		sensorID = points[0].SensorID
	}

	return VelocityCoherentCluster{
		ClusterID:          clusterID,
		CentroidX:          sumX / n,
		CentroidY:          sumY / n,
		CentroidZ:          sumZ / n,
		VelocityX:          sumVX / n,
		VelocityY:          sumVY / n,
		VelocityZ:          sumVZ / n,
		PointCount:         len(points),
		VelocityConfidence: sumConfidence / float32(len(points)),
		PointIndices:       indices,
		BoundingBoxLength:  float32(maxX - minX),
		BoundingBoxWidth:   float32(maxY - minY),
		BoundingBoxHeight:  float32(maxZ - minZ),
		HeightP95:          float32(heights[p95Idx]),
		IntensityMean:      float32(sumIntensity / uint64(len(points))),
		TSUnixNanos:        tsUnixNanos,
		SensorID:           sensorID,
	}
}

// Distance6D computes weighted distance in position-velocity space.
func Distance6D(p1, p2 PointVelocity, positionWeight, velocityWeight float64) float64 {
	// Position distance (Euclidean 3D)
	dx := p1.X - p2.X
	dy := p1.Y - p2.Y
	dz := p1.Z - p2.Z
	positionDist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Velocity distance (Euclidean 3D)
	dvx := p1.VX - p2.VX
	dvy := p1.VY - p2.VY
	dvz := p1.VZ - p2.VZ
	velocityDist := math.Sqrt(dvx*dvx + dvy*dvy + dvz*dvz)

	// Weighted combination
	return positionWeight*positionDist + velocityWeight*velocityDist
}
