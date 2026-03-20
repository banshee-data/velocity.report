package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEngineRegistryFactories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		layer string
		typ   interface{}
	}{
		{name: "ema_baseline_v1", layer: "l3", typ: &L3EmaBaselineV1{}},
		{name: "ema_track_assist_v2", layer: "l3", typ: &L3EmaTrackAssistV2{}},
		{name: "dbscan_xy_v1", layer: "l4", typ: &L4DbscanXyV1{}},
		{name: "two_stage_mahalanobis_v2", layer: "l4", typ: &L4TwoStageMahalanobisV2{}},
		{name: "hdbscan_adaptive_v1", layer: "l4", typ: &L4HdbscanAdaptiveV1{}},
		{name: "cv_kf_v1", layer: "l5", typ: &L5CvKfV1{}},
		{name: "imm_cv_ca_v2", layer: "l5", typ: &L5ImmCvCaV2{}},
		{name: "imm_cv_ca_rts_eval_v2", layer: "l5", typ: &L5ImmCvCaRtsEvalV2{}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := engineRegistry[tc.name]
			if !ok {
				t.Fatalf("engine %q missing from registry", tc.name)
			}
			if spec.Layer != tc.layer {
				t.Fatalf("engine %q layer = %q, want %q", tc.name, spec.Layer, tc.layer)
			}
			got := spec.NewConfig()
			if reflect.TypeOf(got) != reflect.TypeOf(tc.typ) {
				t.Fatalf("engine %q produced %T, want %T", tc.name, got, tc.typ)
			}
		})
	}
}

func TestTuningConfigValidateWrapsSubconfigErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*TuningConfig)
		wantText string
	}{
		{
			name: "version",
			mutate: func(cfg *TuningConfig) {
				cfg.Version = 99
			},
			wantText: "version must equal",
		},
		{
			name: "l1",
			mutate: func(cfg *TuningConfig) {
				cfg.L1.Sensor = ""
			},
			wantText: "l1: sensor must be non-empty",
		},
		{
			name: "l3",
			mutate: func(cfg *TuningConfig) {
				cfg.L3.EmaBaselineV1.BackgroundUpdateFraction = 0
			},
			wantText: "l3: background_update_fraction must be in (0, 1]",
		},
		{
			name: "l4",
			mutate: func(cfg *TuningConfig) {
				cfg.L4.DbscanXyV1.ForegroundDBSCANEps = 0
			},
			wantText: "l4: foreground_dbscan_eps must be positive",
		},
		{
			name: "l5",
			mutate: func(cfg *TuningConfig) {
				cfg.L5.CvKfV1.GatingDistanceSquared = 0
			},
			wantText: "l5: gating_distance_squared must be positive",
		},
		{
			name: "pipeline",
			mutate: func(cfg *TuningConfig) {
				cfg.Pipeline.BufferTimeout = "bad"
			},
			wantText: "pipeline: invalid buffer_timeout",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := sampleValidConfig()
			tc.mutate(cfg)
			err := cfg.Validate()
			requireErrorContains(t, err, tc.wantText)
		})
	}
}

func TestL1ConfigValidateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*L1Config)
		wantText string
	}{
		{
			name: "empty sensor",
			mutate: func(cfg *L1Config) {
				cfg.Sensor = " "
			},
			wantText: "sensor must be non-empty",
		},
		{
			name: "bad datasource",
			mutate: func(cfg *L1Config) {
				cfg.DataSource = "fixtures"
			},
			wantText: "data_source must be one of live, pcap, pcap_analysis",
		},
		{
			name: "bad udp port",
			mutate: func(cfg *L1Config) {
				cfg.UDPPort = 70000
			},
			wantText: "udp_port must be in [1, 65535]",
		},
		{
			name: "bad recv buffer",
			mutate: func(cfg *L1Config) {
				cfg.UDPRcvBuf = 0
			},
			wantText: "udp_rcv_buf must be positive",
		},
		{
			name: "bad forward port",
			mutate: func(cfg *L1Config) {
				cfg.ForwardPort = -1
			},
			wantText: "forward_port must be in [0, 65535]",
		},
		{
			name: "bad foreground forward port",
			mutate: func(cfg *L1Config) {
				cfg.ForegroundForwardPort = 70000
			},
			wantText: "foreground_forward_port must be in [0, 65535]",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := sampleValidConfig().L1
			tc.mutate(&cfg)
			err := cfg.Validate()
			requireErrorContains(t, err, tc.wantText)
		})
	}
}

func TestPipelineConfigValidateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*PipelineConfig)
		wantText string
	}{
		{
			name: "bad buffer timeout",
			mutate: func(cfg *PipelineConfig) {
				cfg.BufferTimeout = "broken"
			},
			wantText: "invalid buffer_timeout",
		},
		{
			name: "negative min frame points",
			mutate: func(cfg *PipelineConfig) {
				cfg.MinFramePoints = -1
			},
			wantText: "min_frame_points must be non-negative",
		},
		{
			name: "bad flush interval",
			mutate: func(cfg *PipelineConfig) {
				cfg.FlushInterval = "broken"
			},
			wantText: "invalid flush_interval",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := sampleValidConfig().Pipeline
			tc.mutate(&cfg)
			err := cfg.Validate()
			requireErrorContains(t, err, tc.wantText)
		})
	}
}

func TestEngineSelectionValidateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		run      func() error
		wantText string
	}{
		{
			name: "l3 missing baseline block",
			run: func() error {
				return (&L3Config{Engine: "ema_baseline_v1"}).Validate()
			},
			wantText: `selected engine block "ema_baseline_v1" missing`,
		},
		{
			name: "l3 missing track assist block",
			run: func() error {
				return (&L3Config{Engine: "ema_track_assist_v2"}).Validate()
			},
			wantText: `selected engine block "ema_track_assist_v2" missing`,
		},
		{
			name: "l3 unknown engine",
			run: func() error {
				return (&L3Config{Engine: "mystery"}).Validate()
			},
			wantText: `unknown engine "mystery"`,
		},
		{
			name: "l4 missing dbscan block",
			run: func() error {
				return (&L4Config{Engine: "dbscan_xy_v1"}).Validate()
			},
			wantText: `selected engine block "dbscan_xy_v1" missing`,
		},
		{
			name: "l4 missing mahalanobis block",
			run: func() error {
				return (&L4Config{Engine: "two_stage_mahalanobis_v2"}).Validate()
			},
			wantText: `selected engine block "two_stage_mahalanobis_v2" missing`,
		},
		{
			name: "l4 missing hdbscan block",
			run: func() error {
				return (&L4Config{Engine: "hdbscan_adaptive_v1"}).Validate()
			},
			wantText: `selected engine block "hdbscan_adaptive_v1" missing`,
		},
		{
			name: "l4 unknown engine",
			run: func() error {
				return (&L4Config{Engine: "mystery"}).Validate()
			},
			wantText: `unknown engine "mystery"`,
		},
		{
			name: "l5 missing cv block",
			run: func() error {
				return (&L5Config{Engine: "cv_kf_v1"}).Validate()
			},
			wantText: `selected engine block "cv_kf_v1" missing`,
		},
		{
			name: "l5 missing imm block",
			run: func() error {
				return (&L5Config{Engine: "imm_cv_ca_v2"}).Validate()
			},
			wantText: `selected engine block "imm_cv_ca_v2" missing`,
		},
		{
			name: "l5 missing rts block",
			run: func() error {
				return (&L5Config{Engine: "imm_cv_ca_rts_eval_v2"}).Validate()
			},
			wantText: `selected engine block "imm_cv_ca_rts_eval_v2" missing`,
		},
		{
			name: "l5 unknown engine",
			run: func() error {
				return (&L5Config{Engine: "mystery"}).Validate()
			},
			wantText: `unknown engine "mystery"`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			requireErrorContains(t, tc.run(), tc.wantText)
		})
	}
}

func TestEngineSelectionValidateSuccessForAlternateEngines(t *testing.T) {
	t.Parallel()

	l3 := &L3Config{
		Engine: "ema_track_assist_v2",
		EmaTrackAssistV2: &L3EmaTrackAssistV2{
			L3Common:              sampleValidConfig().L3.EmaBaselineV1.L3Common,
			PromotionNearGateLow:  0.1,
			PromotionNearGateHigh: 0.2,
			PromotionThreshold:    0.3,
		},
	}
	if err := l3.Validate(); err != nil {
		t.Fatalf("l3 validate: %v", err)
	}

	l4 := &L4Config{
		Engine: "two_stage_mahalanobis_v2",
		TwoStageMahalanobisV2: &L4TwoStageMahalanobisV2{
			L4Common:              sampleValidConfig().L4.DbscanXyV1.L4Common,
			VelocityCoherenceGate: 1,
			MinVelocityConfidence: 0.5,
		},
	}
	if err := l4.Validate(); err != nil {
		t.Fatalf("l4 mahalanobis validate: %v", err)
	}

	l4.Engine = "hdbscan_adaptive_v1"
	l4.TwoStageMahalanobisV2 = nil
	l4.HdbscanAdaptiveV1 = &L4HdbscanAdaptiveV1{
		L4Common:       sampleValidConfig().L4.DbscanXyV1.L4Common,
		MinClusterSize: 4,
		MinSamples:     2,
	}
	if err := l4.Validate(); err != nil {
		t.Fatalf("l4 hdbscan validate: %v", err)
	}

	l5 := &L5Config{
		Engine: "imm_cv_ca_v2",
		ImmCvCaV2: &L5ImmCvCaV2{
			L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA:         0.5,
			TransitionCAToCV:         0.5,
			CAProcessNoiseAcc:        1,
			LowSpeedHeadingFreezeMps: 0.5,
		},
	}
	if err := l5.Validate(); err != nil {
		t.Fatalf("l5 imm validate: %v", err)
	}

	l5.Engine = "imm_cv_ca_rts_eval_v2"
	l5.ImmCvCaV2 = nil
	l5.ImmCvCaRtsEvalV2 = &L5ImmCvCaRtsEvalV2{
		L5ImmCvCaV2: L5ImmCvCaV2{
			L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA:         0.5,
			TransitionCAToCV:         0.5,
			CAProcessNoiseAcc:        1,
			LowSpeedHeadingFreezeMps: 0.5,
		},
		RTSSmoothingWindow: 4,
	}
	if err := l5.Validate(); err != nil {
		t.Fatalf("l5 rts validate: %v", err)
	}
}

func TestCommonAndVariantValidateErrors(t *testing.T) {
	t.Parallel()

	l3Tests := []struct {
		name     string
		mutate   func(*L3Common)
		wantText string
	}{
		{"background update", func(cfg *L3Common) { cfg.BackgroundUpdateFraction = 0 }, "background_update_fraction must be in (0, 1]"},
		{"closeness", func(cfg *L3Common) { cfg.ClosenessMultiplier = 0 }, "closeness_multiplier must be positive"},
		{"safety margin", func(cfg *L3Common) { cfg.SafetyMarginMetres = -1 }, "safety_margin_metres must be non-negative"},
		{"noise", func(cfg *L3Common) { cfg.NoiseRelative = -0.1 }, "noise_relative must be between 0 and 1"},
		{"neighbour count", func(cfg *L3Common) { cfg.NeighbourConfirmationCount = 9 }, "neighbour_confirmation_count must be in [0, 8]"},
		{"warmup duration", func(cfg *L3Common) { cfg.WarmupDurationNanos = -1 }, "warmup_duration_nanos must be non-negative"},
		{"warmup frames", func(cfg *L3Common) { cfg.WarmupMinFrames = -1 }, "warmup_min_frames must be non-negative"},
		{"post settle", func(cfg *L3Common) { cfg.PostSettleUpdateFraction = 2 }, "post_settle_update_fraction must be in [0, 1]"},
		{"freeze duration", func(cfg *L3Common) { cfg.FreezeDuration = "bad" }, "invalid freeze_duration"},
		{"freeze threshold", func(cfg *L3Common) { cfg.FreezeThresholdMultiplier = 0 }, "freeze_threshold_multiplier must be positive"},
		{"settling period", func(cfg *L3Common) { cfg.SettlingPeriod = "bad" }, "invalid settling_period"},
		{"snapshot interval", func(cfg *L3Common) { cfg.SnapshotInterval = "bad" }, "invalid snapshot_interval"},
		{"change threshold", func(cfg *L3Common) { cfg.ChangeThresholdSnapshot = -1 }, "change_threshold_snapshot must be non-negative"},
		{"reacquisition", func(cfg *L3Common) { cfg.ReacquisitionBoostMultiplier = -1 }, "reacquisition_boost_multiplier must be non-negative"},
		{"confidence floor", func(cfg *L3Common) { cfg.MinConfidenceFloor = -1 }, "min_confidence_floor must be non-negative"},
		{"locked threshold", func(cfg *L3Common) { cfg.LockedBaselineThreshold = -1 }, "locked_baseline_threshold must be non-negative"},
		{"locked multiplier", func(cfg *L3Common) { cfg.LockedBaselineMultiplier = -1 }, "locked_baseline_multiplier must be non-negative"},
		{"movement threshold", func(cfg *L3Common) { cfg.SensorMovementForegroundThreshold = 2 }, "sensor_movement_foreground_threshold must be in [0, 1]"},
		{"drift metres", func(cfg *L3Common) { cfg.BackgroundDriftThresholdMetres = -1 }, "background_drift_threshold_metres must be non-negative"},
		{"drift ratio", func(cfg *L3Common) { cfg.BackgroundDriftRatioThreshold = 2 }, "background_drift_ratio_threshold must be in [0, 1]"},
		{"settling coverage", func(cfg *L3Common) { cfg.SettlingMinCoverage = 2 }, "settling_min_coverage must be in [0, 1]"},
		{"spread delta", func(cfg *L3Common) { cfg.SettlingMaxSpreadDelta = -1 }, "settling_max_spread_delta must be non-negative"},
		{"region stability", func(cfg *L3Common) { cfg.SettlingMinRegionStability = 2 }, "settling_min_region_stability must be in [0, 1]"},
		{"settling confidence", func(cfg *L3Common) { cfg.SettlingMinConfidence = -1 }, "settling_min_confidence must be non-negative"},
	}

	for _, tc := range l3Tests {
		tc := tc
		t.Run("l3/"+tc.name, func(t *testing.T) {
			cfg := sampleValidConfig().L3.EmaBaselineV1.L3Common
			tc.mutate(&cfg)
			requireErrorContains(t, cfg.Validate(), tc.wantText)
		})
	}

	t.Run("l3-track-assist-low", func(t *testing.T) {
		cfg := L3EmaTrackAssistV2{L3Common: sampleValidConfig().L3.EmaBaselineV1.L3Common, PromotionNearGateLow: -1}
		requireErrorContains(t, cfg.Validate(), "promotion_near_gate_low must be non-negative")
	})
	t.Run("l3-track-assist-high", func(t *testing.T) {
		cfg := L3EmaTrackAssistV2{L3Common: sampleValidConfig().L3.EmaBaselineV1.L3Common, PromotionNearGateHigh: -1}
		requireErrorContains(t, cfg.Validate(), "promotion_near_gate_high must be non-negative")
	})
	t.Run("l3-track-assist-threshold", func(t *testing.T) {
		cfg := L3EmaTrackAssistV2{L3Common: sampleValidConfig().L3.EmaBaselineV1.L3Common, PromotionThreshold: -1}
		requireErrorContains(t, cfg.Validate(), "promotion_threshold must be non-negative")
	})
	t.Run("l3-track-assist-common-error", func(t *testing.T) {
		cfg := L3EmaTrackAssistV2{
			L3Common:              sampleValidConfig().L3.EmaBaselineV1.L3Common,
			PromotionNearGateLow:  0.1,
			PromotionNearGateHigh: 0.2,
			PromotionThreshold:    0.3,
		}
		cfg.BackgroundUpdateFraction = 0
		requireErrorContains(t, cfg.Validate(), "background_update_fraction must be in (0, 1]")
	})
	t.Run("l3-track-assist-valid", func(t *testing.T) {
		cfg := L3EmaTrackAssistV2{
			L3Common:              sampleValidConfig().L3.EmaBaselineV1.L3Common,
			PromotionNearGateLow:  0.1,
			PromotionNearGateHigh: 0.2,
			PromotionThreshold:    0.3,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("track assist validate: %v", err)
		}
	})

	l4Tests := []struct {
		name     string
		mutate   func(*L4Common)
		wantText string
	}{
		{"eps", func(cfg *L4Common) { cfg.ForegroundDBSCANEps = 0 }, "foreground_dbscan_eps must be positive"},
		{"min points", func(cfg *L4Common) { cfg.ForegroundMinClusterPoints = 0 }, "foreground_min_cluster_points must be >= 1"},
		{"max input", func(cfg *L4Common) { cfg.ForegroundMaxInputPoints = 0 }, "foreground_max_input_points must be >= 1"},
		{"height band", func(cfg *L4Common) { cfg.HeightBandFloor = 2; cfg.HeightBandCeiling = 1 }, "height_band_floor must be <= height_band_ceiling"},
		{"max diameter", func(cfg *L4Common) { cfg.MaxClusterDiameter = 0 }, "max_cluster_diameter must be positive"},
		{"min diameter", func(cfg *L4Common) { cfg.MinClusterDiameter = 0 }, "min_cluster_diameter must be positive"},
		{"aspect ratio", func(cfg *L4Common) { cfg.MaxClusterAspectRatio = 0 }, "max_cluster_aspect_ratio must be positive"},
	}

	for _, tc := range l4Tests {
		tc := tc
		t.Run("l4/"+tc.name, func(t *testing.T) {
			cfg := sampleValidConfig().L4.DbscanXyV1.L4Common
			tc.mutate(&cfg)
			requireErrorContains(t, cfg.Validate(), tc.wantText)
		})
	}

	t.Run("l4-mahalanobis-gate", func(t *testing.T) {
		cfg := L4TwoStageMahalanobisV2{L4Common: sampleValidConfig().L4.DbscanXyV1.L4Common}
		requireErrorContains(t, cfg.Validate(), "velocity_coherence_gate must be positive")
	})
	t.Run("l4-mahalanobis-confidence", func(t *testing.T) {
		cfg := L4TwoStageMahalanobisV2{
			L4Common:              sampleValidConfig().L4.DbscanXyV1.L4Common,
			VelocityCoherenceGate: 1,
			MinVelocityConfidence: 2,
		}
		requireErrorContains(t, cfg.Validate(), "min_velocity_confidence must be in [0, 1]")
	})
	t.Run("l4-mahalanobis-common-error", func(t *testing.T) {
		cfg := L4TwoStageMahalanobisV2{
			L4Common:              sampleValidConfig().L4.DbscanXyV1.L4Common,
			VelocityCoherenceGate: 1,
			MinVelocityConfidence: 0.5,
		}
		cfg.ForegroundDBSCANEps = 0
		requireErrorContains(t, cfg.Validate(), "foreground_dbscan_eps must be positive")
	})
	t.Run("l4-mahalanobis-valid", func(t *testing.T) {
		cfg := L4TwoStageMahalanobisV2{
			L4Common:              sampleValidConfig().L4.DbscanXyV1.L4Common,
			VelocityCoherenceGate: 1,
			MinVelocityConfidence: 0.5,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("two-stage validate: %v", err)
		}
	})
	t.Run("l4-hdbscan-cluster-size", func(t *testing.T) {
		cfg := L4HdbscanAdaptiveV1{L4Common: sampleValidConfig().L4.DbscanXyV1.L4Common}
		requireErrorContains(t, cfg.Validate(), "min_cluster_size must be >= 1")
	})
	t.Run("l4-hdbscan-samples", func(t *testing.T) {
		cfg := L4HdbscanAdaptiveV1{L4Common: sampleValidConfig().L4.DbscanXyV1.L4Common, MinClusterSize: 1}
		requireErrorContains(t, cfg.Validate(), "min_samples must be >= 1")
	})
	t.Run("l4-hdbscan-common-error", func(t *testing.T) {
		cfg := L4HdbscanAdaptiveV1{
			L4Common:       sampleValidConfig().L4.DbscanXyV1.L4Common,
			MinClusterSize: 4,
			MinSamples:     2,
		}
		cfg.ForegroundDBSCANEps = 0
		requireErrorContains(t, cfg.Validate(), "foreground_dbscan_eps must be positive")
	})
	t.Run("l4-hdbscan-valid", func(t *testing.T) {
		cfg := L4HdbscanAdaptiveV1{
			L4Common:       sampleValidConfig().L4.DbscanXyV1.L4Common,
			MinClusterSize: 4,
			MinSamples:     2,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("hdbscan validate: %v", err)
		}
	})

	l5Tests := []struct {
		name     string
		mutate   func(*L5Common)
		wantText string
	}{
		{"gating", func(cfg *L5Common) { cfg.GatingDistanceSquared = 0 }, "gating_distance_squared must be positive"},
		{"noise pos", func(cfg *L5Common) { cfg.ProcessNoisePos = 0 }, "process_noise_pos must be positive"},
		{"noise vel", func(cfg *L5Common) { cfg.ProcessNoiseVel = 0 }, "process_noise_vel must be positive"},
		{"measurement", func(cfg *L5Common) { cfg.MeasurementNoise = 0 }, "measurement_noise must be positive"},
		{"occlusion", func(cfg *L5Common) { cfg.OcclusionCovInflation = -1 }, "occlusion_cov_inflation must be non-negative"},
		{"hits", func(cfg *L5Common) { cfg.HitsToConfirm = 0 }, "hits_to_confirm must be >= 1"},
		{"max misses", func(cfg *L5Common) { cfg.MaxMisses = 0 }, "max_misses must be >= 1"},
		{"max misses confirmed", func(cfg *L5Common) { cfg.MaxMissesConfirmed = 0 }, "max_misses_confirmed must be >= 1"},
		{"max tracks", func(cfg *L5Common) { cfg.MaxTracks = 1001 }, "max_tracks must be in [1, 1000]"},
		{"max speed", func(cfg *L5Common) { cfg.MaxReasonableSpeedMps = 0 }, "max_reasonable_speed_mps must be positive"},
		{"max jump", func(cfg *L5Common) { cfg.MaxPositionJumpMetres = 0 }, "max_position_jump_metres must be positive"},
		{"predict dt", func(cfg *L5Common) { cfg.MaxPredictDt = 0 }, "max_predict_dt must be positive"},
		{"covariance", func(cfg *L5Common) { cfg.MaxCovarianceDiag = 0 }, "max_covariance_diag must be positive"},
		{"pca", func(cfg *L5Common) { cfg.MinPointsForPCA = 0 }, "min_points_for_pca must be >= 1"},
		{"heading alpha", func(cfg *L5Common) { cfg.OBBHeadingSmoothingAlpha = 2 }, "obb_heading_smoothing_alpha must be in [0, 1]"},
		{"aspect lock", func(cfg *L5Common) { cfg.OBBAspectRatioLockThreshold = -1 }, "obb_aspect_ratio_lock_threshold must be non-negative"},
		{"track history", func(cfg *L5Common) { cfg.MaxTrackHistoryLength = 0 }, "max_track_history_length must be >= 1"},
		{"speed history", func(cfg *L5Common) { cfg.MaxSpeedHistoryLength = 0 }, "max_speed_history_length must be >= 1"},
		{"merge ratio", func(cfg *L5Common) { cfg.MergeSizeRatio = 0 }, "merge_size_ratio must be positive"},
		{"split ratio", func(cfg *L5Common) { cfg.SplitSizeRatio = 0 }, "split_size_ratio must be positive"},
		{"grace period", func(cfg *L5Common) { cfg.DeletedTrackGracePeriod = "bad" }, "invalid deleted_track_grace_period"},
		{"min observations", func(cfg *L5Common) { cfg.MinObservationsForClassification = 0 }, "min_observations_for_classification must be >= 1"},
	}

	for _, tc := range l5Tests {
		tc := tc
		t.Run("l5/"+tc.name, func(t *testing.T) {
			cfg := sampleValidConfig().L5.CvKfV1.L5Common
			tc.mutate(&cfg)
			requireErrorContains(t, cfg.Validate(), tc.wantText)
		})
	}

	t.Run("l5-imm-transition-cv-ca", func(t *testing.T) {
		cfg := L5ImmCvCaV2{L5Common: sampleValidConfig().L5.CvKfV1.L5Common, TransitionCVToCA: 2}
		requireErrorContains(t, cfg.Validate(), "transition_cv_to_ca must be in [0, 1]")
	})
	t.Run("l5-imm-transition-ca-cv", func(t *testing.T) {
		cfg := L5ImmCvCaV2{
			L5Common:         sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA: 0.5,
			TransitionCAToCV: 2,
		}
		requireErrorContains(t, cfg.Validate(), "transition_ca_to_cv must be in [0, 1]")
	})
	t.Run("l5-imm-process-noise", func(t *testing.T) {
		cfg := L5ImmCvCaV2{
			L5Common:         sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA: 0.5,
			TransitionCAToCV: 0.5,
		}
		requireErrorContains(t, cfg.Validate(), "ca_process_noise_acc must be positive")
	})
	t.Run("l5-imm-low-speed-freeze", func(t *testing.T) {
		cfg := L5ImmCvCaV2{
			L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA:         0.5,
			TransitionCAToCV:         0.5,
			CAProcessNoiseAcc:        1,
			LowSpeedHeadingFreezeMps: -1,
		}
		requireErrorContains(t, cfg.Validate(), "low_speed_heading_freeze_mps must be non-negative")
	})
	t.Run("l5-imm-common-error", func(t *testing.T) {
		cfg := L5ImmCvCaV2{
			L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA:         0.5,
			TransitionCAToCV:         0.5,
			CAProcessNoiseAcc:        1,
			LowSpeedHeadingFreezeMps: 0.5,
		}
		cfg.GatingDistanceSquared = 0
		requireErrorContains(t, cfg.Validate(), "gating_distance_squared must be positive")
	})
	t.Run("l5-imm-valid", func(t *testing.T) {
		cfg := L5ImmCvCaV2{
			L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
			TransitionCVToCA:         0.5,
			TransitionCAToCV:         0.5,
			CAProcessNoiseAcc:        1,
			LowSpeedHeadingFreezeMps: 0.5,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("imm validate: %v", err)
		}
	})
	t.Run("l5-rts-window", func(t *testing.T) {
		cfg := L5ImmCvCaRtsEvalV2{
			L5ImmCvCaV2: L5ImmCvCaV2{
				L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
				TransitionCVToCA:         0.5,
				TransitionCAToCV:         0.5,
				CAProcessNoiseAcc:        1,
				LowSpeedHeadingFreezeMps: 0,
			},
		}
		requireErrorContains(t, cfg.Validate(), "rts_smoothing_window must be >= 1")
	})
	t.Run("l5-rts-parent-error", func(t *testing.T) {
		cfg := L5ImmCvCaRtsEvalV2{
			L5ImmCvCaV2: L5ImmCvCaV2{
				L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
				TransitionCVToCA:         0.5,
				TransitionCAToCV:         0.5,
				CAProcessNoiseAcc:        1,
				LowSpeedHeadingFreezeMps: 0.5,
			},
			RTSSmoothingWindow: 4,
		}
		cfg.GatingDistanceSquared = 0
		requireErrorContains(t, cfg.Validate(), "gating_distance_squared must be positive")
	})
	t.Run("l5-rts-valid", func(t *testing.T) {
		cfg := L5ImmCvCaRtsEvalV2{
			L5ImmCvCaV2: L5ImmCvCaV2{
				L5Common:                 sampleValidConfig().L5.CvKfV1.L5Common,
				TransitionCVToCA:         0.5,
				TransitionCAToCV:         0.5,
				CAProcessNoiseAcc:        1,
				LowSpeedHeadingFreezeMps: 0.5,
			},
			RTSSmoothingWindow: 4,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("rts validate: %v", err)
		}
	})
}

func TestStrictEngineUnmarshalAndHelpers(t *testing.T) {
	t.Parallel()

	t.Run("l3 baseline", func(t *testing.T) {
		raw := []byte(`{"engine":"ema_baseline_v1","ema_baseline_v1":{"background_update_fraction":0.02,"closeness_multiplier":3,"safety_margin_metres":0.15,"noise_relative":0.02,"neighbour_confirmation_count":3,"seed_from_first":true,"warmup_duration_nanos":30000000000,"warmup_min_frames":100,"post_settle_update_fraction":0,"enable_diagnostics":false,"freeze_duration":"5s","freeze_threshold_multiplier":3,"settling_period":"5m","snapshot_interval":"2h","change_threshold_snapshot":100,"reacquisition_boost_multiplier":5,"min_confidence_floor":3,"locked_baseline_threshold":50,"locked_baseline_multiplier":4,"sensor_movement_foreground_threshold":0.2,"background_drift_threshold_metres":0.5,"background_drift_ratio_threshold":0.1,"settling_min_coverage":0.8,"settling_max_spread_delta":0.001,"settling_min_region_stability":0.95,"settling_min_confidence":10}}`)
		var cfg L3Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("unmarshal l3 baseline: %v", err)
		}
		if cfg.EmaBaselineV1 == nil || cfg.ActiveCommon() == nil || cfg.ActiveConfig() == nil {
			t.Fatal("expected active baseline config")
		}
	})

	t.Run("l3 track assist", func(t *testing.T) {
		raw := []byte(`{"engine":"ema_track_assist_v2","ema_track_assist_v2":{"background_update_fraction":0.02,"closeness_multiplier":3,"safety_margin_metres":0.15,"noise_relative":0.02,"neighbour_confirmation_count":3,"seed_from_first":true,"warmup_duration_nanos":30000000000,"warmup_min_frames":100,"post_settle_update_fraction":0,"enable_diagnostics":false,"freeze_duration":"5s","freeze_threshold_multiplier":3,"settling_period":"5m","snapshot_interval":"2h","change_threshold_snapshot":100,"reacquisition_boost_multiplier":5,"min_confidence_floor":3,"locked_baseline_threshold":50,"locked_baseline_multiplier":4,"sensor_movement_foreground_threshold":0.2,"background_drift_threshold_metres":0.5,"background_drift_ratio_threshold":0.1,"settling_min_coverage":0.8,"settling_max_spread_delta":0.001,"settling_min_region_stability":0.95,"settling_min_confidence":10,"promotion_near_gate_low":0.1,"promotion_near_gate_high":0.2,"promotion_threshold":0.3}}`)
		var cfg L3Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("unmarshal l3 track assist: %v", err)
		}
		if cfg.EmaTrackAssistV2 == nil || cfg.ActiveCommon() == nil || cfg.ActiveConfig() == nil {
			t.Fatal("expected active track assist config")
		}
	})

	t.Run("l4 variants", func(t *testing.T) {
		cases := []string{
			`{"engine":"dbscan_xy_v1","dbscan_xy_v1":{"foreground_dbscan_eps":0.8,"foreground_min_cluster_points":5,"foreground_max_input_points":8000,"height_band_floor":-2.8,"height_band_ceiling":1.5,"remove_ground":true,"max_cluster_diameter":12,"min_cluster_diameter":0.05,"max_cluster_aspect_ratio":15}}`,
			`{"engine":"two_stage_mahalanobis_v2","two_stage_mahalanobis_v2":{"foreground_dbscan_eps":0.8,"foreground_min_cluster_points":5,"foreground_max_input_points":8000,"height_band_floor":-2.8,"height_band_ceiling":1.5,"remove_ground":true,"max_cluster_diameter":12,"min_cluster_diameter":0.05,"max_cluster_aspect_ratio":15,"velocity_coherence_gate":1,"min_velocity_confidence":0.5}}`,
			`{"engine":"hdbscan_adaptive_v1","hdbscan_adaptive_v1":{"foreground_dbscan_eps":0.8,"foreground_min_cluster_points":5,"foreground_max_input_points":8000,"height_band_floor":-2.8,"height_band_ceiling":1.5,"remove_ground":true,"max_cluster_diameter":12,"min_cluster_diameter":0.05,"max_cluster_aspect_ratio":15,"min_cluster_size":4,"min_samples":2}}`,
		}
		for _, raw := range cases {
			var cfg L4Config
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				t.Fatalf("unmarshal l4 variant: %v", err)
			}
			if cfg.ActiveCommon() == nil || cfg.ActiveConfig() == nil {
				t.Fatal("expected active l4 config")
			}
		}
	})

	t.Run("l5 variants", func(t *testing.T) {
		cases := []string{
			`{"engine":"cv_kf_v1","cv_kf_v1":{"gating_distance_squared":36,"process_noise_pos":0.05,"process_noise_vel":0.2,"measurement_noise":0.05,"occlusion_cov_inflation":0.5,"hits_to_confirm":4,"max_misses":3,"max_misses_confirmed":15,"max_tracks":100,"max_reasonable_speed_mps":30,"max_position_jump_metres":5,"max_predict_dt":0.5,"max_covariance_diag":100,"min_points_for_pca":4,"obb_heading_smoothing_alpha":0.08,"obb_aspect_ratio_lock_threshold":0.25,"max_track_history_length":200,"max_speed_history_length":100,"merge_size_ratio":2.5,"split_size_ratio":0.3,"deleted_track_grace_period":"5s","min_observations_for_classification":5}}`,
			`{"engine":"imm_cv_ca_v2","imm_cv_ca_v2":{"gating_distance_squared":36,"process_noise_pos":0.05,"process_noise_vel":0.2,"measurement_noise":0.05,"occlusion_cov_inflation":0.5,"hits_to_confirm":4,"max_misses":3,"max_misses_confirmed":15,"max_tracks":100,"max_reasonable_speed_mps":30,"max_position_jump_metres":5,"max_predict_dt":0.5,"max_covariance_diag":100,"min_points_for_pca":4,"obb_heading_smoothing_alpha":0.08,"obb_aspect_ratio_lock_threshold":0.25,"max_track_history_length":200,"max_speed_history_length":100,"merge_size_ratio":2.5,"split_size_ratio":0.3,"deleted_track_grace_period":"5s","min_observations_for_classification":5,"transition_cv_to_ca":0.2,"transition_ca_to_cv":0.1,"ca_process_noise_acc":1,"low_speed_heading_freeze_mps":0.5}}`,
			`{"engine":"imm_cv_ca_rts_eval_v2","imm_cv_ca_rts_eval_v2":{"gating_distance_squared":36,"process_noise_pos":0.05,"process_noise_vel":0.2,"measurement_noise":0.05,"occlusion_cov_inflation":0.5,"hits_to_confirm":4,"max_misses":3,"max_misses_confirmed":15,"max_tracks":100,"max_reasonable_speed_mps":30,"max_position_jump_metres":5,"max_predict_dt":0.5,"max_covariance_diag":100,"min_points_for_pca":4,"obb_heading_smoothing_alpha":0.08,"obb_aspect_ratio_lock_threshold":0.25,"max_track_history_length":200,"max_speed_history_length":100,"merge_size_ratio":2.5,"split_size_ratio":0.3,"deleted_track_grace_period":"5s","min_observations_for_classification":5,"transition_cv_to_ca":0.2,"transition_ca_to_cv":0.1,"ca_process_noise_acc":1,"low_speed_heading_freeze_mps":0.5,"rts_smoothing_window":4}}`,
		}
		for _, raw := range cases {
			var cfg L5Config
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				t.Fatalf("unmarshal l5 variant: %v", err)
			}
			if cfg.ActiveCommon() == nil || cfg.ActiveConfig() == nil {
				t.Fatal("expected active l5 config")
			}
		}
	})

	t.Run("unmarshal errors", func(t *testing.T) {
		var l3 L3Config
		requireErrorContains(t, json.Unmarshal([]byte(`[]`), &l3), "l3: expected object")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"ema_baseline_v1","unknown":1}`), &l3), "l3: unknown keys: unknown")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"ema_track_assist_v2","ema_track_assist_v2":{"promotion_threshold":1}}`), &l3), "missing required keys")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"unknown"}`), &l3), `l3: unknown engine "unknown"`)
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"ema_baseline_v1","ema_track_assist_v2":{}}`), &l3), `non-selected engine block "ema_track_assist_v2" present`)
		requireErrorContains(t, json.Unmarshal([]byte(`{"ema_baseline_v1":{}}`), &l3), "missing required key: engine")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":123}`), &l3), "expected string")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":" "}`), &l3), "must be non-empty")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"ema_baseline_v1","ema_baseline_v1":{"unknown":1}}`), &l3), "unknown keys: unknown")

		var l4 L4Config
		requireErrorContains(t, json.Unmarshal([]byte(`[]`), &l4), "l4: expected object")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"dbscan_xy_v1","unknown":1}`), &l4), "l4: unknown keys: unknown")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":123}`), &l4), "expected string")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"two_stage_mahalanobis_v2","two_stage_mahalanobis_v2":{"velocity_coherence_gate":1}}`), &l4), "missing required keys")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"hdbscan_adaptive_v1","hdbscan_adaptive_v1":{"min_cluster_size":1}}`), &l4), "missing required keys")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"unknown"}`), &l4), `l4: unknown engine "unknown"`)
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"dbscan_xy_v1","hdbscan_adaptive_v1":{}}`), &l4), `non-selected engine block "hdbscan_adaptive_v1" present`)

		var l5 L5Config
		requireErrorContains(t, json.Unmarshal([]byte(`[]`), &l5), "l5: expected object")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"cv_kf_v1","unknown":1}`), &l5), "l5: unknown keys: unknown")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":123}`), &l5), "expected string")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"imm_cv_ca_v2","imm_cv_ca_v2":{"transition_cv_to_ca":0.2}}`), &l5), "missing required keys")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"imm_cv_ca_rts_eval_v2","imm_cv_ca_rts_eval_v2":{"transition_cv_to_ca":0.2}}`), &l5), "missing required keys")
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"unknown"}`), &l5), `l5: unknown engine "unknown"`)
		requireErrorContains(t, json.Unmarshal([]byte(`{"engine":"cv_kf_v1","imm_cv_ca_v2":{}}`), &l5), `non-selected engine block "imm_cv_ca_v2" present`)
	})
}

func TestDecodeAndReflectionHelpers(t *testing.T) {
	t.Parallel()

	t.Run("parseObject", func(t *testing.T) {
		if raw, err := parseObject([]byte(`{"ok":1}`), "root"); err != nil || len(raw) != 1 {
			t.Fatalf("parseObject success = (%v, %v), want one key and nil error", raw, err)
		}
		requireErrorContains(t, errorOrNil(parseObject([]byte(`[]`), "root")), "root: expected object")
		requireErrorContains(t, errorOrNil(parseObject([]byte(`null`), "root")), "root: expected object")
	})

	t.Run("requiredEngine", func(t *testing.T) {
		engine, err := requiredEngine(map[string]json.RawMessage{"engine": json.RawMessage(`"cv_kf_v1"`)}, "l5")
		if err != nil || engine != "cv_kf_v1" {
			t.Fatalf("requiredEngine = (%q, %v), want (%q, nil)", engine, err, "cv_kf_v1")
		}
		requireErrorContains(t, errorOnly(requiredEngine(map[string]json.RawMessage{}, "l5")), "missing required key: engine")
		requireErrorContains(t, errorOnly(requiredEngine(map[string]json.RawMessage{"engine": json.RawMessage(`123`)}, "l5")), "expected string")
		requireErrorContains(t, errorOnly(requiredEngine(map[string]json.RawMessage{"engine": json.RawMessage(`" "`)}, "l5")), "must be non-empty")
	})

	t.Run("ensureAllowedKeys", func(t *testing.T) {
		raw := map[string]json.RawMessage{"engine": nil, "z": nil, "a": nil}
		requireErrorContains(t, ensureAllowedKeys(raw, "l4", []string{"engine"}), "l4: unknown keys: a, z")
	})

	t.Run("decodeSelectedEngineBlock", func(t *testing.T) {
		raw := map[string]json.RawMessage{
			"engine":          json.RawMessage(`"ema_baseline_v1"`),
			"ema_baseline_v1": json.RawMessage(`{"background_update_fraction":0.02,"closeness_multiplier":3,"safety_margin_metres":0.15,"noise_relative":0.02,"neighbour_confirmation_count":3,"seed_from_first":true,"warmup_duration_nanos":30000000000,"warmup_min_frames":100,"post_settle_update_fraction":0,"enable_diagnostics":false,"freeze_duration":"5s","freeze_threshold_multiplier":3,"settling_period":"5m","snapshot_interval":"2h","change_threshold_snapshot":100,"reacquisition_boost_multiplier":5,"min_confidence_floor":3,"locked_baseline_threshold":50,"locked_baseline_multiplier":4,"sensor_movement_foreground_threshold":0.2,"background_drift_threshold_metres":0.5,"background_drift_ratio_threshold":0.1,"settling_min_coverage":0.8,"settling_max_spread_delta":0.001,"settling_min_region_stability":0.95,"settling_min_confidence":10}`),
		}
		block, err := decodeSelectedEngineBlock[L3EmaBaselineV1](raw, "l3", "ema_baseline_v1")
		if err != nil || block == nil {
			t.Fatalf("decodeSelectedEngineBlock = (%v, %v), want non-nil block and nil error", block, err)
		}
		requireErrorContains(t, errorOnly(decodeSelectedEngineBlock[L3EmaBaselineV1](map[string]json.RawMessage{"engine": nil, "ema_track_assist_v2": nil}, "l3", "ema_baseline_v1")), `non-selected engine block "ema_track_assist_v2" present`)
		requireErrorContains(t, errorOnly(decodeSelectedEngineBlock[L3EmaBaselineV1](map[string]json.RawMessage{"engine": nil}, "l3", "ema_baseline_v1")), `selected engine block "ema_baseline_v1" missing`)
	})

	t.Run("strictDecodeObject", func(t *testing.T) {
		type embedded struct {
			Shared int `json:"shared"`
		}
		type target struct {
			embedded
			Name        string `json:"name"`
			DefaultName string `json:",omitempty"`
			Plain       int
			Skip        int `json:"-"`
		}

		var out target
		if err := strictDecodeObject([]byte(`{"shared":1,"name":"ok","DefaultName":"default","Plain":2}`), &out, "target"); err != nil {
			t.Fatalf("strictDecodeObject success: %v", err)
		}
		requireErrorContains(t, strictDecodeObject([]byte(`{"shared":1,"name":"ok","DefaultName":"default","Plain":2,"extra":3}`), &out, "target"), "target: unknown keys: extra")
		requireErrorContains(t, strictDecodeObject([]byte(`{"shared":1,"name":"ok","Plain":2}`), &out, "target"), "target: missing required keys: DefaultName")
		requireErrorContains(t, strictDecodeObject([]byte(`[]`), &out, "target"), "target: expected object")

		keys := expectedJSONKeys(reflect.TypeOf(&out))
		want := []string{"shared", "name", "DefaultName", "Plain"}
		if !reflect.DeepEqual(keys, want) {
			t.Fatalf("expectedJSONKeys = %v, want %v", keys, want)
		}
		if keys := expectedJSONKeys(reflect.TypeOf(0)); keys != nil {
			t.Fatalf("expectedJSONKeys(non-struct) = %v, want nil", keys)
		}

		var fieldTests = []struct {
			field    reflect.StructField
			wantName string
			wantTag  bool
		}{
			{field: reflect.TypeOf(target{}).Field(1), wantName: "name", wantTag: true},
			{field: reflect.TypeOf(target{}).Field(2), wantName: "DefaultName", wantTag: true},
			{field: reflect.TypeOf(target{}).Field(3), wantName: "Plain", wantTag: false},
			{field: reflect.TypeOf(target{}).Field(4), wantName: "-", wantTag: true},
		}
		for _, tc := range fieldTests {
			got, tagged := jsonFieldName(tc.field)
			if got != tc.wantName || tagged != tc.wantTag {
				t.Fatalf("jsonFieldName(%s) = (%q, %v), want (%q, %v)", tc.field.Name, got, tagged, tc.wantName, tc.wantTag)
			}
		}

		type privateField struct {
			Public int `json:"public"`
			hidden int `json:"hidden"`
		}
		if keys := expectedJSONKeys(reflect.TypeOf(privateField{})); !reflect.DeepEqual(keys, []string{"public"}) {
			t.Fatalf("expectedJSONKeys(privateField) = %v, want [public]", keys)
		}
	})
}

func TestTuningConfigGettersAndActiveConfig(t *testing.T) {
	t.Parallel()

	cfg := sampleValidConfig()
	if cfg.GetSensor() != cfg.L1.Sensor ||
		cfg.GetDataSource() != cfg.L1.DataSource ||
		cfg.GetUDPPort() != cfg.L1.UDPPort ||
		cfg.GetUDPRcvBuf() != cfg.L1.UDPRcvBuf ||
		cfg.GetForwardPort() != cfg.L1.ForwardPort ||
		cfg.GetForegroundForwardPort() != cfg.L1.ForegroundForwardPort ||
		cfg.GetMinFramePoints() != cfg.Pipeline.MinFramePoints ||
		cfg.GetBackgroundFlush() != cfg.Pipeline.BackgroundFlush ||
		cfg.GetNoiseRelative() != cfg.L3.EmaBaselineV1.NoiseRelative ||
		cfg.GetSeedFromFirst() != cfg.L3.EmaBaselineV1.SeedFromFirst ||
		cfg.GetClosenessMultiplier() != cfg.L3.EmaBaselineV1.ClosenessMultiplier ||
		cfg.GetNeighborConfirmationCount() != cfg.L3.EmaBaselineV1.NeighbourConfirmationCount ||
		cfg.GetWarmupDurationNanos() != cfg.L3.EmaBaselineV1.WarmupDurationNanos ||
		cfg.GetWarmupMinFrames() != cfg.L3.EmaBaselineV1.WarmupMinFrames ||
		cfg.GetPostSettleUpdateFraction() != cfg.L3.EmaBaselineV1.PostSettleUpdateFraction ||
		cfg.GetForegroundDBSCANEps() != cfg.L4.DbscanXyV1.ForegroundDBSCANEps ||
		cfg.GetForegroundMinClusterPoints() != cfg.L4.DbscanXyV1.ForegroundMinClusterPoints ||
		cfg.GetForegroundMaxInputPoints() != cfg.L4.DbscanXyV1.ForegroundMaxInputPoints ||
		cfg.GetGatingDistanceSquared() != cfg.L5.CvKfV1.GatingDistanceSquared ||
		cfg.GetProcessNoisePos() != cfg.L5.CvKfV1.ProcessNoisePos ||
		cfg.GetProcessNoiseVel() != cfg.L5.CvKfV1.ProcessNoiseVel ||
		cfg.GetMeasurementNoise() != cfg.L5.CvKfV1.MeasurementNoise ||
		cfg.GetOcclusionCovInflation() != cfg.L5.CvKfV1.OcclusionCovInflation ||
		cfg.GetHitsToConfirm() != cfg.L5.CvKfV1.HitsToConfirm ||
		cfg.GetMaxMisses() != cfg.L5.CvKfV1.MaxMisses ||
		cfg.GetMaxMissesConfirmed() != cfg.L5.CvKfV1.MaxMissesConfirmed ||
		cfg.GetMaxTracks() != cfg.L5.CvKfV1.MaxTracks ||
		cfg.GetBackgroundUpdateFraction() != cfg.L3.EmaBaselineV1.BackgroundUpdateFraction ||
		cfg.GetSafetyMarginMeters() != cfg.L3.EmaBaselineV1.SafetyMarginMetres ||
		cfg.GetEnableDiagnostics() != cfg.L3.EmaBaselineV1.EnableDiagnostics ||
		cfg.GetFreezeThresholdMultiplier() != cfg.L3.EmaBaselineV1.FreezeThresholdMultiplier ||
		cfg.GetChangeThresholdSnapshot() != cfg.L3.EmaBaselineV1.ChangeThresholdSnapshot ||
		cfg.GetReacquisitionBoostMultiplier() != cfg.L3.EmaBaselineV1.ReacquisitionBoostMultiplier ||
		cfg.GetMinConfidenceFloor() != cfg.L3.EmaBaselineV1.MinConfidenceFloor ||
		cfg.GetLockedBaselineThreshold() != cfg.L3.EmaBaselineV1.LockedBaselineThreshold ||
		cfg.GetLockedBaselineMultiplier() != cfg.L3.EmaBaselineV1.LockedBaselineMultiplier ||
		cfg.GetSensorMovementForegroundThreshold() != cfg.L3.EmaBaselineV1.SensorMovementForegroundThreshold ||
		cfg.GetBackgroundDriftThresholdMetres() != cfg.L3.EmaBaselineV1.BackgroundDriftThresholdMetres ||
		cfg.GetBackgroundDriftRatioThreshold() != cfg.L3.EmaBaselineV1.BackgroundDriftRatioThreshold ||
		cfg.GetSettlingMinCoverage() != cfg.L3.EmaBaselineV1.SettlingMinCoverage ||
		cfg.GetSettlingMaxSpreadDelta() != cfg.L3.EmaBaselineV1.SettlingMaxSpreadDelta ||
		cfg.GetSettlingMinRegionStability() != cfg.L3.EmaBaselineV1.SettlingMinRegionStability ||
		cfg.GetSettlingMinConfidence() != cfg.L3.EmaBaselineV1.SettlingMinConfidence ||
		cfg.GetHeightBandFloor() != cfg.L4.DbscanXyV1.HeightBandFloor ||
		cfg.GetHeightBandCeiling() != cfg.L4.DbscanXyV1.HeightBandCeiling ||
		cfg.GetRemoveGround() != cfg.L4.DbscanXyV1.RemoveGround ||
		cfg.GetMaxClusterDiameter() != cfg.L4.DbscanXyV1.MaxClusterDiameter ||
		cfg.GetMinClusterDiameter() != cfg.L4.DbscanXyV1.MinClusterDiameter ||
		cfg.GetMaxClusterAspectRatio() != cfg.L4.DbscanXyV1.MaxClusterAspectRatio ||
		cfg.GetMaxReasonableSpeedMps() != cfg.L5.CvKfV1.MaxReasonableSpeedMps ||
		cfg.GetMaxPositionJumpMeters() != cfg.L5.CvKfV1.MaxPositionJumpMetres ||
		cfg.GetMaxPredictDt() != cfg.L5.CvKfV1.MaxPredictDt ||
		cfg.GetMaxCovarianceDiag() != cfg.L5.CvKfV1.MaxCovarianceDiag ||
		cfg.GetMinPointsForPCA() != cfg.L5.CvKfV1.MinPointsForPCA ||
		cfg.GetOBBHeadingSmoothingAlpha() != cfg.L5.CvKfV1.OBBHeadingSmoothingAlpha ||
		cfg.GetOBBAspectRatioLockThreshold() != cfg.L5.CvKfV1.OBBAspectRatioLockThreshold ||
		cfg.GetMaxTrackHistoryLength() != cfg.L5.CvKfV1.MaxTrackHistoryLength ||
		cfg.GetMaxSpeedHistoryLength() != cfg.L5.CvKfV1.MaxSpeedHistoryLength ||
		cfg.GetMergeSizeRatio() != cfg.L5.CvKfV1.MergeSizeRatio ||
		cfg.GetSplitSizeRatio() != cfg.L5.CvKfV1.SplitSizeRatio ||
		cfg.GetMinObservationsForClassification() != cfg.L5.CvKfV1.MinObservationsForClassification {
		t.Fatal("getter mismatch")
	}
	if cfg.GetFlushInterval() != time.Minute ||
		cfg.GetBufferTimeout() != 500*time.Millisecond ||
		cfg.GetFreezeDuration() != 5*time.Second ||
		cfg.GetSettlingPeriod() != 5*time.Minute ||
		cfg.GetSnapshotInterval() != 2*time.Hour ||
		cfg.GetDeletedTrackGracePeriod() != 5*time.Second {
		t.Fatal("duration getter mismatch")
	}

	if (&L3Config{Engine: "unknown"}).ActiveConfig() != nil || (&L3Config{Engine: "unknown"}).ActiveCommon() != nil {
		t.Fatal("unexpected l3 active config for unknown engine")
	}
	if (&L4Config{Engine: "unknown"}).ActiveConfig() != nil || (&L4Config{Engine: "unknown"}).ActiveCommon() != nil {
		t.Fatal("unexpected l4 active config for unknown engine")
	}
	if (&L5Config{Engine: "unknown"}).ActiveConfig() != nil || (&L5Config{Engine: "unknown"}).ActiveCommon() != nil {
		t.Fatal("unexpected l5 active config for unknown engine")
	}
}

func TestLoadTuningConfigAdditionalErrors(t *testing.T) {
	t.Parallel()

	t.Run("wrong extension", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tuning.txt")
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		requireErrorContains(t, errorOnly(LoadTuningConfig(path)), `config file must have .json extension`)
	})

	t.Run("stat missing", func(t *testing.T) {
		requireErrorContains(t, errorOnly(LoadTuningConfig(filepath.Join(t.TempDir(), "missing.json"))), "failed to stat config file")
	})

	t.Run("too large", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "large.json")
		data := strings.Repeat(" ", (1*1024*1024)+1)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatalf("write large file: %v", err)
		}
		requireErrorContains(t, errorOnly(LoadTuningConfig(path)), "config file too large")
	})

	t.Run("invalid json", func(t *testing.T) {
		path := writeConfigFile(t, []byte(`{"version":`))
		requireErrorContains(t, errorOnly(LoadTuningConfig(path)), "failed to parse config JSON")
	})

	t.Run("validate complete", func(t *testing.T) {
		cfg := sampleValidConfig()
		if err := cfg.ValidateComplete(); err != nil {
			t.Fatalf("ValidateComplete returned error: %v", err)
		}
	})
}

func requireErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func errorOnly[T any](_ T, err error) error {
	return err
}

func errorOrNil[T any](value T, err error) error {
	_ = value
	return err
}
