package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Backward-compatible type aliases — canonical implementation is in pipeline/.

// ForegroundForwarder interface allows forwarding foreground points without importing network package.
type ForegroundForwarder = pipeline.ForegroundForwarder

// VisualiserPublisher interface allows publishing frames to the gRPC visualiser.
type VisualiserPublisher = pipeline.VisualiserPublisher

// VisualiserAdapter interface converts tracking outputs to FrameBundle.
type VisualiserAdapter = pipeline.VisualiserAdapter

// LidarViewAdapter interface forwards FrameBundle to UDP (LidarView format).
type LidarViewAdapter = pipeline.LidarViewAdapter

// ForegroundStage extracts foreground (moving) points from a frame using
// the learned background model (L3 Grid).
type ForegroundStage = pipeline.ForegroundStage

// PerceptionStage transforms foreground points into world coordinates,
// applies ground removal, and clusters them (L4 Perception).
type PerceptionStage = pipeline.PerceptionStage

// TrackingStage updates the tracker state with new cluster observations
// and returns confirmed tracks (L5 Tracks).
type TrackingStage = pipeline.TrackingStage

// ObjectStage classifies confirmed tracks and attaches semantic labels (L6 Objects).
type ObjectStage = pipeline.ObjectStage

// PersistenceSink writes pipeline outputs (tracks, observations) to storage.
type PersistenceSink = pipeline.PersistenceSink

// PublishSink sends pipeline outputs to external consumers (visualiser, gRPC).
type PublishSink = pipeline.PublishSink

// TrackingPipelineConfig holds dependencies for the tracking pipeline callback.
type TrackingPipelineConfig = pipeline.TrackingPipelineConfig

// IsNilInterface is re-exported for tests that need to check interface nil values.
var IsNilInterface = pipeline.IsNilInterface

// isNilInterface is the unexported wrapper for backward-compatible test code.
var isNilInterface = pipeline.IsNilInterface

// Ensure type compatibility: pipeline uses types from layer packages.
// These compile-time assertions verify the aliases remain compatible.
var (
	_ *LiDARFrame    = (*l2frames.LiDARFrame)(nil)
	_ *PointPolar    = (*l4perception.PointPolar)(nil)
	_ *TrackedObject = (*l5tracks.TrackedObject)(nil)
	_ *WorldCluster  = (*l4perception.WorldCluster)(nil)
	_ *TrackObservation = (*sqlite.TrackObservation)(nil)
)
