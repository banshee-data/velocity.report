package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
)

type legacyTuningConfig struct {
	BackgroundUpdateFraction         float64 `json:"background_update_fraction"`
	ClosenessMultiplier              float64 `json:"closeness_multiplier"`
	SafetyMarginMeters               float64 `json:"safety_margin_meters"`
	NoiseRelative                    float64 `json:"noise_relative"`
	NeighborConfirmationCount        int     `json:"neighbor_confirmation_count"`
	SeedFromFirst                    bool    `json:"seed_from_first"`
	WarmupDurationNanos              int64   `json:"warmup_duration_nanos"`
	WarmupMinFrames                  int     `json:"warmup_min_frames"`
	PostSettleUpdateFraction         float64 `json:"post_settle_update_fraction"`
	EnableDiagnostics                bool    `json:"enable_diagnostics"`
	ForegroundDBSCANEps              float64 `json:"foreground_dbscan_eps"`
	ForegroundMinClusterPoints       int     `json:"foreground_min_cluster_points"`
	ForegroundMaxInputPoints         int     `json:"foreground_max_input_points"`
	BufferTimeout                    string  `json:"buffer_timeout"`
	MinFramePoints                   int     `json:"min_frame_points"`
	FlushInterval                    string  `json:"flush_interval"`
	BackgroundFlush                  bool    `json:"background_flush"`
	GatingDistanceSquared            float64 `json:"gating_distance_squared"`
	ProcessNoisePos                  float64 `json:"process_noise_pos"`
	ProcessNoiseVel                  float64 `json:"process_noise_vel"`
	MeasurementNoise                 float64 `json:"measurement_noise"`
	OcclusionCovInflation            float64 `json:"occlusion_cov_inflation"`
	HitsToConfirm                    int     `json:"hits_to_confirm"`
	MaxMisses                        int     `json:"max_misses"`
	MaxMissesConfirmed               int     `json:"max_misses_confirmed"`
	MaxTracks                        int     `json:"max_tracks"`
	HeightBandFloor                  float64 `json:"height_band_floor"`
	HeightBandCeiling                float64 `json:"height_band_ceiling"`
	RemoveGround                     bool    `json:"remove_ground"`
	MaxClusterDiameter               float64 `json:"max_cluster_diameter"`
	MinClusterDiameter               float64 `json:"min_cluster_diameter"`
	MaxClusterAspectRatio            float64 `json:"max_cluster_aspect_ratio"`
	MaxReasonableSpeedMps            float64 `json:"max_reasonable_speed_mps"`
	MaxPositionJumpMeters            float64 `json:"max_position_jump_meters"`
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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config-migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inPath := fs.String("in", "", "Legacy flat JSON config path")
	outPath := fs.String("out", "", "Output path for migrated v2 config (defaults to stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *inPath == "" {
		fmt.Fprintln(stderr, "error: --in is required")
		return 2
	}

	data, err := os.ReadFile(*inPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read %s: %v\n", *inPath, err)
		return 1
	}

	legacyData, err := filterLegacyMetadata(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: parse %s: %v\n", *inPath, err)
		return 1
	}

	var legacy legacyTuningConfig
	if err := json.Unmarshal(legacyData, &legacy); err != nil {
		fmt.Fprintf(stderr, "error: decode legacy config: %v\n", err)
		return 1
	}

	migrated := migrateLegacyConfig(legacy)
	if err := migrated.Validate(); err != nil {
		fmt.Fprintf(stderr, "error: migrated config failed validation: %v\n", err)
		return 1
	}
	output, err := json.MarshalIndent(migrated, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: encode migrated config: %v\n", err)
		return 1
	}
	output = append(output, '\n')

	if *outPath == "" {
		if _, err := stdout.Write(output); err != nil {
			fmt.Fprintf(stderr, "error: write stdout: %v\n", err)
			return 1
		}
		return 0
	}
	if err := os.WriteFile(*outPath, output, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: write %s: %v\n", *outPath, err)
		return 1
	}
	return 0
}

func filterLegacyMetadata(data []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	filtered := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		if len(key) > 0 && key[0] == '_' {
			continue
		}
		filtered[key] = value
	}
	return json.Marshal(filtered)
}

func migrateLegacyConfig(legacy legacyTuningConfig) *cfgpkg.TuningConfig {
	return &cfgpkg.TuningConfig{
		Version: cfgpkg.CurrentConfigVersion,
		L1: cfgpkg.L1Config{
			Sensor:                "hesai-pandar40p",
			DataSource:            "live",
			UDPPort:               2369,
			UDPRcvBuf:             4 << 20,
			ForwardPort:           2368,
			ForegroundForwardPort: 2370,
		},
		L3: cfgpkg.L3Config{
			Engine: "ema_baseline_v1",
			EmaBaselineV1: &cfgpkg.L3EmaBaselineV1{
				L3Common: cfgpkg.L3Common{
					BackgroundUpdateFraction:          legacy.BackgroundUpdateFraction,
					ClosenessMultiplier:               legacy.ClosenessMultiplier,
					SafetyMarginMetres:                legacy.SafetyMarginMeters,
					NoiseRelative:                     legacy.NoiseRelative,
					NeighbourConfirmationCount:        legacy.NeighborConfirmationCount,
					SeedFromFirst:                     legacy.SeedFromFirst,
					WarmupDurationNanos:               legacy.WarmupDurationNanos,
					WarmupMinFrames:                   legacy.WarmupMinFrames,
					PostSettleUpdateFraction:          legacy.PostSettleUpdateFraction,
					EnableDiagnostics:                 legacy.EnableDiagnostics,
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
		L4: cfgpkg.L4Config{
			Engine: "dbscan_xy_v1",
			DbscanXyV1: &cfgpkg.L4DbscanXyV1{
				L4Common: cfgpkg.L4Common{
					ForegroundDBSCANEps:        legacy.ForegroundDBSCANEps,
					ForegroundMinClusterPoints: legacy.ForegroundMinClusterPoints,
					ForegroundMaxInputPoints:   legacy.ForegroundMaxInputPoints,
					HeightBandFloor:            legacy.HeightBandFloor,
					HeightBandCeiling:          legacy.HeightBandCeiling,
					RemoveGround:               legacy.RemoveGround,
					MaxClusterDiameter:         legacy.MaxClusterDiameter,
					MinClusterDiameter:         legacy.MinClusterDiameter,
					MaxClusterAspectRatio:      legacy.MaxClusterAspectRatio,
				},
			},
		},
		L5: cfgpkg.L5Config{
			Engine: "cv_kf_v1",
			CvKfV1: &cfgpkg.L5CvKfV1{
				L5Common: cfgpkg.L5Common{
					GatingDistanceSquared:            legacy.GatingDistanceSquared,
					ProcessNoisePos:                  legacy.ProcessNoisePos,
					ProcessNoiseVel:                  legacy.ProcessNoiseVel,
					MeasurementNoise:                 legacy.MeasurementNoise,
					OcclusionCovInflation:            legacy.OcclusionCovInflation,
					HitsToConfirm:                    legacy.HitsToConfirm,
					MaxMisses:                        legacy.MaxMisses,
					MaxMissesConfirmed:               legacy.MaxMissesConfirmed,
					MaxTracks:                        legacy.MaxTracks,
					MaxReasonableSpeedMps:            legacy.MaxReasonableSpeedMps,
					MaxPositionJumpMetres:            legacy.MaxPositionJumpMeters,
					MaxPredictDt:                     legacy.MaxPredictDt,
					MaxCovarianceDiag:                legacy.MaxCovarianceDiag,
					MinPointsForPCA:                  legacy.MinPointsForPCA,
					OBBHeadingSmoothingAlpha:         legacy.OBBHeadingSmoothingAlpha,
					OBBAspectRatioLockThreshold:      legacy.OBBAspectRatioLockThreshold,
					MaxTrackHistoryLength:            legacy.MaxTrackHistoryLength,
					MaxSpeedHistoryLength:            legacy.MaxSpeedHistoryLength,
					MergeSizeRatio:                   legacy.MergeSizeRatio,
					SplitSizeRatio:                   legacy.SplitSizeRatio,
					DeletedTrackGracePeriod:          legacy.DeletedTrackGracePeriod,
					MinObservationsForClassification: legacy.MinObservationsForClassification,
				},
			},
		},
		Pipeline: cfgpkg.PipelineConfig{
			BufferTimeout:   legacy.BufferTimeout,
			MinFramePoints:  legacy.MinFramePoints,
			FlushInterval:   legacy.FlushInterval,
			BackgroundFlush: legacy.BackgroundFlush,
		},
	}
}
