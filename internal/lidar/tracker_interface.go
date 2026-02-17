package lidar

import "time"

// TrackerInterface abstracts the tracking implementation.
// This interface enables dependency injection and golden replay testing
// by decoupling the tracking algorithm from the pipeline infrastructure.
type TrackerInterface interface {
	// Update processes a new frame of clusters and updates tracks.
	// This is the main entry point for the tracking pipeline.
	Update(clusters []WorldCluster, timestamp time.Time)

	// GetActiveTracks returns currently active (non-deleted) tracks.
	// Active tracks include both tentative and confirmed states.
	GetActiveTracks() []*TrackedObject

	// GetConfirmedTracks returns only confirmed tracks.
	// These are tracks that have accumulated sufficient hits to be reliable.
	GetConfirmedTracks() []*TrackedObject

	// GetTrack returns a track by ID, or nil if not found.
	GetTrack(trackID string) *TrackedObject

	// GetTrackCount returns counts of tracks by state.
	// Returns total, tentative, confirmed, and deleted counts.
	GetTrackCount() (total, tentative, confirmed, deleted int)

	// GetAllTracks returns all tracks including deleted ones.
	// Useful for debugging and comprehensive state inspection.
	GetAllTracks() []*TrackedObject

	// GetRecentlyDeletedTracks returns deleted tracks within the grace period.
	// Used for fade-out rendering in the visualiser.
	GetRecentlyDeletedTracks(nowNanos int64) []*TrackedObject

	// GetLastAssociations returns the cluster-to-track mapping from the
	// most recent Update() call. The slice is indexed by cluster index;
	// each element is a trackID (associated) or "" (unassociated).
	GetLastAssociations() []string

	// GetTrackingMetrics returns aggregate velocity-trail alignment metrics
	// across all active tracks. Used by the sweep tool to evaluate
	// tracking parameter configurations.
	GetTrackingMetrics() TrackingMetrics

	// RecordFrameStats records per-frame foreground point statistics.
	// totalForegroundPoints is the number of world-frame points that entered
	// DBSCAN; clusteredPoints is the sum of PointsCount across all clusters
	// produced by DBSCAN. The difference represents noise points that were
	// not captured by any bounding box.
	RecordFrameStats(totalForegroundPoints, clusteredPoints int)

	// UpdateClassification writes classification results back to the live
	// track under the tracker lock. Call this after ClassifyAndUpdate on a
	// snapshot to propagate the label to in-memory state. Safe for
	// concurrent readers (task 4.3).
	UpdateClassification(trackID, objectClass string, confidence float32, model string)

	// AdvanceMisses increments the miss counter for all active tracks by one.
	// Called on throttled (skipped) frames so tracks are not artificially
	// kept alive when no clusters are delivered (task 7.2).
	AdvanceMisses(timestamp time.Time)
}

// Verify at compile time that *Tracker implements TrackerInterface.
var _ TrackerInterface = (*Tracker)(nil)
