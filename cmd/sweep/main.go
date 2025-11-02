package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// parseCSVFloatSlice parses a comma-separated list of floats
func parseCSVFloatSlice(s string) ([]float64, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float '%s': %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseCSVIntSlice parses a comma-separated list of ints
func parseCSVIntSlice(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid int '%s': %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// toFloat64Slice converts a JSON-decoded value into a fixed-length []float64.
func toFloat64Slice(v interface{}, length int) []float64 {
	out := make([]float64, length)
	if v == nil {
		return out
	}
	switch vv := v.(type) {
	case []interface{}:
		for i := 0; i < len(out) && i < len(vv); i++ {
			switch val := vv[i].(type) {
			case float64:
				out[i] = val
			case int:
				out[i] = float64(val)
			case int64:
				out[i] = float64(val)
			default:
				out[i] = 0
			}
		}
	}
	return out
}

// toInt64Slice converts a JSON-decoded value into a fixed-length []int64.
func toInt64Slice(v interface{}, length int) []int64 {
	out := make([]int64, length)
	if v == nil {
		return out
	}
	switch vv := v.(type) {
	case []interface{}:
		for i := 0; i < len(out) && i < len(vv); i++ {
			switch val := vv[i].(type) {
			case float64:
				out[i] = int64(val)
			case int:
				out[i] = int64(val)
			case int64:
				out[i] = val
			default:
				out[i] = 0
			}
		}
	}
	return out
}

func meanStddev(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	mean := sum / float64(len(xs))
	var sdSum float64
	for _, v := range xs {
		d := v - mean
		sdSum += d * d
	}
	var stddev float64
	if len(xs) > 1 {
		stddev = math.Sqrt(sdSum / float64(len(xs)-1))
	} else {
		stddev = 0
	}
	return mean, stddev
}

func main() {
	// Common flags
	monitorURL := flag.String("monitor", "http://localhost:8081", "Base URL for lidar monitor")
	sensorID := flag.String("sensor", "hesai-pandar40p", "Sensor ID")
	output := flag.String("output", "", "Output CSV filename (defaults to sweep-<timestamp>.csv)")

	// PCAP support
	pcapFile := flag.String("pcap", "", "PCAP file to replay (enables PCAP mode)")
	pcapSettle := flag.Duration("pcap-settle", 20*time.Second, "Time to wait after PCAP replay before sampling")

	// Sweep mode selection
	sweepMode := flag.String("mode", "multi", "Sweep mode: 'multi' (all combinations), 'noise' (vary noise only), 'closeness' (vary closeness only), 'neighbor' (vary neighbor only)")

	// Parameter ranges for multi-sweep
	noiseList := flag.String("noise", "", "Comma-separated noise values (e.g. 0.005,0.01,0.02) or range start:end:step")
	closenessList := flag.String("closeness", "", "Comma-separated closeness values (e.g. 1.5,2.0,2.5) or range start:end:step")
	neighborList := flag.String("neighbors", "", "Comma-separated neighbor values (e.g. 0,1,2)")

	// Single-variable sweep ranges (when mode != multi)
	noiseStart := flag.Float64("noise-start", 0.005, "Start noise value for noise sweep")
	noiseEnd := flag.Float64("noise-end", 0.03, "End noise value for noise sweep")
	noiseStep := flag.Float64("noise-step", 0.005, "Step for noise sweep")

	closenessStart := flag.Float64("closeness-start", 1.5, "Start closeness value for closeness sweep")
	closenessEnd := flag.Float64("closeness-end", 3.0, "End closeness value for closeness sweep")
	closenessStep := flag.Float64("closeness-step", 0.5, "Step for closeness sweep")

	neighborStart := flag.Int("neighbor-start", 0, "Start neighbor value for neighbor sweep")
	neighborEnd := flag.Int("neighbor-end", 3, "End neighbor value for neighbor sweep")
	neighborStep := flag.Int("neighbor-step", 1, "Step for neighbor sweep")

	// Fixed values for single-variable sweeps
	fixedNoise := flag.Float64("fixed-noise", 0.01, "Fixed noise value (when not sweeping noise)")
	fixedCloseness := flag.Float64("fixed-closeness", 2.0, "Fixed closeness value (when not sweeping closeness)")
	fixedNeighbor := flag.Int("fixed-neighbor", 1, "Fixed neighbor value (when not sweeping neighbor)")

	// Sampling configuration
	iterations := flag.Int("iterations", 30, "Number of samples per parameter combination")
	interval := flag.Duration("interval", 2*time.Second, "Interval between samples")
	settleTime := flag.Duration("settle-time", 5*time.Second, "Time to wait for grid to settle after applying params")

	// Seed control
	seedFlag := flag.String("seed", "true", "Seed behavior: 'true', 'false', or 'toggle' (alternates per combo)")

	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}

	// Start PCAP replay if requested
	if *pcapFile != "" {
		if err := startPCAPReplay(client, *monitorURL, *sensorID, *pcapFile); err != nil {
			log.Fatalf("Failed to start PCAP replay: %v", err)
		}
		log.Printf("PCAP mode enabled: %s (settle time: %v)", *pcapFile, *pcapSettle)
	}

	// Fetch acceptance buckets for header construction
	buckets := fetchBuckets(client, *monitorURL, *sensorID)
	log.Printf("Using %d acceptance buckets", len(buckets))

	// Determine parameter combinations based on sweep mode
	var noiseCombos, closenessCombos []float64
	var neighborCombos []int

	switch *sweepMode {
	case "multi":
		// Multi-parameter sweep: use lists or parse ranges
		noiseCombos = parseParamList(*noiseList, *noiseStart, *noiseEnd, *noiseStep)
		closenessCombos = parseParamList(*closenessList, *closenessStart, *closenessEnd, *closenessStep)
		neighborCombos = parseIntParamList(*neighborList, *neighborStart, *neighborEnd, *neighborStep)

	case "noise":
		// Sweep noise only, fix others
		noiseCombos = generateRange(*noiseStart, *noiseEnd, *noiseStep)
		closenessCombos = []float64{*fixedCloseness}
		neighborCombos = []int{*fixedNeighbor}

	case "closeness":
		// Sweep closeness only, fix others
		noiseCombos = []float64{*fixedNoise}
		closenessCombos = generateRange(*closenessStart, *closenessEnd, *closenessStep)
		neighborCombos = []int{*fixedNeighbor}

	case "neighbor":
		// Sweep neighbor only, fix others
		noiseCombos = []float64{*fixedNoise}
		closenessCombos = []float64{*fixedCloseness}
		neighborCombos = generateIntRange(*neighborStart, *neighborEnd, *neighborStep)

	default:
		log.Fatalf("Invalid sweep mode: %s (must be multi, noise, closeness, or neighbor)", *sweepMode)
	}

	// Provide defaults if lists are empty
	if len(noiseCombos) == 0 {
		noiseCombos = []float64{0.005, 0.01, 0.02}
	}
	if len(closenessCombos) == 0 {
		closenessCombos = []float64{1.5, 2.0, 2.5}
	}
	if len(neighborCombos) == 0 {
		neighborCombos = []int{0, 1, 2}
	}

	totalCombos := len(noiseCombos) * len(closenessCombos) * len(neighborCombos)
	log.Printf("Sweep mode: %s", *sweepMode)
	log.Printf("Parameter combinations: %d (noise: %d, closeness: %d, neighbor: %d)",
		totalCombos, len(noiseCombos), len(closenessCombos), len(neighborCombos))

	// Prepare output files
	filename := *output
	if filename == "" {
		filename = fmt.Sprintf("sweep-%s-%s.csv", *sweepMode, time.Now().Format("20060102-150405"))
	}

	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Could not create output file %s: %v", filename, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	rawFilename := strings.TrimSuffix(filename, ".csv") + "-raw.csv"
	fRaw, err := os.Create(rawFilename)
	if err != nil {
		log.Fatalf("Could not create raw output file %s: %v", rawFilename, err)
	}
	defer fRaw.Close()
	rawW := csv.NewWriter(fRaw)
	defer rawW.Flush()

	// Write headers
	writeHeaders(w, rawW, buckets)

	// Run sweep
	comboNum := 0
	seedToggle := false

	for _, noise := range noiseCombos {
		for _, closeness := range closenessCombos {
			for _, neighbor := range neighborCombos {
				comboNum++
				log.Printf("\n=== Combination %d/%d: noise=%.4f, closeness=%.2f, neighbor=%d ===",
					comboNum, totalCombos, noise, closeness, neighbor)

				// Determine seed value for this combo
				var seed bool
				switch *seedFlag {
				case "true":
					seed = true
				case "false":
					seed = false
				case "toggle":
					seed = seedToggle
					seedToggle = !seedToggle
				default:
					seed = true
				}

				// Reset grid
				if err := resetGrid(client, *monitorURL, *sensorID); err != nil {
					log.Printf("WARNING: Grid reset failed: %v", err)
				}

				// Set parameters
				if err := setParams(client, *monitorURL, *sensorID, noise, closeness, neighbor, seed); err != nil {
					log.Printf("ERROR: Failed to set params: %v", err)
					continue
				}

				// Reset acceptance counters
				if err := resetAcceptance(client, *monitorURL, *sensorID); err != nil {
					log.Printf("WARNING: Failed to reset acceptance: %v", err)
				}

				// PCAP mode: trigger replay and wait for settle
				if *pcapFile != "" {
					if err := startPCAPReplay(client, *monitorURL, *sensorID, *pcapFile); err != nil {
						log.Printf("WARNING: PCAP replay failed: %v (will retry)", err)
						time.Sleep(5 * time.Second)
						if err := startPCAPReplay(client, *monitorURL, *sensorID, *pcapFile); err != nil {
							log.Printf("ERROR: PCAP replay failed again: %v", err)
							continue
						}
					}
					time.Sleep(*pcapSettle)
				} else {
					// Live mode: wait for grid to settle
					waitForGridSettle(client, *monitorURL, *sensorID, *settleTime)
				}

				// Sample metrics
				results := sampleMetrics(client, *monitorURL, *sensorID, *iterations, *interval, buckets, noise, closeness, neighbor, rawW)

				// Compute statistics and write summary
				writeSummary(w, noise, closeness, neighbor, results, buckets)
			}
		}
	}

	log.Printf("\nSweep complete!")
	log.Printf("Summary: %s", filename)
	log.Printf("Raw data: %s", rawFilename)
}

// parseParamList parses a comma-separated list or generates a range
func parseParamList(list string, start, end, step float64) []float64 {
	if list != "" {
		vals, err := parseCSVFloatSlice(list)
		if err != nil {
			log.Fatalf("Invalid parameter list: %v", err)
		}
		return vals
	}
	return generateRange(start, end, step)
}

func parseIntParamList(list string, start, end, step int) []int {
	if list != "" {
		vals, err := parseCSVIntSlice(list)
		if err != nil {
			log.Fatalf("Invalid parameter list: %v", err)
		}
		return vals
	}
	return generateIntRange(start, end, step)
}

func generateRange(start, end, step float64) []float64 {
	if step <= 0 {
		step = 0.01
	}
	var result []float64
	for v := start; v <= end+1e-9; v += step {
		result = append(result, v)
	}
	return result
}

func generateIntRange(start, end, step int) []int {
	if step <= 0 {
		step = 1
	}
	var result []int
	for v := start; v <= end; v += step {
		result = append(result, v)
	}
	return result
}

func startPCAPReplay(client *http.Client, baseURL, sensorID, pcapFile string) error {
	url := fmt.Sprintf("%s/api/lidar/pcap/start?sensor_id=%s", baseURL, sensorID)
	payload := map[string]string{"pcap_file": pcapFile}
	data, _ := json.Marshal(payload)

	// Retry logic for 503 (PCAP already in progress)
	maxRetries := 60
	for retry := 0; retry < maxRetries; retry++ {
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusServiceUnavailable {
			if retry == 0 {
				log.Printf("PCAP replay in progress, waiting...")
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("timeout waiting for PCAP replay slot")
}

func fetchBuckets(client *http.Client, baseURL, sensorID string) []string {
	resp, err := client.Get(fmt.Sprintf("%s/api/lidar/acceptance?sensor_id=%s", baseURL, sensorID))
	if err != nil {
		log.Printf("WARNING: Could not fetch buckets: %v (using defaults)", err)
		return []string{"1", "2", "4", "8", "10", "12", "16", "20", "50", "100", "200"}
	}
	defer resp.Body.Close()

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return []string{"1", "2", "4", "8", "10", "12", "16", "20", "50", "100", "200"}
	}

	bm, ok := m["BucketsMeters"].([]interface{})
	if !ok || len(bm) == 0 {
		return []string{"1", "2", "4", "8", "10", "12", "16", "20", "50", "100", "200"}
	}

	buckets := make([]string, 0, len(bm))
	for _, bi := range bm {
		switch v := bi.(type) {
		case float64:
			if v == math.Trunc(v) {
				buckets = append(buckets, fmt.Sprintf("%.0f", v))
			} else {
				buckets = append(buckets, fmt.Sprintf("%.6f", v))
			}
		default:
			buckets = append(buckets, fmt.Sprintf("%v", v))
		}
	}
	return buckets
}

func resetGrid(client *http.Client, baseURL, sensorID string) error {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/lidar/grid_reset?sensor_id=%s", baseURL, sensorID), nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func setParams(client *http.Client, baseURL, sensorID string, noise, closeness float64, neighbor int, seed bool) error {
	params := map[string]interface{}{
		"noise_relative":              noise,
		"closeness_multiplier":        closeness,
		"neighbor_confirmation_count": neighbor,
		"seed_from_first_frame":       seed,
	}
	data, _ := json.Marshal(params)

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/lidar/params?sensor_id=%s", baseURL, sensorID), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Applied: noise=%.4f, closeness=%.2f, neighbor=%d, seed=%v", noise, closeness, neighbor, seed)
	return nil
}

func resetAcceptance(client *http.Client, baseURL, sensorID string) error {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/lidar/acceptance/reset?sensor_id=%s", baseURL, sensorID), nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func waitForGridSettle(client *http.Client, baseURL, sensorID string, timeout time.Duration) {
	if timeout <= 0 {
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("%s/api/lidar/grid_status?sensor_id=%s", baseURL, sensorID))
		if err == nil {
			var gs map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&gs) == nil {
				if bc, ok := gs["background_count"]; ok {
					if n, ok := bc.(float64); ok && n > 0 {
						resp.Body.Close()
						return
					}
				}
			}
			resp.Body.Close()
		}
		time.Sleep(250 * time.Millisecond)
	}
}

type SampleResult struct {
	AcceptCounts     []int64
	RejectCounts     []int64
	Totals           []int64
	AcceptanceRates  []float64
	NonzeroCells     float64
	OverallAcceptPct float64
	Timestamp        time.Time
}

func sampleMetrics(client *http.Client, baseURL, sensorID string, iterations int, interval time.Duration, buckets []string, noise, closeness float64, neighbor int, rawW *csv.Writer) []SampleResult {
	results := make([]SampleResult, 0, iterations)

	for i := 0; i < iterations; i++ {
		// Fetch acceptance metrics
		resp, err := client.Get(fmt.Sprintf("%s/api/lidar/acceptance?sensor_id=%s", baseURL, sensorID))
		if err != nil {
			log.Printf("WARNING: Sample %d failed: %v", i+1, err)
			time.Sleep(interval)
			continue
		}

		var m map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			log.Printf("WARNING: Sample %d decode failed: %v", i+1, err)
			resp.Body.Close()
			time.Sleep(interval)
			continue
		}
		resp.Body.Close()

		acceptCounts := toInt64Slice(m["AcceptCounts"], len(buckets))
		rejectCounts := toInt64Slice(m["RejectCounts"], len(buckets))
		totals := toInt64Slice(m["Totals"], len(buckets))
		rates := toFloat64Slice(m["AcceptanceRates"], len(buckets))

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
		if resp2, err := client.Get(fmt.Sprintf("%s/api/lidar/grid_status?sensor_id=%s", baseURL, sensorID)); err == nil {
			var gs map[string]interface{}
			if json.NewDecoder(resp2.Body).Decode(&gs) == nil {
				if bc, ok := gs["background_count"]; ok {
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
			resp2.Body.Close()
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
		results = append(results, result)

		// Write raw data
		writeRawRow(rawW, noise, closeness, neighbor, i, result, buckets)

		if i < iterations-1 {
			time.Sleep(interval)
		}
	}

	return results
}

func writeHeaders(w, rawW *csv.Writer, buckets []string) {
	// Summary header
	header := []string{"noise_relative", "closeness_multiplier", "neighbor_confirmation_count"}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_mean")
	}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_stddev")
	}
	header = append(header, "nonzero_cells_mean", "nonzero_cells_stddev", "overall_accept_mean", "overall_accept_stddev")
	w.Write(header)

	// Raw header
	rawHeader := []string{"noise_relative", "closeness_multiplier", "neighbor_confirmation_count", "iter", "timestamp"}
	for _, b := range buckets {
		rawHeader = append(rawHeader, "accept_counts_"+b)
	}
	for _, b := range buckets {
		rawHeader = append(rawHeader, "reject_counts_"+b)
	}
	for _, b := range buckets {
		rawHeader = append(rawHeader, "totals_"+b)
	}
	for _, b := range buckets {
		rawHeader = append(rawHeader, "acceptance_rates_"+b)
	}
	rawHeader = append(rawHeader, "nonzero_cells", "overall_accept_percent")
	rawW.Write(rawHeader)
}

func writeRawRow(w *csv.Writer, noise, closeness float64, neighbor, iter int, result SampleResult, buckets []string) {
	row := []string{
		fmt.Sprintf("%.6f", noise),
		fmt.Sprintf("%.6f", closeness),
		fmt.Sprintf("%d", neighbor),
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

func writeSummary(w *csv.Writer, noise, closeness float64, neighbor int, results []SampleResult, buckets []string) {
	if len(results) == 0 {
		log.Printf("WARNING: No results to summarize")
		return
	}

	// Compute per-bucket means and stddevs
	means := make([]float64, len(buckets))
	stds := make([]float64, len(buckets))

	for bi := range buckets {
		vals := make([]float64, len(results))
		for ri, r := range results {
			vals[ri] = r.AcceptanceRates[bi]
		}
		means[bi], stds[bi] = meanStddev(vals)
	}

	// Compute nonzero cells stats
	nonzeroVals := make([]float64, len(results))
	for i, r := range results {
		nonzeroVals[i] = r.NonzeroCells
	}
	nonzeroMean, nonzeroStd := meanStddev(nonzeroVals)

	// Compute overall acceptance stats
	overallVals := make([]float64, len(results))
	for i, r := range results {
		overallVals[i] = r.OverallAcceptPct
	}
	overallMean, overallStd := meanStddev(overallVals)

	log.Printf("Results: overall_accept=%.4f±%.4f, nonzero_cells=%.0f±%.0f",
		overallMean, overallStd, nonzeroMean, nonzeroStd)

	// Write CSV row
	row := []string{
		fmt.Sprintf("%.6f", noise),
		fmt.Sprintf("%.6f", closeness),
		fmt.Sprintf("%d", neighbor),
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

	w.Write(row)
	w.Flush()
}
