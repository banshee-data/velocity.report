package l3grid

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
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
// Raw polar points are stored and projected lazily on first access.
type ForegroundSnapshot struct {
	SensorID         string
	Timestamp        time.Time
	ForegroundPoints []ProjectedPoint
	BackgroundPoints []ProjectedPoint
	TotalPoints      int
	ForegroundCount  int
	BackgroundCount  int

	// Lazy projection: raw polar stored, projected on first Get
	rawForeground []PointPolar
	rawBackground []PointPolar
	projected     bool
}

var (
	fgMu              sync.RWMutex
	latestForegrounds = make(map[string]*ForegroundSnapshot)
)

// StoreForegroundSnapshot saves the latest foreground/background polar points for a sensor.
// Projection to Cartesian is deferred until GetForegroundSnapshot is called,
// avoiding expensive trig when no debug UI is active.
func StoreForegroundSnapshot(sensorID string, ts time.Time, foreground []PointPolar, background []PointPolar, totalPoints int, foregroundPoints int) {
	if sensorID == "" {
		return
	}

	// Copy polar slices since the caller may reuse the backing arrays
	fgCopy := make([]PointPolar, len(foreground))
	copy(fgCopy, foreground)
	bgCopy := make([]PointPolar, len(background))
	copy(bgCopy, background)

	fgMu.Lock()
	latestForegrounds[sensorID] = &ForegroundSnapshot{
		SensorID:        sensorID,
		Timestamp:       ts,
		TotalPoints:     totalPoints,
		ForegroundCount: foregroundPoints,
		BackgroundCount: totalPoints - foregroundPoints,
		rawForeground:   fgCopy,
		rawBackground:   bgCopy,
	}
	fgMu.Unlock()
}

// GetForegroundSnapshot returns a copy of the latest foreground snapshot for a sensor.
// Performs lazy projection from polar to Cartesian on first access.
func GetForegroundSnapshot(sensorID string) *ForegroundSnapshot {
	fgMu.Lock()
	snap, ok := latestForegrounds[sensorID]
	if !ok || snap == nil {
		fgMu.Unlock()
		return nil
	}

	// Lazy project on first access
	if !snap.projected {
		snap.ForegroundPoints = projectPolars(snap.rawForeground)
		snap.BackgroundPoints = projectPolars(snap.rawBackground)
		snap.rawForeground = nil // Free raw data
		snap.rawBackground = nil
		snap.projected = true
	}
	fgMu.Unlock()

	// Return a copy (under read lock is fine now since projected data is stable)
	fgMu.RLock()
	defer fgMu.RUnlock()

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
// Returns the path where the file was written.
func ExportForegroundSnapshotToASC(snap *ForegroundSnapshot) (string, error) {
	if snap == nil {
		return "", fmt.Errorf("nil foreground snapshot")
	}

	points := make([]PointASC, 0, len(snap.ForegroundPoints))
	for _, p := range snap.ForegroundPoints {
		points = append(points, PointASC{X: p.X, Y: p.Y, Z: p.Z, Intensity: int(p.Intensity)})
	}
	return l2frames.ExportPointsToASC(points, "")
}
