package l3grid

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases and function re-exports for foreground extraction.

// FrameMetrics captures per-frame extraction statistics.
type FrameMetrics = lidar.FrameMetrics

// Function re-exports.

// ExtractForegroundPoints filters a polar point array using a boolean mask,
// returning only the foreground (non-background) points.
var ExtractForegroundPoints = lidar.ExtractForegroundPoints

// ComputeFrameMetrics calculates extraction metrics from a foreground mask.
var ComputeFrameMetrics = lidar.ComputeFrameMetrics
