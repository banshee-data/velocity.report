package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultConfigPath is the path to the canonical tuning defaults file.
// This is the single source of truth for all default tuning values.
const DefaultConfigPath = "config/tuning.defaults.json"

// TuningConfig represents the root configuration for tuning parameters.
// The schema matches the /api/lidar/params endpoint so the same JSON
// can be used for both startup configuration and runtime updates.
type TuningConfig struct {
	// Background params
	NoiseRelative              *float64 `json:"noise_relative,omitempty"`
	ClosenessMultiplier        *float64 `json:"closeness_multiplier,omitempty"`
	NeighborConfirmationCount  *int     `json:"neighbor_confirmation_count,omitempty"`
	SeedFromFirst              *bool    `json:"seed_from_first,omitempty"`
	WarmupDurationNanos        *int64   `json:"warmup_duration_nanos,omitempty"`
	WarmupMinFrames            *int     `json:"warmup_min_frames,omitempty"`
	PostSettleUpdateFraction   *float64 `json:"post_settle_update_fraction,omitempty"`
	ForegroundMinClusterPoints *int     `json:"foreground_min_cluster_points,omitempty"`
	ForegroundDBSCANEps        *float64 `json:"foreground_dbscan_eps,omitempty"`

	// Frame builder params
	BufferTimeout  *string `json:"buffer_timeout,omitempty"` // duration string like "500ms"
	MinFramePoints *int    `json:"min_frame_points,omitempty"`

	// Flush params
	FlushInterval   *string `json:"flush_interval,omitempty"` // duration string like "60s"
	BackgroundFlush *bool   `json:"background_flush,omitempty"`

	// Tracker params (optional)
	GatingDistanceSquared *float64 `json:"gating_distance_squared,omitempty"`
	ProcessNoisePos       *float64 `json:"process_noise_pos,omitempty"`
	ProcessNoiseVel       *float64 `json:"process_noise_vel,omitempty"`
	MeasurementNoise      *float64 `json:"measurement_noise,omitempty"`
	OcclusionCovInflation *float64 `json:"occlusion_cov_inflation,omitempty"`
	HitsToConfirm         *int     `json:"hits_to_confirm,omitempty"`
	MaxMisses             *int     `json:"max_misses,omitempty"`
	MaxMissesConfirmed    *int     `json:"max_misses_confirmed,omitempty"`
}

// Helper functions to create pointers
func ptrFloat64(v float64) *float64 { return &v }
func ptrBool(v bool) *bool          { return &v }
func ptrString(v string) *string    { return &v }
func ptrInt(v int) *int             { return &v }
func ptrInt64(v int64) *int64       { return &v }

// EmptyTuningConfig returns a TuningConfig with all fields set to nil.
// Use LoadTuningConfig to load actual values from the defaults file.
func EmptyTuningConfig() *TuningConfig {
	return &TuningConfig{}
}

// LoadTuningConfig loads a TuningConfig from a JSON file.
// The file is validated to ensure it has a .json extension and is under the max file size.
// Fields omitted from the JSON file retain their default values, so
// partial configs are safe.
func LoadTuningConfig(path string) (*TuningConfig, error) {
	// Validate the config file path.
	cleanPath := filepath.Clean(path)
	if ext := filepath.Ext(cleanPath); ext != ".json" {
		return nil, fmt.Errorf("config file must have .json extension, got %q", ext)
	}

	// Check file size for safety (max 1MB)
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}
	const maxFileSize = 1 * 1024 * 1024 // 1MB
	if fileInfo.Size() > maxFileSize {
		return nil, fmt.Errorf("config file too large: %d bytes (max %d)", fileInfo.Size(), maxFileSize)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON into empty config. The Get* methods provide fallback
	// defaults for any fields not specified in the JSON.
	cfg := EmptyTuningConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// MustLoadDefaultConfig loads the canonical tuning defaults from DefaultConfigPath.
// It searches for the file in the current directory and common parent directories.
// Panics if the file cannot be loaded, intended for test setup.
func MustLoadDefaultConfig() *TuningConfig {
	// Try paths from current dir up to repo root
	candidates := []string{
		DefaultConfigPath,
		"../../" + DefaultConfigPath,          // from internal/config/
		"../../../" + DefaultConfigPath,       // from internal/lidar/sweep/
		"../../../../" + DefaultConfigPath,    // deeper packages
		"../../../../../" + DefaultConfigPath, // even deeper
	}
	for _, path := range candidates {
		if cfg, err := LoadTuningConfig(path); err == nil {
			return cfg
		}
	}
	panic("cannot find " + DefaultConfigPath + " - run tests from repository root")
}

// Validate checks that the configuration values are valid.
func (c *TuningConfig) Validate() error {
	// Validate NoiseRelative if set
	if c.NoiseRelative != nil {
		if *c.NoiseRelative < 0 || *c.NoiseRelative > 1 {
			return fmt.Errorf("noise_relative must be between 0 and 1, got %f", *c.NoiseRelative)
		}
	}

	// Validate FlushInterval can be parsed if set
	if c.FlushInterval != nil && *c.FlushInterval != "" {
		if _, err := time.ParseDuration(*c.FlushInterval); err != nil {
			return fmt.Errorf("invalid flush_interval '%s': %w", *c.FlushInterval, err)
		}
	}

	// Validate BufferTimeout can be parsed if set
	if c.BufferTimeout != nil && *c.BufferTimeout != "" {
		if _, err := time.ParseDuration(*c.BufferTimeout); err != nil {
			return fmt.Errorf("invalid buffer_timeout '%s': %w", *c.BufferTimeout, err)
		}
	}

	// Validate MinFramePoints if set
	if c.MinFramePoints != nil {
		if *c.MinFramePoints < 0 {
			return fmt.Errorf("min_frame_points must be non-negative, got %d", *c.MinFramePoints)
		}
	}

	return nil
}

// GetFlushInterval parses and returns the FlushInterval as a time.Duration.
func (c *TuningConfig) GetFlushInterval() time.Duration {
	if c.FlushInterval == nil || *c.FlushInterval == "" {
		return 60 * time.Second // default
	}
	d, err := time.ParseDuration(*c.FlushInterval)
	if err != nil {
		return 60 * time.Second // default on parse error
	}
	return d
}

// GetBufferTimeout parses and returns the BufferTimeout as a time.Duration.
func (c *TuningConfig) GetBufferTimeout() time.Duration {
	if c.BufferTimeout == nil || *c.BufferTimeout == "" {
		return 500 * time.Millisecond // default
	}
	d, err := time.ParseDuration(*c.BufferTimeout)
	if err != nil {
		return 500 * time.Millisecond // default on parse error
	}
	return d
}

// GetNoiseRelative returns the noise_relative value or the default.
func (c *TuningConfig) GetNoiseRelative() float64 {
	if c.NoiseRelative == nil {
		return 0.04 // default
	}
	return *c.NoiseRelative
}

// GetSeedFromFirst returns the seed_from_first value or the default.
func (c *TuningConfig) GetSeedFromFirst() bool {
	if c.SeedFromFirst == nil {
		return true // default
	}
	return *c.SeedFromFirst
}

// GetMinFramePoints returns the min_frame_points value or the default.
func (c *TuningConfig) GetMinFramePoints() int {
	if c.MinFramePoints == nil {
		return 1000 // default
	}
	return *c.MinFramePoints
}

// GetBackgroundFlush returns the background_flush value or the default.
func (c *TuningConfig) GetBackgroundFlush() bool {
	if c.BackgroundFlush == nil {
		return false // default: flushing disabled
	}
	return *c.BackgroundFlush
}

// GetClosenessMultiplier returns the closeness_multiplier value or the default.
func (c *TuningConfig) GetClosenessMultiplier() float64 {
	if c.ClosenessMultiplier == nil {
		return 8.0
	}
	return *c.ClosenessMultiplier
}

// GetNeighborConfirmationCount returns the neighbor_confirmation_count value or the default.
func (c *TuningConfig) GetNeighborConfirmationCount() int {
	if c.NeighborConfirmationCount == nil {
		return 7
	}
	return *c.NeighborConfirmationCount
}

// GetWarmupDurationNanos returns the warmup_duration_nanos value or the default.
func (c *TuningConfig) GetWarmupDurationNanos() int64 {
	if c.WarmupDurationNanos == nil {
		return 30000000000 // 30 seconds
	}
	return *c.WarmupDurationNanos
}

// GetWarmupMinFrames returns the warmup_min_frames value or the default.
func (c *TuningConfig) GetWarmupMinFrames() int {
	if c.WarmupMinFrames == nil {
		return 100
	}
	return *c.WarmupMinFrames
}

// GetPostSettleUpdateFraction returns the post_settle_update_fraction value or the default.
func (c *TuningConfig) GetPostSettleUpdateFraction() float64 {
	if c.PostSettleUpdateFraction == nil {
		return 0
	}
	return *c.PostSettleUpdateFraction
}

// GetForegroundMinClusterPoints returns the foreground_min_cluster_points value or the default.
func (c *TuningConfig) GetForegroundMinClusterPoints() int {
	if c.ForegroundMinClusterPoints == nil {
		return 2
	}
	return *c.ForegroundMinClusterPoints
}

// GetForegroundDBSCANEps returns the foreground_dbscan_eps value or the default.
func (c *TuningConfig) GetForegroundDBSCANEps() float64 {
	if c.ForegroundDBSCANEps == nil {
		return 0.3
	}
	return *c.ForegroundDBSCANEps
}

// GetGatingDistanceSquared returns the gating_distance_squared value or the default.
func (c *TuningConfig) GetGatingDistanceSquared() float64 {
	if c.GatingDistanceSquared == nil {
		return 4.0
	}
	return *c.GatingDistanceSquared
}

// GetProcessNoisePos returns the process_noise_pos value or the default.
func (c *TuningConfig) GetProcessNoisePos() float64 {
	if c.ProcessNoisePos == nil {
		return 0.1
	}
	return *c.ProcessNoisePos
}

// GetProcessNoiseVel returns the process_noise_vel value or the default.
func (c *TuningConfig) GetProcessNoiseVel() float64 {
	if c.ProcessNoiseVel == nil {
		return 0.5
	}
	return *c.ProcessNoiseVel
}

// GetMeasurementNoise returns the measurement_noise value or the default.
func (c *TuningConfig) GetMeasurementNoise() float64 {
	if c.MeasurementNoise == nil {
		return 0.3
	}
	return *c.MeasurementNoise
}

// GetOcclusionCovInflation returns the occlusion_cov_inflation value or the default.
func (c *TuningConfig) GetOcclusionCovInflation() float64 {
	if c.OcclusionCovInflation == nil {
		return 0.5
	}
	return *c.OcclusionCovInflation
}

// GetHitsToConfirm returns the hits_to_confirm value or the default.
func (c *TuningConfig) GetHitsToConfirm() int {
	if c.HitsToConfirm == nil {
		return 3
	}
	return *c.HitsToConfirm
}

// GetMaxMisses returns the max_misses value or the default.
func (c *TuningConfig) GetMaxMisses() int {
	if c.MaxMisses == nil {
		return 3
	}
	return *c.MaxMisses
}

// GetMaxMissesConfirmed returns the max_misses_confirmed value or the default.
func (c *TuningConfig) GetMaxMissesConfirmed() int {
	if c.MaxMissesConfirmed == nil {
		return 15
	}
	return *c.MaxMissesConfirmed
}
