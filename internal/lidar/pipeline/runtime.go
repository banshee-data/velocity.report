package pipeline

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// SensorRuntime bundles per-sensor dependencies that were previously
// accessed via global registries. Passing a SensorRuntime through
// constructors makes wiring explicit and testing deterministic.
type SensorRuntime struct {
	SensorID           string
	FrameBuilder       *l2frames.FrameBuilder
	BackgroundManager  *l3grid.BackgroundManager
	AnalysisRunManager *sqlite.AnalysisRunManager
}
