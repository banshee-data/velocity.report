package lidar

import (
	"math"
	"time"
)

// ProcessFramePolarWithMask classifies each point as foreground/background in polar coordinates.
// Returns a mask where true indicates foreground (object), false indicates background (static).
// This is Phase 2.9 of the foreground tracking pipeline.
//
// Unlike ProcessFramePolar which aggregates points per cell, this operates per-point
// for finer-grained foreground detection suitable for downstream clustering.
func (bm *BackgroundManager) ProcessFramePolarWithMask(points []PointPolar) (foregroundMask []bool, err error) {
	if bm == nil || bm.Grid == nil {
		return nil, nil
	}
	if len(points) == 0 {
		return []bool{}, nil
	}

	g := bm.Grid
	rings := g.Rings
	azBins := g.AzimuthBins
	if rings <= 0 || azBins <= 0 || len(g.Cells) != rings*azBins {
		return nil, nil
	}

	// Allocate mask for all points
	foregroundMask = make([]bool, len(points))

	now := time.Now()
	nowNanos := now.UnixNano()

	// Pre-read parameters outside lock to minimize lock duration
	alpha := float64(g.Params.BackgroundUpdateFraction)
	if alpha <= 0 || alpha > 1 {
		alpha = 0.02
	}
	safety := float64(g.Params.SafetyMarginMeters)
	freezeDur := g.Params.FreezeDurationNanos

	g.mu.Lock()
	defer g.mu.Unlock()

	// Read runtime-tunable params under lock
	noiseRel := float64(g.Params.NoiseRelativeFraction)
	if noiseRel <= 0 {
		noiseRel = 0.01
	}
	closenessMultiplier := float64(g.Params.ClosenessSensitivityMultiplier)
	if closenessMultiplier <= 0 {
		closenessMultiplier = 3.0
	}
	neighConfirm := g.Params.NeighborConfirmationCount
	if neighConfirm <= 0 {
		neighConfirm = 3
	}
	seedFromFirst := g.Params.SeedFromFirstObservation

	// Process each point individually
	foregroundCount := int64(0)
	backgroundCount := int64(0)

	for i, p := range points {
		ring := p.Channel - 1
		if ring < 0 || ring >= rings {
			// Invalid channel - treat as foreground
			foregroundMask[i] = true
			foregroundCount++
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
		cell := &g.Cells[cellIdx]

		// If frozen, treat as foreground
		if cell.FrozenUntilUnixNanos > nowNanos {
			foregroundMask[i] = true
			foregroundCount++
			continue
		}

		// Same-ring neighbor confirmation
		neighborConfirmCount := 0
		for deltaAz := -1; deltaAz <= 1; deltaAz++ {
			if deltaAz == 0 {
				continue
			}
			neighborAzimuth := (azBin + deltaAz + azBins) % azBins
			neighborIdx := g.Idx(ring, neighborAzimuth)
			neighborCell := g.Cells[neighborIdx]
			if neighborCell.TimesSeenCount > 0 {
				neighborDiff := math.Abs(float64(neighborCell.AverageRangeMeters) - p.Distance)
				neighborCloseness := closenessMultiplier * (float64(neighborCell.RangeSpreadMeters) + noiseRel*float64(neighborCell.AverageRangeMeters) + 0.01)
				if neighborDiff <= neighborCloseness {
					neighborConfirmCount++
				}
			}
		}

		// Closeness threshold: scales with cell spread and distance
		closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+noiseRel*p.Distance+0.01) + safety
		cellDiff := math.Abs(float64(cell.AverageRangeMeters) - p.Distance)

		// Classification decision
		isBackgroundLike := cellDiff <= closenessThreshold || neighborConfirmCount >= neighConfirm

		// Handle empty cells based on seed-from-first setting
		initIfEmpty := false
		if seedFromFirst && cell.TimesSeenCount == 0 {
			initIfEmpty = true
		}

		// Set mask value: true = foreground, false = background
		if isBackgroundLike || initIfEmpty {
			foregroundMask[i] = false
			backgroundCount++

			// Update cell EMA for background points
			if cell.TimesSeenCount == 0 {
				cell.AverageRangeMeters = float32(p.Distance)
				cell.RangeSpreadMeters = 0.0 // Single point, no spread yet
				cell.TimesSeenCount = 1
				g.nonzeroCellCount++
			} else {
				oldAvg := float64(cell.AverageRangeMeters)
				newAvg := (1.0-alpha)*oldAvg + alpha*p.Distance
				deviation := math.Abs(p.Distance - oldAvg)
				newSpread := (1.0-alpha)*float64(cell.RangeSpreadMeters) + alpha*deviation
				cell.AverageRangeMeters = float32(newAvg)
				cell.RangeSpreadMeters = float32(newSpread)
				cell.TimesSeenCount++
			}
			cell.LastUpdateUnixNanos = nowNanos
		} else {
			foregroundMask[i] = true
			foregroundCount++

			// Decrease confidence for divergent observations
			if cell.TimesSeenCount > 0 {
				cell.TimesSeenCount--
				if cell.TimesSeenCount == 0 && g.nonzeroCellCount > 0 {
					g.nonzeroCellCount--
				}
			}
			// Freeze cell if divergence is very large
			if cellDiff > 3.0*closenessThreshold {
				cell.FrozenUntilUnixNanos = nowNanos + freezeDur
			}
			cell.LastUpdateUnixNanos = nowNanos
		}

		g.ChangesSinceSnapshot++
	}

	// Update telemetry counters
	g.ForegroundCount = foregroundCount
	g.BackgroundCount = backgroundCount

	return foregroundMask, nil
}

// ExtractForegroundPoints returns only the foreground points from the input slice
// based on the provided mask. Points where mask[i] == true are included.
func ExtractForegroundPoints(points []PointPolar, mask []bool) []PointPolar {
	if len(points) == 0 || len(mask) == 0 {
		return nil
	}
	if len(points) != len(mask) {
		return nil
	}

	// Pre-count foreground for efficient allocation
	count := 0
	for _, isFg := range mask {
		if isFg {
			count++
		}
	}

	foregroundPoints := make([]PointPolar, 0, count)
	for i, isForeground := range mask {
		if isForeground {
			foregroundPoints = append(foregroundPoints, points[i])
		}
	}

	return foregroundPoints
}

// FrameMetrics contains per-frame statistics about foreground extraction.
type FrameMetrics struct {
	TotalPoints        int     `json:"total_points"`
	ForegroundPoints   int     `json:"foreground_points"`
	BackgroundPoints   int     `json:"background_points"`
	ForegroundFraction float64 `json:"foreground_fraction"`
	ProcessingTimeUs   int64   `json:"processing_time_us"`
}

// ComputeFrameMetrics computes metrics from a foreground mask.
func ComputeFrameMetrics(mask []bool, processingTimeUs int64) FrameMetrics {
	total := len(mask)
	fg := 0
	for _, isFg := range mask {
		if isFg {
			fg++
		}
	}
	bg := total - fg

	fraction := 0.0
	if total > 0 {
		fraction = float64(fg) / float64(total)
	}

	return FrameMetrics{
		TotalPoints:        total,
		ForegroundPoints:   fg,
		BackgroundPoints:   bg,
		ForegroundFraction: fraction,
		ProcessingTimeUs:   processingTimeUs,
	}
}
