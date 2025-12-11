package lidar

import (
	"math"
	"sort"
	"time"
)

// Constants for clustering configuration
const (
	// DefaultDBSCANEps is the default neighborhood radius in meters for DBSCAN
	DefaultDBSCANEps = 0.6
	// DefaultDBSCANMinPts is the default minimum points to form a cluster
	DefaultDBSCANMinPts = 12
	// EstimatedPointsPerCell is used for initial spatial index capacity estimation
	EstimatedPointsPerCell = 4
)

// IdentityTransform4x4 is a 4x4 identity matrix for pose transforms.
// T is row-major: [m00,m01,m02,m03, m10,m11,m12,m13, m20,m21,m22,m23, m30,m31,m32,m33]
var IdentityTransform4x4 = [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

// WorldPoint represents a point in Cartesian world coordinates (site frame).
// This is the output of the polar → world transformation (Phase 3.0).
type WorldPoint struct {
	X, Y, Z   float64   // World frame position (meters)
	Intensity uint8     // Laser return intensity
	Timestamp time.Time // Acquisition time
	SensorID  string    // Source sensor
}

// TransformToWorld converts foreground polar points to world frame coordinates.
// This is Phase 3.0 of the foreground tracking pipeline.
//
// Steps:
// 1. Polar → Sensor Cartesian using spherical to Cartesian conversion
// 2. Sensor Cartesian → World Cartesian using pose transform
//
// If pose is nil, an identity transform is used (sensor frame = world frame).
func TransformToWorld(polarPoints []PointPolar, pose *Pose, sensorID string) []WorldPoint {
	if len(polarPoints) == 0 {
		return nil
	}

	worldPoints := make([]WorldPoint, len(polarPoints))

	// Use identity transform if no pose provided
	T := IdentityTransform4x4
	if pose != nil {
		T = pose.T
	}

	for i, p := range polarPoints {
		// Step 1: Polar → Sensor Cartesian
		sensorX, sensorY, sensorZ := SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)

		// Step 2: Apply pose transform (sensor → world)
		worldX, worldY, worldZ := ApplyPose(sensorX, sensorY, sensorZ, T)

		worldPoints[i] = WorldPoint{
			X:         worldX,
			Y:         worldY,
			Z:         worldZ,
			Intensity: p.Intensity,
			Timestamp: time.Unix(0, p.Timestamp),
			SensorID:  sensorID,
		}
	}

	return worldPoints
}

// TransformPointsToWorld is a convenience function that uses Point (Cartesian sensor frame)
// instead of PointPolar. This is useful when points already have X,Y,Z computed.
func TransformPointsToWorld(points []Point, pose *Pose) []WorldPoint {
	if len(points) == 0 {
		return nil
	}

	worldPoints := make([]WorldPoint, len(points))

	// Use identity transform if no pose provided
	T := IdentityTransform4x4
	if pose != nil {
		T = pose.T
	}

	for i, p := range points {
		worldX, worldY, worldZ := ApplyPose(p.X, p.Y, p.Z, T)

		worldPoints[i] = WorldPoint{
			X:         worldX,
			Y:         worldY,
			Z:         worldZ,
			Intensity: p.Intensity,
			Timestamp: p.Timestamp,
			SensorID:  "", // Will be set by caller if needed
		}
	}

	return worldPoints
}

// =============================================================================
// Phase 3.1: DBSCAN Clustering with Spatial Index (World Frame)
// =============================================================================

// SpatialIndex provides efficient nearest neighbor queries using a regular grid.
// Cell size should approximately match the DBSCAN eps parameter.
type SpatialIndex struct {
	CellSize float64
	Grid     map[int64][]int // Cell ID → point indices
}

// NewSpatialIndex creates a spatial index with the specified cell size.
func NewSpatialIndex(cellSize float64) *SpatialIndex {
	return &SpatialIndex{
		CellSize: cellSize,
		Grid:     make(map[int64][]int),
	}
}

// Build populates the spatial index from a set of world points.
// Uses 2D (x, y) coordinates for cell assignment.
func (si *SpatialIndex) Build(points []WorldPoint) {
	si.Grid = make(map[int64][]int, len(points)/EstimatedPointsPerCell)

	for i, p := range points {
		cellID := si.getCellID(p.X, p.Y)
		si.Grid[cellID] = append(si.Grid[cellID], i)
	}
}

// getCellID computes a unique cell identifier using Szudzik's pairing function.
// Handles negative coordinates correctly.
func (si *SpatialIndex) getCellID(x, y float64) int64 {
	cellX := int64(math.Floor(x / si.CellSize))
	cellY := int64(math.Floor(y / si.CellSize))

	// Map signed integers to non-negative using zigzag encoding
	var a, b int64
	if cellX >= 0 {
		a = 2 * cellX
	} else {
		a = -2*cellX - 1
	}
	if cellY >= 0 {
		b = 2 * cellY
	} else {
		b = -2*cellY - 1
	}

	// Szudzik's elegant pairing function
	var pair int64
	if a >= b {
		pair = a*a + a + b
	} else {
		pair = a + b*b
	}

	return pair
}

// RegionQuery returns indices of all points within eps distance of points[idx].
// Uses 2D (x, y) Euclidean distance for neighborhood queries.
func (si *SpatialIndex) RegionQuery(points []WorldPoint, idx int, eps float64) []int {
	p := points[idx]
	neighbors := []int{}
	eps2 := eps * eps // Use squared distance to avoid sqrt

	// Get base cell coordinates
	cellX := int64(math.Floor(p.X / si.CellSize))
	cellY := int64(math.Floor(p.Y / si.CellSize))

	// Search 3x3 neighborhood of cells
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			// Reconstruct world coordinates for neighbor cell to get correct cell ID
			neighborCellX := cellX + dx
			neighborCellY := cellY + dy

			// Map to non-negative for pairing
			var a, b int64
			if neighborCellX >= 0 {
				a = 2 * neighborCellX
			} else {
				a = -2*neighborCellX - 1
			}
			if neighborCellY >= 0 {
				b = 2 * neighborCellY
			} else {
				b = -2*neighborCellY - 1
			}

			var neighborCellID int64
			if a >= b {
				neighborCellID = a*a + a + b
			} else {
				neighborCellID = a + b*b
			}

			// Check all points in this cell
			for _, candidateIdx := range si.Grid[neighborCellID] {
				candidate := points[candidateIdx]
				dx := candidate.X - p.X
				dy := candidate.Y - p.Y
				dist2 := dx*dx + dy*dy

				if dist2 <= eps2 {
					neighbors = append(neighbors, candidateIdx)
				}
			}
		}
	}

	return neighbors
}

// DBSCANParams contains parameters for the DBSCAN clustering algorithm.
type DBSCANParams struct {
	Eps    float64 // Neighborhood radius in meters
	MinPts int     // Minimum points to form a cluster
}

// DefaultDBSCANParams returns default DBSCAN parameters suitable for vehicle detection.
func DefaultDBSCANParams() DBSCANParams {
	return DBSCANParams{
		Eps:    DefaultDBSCANEps,
		MinPts: DefaultDBSCANMinPts,
	}
}

// DBSCAN performs density-based clustering on world points.
// Uses 2D (x, y) Euclidean distance. Z is used only for cluster features.
// Returns a slice of WorldCluster objects representing detected clusters.
func DBSCAN(points []WorldPoint, params DBSCANParams) []WorldCluster {
	if len(points) == 0 {
		return nil
	}

	n := len(points)
	labels := make([]int, n) // 0=unvisited, -1=noise, >0=clusterID
	clusterID := 0

	// Build spatial index (required for performance)
	spatialIndex := NewSpatialIndex(params.Eps)
	spatialIndex.Build(points)

	for i := 0; i < n; i++ {
		if labels[i] != 0 {
			continue // Already processed
		}

		neighbors := spatialIndex.RegionQuery(points, i, params.Eps)

		if len(neighbors) < params.MinPts {
			labels[i] = -1 // Mark as noise
			continue
		}

		clusterID++
		expandCluster(points, spatialIndex, labels, i, neighbors, clusterID, params.Eps, params.MinPts)
	}

	return buildClusters(points, labels, clusterID)
}

// expandCluster expands a cluster from a core point.
func expandCluster(points []WorldPoint, si *SpatialIndex, labels []int,
	seedIdx int, neighbors []int, clusterID int, eps float64, minPts int) {

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
		newNeighbors := si.RegionQuery(points, idx, eps)

		if len(newNeighbors) >= minPts {
			// Core point - add its neighbors to the queue
			neighbors = append(neighbors, newNeighbors...)
		}
	}
}

// buildClusters creates WorldCluster objects from clustering results.
func buildClusters(points []WorldPoint, labels []int, maxClusterID int) []WorldCluster {
	clusters := make([]WorldCluster, 0, maxClusterID)

	for cid := 1; cid <= maxClusterID; cid++ {
		// Collect points for this cluster
		clusterPoints := make([]WorldPoint, 0)
		for i, label := range labels {
			if label == cid {
				clusterPoints = append(clusterPoints, points[i])
			}
		}

		if len(clusterPoints) == 0 {
			continue
		}

		cluster := computeClusterMetrics(clusterPoints, int64(cid))
		clusters = append(clusters, cluster)
	}

	return clusters
}

// computeClusterMetrics computes metrics for a cluster of world points.
func computeClusterMetrics(points []WorldPoint, clusterID int64) WorldCluster {
	n := float64(len(points))

	// Compute centroid
	var sumX, sumY, sumZ float64
	for _, p := range points {
		sumX += p.X
		sumY += p.Y
		sumZ += p.Z
	}
	centroidX := float32(sumX / n)
	centroidY := float32(sumY / n)
	centroidZ := float32(sumZ / n)

	// Compute bounding box and other stats
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	minZ, maxZ := points[0].Z, points[0].Z
	var sumIntensity uint64
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
		sumIntensity += uint64(p.Intensity)
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
		tsUnixNanos = points[0].Timestamp.UnixNano()
		sensorID = points[0].SensorID
	}

	// Phase 1: Compute cluster quality metrics
	length := float32(maxX - minX)
	width := float32(maxY - minY)
	height := float32(maxZ - minZ)

	// Cluster density: points per cubic meter
	volume := length * width * height
	var density float32
	if volume > 0 {
		density = float32(len(points)) / volume
	}

	// Aspect ratio: max dimension / min dimension
	var aspectRatio float32
	if length > 0 && width > 0 {
		if length > width {
			aspectRatio = length / width
		} else {
			aspectRatio = width / length
		}
	}

	return WorldCluster{
		ClusterID:         clusterID,
		SensorID:          sensorID,
		TSUnixNanos:       tsUnixNanos,
		CentroidX:         centroidX,
		CentroidY:         centroidY,
		CentroidZ:         centroidZ,
		BoundingBoxLength: length,
		BoundingBoxWidth:  width,
		BoundingBoxHeight: height,
		PointsCount:       len(points),
		HeightP95:         float32(heights[p95Idx]),
		IntensityMean:     float32(sumIntensity / uint64(len(points))),
		ClusterDensity:    density,
		AspectRatio:       aspectRatio,
		NoisePointsCount:  0, // Will be computed if noise points are tracked
	}
}
