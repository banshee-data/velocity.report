package l4perception

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

// EstimatedPointsPerCell is used for initial spatial index capacity estimation.
const EstimatedPointsPerCell = 4

// IdentityTransform4x4 is a 4x4 identity matrix for pose transforms.
// T is row-major: [m00,m01,m02,m03, m10,m11,m12,m13, m20,m21,m22,m23, m30,m31,m32,m33]
var IdentityTransform4x4 = [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

// TransformToWorld converts foreground polar points to world frame coordinates.
// Polar-to-world coordinate transformation stage.
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

// SphericalToCartesian converts spherical coordinates to Cartesian.
// Uses the sensor coordinate system (X=forward, Y=right, Z=up).
func SphericalToCartesian(distance, azimuthDeg, elevationDeg float64) (x, y, z float64) {
	azimuthRad := azimuthDeg * math.Pi / 180.0
	elevationRad := elevationDeg * math.Pi / 180.0

	cosElevation := math.Cos(elevationRad)
	sinElevation := math.Sin(elevationRad)
	cosAzimuth := math.Cos(azimuthRad)
	sinAzimuth := math.Sin(azimuthRad)

	x = distance * cosElevation * sinAzimuth
	y = distance * cosElevation * cosAzimuth
	z = distance * sinElevation
	return
}

// ApplyPose applies a 4x4 row-major transform T to point (x,y,z).
// T is expected as [16]float64 row-major: m00,m01,m02,m03, m10,...
func ApplyPose(x, y, z float64, T [16]float64) (wx, wy, wz float64) {
	wx = T[0]*x + T[1]*y + T[2]*z + T[3]
	wy = T[4]*x + T[5]*y + T[6]*z + T[7]
	wz = T[8]*x + T[9]*y + T[10]*z + T[11]
	return
}

// =============================================================================
// DBSCAN Clustering with Spatial Index (World Frame)
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
	Eps                   float64 // Neighbourhood radius in metres
	MinPts                int     // Minimum points to form a cluster
	MaxClusterDiameter    float64 // Upper bound (metres) for longest OBB dimension
	MinClusterDiameter    float64 // Lower bound (metres) for longest OBB dimension
	MaxClusterAspectRatio float64 // Maximum length/width ratio

	// MaxInputPoints caps the number of points fed into the core DBSCAN
	// loop. When the input exceeds this value, uniform random subsampling
	// is applied to keep runtime bounded. Zero or negative disables the
	// cap. Typical value: 8000.
	MaxInputPoints int
}

// DefaultDBSCANParams returns DBSCAN parameters loaded from the canonical
// tuning defaults file (config/tuning.defaults.json).
// Panics if the file cannot be found.
func DefaultDBSCANParams() DBSCANParams {
	cfg := config.MustLoadDefaultConfig()
	return DBSCANParams{
		Eps:                   cfg.GetForegroundDBSCANEps(),
		MinPts:                cfg.GetForegroundMinClusterPoints(),
		MaxClusterDiameter:    cfg.GetMaxClusterDiameter(),
		MinClusterDiameter:    cfg.GetMinClusterDiameter(),
		MaxClusterAspectRatio: cfg.GetMaxClusterAspectRatio(),
	}
}

// DBSCAN performs density-based clustering on world points.
// Uses 2D (x, y) Euclidean distance. Z is used only for cluster features.
// Returns a slice of WorldCluster objects representing detected clusters.
//
// When the input exceeds MaxInputPoints, uniform random subsampling is
// applied to bound worst-case runtime. MaxInputPoints <= 0 disables
// the cap (default behaviour for backward compatibility).
func DBSCAN(points []WorldPoint, params DBSCANParams) []WorldCluster {
	if len(points) == 0 {
		return nil
	}

	// Safety cap: subsample when point count exceeds the threshold to
	// prevent O(n²) worst-case DBSCAN on unexpectedly dense frames.
	if params.MaxInputPoints > 0 && len(points) > params.MaxInputPoints {
		points = uniformSubsample(points, params.MaxInputPoints)
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

	return buildClusters(points, labels, clusterID, params)
}

// uniformSubsample returns a random subset of n points from the input
// using Fisher-Yates partial shuffle. The original slice is not modified.
//
// A local *rand.Rand seeded from the current time is used rather than the
// global generator to avoid lock contention when DBSCAN is called
// concurrently and to make the sampling behaviour independent of global
// seed state elsewhere in the process.
func uniformSubsample(points []WorldPoint, n int) []WorldPoint {
	if n >= len(points) {
		return points
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // non-crypto use
	// Work on a copy of the index space to avoid mutating the caller's slice.
	idx := make([]int, len(points))
	for i := range idx {
		idx[i] = i
	}
	for i := 0; i < n; i++ {
		j := i + rng.Intn(len(idx)-i)
		idx[i], idx[j] = idx[j], idx[i]
	}
	result := make([]WorldPoint, n)
	for i := 0; i < n; i++ {
		result[i] = points[idx[i]]
	}
	return result
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
// Uses a single pass over labels to bucket points by cluster ID,
// avoiding repeated O(n) scans per cluster.
func buildClusters(points []WorldPoint, labels []int, maxClusterID int, params DBSCANParams) []WorldCluster {
	// Single pass: bucket points by cluster ID
	buckets := make([][]WorldPoint, maxClusterID+1)
	for i, label := range labels {
		if label >= 1 && label <= maxClusterID {
			buckets[label] = append(buckets[label], points[i])
		}
	}

	clusters := make([]WorldCluster, 0, maxClusterID)
	for cid := 1; cid <= maxClusterID; cid++ {
		clusterPoints := buckets[cid]
		if len(clusterPoints) == 0 {
			continue
		}
		cluster := computeClusterMetrics(clusterPoints, int64(cid))

		// Reject extreme-size and extreme-aspect clusters to filter out
		// environmental artefacts (walls, hedges, speckle noise).
		longest := cluster.BoundingBoxLength
		if cluster.BoundingBoxWidth > longest {
			longest = cluster.BoundingBoxWidth
		}
		shortest := cluster.BoundingBoxWidth
		if cluster.BoundingBoxLength < shortest {
			shortest = cluster.BoundingBoxLength
		}
		if float64(longest) > params.MaxClusterDiameter || float64(longest) < params.MinClusterDiameter {
			continue
		}
		// Only enforce aspect ratio when the shortest axis is above the
		// noise floor (0.03 m). Ultra-thin clusters viewed edge-on or
		// along a radial arc are legitimate detections; their OBB width
		// is near-zero due to LiDAR angular resolution, not because they
		// are environmental artefacts.
		if float64(shortest) > 0.03 && float64(longest)/float64(shortest) > params.MaxClusterAspectRatio {
			continue
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

// computeClusterMetrics computes metrics for a cluster of world points.
func computeClusterMetrics(points []WorldPoint, clusterID int64) WorldCluster {
	n := float64(len(points))

	// Compute centroid as medoid: the actual cluster point closest to the
	// arithmetic mean (task 3.2). For non-convex clusters (L-shapes, arcs)
	// the arithmetic mean can fall outside the point cloud, causing unstable
	// association. The medoid is guaranteed to lie on a real measurement.
	var sumX, sumY, sumZ float64
	for _, p := range points {
		sumX += p.X
		sumY += p.Y
		sumZ += p.Z
	}
	meanX := sumX / n
	meanY := sumY / n
	meanZ := sumZ / n

	bestIdx := 0
	bestDist := math.MaxFloat64
	for i, p := range points {
		dx := p.X - meanX
		dy := p.Y - meanY
		dz := p.Z - meanZ
		d := dx*dx + dy*dy + dz*dz
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	centroidX := float32(points[bestIdx].X)
	centroidY := float32(points[bestIdx].Y)
	centroidZ := float32(points[bestIdx].Z)

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

	// Compute oriented bounding box via PCA
	obb := EstimateOBBFromCluster(points)

	// Use OBB dimensions for Length/Width since the renderer rotates the box
	// by the OBB heading. Using AABB dimensions with an OBB rotation would
	// produce a box that doesn't encompass the cluster points.
	bboxLength := obb.Length
	bboxWidth := obb.Width
	bboxHeight := obb.Height

	return WorldCluster{
		ClusterID:         clusterID,
		SensorID:          sensorID,
		TSUnixNanos:       tsUnixNanos,
		CentroidX:         centroidX,
		CentroidY:         centroidY,
		CentroidZ:         centroidZ,
		BoundingBoxLength: bboxLength,
		BoundingBoxWidth:  bboxWidth,
		BoundingBoxHeight: bboxHeight,
		PointsCount:       len(points),
		HeightP95:         float32(heights[p95Idx]),
		IntensityMean:     float32(sumIntensity / uint64(len(points))),
		OBB:               &obb,
	}
}
