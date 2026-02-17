package l6objects

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export quality assessment types from the parent package.

// RunStatistics summarises an analysis run's track population.
type RunStatistics = lidar.RunStatistics

// TrackQualityMetrics captures quality measures for a single track.
type TrackQualityMetrics = lidar.TrackQualityMetrics

// NoiseCoverageMetrics captures noise and coverage statistics.
type NoiseCoverageMetrics = lidar.NoiseCoverageMetrics

// TrackTrainingFilter configures which tracks to include in training data.
type TrackTrainingFilter = lidar.TrackTrainingFilter

// TrainingDatasetSummary summarises a filtered training dataset.
type TrainingDatasetSummary = lidar.TrainingDatasetSummary

// Function re-exports.

// ComputeRunStatistics calculates aggregate run statistics from tracks.
var ComputeRunStatistics = lidar.ComputeRunStatistics

// ComputeTrackQualityMetrics calculates quality metrics for a single track.
var ComputeTrackQualityMetrics = lidar.ComputeTrackQualityMetrics

// ComputeNoiseCoverageMetrics calculates noise and coverage metrics.
var ComputeNoiseCoverageMetrics = lidar.ComputeNoiseCoverageMetrics

// DefaultTrackTrainingFilter returns production-default training filter settings.
var DefaultTrackTrainingFilter = lidar.DefaultTrackTrainingFilter

// FilterTracksForTraining applies filter criteria to select training-quality tracks.
var FilterTracksForTraining = lidar.FilterTracksForTraining

// SummarizeTrainingDataset produces a summary of the filtered training set.
var SummarizeTrainingDataset = lidar.SummarizeTrainingDataset
