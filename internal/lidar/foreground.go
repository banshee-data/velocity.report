package lidar

import (
	"log"
	"math"
	"time"
)

// Constants for foreground extraction configuration
const (
	// DefaultClosenessSensitivityMultiplier is the default multiplier for closeness threshold
	DefaultClosenessSensitivityMultiplier = 3.0
	// DefaultNeighborConfirmationCount is the default number of neighbors needed for confirmation
	DefaultNeighborConfirmationCount = 3
	// FreezeThresholdMultiplier is the multiplier applied to closeness threshold to trigger cell freeze
	FreezeThresholdMultiplier = 3.0
	// DefaultReacquisitionBoostMultiplier is the default multiplier for fast re-acquisition
	DefaultReacquisitionBoostMultiplier = 5.0
	// DefaultMinConfidenceFloor is the minimum TimesSeenCount to preserve during foreground
	DefaultMinConfidenceFloor = 3
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

	warmupActive := false
	effectiveAlpha := alpha
	warmupFramesRemaining := 0
	warmupDuration := int64(0)
	warmupElapsed := time.Duration(0)

	g.mu.Lock()
	defer g.mu.Unlock()

	// Initialise warmup counters on first frame
	if bm.StartTime.IsZero() {
		bm.StartTime = now
	}
	if g.WarmupFramesRemaining == 0 && g.Params.WarmupMinFrames > 0 && !g.SettlingComplete {
		g.WarmupFramesRemaining = g.Params.WarmupMinFrames
	}

	// Read runtime-tunable params under lock
	noiseRel := float64(g.Params.NoiseRelativeFraction)
	if noiseRel <= 0 {
		noiseRel = 0.01
	}
	closenessMultiplier := float64(g.Params.ClosenessSensitivityMultiplier)
	if closenessMultiplier <= 0 {
		closenessMultiplier = DefaultClosenessSensitivityMultiplier
	}
	neighConfirm := g.Params.NeighborConfirmationCount
	if neighConfirm < 0 {
		neighConfirm = DefaultNeighborConfirmationCount
	}
	seedFromFirst := g.Params.SeedFromFirstObservation

	// Fast re-acquisition parameters
	reacqBoost := float64(g.Params.ReacquisitionBoostMultiplier)
	if reacqBoost <= 0 {
		reacqBoost = DefaultReacquisitionBoostMultiplier
	}
	minConfFloor := g.Params.MinConfidenceFloor
	if minConfFloor == 0 {
		minConfFloor = DefaultMinConfidenceFloor
	}

	// Warmup gating: suppress foreground output until duration and/or frames satisfied.
	postSettleAlpha := float64(g.Params.PostSettleUpdateFraction)
	if postSettleAlpha > 0 && postSettleAlpha <= 1 {
		effectiveAlpha = postSettleAlpha
	}
	if !g.SettlingComplete {
		framesReady := g.Params.WarmupMinFrames <= 0 || g.WarmupFramesRemaining <= 0
		durReady := g.Params.WarmupDurationNanos <= 0 || (nowNanos-bm.StartTime.UnixNano() >= g.Params.WarmupDurationNanos)
		if framesReady && durReady {
			g.SettlingComplete = true
			if postSettleAlpha > 0 && postSettleAlpha <= 1 {
				effectiveAlpha = postSettleAlpha
			}
		} else {
			warmupActive = true
			if g.WarmupFramesRemaining > 0 {
				g.WarmupFramesRemaining--
			}
			// During warmup use pre-set alpha to seed background faster.
			effectiveAlpha = alpha
		}
	}
	warmupFramesRemaining = g.WarmupFramesRemaining
	warmupDuration = g.Params.WarmupDurationNanos
	warmupElapsed = now.Sub(bm.StartTime)
	if effectiveAlpha <= 0 || effectiveAlpha > 1 {
		effectiveAlpha = alpha
	}

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

		// If frozen, treat as foreground but still track for fast re-acquisition
		if cell.FrozenUntilUnixNanos > nowNanos {
			foregroundMask[i] = true
			foregroundCount++
			// Track foreground even when frozen (for fast re-acquisition)
			if cell.RecentForegroundCount < 65535 {
				cell.RecentForegroundCount++
			}
			continue
		}

		// Same-ring neighbor confirmation
		neighborConfirmCount := 0
		if neighConfirm > 0 {
			// Search radius should be at least equal to the required confirmation count
			// to make it possible to satisfy the condition.
			searchRadius := neighConfirm
			if searchRadius < 1 {
				searchRadius = 1
			}
			// Cap search radius to avoid excessive checks
			if searchRadius > 10 {
				searchRadius = 10
			}

			for deltaAz := -searchRadius; deltaAz <= searchRadius; deltaAz++ {
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
		}

		// Closeness threshold: scales with cell spread and distance
		closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+noiseRel*p.Distance+0.01) + safety
		cellDiff := math.Abs(float64(cell.AverageRangeMeters) - p.Distance)

		// Classification decision
		isBackgroundLike := cellDiff <= closenessThreshold || (neighConfirm > 0 && neighborConfirmCount >= neighConfirm)

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
				cell.RecentForegroundCount = 0
			} else {
				// Fast re-acquisition: if cell recently saw foreground, use boosted alpha
				// to quickly re-converge to background after object passes
				updateAlpha := effectiveAlpha
				if cell.RecentForegroundCount > 0 {
					updateAlpha = math.Min(effectiveAlpha*reacqBoost, 0.5) // cap at 0.5 to avoid instability
				}

				oldAvg := float64(cell.AverageRangeMeters)
				newAvg := (1.0-updateAlpha)*oldAvg + updateAlpha*p.Distance
				deviation := math.Abs(p.Distance - oldAvg)
				newSpread := (1.0-updateAlpha)*float64(cell.RangeSpreadMeters) + updateAlpha*deviation
				cell.AverageRangeMeters = float32(newAvg)
				cell.RangeSpreadMeters = float32(newSpread)
				cell.TimesSeenCount++

				// Decay RecentForegroundCount now that we're seeing background again
				if cell.RecentForegroundCount > 0 {
					cell.RecentForegroundCount--
				}
			}
			cell.LastUpdateUnixNanos = nowNanos
		} else {
			foregroundMask[i] = true
			foregroundCount++

			// Track that this cell is seeing foreground (for fast re-acquisition)
			if cell.RecentForegroundCount < 65535 {
				cell.RecentForegroundCount++
			}

			// Decrease confidence for divergent observations, but maintain minimum floor
			// to prevent cells from "forgetting" their settled background
			if cell.TimesSeenCount > minConfFloor {
				cell.TimesSeenCount--
			} else if cell.TimesSeenCount > 0 && minConfFloor == 0 {
				// Only allow full drain if MinConfidenceFloor is explicitly 0
				cell.TimesSeenCount--
				if cell.TimesSeenCount == 0 && g.nonzeroCellCount > 0 {
					g.nonzeroCellCount--
				}
			}
			// Freeze cell if divergence is very large
			if cellDiff > FreezeThresholdMultiplier*closenessThreshold {
				cell.FrozenUntilUnixNanos = nowNanos + freezeDur
			}
			cell.LastUpdateUnixNanos = nowNanos
		}

		// Debug logging for specific region to investigate trailing foreground
		if bm.EnableDiagnostics && g.Params.IsInDebugRange(ring, az) {
			log.Printf("[FG_DEBUG] r=%d az=%.1f dist=%.3f avg=%.3f spread=%.3f diff=%.3f thresh=%.3f seen=%d recFg=%d frozen=%v isBg=%v",
				ring, az, p.Distance, cell.AverageRangeMeters, cell.RangeSpreadMeters,
				cellDiff, closenessThreshold, cell.TimesSeenCount, cell.RecentForegroundCount,
				cell.FrozenUntilUnixNanos > nowNanos, !foregroundMask[i])
		}

		g.ChangesSinceSnapshot++
	}

	// Suppress foreground output during warmup while still allowing background seeding.
	if warmupActive {
		suppressedFg := foregroundCount
		for i := range foregroundMask {
			foregroundMask[i] = false
		}
		foregroundCount = 0
		backgroundCount = int64(len(points))

		if bm != nil && bm.EnableDiagnostics {
			log.Printf("[Foreground] warmup active: suppressed_fg=%d total_points=%d warmup_frames_remaining=%d warmup_duration_ns=%d elapsed_ms=%d",
				suppressedFg, len(points), warmupFramesRemaining, warmupDuration, warmupElapsed.Milliseconds())
		}
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
