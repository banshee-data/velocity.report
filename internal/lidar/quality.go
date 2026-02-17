package lidar

import "github.com/banshee-data/velocity.report/internal/lidar/l6objects"

// Type aliases for quality analysis types migrated to l6objects.
type RunStatistics = l6objects.RunStatistics
type TrackQualityMetrics = l6objects.TrackQualityMetrics
type NoiseCoverageMetrics = l6objects.NoiseCoverageMetrics
type TrackTrainingFilter = l6objects.TrackTrainingFilter
type TrainingDatasetSummary = l6objects.TrainingDatasetSummary

// Function aliases re-exported from l6objects.
var (
ComputeRunStatistics       = l6objects.ComputeRunStatistics
ParseRunStatistics         = l6objects.ParseRunStatistics
ComputeTrackQualityMetrics = l6objects.ComputeTrackQualityMetrics
ComputeNoiseCoverageMetrics = l6objects.ComputeNoiseCoverageMetrics
DefaultTrackTrainingFilter = l6objects.DefaultTrackTrainingFilter
FilterTracksForTraining    = l6objects.FilterTracksForTraining
SummarizeTrainingDataset   = l6objects.SummarizeTrainingDataset
)
