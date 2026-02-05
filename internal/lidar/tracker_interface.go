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

	// GetLastAssociations returns the cluster-to-track mapping from the
	// most recent Update() call. The slice is indexed by cluster index;
	// each element is a trackID (associated) or "" (unassociated).
	GetLastAssociations() []string
}

// Verify at compile time that *Tracker implements TrackerInterface.
var _ TrackerInterface = (*Tracker)(nil)
