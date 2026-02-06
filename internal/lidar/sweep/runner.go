package sweep

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
)

// SweepStatus represents the current state of a sweep run
type SweepStatus string

const (
	SweepStatusIdle     SweepStatus = "idle"
	SweepStatusRunning  SweepStatus = "running"
	SweepStatusComplete SweepStatus = "complete"
	SweepStatusError    SweepStatus = "error"
)

// SweepRequest defines the parameters for starting a sweep
type SweepRequest struct {
	Mode string `json:"mode"` // "multi", "noise", "closeness", "neighbour"

	// Multi-mode: explicit values
	NoiseValues     []float64 `json:"noise_values,omitempty"`
	ClosenessValues []float64 `json:"closeness_values,omitempty"`
	NeighbourValues []int     `json:"neighbour_values,omitempty"`

	// Single-variable sweep ranges
	NoiseStart     float64 `json:"noise_start,omitempty"`
	NoiseEnd       float64 `json:"noise_end,omitempty"`
	NoiseStep      float64 `json:"noise_step,omitempty"`
	ClosenessStart float64 `json:"closeness_start,omitempty"`
	ClosenessEnd   float64 `json:"closeness_end,omitempty"`
	ClosenessStep  float64 `json:"closeness_step,omitempty"`
	NeighbourStart int     `json:"neighbour_start,omitempty"`
	NeighbourEnd   int     `json:"neighbour_end,omitempty"`
	NeighbourStep  int     `json:"neighbour_step,omitempty"`

	// Fixed values for single-variable sweeps
	FixedNoise     float64 `json:"fixed_noise,omitempty"`
	FixedCloseness float64 `json:"fixed_closeness,omitempty"`
	FixedNeighbour int     `json:"fixed_neighbour,omitempty"`

	// Sampling
	Iterations int    `json:"iterations"`  // samples per combo
	Interval   string `json:"interval"`    // duration string e.g. "2s"
	SettleTime string `json:"settle_time"` // duration string e.g. "5s"

	// Seed control
	Seed string `json:"seed"` // "true", "false", or "toggle"
}

// ComboResult holds the summary result for one parameter combination
type ComboResult struct {
	Noise               float64   `json:"noise"`
	Closeness           float64   `json:"closeness"`
	Neighbour           int       `json:"neighbour"`
	OverallAcceptMean   float64   `json:"overall_accept_mean"`
	OverallAcceptStddev float64   `json:"overall_accept_stddev"`
	NonzeroCellsMean    float64   `json:"nonzero_cells_mean"`
	NonzeroCellsStddev  float64   `json:"nonzero_cells_stddev"`
	BucketMeans         []float64 `json:"bucket_means"`
	BucketStddevs       []float64 `json:"bucket_stddevs"`
	Buckets             []string  `json:"buckets"`
}

// SweepState holds the current state and results of a sweep
type SweepState struct {
	Status          SweepStatus   `json:"status"`
	StartedAt       *time.Time    `json:"started_at,omitempty"`
	CompletedAt     *time.Time    `json:"completed_at,omitempty"`
	TotalCombos     int           `json:"total_combos"`
	CompletedCombos int           `json:"completed_combos"`
	CurrentCombo    *ComboResult  `json:"current_combo,omitempty"`
	Results         []ComboResult `json:"results"`
	Error           string        `json:"error,omitempty"`
	Request         *SweepRequest `json:"request,omitempty"`
}

// Runner orchestrates parameter sweeps
type Runner struct {
	client *monitor.Client
	mu     sync.RWMutex
	state  SweepState
	cancel context.CancelFunc
}

// NewRunner creates a new sweep runner
func NewRunner(client *monitor.Client) *Runner {
	return &Runner{
		client: client,
		state:  SweepState{Status: SweepStatusIdle},
	}
}

// GetSweepState returns a typed copy of the current sweep state.
// This is the direct-use method for typed access.
func (r *Runner) GetSweepState() SweepState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Return a copy to avoid race conditions
	state := r.state
	results := make([]ComboResult, len(r.state.Results))
	copy(results, r.state.Results)
	state.Results = results
	return state
}

// GetState implements the monitor.SweepRunner interface.
// It returns the sweep state as interface{}.
func (r *Runner) GetState() interface{} {
	return r.GetSweepState()
}

// Start begins a new sweep run with a typed request.
// This is the main entry point for starting sweeps.
func (r *Runner) StartWithRequest(ctx context.Context, req SweepRequest) error {
	return r.start(ctx, req)
}

// Start implements the monitor.SweepRunner interface.
// It accepts an interface{} which should be a map or SweepRequest.
func (r *Runner) Start(ctx context.Context, reqInterface interface{}) error {
	// Convert interface{} to SweepRequest
	var req SweepRequest

	// Handle both map[string]interface{} from JSON and direct SweepRequest
	switch v := reqInterface.(type) {
	case SweepRequest:
		req = v
	case map[string]interface{}:
		// Manual conversion from map to SweepRequest
		if mode, ok := v["mode"].(string); ok {
			req.Mode = mode
		}
		if noiseValues, ok := v["noise_values"].([]interface{}); ok {
			req.NoiseValues = make([]float64, len(noiseValues))
			for i, val := range noiseValues {
				if f, ok := val.(float64); ok {
					req.NoiseValues[i] = f
				}
			}
		}
		if closenessValues, ok := v["closeness_values"].([]interface{}); ok {
			req.ClosenessValues = make([]float64, len(closenessValues))
			for i, val := range closenessValues {
				if f, ok := val.(float64); ok {
					req.ClosenessValues[i] = f
				}
			}
		}
		if neighbourValues, ok := v["neighbour_values"].([]interface{}); ok {
			req.NeighbourValues = make([]int, len(neighbourValues))
			for i, val := range neighbourValues {
				if f, ok := val.(float64); ok {
					req.NeighbourValues[i] = int(f)
				} else if n, ok := val.(int); ok {
					req.NeighbourValues[i] = n
				}
			}
		}
		if noiseStart, ok := v["noise_start"].(float64); ok {
			req.NoiseStart = noiseStart
		}
		if noiseEnd, ok := v["noise_end"].(float64); ok {
			req.NoiseEnd = noiseEnd
		}
		if noiseStep, ok := v["noise_step"].(float64); ok {
			req.NoiseStep = noiseStep
		}
		if closenessStart, ok := v["closeness_start"].(float64); ok {
			req.ClosenessStart = closenessStart
		}
		if closenessEnd, ok := v["closeness_end"].(float64); ok {
			req.ClosenessEnd = closenessEnd
		}
		if closenessStep, ok := v["closeness_step"].(float64); ok {
			req.ClosenessStep = closenessStep
		}
		if neighbourStart, ok := v["neighbour_start"].(float64); ok {
			req.NeighbourStart = int(neighbourStart)
		} else if neighbourStart, ok := v["neighbour_start"].(int); ok {
			req.NeighbourStart = neighbourStart
		}
		if neighbourEnd, ok := v["neighbour_end"].(float64); ok {
			req.NeighbourEnd = int(neighbourEnd)
		} else if neighbourEnd, ok := v["neighbour_end"].(int); ok {
			req.NeighbourEnd = neighbourEnd
		}
		if neighbourStep, ok := v["neighbour_step"].(float64); ok {
			req.NeighbourStep = int(neighbourStep)
		} else if neighbourStep, ok := v["neighbour_step"].(int); ok {
			req.NeighbourStep = neighbourStep
		}
		if fixedNoise, ok := v["fixed_noise"].(float64); ok {
			req.FixedNoise = fixedNoise
		}
		if fixedCloseness, ok := v["fixed_closeness"].(float64); ok {
			req.FixedCloseness = fixedCloseness
		}
		if fixedNeighbour, ok := v["fixed_neighbour"].(float64); ok {
			req.FixedNeighbour = int(fixedNeighbour)
		} else if fixedNeighbour, ok := v["fixed_neighbour"].(int); ok {
			req.FixedNeighbour = fixedNeighbour
		}
		if iterations, ok := v["iterations"].(float64); ok {
			req.Iterations = int(iterations)
		} else if iterations, ok := v["iterations"].(int); ok {
			req.Iterations = iterations
		}
		if interval, ok := v["interval"].(string); ok {
			req.Interval = interval
		}
		if settleTime, ok := v["settle_time"].(string); ok {
			req.SettleTime = settleTime
		}
		if seed, ok := v["seed"].(string); ok {
			req.Seed = seed
		}
	default:
		return fmt.Errorf("invalid request type: %T", reqInterface)
	}

	return r.start(ctx, req)
}

// start is the internal implementation of Start
func (r *Runner) start(ctx context.Context, req SweepRequest) error {
	// Parse durations with defaults
	interval := 2 * time.Second
	if req.Interval != "" {
		d, err := time.ParseDuration(req.Interval)
		if err != nil {
			return fmt.Errorf("invalid interval %q: %w", req.Interval, err)
		}
		interval = d
	}

	settleTime := 5 * time.Second
	if req.SettleTime != "" {
		d, err := time.ParseDuration(req.SettleTime)
		if err != nil {
			return fmt.Errorf("invalid settle_time %q: %w", req.SettleTime, err)
		}
		settleTime = d
	}

	if req.Iterations <= 0 {
		req.Iterations = 30
	}
	if req.Iterations > 500 {
		return fmt.Errorf("iterations must not exceed 500, got %d", req.Iterations)
	}
	if req.Mode == "" {
		req.Mode = "multi"
	}
	if req.Seed == "" {
		req.Seed = "true"
	}

	// Compute parameter combinations (doesn't need lock)
	noiseCombos, closenessCombos, neighbourCombos := r.computeCombinations(req)

	totalCombos := len(noiseCombos) * len(closenessCombos) * len(neighbourCombos)
	if totalCombos == 0 {
		return fmt.Errorf("no parameter combinations to sweep")
	}
	const maxCombos = 1000
	if totalCombos > maxCombos {
		return fmt.Errorf("parameter range too large: would generate %d combinations (max %d)", totalCombos, maxCombos)
	}

	// Now acquire lock for state modification
	r.mu.Lock()
	if r.state.Status == SweepStatusRunning {
		r.mu.Unlock()
		return fmt.Errorf("sweep already in progress")
	}

	now := time.Now()
	r.state = SweepState{
		Status:      SweepStatusRunning,
		StartedAt:   &now,
		TotalCombos: totalCombos,
		Results:     make([]ComboResult, 0, totalCombos),
		Request:     &req,
	}

	sweepCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Unlock()

	// Run sweep in background
	go r.run(sweepCtx, req, noiseCombos, closenessCombos, neighbourCombos, interval, settleTime)

	return nil
}

// Stop cancels a running sweep
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

// computeCombinations determines the parameter values based on the request mode
func (r *Runner) computeCombinations(req SweepRequest) ([]float64, []float64, []int) {
	var noiseCombos, closenessCombos []float64
	var neighbourCombos []int

	switch req.Mode {
	case "multi":
		noiseCombos = req.NoiseValues
		closenessCombos = req.ClosenessValues
		neighbourCombos = req.NeighbourValues

		if len(noiseCombos) == 0 {
			if req.NoiseStep > 0 {
				noiseCombos = GenerateRange(req.NoiseStart, req.NoiseEnd, req.NoiseStep)
			}
		}
		if len(closenessCombos) == 0 {
			if req.ClosenessStep > 0 {
				closenessCombos = GenerateRange(req.ClosenessStart, req.ClosenessEnd, req.ClosenessStep)
			}
		}
		if len(neighbourCombos) == 0 {
			if req.NeighbourStep > 0 {
				neighbourCombos = GenerateIntRange(req.NeighbourStart, req.NeighbourEnd, req.NeighbourStep)
			}
		}
	case "noise":
		noiseCombos = GenerateRange(req.NoiseStart, req.NoiseEnd, req.NoiseStep)
		closenessCombos = []float64{req.FixedCloseness}
		neighbourCombos = []int{req.FixedNeighbour}
	case "closeness":
		noiseCombos = []float64{req.FixedNoise}
		closenessCombos = GenerateRange(req.ClosenessStart, req.ClosenessEnd, req.ClosenessStep)
		neighbourCombos = []int{req.FixedNeighbour}
	case "neighbour":
		noiseCombos = []float64{req.FixedNoise}
		closenessCombos = []float64{req.FixedCloseness}
		neighbourCombos = GenerateIntRange(req.NeighbourStart, req.NeighbourEnd, req.NeighbourStep)
	}

	// Defaults if still empty
	if len(noiseCombos) == 0 {
		noiseCombos = []float64{0.005, 0.01, 0.02}
	}
	if len(closenessCombos) == 0 {
		closenessCombos = []float64{1.5, 2.0, 2.5}
	}
	if len(neighbourCombos) == 0 {
		neighbourCombos = []int{0, 1, 2}
	}

	return noiseCombos, closenessCombos, neighbourCombos
}

// run executes the sweep in a background goroutine
func (r *Runner) run(ctx context.Context, req SweepRequest, noiseCombos, closenessCombos []float64, neighbourCombos []int, interval, settleTime time.Duration) {
	buckets := r.client.FetchBuckets()
	sampler := NewSampler(r.client, buckets, interval)

	// Read total combos once to avoid race detector warnings
	r.mu.RLock()
	totalCombos := r.state.TotalCombos
	r.mu.RUnlock()

	comboNum := 0
	seedToggle := false

	for _, noise := range noiseCombos {
		for _, closeness := range closenessCombos {
			for _, neighbour := range neighbourCombos {
				// Check for cancellation
				select {
				case <-ctx.Done():
					r.mu.Lock()
					r.state.Status = SweepStatusError
					r.state.Error = "sweep cancelled"
					now := time.Now()
					r.state.CompletedAt = &now
					r.mu.Unlock()
					return
				default:
				}

				comboNum++
				log.Printf("[sweep] Combination %d/%d: noise=%.4f, closeness=%.2f, neighbour=%d",
					comboNum, totalCombos, noise, closeness, neighbour)

				// Determine seed
				var seed bool
				switch req.Seed {
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
				if err := r.client.ResetGrid(); err != nil {
					log.Printf("[sweep] WARNING: Grid reset failed: %v", err)
				}

				// Set parameters
				params := monitor.BackgroundParams{
					NoiseRelative:              noise,
					ClosenessMultiplier:        closeness,
					NeighbourConfirmationCount: neighbour,
					SeedFromFirstFrame:         seed,
				}
				if err := r.client.SetParams(params); err != nil {
					log.Printf("[sweep] ERROR: Failed to set params: %v", err)
					continue
				}

				// Reset acceptance
				if err := r.client.ResetAcceptance(); err != nil {
					log.Printf("[sweep] WARNING: Failed to reset acceptance: %v", err)
				}

				// Wait for settle
				r.client.WaitForGridSettle(settleTime)

				// Sample
				cfg := SampleConfig{
					Noise:      noise,
					Closeness:  closeness,
					Neighbour:  neighbour,
					Iterations: req.Iterations,
				}
				results := sampler.Sample(cfg)

				// Compute summary
				combo := r.computeComboResult(noise, closeness, neighbour, results, buckets)

				// Update state
				r.mu.Lock()
				r.state.Results = append(r.state.Results, combo)
				r.state.CompletedCombos = comboNum
				r.state.CurrentCombo = &combo
				r.mu.Unlock()
			}
		}
	}

	r.mu.Lock()
	r.state.Status = SweepStatusComplete
	now := time.Now()
	r.state.CompletedAt = &now
	r.mu.Unlock()
	log.Printf("[sweep] Sweep complete: %d combinations evaluated", comboNum)
}

// computeComboResult computes summary statistics for a parameter combination
func (r *Runner) computeComboResult(noise, closeness float64, neighbour int, results []SampleResult, buckets []string) ComboResult {
	combo := ComboResult{
		Noise:     noise,
		Closeness: closeness,
		Neighbour: neighbour,
		Buckets:   buckets,
	}

	if len(results) == 0 {
		return combo
	}

	// Per-bucket means and stddevs
	combo.BucketMeans = make([]float64, len(buckets))
	combo.BucketStddevs = make([]float64, len(buckets))
	for bi := range buckets {
		vals := make([]float64, len(results))
		for ri, r := range results {
			if bi < len(r.AcceptanceRates) {
				vals[ri] = r.AcceptanceRates[bi]
			}
		}
		combo.BucketMeans[bi], combo.BucketStddevs[bi] = MeanStddev(vals)
	}

	// Overall acceptance
	overallVals := make([]float64, len(results))
	for ri, r := range results {
		overallVals[ri] = r.OverallAcceptPct
	}
	combo.OverallAcceptMean, combo.OverallAcceptStddev = MeanStddev(overallVals)

	// Nonzero cells
	nzVals := make([]float64, len(results))
	for ri, r := range results {
		nzVals[ri] = r.NonzeroCells
	}
	combo.NonzeroCellsMean, combo.NonzeroCellsStddev = MeanStddev(nzVals)

	return combo
}
