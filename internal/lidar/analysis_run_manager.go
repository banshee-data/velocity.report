package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// AnalysisRunManager coordinates the recording of analysis runs.
// This is now an alias for the implementation in storage/sqlite.
type AnalysisRunManager = sqlite.AnalysisRunManager

// NewAnalysisRunManager creates a new AnalysisRunManager.
var NewAnalysisRunManager = sqlite.NewAnalysisRunManager

// GetAnalysisRunManager returns the singleton AnalysisRunManager for a sensor.
var GetAnalysisRunManager = sqlite.GetAnalysisRunManager

// RegisterAnalysisRunManager registers a new AnalysisRunManager for a sensor.
var RegisterAnalysisRunManager = sqlite.RegisterAnalysisRunManager
