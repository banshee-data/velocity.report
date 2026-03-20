package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()

	validPath := filepath.Join(t.TempDir(), "valid.json")
	if err := os.WriteFile(validPath, []byte(`{
		"version": 2,
		"l1": {
			"sensor": "hesai-pandar40p",
			"data_source": "live",
			"udp_port": 2369,
			"udp_rcv_buf": 4194304,
			"forward_port": 2368,
			"foreground_forward_port": 2370
		},
		"l3": {
			"engine": "ema_baseline_v1",
			"ema_baseline_v1": {
				"background_update_fraction": 0.02,
				"closeness_multiplier": 3.0,
				"safety_margin_metres": 0.15,
				"noise_relative": 0.02,
				"neighbour_confirmation_count": 3,
				"seed_from_first": true,
				"warmup_duration_nanos": 30000000000,
				"warmup_min_frames": 100,
				"post_settle_update_fraction": 0,
				"enable_diagnostics": false,
				"freeze_duration": "5s",
				"freeze_threshold_multiplier": 3.0,
				"settling_period": "5m",
				"snapshot_interval": "2h",
				"change_threshold_snapshot": 100,
				"reacquisition_boost_multiplier": 5.0,
				"min_confidence_floor": 3,
				"locked_baseline_threshold": 50,
				"locked_baseline_multiplier": 4.0,
				"sensor_movement_foreground_threshold": 0.2,
				"background_drift_threshold_metres": 0.5,
				"background_drift_ratio_threshold": 0.1,
				"settling_min_coverage": 0.8,
				"settling_max_spread_delta": 0.001,
				"settling_min_region_stability": 0.95,
				"settling_min_confidence": 10.0
			}
		},
		"l4": {
			"engine": "dbscan_xy_v1",
			"dbscan_xy_v1": {
				"foreground_dbscan_eps": 0.8,
				"foreground_min_cluster_points": 5,
				"foreground_max_input_points": 8000,
				"height_band_floor": -2.8,
				"height_band_ceiling": 1.5,
				"remove_ground": true,
				"max_cluster_diameter": 12.0,
				"min_cluster_diameter": 0.05,
				"max_cluster_aspect_ratio": 15.0
			}
		},
		"l5": {
			"engine": "cv_kf_v1",
			"cv_kf_v1": {
				"gating_distance_squared": 36.0,
				"process_noise_pos": 0.05,
				"process_noise_vel": 0.2,
				"measurement_noise": 0.05,
				"occlusion_cov_inflation": 0.5,
				"hits_to_confirm": 4,
				"max_misses": 3,
				"max_misses_confirmed": 15,
				"max_tracks": 100,
				"max_reasonable_speed_mps": 30.0,
				"max_position_jump_metres": 5.0,
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
			}
		},
		"pipeline": {
			"buffer_timeout": "500ms",
			"min_frame_points": 1000,
			"flush_interval": "60s",
			"background_flush": false
		}
	}`), 0o644); err != nil {
		t.Fatalf("write valid config: %v", err)
	}

	t.Run("missing input flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(nil, &stdout, &stderr); code != 2 {
			t.Fatalf("run returned %d, want 2", code)
		}
		requireContains(t, stderr.String(), "--in is required")
	})

	t.Run("flag parse error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-bad-flag"}, &stdout, &stderr); code != 2 {
			t.Fatalf("run returned %d, want 2", code)
		}
		requireContains(t, stderr.String(), "flag provided but not defined")
	})

	t.Run("invalid config", func(t *testing.T) {
		invalidPath := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(invalidPath, []byte(`{}`), 0o644); err != nil {
			t.Fatalf("write invalid config: %v", err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", invalidPath}, &stdout, &stderr); code != 1 {
			t.Fatalf("run returned %d, want 1", code)
		}
		requireContains(t, stderr.String(), "invalid config:")
	})

	t.Run("success", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-in", validPath}, &stdout, &stderr); code != 0 {
			t.Fatalf("run returned %d, want 0", code)
		}
		requireContains(t, stdout.String(), "valid config:")
		requireContains(t, stdout.String(), "l3=ema_baseline_v1")
		if stderr.Len() != 0 {
			t.Fatalf("expected empty stderr, got %q", stderr.String())
		}
	})
}

func TestMain(t *testing.T) {
	originalExit := exit
	exit = func(code int) {
		panic(code)
	}
	defer func() { exit = originalExit }()

	oldArgs := os.Args
	os.Args = []string{"config-validate", "-bad-flag"}
	defer func() { os.Args = oldArgs }()

	defer func() {
		recovered := recover()
		code, ok := recovered.(int)
		if !ok || code != 2 {
			t.Fatalf("main panic = %#v, want exit code 2", recovered)
		}
	}()
	main()
}

func requireContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}
