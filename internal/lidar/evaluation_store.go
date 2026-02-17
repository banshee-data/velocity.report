package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Evaluation represents a persisted ground truth evaluation result.
// This is now an alias for the implementation in storage/sqlite.
type Evaluation = sqlite.Evaluation

// EvaluationStore manages persistence for ground truth evaluations.
// This is now an alias for the implementation in storage/sqlite.
type EvaluationStore = sqlite.EvaluationStore

// NewEvaluationStore creates an EvaluationStore backed by the given database.
// This is now an alias for the implementation in storage/sqlite.
var NewEvaluationStore = sqlite.NewEvaluationStore
