package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
)

// Backward-compatible type aliases — canonical implementation is in adapters/.

// GroundTruthWeights holds the weights for computing composite ground truth scores.
type GroundTruthWeights = adapters.GroundTruthWeights

// DefaultGroundTruthWeights returns the default weights from the design doc.
var DefaultGroundTruthWeights = adapters.DefaultGroundTruthWeights

// GroundTruthScore holds the evaluation results for a candidate run against reference ground truth.
type GroundTruthScore = adapters.GroundTruthScore

// TrackMatchResult represents a single matched track pair with matching metrics.
type TrackMatchResult = adapters.TrackMatchResult

// EvaluateGroundTruth compares candidate tracks against reference ground truth tracks.
var EvaluateGroundTruth = adapters.EvaluateGroundTruth

// GroundTruthEvaluator provides ground truth evaluation services.
type GroundTruthEvaluator = adapters.GroundTruthEvaluator

// NewGroundTruthEvaluator creates a new evaluator with the specified weights.
var NewGroundTruthEvaluator = adapters.NewGroundTruthEvaluator

// Unexported wrappers for backward-compatible test code.
var computeTemporalIoU = adapters.ComputeTemporalIoU
var matchTracks = adapters.MatchTracks
