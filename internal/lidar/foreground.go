package lidar

import (
	"log"
	"math"
	"time"
)

// Constants for foreground extraction configuration.
// These are internal algorithm parameters, NOT user-tunable defaults.
// All user-tunable values come from config/tuning.defaults.json.
const (
	// safetyClosenessSensitivityMultiplier is a safety guard used only when
	// BackgroundParams.ClosenessSensitivityMultiplier is zero/negative,
	// which should never happen with a properly loaded config.
	safetyClosenessSensitivityMultiplier = 3.0
	// safetyNeighborConfirmationCount is a safety guard used only when
	// BackgroundParams.NeighborConfirmationCount is negative.
	safetyNeighborConfirmationCount = 3
	// FreezeThresholdMultiplier is the multiplier applied to closeness threshold to trigger cell freeze
	FreezeThresholdMultiplier = 3.0
	// DefaultReacquisitionBoostMultiplier is the default multiplier for fast re-acquisition
	DefaultReacquisitionBoostMultiplier = 5.0
	// DefaultMinConfidenceFloor is the minimum TimesSeenCount to preserve during foreground
	DefaultMinConfidenceFloor = 3
	// ThawGracePeriodNanos is the minimum time after freeze expiry before thaw detection triggers.
	// This prevents false triggers when FreezeDurationNanos=0 causes immediate "expiry".
	ThawGracePeriodNanos = int64(1_000_000) // 1ms
	// DefaultLockedBaselineThreshold is the minimum observations before locking baseline
	DefaultLockedBaselineThreshold = 50
	// DefaultLockedBaselineMultiplier defines the acceptance window as LockedSpread * multiplier
	DefaultLockedBaselineMultiplier = 4.0
	// regionRestoreMinFrames is the minimum number of warmup frames before
	// attempting to restore regions from the database. After this many frames,
	// enough data exists to compute a stable scene signature for matching.
	regionRestoreMinFrames = 10
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

	// Reuse mask buffer to avoid allocating ~69 KB every frame.
	// The buffer is zeroed (all false) before use since we only set true
	// for foreground points. Grown if the point count increases.
	n := len(points)
	if cap(bm.maskBuf) < n {
		bm.maskBuf = make([]bool, n)
	}
	foregroundMask = bm.maskBuf[:n]
	// Zero the slice — mandatory since we reuse the backing array and
	// previous iterations may have set elements to true.
	for i := range foregroundMask {
		foregroundMask[i] = false
	}

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
	startTimeWasZero := bm.StartTime.IsZero()
	if startTimeWasZero {
		bm.StartTime = now
	}
	if startTimeWasZero && g.WarmupFramesRemaining == 0 && g.Params.WarmupMinFrames > 0 && !g.SettlingComplete {
		g.WarmupFramesRemaining = g.Params.WarmupMinFrames
	}

	// Read runtime-tunable params under lock
	noiseRel := float64(g.Params.NoiseRelativeFraction)
	if noiseRel <= 0 {
		noiseRel = 0.01
	}
	closenessMultiplier := float64(g.Params.ClosenessSensitivityMultiplier)
	if closenessMultiplier <= 0 {
		closenessMultiplier = safetyClosenessSensitivityMultiplier
	}
	neighConfirm := g.Params.NeighborConfirmationCount
	if neighConfirm < 0 {
		neighConfirm = safetyNeighborConfirmationCount
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

	// Locked baseline parameters
	lockedThreshold := g.Params.LockedBaselineThreshold
	if lockedThreshold == 0 {
		lockedThreshold = DefaultLockedBaselineThreshold
	}
	lockedMultiplier := float64(g.Params.LockedBaselineMultiplier)
	if lockedMultiplier <= 0 {
		lockedMultiplier = DefaultLockedBaselineMultiplier
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
			// Trigger region identification when settling completes
			if g.RegionMgr != nil && !g.RegionMgr.IdentificationComplete {
				err := g.RegionMgr.IdentifyRegions(g, 50) // max 50 regions
				if err != nil {
					log.Printf("[BackgroundManager] Failed to identify regions: %v", err)
				}
				// Persist regions immediately so future runs can skip settling
				bm.persistRegionsOnSettleLocked()
			}
		} else {
			warmupActive = true
			if g.WarmupFramesRemaining > 0 {
				g.WarmupFramesRemaining--
			}
			// During warmup use pre-set alpha to seed background faster.
			effectiveAlpha = alpha
			// Collect variance metrics during settling
			if g.RegionMgr != nil && !g.RegionMgr.IdentificationComplete {
				g.RegionMgr.UpdateVarianceMetrics(g.Cells)
			}
			// Attempt early region restoration from DB after enough frames to
			// build a scene signature (~10 frames). This allows skipping the
			// remaining settling period when the scene matches a previous run.
			if !g.regionRestoreAttempted && bm.store != nil {
				framesProcessed := g.Params.WarmupMinFrames - g.WarmupFramesRemaining
				if framesProcessed >= regionRestoreMinFrames {
					if bm.tryRestoreRegionsFromStoreLocked() {
						// Settling was skipped — update local variables to reflect
						warmupActive = false
						if postSettleAlpha > 0 && postSettleAlpha <= 1 {
							effectiveAlpha = postSettleAlpha
						}
					}
				}
			}
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

		// If frozen, treat as foreground but don't accumulate recFg
		// The freeze itself is sufficient protection; accumulating recFg during freeze
		// causes unnecessarily high values (observed 70+) that persist after thaw.
		if cell.FrozenUntilUnixNanos > nowNanos {
			foregroundMask[i] = true
			foregroundCount++
			// Debug: log frozen cell observations to track freeze behavior
			if bm.EnableDiagnostics && g.Params.IsInDebugRange(ring, az) {
				remainingFreeze := float64(cell.FrozenUntilUnixNanos-nowNanos) / 1e9
				debugf("[FG_FROZEN] r=%d az=%.1f dist=%.3f avg=%.3f remainingFreezeSec=%.2f recFg=%d",
					ring, az, p.Distance, cell.AverageRangeMeters, remainingFreeze, cell.RecentForegroundCount)
			}
			continue
		}

		// Check if freeze just expired - reset recFg to give fast re-acquisition a clean start
		// This prevents artificially high recFg values accumulated during freeze from
		// causing prolonged boosted updates after thaw.
		// Note: We only trigger thaw when the freeze was meaningful (expired at least 1ms ago).
		// This avoids false triggers when FreezeDurationNanos=0 causes immediate "expiry".
		// Limitation: Thaw is only detected when a point observation hits this cell.
		if cell.FrozenUntilUnixNanos > 0 && cell.FrozenUntilUnixNanos+ThawGracePeriodNanos <= nowNanos {
			if bm.EnableDiagnostics && g.Params.IsInDebugRange(ring, az) {
				debugf("[FG_THAW] r=%d az=%.1f thawed after freeze, resetting recFg from %d to 0",
					ring, az, cell.RecentForegroundCount)
			}
			cell.RecentForegroundCount = 0
			cell.FrozenUntilUnixNanos = 0 // Clear the expired freeze timestamp
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
		// Warmup sensitivity scaling:
		// When a cell is new (low confidence), we haven't learned its true variance yet.
		// We should be more tolerant (higher threshold) to avoid classifying noise as foreground,
		// which prevents "initialization trails" where wall points are flagged as FG before spread converges.
		warmupMultiplier := 1.0
		if cell.TimesSeenCount < 100 {
			// Linear decay from 4.0x at count=0 to 1.0x at count=100
			warmupMultiplier = 1.0 + 3.0*float64(100-cell.TimesSeenCount)/100.0
		}

		closenessThreshold := closenessMultiplier*(float64(cell.RangeSpreadMeters)+noiseRel*p.Distance+0.01)*warmupMultiplier + safety
		cellDiff := math.Abs(float64(cell.AverageRangeMeters) - p.Distance)

		// Locked baseline classification: if cell has a locked baseline, use it for classification
		// This protects against EMA drift during transits
		isWithinLockedRange := false
		lockedThresholdU32 := uint32(lockedThreshold)
		if cell.LockedBaseline > 0 && cell.LockedAtCount >= lockedThresholdU32 {
			// Use locked baseline for classification - more stable than EMA average
			lockedDiff := math.Abs(float64(cell.LockedBaseline) - p.Distance)
			// Acceptance window: locked spread * multiplier + noise-based margin + safety
			lockedWindow := lockedMultiplier*float64(cell.LockedSpread) + noiseRel*p.Distance + safety
			if lockedWindow < 0.1 {
				lockedWindow = 0.1 // Minimum 10cm window
			}
			isWithinLockedRange = lockedDiff <= lockedWindow
		}

		// Classification decision: prioritize locked baseline if available
		isBackgroundLike := isWithinLockedRange ||
			cellDiff <= closenessThreshold ||
			(neighConfirm > 0 && neighborConfirmCount >= neighConfirm)

		// Deadlock Breaker:
		// If a cell is persistently classified as foreground (RecentForegroundCount high)
		// but we have low confidence in our background model (TimesSeenCount at floor),
		// and the divergence isn't massive enough to trigger a freeze ("Gray Zone"),
		// we assume our background model is stale/corrupted and force an update.
		// This fixes "ghost trails" where the model learns a transient edge value
		// and then rejects the true background because it differs from that edge.
		if !isBackgroundLike && cell.TimesSeenCount <= minConfFloor && cell.RecentForegroundCount > 4 {
			// Only force-learn if we wouldn't otherwise freeze this cell (avoid learning dynamic obstacles)
			freezeThresh := FreezeThresholdMultiplier * closenessThreshold
			if cellDiff <= freezeThresh {
				isBackgroundLike = true
			}
		}

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
				// Initialize locked baseline tracking
				cell.LockedBaseline = 0
				cell.LockedSpread = 0
				cell.LockedAtCount = 0
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

				// Lock the baseline once we've seen enough observations
				// This provides a stable reference that doesn't drift during transits
				lockedThresholdU32 := uint32(lockedThreshold)
				if cell.LockedAtCount < lockedThresholdU32 && cell.TimesSeenCount >= lockedThresholdU32 {
					cell.LockedBaseline = cell.AverageRangeMeters
					cell.LockedSpread = cell.RangeSpreadMeters
					cell.LockedAtCount = cell.TimesSeenCount
				} else if cell.LockedAtCount >= lockedThresholdU32 && cell.RecentForegroundCount == 0 {
					// Only update locked baseline during sustained background periods
					// (no recent foreground detections)
					lockAlpha := 0.001 // Very slow update rate for locked baseline
					cell.LockedBaseline = float32((1.0-lockAlpha)*float64(cell.LockedBaseline) + lockAlpha*p.Distance)
					lockSpreadDev := math.Abs(p.Distance - float64(cell.LockedBaseline))
					cell.LockedSpread = float32((1.0-lockAlpha)*float64(cell.LockedSpread) + lockAlpha*lockSpreadDev)
				}

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
			// Freeze cell if divergence is very large, but only if we are not confident
			// (TimesSeenCount < 100). If we have a solid background (e.g. static road),
			// a passing object should not freeze the background model.
			if cell.TimesSeenCount < 100 && cellDiff > FreezeThresholdMultiplier*closenessThreshold {
				if bm.EnableDiagnostics && g.Params.IsInDebugRange(ring, az) {
					debugf("[FG_FREEZE] r=%d az=%.1f froze for %.1fs, dist=%.3f avg=%.3f cellDiff=%.3f freezeThresh=%.3f recFg=%d",
						ring, az, float64(freezeDur)/1e9, p.Distance, cell.AverageRangeMeters,
						cellDiff, FreezeThresholdMultiplier*closenessThreshold, cell.RecentForegroundCount)
				}
				cell.FrozenUntilUnixNanos = nowNanos + freezeDur
			}
			cell.LastUpdateUnixNanos = nowNanos
		}

		// Debug logging for specific region to investigate trailing foreground
		if bm.EnableDiagnostics && g.Params.IsInDebugRange(ring, az) {
			debugf("[FG_DEBUG] r=%d az=%.1f dist=%.3f avg=%.3f spread=%.3f diff=%.3f thresh=%.3f seen=%d recFg=%d frozen=%v isBg=%v",
				ring, az, p.Distance, cell.AverageRangeMeters, cell.RangeSpreadMeters,
				cellDiff, closenessThreshold, cell.TimesSeenCount, cell.RecentForegroundCount,
				cell.FrozenUntilUnixNanos > nowNanos, !foregroundMask[i])
		}

		g.ChangesSinceSnapshot++

		// Update per-range acceptance metrics (mirrors ProcessFramePolar logic).
		// This is essential for the sweep tool to measure background-model fit.
		for b := range g.AcceptanceBucketsMeters {
			if p.Distance <= g.AcceptanceBucketsMeters[b] {
				if isBackgroundLike {
					g.AcceptByRangeBuckets[b]++
				} else {
					g.RejectByRangeBuckets[b]++
				}
				break
			}
		}
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
			debugf("[Foreground] warmup active: suppressed_fg=%d total_points=%d warmup_frames_remaining=%d warmup_duration_ns=%d elapsed_ms=%d",
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
