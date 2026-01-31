package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
	"github.com/banshee-data/velocity.report/internal/security"
)

func main() {
	// Common flags
	monitorURL := flag.String("monitor", "http://localhost:8081", "Base URL for lidar monitor")
	sensorID := flag.String("sensor", "hesai-pandar40p", "Sensor ID")
	output := flag.String("output", "", "Output CSV filename (defaults to sweep-<timestamp>.csv)")

	// PCAP support
	pcapFile := flag.String("pcap", "", "PCAP file to replay (enables PCAP mode)")
	pcapSettle := flag.Duration("pcap-settle", 20*time.Second, "Time to wait after PCAP replay before sampling")

	// Sweep mode selection
	sweepMode := flag.String("mode", "multi", "Sweep mode: 'multi' (all combinations), 'noise' (vary noise only), 'closeness' (vary closeness only), 'neighbour' (vary neighbour only)")

	// Parameter ranges for multi-sweep
	noiseList := flag.String("noise", "", "Comma-separated noise values (e.g. 0.005,0.01,0.02) or range start:end:step")
	closenessList := flag.String("closeness", "", "Comma-separated closeness values (e.g. 1.5,2.0,2.5) or range start:end:step")
	neighbourList := flag.String("neighbours", "", "Comma-separated neighbour values (e.g. 0,1,2)")

	// Single-variable sweep ranges (when mode != multi)
	noiseStart := flag.Float64("noise-start", 0.005, "Start noise value for noise sweep")
	noiseEnd := flag.Float64("noise-end", 0.03, "End noise value for noise sweep")
	noiseStep := flag.Float64("noise-step", 0.005, "Step for noise sweep")

	closenessStart := flag.Float64("closeness-start", 1.5, "Start closeness value for closeness sweep")
	closenessEnd := flag.Float64("closeness-end", 3.0, "End closeness value for closeness sweep")
	closenessStep := flag.Float64("closeness-step", 0.5, "Step for closeness sweep")

	neighbourStart := flag.Int("neighbour-start", 0, "Start neighbour value for neighbour sweep")
	neighbourEnd := flag.Int("neighbour-end", 3, "End neighbour value for neighbour sweep")
	neighbourStep := flag.Int("neighbour-step", 1, "Step for neighbour sweep")

	// Fixed values for single-variable sweeps
	fixedNoise := flag.Float64("fixed-noise", 0.01, "Fixed noise value (when not sweeping noise)")
	fixedCloseness := flag.Float64("fixed-closeness", 2.0, "Fixed closeness value (when not sweeping closeness)")
	fixedNeighbour := flag.Int("fixed-neighbour", 1, "Fixed neighbour value (when not sweeping neighbour)")

	// Sampling configuration
	iterations := flag.Int("iterations", 30, "Number of samples per parameter combination")
	interval := flag.Duration("interval", 2*time.Second, "Interval between samples")
	settleTime := flag.Duration("settle-time", 5*time.Second, "Time to wait for grid to settle after applying params")

	// Seed control
	seedFlag := flag.String("seed", "true", "Seed behaviour: 'true', 'false', or 'toggle' (alternates per combo)")

	flag.Parse()

	// Create monitor client
	httpClient := &http.Client{Timeout: 30 * time.Second}
	client := monitor.NewClient(httpClient, *monitorURL, *sensorID)

	// Start PCAP replay if requested
	if *pcapFile != "" {
		if err := client.StartPCAPReplay(*pcapFile, 60); err != nil {
			log.Fatalf("Failed to start PCAP replay: %v", err)
		}
		log.Printf("PCAP mode enabled: %s (settle time: %v)", *pcapFile, *pcapSettle)
	}

	// Fetch acceptance buckets for header construction
	buckets := client.FetchBuckets()
	log.Printf("Using %d acceptance buckets", len(buckets))

	// Determine parameter combinations based on sweep mode
	var noiseCombos, closenessCombos []float64
	var neighbourCombos []int

	switch *sweepMode {
	case "multi":
		// Multi-parameter sweep: use lists or parse ranges
		noiseCombos = parseParamList(*noiseList, *noiseStart, *noiseEnd, *noiseStep)
		closenessCombos = parseParamList(*closenessList, *closenessStart, *closenessEnd, *closenessStep)
		neighbourCombos = parseIntParamList(*neighbourList, *neighbourStart, *neighbourEnd, *neighbourStep)

	case "noise":
		// Sweep noise only, fix others
		noiseCombos = sweep.GenerateRange(*noiseStart, *noiseEnd, *noiseStep)
		closenessCombos = []float64{*fixedCloseness}
		neighbourCombos = []int{*fixedNeighbour}

	case "closeness":
		// Sweep closeness only, fix others
		noiseCombos = []float64{*fixedNoise}
		closenessCombos = sweep.GenerateRange(*closenessStart, *closenessEnd, *closenessStep)
		neighbourCombos = []int{*fixedNeighbour}

	case "neighbour":
		// Sweep neighbour only, fix others
		noiseCombos = []float64{*fixedNoise}
		closenessCombos = []float64{*fixedCloseness}
		neighbourCombos = sweep.GenerateIntRange(*neighbourStart, *neighbourEnd, *neighbourStep)

	default:
		log.Fatalf("Invalid sweep mode: %s (must be multi, noise, closeness, or neighbour)", *sweepMode)
	}

	// Provide defaults if lists are empty
	if len(noiseCombos) == 0 {
		noiseCombos = []float64{0.005, 0.01, 0.02}
	}
	if len(closenessCombos) == 0 {
		closenessCombos = []float64{1.5, 2.0, 2.5}
	}
	if len(neighbourCombos) == 0 {
		neighbourCombos = []int{0, 1, 2}
	}

	totalCombos := len(noiseCombos) * len(closenessCombos) * len(neighbourCombos)
	log.Printf("Sweep mode: %s", *sweepMode)
	log.Printf("Parameter combinations: %d (noise: %d, closeness: %d, neighbour: %d)",
		totalCombos, len(noiseCombos), len(closenessCombos), len(neighbourCombos))

	// Prepare output files
	filename := *output
	if filename == "" {
		filename = fmt.Sprintf("sweep-%s-%s.csv", *sweepMode, time.Now().Format("20060102-150405"))
	}

	// Validate output path to prevent path traversal attacks
	if err := security.ValidateOutputPath(filename); err != nil {
		log.Fatalf("Invalid output path %s: %v", filename, err)
	}

	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Could not create output file %s: %v", filename, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	rawFilename := strings.TrimSuffix(filename, ".csv") + "-raw.csv"

	// Validate raw output path
	if err := security.ValidateOutputPath(rawFilename); err != nil {
		log.Fatalf("Invalid raw output path %s: %v", rawFilename, err)
	}

	fRaw, err := os.Create(rawFilename)
	if err != nil {
		log.Fatalf("Could not create raw output file %s: %v", rawFilename, err)
	}
	defer fRaw.Close()
	rawW := csv.NewWriter(fRaw)
	defer rawW.Flush()

	// Write headers using internal package
	sweep.WriteSummaryHeaders(w, buckets)
	sweep.WriteRawHeaders(rawW, buckets)

	// Create sampler
	sampler := sweep.NewSampler(client, buckets, *interval)

	// Run sweep
	comboNum := 0
	seedToggle := false

	for _, noise := range noiseCombos {
		for _, closeness := range closenessCombos {
			for _, neighbour := range neighbourCombos {
				comboNum++
				log.Printf("\n=== Combination %d/%d: noise=%.4f, closeness=%.2f, neighbour=%d ===",
					comboNum, totalCombos, noise, closeness, neighbour)

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
				if err := client.ResetGrid(); err != nil {
					log.Printf("WARNING: Grid reset failed: %v", err)
				}

				// Set parameters
				params := monitor.BackgroundParams{
					NoiseRelative:              noise,
					ClosenessMultiplier:        closeness,
					NeighbourConfirmationCount: neighbour,
					SeedFromFirstFrame:         seed,
				}
				if err := client.SetParams(params); err != nil {
					log.Printf("ERROR: Failed to set params: %v", err)
					continue
				}

				// Reset acceptance counters
				if err := client.ResetAcceptance(); err != nil {
					log.Printf("WARNING: Failed to reset acceptance: %v", err)
				}

				// PCAP mode: trigger replay and wait for settle
				if *pcapFile != "" {
					if err := client.StartPCAPReplay(*pcapFile, 60); err != nil {
						log.Printf("WARNING: PCAP replay failed: %v (will retry)", err)
						time.Sleep(5 * time.Second)
						if err := client.StartPCAPReplay(*pcapFile, 60); err != nil {
							log.Printf("ERROR: PCAP replay failed again: %v", err)
							continue
						}
					}
					time.Sleep(*pcapSettle)
				} else {
					// Live mode: wait for grid to settle
					client.WaitForGridSettle(*settleTime)
				}

				// Sample metrics using the sampler
				cfg := sweep.SampleConfig{
					Noise:      noise,
					Closeness:  closeness,
					Neighbour:  neighbour,
					Iterations: *iterations,
					RawWriter:  rawW,
				}
				results := sampler.Sample(cfg)

				// Write summary using internal package
				sweep.WriteSummary(w, noise, closeness, neighbour, results, buckets)
			}
		}
	}

	log.Printf("\nSweep complete!")
	log.Printf("Summary: %s", filename)
	log.Printf("Raw data: %s", rawFilename)
}

// parseParamList parses a comma-separated list or generates a range using internal packages.
func parseParamList(list string, start, end, step float64) []float64 {
	if list != "" {
		vals, err := sweep.ParseCSVFloat64s(list)
		if err != nil {
			log.Fatalf("Invalid parameter list: %v", err)
		}
		return vals
	}
	return sweep.GenerateRange(start, end, step)
}

// parseIntParamList parses a comma-separated list or generates an integer range using internal packages.
func parseIntParamList(list string, start, end, step int) []int {
	if list != "" {
		vals, err := sweep.ParseCSVInts(list)
		if err != nil {
			log.Fatalf("Invalid parameter list: %v", err)
		}
		return vals
	}
	return sweep.GenerateIntRange(start, end, step)
}
