package lidar

import "github.com/banshee-data/velocity.report/internal/lidar/l2frames"

// Type aliases re-export frame assembly types from l2frames.
// This file provides backward compatibility while the implementation
// has been moved to internal/lidar/l2frames.

// LiDARFrame is a complete rotation frame assembled from raw points.
type LiDARFrame = l2frames.LiDARFrame

// FrameBuilder assembles incoming points into complete rotation frames.
type FrameBuilder = l2frames.FrameBuilder

// FrameBuilderConfig holds configuration for the FrameBuilder.
type FrameBuilderConfig = l2frames.FrameBuilderConfig

// Constructor and registry function re-exports.

// NewFrameBuilder creates a FrameBuilder with the given configuration.
var NewFrameBuilder = l2frames.NewFrameBuilder

// RegisterFrameBuilder registers a FrameBuilder in the global registry.
var RegisterFrameBuilder = l2frames.RegisterFrameBuilder

// GetFrameBuilder retrieves a registered FrameBuilder by sensor ID.
var GetFrameBuilder = l2frames.GetFrameBuilder
