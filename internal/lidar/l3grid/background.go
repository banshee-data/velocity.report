package l3grid

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export background model types from the parent package.
// These aliases enable gradual migration: callers can import from l3grid
// while the implementation remains in internal/lidar until fully migrated.

// BackgroundParams configures the background model learning algorithm.
type BackgroundParams = lidar.BackgroundParams

// BackgroundGrid is the spatial grid used for background modelling.
type BackgroundGrid = lidar.BackgroundGrid

// BackgroundCell represents a single cell in the background grid.
type BackgroundCell = lidar.BackgroundCell

// BackgroundManager orchestrates background model updates for a sensor.
type BackgroundManager = lidar.BackgroundManager

// Region represents a detected spatial region in the background grid.
type Region = lidar.Region

// RegionParams holds tuning parameters for a specific region.
type RegionParams = lidar.RegionParams

// RegionManager manages region identification and per-region parameterisation.
type RegionManager = lidar.RegionManager

// Constructor re-exports.

// NewRegionManager creates a RegionManager for the given grid dimensions.
var NewRegionManager = lidar.NewRegionManager
