package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

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

// toFloat64Slice converts a JSON-decoded value into a fixed-length []float64.
// If the value is nil or shorter than length, it will be padded with zeros.
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

func main() {
	monitorURL := flag.String("monitor", "http://localhost:8081", "Base URL for lidar monitor")
	sensorID := flag.String("sensor", "hesai-pandar40p", "Sensor ID")
	pcapFile := flag.String("pcap-file", "", "PCAP file to replay on the monitor (optional)")
	start := flag.Float64("start", 0.01, "Start noise relative fraction")
	end := flag.Float64("end", 0.3, "End noise relative fraction")
	step := flag.Float64("step", 0.01, "Step increment")
	wait := flag.Duration("wait", 3*time.Second, "Wait time after setting param before sampling metrics")
	enableDiag := flag.Bool("diag", false, "Enable diagnostics while sweeping")
	// New settle mode: do not change parameters, reset grid and sample acceptance repeatedly
	settle := flag.Bool("settle", false, "Run settling measurement: reset grid and sample acceptance repeatedly without changing params")
	iterations := flag.Int("iterations", 100, "Number of sampling iterations in settle mode")
	interval := flag.Duration("interval", 2*time.Second, "Interval between samples in settle mode (overrides wait)")
	// incremental mode: for each noise value, reset grid, set noise, then sample for iterationsPer
	incremental := flag.Bool("incremental", false, "Run incremental sweep: for each noise step reset grid, set param, then sample iterationsPer times")
	iterationsPer := flag.Int("iterations-per", 50, "Number of sampling iterations to run per noise step in incremental mode")
	intervalPer := flag.Duration("interval-per", 2*time.Second, "Interval between samples in incremental mode (overrides wait)")
	output := flag.String("output", "", "Output CSV filename (defaults to bg-sweep-<timestamp>.csv in current directory)")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}

	// prepare output file in current directory
	var outFile *os.File
	var outWriter *bufio.Writer
	filename := *output
	if filename == "" {
		filename = fmt.Sprintf("bg-sweep-%s.csv", time.Now().Format("20060102-150405"))
	}
	if f, err := os.Create(filename); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create output file %s: %v\n", filename, err)
		outFile = nil
		outWriter = nil
	} else {
		outFile = f
		outWriter = bufio.NewWriter(f)
		defer func() {
			outWriter.Flush()
			outFile.Close()
		}()
	}

	// helper writes to stdout and to the CSV file if available
	writeData := func(s string) {
		fmt.Println(s)
		if outWriter != nil {
			outWriter.WriteString(s + "\n")
			outWriter.Flush()
		}
	}

	// Query the monitor once to obtain the live AcceptanceBucketsMeters
	buckets := []string{}
	if resp, err := client.Get(*monitorURL + "/api/lidar/acceptance?sensor_id=" + *sensorID); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch buckets from monitor: %v -- falling back to defaults\n", err)
	} else {
		var m map[string]interface{}
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&m); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not decode buckets response: %v -- falling back to defaults\n", err)
		} else {
			if bm, ok := m["BucketsMeters"].([]interface{}); ok && len(bm) > 0 {
				for _, bi := range bm {
					switch v := bi.(type) {
					case float64:
						// format integers without decimal when possible
						if v == math.Trunc(v) {
							buckets = append(buckets, fmt.Sprintf("%.0f", v))
						} else {
							// use fixed decimal to avoid scientific notation
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
		// fallback that matches the default in BackgroundGrid.NewBackgroundManager
		buckets = []string{"1", "2", "4", "8", "10", "12", "16", "20", "50", "100", "200"}
		fmt.Fprintln(os.Stderr, "note: using fallback acceptance buckets")
	}
	// print header for the regular sweep mode
	header := "noise_relative"
	for _, b := range buckets {
		header += ",accept_counts_" + b
	}
	for _, b := range buckets {
		header += ",reject_counts_" + b
	}
	for _, b := range buckets {
		header += ",totals_" + b
	}
	for _, b := range buckets {
		header += ",acceptance_rates_" + b
	}
	writeData(header)
	// If PCAP file is provided, send it to the monitor to start PCAP replay
	if *pcapFile != "" {
		params := map[string]interface{}{"pcap_file": *pcapFile}
		b, _ := json.Marshal(params)
		req, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/pcap/start?sensor_id="+*sensorID, bytes.NewReader(b))
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

	// If settle mode, perform reset + repeated sampling
	if *settle {
		// Reset grid
		reqReset, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/grid_reset?sensor_id="+*sensorID, nil)
		if resp, err := client.Do(reqReset); err != nil {
			fmt.Fprintf(os.Stderr, "grid reset error: %v\n", err)
			os.Exit(1)
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if *incremental {
			inter := *intervalPer
			if inter <= 0 {
				inter = *wait
			}
			// iterate noise values
			for v := *start; v <= *end+1e-9; v += *step {
				// Reset grid
				reqReset, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/grid_reset?sensor_id="+*sensorID, nil)
				if resp, err := client.Do(reqReset); err != nil {
					fmt.Fprintf(os.Stderr, "grid reset error: %v\n", err)
					os.Exit(1)
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}

				// Set noise param for this step
				params := map[string]interface{}{"noise_relative": v, "enable_diagnostics": *enableDiag}
				b, _ := json.Marshal(params)
				req, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/params?sensor_id="+*sensorID, bytes.NewReader(b))
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
					fmt.Printf("Applied params response: %s\n", string(body))
				}

				// Reset acceptance counters
				reqAR, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/acceptance/reset?sensor_id="+*sensorID, nil)
				if resp, err := client.Do(reqAR); err != nil {
					fmt.Fprintf(os.Stderr, "acceptance reset error: %v\n", err)
					os.Exit(1)
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}

				// Header per noise value
				writeData(fmt.Sprintf("# noise_relative=%v", v))
				writeData("iter,timestamp,accept_counts_json,reject_counts_json,totals_json,acceptance_rates_json")

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
						// expand arrays into individual columns
						a := toInt64Slice(m["AcceptCounts"], len(buckets))
						rj := toInt64Slice(m["RejectCounts"], len(buckets))
						tot := toInt64Slice(m["Totals"], len(buckets))
						rates := toFloat64Slice(m["AcceptanceRates"], len(buckets))
						// build CSV line: iter,timestamp,<accept...>,<reject...>,<totals...>,<rates...>
						line := fmt.Sprintf("%d,%s", i, time.Now().Format(time.RFC3339Nano))
						for _, v := range a {
							line += fmt.Sprintf(",%d", v)
						}
						for _, v := range rj {
							line += fmt.Sprintf(",%d", v)
						}
						for _, v := range tot {
							line += fmt.Sprintf(",%d", v)
						}
						for _, v := range rates {
							// use fixed format with 6 decimal places to avoid scientific notation
							line += fmt.Sprintf(",%.6f", v)
						}
						writeData(line)
					}
					time.Sleep(inter)
				}
			}
			return
		}

		// Reset acceptance counters
		reqAR, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/acceptance/reset?sensor_id="+*sensorID, nil)
		if resp, err := client.Do(reqAR); err != nil {
			fmt.Fprintf(os.Stderr, "acceptance reset error: %v\n", err)
			os.Exit(1)
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		// Header for settle output
		writeData("iter,timestamp,accept_counts_json,reject_counts_json,totals_json,acceptance_rates_json")
		inter := *interval
		if inter <= 0 {
			inter = *wait
		}
		for i := 0; i < *iterations; i++ {
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
				accept, _ := json.Marshal(m["AcceptCounts"])
				reject, _ := json.Marshal(m["RejectCounts"])
				totals, _ := json.Marshal(m["Totals"])
				rates, _ := json.Marshal(m["AcceptanceRates"])
				writeData(fmt.Sprintf("%d,%s,%s,%s,%s,%s", i, time.Now().Format(time.RFC3339Nano), accept, reject, totals, rates))
			}
			time.Sleep(inter)
		}
		return
	}

	// If incremental mode (not settle), for each noise value reset grid, set params,
	// reset acceptance counters and sample iterationsPer times.
	if *incremental {
		inter := *intervalPer
		if inter <= 0 {
			inter = *wait
		}
		for noise := *start; noise <= *end+1e-9; noise += *step {
			// Reset grid for a clean start at each noise value
			reqReset, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/grid_reset?sensor_id="+*sensorID, nil)
			if resp, err := client.Do(reqReset); err != nil {
				fmt.Fprintf(os.Stderr, "grid reset error: %v\n", err)
				os.Exit(1)
			} else {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}

			// Set noise param for this step
			params := map[string]interface{}{"noise_relative": noise, "enable_diagnostics": *enableDiag}
			b, _ := json.Marshal(params)
			req, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/params?sensor_id="+*sensorID, bytes.NewReader(b))
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
				fmt.Printf("Applied params response: %s\n", string(body))
			}

			// Reset acceptance counters
			reqAR, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/acceptance/reset?sensor_id="+*sensorID, nil)
			if resp, err := client.Do(reqAR); err != nil {
				fmt.Fprintf(os.Stderr, "acceptance reset error: %v\n", err)
				os.Exit(1)
			} else {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}

			// Print a header for this noise step and then per-iteration data
			writeData(fmt.Sprintf("# noise_relative=%v", noise))
			// build iterative header: noise_relative,iter,timestamp,...
			head := "noise_relative,iter,timestamp"
			for _, b := range buckets {
				head += ",accept_counts_" + b
			}
			for _, b := range buckets {
				head += ",reject_counts_" + b
			}
			for _, b := range buckets {
				head += ",totals_" + b
			}
			for _, b := range buckets {
				head += ",acceptance_rates_" + b
			}
			writeData(head)

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
					a := toInt64Slice(m["AcceptCounts"], len(buckets))
					rj := toInt64Slice(m["RejectCounts"], len(buckets))
					tot := toInt64Slice(m["Totals"], len(buckets))
					rates := toFloat64Slice(m["AcceptanceRates"], len(buckets))
					line := fmt.Sprintf("%.6f,%d,%s", noise, i, time.Now().Format(time.RFC3339Nano))
					for _, v := range a {
						line += fmt.Sprintf(",%d", v)
					}
					for _, v := range rj {
						line += fmt.Sprintf(",%d", v)
					}
					for _, v := range tot {
						line += fmt.Sprintf(",%d", v)
					}
					for _, v := range rates {
						line += fmt.Sprintf(",%.6f", v)
					}
					writeData(line)
				}
				time.Sleep(inter)
			}
		}
		return
	}
	for v := *start; v <= *end+1e-9; v += *step {
		// set params
		params := map[string]interface{}{"noise_relative": v, "enable_diagnostics": *enableDiag}
		b, _ := json.Marshal(params)
		req, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/params?sensor_id="+*sensorID, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if resp, err := client.Do(req); err != nil {
			fmt.Fprintf(os.Stderr, "set params error: %v\n", err)
			os.Exit(1)
		} else {
			// Read the response to confirm the monitor applied the params.
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				fmt.Fprintf(os.Stderr, "set params returned %d: %s\n", resp.StatusCode, string(body))
				os.Exit(1)
			}
		}

		// reset acceptance metrics
		req2, _ := http.NewRequest(http.MethodPost, *monitorURL+"/api/lidar/acceptance/reset?sensor_id="+*sensorID, nil)
		if resp, err := client.Do(req2); err != nil {
			fmt.Fprintf(os.Stderr, "reset metrics error: %v\n", err)
			os.Exit(1)
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		time.Sleep(*wait)

		// fetch acceptance metrics
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
			// Print CSV line with JSON blobs for the arrays
			accept, _ := json.Marshal(m["AcceptCounts"])
			reject, _ := json.Marshal(m["RejectCounts"])
			totals, _ := json.Marshal(m["Totals"])
			rates, _ := json.Marshal(m["AcceptanceRates"])
			writeData(fmt.Sprintf("%v,%s,%s,%s,%s", v, accept, reject, totals, rates))
		}
	}
}
