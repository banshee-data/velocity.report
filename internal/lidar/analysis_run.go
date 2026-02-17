package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// AnalysisRun represents a complete analysis session with parameters.
// This is now an alias for the implementation in storage/sqlite.
type AnalysisRun = sqlite.AnalysisRun

// RunParams captures all configurable parameters for reproducibility.
// This is now an alias for the implementation in storage/sqlite.
type RunParams = sqlite.RunParams

// BackgroundParamsExport is the JSON-serializable background params.
// This is now an alias for the implementation in storage/sqlite.
type BackgroundParamsExport = sqlite.BackgroundParamsExport

// ClusteringParamsExport is the JSON-serializable clustering params.
// This is now an alias for the implementation in storage/sqlite.
type ClusteringParamsExport = sqlite.ClusteringParamsExport

// TrackingParamsExport is the JSON-serializable tracking params.
// This is now an alias for the implementation in storage/sqlite.
type TrackingParamsExport = sqlite.TrackingParamsExport

// ClassificationParamsExport is the JSON-serializable classification params.
// This is now an alias for the implementation in storage/sqlite.
type ClassificationParamsExport = sqlite.ClassificationParamsExport

// RunTrack represents a tracked object within an analysis run.
// This is now an alias for the implementation in storage/sqlite.
type RunTrack = sqlite.RunTrack

// RunComparison holds the results of comparing two analysis runs.
// This is now an alias for the implementation in storage/sqlite.
type RunComparison = sqlite.RunComparison

// TrackMatch represents a matched track between two runs.
// This is now an alias for the implementation in storage/sqlite.
type TrackMatch = sqlite.TrackMatch

// TrackSplit represents a track split detected between runs.
// This is now an alias for the implementation in storage/sqlite.
type TrackSplit = sqlite.TrackSplit

// TrackMerge represents a track merge detected between runs.
// This is now an alias for the implementation in storage/sqlite.
type TrackMerge = sqlite.TrackMerge

// AnalysisRunStore manages persistence for analysis runs and run tracks.
// This is now an alias for the implementation in storage/sqlite.
type AnalysisRunStore = sqlite.AnalysisRunStore

// Function aliases.

// DefaultRunParams returns run parameters loaded from the canonical tuning defaults file.
var DefaultRunParams = sqlite.DefaultRunParams

// RunParamsFromTuning builds RunParams from a loaded TuningConfig.
var RunParamsFromTuning = sqlite.RunParamsFromTuning

// FromBackgroundParams creates export params from BackgroundParams.
var FromBackgroundParams = sqlite.FromBackgroundParams

// FromDBSCANParams creates export params from DBSCANParams.
var FromDBSCANParams = sqlite.FromDBSCANParams

// FromTrackerConfig creates export params from TrackerConfig.
var FromTrackerConfig = sqlite.FromTrackerConfig

// RunTrackFromTrackedObject creates a RunTrack from a TrackedObject.
var RunTrackFromTrackedObject = sqlite.RunTrackFromTrackedObject

// NewAnalysisRunStore creates an AnalysisRunStore backed by the given database.
var NewAnalysisRunStore = sqlite.NewAnalysisRunStore
