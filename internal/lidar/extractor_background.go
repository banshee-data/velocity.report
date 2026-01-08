package lidar

import (
	"encoding/json"
	"fmt"
	"time"
)

// BackgroundSubtractorExtractor wraps BackgroundManager to implement ForegroundExtractor.
// This enables the existing background subtraction algorithm to be used in the
// evaluation harness alongside other algorithms.
type BackgroundSubtractorExtractor struct {
	Manager  *BackgroundManager
	SensorID string
}

// Ensure BackgroundSubtractorExtractor implements ForegroundExtractor
var _ ForegroundExtractor = (*BackgroundSubtractorExtractor)(nil)

// NewBackgroundSubtractorExtractor creates a new extractor wrapping an existing BackgroundManager.
func NewBackgroundSubtractorExtractor(manager *BackgroundManager, sensorID string) *BackgroundSubtractorExtractor {
	return &BackgroundSubtractorExtractor{
		Manager:  manager,
		SensorID: sensorID,
	}
}

// Name returns the algorithm name.
func (e *BackgroundSubtractorExtractor) Name() string {
	return "background_subtraction"
}

// ProcessFrame extracts foreground points using background subtraction.
func (e *BackgroundSubtractorExtractor) ProcessFrame(points []PointPolar, timestamp time.Time) (
	foregroundMask []bool,
	metrics ExtractorMetrics,
	err error,
) {
	if e.Manager == nil {
		return nil, ExtractorMetrics{}, fmt.Errorf("background manager is nil")
	}

	start := time.Now()

	// Use the existing ProcessFramePolarWithMask
	mask, err := e.Manager.ProcessFramePolarWithMask(points)
	if err != nil {
		return nil, ExtractorMetrics{}, err
	}

	elapsed := time.Since(start)

	// Count foreground/background
	fgCount := 0
	for _, v := range mask {
		if v {
			fgCount++
		}
	}
	bgCount := len(mask) - fgCount

	metrics = ExtractorMetrics{
		ForegroundCount:  fgCount,
		BackgroundCount:  bgCount,
		ProcessingTimeUs: elapsed.Microseconds(),
		AlgorithmSpecific: map[string]interface{}{
			"settling_complete": e.Manager.Grid.SettlingComplete,
			"nonzero_cells":     e.Manager.Grid.nonzeroCellCount,
		},
	}

	return mask, metrics, nil
}

// GetParams returns the current background parameters as a map.
func (e *BackgroundSubtractorExtractor) GetParams() map[string]interface{} {
	if e.Manager == nil || e.Manager.Grid == nil {
		return nil
	}

	params := e.Manager.GetParams()

	// Convert to map for JSON serialization
	result := map[string]interface{}{
		"background_update_fraction":        params.BackgroundUpdateFraction,
		"closeness_sensitivity_multiplier":  params.ClosenessSensitivityMultiplier,
		"safety_margin_meters":              params.SafetyMarginMeters,
		"freeze_duration_nanos":             params.FreezeDurationNanos,
		"neighbor_confirmation_count":       params.NeighborConfirmationCount,
		"warmup_duration_nanos":             params.WarmupDurationNanos,
		"warmup_min_frames":                 params.WarmupMinFrames,
		"post_settle_update_fraction":       params.PostSettleUpdateFraction,
		"foreground_min_cluster_points":     params.ForegroundMinClusterPoints,
		"foreground_dbscan_eps":             params.ForegroundDBSCANEps,
		"noise_relative_fraction":           params.NoiseRelativeFraction,
		"seed_from_first_observation":       params.SeedFromFirstObservation,
		"reacquisition_boost_multiplier":    params.ReacquisitionBoostMultiplier,
		"min_confidence_floor":              params.MinConfidenceFloor,
		"locked_baseline_threshold":         params.LockedBaselineThreshold,
		"locked_baseline_multiplier":        params.LockedBaselineMultiplier,
	}

	return result
}

// SetParams updates the background parameters from a map.
func (e *BackgroundSubtractorExtractor) SetParams(params map[string]interface{}) error {
	if e.Manager == nil {
		return fmt.Errorf("background manager is nil")
	}

	// Get current params
	current := e.Manager.GetParams()

	// Parse JSON-encoded params if needed
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	// Create a temporary struct and unmarshal
	var updates struct {
		BackgroundUpdateFraction       *float32 `json:"background_update_fraction"`
		ClosenessSensitivityMultiplier *float32 `json:"closeness_sensitivity_multiplier"`
		SafetyMarginMeters             *float32 `json:"safety_margin_meters"`
		FreezeDurationNanos            *int64   `json:"freeze_duration_nanos"`
		NeighborConfirmationCount      *int     `json:"neighbor_confirmation_count"`
		WarmupDurationNanos            *int64   `json:"warmup_duration_nanos"`
		WarmupMinFrames                *int     `json:"warmup_min_frames"`
		PostSettleUpdateFraction       *float32 `json:"post_settle_update_fraction"`
		ForegroundMinClusterPoints     *int     `json:"foreground_min_cluster_points"`
		ForegroundDBSCANEps            *float32 `json:"foreground_dbscan_eps"`
		NoiseRelativeFraction          *float32 `json:"noise_relative_fraction"`
		SeedFromFirstObservation       *bool    `json:"seed_from_first_observation"`
		ReacquisitionBoostMultiplier   *float32 `json:"reacquisition_boost_multiplier"`
		MinConfidenceFloor             *uint32  `json:"min_confidence_floor"`
		LockedBaselineThreshold        *uint32  `json:"locked_baseline_threshold"`
		LockedBaselineMultiplier       *float32 `json:"locked_baseline_multiplier"`
	}

	if err := json.Unmarshal(paramsJSON, &updates); err != nil {
		return fmt.Errorf("failed to unmarshal params: %w", err)
	}

	// Apply updates
	if updates.BackgroundUpdateFraction != nil {
		current.BackgroundUpdateFraction = *updates.BackgroundUpdateFraction
	}
	if updates.ClosenessSensitivityMultiplier != nil {
		current.ClosenessSensitivityMultiplier = *updates.ClosenessSensitivityMultiplier
	}
	if updates.SafetyMarginMeters != nil {
		current.SafetyMarginMeters = *updates.SafetyMarginMeters
	}
	if updates.FreezeDurationNanos != nil {
		current.FreezeDurationNanos = *updates.FreezeDurationNanos
	}
	if updates.NeighborConfirmationCount != nil {
		current.NeighborConfirmationCount = *updates.NeighborConfirmationCount
	}
	if updates.WarmupDurationNanos != nil {
		current.WarmupDurationNanos = *updates.WarmupDurationNanos
	}
	if updates.WarmupMinFrames != nil {
		current.WarmupMinFrames = *updates.WarmupMinFrames
	}
	if updates.PostSettleUpdateFraction != nil {
		current.PostSettleUpdateFraction = *updates.PostSettleUpdateFraction
	}
	if updates.ForegroundMinClusterPoints != nil {
		current.ForegroundMinClusterPoints = *updates.ForegroundMinClusterPoints
	}
	if updates.ForegroundDBSCANEps != nil {
		current.ForegroundDBSCANEps = *updates.ForegroundDBSCANEps
	}
	if updates.NoiseRelativeFraction != nil {
		current.NoiseRelativeFraction = *updates.NoiseRelativeFraction
	}
	if updates.SeedFromFirstObservation != nil {
		current.SeedFromFirstObservation = *updates.SeedFromFirstObservation
	}
	if updates.ReacquisitionBoostMultiplier != nil {
		current.ReacquisitionBoostMultiplier = *updates.ReacquisitionBoostMultiplier
	}
	if updates.MinConfidenceFloor != nil {
		current.MinConfidenceFloor = *updates.MinConfidenceFloor
	}
	if updates.LockedBaselineThreshold != nil {
		current.LockedBaselineThreshold = *updates.LockedBaselineThreshold
	}
	if updates.LockedBaselineMultiplier != nil {
		current.LockedBaselineMultiplier = *updates.LockedBaselineMultiplier
	}

	return e.Manager.SetParams(current)
}

// Reset clears the background grid state for a fresh start.
func (e *BackgroundSubtractorExtractor) Reset() {
	if e.Manager == nil {
		return
	}
	_ = e.Manager.ResetGrid()
}
