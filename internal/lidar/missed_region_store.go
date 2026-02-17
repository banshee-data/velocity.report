package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// MissedRegion represents a spatial-temporal region of missed detections.
// This is now an alias for the implementation in storage/sqlite.
type MissedRegion = sqlite.MissedRegion

// MissedRegionStore manages persistence of missed detection regions.
// This is now an alias for the implementation in storage/sqlite.
type MissedRegionStore = sqlite.MissedRegionStore

// NewMissedRegionStore creates a MissedRegionStore backed by the given database.
// This is now an alias for the implementation in storage/sqlite.
var NewMissedRegionStore = sqlite.NewMissedRegionStore
