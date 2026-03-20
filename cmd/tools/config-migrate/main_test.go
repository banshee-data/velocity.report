package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
)

func TestFilterLegacyMetadata(t *testing.T) {
	t.Parallel()

	filtered, err := filterLegacyMetadata([]byte(`{"_comment":"ignore","noise_relative":0.02}`))
	if err != nil {
		t.Fatalf("filterLegacyMetadata returned error: %v", err)
	}
	if strings.Contains(string(filtered), "_comment") {
		t.Fatalf("filtered output still contains private metadata: %s", filtered)
	}
	_, err = filterLegacyMetadata([]byte(`[`))
	requireErrorContains(t, err, "unexpected end of JSON input")
}

func TestMigrateLegacyConfig(t *testing.T) {
	t.Parallel()

	legacy := legacyTuningConfig{
		BackgroundUpdateFraction:         0.02,
		ClosenessMultiplier:              3.0,
		SafetyMarginMeters:               0.15,
		NoiseRelative:                    0.02,
		NeighborConfirmationCount:        3,
		SeedFromFirst:                    true,
		WarmupDurationNanos:              30_000_000_000,
		WarmupMinFrames:                  100,
		PostSettleUpdateFraction:         0.1,
		EnableDiagnostics:                true,
		ForegroundDBSCANEps:              0.8,
		ForegroundMinClusterPoints:       5,
		ForegroundMaxInputPoints:         8000,
		BufferTimeout:                    "500ms",
		MinFramePoints:                   1000,
		FlushInterval:                    "60s",
		BackgroundFlush:                  true,
		GatingDistanceSquared:            36,
		ProcessNoisePos:                  0.05,
		ProcessNoiseVel:                  0.2,
		MeasurementNoise:                 0.05,
		OcclusionCovInflation:            0.5,
		HitsToConfirm:                    4,
		MaxMisses:                        3,
		MaxMissesConfirmed:               15,
		MaxTracks:                        100,
		HeightBandFloor:                  -2.8,
		HeightBandCeiling:                1.5,
		RemoveGround:                     true,
		MaxClusterDiameter:               12,
		MinClusterDiameter:               0.05,
		MaxClusterAspectRatio:            15,
		MaxReasonableSpeedMps:            30,
		MaxPositionJumpMeters:            5,
		MaxPredictDt:                     0.5,
		MaxCovarianceDiag:                100,
		MinPointsForPCA:                  4,
		OBBHeadingSmoothingAlpha:         0.08,
		OBBAspectRatioLockThreshold:      0.25,
		MaxTrackHistoryLength:            200,
		MaxSpeedHistoryLength:            100,
		MergeSizeRatio:                   2.5,
		SplitSizeRatio:                   0.3,
		DeletedTrackGracePeriod:          "5s",
		MinObservationsForClassification: 5,
	}

	cfg := migrateLegacyConfig(legacy)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("migrated config failed validation: %v", err)
	}
	if cfg.Version != cfgpkg.CurrentConfigVersion ||
		cfg.L3.EmaBaselineV1.BackgroundUpdateFraction != legacy.BackgroundUpdateFraction ||
		cfg.L3.EmaBaselineV1.SafetyMarginMetres != legacy.SafetyMarginMeters ||
		cfg.L3.EmaBaselineV1.NeighbourConfirmationCount != legacy.NeighborConfirmationCount ||
		cfg.L4.DbscanXyV1.ForegroundDBSCANEps != legacy.ForegroundDBSCANEps ||
		cfg.L5.CvKfV1.MaxPositionJumpMetres != legacy.MaxPositionJumpMeters ||
		cfg.Pipeline.BufferTimeout != legacy.BufferTimeout {
		t.Fatal("legacy config fields were not mapped correctly")
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	validLegacy := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(validLegacy, []byte(`{
		"background_update_fraction": 0.02,
		"closeness_multiplier": 3.0,
		"safety_margin_meters": 0.15,
		"noise_relative": 0.02,
		"neighbor_confirmation_count": 3,
		"seed_from_first": true,
		"warmup_duration_nanos": 30000000000,
		"warmup_min_frames": 100,
		"post_settle_update_fraction": 0.1,
		"enable_diagnostics": false,
		"foreground_dbscan_eps": 0.8,
		"foreground_min_cluster_points": 5,
		"foreground_max_input_points": 8000,
		"buffer_timeout": "500ms",
		"min_frame_points": 1000,
		"flush_interval": "60s",
		"background_flush": false,
		"gating_distance_squared": 36.0,
		"process_noise_pos": 0.05,
		"process_noise_vel": 0.2,
		"measurement_noise": 0.05,
		"occlusion_cov_inflation": 0.5,
		"hits_to_confirm": 4,
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
	}`), 0o644); err != nil {
		t.Fatalf("write valid legacy config: %v", err)
	}

	t.Run("missing input flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(nil, &stdout, &stderr); code != 2 {
			t.Fatalf("run returned %d, want 2", code)
		}
		requireContains(t, stderr.String(), "--in is required")
	})

	t.Run("read error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "error: read")
	})

	t.Run("parse error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, []byte(`[`), 0o644); err != nil {
			t.Fatalf("write parse error fixture: %v", err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", path}, &stdout, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "error: parse")
	})

	t.Run("decode error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "decode.json")
		if err := os.WriteFile(path, []byte(`{"noise_relative":"bad"}`), 0o644); err != nil {
			t.Fatalf("write decode fixture: %v", err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", path}, &stdout, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "error: decode legacy config")
	})

	t.Run("validation error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
			t.Fatalf("write invalid fixture: %v", err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", path}, &stdout, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "migrated config failed validation")
	})

	t.Run("stdout success", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", validLegacy}, &stdout, &stderr); code != 0 {
			t.Fatalf("run returned %d, want 0", code)
		}
		var cfg cfgpkg.TuningConfig
		if err := json.Unmarshal(stdout.Bytes(), &cfg); err != nil {
			t.Fatalf("stdout did not contain valid config JSON: %v", err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("expected empty stderr, got %q", stderr.String())
		}
	})

	t.Run("stdout write error", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := run([]string{"-in", validLegacy}, errWriter{}, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "error: write stdout")
	})

	t.Run("file output success", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "nested.json")
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", validLegacy, "-out", outPath}, &stdout, &stderr); code != 0 {
			t.Fatalf("run returned %d, want 0", code)
		}
		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		var cfg cfgpkg.TuningConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("output file did not contain valid config JSON: %v", err)
		}
	})

	t.Run("file output error", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "missing", "nested.json")
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", validLegacy, "-out", outDir}, &stdout, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "error: write")
	})
}

type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func requireContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}

func requireErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("%q does not contain %q", err.Error(), want)
	}
}
