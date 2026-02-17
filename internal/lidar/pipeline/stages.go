package pipeline

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export pipeline orchestration types from the parent
// package. These aliases enable callers to import from pipeline/ while
// the implementation remains in internal/lidar/tracking_pipeline.go.

// TrackingPipelineConfig configures the real-time tracking pipeline.
type TrackingPipelineConfig = lidar.TrackingPipelineConfig

// Stage interfaces — these define the contracts between pipeline stages
// and are the primary API for testing and composition.

// ForegroundStage (L3) extracts foreground points from a polar frame.
type ForegroundStage = lidar.ForegroundStage

// PerceptionStage (L4) transforms, filters, and clusters foreground points.
type PerceptionStage = lidar.PerceptionStage

// TrackingStage (L5) updates multi-object tracks from clusters.
type TrackingStage = lidar.TrackingStage

// ObjectStage (L6) classifies tracked objects and computes quality metrics.
type ObjectStage = lidar.ObjectStage

// PersistenceSink persists tracking results to storage.
type PersistenceSink = lidar.PersistenceSink

// PublishSink broadcasts tracking results to subscribers.
type PublishSink = lidar.PublishSink
