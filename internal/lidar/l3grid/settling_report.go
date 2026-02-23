package l3grid

import (
	"encoding/json"
	"fmt"
	"os"
)

// SettlingReport is the structured output of a settling convergence evaluation.
// It contains the full metrics history, recommended warmup frame, and a
// human-readable rationale.
type SettlingReport struct {
	PCAPFile            string             `json:"pcap_file"`
	TuningFile          string             `json:"tuning_file"`
	SensorID            string             `json:"sensor_id"`
	TotalSamples        int                `json:"total_samples"`
	TotalFrames         int                `json:"total_frames"`
	MetricsHistory      []SettlingMetrics  `json:"metrics_history"`
	RecommendedFrame    int                `json:"recommended_settling_frame"`
	RecommendedDuration string             `json:"recommended_settling_duration"`
	Thresholds          SettlingThresholds `json:"thresholds"`
	Rationale           string             `json:"rationale"`
	WallDuration        string             `json:"wall_duration"`
}

// BuildRationale produces a human-readable explanation of the settling
// evaluation result suitable for inclusion in the JSON report.
func BuildRationale(history []SettlingMetrics, recFrame int, th SettlingThresholds) string {
	if len(history) == 0 {
		return "No frames processed; the PCAP file may be empty or contain no valid LiDAR packets."
	}
	if recFrame < 0 {
		last := history[len(history)-1]
		return fmt.Sprintf(
			"Convergence was not reached within the evaluation window. "+
				"Final metrics: coverage=%.2f (need ≥%.2f), spread_delta=%.6f (need ≤%.6f), "+
				"region_stability=%.3f (need ≥%.3f), confidence=%.1f (need ≥%.1f). "+
				"Consider increasing --timeout or adjusting WarmupMinFrames upwards.",
			last.CoverageRate, th.MinCoverage,
			last.SpreadDeltaRate, th.MaxSpreadDelta,
			last.RegionStability, th.MinRegionStability,
			last.MeanConfidence, th.MinConfidence,
		)
	}
	return fmt.Sprintf(
		"Convergence reached at frame %d. Recommended WarmupMinFrames=%d "+
			"(with 20%% safety margin: %d). At 10 Hz this is ~%.0fs; at 20 Hz ~%.0fs.",
		recFrame, recFrame,
		int(float64(recFrame)*1.2),
		float64(recFrame)/10.0,
		float64(recFrame)/20.0,
	)
}

// FormatRecommendedDuration returns a human-readable duration string for the
// given convergence frame. Returns "unknown" if the frame is non-positive.
func FormatRecommendedDuration(recFrame int) string {
	if recFrame <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.1fs (at 10 Hz)", float64(recFrame)/10.0)
}

// WriteReport marshals the report to indented JSON and writes it to the given
// path. If path is empty, the JSON is written to stdout.
func WriteReport(report *SettlingReport, path string) error {
	return writeJSON(report, path)
}

// writeJSON marshals v to indented JSON and writes to path (or stdout if empty).
func writeJSON(v any, path string) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON encode: %w", err)
	}

	if path != "" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		return nil
	}

	_, err = fmt.Println(string(data))
	return err
}
