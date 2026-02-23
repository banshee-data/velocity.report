// Command settling-eval evaluates background grid settling convergence for a
// running velocity.report server. It polls the /api/lidar/settling_eval
// endpoint at a configurable interval and produces a JSON report with
// convergence metrics and a recommended WarmupMinFrames value.
//
// Usage:
//
//	settling-eval --server http://localhost:8080 --sensor hesai-01 [--output report.json] [--interval 1s] [--timeout 120s]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
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

// EvalResponse is the JSON body returned by /api/lidar/settling_eval.
type EvalResponse struct {
	SensorID         string             `json:"sensor_id"`
	Metrics          SettlingMetrics    `json:"metrics"`
	Thresholds       SettlingThresholds `json:"thresholds"`
	Converged        bool               `json:"converged"`
	SettlingComplete bool               `json:"settling_complete"`
}

// SettlingEvaluation is the final JSON report written to --output.
type SettlingEvaluation struct {
	ServerURL           string             `json:"server_url"`
	SensorID            string             `json:"sensor_id"`
	TotalSamples        int                `json:"total_samples"`
	MetricsHistory      []SettlingMetrics  `json:"metrics_history"`
	RecommendedFrame    int                `json:"recommended_settling_frame"`
	RecommendedDuration string             `json:"recommended_settling_duration"`
	Thresholds          SettlingThresholds `json:"thresholds"`
	Rationale           string             `json:"rationale"`
}

func main() {
	server := flag.String("server", "http://localhost:8080", "velocity.report server URL")
	sensor := flag.String("sensor", "", "sensor ID (required)")
	output := flag.String("output", "", "output JSON path (default: stdout)")
	interval := flag.Duration("interval", 1*time.Second, "polling interval")
	timeout := flag.Duration("timeout", 120*time.Second, "maximum evaluation duration")
	flag.Parse()

	if *sensor == "" {
		fmt.Fprintln(os.Stderr, "error: --sensor is required")
		flag.Usage()
		os.Exit(1)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/api/lidar/settling_eval?sensor_id=%s", *server, *sensor)

	log.Printf("settling-eval: polling %s every %v (timeout %v)", url, *interval, *timeout)

	var history []SettlingMetrics
	var lastThresholds SettlingThresholds
	recommendedFrame := -1
	deadline := time.Now().Add(*timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("poll error: %v", err)
			time.Sleep(*interval)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("server returned %d: %s", resp.StatusCode, string(body))
			time.Sleep(*interval)
			continue
		}

		var evalResp EvalResponse
		if err := json.Unmarshal(body, &evalResp); err != nil {
			log.Printf("JSON decode error: %v", err)
			time.Sleep(*interval)
			continue
		}

		history = append(history, evalResp.Metrics)
		lastThresholds = evalResp.Thresholds

		log.Printf("frame=%d coverage=%.3f spread_delta=%.6f region_stability=%.3f confidence=%.1f converged=%v settling_complete=%v",
			evalResp.Metrics.FrameNumber,
			evalResp.Metrics.CoverageRate,
			evalResp.Metrics.SpreadDeltaRate,
			evalResp.Metrics.RegionStability,
			evalResp.Metrics.MeanConfidence,
			evalResp.Converged,
			evalResp.SettlingComplete,
		)

		if evalResp.Converged && recommendedFrame < 0 {
			recommendedFrame = evalResp.Metrics.FrameNumber
			log.Printf("✓ convergence detected at frame %d", recommendedFrame)
		}

		// If both converged AND settling is already complete, we have
		// enough data to produce a recommendation.
		if evalResp.Converged && evalResp.SettlingComplete {
			break
		}

		time.Sleep(*interval)
	}

	// Build recommendation
	rationale := buildRationale(history, recommendedFrame, lastThresholds)
	recDuration := "unknown"
	if recommendedFrame > 0 && len(history) > 0 {
		// Estimate duration assuming ~10 Hz frame rate
		recDuration = fmt.Sprintf("%.1fs (at 10 Hz)", float64(recommendedFrame)/10.0)
	}

	eval := SettlingEvaluation{
		ServerURL:           *server,
		SensorID:            *sensor,
		TotalSamples:        len(history),
		MetricsHistory:      history,
		RecommendedFrame:    recommendedFrame,
		RecommendedDuration: recDuration,
		Thresholds:          lastThresholds,
		Rationale:           rationale,
	}

	data, err := json.MarshalIndent(eval, "", "  ")
	if err != nil {
		log.Fatalf("JSON encode: %v", err)
	}

	if *output != "" {
		if err := os.WriteFile(*output, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", *output, err)
		}
		log.Printf("✓ report written to %s", *output)
	} else {
		fmt.Println(string(data))
	}
}

func buildRationale(history []SettlingMetrics, recFrame int, th SettlingThresholds) string {
	if len(history) == 0 {
		return "No samples collected; the server may not be running or the sensor may not be active."
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
