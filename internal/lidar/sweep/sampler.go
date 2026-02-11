// Package sweep provides utilities for parameter sweeping and sampling in LiDAR background detection tuning.
package sweep

import (
	"encoding/csv"
	"fmt"
	"log"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
)

// Sampler collects acceptance metrics over multiple iterations for parameter sweep analysis.
type Sampler struct {
	Client   *monitor.Client
	Buckets  []string
	Interval time.Duration
}

// NewSampler creates a new Sampler with the given client, buckets, and sampling interval.
func NewSampler(client *monitor.Client, buckets []string, interval time.Duration) *Sampler {
	return &Sampler{
		Client:   client,
		Buckets:  buckets,
		Interval: interval,
	}
}

// SampleConfig holds configuration for a single sampling run.
type SampleConfig struct {
	Noise      float64
	Closeness  float64
	Neighbour  int
	Iterations int
	RawWriter  *csv.Writer
}

// Sample collects acceptance metrics over the configured number of iterations.
// Returns a slice of SampleResult, one per iteration.
func (s *Sampler) Sample(cfg SampleConfig) []SampleResult {
	// Validate and clamp iterations to prevent excessive memory allocation (CWE-770).
	const maxIterations = 500
	const defaultIterations = 30

	iterations := defaultIterations
	if cfg.Iterations > 0 && cfg.Iterations <= maxIterations {
		iterations = cfg.Iterations
	} else if cfg.Iterations > maxIterations {
		iterations = maxIterations
		log.Printf("WARNING: Iterations %d exceeds maximum %d, clamping to maximum", cfg.Iterations, maxIterations)
	} else if cfg.Iterations <= 0 {
		log.Printf("WARNING: Invalid iterations %d, using default %d", cfg.Iterations, defaultIterations)
	}

	// Allocate with a compile-time constant cap to satisfy static analysis (CodeQL CWE-770).
	// maxIterations is small enough (500) that pre-allocating the full capacity is acceptable.
	results := make([]SampleResult, 0, maxIterations)

	for i := 0; i < iterations; i++ {
		metrics, err := s.Client.FetchAcceptanceMetrics()
		if err != nil {
			log.Printf("WARNING: Sample %d failed: %v", i+1, err)
			time.Sleep(s.Interval)
			continue
		}

		acceptCounts := ToInt64Slice(metrics["AcceptCounts"], len(s.Buckets))
		rejectCounts := ToInt64Slice(metrics["RejectCounts"], len(s.Buckets))
		totals := ToInt64Slice(metrics["Totals"], len(s.Buckets))
		rates := ToFloat64Slice(metrics["AcceptanceRates"], len(s.Buckets))

		// Calculate overall acceptance
		var sumAccept, sumTotal int64
		for j := range acceptCounts {
			sumAccept += acceptCounts[j]
			sumTotal += totals[j]
		}
		var overallPct float64
		if sumTotal > 0 {
			overallPct = float64(sumAccept) / float64(sumTotal)
		}

		// Fetch grid status for nonzero cells
		var nonzero float64
		if status, err := s.Client.FetchGridStatus(); err == nil {
			if bc, ok := status["background_count"]; ok {
				switch v := bc.(type) {
				case float64:
					nonzero = v
				case int:
					nonzero = float64(v)
				case int64:
					nonzero = float64(v)
				}
			}
		}

		result := SampleResult{
			AcceptCounts:     acceptCounts,
			RejectCounts:     rejectCounts,
			Totals:           totals,
			AcceptanceRates:  rates,
			NonzeroCells:     nonzero,
			OverallAcceptPct: overallPct,
			Timestamp:        time.Now(),
		}

		// Fetch tracking metrics (best-effort)
		if trackMetrics, err := s.Client.FetchTrackingMetrics(); err == nil {
			if v, ok := trackMetrics["active_tracks"]; ok {
				result.ActiveTracks = toIntFromMap(v)
			}
			if v, ok := trackMetrics["mean_alignment_deg"]; ok {
				result.MeanAlignmentDeg = toFloat64FromMap(v)
			}
			if v, ok := trackMetrics["misalignment_ratio"]; ok {
				result.MisalignmentRatio = toFloat64FromMap(v)
			}
			if v, ok := trackMetrics["heading_jitter_deg"]; ok {
				result.HeadingJitterDeg = toFloat64FromMap(v)
			}
			if v, ok := trackMetrics["fragmentation_ratio"]; ok {
				result.FragmentationRatio = toFloat64FromMap(v)
			}
			if v, ok := trackMetrics["tracks_created"]; ok {
				result.TracksCreated = toIntFromMap(v)
			}
			if v, ok := trackMetrics["tracks_confirmed"]; ok {
				result.TracksConfirmed = toIntFromMap(v)
			}
			if v, ok := trackMetrics["foreground_capture_ratio"]; ok {
				result.ForegroundCaptureRatio = toFloat64FromMap(v)
			}
			if v, ok := trackMetrics["unbounded_point_ratio"]; ok {
				result.UnboundedPointRatio = toFloat64FromMap(v)
			}
			if v, ok := trackMetrics["empty_box_ratio"]; ok {
				result.EmptyBoxRatio = toFloat64FromMap(v)
			}
		}

		results = append(results, result)

		// Write raw data if writer is provided
		if cfg.RawWriter != nil {
			WriteRawRow(cfg.RawWriter, cfg.Noise, cfg.Closeness, cfg.Neighbour, i, result, s.Buckets)
		}

		if i < iterations-1 {
			time.Sleep(s.Interval)
		}
	}

	return results
}

// WriteRawRow writes a single sample result row to the raw CSV writer.
func WriteRawRow(w *csv.Writer, noise, closeness float64, neighbour, iter int, result SampleResult, buckets []string) {
	row := []string{
		fmt.Sprintf("%.6f", noise),
		fmt.Sprintf("%.6f", closeness),
		fmt.Sprintf("%d", neighbour),
		fmt.Sprintf("%d", iter),
		result.Timestamp.Format(time.RFC3339Nano),
	}

	for _, v := range result.AcceptCounts {
		row = append(row, fmt.Sprintf("%d", v))
	}
	for _, v := range result.RejectCounts {
		row = append(row, fmt.Sprintf("%d", v))
	}
	for _, v := range result.Totals {
		row = append(row, fmt.Sprintf("%d", v))
	}
	for _, v := range result.AcceptanceRates {
		row = append(row, fmt.Sprintf("%.6f", v))
	}
	row = append(row, fmt.Sprintf("%.0f", result.NonzeroCells))
	row = append(row, fmt.Sprintf("%.6f", result.OverallAcceptPct))

	w.Write(row)
	w.Flush()
}

// WriteRawHeaders writes the header row for raw sample data.
func WriteRawHeaders(w *csv.Writer, buckets []string) {
	header := []string{"noise_relative", "closeness_multiplier", "neighbour_confirmation_count", "iter", "timestamp"}
	for _, b := range buckets {
		header = append(header, "accept_counts_"+b)
	}
	for _, b := range buckets {
		header = append(header, "reject_counts_"+b)
	}
	for _, b := range buckets {
		header = append(header, "totals_"+b)
	}
	for _, b := range buckets {
		header = append(header, "acceptance_rates_"+b)
	}
	header = append(header, "nonzero_cells", "overall_accept_percent")
	w.Write(header)
}

// WriteSummaryHeaders writes the header row for summary data.
func WriteSummaryHeaders(w *csv.Writer, buckets []string) {
	header := []string{"noise_relative", "closeness_multiplier", "neighbour_confirmation_count"}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_mean")
	}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_stddev")
	}
	header = append(header, "nonzero_cells_mean", "nonzero_cells_stddev", "overall_accept_mean", "overall_accept_stddev")
	w.Write(header)
}

// WriteSummary writes a summary row aggregating multiple sample results.
func WriteSummary(w *csv.Writer, noise, closeness float64, neighbour int, results []SampleResult, buckets []string) {
	if len(results) == 0 {
		log.Printf("WARNING: No results to summarise")
		return
	}

	// Compute per-bucket means and stddevs
	means := make([]float64, len(buckets))
	stds := make([]float64, len(buckets))

	for bi := range buckets {
		vals := make([]float64, len(results))
		for ri, r := range results {
			if bi < len(r.AcceptanceRates) {
				vals[ri] = r.AcceptanceRates[bi]
			}
		}
		means[bi], stds[bi] = MeanStddev(vals)
	}

	// Compute nonzero cells mean/stddev
	nonzeroVals := make([]float64, len(results))
	for ri, r := range results {
		nonzeroVals[ri] = r.NonzeroCells
	}
	nonzeroMean, nonzeroStd := MeanStddev(nonzeroVals)

	// Compute overall accept mean/stddev
	overallVals := make([]float64, len(results))
	for ri, r := range results {
		overallVals[ri] = r.OverallAcceptPct
	}
	overallMean, overallStd := MeanStddev(overallVals)

	// Build row
	row := []string{
		fmt.Sprintf("%.6f", noise),
		fmt.Sprintf("%.6f", closeness),
		fmt.Sprintf("%d", neighbour),
	}
	for _, m := range means {
		row = append(row, fmt.Sprintf("%.6f", m))
	}
	for _, s := range stds {
		row = append(row, fmt.Sprintf("%.6f", s))
	}
	row = append(row, fmt.Sprintf("%.6f", nonzeroMean))
	row = append(row, fmt.Sprintf("%.6f", nonzeroStd))
	row = append(row, fmt.Sprintf("%.6f", overallMean))
	row = append(row, fmt.Sprintf("%.6f", overallStd))

	w.Write(row)
	w.Flush()
}
