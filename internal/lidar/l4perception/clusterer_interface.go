package l4perception

import "time"

// ClustererInterface abstracts the clustering implementation.
// This interface enables swapping clustering algorithms and testing with
// different clustering strategies without modifying the tracking pipeline.
type ClustererInterface interface {
	// Cluster performs clustering on world points.
	// Returns a slice of WorldCluster objects representing detected clusters.
	// Clusters are sorted deterministically by centroid (X, then Y) for reproducibility.
	Cluster(points []WorldPoint, sensorID string, timestamp time.Time) []WorldCluster

	// GetParams returns the current clustering parameters.
	GetParams() ClusteringParams

	// SetParams updates the clustering parameters.
	// This allows runtime tuning of clustering behaviour.
	SetParams(params ClusteringParams)
}

// ClusteringParams holds clustering algorithm parameters.
// These are intentionally generic to support different clustering algorithms.
type ClusteringParams struct {
	Eps    float64 // Neighbourhood radius in metres (for DBSCAN)
	MinPts int     // Minimum points to form a cluster
}
