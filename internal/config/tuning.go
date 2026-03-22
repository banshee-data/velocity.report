package config

import (
	"fmt"
	"os"
	"path/filepath"
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

// L1Config holds sensor identity and data-source settings.
type L1Config struct {
	Sensor     string `json:"sensor"`
	DataSource string `json:"data_source"`
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
