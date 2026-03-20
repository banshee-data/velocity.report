package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// DefaultConfigPath is the path to the canonical tuning defaults file.
const DefaultConfigPath = "config/tuning.defaults.json"

// CurrentConfigVersion is the only config schema version accepted by the binary.
const CurrentConfigVersion = 2

// TuningConfig is the Phase 1 + 2 layer-scoped tuning schema.
type TuningConfig struct {
	Version  int            `json:"version"`
	L1       L1Config       `json:"l1"`
	L3       L3Config       `json:"l3"`
	L4       L4Config       `json:"l4"`
	L5       L5Config       `json:"l5"`
	Pipeline PipelineConfig `json:"pipeline"`
}

// L1Config holds sensor and network settings.
type L1Config struct {
	Sensor                string `json:"sensor"`
	DataSource            string `json:"data_source"`
	UDPPort               int    `json:"udp_port"`
	UDPRcvBuf             int    `json:"udp_rcv_buf"`
	ForwardPort           int    `json:"forward_port"`
	ForegroundForwardPort int    `json:"foreground_forward_port"`
}

// PipelineConfig holds cross-cutting runtime settings already exposed pre-restructure.
type PipelineConfig struct {
	BufferTimeout   string `json:"buffer_timeout"`
	MinFramePoints  int    `json:"min_frame_points"`
	FlushInterval   string `json:"flush_interval"`
	BackgroundFlush bool   `json:"background_flush"`
}

// L3Config selects the active L3 engine.
type L3Config struct {
	Engine           string              `json:"engine"`
	EmaBaselineV1    *L3EmaBaselineV1    `json:"ema_baseline_v1,omitempty"`
	EmaTrackAssistV2 *L3EmaTrackAssistV2 `json:"ema_track_assist_v2,omitempty"`
}

// L3Common contains fields shared by all L3 engines.
type L3Common struct {
	BackgroundUpdateFraction          float64 `json:"background_update_fraction"`
	ClosenessMultiplier               float64 `json:"closeness_multiplier"`
	SafetyMarginMetres                float64 `json:"safety_margin_metres"`
	NoiseRelative                     float64 `json:"noise_relative"`
	NeighbourConfirmationCount        int     `json:"neighbour_confirmation_count"`
	SeedFromFirst                     bool    `json:"seed_from_first"`
	WarmupDurationNanos               int64   `json:"warmup_duration_nanos"`
	WarmupMinFrames                   int     `json:"warmup_min_frames"`
	PostSettleUpdateFraction          float64 `json:"post_settle_update_fraction"`
	EnableDiagnostics                 bool    `json:"enable_diagnostics"`
	FreezeDuration                    string  `json:"freeze_duration"`
	FreezeThresholdMultiplier         float64 `json:"freeze_threshold_multiplier"`
	SettlingPeriod                    string  `json:"settling_period"`
	SnapshotInterval                  string  `json:"snapshot_interval"`
	ChangeThresholdSnapshot           int     `json:"change_threshold_snapshot"`
	ReacquisitionBoostMultiplier      float64 `json:"reacquisition_boost_multiplier"`
	MinConfidenceFloor                int     `json:"min_confidence_floor"`
	LockedBaselineThreshold           int     `json:"locked_baseline_threshold"`
	LockedBaselineMultiplier          float64 `json:"locked_baseline_multiplier"`
	SensorMovementForegroundThreshold float64 `json:"sensor_movement_foreground_threshold"`
	BackgroundDriftThresholdMetres    float64 `json:"background_drift_threshold_metres"`
	BackgroundDriftRatioThreshold     float64 `json:"background_drift_ratio_threshold"`
	SettlingMinCoverage               float64 `json:"settling_min_coverage"`
	SettlingMaxSpreadDelta            float64 `json:"settling_max_spread_delta"`
	SettlingMinRegionStability        float64 `json:"settling_min_region_stability"`
	SettlingMinConfidence             float64 `json:"settling_min_confidence"`
}

// L3EmaBaselineV1 is the current production L3 engine.
type L3EmaBaselineV1 struct {
	L3Common
}

// L3EmaTrackAssistV2 is the track-assisted L3 variant.
type L3EmaTrackAssistV2 struct {
	L3Common
	PromotionNearGateLow  float64 `json:"promotion_near_gate_low"`
	PromotionNearGateHigh float64 `json:"promotion_near_gate_high"`
	PromotionThreshold    float64 `json:"promotion_threshold"`
}

// L4Config selects the active L4 engine.
type L4Config struct {
	Engine                string                   `json:"engine"`
	DbscanXyV1            *L4DbscanXyV1            `json:"dbscan_xy_v1,omitempty"`
	TwoStageMahalanobisV2 *L4TwoStageMahalanobisV2 `json:"two_stage_mahalanobis_v2,omitempty"`
	HdbscanAdaptiveV1     *L4HdbscanAdaptiveV1     `json:"hdbscan_adaptive_v1,omitempty"`
}

// L4Common contains fields shared by all L4 engines.
type L4Common struct {
	ForegroundDBSCANEps        float64 `json:"foreground_dbscan_eps"`
	ForegroundMinClusterPoints int     `json:"foreground_min_cluster_points"`
	ForegroundMaxInputPoints   int     `json:"foreground_max_input_points"`
	HeightBandFloor            float64 `json:"height_band_floor"`
	HeightBandCeiling          float64 `json:"height_band_ceiling"`
	RemoveGround               bool    `json:"remove_ground"`
	MaxClusterDiameter         float64 `json:"max_cluster_diameter"`
	MinClusterDiameter         float64 `json:"min_cluster_diameter"`
	MaxClusterAspectRatio      float64 `json:"max_cluster_aspect_ratio"`
}

// L4DbscanXyV1 is the current production L4 engine.
type L4DbscanXyV1 struct {
	L4Common
}

// L4TwoStageMahalanobisV2 is the future velocity-coherent L4 variant.
type L4TwoStageMahalanobisV2 struct {
	L4Common
	VelocityCoherenceGate float64 `json:"velocity_coherence_gate"`
	MinVelocityConfidence float64 `json:"min_velocity_confidence"`
}

// L4HdbscanAdaptiveV1 is the future adaptive HDBSCAN L4 variant.
type L4HdbscanAdaptiveV1 struct {
	L4Common
	MinClusterSize int `json:"min_cluster_size"`
	MinSamples     int `json:"min_samples"`
}

// L5Config selects the active L5 engine.
type L5Config struct {
	Engine           string              `json:"engine"`
	CvKfV1           *L5CvKfV1           `json:"cv_kf_v1,omitempty"`
	ImmCvCaV2        *L5ImmCvCaV2        `json:"imm_cv_ca_v2,omitempty"`
	ImmCvCaRtsEvalV2 *L5ImmCvCaRtsEvalV2 `json:"imm_cv_ca_rts_eval_v2,omitempty"`
}

// L5Common contains fields shared by all L5 engines.
type L5Common struct {
	GatingDistanceSquared            float64 `json:"gating_distance_squared"`
	ProcessNoisePos                  float64 `json:"process_noise_pos"`
	ProcessNoiseVel                  float64 `json:"process_noise_vel"`
	MeasurementNoise                 float64 `json:"measurement_noise"`
	OcclusionCovInflation            float64 `json:"occlusion_cov_inflation"`
	HitsToConfirm                    int     `json:"hits_to_confirm"`
	MaxMisses                        int     `json:"max_misses"`
	MaxMissesConfirmed               int     `json:"max_misses_confirmed"`
	MaxTracks                        int     `json:"max_tracks"`
	MaxReasonableSpeedMps            float64 `json:"max_reasonable_speed_mps"`
	MaxPositionJumpMetres            float64 `json:"max_position_jump_metres"`
	MaxPredictDt                     float64 `json:"max_predict_dt"`
	MaxCovarianceDiag                float64 `json:"max_covariance_diag"`
	MinPointsForPCA                  int     `json:"min_points_for_pca"`
	OBBHeadingSmoothingAlpha         float64 `json:"obb_heading_smoothing_alpha"`
	OBBAspectRatioLockThreshold      float64 `json:"obb_aspect_ratio_lock_threshold"`
	MaxTrackHistoryLength            int     `json:"max_track_history_length"`
	MaxSpeedHistoryLength            int     `json:"max_speed_history_length"`
	MergeSizeRatio                   float64 `json:"merge_size_ratio"`
	SplitSizeRatio                   float64 `json:"split_size_ratio"`
	DeletedTrackGracePeriod          string  `json:"deleted_track_grace_period"`
	MinObservationsForClassification int     `json:"min_observations_for_classification"`
}

// L5CvKfV1 is the current production L5 engine.
type L5CvKfV1 struct {
	L5Common
}

// L5ImmCvCaV2 is the future IMM CV/CA tracking variant.
type L5ImmCvCaV2 struct {
	L5Common
	TransitionCVToCA         float64 `json:"transition_cv_to_ca"`
	TransitionCAToCV         float64 `json:"transition_ca_to_cv"`
	CAProcessNoiseAcc        float64 `json:"ca_process_noise_acc"`
	LowSpeedHeadingFreezeMps float64 `json:"low_speed_heading_freeze_mps"`
}

// L5ImmCvCaRtsEvalV2 is the future IMM + RTS evaluation variant.
type L5ImmCvCaRtsEvalV2 struct {
	L5ImmCvCaV2
	RTSSmoothingWindow int `json:"rts_smoothing_window"`
}

// EngineSpec describes one selectable engine variant.
type EngineSpec struct {
	Layer     string
	NewConfig func() interface{}
}

var engineRegistry = map[string]EngineSpec{
	// L3
	"ema_baseline_v1":     {Layer: "l3", NewConfig: func() interface{} { return &L3EmaBaselineV1{} }},
	"ema_track_assist_v2": {Layer: "l3", NewConfig: func() interface{} { return &L3EmaTrackAssistV2{} }},
	// L4
	"dbscan_xy_v1":             {Layer: "l4", NewConfig: func() interface{} { return &L4DbscanXyV1{} }},
	"two_stage_mahalanobis_v2": {Layer: "l4", NewConfig: func() interface{} { return &L4TwoStageMahalanobisV2{} }},
	"hdbscan_adaptive_v1":      {Layer: "l4", NewConfig: func() interface{} { return &L4HdbscanAdaptiveV1{} }},
	// L5
	"cv_kf_v1":              {Layer: "l5", NewConfig: func() interface{} { return &L5CvKfV1{} }},
	"imm_cv_ca_v2":          {Layer: "l5", NewConfig: func() interface{} { return &L5ImmCvCaV2{} }},
	"imm_cv_ca_rts_eval_v2": {Layer: "l5", NewConfig: func() interface{} { return &L5ImmCvCaRtsEvalV2{} }},
}

// LoadTuningConfig loads a versioned tuning config from a JSON file.
func LoadTuningConfig(path string) (*TuningConfig, error) {
	cleanPath := filepath.Clean(path)
	if ext := filepath.Ext(cleanPath); ext != ".json" {
		return nil, fmt.Errorf("config file must have .json extension, got %q", ext)
	}

	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}
	const maxFileSize = 1 * 1024 * 1024
	if fileInfo.Size() > maxFileSize {
		return nil, fmt.Errorf("config file too large: %d bytes (max %d)", fileInfo.Size(), maxFileSize)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg TuningConfig
	if err := strictDecodeObject(data, &cfg, "config"); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// MustLoadDefaultConfig loads the canonical defaults file or panics.
func MustLoadDefaultConfig() *TuningConfig {
	candidates := []string{
		DefaultConfigPath,
		"../../" + DefaultConfigPath,
		"../../../" + DefaultConfigPath,
		"../../../../" + DefaultConfigPath,
		"../../../../../" + DefaultConfigPath,
	}
	for _, path := range candidates {
		if cfg, err := LoadTuningConfig(path); err == nil {
			return cfg
		}
	}
	panic("cannot find " + DefaultConfigPath + " - run tests from repository root")
}

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

// UnmarshalJSON enforces strict engine-block validation for L3.
func (c *L3Config) UnmarshalJSON(data []byte) error {
	raw, err := parseObject(data, "l3")
	if err != nil {
		return err
	}
	if err := ensureAllowedKeys(raw, "l3", []string{"engine", "ema_baseline_v1", "ema_track_assist_v2"}); err != nil {
		return err
	}
	engine, err := requiredEngine(raw, "l3")
	if err != nil {
		return err
	}
	spec, ok := engineRegistry[engine]
	if !ok || spec.Layer != "l3" {
		return fmt.Errorf("l3: unknown engine %q", engine)
	}

	c.Engine = engine
	switch engine {
	case "ema_baseline_v1":
		block, err := decodeSelectedEngineBlock[L3EmaBaselineV1](raw, "l3", engine)
		if err != nil {
			return err
		}
		c.EmaBaselineV1 = block
	case "ema_track_assist_v2":
		block, err := decodeSelectedEngineBlock[L3EmaTrackAssistV2](raw, "l3", engine)
		if err != nil {
			return err
		}
		c.EmaTrackAssistV2 = block
	default:
		return fmt.Errorf("l3: unknown engine %q", engine)
	}
	return nil
}

// UnmarshalJSON enforces strict engine-block validation for L4.
func (c *L4Config) UnmarshalJSON(data []byte) error {
	raw, err := parseObject(data, "l4")
	if err != nil {
		return err
	}
	if err := ensureAllowedKeys(raw, "l4", []string{"engine", "dbscan_xy_v1", "two_stage_mahalanobis_v2", "hdbscan_adaptive_v1"}); err != nil {
		return err
	}
	engine, err := requiredEngine(raw, "l4")
	if err != nil {
		return err
	}
	spec, ok := engineRegistry[engine]
	if !ok || spec.Layer != "l4" {
		return fmt.Errorf("l4: unknown engine %q", engine)
	}

	c.Engine = engine
	switch engine {
	case "dbscan_xy_v1":
		block, err := decodeSelectedEngineBlock[L4DbscanXyV1](raw, "l4", engine)
		if err != nil {
			return err
		}
		c.DbscanXyV1 = block
	case "two_stage_mahalanobis_v2":
		block, err := decodeSelectedEngineBlock[L4TwoStageMahalanobisV2](raw, "l4", engine)
		if err != nil {
			return err
		}
		c.TwoStageMahalanobisV2 = block
	case "hdbscan_adaptive_v1":
		block, err := decodeSelectedEngineBlock[L4HdbscanAdaptiveV1](raw, "l4", engine)
		if err != nil {
			return err
		}
		c.HdbscanAdaptiveV1 = block
	default:
		return fmt.Errorf("l4: unknown engine %q", engine)
	}
	return nil
}

// UnmarshalJSON enforces strict engine-block validation for L5.
func (c *L5Config) UnmarshalJSON(data []byte) error {
	raw, err := parseObject(data, "l5")
	if err != nil {
		return err
	}
	if err := ensureAllowedKeys(raw, "l5", []string{"engine", "cv_kf_v1", "imm_cv_ca_v2", "imm_cv_ca_rts_eval_v2"}); err != nil {
		return err
	}
	engine, err := requiredEngine(raw, "l5")
	if err != nil {
		return err
	}
	spec, ok := engineRegistry[engine]
	if !ok || spec.Layer != "l5" {
		return fmt.Errorf("l5: unknown engine %q", engine)
	}

	c.Engine = engine
	switch engine {
	case "cv_kf_v1":
		block, err := decodeSelectedEngineBlock[L5CvKfV1](raw, "l5", engine)
		if err != nil {
			return err
		}
		c.CvKfV1 = block
	case "imm_cv_ca_v2":
		block, err := decodeSelectedEngineBlock[L5ImmCvCaV2](raw, "l5", engine)
		if err != nil {
			return err
		}
		c.ImmCvCaV2 = block
	case "imm_cv_ca_rts_eval_v2":
		block, err := decodeSelectedEngineBlock[L5ImmCvCaRtsEvalV2](raw, "l5", engine)
		if err != nil {
			return err
		}
		c.ImmCvCaRtsEvalV2 = block
	default:
		return fmt.Errorf("l5: unknown engine %q", engine)
	}
	return nil
}

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

// GetUDPPort returns the configured UDP port.
func (c *TuningConfig) GetUDPPort() int { return c.L1.UDPPort }

// GetUDPRcvBuf returns the configured UDP receive buffer size.
func (c *TuningConfig) GetUDPRcvBuf() int { return c.L1.UDPRcvBuf }

// GetForwardPort returns the configured raw-packet forwarding port.
func (c *TuningConfig) GetForwardPort() int { return c.L1.ForwardPort }

// GetForegroundForwardPort returns the configured foreground forwarding port.
func (c *TuningConfig) GetForegroundForwardPort() int { return c.L1.ForegroundForwardPort }

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

// GetNeighborConfirmationCount returns the active L3 neighbour confirmation count.
func (c *TuningConfig) GetNeighborConfirmationCount() int {
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

// GetSafetyMarginMeters returns the active L3 safety margin in metres.
func (c *TuningConfig) GetSafetyMarginMeters() float64 { return c.L3.ActiveCommon().SafetyMarginMetres }

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

// GetMaxPositionJumpMeters returns the active L5 max position jump.
func (c *TuningConfig) GetMaxPositionJumpMeters() float64 {
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

func validateOptionalPort(name string, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("%s must be in [0, 65535], got %d", name, port)
	}
	return nil
}

func parseObject(data []byte, path string) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: expected object: %w", path, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("%s: expected object", path)
	}
	return raw, nil
}

func requiredEngine(raw map[string]json.RawMessage, path string) (string, error) {
	engineRaw, ok := raw["engine"]
	if !ok {
		return "", fmt.Errorf("%s: missing required key: engine", path)
	}
	var engine string
	if err := json.Unmarshal(engineRaw, &engine); err != nil {
		return "", fmt.Errorf("%s.engine: expected string: %w", path, err)
	}
	if strings.TrimSpace(engine) == "" {
		return "", fmt.Errorf("%s.engine: must be non-empty", path)
	}
	return engine, nil
}

func ensureAllowedKeys(raw map[string]json.RawMessage, path string, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	var unknown []string
	for key := range raw {
		if _, ok := allowedSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%s: unknown keys: %s", path, strings.Join(unknown, ", "))
}

func decodeSelectedEngineBlock[T any](raw map[string]json.RawMessage, path, engine string) (*T, error) {
	for key := range raw {
		if key == "engine" {
			continue
		}
		if key != engine {
			return nil, fmt.Errorf("%s: non-selected engine block %q present while engine=%q", path, key, engine)
		}
	}

	blockRaw, ok := raw[engine]
	if !ok {
		return nil, fmt.Errorf("%s: selected engine block %q missing", path, engine)
	}

	var block T
	if err := strictDecodeObject(blockRaw, &block, path+"."+engine); err != nil {
		return nil, err
	}
	return &block, nil
}

func strictDecodeObject(data []byte, dst interface{}, path string) error {
	raw, err := parseObject(data, path)
	if err != nil {
		return err
	}

	expected := expectedJSONKeys(reflect.TypeOf(dst))
	expectedSet := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		expectedSet[key] = struct{}{}
	}

	var unknown []string
	for key := range raw {
		if _, ok := expectedSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%s: unknown keys: %s", path, strings.Join(unknown, ", "))
	}

	var missing []string
	for _, key := range expected {
		if _, ok := raw[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s: missing required keys: %s", path, strings.Join(missing, ", "))
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func expectedJSONKeys(t reflect.Type) []string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var keys []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name, tagged := jsonFieldName(field)
		if name == "-" {
			continue
		}
		if field.Anonymous && !tagged {
			keys = append(keys, expectedJSONKeys(field.Type)...)
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name, false
	}
	if tag == "-" {
		return "-", true
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		return field.Name, true
	}
	return parts[0], true
}
