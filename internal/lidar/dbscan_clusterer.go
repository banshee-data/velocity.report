package lidar

import (
	"sort"
	"time"
)

// DBSCANClusterer implements ClustererInterface using the DBSCAN algorithm.
// DBSCAN (Density-Based Spatial Clustering of Applications with Noise) is
// well-suited for detecting vehicle-shaped clusters in LiDAR point clouds.
type DBSCANClusterer struct {
	params ClusteringParams
}

// NewDBSCANClusterer creates a new DBSCAN clusterer with the specified parameters.
func NewDBSCANClusterer(eps float64, minPts int) *DBSCANClusterer {
	return &DBSCANClusterer{
		params: ClusteringParams{
			Eps:    eps,
			MinPts: minPts,
		},
	}
}

// NewDefaultDBSCANClusterer creates a DBSCAN clusterer with default parameters.
func NewDefaultDBSCANClusterer() *DBSCANClusterer {
	params := DefaultDBSCANParams()
	return NewDBSCANClusterer(params.Eps, params.MinPts)
}

// Cluster performs DBSCAN clustering on world points.
// The output is deterministic: clusters are sorted by centroid (X, then Y)
// to ensure golden replay tests produce identical results.
func (c *DBSCANClusterer) Cluster(points []WorldPoint, sensorID string, timestamp time.Time) []WorldCluster {
	if len(points) == 0 {
		return nil
	}

	// Build full DBSCANParams from defaults (includes filter thresholds)
	// and override eps/minPts from runtime config.
	dbscanParams := DefaultDBSCANParams()
	dbscanParams.Eps = c.params.Eps
	dbscanParams.MinPts = c.params.MinPts

	// Run DBSCAN clustering
	clusters := DBSCAN(points, dbscanParams)

	// Sort clusters deterministically by centroid (X, then Y)
	// This ensures reproducibility for golden replay tests
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].CentroidX != clusters[j].CentroidX {
			return clusters[i].CentroidX < clusters[j].CentroidX
		}
		return clusters[i].CentroidY < clusters[j].CentroidY
	})

	return clusters
}

// GetParams returns the current clustering parameters.
func (c *DBSCANClusterer) GetParams() ClusteringParams {
	return c.params
}

// SetParams updates the clustering parameters.
func (c *DBSCANClusterer) SetParams(params ClusteringParams) {
	c.params = params
}

// Verify at compile time that *DBSCANClusterer implements ClustererInterface.
var _ ClustererInterface = (*DBSCANClusterer)(nil)
