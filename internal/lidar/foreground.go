package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Type aliases and function re-exports for foreground extraction.

// FrameMetrics captures per-frame extraction statistics.
type FrameMetrics = l3grid.FrameMetrics

// Function re-exports.

// ExtractForegroundPoints filters a polar point array using a boolean mask,
// returning only the foreground (non-background) points.
var ExtractForegroundPoints = l3grid.ExtractForegroundPoints

// ComputeFrameMetrics calculates extraction metrics from a foreground mask.
var ComputeFrameMetrics = l3grid.ComputeFrameMetrics
