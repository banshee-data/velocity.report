package lidar

import (
	"sync"
	"time"
)

// ForegroundSnapshot stores the latest foreground points and basic metrics for a sensor.
type ForegroundSnapshot struct {
	SensorID         string
	Timestamp        time.Time
	ForegroundPoints []WorldPoint
	BackgroundPoints []WorldPoint
	TotalPoints      int
	ForegroundCount  int
	BackgroundCount  int
}

var (
	fgMu              sync.RWMutex
	latestForegrounds = make(map[string]*ForegroundSnapshot)
)

// StoreForegroundSnapshot saves the latest foreground points for a sensor.
// Points are copied to avoid data races with downstream consumers.
func StoreForegroundSnapshot(sensorID string, ts time.Time, foreground []WorldPoint, background []WorldPoint, totalPoints int, foregroundPoints int) {
	if sensorID == "" {
		return
	}

	fgCopy := make([]WorldPoint, len(foreground))
	copy(fgCopy, foreground)

	bgCopy := make([]WorldPoint, len(background))
	copy(bgCopy, background)

	fgMu.Lock()
	latestForegrounds[sensorID] = &ForegroundSnapshot{
		SensorID:         sensorID,
		Timestamp:        ts,
		ForegroundPoints: fgCopy,
		BackgroundPoints: bgCopy,
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

	fgCopy := make([]WorldPoint, len(snap.ForegroundPoints))
	copy(fgCopy, snap.ForegroundPoints)

	bgCopy := make([]WorldPoint, len(snap.BackgroundPoints))
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
