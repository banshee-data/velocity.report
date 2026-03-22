package config

import "time"

// ActiveConfig returns the selected L3 engine block.
func (c *L3Config) ActiveConfig() interface{} {
	switch c.Engine {
	case "ema_baseline_v1":
		return c.EmaBaselineV1
	case "ema_track_assist_v2":
		return c.EmaTrackAssistV2
	default:
		return nil
	}
}

// ActiveCommon returns the selected L3 common block.
func (c *L3Config) ActiveCommon() *L3Common {
	switch c.Engine {
	case "ema_baseline_v1":
		if c.EmaBaselineV1 != nil {
			return &c.EmaBaselineV1.L3Common
		}
	case "ema_track_assist_v2":
		if c.EmaTrackAssistV2 != nil {
			return &c.EmaTrackAssistV2.L3Common
		}
	}
	return nil
}

// ActiveConfig returns the selected L4 engine block.
func (c *L4Config) ActiveConfig() interface{} {
	switch c.Engine {
	case "dbscan_xy_v1":
		return c.DbscanXyV1
	case "two_stage_mahalanobis_v2":
		return c.TwoStageMahalanobisV2
	case "hdbscan_adaptive_v1":
		return c.HdbscanAdaptiveV1
	default:
		return nil
	}
}

// ActiveCommon returns the selected L4 common block.
func (c *L4Config) ActiveCommon() *L4Common {
	switch c.Engine {
	case "dbscan_xy_v1":
		if c.DbscanXyV1 != nil {
			return &c.DbscanXyV1.L4Common
		}
	case "two_stage_mahalanobis_v2":
		if c.TwoStageMahalanobisV2 != nil {
			return &c.TwoStageMahalanobisV2.L4Common
		}
	case "hdbscan_adaptive_v1":
		if c.HdbscanAdaptiveV1 != nil {
			return &c.HdbscanAdaptiveV1.L4Common
		}
	}
	return nil
}

// ActiveConfig returns the selected L5 engine block.
func (c *L5Config) ActiveConfig() interface{} {
	switch c.Engine {
	case "cv_kf_v1":
		return c.CvKfV1
	case "imm_cv_ca_v2":
		return c.ImmCvCaV2
	case "imm_cv_ca_rts_eval_v2":
		return c.ImmCvCaRtsEvalV2
	default:
		return nil
	}
}

// ActiveCommon returns the selected L5 common block.
func (c *L5Config) ActiveCommon() *L5Common {
	switch c.Engine {
	case "cv_kf_v1":
		if c.CvKfV1 != nil {
			return &c.CvKfV1.L5Common
		}
	case "imm_cv_ca_v2":
		if c.ImmCvCaV2 != nil {
			return &c.ImmCvCaV2.L5Common
		}
	case "imm_cv_ca_rts_eval_v2":
		if c.ImmCvCaRtsEvalV2 != nil {
			return &c.ImmCvCaRtsEvalV2.L5Common
		}
	}
	return nil
}

// GetSensor returns the configured sensor identifier.
func (c *TuningConfig) GetSensor() string { return c.L1.Sensor }

// GetDataSource returns the configured initial data source.
func (c *TuningConfig) GetDataSource() string { return c.L1.DataSource }

// GetFlushInterval parses and returns the flush interval.
func (c *TuningConfig) GetFlushInterval() time.Duration {
	d, _ := time.ParseDuration(c.Pipeline.FlushInterval)
	return d
}

// GetBufferTimeout parses and returns the frame buffer timeout.
func (c *TuningConfig) GetBufferTimeout() time.Duration {
	d, _ := time.ParseDuration(c.Pipeline.BufferTimeout)
	return d
}

// GetNoiseRelative returns the active L3 noise_relative value.
func (c *TuningConfig) GetNoiseRelative() float64 { return c.L3.ActiveCommon().NoiseRelative }

// GetSeedFromFirst returns the active L3 seed_from_first value.
func (c *TuningConfig) GetSeedFromFirst() bool { return c.L3.ActiveCommon().SeedFromFirst }

// GetMinFramePoints returns the pipeline min_frame_points value.
func (c *TuningConfig) GetMinFramePoints() int { return c.Pipeline.MinFramePoints }

// GetBackgroundFlush returns the pipeline background_flush value.
func (c *TuningConfig) GetBackgroundFlush() bool { return c.Pipeline.BackgroundFlush }

// GetClosenessMultiplier returns the active L3 closeness_multiplier value.
func (c *TuningConfig) GetClosenessMultiplier() float64 {
	return c.L3.ActiveCommon().ClosenessMultiplier
}

// GetNeighbourConfirmationCount returns the active L3 neighbour confirmation count.
func (c *TuningConfig) GetNeighbourConfirmationCount() int {
	return c.L3.ActiveCommon().NeighbourConfirmationCount
}

// GetWarmupDurationNanos returns the active L3 warmup duration.
func (c *TuningConfig) GetWarmupDurationNanos() int64 { return c.L3.ActiveCommon().WarmupDurationNanos }

// GetWarmupMinFrames returns the active L3 warmup frame count.
func (c *TuningConfig) GetWarmupMinFrames() int { return c.L3.ActiveCommon().WarmupMinFrames }

// GetPostSettleUpdateFraction returns the active L3 post-settle alpha.
func (c *TuningConfig) GetPostSettleUpdateFraction() float64 {
	return c.L3.ActiveCommon().PostSettleUpdateFraction
}

// GetForegroundDBSCANEps returns the active L4 DBSCAN epsilon.
func (c *TuningConfig) GetForegroundDBSCANEps() float64 {
	return c.L4.ActiveCommon().ForegroundDBSCANEps
}

// GetForegroundMinClusterPoints returns the active L4 minimum cluster size.
func (c *TuningConfig) GetForegroundMinClusterPoints() int {
	return c.L4.ActiveCommon().ForegroundMinClusterPoints
}

// GetForegroundMaxInputPoints returns the active L4 point cap.
func (c *TuningConfig) GetForegroundMaxInputPoints() int {
	return c.L4.ActiveCommon().ForegroundMaxInputPoints
}

// GetGatingDistanceSquared returns the active L5 gating threshold.
func (c *TuningConfig) GetGatingDistanceSquared() float64 {
	return c.L5.ActiveCommon().GatingDistanceSquared
}

// GetProcessNoisePos returns the active L5 position process noise.
func (c *TuningConfig) GetProcessNoisePos() float64 { return c.L5.ActiveCommon().ProcessNoisePos }

// GetProcessNoiseVel returns the active L5 velocity process noise.
func (c *TuningConfig) GetProcessNoiseVel() float64 { return c.L5.ActiveCommon().ProcessNoiseVel }

// GetMeasurementNoise returns the active L5 measurement noise.
func (c *TuningConfig) GetMeasurementNoise() float64 { return c.L5.ActiveCommon().MeasurementNoise }

// GetOcclusionCovInflation returns the active L5 occlusion covariance inflation.
func (c *TuningConfig) GetOcclusionCovInflation() float64 {
	return c.L5.ActiveCommon().OcclusionCovInflation
}

// GetHitsToConfirm returns the active L5 confirmation threshold.
func (c *TuningConfig) GetHitsToConfirm() int { return c.L5.ActiveCommon().HitsToConfirm }

// GetMaxMisses returns the active L5 tentative miss threshold.
func (c *TuningConfig) GetMaxMisses() int { return c.L5.ActiveCommon().MaxMisses }

// GetMaxMissesConfirmed returns the active L5 confirmed miss threshold.
func (c *TuningConfig) GetMaxMissesConfirmed() int { return c.L5.ActiveCommon().MaxMissesConfirmed }

// GetMaxTracks returns the active L5 max_tracks value.
func (c *TuningConfig) GetMaxTracks() int { return c.L5.ActiveCommon().MaxTracks }

// GetBackgroundUpdateFraction returns the active L3 update alpha.
func (c *TuningConfig) GetBackgroundUpdateFraction() float64 {
	return c.L3.ActiveCommon().BackgroundUpdateFraction
}

// GetSafetyMarginMetres returns the active L3 safety margin in metres.
func (c *TuningConfig) GetSafetyMarginMetres() float64 { return c.L3.ActiveCommon().SafetyMarginMetres }

// GetEnableDiagnostics returns the active L3 diagnostics switch.
func (c *TuningConfig) GetEnableDiagnostics() bool { return c.L3.ActiveCommon().EnableDiagnostics }

// GetFreezeDuration returns the active L3 freeze duration.
func (c *TuningConfig) GetFreezeDuration() time.Duration {
	d, _ := time.ParseDuration(c.L3.ActiveCommon().FreezeDuration)
	return d
}

// GetFreezeThresholdMultiplier returns the active L3 freeze threshold multiplier.
func (c *TuningConfig) GetFreezeThresholdMultiplier() float64 {
	return c.L3.ActiveCommon().FreezeThresholdMultiplier
}

// GetSettlingPeriod returns the active L3 settling period.
func (c *TuningConfig) GetSettlingPeriod() time.Duration {
	d, _ := time.ParseDuration(c.L3.ActiveCommon().SettlingPeriod)
	return d
}

// GetSnapshotInterval returns the active L3 snapshot interval.
func (c *TuningConfig) GetSnapshotInterval() time.Duration {
	d, _ := time.ParseDuration(c.L3.ActiveCommon().SnapshotInterval)
	return d
}

// GetChangeThresholdSnapshot returns the active L3 snapshot change threshold.
func (c *TuningConfig) GetChangeThresholdSnapshot() int {
	return c.L3.ActiveCommon().ChangeThresholdSnapshot
}

// GetReacquisitionBoostMultiplier returns the active L3 reacquisition boost.
func (c *TuningConfig) GetReacquisitionBoostMultiplier() float64 {
	return c.L3.ActiveCommon().ReacquisitionBoostMultiplier
}

// GetMinConfidenceFloor returns the active L3 minimum confidence floor.
func (c *TuningConfig) GetMinConfidenceFloor() int { return c.L3.ActiveCommon().MinConfidenceFloor }

// GetLockedBaselineThreshold returns the active L3 locked-baseline threshold.
func (c *TuningConfig) GetLockedBaselineThreshold() int {
	return c.L3.ActiveCommon().LockedBaselineThreshold
}

// GetLockedBaselineMultiplier returns the active L3 locked-baseline multiplier.
func (c *TuningConfig) GetLockedBaselineMultiplier() float64 {
	return c.L3.ActiveCommon().LockedBaselineMultiplier
}

// GetSensorMovementForegroundThreshold returns the active L3 movement threshold.
func (c *TuningConfig) GetSensorMovementForegroundThreshold() float64 {
	return c.L3.ActiveCommon().SensorMovementForegroundThreshold
}

// GetBackgroundDriftThresholdMetres returns the active L3 drift distance threshold.
func (c *TuningConfig) GetBackgroundDriftThresholdMetres() float64 {
	return c.L3.ActiveCommon().BackgroundDriftThresholdMetres
}

// GetBackgroundDriftRatioThreshold returns the active L3 drift ratio threshold.
func (c *TuningConfig) GetBackgroundDriftRatioThreshold() float64 {
	return c.L3.ActiveCommon().BackgroundDriftRatioThreshold
}

// GetSettlingMinCoverage returns the active L3 minimum settling coverage.
func (c *TuningConfig) GetSettlingMinCoverage() float64 {
	return c.L3.ActiveCommon().SettlingMinCoverage
}

// GetSettlingMaxSpreadDelta returns the active L3 maximum settling spread delta.
func (c *TuningConfig) GetSettlingMaxSpreadDelta() float64 {
	return c.L3.ActiveCommon().SettlingMaxSpreadDelta
}

// GetSettlingMinRegionStability returns the active L3 minimum region stability.
func (c *TuningConfig) GetSettlingMinRegionStability() float64 {
	return c.L3.ActiveCommon().SettlingMinRegionStability
}

// GetSettlingMinConfidence returns the active L3 minimum settling confidence.
func (c *TuningConfig) GetSettlingMinConfidence() float64 {
	return c.L3.ActiveCommon().SettlingMinConfidence
}

// GetHeightBandFloor returns the active L4 lower height-band bound.
func (c *TuningConfig) GetHeightBandFloor() float64 { return c.L4.ActiveCommon().HeightBandFloor }

// GetHeightBandCeiling returns the active L4 upper height-band bound.
func (c *TuningConfig) GetHeightBandCeiling() float64 { return c.L4.ActiveCommon().HeightBandCeiling }

// GetRemoveGround returns the active L4 ground-removal switch.
func (c *TuningConfig) GetRemoveGround() bool { return c.L4.ActiveCommon().RemoveGround }

// GetMaxClusterDiameter returns the active L4 max cluster diameter.
func (c *TuningConfig) GetMaxClusterDiameter() float64 { return c.L4.ActiveCommon().MaxClusterDiameter }

// GetMinClusterDiameter returns the active L4 min cluster diameter.
func (c *TuningConfig) GetMinClusterDiameter() float64 { return c.L4.ActiveCommon().MinClusterDiameter }

// GetMaxClusterAspectRatio returns the active L4 max cluster aspect ratio.
func (c *TuningConfig) GetMaxClusterAspectRatio() float64 {
	return c.L4.ActiveCommon().MaxClusterAspectRatio
}

// GetMaxReasonableSpeedMps returns the active L5 max speed limit.
func (c *TuningConfig) GetMaxReasonableSpeedMps() float64 {
	return c.L5.ActiveCommon().MaxReasonableSpeedMps
}

// GetMaxPositionJumpMetres returns the active L5 max position jump.
func (c *TuningConfig) GetMaxPositionJumpMetres() float64 {
	return c.L5.ActiveCommon().MaxPositionJumpMetres
}

// GetMaxPredictDt returns the active L5 max predict dt.
func (c *TuningConfig) GetMaxPredictDt() float64 { return c.L5.ActiveCommon().MaxPredictDt }

// GetMaxCovarianceDiag returns the active L5 covariance clamp.
func (c *TuningConfig) GetMaxCovarianceDiag() float64 { return c.L5.ActiveCommon().MaxCovarianceDiag }

// GetMinPointsForPCA returns the active L5 PCA minimum.
func (c *TuningConfig) GetMinPointsForPCA() int { return c.L5.ActiveCommon().MinPointsForPCA }

// GetOBBHeadingSmoothingAlpha returns the active L5 OBB smoothing factor.
func (c *TuningConfig) GetOBBHeadingSmoothingAlpha() float64 {
	return c.L5.ActiveCommon().OBBHeadingSmoothingAlpha
}

// GetOBBAspectRatioLockThreshold returns the active L5 OBB aspect-ratio lock threshold.
func (c *TuningConfig) GetOBBAspectRatioLockThreshold() float64 {
	return c.L5.ActiveCommon().OBBAspectRatioLockThreshold
}

// GetMaxTrackHistoryLength returns the active L5 trail history cap.
func (c *TuningConfig) GetMaxTrackHistoryLength() int {
	return c.L5.ActiveCommon().MaxTrackHistoryLength
}

// GetMaxSpeedHistoryLength returns the active L5 speed history cap.
func (c *TuningConfig) GetMaxSpeedHistoryLength() int {
	return c.L5.ActiveCommon().MaxSpeedHistoryLength
}

// GetMergeSizeRatio returns the active L5 merge ratio.
func (c *TuningConfig) GetMergeSizeRatio() float64 { return c.L5.ActiveCommon().MergeSizeRatio }

// GetSplitSizeRatio returns the active L5 split ratio.
func (c *TuningConfig) GetSplitSizeRatio() float64 { return c.L5.ActiveCommon().SplitSizeRatio }

// GetDeletedTrackGracePeriod returns the active L5 deleted-track grace period.
func (c *TuningConfig) GetDeletedTrackGracePeriod() time.Duration {
	d, _ := time.ParseDuration(c.L5.ActiveCommon().DeletedTrackGracePeriod)
	return d
}

// GetMinObservationsForClassification returns the active L5 classification threshold.
func (c *TuningConfig) GetMinObservationsForClassification() int {
	return c.L5.ActiveCommon().MinObservationsForClassification
}
