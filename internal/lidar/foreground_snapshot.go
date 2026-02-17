package lidar

import (
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Type aliases and function re-exports for foreground snapshot.

// ProjectedPoint is a lightweight 3D sensor-frame projection used for debug charts.
type ProjectedPoint = l3grid.ProjectedPoint

// ForegroundSnapshot stores the latest foreground/background projections for a sensor.
type ForegroundSnapshot = l3grid.ForegroundSnapshot

// Function re-exports.

// StoreForegroundSnapshot saves the latest foreground/background polar points for a sensor.
func StoreForegroundSnapshot(sensorID string, ts time.Time, foreground []PointPolar, background []PointPolar, totalPoints int, foregroundPoints int) {
	l3grid.StoreForegroundSnapshot(sensorID, ts, foreground, background, totalPoints, foregroundPoints)
}

// GetForegroundSnapshot retrieves the latest foreground snapshot for a sensor.
var GetForegroundSnapshot = l3grid.GetForegroundSnapshot

// ExportForegroundSnapshotToASC writes only the foreground points to an ASC file.
var ExportForegroundSnapshotToASC = l3grid.ExportForegroundSnapshotToASC
