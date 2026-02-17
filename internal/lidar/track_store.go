package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// TrackStore defines the interface for track persistence operations.
// This is now an alias for the implementation in storage/sqlite.
type TrackStore = sqlite.TrackStore

// TrackObservation represents a single observation of a track at a point in time.
// This is now an alias for the implementation in storage/sqlite.
type TrackObservation = sqlite.TrackObservation

// Standalone functions for track persistence.
// These are now aliases for the implementations in storage/sqlite.

// InsertCluster inserts a cluster into the database and returns its ID.
var InsertCluster = sqlite.InsertCluster

// InsertTrack inserts a new track into the database.
var InsertTrack = sqlite.InsertTrack

// UpdateTrack updates an existing track in the database.
var UpdateTrack = sqlite.UpdateTrack

// InsertTrackObservation inserts a track observation into the database.
var InsertTrackObservation = sqlite.InsertTrackObservation

// ClearTracks removes all tracks for a sensor.
var ClearTracks = sqlite.ClearTracks

// GetActiveTracks retrieves active tracks for a sensor.
var GetActiveTracks = sqlite.GetActiveTracks

// GetTracksInRange retrieves tracks within a time range.
var GetTracksInRange = sqlite.GetTracksInRange

// GetTrackObservations retrieves observations for a track.
var GetTrackObservations = sqlite.GetTrackObservations

// GetTrackObservationsInRange retrieves observations within a time range.
var GetTrackObservationsInRange = sqlite.GetTrackObservationsInRange

// GetRecentClusters retrieves recent clusters within a time range.
var GetRecentClusters = sqlite.GetRecentClusters

// PruneDeletedTracks removes old deleted tracks beyond the TTL.
var PruneDeletedTracks = sqlite.PruneDeletedTracks

// ClearRuns removes all runs for a sensor.
var ClearRuns = sqlite.ClearRuns

// DeleteRun removes a specific run and its tracks.
var DeleteRun = sqlite.DeleteRun
