package sqlite

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export SQLite storage types from the parent package.
// These aliases enable gradual migration: callers can import from
// storage/sqlite while the implementation remains in internal/lidar.
//
// Domain types (Scene, Evaluation, AnalysisRun, RunTrack) are also
// aliased here for convenience; they belong conceptually to their
// respective layers but are closely tied to their store counterparts.

// Store types.

// SceneStore manages persistence for LiDAR evaluation scenes.
type SceneStore = lidar.SceneStore

// EvaluationStore manages persistence for ground truth evaluations.
type EvaluationStore = lidar.EvaluationStore

// AnalysisRunStore manages persistence for analysis runs and run tracks.
type AnalysisRunStore = lidar.AnalysisRunStore

// TrackStore defines the interface for track persistence operations.
type TrackStore = lidar.TrackStore

// Domain types persisted by these stores.

// Scene represents a LiDAR evaluation scene.
type Scene = lidar.Scene

// Evaluation represents a persisted ground truth evaluation result.
type Evaluation = lidar.Evaluation

// AnalysisRun represents a single analysis run over a PCAP or live session.
type AnalysisRun = lidar.AnalysisRun

// RunTrack represents a tracked object within an analysis run.
type RunTrack = lidar.RunTrack

// Constructor re-exports.

// NewSceneStore creates a SceneStore backed by the given database.
var NewSceneStore = lidar.NewSceneStore

// NewEvaluationStore creates an EvaluationStore backed by the given database.
var NewEvaluationStore = lidar.NewEvaluationStore

// NewAnalysisRunStore creates an AnalysisRunStore backed by the given database.
var NewAnalysisRunStore = lidar.NewAnalysisRunStore
