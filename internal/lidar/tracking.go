package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Type aliases for tracking types migrated to l5tracks.
// These maintain backward compatibility for existing code that imports from internal/lidar.

// TrackState represents the lifecycle state of a track.
type TrackState = l5tracks.TrackState

// TrackerConfig holds configuration parameters for the tracker.
type TrackerConfig = l5tracks.TrackerConfig

// TrackPoint is a 2D position with timestamp used in track history.
type TrackPoint = l5tracks.TrackPoint

// TrackedObject represents a single tracked object.
type TrackedObject = l5tracks.TrackedObject

// TrackingMetrics holds aggregate tracking quality metrics.
type TrackingMetrics = l5tracks.TrackingMetrics

// TrackAlignmentMetrics holds velocity alignment metrics for a single track.
type TrackAlignmentMetrics = l5tracks.TrackAlignmentMetrics

// Tracker manages multi-object tracking.
type Tracker = l5tracks.Tracker

// DebugCollector interface for tracking algorithm instrumentation.
type DebugCollector = l5tracks.DebugCollector

// Constants re-exported from l5tracks.
const (
TrackTentative = l5tracks.TrackTentative
TrackConfirmed = l5tracks.TrackConfirmed
TrackDeleted   = l5tracks.TrackDeleted

MinDeterminantThreshold   = l5tracks.MinDeterminantThreshold
SingularDistanceRejection = l5tracks.SingularDistanceRejection
)

// Function aliases re-exported from l5tracks.
var (
NewTracker             = l5tracks.NewTracker
DefaultTrackerConfig   = l5tracks.DefaultTrackerConfig
TrackerConfigFromTuning = l5tracks.TrackerConfigFromTuning
)
