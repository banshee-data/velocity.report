package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultConfigPath is the path to the canonical tuning defaults file.
// This is the single source of truth for all tuning parameter values.
// The Go binary has NO hardcoded fallback defaults; all values must come
// from this file (or an alternative specified via --config).
const DefaultConfigPath = "config/tuning.defaults.json"

// TuningConfig represents the root configuration for tuning parameters.
// The schema matches the /api/lidar/params endpoint so the same JSON
// can be used for both startup configuration and runtime updates.
type TuningConfig struct {
	// Background params
	BackgroundUpdateFraction  *float64 `json:"background_update_fraction,omitempty"`
	ClosenessMultiplier       *float64 `json:"closeness_multiplier,omitempty"`
	SafetyMarginMeters        *float64 `json:"safety_margin_meters,omitempty"`
	NoiseRelative             *float64 `json:"noise_relative,omitempty"`
	NeighborConfirmationCount *int     `json:"neighbor_confirmation_count,omitempty"`
	SeedFromFirst             *bool    `json:"seed_from_first,omitempty"`
	WarmupDurationNanos       *int64   `json:"warmup_duration_nanos,omitempty"`
	WarmupMinFrames           *int     `json:"warmup_min_frames,omitempty"`
	PostSettleUpdateFraction  *float64 `json:"post_settle_update_fraction,omitempty"`
	EnableDiagnostics         *bool    `json:"enable_diagnostics,omitempty"`

	// Foreground clustering params
	ForegroundDBSCANEps        *float64 `json:"foreground_dbscan_eps,omitempty"`
	ForegroundMinClusterPoints *int     `json:"foreground_min_cluster_points,omitempty"`
	ForegroundMaxInputPoints   *int     `json:"foreground_max_input_points,omitempty"`

	// Frame builder params
	BufferTimeout  *string `json:"buffer_timeout,omitempty"` // duration string like "500ms"
	MinFramePoints *int    `json:"min_frame_points,omitempty"`

	// Flush params
	FlushInterval   *string `json:"flush_interval,omitempty"` // duration string like "60s"
	BackgroundFlush *bool   `json:"background_flush,omitempty"`

	// Tracker params
	GatingDistanceSquared *float64 `json:"gating_distance_squared,omitempty"`
	ProcessNoisePos       *float64 `json:"process_noise_pos,omitempty"`
	ProcessNoiseVel       *float64 `json:"process_noise_vel,omitempty"`
	MeasurementNoise      *float64 `json:"measurement_noise,omitempty"`
	OcclusionCovInflation *float64 `json:"occlusion_cov_inflation,omitempty"`
	HitsToConfirm         *int     `json:"hits_to_confirm,omitempty"`
	MaxMisses             *int     `json:"max_misses,omitempty"`
	MaxMissesConfirmed    *int     `json:"max_misses_confirmed,omitempty"`
	MaxTracks             *int     `json:"max_tracks,omitempty"`

	// Height band filter params (sensor-frame Z coordinates)
	HeightBandFloor   *float64 `json:"height_band_floor,omitempty"`
	HeightBandCeiling *float64 `json:"height_band_ceiling,omitempty"`
	RemoveGround      *bool    `json:"remove_ground,omitempty"`

	// Cluster filtering params
	MaxClusterDiameter    *float64 `json:"max_cluster_diameter,omitempty"`
	MinClusterDiameter    *float64 `json:"min_cluster_diameter,omitempty"`
	MaxClusterAspectRatio *float64 `json:"max_cluster_aspect_ratio,omitempty"`

	// Tracker physics/kinematics params
	MaxReasonableSpeedMps       *float64 `json:"max_reasonable_speed_mps,omitempty"`
	MaxPositionJumpMeters       *float64 `json:"max_position_jump_meters,omitempty"`
	MaxPredictDt                *float64 `json:"max_predict_dt,omitempty"`
	MaxCovarianceDiag           *float64 `json:"max_covariance_diag,omitempty"`
	MinPointsForPCA             *int     `json:"min_points_for_pca,omitempty"`
	OBBHeadingSmoothingAlpha    *float64 `json:"obb_heading_smoothing_alpha,omitempty"`
	OBBAspectRatioLockThreshold *float64 `json:"obb_aspect_ratio_lock_threshold,omitempty"`
	MaxTrackHistoryLength       *int     `json:"max_track_history_length,omitempty"`
	MaxSpeedHistoryLength       *int     `json:"max_speed_history_length,omitempty"`
	MergeSizeRatio              *float64 `json:"merge_size_ratio,omitempty"`
	SplitSizeRatio              *float64 `json:"split_size_ratio,omitempty"`
	DeletedTrackGracePeriod     *string  `json:"deleted_track_grace_period,omitempty"`

	// Classification params
	MinObservationsForClassification *int `json:"min_observations_for_classification,omitempty"`
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
// The file must contain ALL required keys — there are no fallback defaults.
// If any required field is missing, validation will fail.
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

	cfg := EmptyTuningConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Validate ranges and completeness (all fields must be present).
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	if err := cfg.ValidateComplete(); err != nil {
		return nil, fmt.Errorf("incomplete configuration: %w", err)
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

// Validate checks that the configuration values are within acceptable ranges.
func (c *TuningConfig) Validate() error {
	if c.NoiseRelative != nil {
		if *c.NoiseRelative < 0 || *c.NoiseRelative > 1 {
			return fmt.Errorf("noise_relative must be between 0 and 1, got %f", *c.NoiseRelative)
		}
	}
	if c.FlushInterval != nil && *c.FlushInterval != "" {
		if _, err := time.ParseDuration(*c.FlushInterval); err != nil {
			return fmt.Errorf("invalid flush_interval '%s': %w", *c.FlushInterval, err)
		}
	}
	if c.BufferTimeout != nil && *c.BufferTimeout != "" {
		if _, err := time.ParseDuration(*c.BufferTimeout); err != nil {
			return fmt.Errorf("invalid buffer_timeout '%s': %w", *c.BufferTimeout, err)
		}
	}
	if c.MinFramePoints != nil {
		if *c.MinFramePoints < 0 {
			return fmt.Errorf("min_frame_points must be non-negative, got %d", *c.MinFramePoints)
		}
	}
	if c.DeletedTrackGracePeriod != nil && *c.DeletedTrackGracePeriod != "" {
		if _, err := time.ParseDuration(*c.DeletedTrackGracePeriod); err != nil {
			return fmt.Errorf("invalid deleted_track_grace_period '%s': %w", *c.DeletedTrackGracePeriod, err)
		}
	}
	return nil
}

// ValidateComplete checks that ALL required fields are present (non-nil).
// Called after loading from a config file to ensure no keys are missing.
// There are no hardcoded fallback defaults — every value must come from the file.
func (c *TuningConfig) ValidateComplete() error {
	var missing []string
	if c.BackgroundUpdateFraction == nil {
		missing = append(missing, "background_update_fraction")
	}
	if c.ClosenessMultiplier == nil {
		missing = append(missing, "closeness_multiplier")
	}
	if c.SafetyMarginMeters == nil {
		missing = append(missing, "safety_margin_meters")
	}
	if c.NoiseRelative == nil {
		missing = append(missing, "noise_relative")
	}
	if c.NeighborConfirmationCount == nil {
		missing = append(missing, "neighbor_confirmation_count")
	}
	if c.SeedFromFirst == nil {
		missing = append(missing, "seed_from_first")
	}
	if c.WarmupDurationNanos == nil {
		missing = append(missing, "warmup_duration_nanos")
	}
	if c.WarmupMinFrames == nil {
		missing = append(missing, "warmup_min_frames")
	}
	if c.PostSettleUpdateFraction == nil {
		missing = append(missing, "post_settle_update_fraction")
	}
	if c.EnableDiagnostics == nil {
		missing = append(missing, "enable_diagnostics")
	}
	if c.ForegroundDBSCANEps == nil {
		missing = append(missing, "foreground_dbscan_eps")
	}
	if c.ForegroundMinClusterPoints == nil {
		missing = append(missing, "foreground_min_cluster_points")
	}
	if c.ForegroundMaxInputPoints == nil {
		missing = append(missing, "foreground_max_input_points")
	}
	if c.BufferTimeout == nil {
		missing = append(missing, "buffer_timeout")
	}
	if c.MinFramePoints == nil {
		missing = append(missing, "min_frame_points")
	}
	if c.FlushInterval == nil {
		missing = append(missing, "flush_interval")
	}
	if c.BackgroundFlush == nil {
		missing = append(missing, "background_flush")
	}
	if c.GatingDistanceSquared == nil {
		missing = append(missing, "gating_distance_squared")
	}
	if c.ProcessNoisePos == nil {
		missing = append(missing, "process_noise_pos")
	}
	if c.ProcessNoiseVel == nil {
		missing = append(missing, "process_noise_vel")
	}
	if c.MeasurementNoise == nil {
		missing = append(missing, "measurement_noise")
	}
	if c.OcclusionCovInflation == nil {
		missing = append(missing, "occlusion_cov_inflation")
	}
	if c.HitsToConfirm == nil {
		missing = append(missing, "hits_to_confirm")
	}
	if c.MaxMisses == nil {
		missing = append(missing, "max_misses")
	}
	if c.MaxMissesConfirmed == nil {
		missing = append(missing, "max_misses_confirmed")
	}
	if c.MaxTracks == nil {
		missing = append(missing, "max_tracks")
	}
	if c.HeightBandFloor == nil {
		missing = append(missing, "height_band_floor")
	}
	if c.HeightBandCeiling == nil {
		missing = append(missing, "height_band_ceiling")
	}
	if c.RemoveGround == nil {
		missing = append(missing, "remove_ground")
	}
	if c.MaxClusterDiameter == nil {
		missing = append(missing, "max_cluster_diameter")
	}
	if c.MinClusterDiameter == nil {
		missing = append(missing, "min_cluster_diameter")
	}
	if c.MaxClusterAspectRatio == nil {
		missing = append(missing, "max_cluster_aspect_ratio")
	}
	if c.MaxReasonableSpeedMps == nil {
		missing = append(missing, "max_reasonable_speed_mps")
	}
	if c.MaxPositionJumpMeters == nil {
		missing = append(missing, "max_position_jump_meters")
	}
	if c.MaxPredictDt == nil {
		missing = append(missing, "max_predict_dt")
	}
	if c.MaxCovarianceDiag == nil {
		missing = append(missing, "max_covariance_diag")
	}
	if c.MinPointsForPCA == nil {
		missing = append(missing, "min_points_for_pca")
	}
	if c.OBBHeadingSmoothingAlpha == nil {
		missing = append(missing, "obb_heading_smoothing_alpha")
	}
	if c.OBBAspectRatioLockThreshold == nil {
		missing = append(missing, "obb_aspect_ratio_lock_threshold")
	}
	if c.MaxTrackHistoryLength == nil {
		missing = append(missing, "max_track_history_length")
	}
	if c.MaxSpeedHistoryLength == nil {
		missing = append(missing, "max_speed_history_length")
	}
	if c.MergeSizeRatio == nil {
		missing = append(missing, "merge_size_ratio")
	}
	if c.SplitSizeRatio == nil {
		missing = append(missing, "split_size_ratio")
	}
	if c.DeletedTrackGracePeriod == nil {
		missing = append(missing, "deleted_track_grace_period")
	}
	if c.MinObservationsForClassification == nil {
		missing = append(missing, "min_observations_for_classification")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required keys: %v", missing)
	}
	return nil
}

// ─── Getters ────────────────────────────────────────────────────────────────
// All getters dereference the pointer field directly. They panic on nil,
// which indicates the config was not loaded from a complete file. Always
// call ValidateComplete() before using getters.

// GetFlushInterval parses and returns the FlushInterval as a time.Duration.
func (c *TuningConfig) GetFlushInterval() time.Duration {
	d, _ := time.ParseDuration(*c.FlushInterval)
	return d
}

// GetBufferTimeout parses and returns the BufferTimeout as a time.Duration.
func (c *TuningConfig) GetBufferTimeout() time.Duration {
	d, _ := time.ParseDuration(*c.BufferTimeout)
	return d
}

// GetNoiseRelative returns the noise_relative value.
func (c *TuningConfig) GetNoiseRelative() float64 { return *c.NoiseRelative }

// GetSeedFromFirst returns the seed_from_first value.
func (c *TuningConfig) GetSeedFromFirst() bool { return *c.SeedFromFirst }

// GetMinFramePoints returns the min_frame_points value.
func (c *TuningConfig) GetMinFramePoints() int { return *c.MinFramePoints }

// GetBackgroundFlush returns the background_flush value.
func (c *TuningConfig) GetBackgroundFlush() bool { return *c.BackgroundFlush }

// GetClosenessMultiplier returns the closeness_multiplier value.
func (c *TuningConfig) GetClosenessMultiplier() float64 { return *c.ClosenessMultiplier }

// GetNeighborConfirmationCount returns the neighbor_confirmation_count value.
func (c *TuningConfig) GetNeighborConfirmationCount() int { return *c.NeighborConfirmationCount }

// GetWarmupDurationNanos returns the warmup_duration_nanos value.
func (c *TuningConfig) GetWarmupDurationNanos() int64 { return *c.WarmupDurationNanos }

// GetWarmupMinFrames returns the warmup_min_frames value.
func (c *TuningConfig) GetWarmupMinFrames() int { return *c.WarmupMinFrames }

// GetPostSettleUpdateFraction returns the post_settle_update_fraction value.
func (c *TuningConfig) GetPostSettleUpdateFraction() float64 { return *c.PostSettleUpdateFraction }

// GetForegroundDBSCANEps returns the foreground_dbscan_eps value.
func (c *TuningConfig) GetForegroundDBSCANEps() float64 { return *c.ForegroundDBSCANEps }

// GetForegroundMinClusterPoints returns the foreground_min_cluster_points value.
func (c *TuningConfig) GetForegroundMinClusterPoints() int { return *c.ForegroundMinClusterPoints }

// GetForegroundMaxInputPoints returns the foreground_max_input_points value.
func (c *TuningConfig) GetForegroundMaxInputPoints() int { return *c.ForegroundMaxInputPoints }

// GetGatingDistanceSquared returns the gating_distance_squared value.
func (c *TuningConfig) GetGatingDistanceSquared() float64 { return *c.GatingDistanceSquared }

// GetProcessNoisePos returns the process_noise_pos value (dt-normalised).
func (c *TuningConfig) GetProcessNoisePos() float64 { return *c.ProcessNoisePos }

// GetProcessNoiseVel returns the process_noise_vel value (dt-normalised).
func (c *TuningConfig) GetProcessNoiseVel() float64 { return *c.ProcessNoiseVel }

// GetMeasurementNoise returns the measurement_noise value.
func (c *TuningConfig) GetMeasurementNoise() float64 { return *c.MeasurementNoise }

// GetOcclusionCovInflation returns the occlusion_cov_inflation value.
func (c *TuningConfig) GetOcclusionCovInflation() float64 { return *c.OcclusionCovInflation }

// GetHitsToConfirm returns the hits_to_confirm value.
func (c *TuningConfig) GetHitsToConfirm() int { return *c.HitsToConfirm }

// GetMaxMisses returns the max_misses value.
func (c *TuningConfig) GetMaxMisses() int { return *c.MaxMisses }

// GetMaxMissesConfirmed returns the max_misses_confirmed value.
func (c *TuningConfig) GetMaxMissesConfirmed() int { return *c.MaxMissesConfirmed }

// GetMaxTracks returns the max_tracks value.
func (c *TuningConfig) GetMaxTracks() int { return *c.MaxTracks }

// GetBackgroundUpdateFraction returns the background_update_fraction value.
func (c *TuningConfig) GetBackgroundUpdateFraction() float64 { return *c.BackgroundUpdateFraction }

// GetSafetyMarginMeters returns the safety_margin_meters value.
func (c *TuningConfig) GetSafetyMarginMeters() float64 { return *c.SafetyMarginMeters }

// GetEnableDiagnostics returns the enable_diagnostics value.
func (c *TuningConfig) GetEnableDiagnostics() bool { return *c.EnableDiagnostics }

// GetHeightBandFloor returns the height_band_floor value (metres, sensor-frame Z).
func (c *TuningConfig) GetHeightBandFloor() float64 { return *c.HeightBandFloor }

// GetHeightBandCeiling returns the height_band_ceiling value (metres, sensor-frame Z).
func (c *TuningConfig) GetHeightBandCeiling() float64 { return *c.HeightBandCeiling }

// GetRemoveGround returns the remove_ground value.
func (c *TuningConfig) GetRemoveGround() bool { return *c.RemoveGround }

// GetMaxClusterDiameter returns the max_cluster_diameter value (metres).
func (c *TuningConfig) GetMaxClusterDiameter() float64 { return *c.MaxClusterDiameter }

// GetMinClusterDiameter returns the min_cluster_diameter value (metres).
func (c *TuningConfig) GetMinClusterDiameter() float64 { return *c.MinClusterDiameter }

// GetMaxClusterAspectRatio returns the max_cluster_aspect_ratio value.
func (c *TuningConfig) GetMaxClusterAspectRatio() float64 { return *c.MaxClusterAspectRatio }

// GetMaxReasonableSpeedMps returns the max_reasonable_speed_mps value (m/s).
func (c *TuningConfig) GetMaxReasonableSpeedMps() float64 { return *c.MaxReasonableSpeedMps }

// GetMaxPositionJumpMeters returns the max_position_jump_meters value (metres).
func (c *TuningConfig) GetMaxPositionJumpMeters() float64 { return *c.MaxPositionJumpMeters }

// GetMaxPredictDt returns the max_predict_dt value (seconds).
func (c *TuningConfig) GetMaxPredictDt() float64 { return *c.MaxPredictDt }

// GetMaxCovarianceDiag returns the max_covariance_diag value.
func (c *TuningConfig) GetMaxCovarianceDiag() float64 { return *c.MaxCovarianceDiag }

// GetMinPointsForPCA returns the min_points_for_pca value.
func (c *TuningConfig) GetMinPointsForPCA() int { return *c.MinPointsForPCA }

// GetOBBHeadingSmoothingAlpha returns the obb_heading_smoothing_alpha value.
func (c *TuningConfig) GetOBBHeadingSmoothingAlpha() float64 { return *c.OBBHeadingSmoothingAlpha }

// GetOBBAspectRatioLockThreshold returns the obb_aspect_ratio_lock_threshold value.
func (c *TuningConfig) GetOBBAspectRatioLockThreshold() float64 {
	return *c.OBBAspectRatioLockThreshold
}

// GetMaxTrackHistoryLength returns the max_track_history_length value.
func (c *TuningConfig) GetMaxTrackHistoryLength() int { return *c.MaxTrackHistoryLength }

// GetMaxSpeedHistoryLength returns the max_speed_history_length value.
func (c *TuningConfig) GetMaxSpeedHistoryLength() int { return *c.MaxSpeedHistoryLength }

// GetMergeSizeRatio returns the merge_size_ratio value.
func (c *TuningConfig) GetMergeSizeRatio() float64 { return *c.MergeSizeRatio }

// GetSplitSizeRatio returns the split_size_ratio value.
func (c *TuningConfig) GetSplitSizeRatio() float64 { return *c.SplitSizeRatio }

// GetDeletedTrackGracePeriod parses and returns the deleted_track_grace_period as a time.Duration.
func (c *TuningConfig) GetDeletedTrackGracePeriod() time.Duration {
	d, _ := time.ParseDuration(*c.DeletedTrackGracePeriod)
	return d
}

// GetMinObservationsForClassification returns the min_observations_for_classification value.
func (c *TuningConfig) GetMinObservationsForClassification() int {
	return *c.MinObservationsForClassification
}
