package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TuningConfig represents the root configuration for tuning parameters.
type TuningConfig struct {
	Lidar *LidarTuning `json:"lidar,omitempty"`
}

// LidarTuning contains LiDAR-specific tuning parameters.
type LidarTuning struct {
	Background   *BackgroundTuning   `json:"background,omitempty"`
	FrameBuilder *FrameBuilderTuning `json:"frame_builder,omitempty"`
}

// BackgroundTuning contains background manager tuning parameters.
type BackgroundTuning struct {
	NoiseRelativeFraction float64 `json:"noise_relative_fraction"`
	FlushInterval         string  `json:"flush_interval"` // duration string like "60s"
	FlushDisable          bool    `json:"flush_disable"`
	SeedFromFirst         bool    `json:"seed_from_first"`
}

// FrameBuilderTuning contains frame builder tuning parameters.
type FrameBuilderTuning struct {
	BufferTimeout  string `json:"buffer_timeout"` // duration string like "500ms"
	MinFramePoints int    `json:"min_frame_points"`
}

// DefaultTuningConfig returns a TuningConfig with default values.
func DefaultTuningConfig() *TuningConfig {
	return &TuningConfig{
		Lidar: &LidarTuning{
			Background: &BackgroundTuning{
				NoiseRelativeFraction: 0.04,
				FlushInterval:         "60s",
				FlushDisable:          false,
				SeedFromFirst:         true,
			},
			FrameBuilder: &FrameBuilderTuning{
				BufferTimeout:  "500ms",
				MinFramePoints: 1000,
			},
		},
	}
}

// LoadTuningConfig loads a TuningConfig from a JSON file.
func LoadTuningConfig(path string) (*TuningConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg TuningConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the configuration values are valid.
func (c *TuningConfig) Validate() error {
	if c.Lidar == nil {
		return fmt.Errorf("lidar configuration is required")
	}

	if c.Lidar.Background == nil {
		return fmt.Errorf("lidar.background configuration is required")
	}

	bg := c.Lidar.Background

	// Validate NoiseRelativeFraction
	if bg.NoiseRelativeFraction < 0 || bg.NoiseRelativeFraction > 1 {
		return fmt.Errorf("noise_relative_fraction must be between 0 and 1, got %f", bg.NoiseRelativeFraction)
	}

	// Validate FlushInterval can be parsed
	if bg.FlushInterval != "" {
		if _, err := time.ParseDuration(bg.FlushInterval); err != nil {
			return fmt.Errorf("invalid flush_interval '%s': %w", bg.FlushInterval, err)
		}
	}

	if c.Lidar.FrameBuilder == nil {
		return fmt.Errorf("lidar.frame_builder configuration is required")
	}

	fb := c.Lidar.FrameBuilder

	// Validate BufferTimeout can be parsed
	if fb.BufferTimeout != "" {
		if _, err := time.ParseDuration(fb.BufferTimeout); err != nil {
			return fmt.Errorf("invalid buffer_timeout '%s': %w", fb.BufferTimeout, err)
		}
	}

	// Validate MinFramePoints
	if fb.MinFramePoints < 0 {
		return fmt.Errorf("min_frame_points must be non-negative, got %d", fb.MinFramePoints)
	}

	return nil
}

// GetFlushInterval parses and returns the FlushInterval as a time.Duration.
func (b *BackgroundTuning) GetFlushInterval() time.Duration {
	if b.FlushInterval == "" {
		return 0
	}
	d, err := time.ParseDuration(b.FlushInterval)
	if err != nil {
		// This shouldn't happen if Validate() was called, but return 0 as a safe default
		return 0
	}
	return d
}

// GetBufferTimeout parses and returns the BufferTimeout as a time.Duration.
func (f *FrameBuilderTuning) GetBufferTimeout() time.Duration {
	if f.BufferTimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(f.BufferTimeout)
	if err != nil {
		// This shouldn't happen if Validate() was called, but return 0 as a safe default
		return 0
	}
	return d
}
