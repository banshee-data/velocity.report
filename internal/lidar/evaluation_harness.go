package lidar

import (
	"encoding/json"
	"sync"
	"time"
)

// EvaluationHarnessConfig holds configuration for the evaluation harness.
type EvaluationHarnessConfig struct {
	// LogComparisons enables detailed per-frame comparison logging
	LogComparisons bool

	// ComparisonLogCallback is called for each frame with comparison results
	ComparisonLogCallback func(timestamp time.Time, results []FrameResult)

	// GroundTruthProvider supplies ground truth masks for precision/recall
	GroundTruthProvider GroundTruthProvider
}

// GroundTruthProvider supplies ground truth foreground masks for evaluation.
type GroundTruthProvider interface {
	// GetMask returns the ground truth foreground mask for a given timestamp.
	// Returns nil if no ground truth is available for this timestamp.
	GetMask(timestamp time.Time) []bool

	// HasGroundTruth returns true if ground truth is available.
	HasGroundTruth() bool
}

// EvaluationHarness runs multiple foreground extractors and compares their results.
// This is used for algorithm evaluation and A/B testing.
type EvaluationHarness struct {
	Config     EvaluationHarnessConfig
	Extractors []ForegroundExtractor

	// Accumulated statistics
	mu                sync.RWMutex
	FrameCount        int64
	TotalProcessingUs int64
	PerExtractorStats map[string]*ExtractorStats

	// Comparison results buffer
	ComparisonBuffer []*FrameComparison
	MaxBufferSize    int
}

// ExtractorStats holds accumulated statistics for a single extractor.
type ExtractorStats struct {
	Name               string
	FrameCount         int64
	TotalForeground    int64
	TotalBackground    int64
	TotalProcessingUs  int64
	SumPrecision       float64
	SumRecall          float64
	FramesWithGroundTruth int64
}

// FrameComparison holds comparison results for a single frame.
type FrameComparison struct {
	Timestamp    int64                    `json:"timestamp_nanos"`
	Results      []FrameResultSummary     `json:"results"`
	AgreementPct float64                  `json:"agreement_pct"`
}

// FrameResultSummary is a JSON-safe summary of FrameResult.
type FrameResultSummary struct {
	AlgorithmName    string  `json:"algorithm"`
	ForegroundCount  int     `json:"fg_count"`
	BackgroundCount  int     `json:"bg_count"`
	ProcessingTimeUs int64   `json:"time_us"`
	Precision        float64 `json:"precision,omitempty"`
	Recall           float64 `json:"recall,omitempty"`
	HasError         bool    `json:"has_error"`
}

// NewEvaluationHarness creates a new evaluation harness.
func NewEvaluationHarness(config EvaluationHarnessConfig, extractors []ForegroundExtractor) *EvaluationHarness {
	stats := make(map[string]*ExtractorStats)
	for _, ext := range extractors {
		stats[ext.Name()] = &ExtractorStats{Name: ext.Name()}
	}

	return &EvaluationHarness{
		Config:            config,
		Extractors:        extractors,
		PerExtractorStats: stats,
		MaxBufferSize:     1000, // Keep last 1000 comparisons
	}
}

// ProcessFrame runs all extractors and compares results.
func (h *EvaluationHarness) ProcessFrame(points []PointPolar, timestamp time.Time) []FrameResult {
	if len(points) == 0 {
		return nil
	}

	start := time.Now()
	results := make([]FrameResult, len(h.Extractors))

	// Run all extractors
	for i, extractor := range h.Extractors {
		extStart := time.Now()
		mask, metrics, err := extractor.ProcessFrame(points, timestamp)
		extElapsed := time.Since(extStart)

		results[i] = FrameResult{
			AlgorithmName:  extractor.Name(),
			ForegroundMask: mask,
			Metrics:        metrics,
			ProcessingTime: extElapsed,
			Error:          err,
		}

		// Compute precision/recall if ground truth available
		if h.Config.GroundTruthProvider != nil && h.Config.GroundTruthProvider.HasGroundTruth() {
			gtMask := h.Config.GroundTruthProvider.GetMask(timestamp)
			if gtMask != nil {
				results[i].Precision, results[i].Recall = ComputePrecisionRecall(mask, gtMask)
			}
		}
	}

	totalElapsed := time.Since(start)

	// Update statistics
	h.mu.Lock()
	h.FrameCount++
	h.TotalProcessingUs += totalElapsed.Microseconds()

	for _, result := range results {
		stats := h.PerExtractorStats[result.AlgorithmName]
		if stats == nil {
			stats = &ExtractorStats{Name: result.AlgorithmName}
			h.PerExtractorStats[result.AlgorithmName] = stats
		}

		stats.FrameCount++
		stats.TotalForeground += int64(result.Metrics.ForegroundCount)
		stats.TotalBackground += int64(result.Metrics.BackgroundCount)
		stats.TotalProcessingUs += result.ProcessingTime.Microseconds()

		if result.Precision > 0 || result.Recall > 0 {
			stats.SumPrecision += result.Precision
			stats.SumRecall += result.Recall
			stats.FramesWithGroundTruth++
		}
	}

	// Store comparison
	if h.Config.LogComparisons && len(results) >= 2 {
		comparison := &FrameComparison{
			Timestamp: timestamp.UnixNano(),
			Results:   make([]FrameResultSummary, len(results)),
		}

		for i, r := range results {
			comparison.Results[i] = FrameResultSummary{
				AlgorithmName:    r.AlgorithmName,
				ForegroundCount:  r.Metrics.ForegroundCount,
				BackgroundCount:  r.Metrics.BackgroundCount,
				ProcessingTimeUs: r.ProcessingTime.Microseconds(),
				Precision:        r.Precision,
				Recall:           r.Recall,
				HasError:         r.Error != nil,
			}
		}

		// Compute agreement between first two extractors
		if len(results) >= 2 && len(results[0].ForegroundMask) > 0 && len(results[1].ForegroundMask) > 0 {
			comparison.AgreementPct = ComputeMaskAgreement(results[0].ForegroundMask, results[1].ForegroundMask) * 100
		}

		// Add to buffer
		h.ComparisonBuffer = append(h.ComparisonBuffer, comparison)
		if len(h.ComparisonBuffer) > h.MaxBufferSize {
			h.ComparisonBuffer = h.ComparisonBuffer[1:]
		}
	}
	h.mu.Unlock()

	// Callback if configured
	if h.Config.ComparisonLogCallback != nil {
		h.Config.ComparisonLogCallback(timestamp, results)
	}

	return results
}

// GetStats returns accumulated statistics.
func (h *EvaluationHarness) GetStats() map[string]*ExtractorStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Deep copy to avoid race conditions
	result := make(map[string]*ExtractorStats)
	for k, v := range h.PerExtractorStats {
		statsCopy := *v
		result[k] = &statsCopy
	}
	return result
}

// GetSummary returns a JSON-serializable summary.
func (h *EvaluationHarness) GetSummary() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	perExtractor := make(map[string]interface{})
	for name, stats := range h.PerExtractorStats {
		extSummary := map[string]interface{}{
			"frame_count":        stats.FrameCount,
			"total_foreground":   stats.TotalForeground,
			"total_background":   stats.TotalBackground,
			"avg_processing_us":  float64(stats.TotalProcessingUs) / float64(max(stats.FrameCount, 1)),
		}

		if stats.FramesWithGroundTruth > 0 {
			extSummary["avg_precision"] = stats.SumPrecision / float64(stats.FramesWithGroundTruth)
			extSummary["avg_recall"] = stats.SumRecall / float64(stats.FramesWithGroundTruth)
		}

		// Compute foreground ratio
		totalPoints := stats.TotalForeground + stats.TotalBackground
		if totalPoints > 0 {
			extSummary["foreground_ratio"] = float64(stats.TotalForeground) / float64(totalPoints)
		}

		perExtractor[name] = extSummary
	}

	return map[string]interface{}{
		"frame_count":          h.FrameCount,
		"total_processing_us":  h.TotalProcessingUs,
		"avg_processing_us":    float64(h.TotalProcessingUs) / float64(max(h.FrameCount, 1)),
		"extractor_count":      len(h.Extractors),
		"per_extractor":        perExtractor,
	}
}

// GetRecentComparisons returns recent frame comparisons.
func (h *EvaluationHarness) GetRecentComparisons(limit int) []*FrameComparison {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.ComparisonBuffer) {
		limit = len(h.ComparisonBuffer)
	}

	// Return most recent
	start := len(h.ComparisonBuffer) - limit
	result := make([]*FrameComparison, limit)
	copy(result, h.ComparisonBuffer[start:])
	return result
}

// ExportComparisonsJSON exports comparisons as JSON.
func (h *EvaluationHarness) ExportComparisonsJSON() ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return json.Marshal(h.ComparisonBuffer)
}

// Reset clears all statistics and extractors' state.
func (h *EvaluationHarness) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.FrameCount = 0
	h.TotalProcessingUs = 0
	h.ComparisonBuffer = nil

	for name := range h.PerExtractorStats {
		h.PerExtractorStats[name] = &ExtractorStats{Name: name}
	}

	for _, ext := range h.Extractors {
		ext.Reset()
	}
}

// AddExtractor adds a new extractor to the harness.
func (h *EvaluationHarness) AddExtractor(ext ForegroundExtractor) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.Extractors = append(h.Extractors, ext)
	h.PerExtractorStats[ext.Name()] = &ExtractorStats{Name: ext.Name()}
}

// RemoveExtractor removes an extractor by name.
func (h *EvaluationHarness) RemoveExtractor(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i, ext := range h.Extractors {
		if ext.Name() == name {
			h.Extractors = append(h.Extractors[:i], h.Extractors[i+1:]...)
			delete(h.PerExtractorStats, name)
			return true
		}
	}
	return false
}

// Helper function for min of two ints
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
