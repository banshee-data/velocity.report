package sweep

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
)

func TestNewSampler(t *testing.T) {
	client := monitor.NewClient(nil, "http://localhost:8080", "sensor1")
	buckets := []string{"1", "2", "4"}
	interval := 100 * time.Millisecond

	s := NewSampler(client, buckets, interval)

	if s.Client != client {
		t.Error("Client mismatch")
	}
	if len(s.Buckets) != 3 {
		t.Errorf("Expected 3 buckets, got %d", len(s.Buckets))
	}
	if s.Interval != interval {
		t.Errorf("Expected interval %v, got %v", interval, s.Interval)
	}
}

func TestSampler_Sample_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if strings.Contains(r.URL.Path, "acceptance") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AcceptCounts":    []interface{}{100.0, 200.0, 300.0},
				"RejectCounts":    []interface{}{10.0, 20.0, 30.0},
				"Totals":          []interface{}{110.0, 220.0, 330.0},
				"AcceptanceRates": []interface{}{0.909, 0.909, 0.909},
			})
		} else if strings.Contains(r.URL.Path, "grid_status") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"background_count": 150.0,
			})
		}
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1", "2", "4"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	cfg := SampleConfig{
		Noise:      0.1,
		Closeness:  2.0,
		Neighbour:  3,
		Iterations: 2,
	}

	results := s.Sample(cfg)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
	if results[0].NonzeroCells != 150.0 {
		t.Errorf("Expected nonzero cells 150, got %f", results[0].NonzeroCells)
	}
	if callCount != 4 { // 2 acceptance + 2 grid_status
		t.Errorf("Expected 4 calls, got %d", callCount)
	}
}

func TestSampler_Sample_WithRawWriter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "acceptance") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AcceptCounts":    []interface{}{100.0},
				"RejectCounts":    []interface{}{10.0},
				"Totals":          []interface{}{110.0},
				"AcceptanceRates": []interface{}{0.909},
			})
		} else if strings.Contains(r.URL.Path, "grid_status") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"background_count": 150.0,
			})
		}
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	var buf bytes.Buffer
	rawW := csv.NewWriter(&buf)

	cfg := SampleConfig{
		Noise:      0.1,
		Closeness:  2.0,
		Neighbour:  3,
		Iterations: 1,
		RawWriter:  rawW,
	}

	s.Sample(cfg)
	rawW.Flush()

	output := buf.String()
	if !strings.Contains(output, "0.100000") {
		t.Errorf("Output should contain noise value: %s", output)
	}
}

func TestSampler_Sample_AcceptanceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	cfg := SampleConfig{
		Iterations: 2,
	}

	results := s.Sample(cfg)

	// Should have 0 results due to errors
	if len(results) != 0 {
		t.Errorf("Expected 0 results on error, got %d", len(results))
	}
}

func TestSampler_Sample_GridStatusError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "acceptance") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AcceptCounts":    []interface{}{100.0},
				"RejectCounts":    []interface{}{10.0},
				"Totals":          []interface{}{110.0},
				"AcceptanceRates": []interface{}{0.909},
			})
		} else if strings.Contains(r.URL.Path, "grid_status") {
			callCount++
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	cfg := SampleConfig{
		Iterations: 1,
	}

	results := s.Sample(cfg)

	// Should still get result, but nonzero cells should be 0
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].NonzeroCells != 0 {
		t.Errorf("Expected nonzero cells 0 on error, got %f", results[0].NonzeroCells)
	}
}

func TestWriteRawRow(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	result := SampleResult{
		AcceptCounts:     []int64{100, 200},
		RejectCounts:     []int64{10, 20},
		Totals:           []int64{110, 220},
		AcceptanceRates:  []float64{0.909, 0.909},
		NonzeroCells:     150,
		OverallAcceptPct: 0.909,
		Timestamp:        time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	buckets := []string{"1", "2"}

	WriteRawRow(w, 0.1, 2.0, 3, 0, result, buckets)
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "0.100000") {
		t.Errorf("Output should contain noise: %s", output)
	}
	if !strings.Contains(output, "2.000000") {
		t.Errorf("Output should contain closeness: %s", output)
	}
	if !strings.Contains(output, "100") {
		t.Errorf("Output should contain accept count: %s", output)
	}
	if !strings.Contains(output, "150") {
		t.Errorf("Output should contain nonzero cells: %s", output)
	}
}

func TestWriteRawHeaders(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	buckets := []string{"1", "4", "8"}

	WriteRawHeaders(w, buckets)
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "noise_relative") {
		t.Errorf("Output should contain noise_relative: %s", output)
	}
	if !strings.Contains(output, "neighbour_confirmation_count") {
		t.Errorf("Output should contain neighbour_confirmation_count: %s", output)
	}
	if !strings.Contains(output, "accept_counts_1") {
		t.Errorf("Output should contain accept_counts_1: %s", output)
	}
	if !strings.Contains(output, "acceptance_rates_8") {
		t.Errorf("Output should contain acceptance_rates_8: %s", output)
	}
}

func TestWriteSummaryHeaders(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	buckets := []string{"1", "4"}

	WriteSummaryHeaders(w, buckets)
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "noise_relative") {
		t.Errorf("Output should contain noise_relative: %s", output)
	}
	if !strings.Contains(output, "bucket_1_mean") {
		t.Errorf("Output should contain bucket_1_mean: %s", output)
	}
	if !strings.Contains(output, "bucket_4_stddev") {
		t.Errorf("Output should contain bucket_4_stddev: %s", output)
	}
	if !strings.Contains(output, "overall_accept_mean") {
		t.Errorf("Output should contain overall_accept_mean: %s", output)
	}
}

func TestWriteSummary(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	buckets := []string{"1", "4"}

	results := []SampleResult{
		{
			AcceptanceRates:  []float64{0.9, 0.8},
			NonzeroCells:     100,
			OverallAcceptPct: 0.85,
		},
		{
			AcceptanceRates:  []float64{0.95, 0.85},
			NonzeroCells:     110,
			OverallAcceptPct: 0.9,
		},
	}

	WriteSummary(w, 0.1, 2.0, 3, results, buckets)
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "0.100000") {
		t.Errorf("Output should contain noise: %s", output)
	}
	if !strings.Contains(output, "2.000000") {
		t.Errorf("Output should contain closeness: %s", output)
	}
}

func TestWriteSummary_EmptyResults(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	buckets := []string{"1"}

	WriteSummary(w, 0.1, 2.0, 3, []SampleResult{}, buckets)
	w.Flush()

	// Should have no output
	if buf.Len() != 0 {
		t.Errorf("Expected empty output for empty results, got: %s", buf.String())
	}
}

func TestWriteSummary_ShortAcceptanceRates(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	buckets := []string{"1", "4", "8"}

	// AcceptanceRates has fewer elements than buckets
	results := []SampleResult{
		{
			AcceptanceRates:  []float64{0.9},
			NonzeroCells:     100,
			OverallAcceptPct: 0.85,
		},
	}

	WriteSummary(w, 0.1, 2.0, 3, results, buckets)
	w.Flush()

	// Should not panic, should write row
	output := buf.String()
	if !strings.Contains(output, "0.100000") {
		t.Errorf("Output should contain noise: %s", output)
	}
}

func TestSampler_Sample_OverallAcceptCalculation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "acceptance") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AcceptCounts":    []interface{}{100.0, 200.0},
				"RejectCounts":    []interface{}{0.0, 0.0},
				"Totals":          []interface{}{100.0, 200.0},
				"AcceptanceRates": []interface{}{1.0, 1.0},
			})
		} else if strings.Contains(r.URL.Path, "grid_status") {
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1", "2"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	cfg := SampleConfig{
		Iterations: 1,
	}

	results := s.Sample(cfg)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	// Overall: (100+200) / (100+200) = 1.0
	if results[0].OverallAcceptPct != 1.0 {
		t.Errorf("Expected overall accept 1.0, got %f", results[0].OverallAcceptPct)
	}
}

func TestSampler_Sample_ZeroTotals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "acceptance") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AcceptCounts":    []interface{}{0.0},
				"RejectCounts":    []interface{}{0.0},
				"Totals":          []interface{}{0.0},
				"AcceptanceRates": []interface{}{0.0},
			})
		} else if strings.Contains(r.URL.Path, "grid_status") {
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	cfg := SampleConfig{
		Iterations: 1,
	}

	results := s.Sample(cfg)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	// Division by zero case
	if results[0].OverallAcceptPct != 0.0 {
		t.Errorf("Expected overall accept 0.0 for zero totals, got %f", results[0].OverallAcceptPct)
	}
}

func TestSampler_Sample_BackgroundCountTypes(t *testing.T) {
	testCases := []struct {
		name  string
		value interface{}
		want  float64
	}{
		{"float64", 150.0, 150.0},
		{"int", 100, 100.0},
		{"int64", int64(200), 200.0},
		{"string", "invalid", 0.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "acceptance") {
					json.NewEncoder(w).Encode(map[string]interface{}{
						"AcceptCounts":    []interface{}{100.0},
						"RejectCounts":    []interface{}{10.0},
						"Totals":          []interface{}{110.0},
						"AcceptanceRates": []interface{}{0.909},
					})
				} else if strings.Contains(r.URL.Path, "grid_status") {
					json.NewEncoder(w).Encode(map[string]interface{}{
						"background_count": tc.value,
					})
				}
			}))
			defer server.Close()

			client := monitor.NewClient(server.Client(), server.URL, "sensor1")
			buckets := []string{"1"}
			s := NewSampler(client, buckets, 10*time.Millisecond)

			cfg := SampleConfig{
				Iterations: 1,
			}

			results := s.Sample(cfg)

			if len(results) != 1 {
				t.Fatalf("Expected 1 result, got %d", len(results))
			}
			if results[0].NonzeroCells != tc.want {
				t.Errorf("Expected nonzero cells %f, got %f", tc.want, results[0].NonzeroCells)
			}
		})
	}
}

func TestSampler_Sample_ExcessiveIterationsClamp(t *testing.T) {
	// Test that excessive iterations are clamped to prevent DoS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "acceptance") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"AcceptCounts":    []interface{}{100.0},
				"RejectCounts":    []interface{}{10.0},
				"Totals":          []interface{}{110.0},
				"AcceptanceRates": []interface{}{0.909},
			})
		} else if strings.Contains(r.URL.Path, "grid_status") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"background_count": 100.0,
			})
		}
	}))
	defer server.Close()

	client := monitor.NewClient(server.Client(), server.URL, "sensor1")
	buckets := []string{"1"}
	s := NewSampler(client, buckets, 10*time.Millisecond)

	testCases := []struct {
		name              string
		iterations        int
		expectedMaxLength int
	}{
		{"excessive iterations", 10000, 500},
		{"negative iterations", -5, 30},
		{"zero iterations", 0, 30},
		{"valid iterations", 50, 50},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := SampleConfig{
				Iterations: tc.iterations,
			}

			results := s.Sample(cfg)

			if len(results) > tc.expectedMaxLength {
				t.Errorf("Expected at most %d results, got %d (DoS protection failed)", tc.expectedMaxLength, len(results))
			}
		})
	}
}
