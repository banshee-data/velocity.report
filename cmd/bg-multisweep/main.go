package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
// If the value is nil or shorter than length, it will be padded with zeros.
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
	monitorURL := flag.String("monitor", "http://localhost:8081", "Base URL for lidar monitor")
	sensorID := flag.String("sensor", "hesai-pandar40p", "Sensor ID")
	pcapFile := flag.String("pcap-file", "", "PCAP file to replay on the monitor (optional)")
	noiseStart := flag.Float64("start", 0.01, "Start noise relative fraction")
	noiseEnd := flag.Float64("end", 0.3, "End noise relative fraction")
	noiseStep := flag.Float64("step", 0.01, "Step increment for noise")
	closenessList := flag.String("closeness", "", "Comma-separated list of closeness_multiplier values to try (e.g. 2.0,3.0,4.0)")
	neighborList := flag.String("neighbors", "", "Comma-separated list of neighbor confirmation integer counts to try (e.g. 1,2,3)")
	iterationsPer := flag.Int("iterations-per", 30, "Number of acceptance samples to take per parameter combination")
	intervalPer := flag.Duration("interval-per", 2*time.Second, "Interval between samples")
	// Time to wait after applying params and resetting grid before sampling
	settleTime := flag.Duration("settle-time", 5*time.Second, "Time to wait for grid to settle after applying params (or 0 to skip)")
	output := flag.String("output", "", "Output CSV filename (defaults to bg-multisweep-<timestamp>.csv)")
	rawOutput := flag.String("raw-output", "", "Raw per-iteration CSV filename (defaults to <output>-raw.csv)")
	seedFlag := flag.String("seed", "", "Seed behavior: one of '', 'on', 'off', 'toggle' - applied via API per-parameter-combo")
	dryRun := flag.Bool("dry-run", false, "Print parsed flags and exit (no network calls)")
	flag.Parse()

	if *dryRun {
		// lightweight debug output for diagnosing shell/flag issues
		fmt.Fprintf(os.Stderr, "os.Args=%v\n", os.Args)
		fmt.Fprintf(os.Stderr, "parsed: monitor=%s sensor=%s start=%.6f end=%.6f step=%.6f closeness=%s neighbors=%s iterations-per=%d interval-per=%v\n",
			*monitorURL, *sensorID, *noiseStart, *noiseEnd, *noiseStep, *closenessList, *neighborList, *iterationsPer, *intervalPer)
		return
	}

	closenessVals, err := parseCSVFloatSlice(*closenessList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid closeness list: %v\n", err)
		os.Exit(1)
	}
	neighborVals, err := parseCSVIntSlice(*neighborList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid neighbor list: %v\n", err)
		os.Exit(1)
	}

	// If closeness or neighbor lists are empty, use sensible defaults
	if len(closenessVals) == 0 {
		closenessVals = []float64{2.0, 3.0, 4.0}
	}
	if len(neighborVals) == 0 {
		neighborVals = []int{1, 2, 3}
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// If PCAP file is provided, send it to the monitor to start PCAP replay
	if *pcapFile != "" {
		params := map[string]interface{}{"pcap_file": *pcapFile}
		b, _ := json.Marshal(params)
		req, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/pcap/start?sensor_id="+*sensorID, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		if resp, err := client.Do(req); err != nil {
			fmt.Fprintf(os.Stderr, "start PCAP replay error: %v\n", err)
			os.Exit(1)
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				fmt.Fprintf(os.Stderr, "start PCAP returned %d: %s\n", resp.StatusCode, string(body))
				os.Exit(1)
			}
			fmt.Printf("PCAP replay started: %s\n", string(body))
		}
	}

	// fetch buckets to build header
	buckets := []string{}
	if resp, err := client.Get(*monitorURL + "/api/lidar/acceptance?sensor_id=" + *sensorID); err == nil {
		var m map[string]interface{}
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&m); err == nil {
			if bm, ok := m["BucketsMeters"].([]interface{}); ok && len(bm) > 0 {
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
			}
		}
		resp.Body.Close()
	}
	if len(buckets) == 0 {
		buckets = []string{"1", "2", "4", "8", "10", "12", "16", "20", "50", "100", "200"}
		fmt.Fprintln(os.Stderr, "note: using fallback acceptance buckets")
	}

	// prepare output file
	filename := *output
	if filename == "" {
		filename = fmt.Sprintf("bg-multisweep-%s.csv", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not create output file %s: %v\n", filename, err)
		os.Exit(1)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	// prepare raw output file
	rawFilename := *rawOutput
	if rawFilename == "" {
		rawFilename = filename + "-raw.csv"
	}
	fRaw, err := os.Create(rawFilename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not create raw output file %s: %v\n", rawFilename, err)
		os.Exit(1)
	}
	defer fRaw.Close()
	rawW := csv.NewWriter(fRaw)
	defer rawW.Flush()
	// raw header: expand per-bucket columns so downstream tools can read directly
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
	// Also capture nonzero cell count for the background snapshot at each sample
	rawHeader = append(rawHeader, "nonzero_cells")
	rawHeader = append(rawHeader, "overall_accept_percent")
	if err := rawW.Write(rawHeader); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write raw header: %v\n", err)
		os.Exit(1)
	}

	// build header
	header := []string{"noise_relative", "closeness_multiplier", "neighbor_confirmation_count"}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_mean")
	}
	for _, b := range buckets {
		header = append(header, "bucket_"+b+"_stddev")
	}
	if err := w.Write(header); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write header: %v\n", err)
		os.Exit(1)
	}

	// iterate parameter combinations
	for noise := *noiseStart; noise <= *noiseEnd+1e-12; noise += *noiseStep {
		for _, clos := range closenessVals {
			for _, neigh := range neighborVals {
				// reset grid
				reqReset, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/grid_reset?sensor_id="+*sensorID, nil)
				if resp, err := client.Do(reqReset); err != nil {
					fmt.Fprintf(os.Stderr, "grid reset error: %v\n", err)
					os.Exit(1)
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}

				// set params: noise_relative, closeness_multiplier, neighbor_confirmation_count
				params := map[string]interface{}{
					"noise_relative":              noise,
					"closeness_multiplier":        clos,
					"neighbor_confirmation_count": neigh,
				}

				// handle seed flag variants: on/off/toggle
				if *seedFlag != "" {
					if *seedFlag == "on" {
						params["seed_from_first"] = true
					} else if *seedFlag == "off" {
						params["seed_from_first"] = false
					} else if *seedFlag == "toggle" {
						// fetch current params and invert seed setting
						if respCur, err := client.Get(*monitorURL + "/api/lidar/params?sensor_id=" + *sensorID); err == nil {
							var cur map[string]interface{}
							dec := json.NewDecoder(respCur.Body)
							if err := dec.Decode(&cur); err == nil {
								curSeed := false
								if v, ok := cur["seed_from_first"]; ok {
									switch vv := v.(type) {
									case bool:
										curSeed = vv
									case float64:
										curSeed = vv != 0
									}
								}
								params["seed_from_first"] = !curSeed
							}
							respCur.Body.Close()
						}
					}
				}
				b, _ := json.Marshal(params)
				req, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/params?sensor_id="+*sensorID, strings.NewReader(string(b)))
				req.Header.Set("Content-Type", "application/json")
				if resp, err := client.Do(req); err != nil {
					fmt.Fprintf(os.Stderr, "set params error: %v\n", err)
					os.Exit(1)
				} else {
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode/100 != 2 {
						fmt.Fprintf(os.Stderr, "set params returned %d: %s\n", resp.StatusCode, string(body))
						os.Exit(1)
					}
					fmt.Printf("Applied params: noise=%.6f closeness=%.3f neigh=%d -> %s\n", noise, clos, neigh, string(body))
				}

				// reset acceptance counters
				reqAR, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/acceptance/reset?sensor_id="+*sensorID, nil)
				if resp, err := client.Do(reqAR); err != nil {
					fmt.Fprintf(os.Stderr, "acceptance reset error: %v\n", err)
					os.Exit(1)
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}

				// Wait for grid to settle/populate after reset+params. This helps avoid
				// sampling too quickly before the BackgroundManager has processed frames.
				if *settleTime > 0 {
					deadline := time.Now().Add(*settleTime)
					for time.Now().Before(deadline) {
						// Query grid status
						if resp, err := client.Get(*monitorURL + "/api/lidar/grid_status?sensor_id=" + *sensorID); err == nil {
							var gs map[string]interface{}
							dec := json.NewDecoder(resp.Body)
							if err := dec.Decode(&gs); err == nil {
								// Prefer background_count if present
								if bc, ok := gs["background_count"]; ok {
									if n, ok := bc.(float64); ok && n > 0 {
										resp.Body.Close()
										break
									}
								}
								// Fallback: inspect times_seen_dist map to see if not all zeros
								if tsd, ok := gs["times_seen_dist"].(map[string]interface{}); ok {
									// if any key != "0" has count > 0, consider settled
									for k, v := range tsd {
										if k != "0" {
											if nv, ok := v.(float64); ok && nv > 0 {
												resp.Body.Close()
												goto settled
											}
										}
									}
								}
							}
							resp.Body.Close()
						}
						time.Sleep(250 * time.Millisecond)
					}
				}
			settled:

				// sample acceptance rates iterationsPer times and write raw rows
				rateSamples := make([][]float64, 0, *iterationsPer)
				overallSamples := make([]float64, 0, *iterationsPer)
				nonzeroSamples := make([]float64, 0, *iterationsPer)
				for i := 0; i < *iterationsPer; i++ {
					if resp, err := client.Get(*monitorURL + "/api/lidar/acceptance?sensor_id=" + *sensorID); err != nil {
						fmt.Fprintf(os.Stderr, "fetch metrics error: %v\n", err)
						os.Exit(1)
					} else {
						var m map[string]interface{}
						dec := json.NewDecoder(resp.Body)
						if err := dec.Decode(&m); err != nil {
							fmt.Fprintf(os.Stderr, "decode error: %v\n", err)
							resp.Body.Close()
							os.Exit(1)
						}
						resp.Body.Close()
						rates := toFloat64Slice(m["AcceptanceRates"], len(buckets))
						rateSamples = append(rateSamples, rates)

						// also gather accept counts and totals to compute overall acceptance percent
						acceptCounts := toInt64Slice(m["AcceptCounts"], len(buckets))
						totals := toInt64Slice(m["Totals"], len(buckets))
						var sumAccept int64
						var sumTotals int64
						for k := range acceptCounts {
							sumAccept += acceptCounts[k]
							sumTotals += totals[k]
						}
						var overall float64
						if sumTotals > 0 {
							overall = float64(sumAccept) / float64(sumTotals)
						} else {
							overall = 0
						}
						overallSamples = append(overallSamples, overall)

						// fetch snapshot summary to get nonzero cell count (if available)
						nonzero := 0.0
						if resp2, err := client.Get(*monitorURL + "/api/lidar/snapshot?sensor_id=" + *sensorID); err == nil {
							var s map[string]interface{}
							dec2 := json.NewDecoder(resp2.Body)
							if err := dec2.Decode(&s); err == nil {
								if v, ok := s["non_empty_cells"]; ok {
									switch nv := v.(type) {
									case float64:
										nonzero = nv
									case int:
										nonzero = float64(nv)
									case int64:
										nonzero = float64(nv)
									}
								}
							}
							resp2.Body.Close()
						}
						nonzeroSamples = append(nonzeroSamples, nonzero)

						// write raw CSV row: expand arrays into individual numeric columns
						// reuse acceptCounts and totals already computed above
						rejectCounts := toInt64Slice(m["RejectCounts"], len(buckets))
						totalsCounts := totals
						ratesVals := toFloat64Slice(m["AcceptanceRates"], len(buckets))

						rawRow := []string{fmt.Sprintf("%.6f", noise), fmt.Sprintf("%.6f", clos), fmt.Sprintf("%d", neigh), fmt.Sprintf("%d", i), time.Now().Format(time.RFC3339Nano)}
						for _, v := range acceptCounts {
							rawRow = append(rawRow, fmt.Sprintf("%d", v))
						}
						for _, v := range rejectCounts {
							rawRow = append(rawRow, fmt.Sprintf("%d", v))
						}
						for _, v := range totalsCounts {
							rawRow = append(rawRow, fmt.Sprintf("%d", v))
						}
						for _, v := range ratesVals {
							rawRow = append(rawRow, fmt.Sprintf("%.6f", v))
						}
						rawRow = append(rawRow, fmt.Sprintf("%.0f", nonzero))
						rawRow = append(rawRow, fmt.Sprintf("%.6f", overall))
						if err := rawW.Write(rawRow); err != nil {
							fmt.Fprintf(os.Stderr, "failed to write raw csv row: %v\n", err)
							os.Exit(1)
						}
						rawW.Flush()
					}
					time.Sleep(*intervalPer)
				}

				// compute per-bucket mean/stddev
				means := make([]string, len(buckets))
				stds := make([]string, len(buckets))
				for bi := range buckets {
					vals := make([]float64, 0, len(rateSamples))
					for _, s := range rateSamples {
						vals = append(vals, s[bi])
					}
					m, sd := meanStddev(vals)
					means[bi] = fmt.Sprintf("%.6f", m)
					stds[bi] = fmt.Sprintf("%.6f", sd)
				}

				// compute overall acceptance mean/stddev and nonzero cell mean/stddev, then log
				overallMean, overallStd := meanStddev(overallSamples)
				nonzeroMean, nonzeroStd := meanStddev(nonzeroSamples)
				fmt.Printf("Summary: noise=%.6f closeness=%.6f neigh=%d overall_accept_mean=%.6f overall_accept_std=%.6f nonzero_cells_mean=%.0f nonzero_cells_std=%.0f\n",
					noise, clos, neigh, overallMean, overallStd, nonzeroMean, nonzeroStd)

				// write CSV line: noise,closeness,neighbor, means..., stds..., nonzero_mean, nonzero_std
				line := []string{fmt.Sprintf("%.6f", noise), fmt.Sprintf("%.6f", clos), fmt.Sprintf("%d", neigh)}
				line = append(line, means...)
				line = append(line, stds...)
				line = append(line, fmt.Sprintf("%.0f", nonzeroMean))
				line = append(line, fmt.Sprintf("%.0f", nonzeroStd))
				if err := w.Write(line); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write csv line: %v\n", err)
					os.Exit(1)
				}
				w.Flush()
			}
		}
	}

	fmt.Printf("multisweep complete, results written to %s\n", filename)
}
