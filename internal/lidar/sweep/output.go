package sweep

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"time"
)

// SampleResult holds the metrics collected from a single sample iteration.
type SampleResult struct {
	AcceptCounts     []int64
	RejectCounts     []int64
	Totals           []int64
	AcceptanceRates  []float64
	NonzeroCells     float64
	OverallAcceptPct float64
	Timestamp        time.Time

	// Track health metrics (best-effort; zero if tracker unavailable)
	ActiveTracks      int
	MeanAlignmentDeg  float64
	MisalignmentRatio float64
}

// SweepParams holds the sweep parameters for a single test run.
type SweepParams struct {
	NoiseRelative              float64
	ClosenessMultiplier        float64
	NeighbourConfirmationCount int
}

// CSVWriter wraps csv.Writer with methods for sweep output.
type CSVWriter struct {
	Summary *csv.Writer
	Raw     *csv.Writer
}

// NewCSVWriter creates a new CSVWriter with the given summary and raw writers.
func NewCSVWriter(summary, raw io.Writer) *CSVWriter {
	return &CSVWriter{
		Summary: csv.NewWriter(summary),
		Raw:     csv.NewWriter(raw),
	}
}

// WriteHeaders writes the headers to both summary and raw CSV files.
func (c *CSVWriter) WriteHeaders(buckets []string) {
	c.writeSummaryHeader(buckets)
	c.writeRawHeader(buckets)
}

// writeSummaryHeader writes the summary CSV header.
func (c *CSVWriter) writeSummaryHeader(buckets []string) {
	header := []string{"noise_relative", "closeness_multiplier", "neighbour_confirmation_count"}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_mean")
	}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_stddev")
	}
	header = append(header, "nonzero_cells_mean", "nonzero_cells_stddev", "overall_accept_mean", "overall_accept_stddev")
	c.Summary.Write(header)
}

// writeRawHeader writes the raw data CSV header.
func (c *CSVWriter) writeRawHeader(buckets []string) {
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
	c.Raw.Write(header)
}

// WriteRawRow writes a single raw data row to the raw CSV file.
func (c *CSVWriter) WriteRawRow(params SweepParams, iter int, result SampleResult, buckets []string) {
	row := []string{
		fmt.Sprintf("%.6f", params.NoiseRelative),
		fmt.Sprintf("%.6f", params.ClosenessMultiplier),
		fmt.Sprintf("%d", params.NeighbourConfirmationCount),
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

	c.Raw.Write(row)
	c.Raw.Flush()
}

// WriteSummary computes and writes summary statistics for a set of sample results.
func (c *CSVWriter) WriteSummary(params SweepParams, results []SampleResult, buckets []string) {
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

	// Compute nonzero cells stats
	nonzeroVals := make([]float64, len(results))
	for i, r := range results {
		nonzeroVals[i] = r.NonzeroCells
	}
	nonzeroMean, nonzeroStd := MeanStddev(nonzeroVals)

	// Compute overall acceptance stats
	overallVals := make([]float64, len(results))
	for i, r := range results {
		overallVals[i] = r.OverallAcceptPct
	}
	overallMean, overallStd := MeanStddev(overallVals)

	log.Printf("Results: overall_accept=%.4f±%.4f, nonzero_cells=%.0f±%.0f",
		overallMean, overallStd, nonzeroMean, nonzeroStd)

	// Write CSV row
	row := []string{
		fmt.Sprintf("%.6f", params.NoiseRelative),
		fmt.Sprintf("%.6f", params.ClosenessMultiplier),
		fmt.Sprintf("%d", params.NeighbourConfirmationCount),
	}
	for _, m := range means {
		row = append(row, fmt.Sprintf("%.6f", m))
	}
	for _, s := range stds {
		row = append(row, fmt.Sprintf("%.6f", s))
	}
	row = append(row, fmt.Sprintf("%.0f", nonzeroMean))
	row = append(row, fmt.Sprintf("%.0f", nonzeroStd))
	row = append(row, fmt.Sprintf("%.6f", overallMean))
	row = append(row, fmt.Sprintf("%.6f", overallStd))

	c.Summary.Write(row)
	c.Summary.Flush()
}

// Flush flushes both summary and raw writers.
func (c *CSVWriter) Flush() {
	c.Summary.Flush()
	c.Raw.Flush()
}

// FormatSummaryHeaders returns the summary header column names for given buckets.
func FormatSummaryHeaders(buckets []string) []string {
	header := []string{"noise_relative", "closeness_multiplier", "neighbour_confirmation_count"}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_mean")
	}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_stddev")
	}
	header = append(header, "nonzero_cells_mean", "nonzero_cells_stddev", "overall_accept_mean", "overall_accept_stddev")
	return header
}

// FormatRawHeaders returns the raw data header column names for given buckets.
func FormatRawHeaders(buckets []string) []string {
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
	return header
}
