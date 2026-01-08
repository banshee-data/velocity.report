package lidar

import (
	"time"
)

// ExtractorMetrics contains per-frame metrics from an extractor.
type ExtractorMetrics struct {
	ForegroundCount   int                    `json:"foreground_count"`
	BackgroundCount   int                    `json:"background_count"`
	ProcessingTimeUs  int64                  `json:"processing_time_us"`
	AlgorithmSpecific map[string]interface{} `json:"algorithm_specific,omitempty"`
}

// ForegroundExtractor is the interface for all foreground extraction algorithms.
// This enables pluggable algorithms for A/B comparison and hybrid approaches.
type ForegroundExtractor interface {
	// Name returns the algorithm name for logging/metrics
	Name() string

	// ProcessFrame extracts foreground points from a frame.
	// Returns foreground mask (true = foreground) and processing metrics.
	ProcessFrame(points []PointPolar, timestamp time.Time) (
		foregroundMask []bool,
		metrics ExtractorMetrics,
		err error,
	)

	// GetParams returns current algorithm parameters (for serialization)
	GetParams() map[string]interface{}

	// SetParams updates algorithm parameters at runtime
	SetParams(params map[string]interface{}) error

	// Reset clears internal state (for PCAP replay restart)
	Reset()
}

// MergeMode defines how multiple extractors' results are combined.
type MergeMode string

const (
	// MergeModeUnion combines foreground masks with OR (point is fg if ANY algorithm says so)
	MergeModeUnion MergeMode = "union"
	// MergeModeIntersection combines foreground masks with AND (point is fg if ALL algorithms say so)
	MergeModeIntersection MergeMode = "intersection"
	// MergeModeWeighted uses confidence-weighted voting (requires confidence per point)
	MergeModeWeighted MergeMode = "weighted"
	// MergeModePrimary uses the primary extractor's output, secondary for fallback
	MergeModePrimary MergeMode = "primary"
)

// FrameResult contains the output of a single extractor for a frame.
type FrameResult struct {
	AlgorithmName  string           `json:"algorithm_name"`
	ForegroundMask []bool           `json:"-"` // Not serialized (too large)
	Metrics        ExtractorMetrics `json:"metrics"`
	ProcessingTime time.Duration    `json:"processing_time_ns"`
	Error          error            `json:"error,omitempty"`
	Precision      float64          `json:"precision,omitempty"` // If ground truth available
	Recall         float64          `json:"recall,omitempty"`    // If ground truth available
}

// MergeForegroundMasks combines multiple foreground masks using the specified mode.
func MergeForegroundMasks(masks [][]bool, mode MergeMode) []bool {
	if len(masks) == 0 {
		return nil
	}

	// Find the longest mask (they should all be same length, but be safe)
	maxLen := 0
	for _, m := range masks {
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}

	if maxLen == 0 {
		return nil
	}

	result := make([]bool, maxLen)

	switch mode {
	case MergeModeUnion:
		// Point is foreground if ANY algorithm says so
		for _, mask := range masks {
			for i, v := range mask {
				if v {
					result[i] = true
				}
			}
		}

	case MergeModeIntersection:
		// Point is foreground if ALL algorithms say so
		// Start with true, AND with each mask
		for i := range result {
			result[i] = true
		}
		for _, mask := range masks {
			for i := 0; i < len(mask) && i < maxLen; i++ {
				result[i] = result[i] && mask[i]
			}
			// If mask is shorter, treat missing entries as false
			for i := len(mask); i < maxLen; i++ {
				result[i] = false
			}
		}

	case MergeModePrimary:
		// Use first extractor's mask (primary)
		if len(masks) > 0 && len(masks[0]) > 0 {
			copy(result, masks[0])
		}

	case MergeModeWeighted:
		// For weighted mode without confidence, fall back to majority voting
		fallthrough
	default:
		// Majority voting: point is foreground if >50% of algorithms say so
		threshold := len(masks) / 2
		for i := 0; i < maxLen; i++ {
			votes := 0
			for _, mask := range masks {
				if i < len(mask) && mask[i] {
					votes++
				}
			}
			result[i] = votes > threshold
		}
	}

	return result
}

// CountForeground returns the number of true values in a mask.
func CountForeground(mask []bool) int {
	count := 0
	for _, v := range mask {
		if v {
			count++
		}
	}
	return count
}

// ComputeMaskAgreement calculates the percentage of points where two masks agree.
func ComputeMaskAgreement(mask1, mask2 []bool) float64 {
	if len(mask1) == 0 || len(mask2) == 0 {
		return 0
	}

	minLen := len(mask1)
	if len(mask2) < minLen {
		minLen = len(mask2)
	}

	agreements := 0
	for i := 0; i < minLen; i++ {
		if mask1[i] == mask2[i] {
			agreements++
		}
	}

	return float64(agreements) / float64(minLen)
}

// ComputePrecisionRecall calculates precision and recall given predicted and ground truth masks.
// Precision = TP / (TP + FP), Recall = TP / (TP + FN)
func ComputePrecisionRecall(predicted, groundTruth []bool) (precision, recall float64) {
	if len(predicted) == 0 || len(groundTruth) == 0 {
		return 0, 0
	}

	minLen := len(predicted)
	if len(groundTruth) < minLen {
		minLen = len(groundTruth)
	}

	var tp, fp, fn int
	for i := 0; i < minLen; i++ {
		if predicted[i] && groundTruth[i] {
			tp++ // True positive
		} else if predicted[i] && !groundTruth[i] {
			fp++ // False positive
		} else if !predicted[i] && groundTruth[i] {
			fn++ // False negative
		}
	}

	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}

	return precision, recall
}
