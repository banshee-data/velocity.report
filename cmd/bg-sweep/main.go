package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	monitorURL := flag.String("monitor", "http://localhost:8081", "Base URL for lidar monitor")
	sensorID := flag.String("sensor", "hesai-pandar40p", "Sensor ID")
	start := flag.Float64("start", 0.01, "Start noise relative fraction")
	end := flag.Float64("end", 0.3, "End noise relative fraction")
	step := flag.Float64("step", 0.01, "Step increment")
	wait := flag.Duration("wait", 3*time.Second, "Wait time after setting param before sampling metrics")
	enableDiag := flag.Bool("diag", false, "Enable diagnostics while sweeping")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}
	fmt.Println("noise_relative,accept_counts_json,reject_counts_json,totals_json,acceptance_rates_json")
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
			io.Copy(io.Discard, resp.Body)
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
			fmt.Printf("%v,%s,%s,%s,%s\n", v, accept, reject, totals, rates)
		}
	}
}
