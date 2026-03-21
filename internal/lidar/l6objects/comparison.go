package l6objects

import "github.com/banshee-data/velocity.report/internal/lidar/l8analytics"

// ComputeTemporalIoU delegates to the canonical implementation in l8analytics.
var ComputeTemporalIoU = l8analytics.ComputeTemporalIoU

// Run comparison types — canonical definitions are in l8analytics.
// These aliases preserve backward compatibility for existing callers.

// RunComparison shows differences between two analysis runs.
type RunComparison = l8analytics.RunComparison

// TrackSplit represents a suspected track split between runs.
type TrackSplit = l8analytics.TrackSplit

// TrackMerge represents a suspected track merge between runs.
type TrackMerge = l8analytics.TrackMerge

// TrackMatch represents a matched track between two runs.
type TrackMatch = l8analytics.TrackMatch
