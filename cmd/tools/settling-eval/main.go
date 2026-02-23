// Command settling-eval evaluates background grid settling convergence by
// replaying a captured PCAP file offline through a local BackgroundManager
// at full speed. It evaluates convergence on every frame and produces a JSON
// report with convergence metrics and a recommended WarmupMinFrames value.
//
// Usage:
//
//	go run -tags=pcap ./cmd/tools/settling-eval capture.pcap [--tuning config/tuning.defaults.json] [--output report.json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

// SettlingMetrics mirrors l3grid.SettlingMetrics for JSON deserialisation.
type SettlingMetrics struct {
	CoverageRate    float64   `json:"coverage_rate"`
	SpreadDeltaRate float64   `json:"spread_delta_rate"`
	RegionStability float64   `json:"region_stability"`
	MeanConfidence  float64   `json:"mean_confidence"`
	EvaluatedAt     time.Time `json:"evaluated_at"`
	FrameNumber     int       `json:"frame_number"`
}

// SettlingThresholds mirrors l3grid.SettlingThresholds.
type SettlingThresholds struct {
	MinCoverage        float64 `json:"min_coverage"`
	MaxSpreadDelta     float64 `json:"max_spread_delta"`
	MinRegionStability float64 `json:"min_region_stability"`
	MinConfidence      float64 `json:"min_confidence"`
}

// SettlingEvaluation is the final JSON report written to --output.
type SettlingEvaluation struct {
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

func main() {
	output := flag.String("output", "", "output JSON path (default: stdout)")
	sensor := flag.String("sensor", "pcap-eval", "sensor ID")
	tuningFile := flag.String("tuning", "", "tuning config JSON path (default: config/tuning.defaults.json)")
	udpPort := flag.Int("port", 2368, "UDP port filter for PCAP packets")

	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: settling-eval [flags] <pcap-file>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	pcapFile := flag.Arg(0)

	eval, err := runPCAPEval(pcapFile, *tuningFile, *sensor, *udpPort)
	if err != nil {
		log.Fatalf("pcap eval: %v", err)
	}
	writeReport(eval, *output)
}

// writeReport marshals eval to JSON and writes to output (or stdout).
func writeReport(eval *SettlingEvaluation, output string) {
	data, err := json.MarshalIndent(eval, "", "  ")
	if err != nil {
		log.Fatalf("JSON encode: %v", err)
	}

	if output != "" {
		if err := os.WriteFile(output, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", output, err)
		}
		log.Printf("✓ report written to %s", output)
	} else {
		fmt.Println(string(data))
	}
}

func buildRationale(history []SettlingMetrics, recFrame int, th SettlingThresholds) string {
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
