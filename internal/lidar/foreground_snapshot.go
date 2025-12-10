package lidar

import (
	"sync"
	"time"
)

// ForegroundSnapshot stores the latest foreground points and basic metrics for a sensor.
type ForegroundSnapshot struct {
	SensorID         string
	Timestamp        time.Time
	Points           []WorldPoint
	TotalPoints      int
	ForegroundPoints int
	BackgroundPoints int
}

var (
	fgMu              sync.RWMutex
	latestForegrounds = make(map[string]*ForegroundSnapshot)
)

// StoreForegroundSnapshot saves the latest foreground points for a sensor.
// Points are copied to avoid data races with downstream consumers.
func StoreForegroundSnapshot(sensorID string, ts time.Time, points []WorldPoint, totalPoints int, foregroundPoints int) {
	if sensorID == "" {
		return
	}

	copyPoints := make([]WorldPoint, len(points))
	copy(copyPoints, points)

	fgMu.Lock()
	latestForegrounds[sensorID] = &ForegroundSnapshot{
		SensorID:         sensorID,
		Timestamp:        ts,
		Points:           copyPoints,
		TotalPoints:      totalPoints,
		ForegroundPoints: foregroundPoints,
		BackgroundPoints: totalPoints - foregroundPoints,
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

	pointsCopy := make([]WorldPoint, len(snap.Points))
	copy(pointsCopy, snap.Points)

	return &ForegroundSnapshot{
		SensorID:         snap.SensorID,
		Timestamp:        snap.Timestamp,
		Points:           pointsCopy,
		TotalPoints:      snap.TotalPoints,
		ForegroundPoints: snap.ForegroundPoints,
		BackgroundPoints: snap.BackgroundPoints,
	}
}
