package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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

// DefaultTuningConfig returns a TuningConfig with default values for all parameters.
// These defaults are derived from DefaultBackgroundConfig and DefaultTrackerConfig.
func DefaultTuningConfig() *TuningConfig {
	return &TuningConfig{
		// Background params
		NoiseRelative:              ptrFloat64(0.04),
		ClosenessMultiplier:        ptrFloat64(8.0),
		NeighborConfirmationCount:  ptrInt(7),
		SeedFromFirst:              ptrBool(true),
		WarmupDurationNanos:        ptrInt64(30000000000), // 30 seconds
		WarmupMinFrames:            ptrInt(100),
		PostSettleUpdateFraction:   ptrFloat64(0),
		ForegroundMinClusterPoints: ptrInt(0),
		ForegroundDBSCANEps:        ptrFloat64(0),

		// Frame builder params
		BufferTimeout:  ptrString("500ms"),
		MinFramePoints: ptrInt(1000),

		// Flush params
		FlushInterval:   ptrString("60s"),
		BackgroundFlush: ptrBool(false),

		// Tracker params
		GatingDistanceSquared: ptrFloat64(36.0),
		ProcessNoisePos:       ptrFloat64(0.1),
		ProcessNoiseVel:       ptrFloat64(0.5),
		MeasurementNoise:      ptrFloat64(0.2),
		OcclusionCovInflation: ptrFloat64(0.5),
		HitsToConfirm:         ptrInt(3),
		MaxMisses:             ptrInt(3),
		MaxMissesConfirmed:    ptrInt(15),
	}
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

	// Start from defaults so omitted JSON fields keep sensible values.
	cfg := DefaultTuningConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
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
