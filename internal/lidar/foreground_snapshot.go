package lidar

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/security"
)

// ProjectedPoint is a lightweight 3D sensor-frame projection used for debug charts.
type ProjectedPoint struct {
	X         float64
	Y         float64
	Z         float64
	Intensity uint8
}

// ForegroundSnapshot stores the latest foreground/background projections for a sensor.
// Points are kept in sensor frame (X=right, Y=forward) to match the background polar chart.
type ForegroundSnapshot struct {
	SensorID         string
	Timestamp        time.Time
	ForegroundPoints []ProjectedPoint
	BackgroundPoints []ProjectedPoint
	TotalPoints      int
	ForegroundCount  int
	BackgroundCount  int
}

var (
	fgMu              sync.RWMutex
	latestForegrounds = make(map[string]*ForegroundSnapshot)
)

// StoreForegroundSnapshot saves the latest foreground/background projections for a sensor.
// Points are projected in sensor frame (using az/el) to align with background polar charts.
func StoreForegroundSnapshot(sensorID string, ts time.Time, foreground []PointPolar, background []PointPolar, totalPoints int, foregroundPoints int) {
	if sensorID == "" {
		return
	}

	fgProj := projectPolars(foreground)
	bgProj := projectPolars(background)

	fgMu.Lock()
	latestForegrounds[sensorID] = &ForegroundSnapshot{
		SensorID:         sensorID,
		Timestamp:        ts,
		ForegroundPoints: fgProj,
		BackgroundPoints: bgProj,
		TotalPoints:      totalPoints,
		ForegroundCount:  foregroundPoints,
		BackgroundCount:  totalPoints - foregroundPoints,
	}
	fgMu.Unlock()
}

// GetForegroundSnapshot returns a copy of the latest foreground snapshot for a sensor.
func GetForegroundSnapshot(sensorID string) *ForegroundSnapshot {
	fgMu.RLock()
	snap, ok := latestForegrounds[sensorID]
	fgMu.RUnlock()
	if !ok || snap == nil {
		return nil
	}

	fgCopy := make([]ProjectedPoint, len(snap.ForegroundPoints))
	copy(fgCopy, snap.ForegroundPoints)

	bgCopy := make([]ProjectedPoint, len(snap.BackgroundPoints))
	copy(bgCopy, snap.BackgroundPoints)

	return &ForegroundSnapshot{
		SensorID:         snap.SensorID,
		Timestamp:        snap.Timestamp,
		ForegroundPoints: fgCopy,
		BackgroundPoints: bgCopy,
		TotalPoints:      snap.TotalPoints,
		ForegroundCount:  snap.ForegroundCount,
		BackgroundCount:  snap.BackgroundCount,
	}
}

// projectPolars converts polar points into sensor-frame XYZ projections for charting.
// Corrects for 90-degree rotation (0° = +Y/North) and includes elevation for 3D.
func projectPolars(points []PointPolar) []ProjectedPoint {
	if len(points) == 0 {
		return nil
	}

	out := make([]ProjectedPoint, len(points))
	for i, p := range points {
		az := math.Mod(p.Azimuth, 360.0)
		if az < 0 {
			az += 360.0
		}

		// Convert to radians
		theta := az * math.Pi / 180.0
		phi := p.Elevation * math.Pi / 180.0

		// Calculate 3D coordinates
		// Standard Lidar/Navigation frame:
		// +Y = North (0° azimuth)
		// +X = East (90° azimuth)
		// +Z = Up

		// Horizontal distance
		rXY := p.Distance * math.Cos(phi)

		// X = rXY * sin(theta)
		// Y = rXY * cos(theta)
		// Z = dist * sin(phi)
		x := rXY * math.Sin(theta)
		y := rXY * math.Cos(theta)
		z := p.Distance * math.Sin(phi)

		out[i] = ProjectedPoint{X: x, Y: y, Z: z, Intensity: p.Intensity}
	}
	return out
}

// ExportForegroundSnapshotToASC writes only the foreground points to an ASC file.
// This is intended for quick inspection of live/replayed foreground extraction.
func ExportForegroundSnapshotToASC(snap *ForegroundSnapshot, outPath string) error {
	if snap == nil {
		return fmt.Errorf("nil foreground snapshot")
	}

	if err := security.ValidateExportPath(outPath); err != nil {
		return err
	}

	points := make([]PointASC, 0, len(snap.ForegroundPoints))
	for _, p := range snap.ForegroundPoints {
		points = append(points, PointASC{X: p.X, Y: p.Y, Z: p.Z, Intensity: int(p.Intensity)})
	}
	return ExportPointsToASC(points, outPath, "")
}
