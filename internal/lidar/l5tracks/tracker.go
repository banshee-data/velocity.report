package l5tracks

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export tracking types from the parent package.
// These aliases enable gradual migration: callers can import from
// l5tracks while the implementation remains in internal/lidar.

// TrackState represents the lifecycle state of a tracked object.
type TrackState = lidar.TrackState

// TrackerConfig holds configuration for the multi-object tracker.
type TrackerConfig = lidar.TrackerConfig

// TrackPoint is a 2D position used within the Kalman filter.
type TrackPoint = lidar.TrackPoint

// TrackedObject represents a single object being tracked over time.
type TrackedObject = lidar.TrackedObject

// TrackingMetrics captures aggregate tracking statistics per frame.
type TrackingMetrics = lidar.TrackingMetrics

// TrackAlignmentMetrics captures track-to-cluster alignment quality.
type TrackAlignmentMetrics = lidar.TrackAlignmentMetrics

// Tracker manages the lifecycle of all tracked objects.
type Tracker = lidar.Tracker

// DebugCollector is the interface for collecting per-frame debug data.
type DebugCollector = lidar.DebugCollector

// Constructor and function re-exports.

// NewTracker creates a Tracker with the given configuration.
var NewTracker = lidar.NewTracker

// DefaultTrackerConfig returns production-default tracker parameters.
var DefaultTrackerConfig = lidar.DefaultTrackerConfig

// TrackerConfigFromTuning derives tracker config from a TuningConfig.
var TrackerConfigFromTuning = lidar.TrackerConfigFromTuning
