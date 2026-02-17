package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadDefaultsFile verifies that the canonical defaults file loads correctly
// and that all fields are populated with values in valid ranges.
func TestLoadDefaultsFile(t *testing.T) {
	cfg := MustLoadDefaultConfig()

	// All tunable fields must be non-nil (populated from file).
	if cfg.NoiseRelative == nil {
		t.Fatal("NoiseRelative must be set")
	}
	if cfg.SeedFromFirst == nil {
		t.Fatal("SeedFromFirst must be set")
	}
	if cfg.BufferTimeout == nil {
		t.Fatal("BufferTimeout must be set")
	}
	if cfg.MinFramePoints == nil {
		t.Fatal("MinFramePoints must be set")
	}
	if cfg.FlushInterval == nil {
		t.Fatal("FlushInterval must be set")
	}
	if cfg.BackgroundFlush == nil {
		t.Fatal("BackgroundFlush must be set")
	}

	// Structural range checks.
	if *cfg.NoiseRelative < 0 || *cfg.NoiseRelative > 1 {
		t.Errorf("NoiseRelative must be in [0, 1], got %f", *cfg.NoiseRelative)
	}
	if *cfg.MinFramePoints < 0 {
		t.Errorf("MinFramePoints must be non-negative, got %d", *cfg.MinFramePoints)
	}
	if _, err := time.ParseDuration(*cfg.BufferTimeout); err != nil {
		t.Errorf("BufferTimeout must be a valid duration, got %q: %v", *cfg.BufferTimeout, err)
	}
	if _, err := time.ParseDuration(*cfg.FlushInterval); err != nil {
		t.Errorf("FlushInterval must be a valid duration, got %q: %v", *cfg.FlushInterval, err)
	}

	// Getter methods must return consistent values (non-zero where applicable).
	if cfg.GetNoiseRelative() < 0 || cfg.GetNoiseRelative() > 1 {
		t.Errorf("GetNoiseRelative() out of range: %f", cfg.GetNoiseRelative())
	}
	if cfg.GetMinFramePoints() < 0 {
		t.Errorf("GetMinFramePoints() must be non-negative: %d", cfg.GetMinFramePoints())
	}
	if cfg.GetFlushInterval() <= 0 {
		t.Errorf("GetFlushInterval() must be positive: %v", cfg.GetFlushInterval())
	}
	if cfg.GetBufferTimeout() <= 0 {
		t.Errorf("GetBufferTimeout() must be positive: %v", cfg.GetBufferTimeout())
	}

	// The full config must pass validation.
	if err := cfg.Validate(); err != nil {
		t.Errorf("defaults must pass Validate(): %v", err)
	}
	if err := cfg.ValidateComplete(); err != nil {
		t.Errorf("defaults must pass ValidateComplete(): %v", err)
	}
}

// TestEmptyTuningConfig verifies that EmptyTuningConfig returns all nil fields.
func TestEmptyTuningConfig(t *testing.T) {
	cfg := EmptyTuningConfig()

	// All fields should be nil
	if cfg.NoiseRelative != nil {
		t.Error("Expected NoiseRelative to be nil")
	}
	if cfg.SeedFromFirst != nil {
		t.Error("Expected SeedFromFirst to be nil")
	}
	if cfg.BufferTimeout != nil {
		t.Error("Expected BufferTimeout to be nil")
	}

	// ValidateComplete should fail on empty config
	if err := cfg.ValidateComplete(); err == nil {
		t.Error("Expected ValidateComplete to fail on empty config")
	}
}

// TestDefaultsFileComplete verifies that config/tuning.defaults.json has all fields.
// This ensures no field is accidentally omitted from the canonical defaults file.
func TestDefaultsFileComplete(t *testing.T) {
	cfg := MustLoadDefaultConfig()

	// Verify all 25 fields are non-nil (must match tuning.defaults.json field count)
	if cfg.BackgroundUpdateFraction == nil {
		t.Error("BackgroundUpdateFraction should have default value")
	}
	if cfg.ClosenessMultiplier == nil {
		t.Error("ClosenessMultiplier should have default value")
	}
	if cfg.SafetyMarginMeters == nil {
		t.Error("SafetyMarginMeters should have default value")
	}
	if cfg.NoiseRelative == nil {
		t.Error("NoiseRelative should have default value")
	}
	if cfg.NeighborConfirmationCount == nil {
		t.Error("NeighborConfirmationCount should have default value")
	}
	if cfg.SeedFromFirst == nil {
		t.Error("SeedFromFirst should have default value")
	}
	if cfg.WarmupDurationNanos == nil {
		t.Error("WarmupDurationNanos should have default value")
	}
	if cfg.WarmupMinFrames == nil {
		t.Error("WarmupMinFrames should have default value")
	}
	if cfg.PostSettleUpdateFraction == nil {
		t.Error("PostSettleUpdateFraction should have default value")
	}
	if cfg.ForegroundMinClusterPoints == nil {
		t.Error("ForegroundMinClusterPoints should have default value")
	}
	if cfg.ForegroundDBSCANEps == nil {
		t.Error("ForegroundDBSCANEps should have default value")
	}
	if cfg.BufferTimeout == nil {
		t.Error("BufferTimeout should have default value")
	}
	if cfg.MinFramePoints == nil {
		t.Error("MinFramePoints should have default value")
	}
	if cfg.FlushInterval == nil {
		t.Error("FlushInterval should have default value")
	}
	if cfg.BackgroundFlush == nil {
		t.Error("BackgroundFlush should have default value")
	}
	if cfg.GatingDistanceSquared == nil {
		t.Error("GatingDistanceSquared should have default value")
	}
	if cfg.ProcessNoisePos == nil {
		t.Error("ProcessNoisePos should have default value")
	}
	if cfg.ProcessNoiseVel == nil {
		t.Error("ProcessNoiseVel should have default value")
	}
	if cfg.MeasurementNoise == nil {
		t.Error("MeasurementNoise should have default value")
	}
	if cfg.OcclusionCovInflation == nil {
		t.Error("OcclusionCovInflation should have default value")
	}
	if cfg.HitsToConfirm == nil {
		t.Error("HitsToConfirm should have default value")
	}
	if cfg.MaxMisses == nil {
		t.Error("MaxMisses should have default value")
	}
	if cfg.MaxMissesConfirmed == nil {
		t.Error("MaxMissesConfirmed should have default value")
	}
	if cfg.MaxTracks == nil {
		t.Error("MaxTracks should have default value")
	}
	if cfg.EnableDiagnostics == nil {
		t.Error("EnableDiagnostics should have default value")
	}

	// Verify values are within structurally valid ranges (not hardcoded to
	// specific numbers so the test is immune to tuning adjustments).
	if *cfg.ClosenessMultiplier <= 0 {
		t.Errorf("ClosenessMultiplier must be positive, got %v", *cfg.ClosenessMultiplier)
	}
	if *cfg.NeighborConfirmationCount < 0 || *cfg.NeighborConfirmationCount > 8 {
		t.Errorf("NeighborConfirmationCount must be in [0, 8], got %v", *cfg.NeighborConfirmationCount)
	}
	if *cfg.WarmupDurationNanos <= 0 {
		t.Errorf("WarmupDurationNanos must be positive, got %v", *cfg.WarmupDurationNanos)
	}
	if *cfg.WarmupMinFrames < 0 {
		t.Errorf("WarmupMinFrames must be non-negative, got %v", *cfg.WarmupMinFrames)
	}
	if *cfg.GatingDistanceSquared <= 0 {
		t.Errorf("GatingDistanceSquared must be positive, got %v", *cfg.GatingDistanceSquared)
	}
	if *cfg.ProcessNoisePos <= 0 {
		t.Errorf("ProcessNoisePos must be positive, got %v", *cfg.ProcessNoisePos)
	}
	if *cfg.ProcessNoiseVel <= 0 {
		t.Errorf("ProcessNoiseVel must be positive, got %v", *cfg.ProcessNoiseVel)
	}
	if *cfg.MeasurementNoise <= 0 {
		t.Errorf("MeasurementNoise must be positive, got %v", *cfg.MeasurementNoise)
	}
	if *cfg.OcclusionCovInflation <= 0 {
		t.Errorf("OcclusionCovInflation must be positive, got %v", *cfg.OcclusionCovInflation)
	}
	if *cfg.HitsToConfirm < 1 {
		t.Errorf("HitsToConfirm must be >= 1, got %v", *cfg.HitsToConfirm)
	}
	if *cfg.MaxMisses < 1 {
		t.Errorf("MaxMisses must be >= 1, got %v", *cfg.MaxMisses)
	}
	if *cfg.MaxMissesConfirmed < 1 {
		t.Errorf("MaxMissesConfirmed must be >= 1, got %v", *cfg.MaxMissesConfirmed)
	}

	// Full validation must pass.
	if err := cfg.Validate(); err != nil {
		t.Errorf("defaults must pass Validate(): %v", err)
	}
	if err := cfg.ValidateComplete(); err != nil {
		t.Errorf("defaults must pass ValidateComplete(): %v", err)
	}
}

func TestLoadTuningConfig(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.json")

	// Write test config with all required keys
	testJSON := `{
  "background_update_fraction": 0.02,
  "closeness_multiplier": 8.0,
  "safety_margin_meters": 0.4,
  "noise_relative": 0.05,
  "neighbor_confirmation_count": 7,
  "seed_from_first": false,
  "warmup_duration_nanos": 30000000000,
  "warmup_min_frames": 100,
  "post_settle_update_fraction": 0,
  "enable_diagnostics": false,
  "foreground_dbscan_eps": 0.8,
  "foreground_min_cluster_points": 5,
  "buffer_timeout": "250ms",
  "min_frame_points": 500,
  "flush_interval": "120s",
  "background_flush": true,
  "gating_distance_squared": 36.0,
  "process_noise_pos": 1.0,
  "process_noise_vel": 5.0,
  "measurement_noise": 0.3,
  "occlusion_cov_inflation": 0.5,
  "hits_to_confirm": 3,
  "max_misses": 3,
  "max_misses_confirmed": 15,
  "max_tracks": 100,
  "height_band_floor": -2.8,
  "height_band_ceiling": 1.5,
  "remove_ground": true,
  "max_cluster_diameter": 12.0,
  "min_cluster_diameter": 0.05,
  "max_cluster_aspect_ratio": 15.0,
  "max_reasonable_speed_mps": 30.0,
  "max_position_jump_meters": 5.0,
  "max_predict_dt": 0.5,
  "max_covariance_diag": 100.0,
  "min_points_for_pca": 4,
  "obb_heading_smoothing_alpha": 0.08,
  "obb_aspect_ratio_lock_threshold": 0.25,
  "max_track_history_length": 200,
  "max_speed_history_length": 100,
  "merge_size_ratio": 2.5,
  "split_size_ratio": 0.3,
  "deleted_track_grace_period": "5s",
  "min_observations_for_classification": 5
}`
	if err := os.WriteFile(configPath, []byte(testJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load the config
	cfg, err := LoadTuningConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values
	if cfg.NoiseRelative == nil || *cfg.NoiseRelative != 0.05 {
		t.Errorf("Expected NoiseRelative 0.05, got %v", cfg.NoiseRelative)
	}
	if cfg.SeedFromFirst == nil || *cfg.SeedFromFirst != false {
		t.Errorf("Expected SeedFromFirst false, got %v", cfg.SeedFromFirst)
	}
	if cfg.BufferTimeout == nil || *cfg.BufferTimeout != "250ms" {
		t.Errorf("Expected BufferTimeout '250ms', got %v", cfg.BufferTimeout)
	}
	if cfg.MinFramePoints == nil || *cfg.MinFramePoints != 500 {
		t.Errorf("Expected MinFramePoints 500, got %v", cfg.MinFramePoints)
	}
	if cfg.FlushInterval == nil || *cfg.FlushInterval != "120s" {
		t.Errorf("Expected FlushInterval '120s', got %v", cfg.FlushInterval)
	}
	if cfg.BackgroundFlush == nil || *cfg.BackgroundFlush != true {
		t.Errorf("Expected BackgroundFlush true, got %v", cfg.BackgroundFlush)
	}
}

func TestLoadTuningConfigMissing(t *testing.T) {
	_, err := LoadTuningConfig("/nonexistent/path/to/config.json")
	if err == nil {
		t.Error("Expected error when loading missing file, got nil")
	}
}

func TestLoadTuningConfigInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_config.json")

	// Write invalid JSON
	invalidJSON := `{
  "noise_relative": "invalid"
`
	if err := os.WriteFile(configPath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadTuningConfig(configPath)
	if err == nil {
		t.Error("Expected error when loading invalid JSON, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *TuningConfig
		wantErr bool
	}{
		{
			name:    "valid config from defaults file",
			cfg:     MustLoadDefaultConfig(),
			wantErr: false,
		},
		{
			name:    "empty config is valid",
			cfg:     &TuningConfig{},
			wantErr: false,
		},
		{
			name: "invalid noise relative (too low)",
			cfg: &TuningConfig{
				NoiseRelative: ptrFloat64(-0.1),
			},
			wantErr: true,
		},
		{
			name: "invalid noise relative (too high)",
			cfg: &TuningConfig{
				NoiseRelative: ptrFloat64(1.5),
			},
			wantErr: true,
		},
		{
			name: "invalid flush interval",
			cfg: &TuningConfig{
				FlushInterval: ptrString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid buffer timeout",
			cfg: &TuningConfig{
				BufferTimeout: ptrString("invalid"),
			},
			wantErr: true,
		},
		{
			name: "negative min frame points",
			cfg: &TuningConfig{
				MinFramePoints: ptrInt(-1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetFlushInterval(t *testing.T) {
	tests := []struct {
		name string
		cfg  *TuningConfig
		want time.Duration
	}{
		{
			name: "60 seconds",
			cfg: &TuningConfig{
				FlushInterval: ptrString("60s"),
			},
			want: 60 * time.Second,
		},
		{
			name: "2 minutes",
			cfg: &TuningConfig{
				FlushInterval: ptrString("2m"),
			},
			want: 2 * time.Minute,
		},
		{
			name: "1 hour",
			cfg: &TuningConfig{
				FlushInterval: ptrString("1h"),
			},
			want: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.GetFlushInterval()
			if got != tt.want {
				t.Errorf("GetFlushInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBufferTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  *TuningConfig
		want time.Duration
	}{
		{
			name: "500 milliseconds",
			cfg: &TuningConfig{
				BufferTimeout: ptrString("500ms"),
			},
			want: 500 * time.Millisecond,
		},
		{
			name: "1 second",
			cfg: &TuningConfig{
				BufferTimeout: ptrString("1s"),
			},
			want: 1 * time.Second,
		},
		{
			name: "250 milliseconds",
			cfg: &TuningConfig{
				BufferTimeout: ptrString("250ms"),
			},
			want: 250 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.GetBufferTimeout()
			if got != tt.want {
				t.Errorf("GetBufferTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadDefaultConfigFile(t *testing.T) {
	cfg, err := LoadTuningConfig("../../config/tuning.defaults.json")
	if err != nil {
		t.Fatalf("Failed to load defaults: %v", err)
	}
	// Structural: noise_relative is within valid range.
	if cfg.GetNoiseRelative() < 0 || cfg.GetNoiseRelative() > 1 {
		t.Errorf("NoiseRelative out of range [0,1]: %f", cfg.GetNoiseRelative())
	}
	// Structural: file must pass full validation.
	if err := cfg.Validate(); err != nil {
		t.Errorf("defaults must pass Validate(): %v", err)
	}
	if err := cfg.ValidateComplete(); err != nil {
		t.Errorf("defaults must pass ValidateComplete(): %v", err)
	}
}

func TestLoadExampleConfigFile(t *testing.T) {
	cfg, err := LoadTuningConfig("../../config/tuning.example.json")
	if err != nil {
		t.Fatalf("Failed to load example: %v", err)
	}
	if cfg.GetNoiseRelative() != 0.06 {
		t.Errorf("Expected 0.06, got %f", cfg.GetNoiseRelative())
	}
	if cfg.GetMinFramePoints() != 500 {
		t.Errorf("Expected 500, got %d", cfg.GetMinFramePoints())
	}
}

func TestLoadTuningConfigPartial(t *testing.T) {
	// Partial configs are now rejected — all keys must be present.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.json")

	partialJSON := `{
  "noise_relative": 0.08
}`
	if err := os.WriteFile(configPath, []byte(partialJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadTuningConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for partial config (missing required keys), got nil")
	}
	if !strings.Contains(err.Error(), "missing required") {
		t.Errorf("Expected 'missing required' in error, got: %v", err)
	}
}

func TestLoadTuningConfigRejectsPathTraversal(t *testing.T) {
	// Path traversal with ".." is allowed since this is a CLI-only flag,
	// but the file must still have a .json extension.
	_, err := LoadTuningConfig("../../etc/passwd")
	if err == nil {
		t.Error("Expected error for non-.json path, got nil")
	}
}

func TestLoadTuningConfigRejectsNonJSON(t *testing.T) {
	_, err := LoadTuningConfig("/some/path/config.yaml")
	if err == nil {
		t.Error("Expected error for non-.json extension, got nil")
	}
}

func TestLoadTuningConfigRejectsLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "large.json")

	// Create a file larger than 1MB
	largeData := make([]byte, 2*1024*1024) // 2MB
	if err := os.WriteFile(configPath, largeData, 0644); err != nil {
		t.Fatalf("Failed to write large file: %v", err)
	}

	_, err := LoadTuningConfig(configPath)
	if err == nil {
		t.Error("Expected error for file size > 1MB, got nil")
	}
}

func TestAllTuningParams(t *testing.T) {
	// Test that all tunable parameters can be set via JSON
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "all_params.json")

	allParamsJSON := `{
  "background_update_fraction": 0.03,
  "closeness_multiplier": 2.5,
  "safety_margin_meters": 0.5,
  "noise_relative": 0.05,
  "neighbor_confirmation_count": 3,
  "seed_from_first": false,
  "warmup_duration_nanos": 5000000000,
  "warmup_min_frames": 50,
  "post_settle_update_fraction": 0.1,
  "enable_diagnostics": true,
  "foreground_min_cluster_points": 10,
  "foreground_dbscan_eps": 0.5,
  "buffer_timeout": "250ms",
  "min_frame_points": 500,
  "flush_interval": "120s",
  "background_flush": true,
  "gating_distance_squared": 100.0,
  "process_noise_pos": 0.1,
  "process_noise_vel": 0.05,
  "measurement_noise": 0.2,
  "occlusion_cov_inflation": 2.0,
  "hits_to_confirm": 3,
  "max_misses": 5,
  "max_misses_confirmed": 10,
  "max_tracks": 200,
  "height_band_floor": -3.5,
  "height_band_ceiling": 2.0,
  "remove_ground": true,
  "max_cluster_diameter": 12.0,
  "min_cluster_diameter": 0.05,
  "max_cluster_aspect_ratio": 15.0,
  "max_reasonable_speed_mps": 30.0,
  "max_position_jump_meters": 5.0,
  "max_predict_dt": 0.5,
  "max_covariance_diag": 100.0,
  "min_points_for_pca": 4,
  "obb_heading_smoothing_alpha": 0.08,
  "obb_aspect_ratio_lock_threshold": 0.25,
  "max_track_history_length": 200,
  "max_speed_history_length": 100,
  "merge_size_ratio": 2.5,
  "split_size_ratio": 0.3,
  "deleted_track_grace_period": "5s",
  "min_observations_for_classification": 5
}`
	if err := os.WriteFile(configPath, []byte(allParamsJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadTuningConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify all fields are loaded correctly
	if cfg.NoiseRelative == nil || *cfg.NoiseRelative != 0.05 {
		t.Errorf("NoiseRelative = %v, want 0.05", cfg.NoiseRelative)
	}
	if cfg.ClosenessMultiplier == nil || *cfg.ClosenessMultiplier != 2.5 {
		t.Errorf("ClosenessMultiplier = %v, want 2.5", cfg.ClosenessMultiplier)
	}
	if cfg.NeighborConfirmationCount == nil || *cfg.NeighborConfirmationCount != 3 {
		t.Errorf("NeighborConfirmationCount = %v, want 3", cfg.NeighborConfirmationCount)
	}
	if cfg.SeedFromFirst == nil || *cfg.SeedFromFirst != false {
		t.Errorf("SeedFromFirst = %v, want false", cfg.SeedFromFirst)
	}
	if cfg.WarmupDurationNanos == nil || *cfg.WarmupDurationNanos != 5000000000 {
		t.Errorf("WarmupDurationNanos = %v, want 5000000000", cfg.WarmupDurationNanos)
	}
	if cfg.WarmupMinFrames == nil || *cfg.WarmupMinFrames != 50 {
		t.Errorf("WarmupMinFrames = %v, want 50", cfg.WarmupMinFrames)
	}
	if cfg.PostSettleUpdateFraction == nil || *cfg.PostSettleUpdateFraction != 0.1 {
		t.Errorf("PostSettleUpdateFraction = %v, want 0.1", cfg.PostSettleUpdateFraction)
	}
	if cfg.ForegroundMinClusterPoints == nil || *cfg.ForegroundMinClusterPoints != 10 {
		t.Errorf("ForegroundMinClusterPoints = %v, want 10", cfg.ForegroundMinClusterPoints)
	}
	if cfg.ForegroundDBSCANEps == nil || *cfg.ForegroundDBSCANEps != 0.5 {
		t.Errorf("ForegroundDBSCANEps = %v, want 0.5", cfg.ForegroundDBSCANEps)
	}
	if cfg.BufferTimeout == nil || *cfg.BufferTimeout != "250ms" {
		t.Errorf("BufferTimeout = %v, want '250ms'", cfg.BufferTimeout)
	}
	if cfg.MinFramePoints == nil || *cfg.MinFramePoints != 500 {
		t.Errorf("MinFramePoints = %v, want 500", cfg.MinFramePoints)
	}
	if cfg.FlushInterval == nil || *cfg.FlushInterval != "120s" {
		t.Errorf("FlushInterval = %v, want '120s'", cfg.FlushInterval)
	}
	if cfg.BackgroundFlush == nil || *cfg.BackgroundFlush != true {
		t.Errorf("BackgroundFlush = %v, want true", cfg.BackgroundFlush)
	}
	if cfg.GatingDistanceSquared == nil || *cfg.GatingDistanceSquared != 100.0 {
		t.Errorf("GatingDistanceSquared = %v, want 100.0", cfg.GatingDistanceSquared)
	}
	if cfg.ProcessNoisePos == nil || *cfg.ProcessNoisePos != 0.1 {
		t.Errorf("ProcessNoisePos = %v, want 0.1", cfg.ProcessNoisePos)
	}
	if cfg.ProcessNoiseVel == nil || *cfg.ProcessNoiseVel != 0.05 {
		t.Errorf("ProcessNoiseVel = %v, want 0.05", cfg.ProcessNoiseVel)
	}
	if cfg.MeasurementNoise == nil || *cfg.MeasurementNoise != 0.2 {
		t.Errorf("MeasurementNoise = %v, want 0.2", cfg.MeasurementNoise)
	}
	if cfg.OcclusionCovInflation == nil || *cfg.OcclusionCovInflation != 2.0 {
		t.Errorf("OcclusionCovInflation = %v, want 2.0", cfg.OcclusionCovInflation)
	}
	if cfg.HitsToConfirm == nil || *cfg.HitsToConfirm != 3 {
		t.Errorf("HitsToConfirm = %v, want 3", cfg.HitsToConfirm)
	}
	if cfg.MaxMisses == nil || *cfg.MaxMisses != 5 {
		t.Errorf("MaxMisses = %v, want 5", cfg.MaxMisses)
	}
	if cfg.MaxMissesConfirmed == nil || *cfg.MaxMissesConfirmed != 10 {
		t.Errorf("MaxMissesConfirmed = %v, want 10", cfg.MaxMissesConfirmed)
	}
	if cfg.MaxTracks == nil || *cfg.MaxTracks != 200 {
		t.Errorf("MaxTracks = %v, want 200", cfg.MaxTracks)
	}
	if cfg.BackgroundUpdateFraction == nil || *cfg.BackgroundUpdateFraction != 0.03 {
		t.Errorf("BackgroundUpdateFraction = %v, want 0.03", cfg.BackgroundUpdateFraction)
	}
	if cfg.SafetyMarginMeters == nil || *cfg.SafetyMarginMeters != 0.5 {
		t.Errorf("SafetyMarginMeters = %v, want 0.5", cfg.SafetyMarginMeters)
	}
	if cfg.EnableDiagnostics == nil || *cfg.EnableDiagnostics != true {
		t.Errorf("EnableDiagnostics = %v, want true", cfg.EnableDiagnostics)
	}
}

func TestGetterDefaults(t *testing.T) {
	// Test that getter methods return structurally valid values from the defaults.
	cfg := MustLoadDefaultConfig()

	if cfg.GetNoiseRelative() < 0 || cfg.GetNoiseRelative() > 1 {
		t.Errorf("GetNoiseRelative() out of range [0,1]: %f", cfg.GetNoiseRelative())
	}
	if cfg.GetMinFramePoints() < 0 {
		t.Errorf("GetMinFramePoints() must be non-negative: %d", cfg.GetMinFramePoints())
	}
	if cfg.GetFlushInterval() <= 0 {
		t.Errorf("GetFlushInterval() must be positive: %v", cfg.GetFlushInterval())
	}
	if cfg.GetBufferTimeout() <= 0 {
		t.Errorf("GetBufferTimeout() must be positive: %v", cfg.GetBufferTimeout())
	}
}
