package l2frames

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export frame assembly types from the parent package.
// These aliases enable gradual migration: callers can import from
// l2frames while the implementation remains in internal/lidar.

// LiDARFrame is a complete rotation frame assembled from raw points.
type LiDARFrame = lidar.LiDARFrame

// FrameBuilder assembles incoming points into complete rotation frames.
type FrameBuilder = lidar.FrameBuilder

// FrameBuilderConfig holds configuration for the FrameBuilder.
type FrameBuilderConfig = lidar.FrameBuilderConfig

// Constructor re-exports.

// NewFrameBuilder creates a FrameBuilder with the given configuration.
var NewFrameBuilder = lidar.NewFrameBuilder

// RegisterFrameBuilder registers a FrameBuilder in the global registry.
var RegisterFrameBuilder = lidar.RegisterFrameBuilder

// GetFrameBuilder retrieves a registered FrameBuilder by sensor ID.
var GetFrameBuilder = lidar.GetFrameBuilder
