package sweep

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestNewCSVWriter(t *testing.T) {
	var summary, raw bytes.Buffer
	w := NewCSVWriter(&summary, &raw)

	if w.Summary == nil {
		t.Error("Summary writer should not be nil")
	}
	if w.Raw == nil {
		t.Error("Raw writer should not be nil")
	}
}

func TestCSVWriter_WriteHeaders(t *testing.T) {
	var summary, raw bytes.Buffer
	w := NewCSVWriter(&summary, &raw)

	buckets := []string{"15", "20", "25"}
	w.WriteHeaders(buckets)
	w.Flush()

	// Check summary header
	summaryLines := strings.Split(strings.TrimSpace(summary.String()), "\n")
	if len(summaryLines) != 1 {
		t.Errorf("Expected 1 summary line, got %d", len(summaryLines))
	}
	if !strings.Contains(summaryLines[0], "noise_relative") {
		t.Error("Summary header should contain noise_relative")
	}
	if !strings.Contains(summaryLines[0], "bucket_15_mean") {
		t.Error("Summary header should contain bucket_15_mean")
	}

	// Check raw header
	rawLines := strings.Split(strings.TrimSpace(raw.String()), "\n")
	if len(rawLines) != 1 {
		t.Errorf("Expected 1 raw line, got %d", len(rawLines))
	}
	if !strings.Contains(rawLines[0], "accept_counts_15") {
		t.Error("Raw header should contain accept_counts_15")
	}
}

func TestCSVWriter_WriteRawRow(t *testing.T) {
	var summary, raw bytes.Buffer
	w := NewCSVWriter(&summary, &raw)

	buckets := []string{"15", "20"}
	params := SweepParams{
		NoiseRelative:              0.1,
		ClosenessMultiplier:        2.0,
		NeighbourConfirmationCount: 3,
	}
	result := SampleResult{
		AcceptCounts:     []int64{100, 200},
		RejectCounts:     []int64{10, 20},
		Totals:           []int64{110, 220},
		AcceptanceRates:  []float64{0.909091, 0.909091},
		NonzeroCells:     50,
		OverallAcceptPct: 0.909091,
		Timestamp:        time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC),
	}

	w.WriteRawRow(params, 0, result, buckets)

	rawContent := raw.String()
	if !strings.Contains(rawContent, "0.100000") {
		t.Error("Raw row should contain noise value")
	}
	if !strings.Contains(rawContent, "2.000000") {
		t.Error("Raw row should contain closeness value")
	}
	if !strings.Contains(rawContent, "3,") {
		t.Error("Raw row should contain neighbour count")
	}
	if !strings.Contains(rawContent, "100,") {
		t.Error("Raw row should contain accept counts")
	}
}

func TestCSVWriter_WriteSummary(t *testing.T) {
	var summary, raw bytes.Buffer
	w := NewCSVWriter(&summary, &raw)

	buckets := []string{"15", "20"}
	params := SweepParams{
		NoiseRelative:              0.1,
		ClosenessMultiplier:        2.0,
		NeighbourConfirmationCount: 3,
	}
	results := []SampleResult{
		{
			AcceptanceRates:  []float64{0.9, 0.8},
			NonzeroCells:     50,
			OverallAcceptPct: 0.85,
		},
		{
			AcceptanceRates:  []float64{0.8, 0.7},
			NonzeroCells:     60,
			OverallAcceptPct: 0.75,
		},
	}

	w.WriteSummary(params, results, buckets)

	summaryContent := summary.String()
	if !strings.Contains(summaryContent, "0.100000") {
		t.Error("Summary should contain noise value")
	}
	// Mean of 0.9 and 0.8 is 0.85
	if !strings.Contains(summaryContent, "0.850000") {
		t.Error("Summary should contain mean acceptance rate")
	}
}

func TestCSVWriter_WriteSummary_Empty(t *testing.T) {
	var summary, raw bytes.Buffer
	w := NewCSVWriter(&summary, &raw)

	params := SweepParams{}
	buckets := []string{"15"}

	// Should not panic on empty results
	w.WriteSummary(params, []SampleResult{}, buckets)

	if summary.Len() != 0 {
		t.Error("Summary should be empty for empty results")
	}
}

func TestFormatSummaryHeaders(t *testing.T) {
	buckets := []string{"15", "20", "25"}
	headers := FormatSummaryHeaders(buckets)

	expectedStart := []string{"noise_relative", "closeness_multiplier", "neighbour_confirmation_count"}
	for i, expected := range expectedStart {
		if headers[i] != expected {
			t.Errorf("Header %d: expected %s, got %s", i, expected, headers[i])
		}
	}

	// Check bucket columns exist
	found := false
	for _, h := range headers {
		if h == "bucket_20_mean" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected bucket_20_mean in headers")
	}

	// Check end columns
	last := headers[len(headers)-1]
	if last != "overall_accept_stddev" {
		t.Errorf("Last header should be overall_accept_stddev, got %s", last)
	}
}

func TestFormatRawHeaders(t *testing.T) {
	buckets := []string{"15", "20"}
	headers := FormatRawHeaders(buckets)

	if headers[0] != "noise_relative" {
		t.Errorf("First header should be noise_relative, got %s", headers[0])
	}
	if headers[3] != "iter" {
		t.Errorf("Fourth header should be iter, got %s", headers[3])
	}

	// Check bucket columns
	found := false
	for _, h := range headers {
		if h == "accept_counts_15" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected accept_counts_15 in headers")
	}

	last := headers[len(headers)-1]
	if last != "overall_accept_percent" {
		t.Errorf("Last header should be overall_accept_percent, got %s", last)
	}
}

func TestCSVWriter_RoundTrip(t *testing.T) {
	var summary, raw bytes.Buffer
	w := NewCSVWriter(&summary, &raw)

	buckets := []string{"15"}
	w.WriteHeaders(buckets)

	params := SweepParams{
		NoiseRelative:              0.15,
		ClosenessMultiplier:        1.5,
		NeighbourConfirmationCount: 2,
	}

	result := SampleResult{
		AcceptCounts:     []int64{100},
		RejectCounts:     []int64{10},
		Totals:           []int64{110},
		AcceptanceRates:  []float64{0.909091},
		NonzeroCells:     42,
		OverallAcceptPct: 0.909091,
		Timestamp:        time.Now(),
	}

	w.WriteRawRow(params, 0, result, buckets)
	w.WriteSummary(params, []SampleResult{result}, buckets)
	w.Flush()

	// Parse summary CSV
	summaryReader := csv.NewReader(strings.NewReader(summary.String()))
	summaryRecords, err := summaryReader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read summary CSV: %v", err)
	}
	if len(summaryRecords) != 2 { // header + 1 data row
		t.Errorf("Expected 2 summary records, got %d", len(summaryRecords))
	}

	// Parse raw CSV
	rawReader := csv.NewReader(strings.NewReader(raw.String()))
	rawRecords, err := rawReader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read raw CSV: %v", err)
	}
	if len(rawRecords) != 2 { // header + 1 data row
		t.Errorf("Expected 2 raw records, got %d", len(rawRecords))
	}
}
