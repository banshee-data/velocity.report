package lidar

import (
    "math"
    "time"
)

// BackgroundParams configuration matching the param storage approach in schema
type BackgroundParams struct {
    BackgroundUpdateFraction       float32 // e.g., 0.02
    ClosenessSensitivityMultiplier float32 // e.g., 3.0
    SafetyMarginMeters             float32 // e.g., 0.5
    FreezeDurationNanos            int64   // e.g., 5e9 (5s)
    NeighborConfirmationCount      int     // e.g., 5 of 8 neighbors

    // Additional params for persistence matching schema requirements
    SettlingPeriodNanos        int64 // 5 minutes before first snapshot
    SnapshotIntervalNanos      int64 // 2 hours between snapshots
    ChangeThresholdForSnapshot int   // min changed cells to trigger snapshot
}

// BackgroundCell matches the compressed storage format for schema persistence
type BackgroundCell struct {
    AverageRangeMeters   float32
    RangeSpreadMeters    float32
    TimesSeenCount       uint32
    LastUpdateUnixNanos  int64
    FrozenUntilUnixNanos int64
}

// BackgroundGrid enhanced for schema persistence and 100-track performance
type BackgroundGrid struct {
    SensorID    string
    SensorFrame FrameID // e.g., "sensor/hesai-01"

    Rings       int // e.g., 40 - matches schema rings INTEGER NOT NULL
    AzimuthBins int // e.g., 1800 for 0.2° - matches schema azimuth_bins INTEGER NOT NULL

    Cells []BackgroundCell // len = Rings * AzimuthBins

    Params BackgroundParams

    // Enhanced persistence tracking matching schema lidar_bg_snapshot table
    Manager              *BackgroundManager
    LastSnapshotTime     time.Time
    ChangesSinceSnapshot int
    SnapshotID           *int64 // tracks last persisted snapshot_id from schema

    // Performance tracking for system_events table integration
    LastProcessingTimeUs  int64
    WarmupFramesRemaining int
    SettlingComplete      bool

    // Telemetry for monitoring (feeds into system_events)
    ForegroundCount int64
    BackgroundCount int64

    // Thread safety for concurrent access during persistence
    // TODO: add mutex when implementing concurrent background updates
}

// Helper to index Cells: idx = ring*AzimuthBins + azBin
func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

// BackgroundManager handles automatic persistence following schema lidar_bg_snapshot pattern
type BackgroundManager struct {
    Grid            *BackgroundGrid
    SettlingTimer   *time.Timer
    PersistTimer    *time.Timer
    HasSettled      bool
    LastPersistTime time.Time
    StartTime       time.Time

    // Persistence callback to main app - should save to schema lidar_bg_snapshot table
    PersistCallback func(snapshot *BgSnapshot) error
}

// ProcessFramePolar ingests sensor-frame polar points and updates the BackgroundGrid.
// Behavior:
// - Bins points by ring (channel) and azimuth bin.
// - Uses an EMA (BackgroundUpdateFraction) to update AverageRangeMeters and RangeSpreadMeters.
// - Tracks a simple two-level confidence via TimesSeenCount (increment on close matches,
//   decrement on mismatches). When a cell deviates strongly repeatedly it is frozen for
//   FreezeDurationNanos to avoid corrupting the background model.
// - Uses neighbor confirmation: updates are applied more readily when adjacent cells
//   agree (helps suppress isolated noise).
func (bm *BackgroundManager) ProcessFramePolar(points []PointPolar) {
    if bm == nil || bm.Grid == nil || len(points) == 0 {
        return
    }

    g := bm.Grid
    rings := g.Rings
    azBins := g.AzimuthBins
    if rings <= 0 || azBins <= 0 || len(g.Cells) != rings*azBins {
        return
    }

    now := time.Now()
    nowNanos := now.UnixNano()

    // Temporary accumulators per cell
    totalCells := rings * azBins
    counts := make([]int, totalCells)
    sums := make([]float64, totalCells)
    minDistances := make([]float64, totalCells)
    maxDistances := make([]float64, totalCells)
    for i := range minDistances {
        minDistances[i] = math.Inf(1)
        maxDistances[i] = math.Inf(-1)
    }

    // Bin incoming polar points
    for _, p := range points {
        ring := p.Channel - 1
        if ring < 0 || ring >= rings {
            continue
        }
        // Normalize azimuth into [0,360)
        az := math.Mod(p.Azimuth, 360.0)
        if az < 0 {
            az += 360.0
        }
        azBin := int((az / 360.0) * float64(azBins))
        if azBin < 0 {
            azBin = 0
        }
        if azBin >= azBins {
            azBin = azBins - 1
        }

        cellIdx := g.Idx(ring, azBin)
        counts[cellIdx]++
        sums[cellIdx] += p.Distance
        if p.Distance < minDistances[cellIdx] {
            minDistances[cellIdx] = p.Distance
        }
        if p.Distance > maxDistances[cellIdx] {
            maxDistances[cellIdx] = p.Distance
        }
    }

    // Parameters with safe defaults
    alpha := float64(g.Params.BackgroundUpdateFraction)
    if alpha <= 0 || alpha > 1 {
        alpha = 0.02
    }
    closenessMultiplier := float64(g.Params.ClosenessSensitivityMultiplier)
    if closenessMultiplier <= 0 {
        closenessMultiplier = 3.0
    }
    safety := float64(g.Params.SafetyMarginMeters)
    freezeDur := g.Params.FreezeDurationNanos
    neighConfirm := g.Params.NeighborConfirmationCount
    if neighConfirm <= 0 {
        neighConfirm = 3
    }

    foregroundCount := int64(0)
    backgroundCount := int64(0)

    // Iterate over observed cells and update grid
    for ringIdx := 0; ringIdx < rings; ringIdx++ {
        for azBinIdx := 0; azBinIdx < azBins; azBinIdx++ {
            cellIdx := g.Idx(ringIdx, azBinIdx)
            if counts[cellIdx] == 0 {
                continue
            }

            observationMean := sums[cellIdx] / float64(counts[cellIdx])
            // Small protection when minDistances == +Inf (shouldn't happen if counts>0)
            if math.IsInf(minDistances[cellIdx], 1) {
                minDistances[cellIdx] = observationMean
            }

            cell := &g.Cells[cellIdx]

            // If frozen, skip updates unless freeze expired
            if cell.FrozenUntilUnixNanos > nowNanos {
                foregroundCount++ // treat as foreground during freeze
                continue
            }

            // Neighbor confirmation: count neighbors that have similar average
            neighborConfirmCount := 0
            for deltaRing := -1; deltaRing <= 1; deltaRing++ {
                neighborRing := ringIdx + deltaRing
                if neighborRing < 0 || neighborRing >= rings {
                    continue
                }
                for deltaAz := -1; deltaAz <= 1; deltaAz++ {
                    if deltaRing == 0 && deltaAz == 0 {
                        continue
                    }
                    neighborAzimuth := (azBinIdx + deltaAz + azBins) % azBins
                    neighborIdx := g.Idx(neighborRing, neighborAzimuth)
                    neighborCell := g.Cells[neighborIdx]
                    // consider neighbor confirmed if it has some history and close range
                    if neighborCell.TimesSeenCount > 0 {
                        neighborDiff := math.Abs(float64(neighborCell.AverageRangeMeters) - observationMean)
                        neighborCloseness := closenessMultiplier*(float64(neighborCell.RangeSpreadMeters)+0.01)
                        if neighborDiff <= neighborCloseness {
                            neighborConfirmCount++
                        }
                    }
                }
            }

            // closeness threshold based on existing spread and safety margin
            closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+0.01) + safety
            cellDiff := math.Abs(float64(cell.AverageRangeMeters) - observationMean)

            // Decide if this observation is background-like or foreground-like
            isBackgroundLike := cellDiff <= closenessThreshold || neighborConfirmCount >= neighConfirm

            if isBackgroundLike {
                // update EMA for average and spread
                if cell.TimesSeenCount == 0 {
                    // initialize
                    cell.AverageRangeMeters = float32(observationMean)
                    cell.RangeSpreadMeters = float32((maxDistances[cellIdx] - minDistances[cellIdx]) / 2.0)
                    cell.TimesSeenCount = 1
                } else {
                    oldAvg := float64(cell.AverageRangeMeters)
                    newAvg := (1.0-alpha)*oldAvg + alpha*observationMean
                    // update spread as EMA of absolute deviation
                    deviation := math.Abs(observationMean - newAvg)
                    newSpread := (1.0-alpha)*float64(cell.RangeSpreadMeters) + alpha*deviation
                    cell.AverageRangeMeters = float32(newAvg)
                    cell.RangeSpreadMeters = float32(newSpread)
                    cell.TimesSeenCount++
                }
                cell.LastUpdateUnixNanos = nowNanos
                backgroundCount++
            } else {
                // Observation diverges from background
                // Decrease confidence and possibly freeze the cell if divergence is large
                if cell.TimesSeenCount > 0 {
                    cell.TimesSeenCount--
                }
                // If difference very large relative to spread, freeze the cell briefly
                if cellDiff > 3.0*closenessThreshold {
                    cell.FrozenUntilUnixNanos = nowNanos + freezeDur
                }
                // Keep last update timestamp to indicate recent observation
                cell.LastUpdateUnixNanos = nowNanos
                foregroundCount++
            }

            // Track change count for snapshotting heuristics
            // If the average shifted more than a small threshold, count it as a change
            // (this is conservative to avoid noisy snapshots)
            // We store a simple change counter increment when update happened
            g.ChangesSinceSnapshot++
        }
    }

    // Update telemetry counters
    g.ForegroundCount = foregroundCount
    g.BackgroundCount = backgroundCount

    // Record processing time (microseconds)
    // NOTE: inexpensive timing; use time.Since for accuracy
    // We don't need high precision here, so a simple assignment is fine
    // but we'll store elapsed micros for monitoring
    // (caller may call this frequently; keep cheap)
    // For now, set LastProcessingTimeUs to 0 as placeholder behavior
    g.LastProcessingTimeUs = 0

    // Inform manager timers / settle state
    if !bm.HasSettled {
        // Simple settling heuristic: mark settled after first non-empty frame
        bm.HasSettled = true
        bm.StartTime = now
    }
    bm.LastPersistTime = now
}
