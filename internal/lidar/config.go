// Package lidar provides LiDAR processing and background subtraction.
package lidar

import (
	"fmt"
	"time"
)

// BackgroundConfig provides a configuration builder for BackgroundParams.
// It allows setting parameters with defaults and validation before creating
// a BackgroundManager.
type BackgroundConfig struct {
	// Core background model parameters
	UpdateFraction           float32       // Alpha for background update (default: 0.02)
	ClosenessSensitivity     float32       // Multiplier for closeness threshold (default: 8.0)
	SafetyMargin             float32       // Meters added to threshold (default: 0.4)
	FreezeDuration           time.Duration // Time to freeze cell after foreground (default: 5s)
	NeighborConfirmation     int           // Neighbors required for foreground (default: 7)
	NoiseRelativeFraction    float32       // Fraction of range as noise (default: 0.04)
	MinConfidenceFloor       uint32        // Min confidence to preserve (default: 3)
	SeedFromFirstObservation bool          // Seed from first observation (default: true)

	// Settling and warmup
	SettlingPeriod  time.Duration // Time before first snapshot (default: 5m)
	WarmupDuration  time.Duration // Time for warmup phase (default: 30s)
	WarmupMinFrames int           // Min frames before settling (default: 100)

	// Snapshot persistence
	SnapshotInterval        time.Duration // Interval between snapshots (default: 2h)
	ChangeThresholdSnapshot int           // Min changed cells for snapshot (default: 100)

	// Advanced tuning (typically left at defaults)
	PostSettleUpdateFraction     float32 // Alpha after settling (default: 0)
	ReacquisitionBoostMultiplier float32 // Boost for re-acquiring background (default: 5.0)
	LockedBaselineThreshold      uint32  // Min count before locking baseline (default: 50)
	LockedBaselineMultiplier     float32 // Spread multiplier for locked baseline (default: 4.0)

	// Foreground filtering
	ForegroundMinClusterPoints int     // Min points for cluster (default: 0)
	ForegroundDBSCANEps        float32 // DBSCAN epsilon (default: 0)
}

// DefaultBackgroundConfig returns a BackgroundConfig with sensible defaults.
// These defaults are tuned for typical traffic monitoring scenarios.
func DefaultBackgroundConfig() *BackgroundConfig {
	return &BackgroundConfig{
		UpdateFraction:               0.02,
		ClosenessSensitivity:         8.0,
		SafetyMargin:                 0.4,
		FreezeDuration:               5 * time.Second,
		NeighborConfirmation:         7,
		NoiseRelativeFraction:        0.04,
		MinConfidenceFloor:           DefaultMinConfidenceFloor,
		SeedFromFirstObservation:     true,
		SettlingPeriod:               5 * time.Minute,
		WarmupDuration:               30 * time.Second,
		WarmupMinFrames:              100,
		SnapshotInterval:             2 * time.Hour,
		ChangeThresholdSnapshot:      100,
		ReacquisitionBoostMultiplier: 5.0,
		LockedBaselineThreshold:      50,
		LockedBaselineMultiplier:     4.0,

		// Foreground filtering
		ForegroundMinClusterPoints: 2,
		ForegroundDBSCANEps:        0.3,
	}
}

// Validate checks if the configuration is valid.
// Returns an error if any parameter is out of acceptable range.
func (c *BackgroundConfig) Validate() error {
	if c.UpdateFraction <= 0 || c.UpdateFraction > 1 {
		return fmt.Errorf("UpdateFraction must be in (0, 1], got %f", c.UpdateFraction)
	}
	if c.ClosenessSensitivity <= 0 {
		return fmt.Errorf("ClosenessSensitivity must be positive, got %f", c.ClosenessSensitivity)
	}
	if c.SafetyMargin < 0 {
		return fmt.Errorf("SafetyMargin must be non-negative, got %f", c.SafetyMargin)
	}
	if c.FreezeDuration < 0 {
		return fmt.Errorf("FreezeDuration must be non-negative, got %v", c.FreezeDuration)
	}
	if c.NeighborConfirmation < 0 || c.NeighborConfirmation > 8 {
		return fmt.Errorf("NeighborConfirmation must be in [0, 8], got %d", c.NeighborConfirmation)
	}
	if c.NoiseRelativeFraction < 0 || c.NoiseRelativeFraction > 1 {
		return fmt.Errorf("NoiseRelativeFraction must be in [0, 1], got %f", c.NoiseRelativeFraction)
	}
	if c.WarmupMinFrames < 0 {
		return fmt.Errorf("WarmupMinFrames must be non-negative, got %d", c.WarmupMinFrames)
	}
	if c.SettlingPeriod < 0 {
		return fmt.Errorf("SettlingPeriod must be non-negative, got %v", c.SettlingPeriod)
	}
	if c.WarmupDuration < 0 {
		return fmt.Errorf("WarmupDuration must be non-negative, got %v", c.WarmupDuration)
	}
	if c.SnapshotInterval < 0 {
		return fmt.Errorf("SnapshotInterval must be non-negative, got %v", c.SnapshotInterval)
	}
	if c.ChangeThresholdSnapshot < 0 {
		return fmt.Errorf("ChangeThresholdSnapshot must be non-negative, got %d", c.ChangeThresholdSnapshot)
	}
	return nil
}

// ToBackgroundParams converts the config to BackgroundParams for use with BackgroundManager.
func (c *BackgroundConfig) ToBackgroundParams() BackgroundParams {
	return BackgroundParams{
		BackgroundUpdateFraction:       c.UpdateFraction,
		ClosenessSensitivityMultiplier: c.ClosenessSensitivity,
		SafetyMarginMeters:             c.SafetyMargin,
		FreezeDurationNanos:            c.FreezeDuration.Nanoseconds(),
		NeighborConfirmationCount:      c.NeighborConfirmation,
		NoiseRelativeFraction:          c.NoiseRelativeFraction,
		MinConfidenceFloor:             c.MinConfidenceFloor,
		SeedFromFirstObservation:       c.SeedFromFirstObservation,
		SettlingPeriodNanos:            c.SettlingPeriod.Nanoseconds(),
		WarmupDurationNanos:            c.WarmupDuration.Nanoseconds(),
		WarmupMinFrames:                c.WarmupMinFrames,
		SnapshotIntervalNanos:          c.SnapshotInterval.Nanoseconds(),
		ChangeThresholdForSnapshot:     c.ChangeThresholdSnapshot,
		PostSettleUpdateFraction:       c.PostSettleUpdateFraction,
		ReacquisitionBoostMultiplier:   c.ReacquisitionBoostMultiplier,
		LockedBaselineThreshold:        c.LockedBaselineThreshold,
		LockedBaselineMultiplier:       c.LockedBaselineMultiplier,
		ForegroundMinClusterPoints:     c.ForegroundMinClusterPoints,
		ForegroundDBSCANEps:            c.ForegroundDBSCANEps,
	}
}

// WithUpdateFraction sets the background update fraction (alpha).
func (c *BackgroundConfig) WithUpdateFraction(f float32) *BackgroundConfig {
	c.UpdateFraction = f
	return c
}

// WithClosenessSensitivity sets the closeness sensitivity multiplier.
func (c *BackgroundConfig) WithClosenessSensitivity(s float32) *BackgroundConfig {
	c.ClosenessSensitivity = s
	return c
}

// WithSafetyMargin sets the safety margin in metres.
func (c *BackgroundConfig) WithSafetyMargin(m float32) *BackgroundConfig {
	c.SafetyMargin = m
	return c
}

// WithFreezeDuration sets the freeze duration after foreground.
func (c *BackgroundConfig) WithFreezeDuration(d time.Duration) *BackgroundConfig {
	c.FreezeDuration = d
	return c
}

// WithNeighborConfirmation sets the neighbor confirmation count.
func (c *BackgroundConfig) WithNeighborConfirmation(n int) *BackgroundConfig {
	c.NeighborConfirmation = n
	return c
}

// WithNoiseRelativeFraction sets the noise relative fraction.
func (c *BackgroundConfig) WithNoiseRelativeFraction(f float32) *BackgroundConfig {
	c.NoiseRelativeFraction = f
	return c
}

// WithSeedFromFirstObservation enables or disables seeding from first observation.
func (c *BackgroundConfig) WithSeedFromFirstObservation(enabled bool) *BackgroundConfig {
	c.SeedFromFirstObservation = enabled
	return c
}

// WithSettlingPeriod sets the settling period before first snapshot.
func (c *BackgroundConfig) WithSettlingPeriod(d time.Duration) *BackgroundConfig {
	c.SettlingPeriod = d
	return c
}

// WithWarmupDuration sets the warmup duration.
func (c *BackgroundConfig) WithWarmupDuration(d time.Duration) *BackgroundConfig {
	c.WarmupDuration = d
	return c
}

// WithWarmupMinFrames sets the minimum frames for warmup.
func (c *BackgroundConfig) WithWarmupMinFrames(n int) *BackgroundConfig {
	c.WarmupMinFrames = n
	return c
}

// WithSnapshotInterval sets the snapshot interval.
func (c *BackgroundConfig) WithSnapshotInterval(d time.Duration) *BackgroundConfig {
	c.SnapshotInterval = d
	return c
}

// WithChangeThresholdSnapshot sets the minimum changed cells for snapshot.
func (c *BackgroundConfig) WithChangeThresholdSnapshot(n int) *BackgroundConfig {
	c.ChangeThresholdSnapshot = n
	return c
}

// WithForegroundMinClusterPoints sets the minimum points for a foreground cluster.
func (c *BackgroundConfig) WithForegroundMinClusterPoints(n int) *BackgroundConfig {
	c.ForegroundMinClusterPoints = n
	return c
}

// WithForegroundDBSCANEps sets the DBSCAN epsilon for foreground clustering.
func (c *BackgroundConfig) WithForegroundDBSCANEps(eps float32) *BackgroundConfig {
	c.ForegroundDBSCANEps = eps
	return c
}
