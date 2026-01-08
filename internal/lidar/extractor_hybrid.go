package lidar

import (
	"fmt"
	"time"
)

// HybridExtractorConfig holds configuration for the hybrid extractor.
type HybridExtractorConfig struct {
	// MergeMode determines how multiple extractors' results are combined
	MergeMode MergeMode

	// PrimaryExtractor name (used when MergeMode is "primary")
	PrimaryExtractor string

	// EnableMetricsComparison logs per-frame comparison metrics
	EnableMetricsComparison bool
}

// DefaultHybridExtractorConfig returns sensible defaults.
func DefaultHybridExtractorConfig() HybridExtractorConfig {
	return HybridExtractorConfig{
		MergeMode:               MergeModeUnion,
		PrimaryExtractor:        "background_subtraction",
		EnableMetricsComparison: true,
	}
}

// HybridExtractor runs multiple foreground extractors and merges their results.
// This enables:
// - Parallel algorithm evaluation
// - Redundant detection (union mode catches objects either algorithm finds)
// - Gradual migration from one algorithm to another
type HybridExtractor struct {
	Config     HybridExtractorConfig
	Extractors []ForegroundExtractor
	SensorID   string

	// Per-extractor results from last frame (for metrics comparison)
	LastResults []FrameResult
}

// Ensure HybridExtractor implements ForegroundExtractor
var _ ForegroundExtractor = (*HybridExtractor)(nil)

// NewHybridExtractor creates a new hybrid extractor with the given sub-extractors.
func NewHybridExtractor(
	config HybridExtractorConfig,
	extractors []ForegroundExtractor,
	sensorID string,
) *HybridExtractor {
	return &HybridExtractor{
		Config:      config,
		Extractors:  extractors,
		SensorID:    sensorID,
		LastResults: make([]FrameResult, len(extractors)),
	}
}

// Name returns the algorithm name.
func (e *HybridExtractor) Name() string {
	return fmt.Sprintf("hybrid_%s", e.Config.MergeMode)
}

// ProcessFrame runs all extractors and merges their results.
func (e *HybridExtractor) ProcessFrame(points []PointPolar, timestamp time.Time) (
	foregroundMask []bool,
	metrics ExtractorMetrics,
	err error,
) {
	if len(points) == 0 {
		return []bool{}, ExtractorMetrics{}, nil
	}

	if len(e.Extractors) == 0 {
		return nil, ExtractorMetrics{}, fmt.Errorf("no extractors configured")
	}

	start := time.Now()

	// Run all extractors and collect results
	masks := make([][]bool, len(e.Extractors))
	results := make([]FrameResult, len(e.Extractors))

	var firstError error
	for i, extractor := range e.Extractors {
		extStart := time.Now()
		mask, extMetrics, extErr := extractor.ProcessFrame(points, timestamp)
		extElapsed := time.Since(extStart)

		results[i] = FrameResult{
			AlgorithmName:  extractor.Name(),
			ForegroundMask: mask,
			Metrics:        extMetrics,
			ProcessingTime: extElapsed,
			Error:          extErr,
		}

		if extErr != nil {
			if firstError == nil {
				firstError = extErr
			}
			// Continue with other extractors even if one fails
			masks[i] = make([]bool, len(points)) // Empty mask on error
		} else {
			masks[i] = mask
		}
	}

	// Store results for metrics comparison
	e.LastResults = results

	// Merge masks according to configuration
	var mergedMask []bool
	switch e.Config.MergeMode {
	case MergeModePrimary:
		// Find primary extractor and use its mask
		for i, ext := range e.Extractors {
			if ext.Name() == e.Config.PrimaryExtractor {
				mergedMask = masks[i]
				break
			}
		}
		if mergedMask == nil && len(masks) > 0 {
			mergedMask = masks[0] // Fallback to first
		}
	default:
		mergedMask = MergeForegroundMasks(masks, e.Config.MergeMode)
	}

	elapsed := time.Since(start)

	// Compute merged metrics
	fgCount := CountForeground(mergedMask)
	bgCount := len(points) - fgCount

	// Build algorithm-specific metrics with per-extractor breakdown
	perExtractor := make(map[string]interface{})
	for _, result := range results {
		perExtractor[result.AlgorithmName] = map[string]interface{}{
			"foreground_count":   result.Metrics.ForegroundCount,
			"background_count":   result.Metrics.BackgroundCount,
			"processing_time_us": result.ProcessingTime.Microseconds(),
			"error":              result.Error != nil,
		}
	}

	// Compute agreement between extractors
	if len(masks) >= 2 {
		agreement := ComputeMaskAgreement(masks[0], masks[1])
		perExtractor["agreement_01"] = agreement
	}

	metrics = ExtractorMetrics{
		ForegroundCount:  fgCount,
		BackgroundCount:  bgCount,
		ProcessingTimeUs: elapsed.Microseconds(),
		AlgorithmSpecific: map[string]interface{}{
			"merge_mode":      string(e.Config.MergeMode),
			"extractor_count": len(e.Extractors),
			"per_extractor":   perExtractor,
		},
	}

	return mergedMask, metrics, firstError
}

// GetParams returns configuration for all sub-extractors.
func (e *HybridExtractor) GetParams() map[string]interface{} {
	params := map[string]interface{}{
		"merge_mode":        string(e.Config.MergeMode),
		"primary_extractor": e.Config.PrimaryExtractor,
	}

	// Add per-extractor params
	extractorParams := make(map[string]interface{})
	for _, ext := range e.Extractors {
		extractorParams[ext.Name()] = ext.GetParams()
	}
	params["extractors"] = extractorParams

	return params
}

// SetParams updates configuration.
func (e *HybridExtractor) SetParams(params map[string]interface{}) error {
	if v, ok := params["merge_mode"].(string); ok {
		e.Config.MergeMode = MergeMode(v)
	}
	if v, ok := params["primary_extractor"].(string); ok {
		e.Config.PrimaryExtractor = v
	}

	// Update per-extractor params
	if extractorParams, ok := params["extractors"].(map[string]interface{}); ok {
		for _, ext := range e.Extractors {
			if extParams, ok := extractorParams[ext.Name()].(map[string]interface{}); ok {
				if err := ext.SetParams(extParams); err != nil {
					return fmt.Errorf("failed to set params for %s: %w", ext.Name(), err)
				}
			}
		}
	}

	return nil
}

// Reset clears state for all sub-extractors.
func (e *HybridExtractor) Reset() {
	for _, ext := range e.Extractors {
		ext.Reset()
	}
	e.LastResults = make([]FrameResult, len(e.Extractors))
}

// GetLastResults returns per-extractor results from the last frame.
func (e *HybridExtractor) GetLastResults() []FrameResult {
	return e.LastResults
}

// GetExtractor returns a specific extractor by name.
func (e *HybridExtractor) GetExtractor(name string) ForegroundExtractor {
	for _, ext := range e.Extractors {
		if ext.Name() == name {
			return ext
		}
	}
	return nil
}

// AddExtractor adds a new extractor to the hybrid.
func (e *HybridExtractor) AddExtractor(ext ForegroundExtractor) {
	e.Extractors = append(e.Extractors, ext)
	e.LastResults = make([]FrameResult, len(e.Extractors))
}

// RemoveExtractor removes an extractor by name.
func (e *HybridExtractor) RemoveExtractor(name string) bool {
	for i, ext := range e.Extractors {
		if ext.Name() == name {
			e.Extractors = append(e.Extractors[:i], e.Extractors[i+1:]...)
			e.LastResults = make([]FrameResult, len(e.Extractors))
			return true
		}
	}
	return false
}
