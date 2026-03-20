package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaultsFile(t *testing.T) {
	cfg := MustLoadDefaultConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if err := cfg.ValidateComplete(); err != nil {
		t.Fatalf("defaults must validate via compatibility entrypoint: %v", err)
	}

	if cfg.Version != CurrentConfigVersion {
		t.Fatalf("expected version %d, got %d", CurrentConfigVersion, cfg.Version)
	}
	if cfg.L3.Engine != "ema_baseline_v1" || cfg.L4.Engine != "dbscan_xy_v1" || cfg.L5.Engine != "cv_kf_v1" {
		t.Fatalf("unexpected active engines: l3=%s l4=%s l5=%s", cfg.L3.Engine, cfg.L4.Engine, cfg.L5.Engine)
	}

	if cfg.GetSensor() == "" {
		t.Fatal("GetSensor returned empty value")
	}
	if cfg.GetUDPPort() <= 0 {
		t.Fatalf("GetUDPPort must be positive, got %d", cfg.GetUDPPort())
	}
	if cfg.GetNoiseRelative() <= 0 || cfg.GetNoiseRelative() > 1 {
		t.Fatalf("GetNoiseRelative out of range: %f", cfg.GetNoiseRelative())
	}
	if cfg.GetFreezeDuration() <= 0 {
		t.Fatalf("GetFreezeDuration must be positive, got %v", cfg.GetFreezeDuration())
	}
	if cfg.GetSettlingPeriod() <= 0 {
		t.Fatalf("GetSettlingPeriod must be positive, got %v", cfg.GetSettlingPeriod())
	}
	if cfg.GetForegroundMaxInputPoints() < 1 {
		t.Fatalf("GetForegroundMaxInputPoints must be positive, got %d", cfg.GetForegroundMaxInputPoints())
	}
}

func TestLoadTuningConfigRoundTrip(t *testing.T) {
	path := writeConfigFile(t, mustMarshalJSON(t, sampleValidConfig()))

	cfg, err := LoadTuningConfig(path)
	if err != nil {
		t.Fatalf("LoadTuningConfig returned error: %v", err)
	}

	if cfg.GetSensor() != "hesai-pandar40p" {
		t.Fatalf("unexpected sensor: %q", cfg.GetSensor())
	}
	if cfg.GetFreezeDuration() != 5*time.Second {
		t.Fatalf("unexpected freeze duration: %v", cfg.GetFreezeDuration())
	}
	if cfg.GetSettlingPeriod() != 5*time.Minute {
		t.Fatalf("unexpected settling period: %v", cfg.GetSettlingPeriod())
	}
	if cfg.GetDeletedTrackGracePeriod() != 5*time.Second {
		t.Fatalf("unexpected deleted track grace period: %v", cfg.GetDeletedTrackGracePeriod())
	}
}

func TestLoadTuningConfigRejectsFlatSchema(t *testing.T) {
	path := writeConfigFile(t, []byte(`{
  "noise_relative": 0.02,
  "closeness_multiplier": 3.0,
  "buffer_timeout": "500ms"
}`))

	_, err := LoadTuningConfig(path)
	if err == nil {
		t.Fatal("expected flat schema to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown keys") && !strings.Contains(err.Error(), "missing required keys") {
		t.Fatalf("expected strict schema error, got: %v", err)
	}
}

func TestLoadTuningConfigRejectsUnknownTopLevelKey(t *testing.T) {
	cfg := sampleValidConfig()
	raw := mustMarshalJSON(t, cfg)

	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal sample config: %v", err)
	}
	object["extra"] = true

	path := writeConfigFile(t, mustMarshalJSON(t, object))
	_, err := LoadTuningConfig(path)
	if err == nil {
		t.Fatal("expected unknown top-level key to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown keys: extra") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadTuningConfigRejectsMissingSelectedEngineBlock(t *testing.T) {
	path := writeConfigFile(t, []byte(`{
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
    "engine": "ema_baseline_v1"
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
}`))

	_, err := LoadTuningConfig(path)
	if err == nil {
		t.Fatal("expected missing selected engine block to be rejected")
	}
	if !strings.Contains(err.Error(), `selected engine block "ema_baseline_v1" missing`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadTuningConfigRejectsNonSelectedEngineBlock(t *testing.T) {
	cfg := sampleValidConfig()
	cfg.L3.EmaTrackAssistV2 = &L3EmaTrackAssistV2{
		L3Common: sampleValidConfig().L3.EmaBaselineV1.L3Common,
	}

	path := writeConfigFile(t, mustMarshalJSON(t, cfg))
	_, err := LoadTuningConfig(path)
	if err == nil {
		t.Fatal("expected non-selected engine block to be rejected")
	}
	if !strings.Contains(err.Error(), `non-selected engine block "ema_track_assist_v2" present`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadTuningConfigRejectsLegacySpellings(t *testing.T) {
	path := writeConfigFile(t, []byte(`{
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
      "safety_margin_meters": 0.15,
      "noise_relative": 0.02,
      "neighbor_confirmation_count": 3,
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
    }
  },
  "pipeline": {
    "buffer_timeout": "500ms",
    "min_frame_points": 1000,
    "flush_interval": "60s",
    "background_flush": false
  }
}`))

	_, err := LoadTuningConfig(path)
	if err == nil {
		t.Fatal("expected legacy spellings to be rejected")
	}
	if !strings.Contains(err.Error(), "safety_margin_meters") ||
		!strings.Contains(err.Error(), "neighbor_confirmation_count") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeConfigFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tuning.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return data
}

func sampleValidConfig() *TuningConfig {
	return &TuningConfig{
		Version: CurrentConfigVersion,
		L1: L1Config{
			Sensor:                "hesai-pandar40p",
			DataSource:            "live",
			UDPPort:               2369,
			UDPRcvBuf:             4194304,
			ForwardPort:           2368,
			ForegroundForwardPort: 2370,
		},
		L3: L3Config{
			Engine: "ema_baseline_v1",
			EmaBaselineV1: &L3EmaBaselineV1{
				L3Common: L3Common{
					BackgroundUpdateFraction:          0.02,
					ClosenessMultiplier:               3.0,
					SafetyMarginMetres:                0.15,
					NoiseRelative:                     0.02,
					NeighbourConfirmationCount:        3,
					SeedFromFirst:                     true,
					WarmupDurationNanos:               30 * int64(time.Second),
					WarmupMinFrames:                   100,
					PostSettleUpdateFraction:          0,
					EnableDiagnostics:                 false,
					FreezeDuration:                    "5s",
					FreezeThresholdMultiplier:         3.0,
					SettlingPeriod:                    "5m",
					SnapshotInterval:                  "2h",
					ChangeThresholdSnapshot:           100,
					ReacquisitionBoostMultiplier:      5.0,
					MinConfidenceFloor:                3,
					LockedBaselineThreshold:           50,
					LockedBaselineMultiplier:          4.0,
					SensorMovementForegroundThreshold: 0.2,
					BackgroundDriftThresholdMetres:    0.5,
					BackgroundDriftRatioThreshold:     0.1,
					SettlingMinCoverage:               0.8,
					SettlingMaxSpreadDelta:            0.001,
					SettlingMinRegionStability:        0.95,
					SettlingMinConfidence:             10.0,
				},
			},
		},
		L4: L4Config{
			Engine: "dbscan_xy_v1",
			DbscanXyV1: &L4DbscanXyV1{
				L4Common: L4Common{
					ForegroundDBSCANEps:        0.8,
					ForegroundMinClusterPoints: 5,
					ForegroundMaxInputPoints:   8000,
					HeightBandFloor:            -2.8,
					HeightBandCeiling:          1.5,
					RemoveGround:               true,
					MaxClusterDiameter:         12.0,
					MinClusterDiameter:         0.05,
					MaxClusterAspectRatio:      15.0,
				},
			},
		},
		L5: L5Config{
			Engine: "cv_kf_v1",
			CvKfV1: &L5CvKfV1{
				L5Common: L5Common{
					GatingDistanceSquared:            36.0,
					ProcessNoisePos:                  0.05,
					ProcessNoiseVel:                  0.2,
					MeasurementNoise:                 0.05,
					OcclusionCovInflation:            0.5,
					HitsToConfirm:                    4,
					MaxMisses:                        3,
					MaxMissesConfirmed:               15,
					MaxTracks:                        100,
					MaxReasonableSpeedMps:            30.0,
					MaxPositionJumpMetres:            5.0,
					MaxPredictDt:                     0.5,
					MaxCovarianceDiag:                100.0,
					MinPointsForPCA:                  4,
					OBBHeadingSmoothingAlpha:         0.08,
					OBBAspectRatioLockThreshold:      0.25,
					MaxTrackHistoryLength:            200,
					MaxSpeedHistoryLength:            100,
					MergeSizeRatio:                   2.5,
					SplitSizeRatio:                   0.3,
					DeletedTrackGracePeriod:          "5s",
					MinObservationsForClassification: 5,
				},
			},
		},
		Pipeline: PipelineConfig{
			BufferTimeout:   "500ms",
			MinFramePoints:  1000,
			FlushInterval:   "60s",
			BackgroundFlush: false,
		},
	}
}
