package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// SweepRecord represents a single LiDAR sweep record.
// This is now an alias for the implementation in storage/sqlite.
type SweepRecord = sqlite.SweepRecord

// SweepSummary contains aggregate statistics for a sweep.
// This is now an alias for the implementation in storage/sqlite.
type SweepSummary = sqlite.SweepSummary

// SweepStore manages persistence of LiDAR sweep data.
// This is now an alias for the implementation in storage/sqlite.
type SweepStore = sqlite.SweepStore

// NewSweepStore creates a SweepStore backed by the given database.
// This is now an alias for the implementation in storage/sqlite.
var NewSweepStore = sqlite.NewSweepStore
