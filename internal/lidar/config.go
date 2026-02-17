package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Type aliases and function re-exports for background configuration.

// BackgroundConfig provides a configuration builder for BackgroundParams.
type BackgroundConfig = l3grid.BackgroundConfig

// Function re-exports.

// DefaultBackgroundConfig returns a BackgroundConfig loaded from the
// canonical tuning defaults file (config/tuning.defaults.json).
var DefaultBackgroundConfig = l3grid.DefaultBackgroundConfig

// BackgroundConfigFromTuning creates a BackgroundConfig from a TuningConfig.
var BackgroundConfigFromTuning = l3grid.BackgroundConfigFromTuning
