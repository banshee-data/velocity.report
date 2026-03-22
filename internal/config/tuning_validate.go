package config

import (
	"fmt"
	"strings"
	"time"
)

// Validate checks the loaded config for structural and value correctness.
func (c *TuningConfig) Validate() error {
	if c.Version != CurrentConfigVersion {
		return fmt.Errorf("version must equal %d, got %d", CurrentConfigVersion, c.Version)
	}
	if err := c.L1.Validate(); err != nil {
		return fmt.Errorf("l1: %w", err)
	}
	if err := c.L3.Validate(); err != nil {
		return fmt.Errorf("l3: %w", err)
	}
	if err := c.L4.Validate(); err != nil {
		return fmt.Errorf("l4: %w", err)
	}
	if err := c.L5.Validate(); err != nil {
		return fmt.Errorf("l5: %w", err)
	}
	if err := c.Pipeline.Validate(); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	return nil
}

// ValidateComplete is kept as a compatibility entry point for callers and tooling.
func (c *TuningConfig) ValidateComplete() error {
	return c.Validate()
}

// Validate validates L1 values.
func (c *L1Config) Validate() error {
	if strings.TrimSpace(c.Sensor) == "" {
		return fmt.Errorf("sensor must be non-empty")
	}
	switch c.DataSource {
	case "live", "pcap", "pcap_analysis":
	default:
		return fmt.Errorf("data_source must be one of live, pcap, pcap_analysis, got %q", c.DataSource)
	}
	if c.UDPPort <= 0 || c.UDPPort > 65535 {
		return fmt.Errorf("udp_port must be in [1, 65535], got %d", c.UDPPort)
	}
	if c.UDPRcvBuf <= 0 {
		return fmt.Errorf("udp_rcv_buf must be positive, got %d", c.UDPRcvBuf)
	}
	if err := validateOptionalPort("forward_port", c.ForwardPort); err != nil {
		return err
	}
	if err := validateOptionalPort("foreground_forward_port", c.ForegroundForwardPort); err != nil {
		return err
	}
	return nil
}

// Validate validates pipeline values.
func (c *PipelineConfig) Validate() error {
	if _, err := time.ParseDuration(c.BufferTimeout); err != nil {
		return fmt.Errorf("invalid buffer_timeout %q: %w", c.BufferTimeout, err)
	}
	if c.MinFramePoints < 0 {
		return fmt.Errorf("min_frame_points must be non-negative, got %d", c.MinFramePoints)
	}
	if _, err := time.ParseDuration(c.FlushInterval); err != nil {
		return fmt.Errorf("invalid flush_interval %q: %w", c.FlushInterval, err)
	}
	return nil
}

// Validate validates the selected L3 engine and its block.
func (c *L3Config) Validate() error {
	switch c.Engine {
	case "ema_baseline_v1":
		if c.EmaBaselineV1 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.EmaBaselineV1.Validate()
	case "ema_track_assist_v2":
		if c.EmaTrackAssistV2 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.EmaTrackAssistV2.Validate()
	default:
		return fmt.Errorf("unknown engine %q", c.Engine)
	}
}

// Validate validates the selected L4 engine and its block.
func (c *L4Config) Validate() error {
	switch c.Engine {
	case "dbscan_xy_v1":
		if c.DbscanXyV1 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.DbscanXyV1.Validate()
	case "two_stage_mahalanobis_v2":
		if c.TwoStageMahalanobisV2 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.TwoStageMahalanobisV2.Validate()
	case "hdbscan_adaptive_v1":
		if c.HdbscanAdaptiveV1 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.HdbscanAdaptiveV1.Validate()
	default:
		return fmt.Errorf("unknown engine %q", c.Engine)
	}
}

// Validate validates the selected L5 engine and its block.
func (c *L5Config) Validate() error {
	switch c.Engine {
	case "cv_kf_v1":
		if c.CvKfV1 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.CvKfV1.Validate()
	case "imm_cv_ca_v2":
		if c.ImmCvCaV2 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.ImmCvCaV2.Validate()
	case "imm_cv_ca_rts_eval_v2":
		if c.ImmCvCaRtsEvalV2 == nil {
			return fmt.Errorf("selected engine block %q missing", c.Engine)
		}
		return c.ImmCvCaRtsEvalV2.Validate()
	default:
		return fmt.Errorf("unknown engine %q", c.Engine)
	}
}

// Validate validates common L3 fields.
func (c *L3Common) Validate() error {
	if c.BackgroundUpdateFraction <= 0 || c.BackgroundUpdateFraction > 1 {
		return fmt.Errorf("background_update_fraction must be in (0, 1], got %f", c.BackgroundUpdateFraction)
	}
	if c.ClosenessMultiplier <= 0 {
		return fmt.Errorf("closeness_multiplier must be positive, got %f", c.ClosenessMultiplier)
	}
	if c.SafetyMarginMetres < 0 {
		return fmt.Errorf("safety_margin_metres must be non-negative, got %f", c.SafetyMarginMetres)
	}
	if c.NoiseRelative < 0 || c.NoiseRelative > 1 {
		return fmt.Errorf("noise_relative must be between 0 and 1, got %f", c.NoiseRelative)
	}
	if c.NeighbourConfirmationCount < 0 || c.NeighbourConfirmationCount > 8 {
		return fmt.Errorf("neighbour_confirmation_count must be in [0, 8], got %d", c.NeighbourConfirmationCount)
	}
	if c.WarmupDurationNanos < 0 {
		return fmt.Errorf("warmup_duration_nanos must be non-negative, got %d", c.WarmupDurationNanos)
	}
	if c.WarmupMinFrames < 0 {
		return fmt.Errorf("warmup_min_frames must be non-negative, got %d", c.WarmupMinFrames)
	}
	if c.PostSettleUpdateFraction < 0 || c.PostSettleUpdateFraction > 1 {
		return fmt.Errorf("post_settle_update_fraction must be in [0, 1], got %f", c.PostSettleUpdateFraction)
	}
	if _, err := time.ParseDuration(c.FreezeDuration); err != nil {
		return fmt.Errorf("invalid freeze_duration %q: %w", c.FreezeDuration, err)
	}
	if c.FreezeThresholdMultiplier <= 0 {
		return fmt.Errorf("freeze_threshold_multiplier must be positive, got %f", c.FreezeThresholdMultiplier)
	}
	if _, err := time.ParseDuration(c.SettlingPeriod); err != nil {
		return fmt.Errorf("invalid settling_period %q: %w", c.SettlingPeriod, err)
	}
	if _, err := time.ParseDuration(c.SnapshotInterval); err != nil {
		return fmt.Errorf("invalid snapshot_interval %q: %w", c.SnapshotInterval, err)
	}
	if c.ChangeThresholdSnapshot < 0 {
		return fmt.Errorf("change_threshold_snapshot must be non-negative, got %d", c.ChangeThresholdSnapshot)
	}
	if c.ReacquisitionBoostMultiplier < 0 {
		return fmt.Errorf("reacquisition_boost_multiplier must be non-negative, got %f", c.ReacquisitionBoostMultiplier)
	}
	if c.MinConfidenceFloor < 0 {
		return fmt.Errorf("min_confidence_floor must be non-negative, got %d", c.MinConfidenceFloor)
	}
	if c.LockedBaselineThreshold < 0 {
		return fmt.Errorf("locked_baseline_threshold must be non-negative, got %d", c.LockedBaselineThreshold)
	}
	if c.LockedBaselineMultiplier < 0 {
		return fmt.Errorf("locked_baseline_multiplier must be non-negative, got %f", c.LockedBaselineMultiplier)
	}
	if c.SensorMovementForegroundThreshold < 0 || c.SensorMovementForegroundThreshold > 1 {
		return fmt.Errorf("sensor_movement_foreground_threshold must be in [0, 1], got %f", c.SensorMovementForegroundThreshold)
	}
	if c.BackgroundDriftThresholdMetres < 0 {
		return fmt.Errorf("background_drift_threshold_metres must be non-negative, got %f", c.BackgroundDriftThresholdMetres)
	}
	if c.BackgroundDriftRatioThreshold < 0 || c.BackgroundDriftRatioThreshold > 1 {
		return fmt.Errorf("background_drift_ratio_threshold must be in [0, 1], got %f", c.BackgroundDriftRatioThreshold)
	}
	if c.SettlingMinCoverage < 0 || c.SettlingMinCoverage > 1 {
		return fmt.Errorf("settling_min_coverage must be in [0, 1], got %f", c.SettlingMinCoverage)
	}
	if c.SettlingMaxSpreadDelta < 0 {
		return fmt.Errorf("settling_max_spread_delta must be non-negative, got %f", c.SettlingMaxSpreadDelta)
	}
	if c.SettlingMinRegionStability < 0 || c.SettlingMinRegionStability > 1 {
		return fmt.Errorf("settling_min_region_stability must be in [0, 1], got %f", c.SettlingMinRegionStability)
	}
	if c.SettlingMinConfidence < 0 {
		return fmt.Errorf("settling_min_confidence must be non-negative, got %f", c.SettlingMinConfidence)
	}
	return nil
}

// Validate validates the production L3 engine.
func (c *L3EmaBaselineV1) Validate() error {
	return c.L3Common.Validate()
}

// Validate validates the track-assist L3 engine.
func (c *L3EmaTrackAssistV2) Validate() error {
	if err := c.L3Common.Validate(); err != nil {
		return err
	}
	if c.PromotionNearGateLow < 0 {
		return fmt.Errorf("promotion_near_gate_low must be non-negative, got %f", c.PromotionNearGateLow)
	}
	if c.PromotionNearGateHigh < 0 {
		return fmt.Errorf("promotion_near_gate_high must be non-negative, got %f", c.PromotionNearGateHigh)
	}
	if c.PromotionThreshold < 0 {
		return fmt.Errorf("promotion_threshold must be non-negative, got %f", c.PromotionThreshold)
	}
	return nil
}

// Validate validates common L4 fields.
func (c *L4Common) Validate() error {
	if c.ForegroundDBSCANEps <= 0 {
		return fmt.Errorf("foreground_dbscan_eps must be positive, got %f", c.ForegroundDBSCANEps)
	}
	if c.ForegroundMinClusterPoints < 1 {
		return fmt.Errorf("foreground_min_cluster_points must be >= 1, got %d", c.ForegroundMinClusterPoints)
	}
	if c.ForegroundMaxInputPoints < 1 {
		return fmt.Errorf("foreground_max_input_points must be >= 1, got %d", c.ForegroundMaxInputPoints)
	}
	if c.HeightBandFloor > c.HeightBandCeiling {
		return fmt.Errorf("height_band_floor must be <= height_band_ceiling, got %f > %f", c.HeightBandFloor, c.HeightBandCeiling)
	}
	if c.MaxClusterDiameter <= 0 {
		return fmt.Errorf("max_cluster_diameter must be positive, got %f", c.MaxClusterDiameter)
	}
	if c.MinClusterDiameter <= 0 {
		return fmt.Errorf("min_cluster_diameter must be positive, got %f", c.MinClusterDiameter)
	}
	if c.MaxClusterAspectRatio <= 0 {
		return fmt.Errorf("max_cluster_aspect_ratio must be positive, got %f", c.MaxClusterAspectRatio)
	}
	return nil
}

// Validate validates the production L4 engine.
func (c *L4DbscanXyV1) Validate() error {
	return c.L4Common.Validate()
}

// Validate validates the two-stage Mahalanobis L4 engine.
func (c *L4TwoStageMahalanobisV2) Validate() error {
	if err := c.L4Common.Validate(); err != nil {
		return err
	}
	if c.VelocityCoherenceGate <= 0 {
		return fmt.Errorf("velocity_coherence_gate must be positive, got %f", c.VelocityCoherenceGate)
	}
	if c.MinVelocityConfidence < 0 || c.MinVelocityConfidence > 1 {
		return fmt.Errorf("min_velocity_confidence must be in [0, 1], got %f", c.MinVelocityConfidence)
	}
	return nil
}

// Validate validates the HDBSCAN L4 engine.
func (c *L4HdbscanAdaptiveV1) Validate() error {
	if err := c.L4Common.Validate(); err != nil {
		return err
	}
	if c.MinClusterSize < 1 {
		return fmt.Errorf("min_cluster_size must be >= 1, got %d", c.MinClusterSize)
	}
	if c.MinSamples < 1 {
		return fmt.Errorf("min_samples must be >= 1, got %d", c.MinSamples)
	}
	return nil
}

// Validate validates common L5 fields.
func (c *L5Common) Validate() error {
	if c.GatingDistanceSquared <= 0 {
		return fmt.Errorf("gating_distance_squared must be positive, got %f", c.GatingDistanceSquared)
	}
	if c.ProcessNoisePos <= 0 {
		return fmt.Errorf("process_noise_pos must be positive, got %f", c.ProcessNoisePos)
	}
	if c.ProcessNoiseVel <= 0 {
		return fmt.Errorf("process_noise_vel must be positive, got %f", c.ProcessNoiseVel)
	}
	if c.MeasurementNoise <= 0 {
		return fmt.Errorf("measurement_noise must be positive, got %f", c.MeasurementNoise)
	}
	if c.OcclusionCovInflation < 0 {
		return fmt.Errorf("occlusion_cov_inflation must be non-negative, got %f", c.OcclusionCovInflation)
	}
	if c.HitsToConfirm < 1 {
		return fmt.Errorf("hits_to_confirm must be >= 1, got %d", c.HitsToConfirm)
	}
	if c.MaxMisses < 1 {
		return fmt.Errorf("max_misses must be >= 1, got %d", c.MaxMisses)
	}
	if c.MaxMissesConfirmed < 1 {
		return fmt.Errorf("max_misses_confirmed must be >= 1, got %d", c.MaxMissesConfirmed)
	}
	if c.MaxTracks < 1 || c.MaxTracks > 1000 {
		return fmt.Errorf("max_tracks must be in [1, 1000], got %d", c.MaxTracks)
	}
	if c.MaxReasonableSpeedMps <= 0 {
		return fmt.Errorf("max_reasonable_speed_mps must be positive, got %f", c.MaxReasonableSpeedMps)
	}
	if c.MaxPositionJumpMetres <= 0 {
		return fmt.Errorf("max_position_jump_metres must be positive, got %f", c.MaxPositionJumpMetres)
	}
	if c.MaxPredictDt <= 0 {
		return fmt.Errorf("max_predict_dt must be positive, got %f", c.MaxPredictDt)
	}
	if c.MaxCovarianceDiag <= 0 {
		return fmt.Errorf("max_covariance_diag must be positive, got %f", c.MaxCovarianceDiag)
	}
	if c.MinPointsForPCA < 1 {
		return fmt.Errorf("min_points_for_pca must be >= 1, got %d", c.MinPointsForPCA)
	}
	if c.OBBHeadingSmoothingAlpha < 0 || c.OBBHeadingSmoothingAlpha > 1 {
		return fmt.Errorf("obb_heading_smoothing_alpha must be in [0, 1], got %f", c.OBBHeadingSmoothingAlpha)
	}
	if c.OBBAspectRatioLockThreshold < 0 {
		return fmt.Errorf("obb_aspect_ratio_lock_threshold must be non-negative, got %f", c.OBBAspectRatioLockThreshold)
	}
	if c.MaxTrackHistoryLength < 1 {
		return fmt.Errorf("max_track_history_length must be >= 1, got %d", c.MaxTrackHistoryLength)
	}
	if c.MaxSpeedHistoryLength < 1 {
		return fmt.Errorf("max_speed_history_length must be >= 1, got %d", c.MaxSpeedHistoryLength)
	}
	if c.MergeSizeRatio <= 0 {
		return fmt.Errorf("merge_size_ratio must be positive, got %f", c.MergeSizeRatio)
	}
	if c.SplitSizeRatio <= 0 {
		return fmt.Errorf("split_size_ratio must be positive, got %f", c.SplitSizeRatio)
	}
	if _, err := time.ParseDuration(c.DeletedTrackGracePeriod); err != nil {
		return fmt.Errorf("invalid deleted_track_grace_period %q: %w", c.DeletedTrackGracePeriod, err)
	}
	if c.MinObservationsForClassification < 1 {
		return fmt.Errorf("min_observations_for_classification must be >= 1, got %d", c.MinObservationsForClassification)
	}
	return nil
}

// Validate validates the production L5 engine.
func (c *L5CvKfV1) Validate() error {
	return c.L5Common.Validate()
}

// Validate validates the IMM L5 engine.
func (c *L5ImmCvCaV2) Validate() error {
	if err := c.L5Common.Validate(); err != nil {
		return err
	}
	if c.TransitionCVToCA < 0 || c.TransitionCVToCA > 1 {
		return fmt.Errorf("transition_cv_to_ca must be in [0, 1], got %f", c.TransitionCVToCA)
	}
	if c.TransitionCAToCV < 0 || c.TransitionCAToCV > 1 {
		return fmt.Errorf("transition_ca_to_cv must be in [0, 1], got %f", c.TransitionCAToCV)
	}
	if c.CAProcessNoiseAcc <= 0 {
		return fmt.Errorf("ca_process_noise_acc must be positive, got %f", c.CAProcessNoiseAcc)
	}
	if c.LowSpeedHeadingFreezeMps < 0 {
		return fmt.Errorf("low_speed_heading_freeze_mps must be non-negative, got %f", c.LowSpeedHeadingFreezeMps)
	}
	return nil
}

// Validate validates the IMM + RTS L5 engine.
func (c *L5ImmCvCaRtsEvalV2) Validate() error {
	if err := c.L5ImmCvCaV2.Validate(); err != nil {
		return err
	}
	if c.RTSSmoothingWindow < 1 {
		return fmt.Errorf("rts_smoothing_window must be >= 1, got %d", c.RTSSmoothingWindow)
	}
	return nil
}

func validateOptionalPort(name string, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("%s must be in [0, 65535], got %d", name, port)
	}
	return nil
}
