package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// SweepParam defines one parameter dimension to sweep.
type SweepParam struct {
	Name   string        `json:"name"`             // JSON key from /api/lidar/params e.g. "noise_relative"
	Type   string        `json:"type"`             // "float64", "int", "int64", "bool", "string"
	Values []interface{} `json:"values,omitempty"` // explicit values (parsed from start/end/step or comma list)

	// Range fields (for numeric types, dashboard sends these; runner generates Values)
	Start float64 `json:"start,omitempty"`
	End   float64 `json:"end,omitempty"`
	Step  float64 `json:"step,omitempty"`
}

// SweepRequest defines the parameters for starting a sweep
type SweepRequest struct {
	Mode string `json:"mode"` // "multi", "noise", "closeness", "neighbour", "params"

	// Generic param list (new)
	Params []SweepParam `json:"params,omitempty"`

	// Data source
	DataSource       string  `json:"data_source,omitempty"`        // "live" (default) or "pcap"
	PCAPFile         string  `json:"pcap_file,omitempty"`          // filename (basename only)
	PCAPStartSecs    float64 `json:"pcap_start_secs,omitempty"`    // start offset in seconds
	PCAPDurationSecs float64 `json:"pcap_duration_secs,omitempty"` // duration in seconds (-1 = full)

	// Multi-mode: explicit values (legacy)
	NoiseValues     []float64 `json:"noise_values,omitempty"`
	ClosenessValues []float64 `json:"closeness_values,omitempty"`
	NeighbourValues []int     `json:"neighbour_values,omitempty"`

	// Single-variable sweep ranges (legacy)
	NoiseStart     float64 `json:"noise_start,omitempty"`
	NoiseEnd       float64 `json:"noise_end,omitempty"`
	NoiseStep      float64 `json:"noise_step,omitempty"`
	ClosenessStart float64 `json:"closeness_start,omitempty"`
	ClosenessEnd   float64 `json:"closeness_end,omitempty"`
	ClosenessStep  float64 `json:"closeness_step,omitempty"`
	NeighbourStart int     `json:"neighbour_start,omitempty"`
	NeighbourEnd   int     `json:"neighbour_end,omitempty"`
	NeighbourStep  int     `json:"neighbour_step,omitempty"`

	// Fixed values for single-variable sweeps (legacy)
	FixedNoise     float64 `json:"fixed_noise,omitempty"`
	FixedCloseness float64 `json:"fixed_closeness,omitempty"`
	FixedNeighbour int     `json:"fixed_neighbour,omitempty"`

	// Sampling
	Iterations int    `json:"iterations"`  // samples per combo
	Interval   string `json:"interval"`    // duration string e.g. "2s"
	SettleTime string `json:"settle_time"` // duration string e.g. "5s"

	// Settle mode: "per_combo" (default) = full grid+region settle each combination;
	// "once" = first combo does full settle, subsequent combos restore regions from store (~10 frames).
	SettleMode string `json:"settle_mode,omitempty"`

	// Seed control
	Seed string `json:"seed"` // "true", "false", or "toggle"
}

// ComboResult holds the summary result for one parameter combination
type ComboResult struct {
	// Generic param values (new)
	ParamValues map[string]interface{} `json:"param_values,omitempty"`

	// Legacy fields (populated from ParamValues for backward compat)
	Noise     float64 `json:"noise"`
	Closeness float64 `json:"closeness"`
	Neighbour int     `json:"neighbour"`

	OverallAcceptMean   float64   `json:"overall_accept_mean"`
	OverallAcceptStddev float64   `json:"overall_accept_stddev"`
	NonzeroCellsMean    float64   `json:"nonzero_cells_mean"`
	NonzeroCellsStddev  float64   `json:"nonzero_cells_stddev"`
	BucketMeans         []float64 `json:"bucket_means"`
	BucketStddevs       []float64 `json:"bucket_stddevs"`
	Buckets             []string  `json:"buckets"`

	// Track health metrics
	ActiveTracksMean        float64 `json:"active_tracks_mean"`
	ActiveTracksStddev      float64 `json:"active_tracks_stddev"`
	AlignmentDegMean        float64 `json:"alignment_deg_mean"`
	AlignmentDegStddev      float64 `json:"alignment_deg_stddev"`
	MisalignmentRatioMean   float64 `json:"misalignment_ratio_mean"`
	MisalignmentRatioStddev float64 `json:"misalignment_ratio_stddev"`
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
	Warnings        []string      `json:"warnings,omitempty"`
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

// addWarning appends a warning message to the sweep state.
func (r *Runner) addWarning(msg string) {
	r.mu.Lock()
	r.state.Warnings = append(r.state.Warnings, msg)
	r.mu.Unlock()
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
	var req SweepRequest

	switch v := reqInterface.(type) {
	case SweepRequest:
		req = v
	case map[string]interface{}:
		// Re-marshal via JSON so the mapping stays consistent with the
		// SweepRequest struct tags and validation is centralised.
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshalling sweep request: %w", err)
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return fmt.Errorf("parsing sweep request: %w", err)
		}
	default:
		return fmt.Errorf("invalid request type: %T", reqInterface)
	}

	return r.start(ctx, req)
}

// start is the internal implementation of Start
func (r *Runner) start(ctx context.Context, req SweepRequest) error {
	// Guard nil context to prevent panic in context.WithCancel
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate client is configured
	if r.client == nil {
		return fmt.Errorf("sweep runner has no client configured")
	}

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

	// Validate mode
	switch req.Mode {
	case "multi", "noise", "closeness", "neighbour", "params":
		// supported modes
	default:
		return fmt.Errorf("unsupported sweep mode %q", req.Mode)
	}

	// Use generic params path if params are provided
	if len(req.Params) > 0 {
		return r.startGeneric(ctx, req, interval, settleTime)
	}

	// Legacy path: 3 fixed parameter dimensions
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

// startGeneric handles the generic N-dimensional parameter sweep.
func (r *Runner) startGeneric(ctx context.Context, req SweepRequest, interval, settleTime time.Duration) error {
	// Expand SweepParam values from ranges
	for i := range req.Params {
		if err := expandSweepParam(&req.Params[i]); err != nil {
			return fmt.Errorf("param %q: %w", req.Params[i].Name, err)
		}
		if len(req.Params[i].Values) == 0 {
			return fmt.Errorf("param %q has no values", req.Params[i].Name)
		}
	}

	// Compute Cartesian product
	combos := cartesianProduct(req.Params)
	totalCombos := len(combos)

	if totalCombos == 0 {
		return fmt.Errorf("no parameter combinations to sweep")
	}
	const maxCombos = 1000
	if totalCombos > maxCombos {
		return fmt.Errorf("parameter range too large: would generate %d combinations (max %d)", totalCombos, maxCombos)
	}

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

	go r.runGeneric(sweepCtx, req, combos, interval, settleTime)

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

// computeCombinations determines the parameter values based on the request mode (legacy)
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

// run executes the legacy sweep in a background goroutine
func (r *Runner) run(ctx context.Context, req SweepRequest, noiseCombos, closenessCombos []float64, neighbourCombos []int, interval, settleTime time.Duration) {
	isPCAP := req.DataSource == "pcap" && req.PCAPFile != ""
	settleOnce := req.SettleMode == "once"
	const regionRestoreWait = 2 * time.Second

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
					r.state.Error = fmt.Sprintf("sweep stopped at combination %d/%d: %v", comboNum, totalCombos, ctx.Err())
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

				// Set parameters FIRST (before reset, so new config is active)
				params := monitor.BackgroundParams{
					NoiseRelative:              noise,
					ClosenessMultiplier:        closeness,
					NeighbourConfirmationCount: neighbour,
					SeedFromFirst:              seed,
				}
				if err := r.client.SetParams(params); err != nil {
					log.Printf("[sweep] ERROR: Failed to set params: %v", err)
					r.mu.Lock()
					r.state.Status = SweepStatusError
					r.state.Error = fmt.Sprintf("combo %d: failed to set params: %v", comboNum+1, err)
					r.mu.Unlock()
					return
				}

				if isPCAP {
					// PCAP mode: replay per-combination with analysis_mode so grid is preserved after completion.
					if err := r.client.StartPCAPReplayWithConfig(monitor.PCAPReplayConfig{
						PCAPFile:        req.PCAPFile,
						StartSeconds:    req.PCAPStartSecs,
						DurationSeconds: req.PCAPDurationSecs,
						MaxRetries:      30,
						AnalysisMode:    true,
					}); err != nil {
						log.Printf("[sweep] ERROR: Failed to start PCAP for combo %d: %v", comboNum, err)
						r.addWarning(fmt.Sprintf("combo %d: failed to start PCAP (skipped): %v", comboNum, err))
						continue
					}

					// Wait for PCAP replay to finish so all data is processed
					if err := r.client.WaitForPCAPComplete(120 * time.Second); err != nil {
						log.Printf("[sweep] WARNING: PCAP wait timeout for combo %d: %v", comboNum, err)
						r.addWarning(fmt.Sprintf("combo %d: PCAP wait timeout: %v", comboNum, err))
					}

					// Settle after PCAP completion: full settle for first combo, short wait for subsequent in "once" mode
					if settleOnce && comboNum > 1 {
						time.Sleep(regionRestoreWait)
					} else if settleTime > 0 {
						time.Sleep(settleTime)
					}
				} else {
					// Live mode: reset grid and acceptance, then wait for data
					if err := r.client.ResetGrid(); err != nil {
						log.Printf("[sweep] WARNING: Grid reset failed: %v", err)
						r.addWarning(fmt.Sprintf("combo %d: grid reset failed: %v", comboNum+1, err))
					}

					if err := r.client.ResetAcceptance(); err != nil {
						log.Printf("[sweep] WARNING: Failed to reset acceptance: %v", err)
						r.addWarning(fmt.Sprintf("combo %d: reset acceptance failed: %v", comboNum+1, err))
					}

					// Settle: full settle for first combo, short wait for subsequent in "once" mode
					if settleOnce && comboNum > 1 {
						r.client.WaitForGridSettle(regionRestoreWait)
					} else {
						r.client.WaitForGridSettle(settleTime)
					}
				}

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

	// Clean up: stop any lingering PCAP replay
	if isPCAP {
		if err := r.client.StopPCAPReplay(); err != nil {
			log.Printf("[sweep] WARNING: Failed to stop PCAP: %v", err)
		}
	}

	r.mu.Lock()
	r.state.Status = SweepStatusComplete
	now := time.Now()
	r.state.CompletedAt = &now
	r.mu.Unlock()
	log.Printf("[sweep] Sweep complete: %d combinations evaluated", comboNum)
}

// runGeneric executes the generic N-dimensional sweep.
func (r *Runner) runGeneric(ctx context.Context, req SweepRequest, combos []map[string]interface{}, interval, settleTime time.Duration) {
	isPCAP := req.DataSource == "pcap" && req.PCAPFile != ""
	settleOnce := req.SettleMode == "once"
	const regionRestoreWait = 2 * time.Second

	buckets := r.client.FetchBuckets()
	sampler := NewSampler(r.client, buckets, interval)

	r.mu.RLock()
	totalCombos := r.state.TotalCombos
	r.mu.RUnlock()

	seedToggle := false

	for comboNum, paramValues := range combos {
		// Check for cancellation
		select {
		case <-ctx.Done():
			r.mu.Lock()
			r.state.Status = SweepStatusError
			r.state.Error = fmt.Sprintf("sweep stopped at combination %d/%d: %v", comboNum+1, totalCombos, ctx.Err())
			now := time.Now()
			r.state.CompletedAt = &now
			r.mu.Unlock()
			return
		default:
		}

		log.Printf("[sweep] Combination %d/%d: %v", comboNum+1, totalCombos, paramValues)

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

		// Build tuning params map, include seed
		tuningParams := make(map[string]interface{}, len(paramValues)+1)
		for k, v := range paramValues {
			tuningParams[k] = v
		}
		// Always include seed_from_first unless the sweep is explicitly sweeping it
		if _, hasSeed := tuningParams["seed_from_first"]; !hasSeed {
			tuningParams["seed_from_first"] = seed
		}

		// Set parameters FIRST (before reset, so new config is active)
		if err := r.client.SetTuningParams(tuningParams); err != nil {
			log.Printf("[sweep] ERROR: Failed to set params: %v", err)
			r.addWarning(fmt.Sprintf("combo %d: failed to set params (skipped): %v", comboNum+1, err))
			continue
		}

		if isPCAP {
			// PCAP mode: replay per-combination with analysis_mode so grid is preserved after completion.
			// Starting PCAP internally resets all state (grid, frame builder), so no separate reset needed.
			if err := r.client.StartPCAPReplayWithConfig(monitor.PCAPReplayConfig{
				PCAPFile:        req.PCAPFile,
				StartSeconds:    req.PCAPStartSecs,
				DurationSeconds: req.PCAPDurationSecs,
				MaxRetries:      30,
				AnalysisMode:    true,
			}); err != nil {
				log.Printf("[sweep] ERROR: Failed to start PCAP for combo %d: %v", comboNum+1, err)
				r.addWarning(fmt.Sprintf("combo %d: failed to start PCAP (skipped): %v", comboNum+1, err))
				continue
			}

			// Wait for PCAP replay to finish so all data is processed
			if err := r.client.WaitForPCAPComplete(120 * time.Second); err != nil {
				log.Printf("[sweep] WARNING: PCAP wait timeout for combo %d: %v", comboNum+1, err)
				r.addWarning(fmt.Sprintf("combo %d: PCAP wait timeout: %v", comboNum+1, err))
			}

			// Settle after PCAP completion: full settle for first combo, short wait for subsequent in "once" mode
			if settleOnce && comboNum > 0 {
				time.Sleep(regionRestoreWait)
			} else if settleTime > 0 {
				time.Sleep(settleTime)
			}
		} else {
			// Live mode: reset grid and acceptance, then wait for data
			if err := r.client.ResetGrid(); err != nil {
				log.Printf("[sweep] WARNING: Grid reset failed: %v", err)
				r.addWarning(fmt.Sprintf("combo %d: grid reset failed: %v", comboNum+1, err))
			}

			if err := r.client.ResetAcceptance(); err != nil {
				log.Printf("[sweep] WARNING: Failed to reset acceptance: %v", err)
				r.addWarning(fmt.Sprintf("combo %d: reset acceptance failed: %v", comboNum+1, err))
			}

			// Settle: full settle for first combo, short wait for subsequent in "once" mode
			if settleOnce && comboNum > 0 {
				r.client.WaitForGridSettle(regionRestoreWait)
			} else {
				r.client.WaitForGridSettle(settleTime)
			}
		}

		// Extract legacy values for SampleConfig if present
		noise, _ := toFloat64(paramValues["noise_relative"])
		closeness, _ := toFloat64(paramValues["closeness_multiplier"])
		neighbour, _ := toInt(paramValues["neighbor_confirmation_count"])

		// Sample
		cfg := SampleConfig{
			Noise:      noise,
			Closeness:  closeness,
			Neighbour:  neighbour,
			Iterations: req.Iterations,
		}
		results := sampler.Sample(cfg)

		// Compute summary with generic param values
		combo := r.computeComboResult(noise, closeness, neighbour, results, buckets)
		combo.ParamValues = paramValues

		// Update state
		r.mu.Lock()
		r.state.Results = append(r.state.Results, combo)
		r.state.CompletedCombos = comboNum + 1
		r.state.CurrentCombo = &combo
		r.mu.Unlock()
	}

	// Clean up: stop any lingering PCAP replay
	if isPCAP {
		if err := r.client.StopPCAPReplay(); err != nil {
			log.Printf("[sweep] WARNING: Failed to stop PCAP: %v", err)
		}
	}

	r.mu.Lock()
	r.state.Status = SweepStatusComplete
	now := time.Now()
	r.state.CompletedAt = &now
	r.mu.Unlock()
	log.Printf("[sweep] Sweep complete: %d combinations evaluated", len(combos))
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

	// Track health: active tracks
	atVals := make([]float64, len(results))
	for ri, r := range results {
		atVals[ri] = float64(r.ActiveTracks)
	}
	combo.ActiveTracksMean, combo.ActiveTracksStddev = MeanStddev(atVals)

	// Track health: alignment
	alignVals := make([]float64, len(results))
	for ri, r := range results {
		alignVals[ri] = r.MeanAlignmentDeg
	}
	combo.AlignmentDegMean, combo.AlignmentDegStddev = MeanStddev(alignVals)

	// Track health: misalignment ratio
	misVals := make([]float64, len(results))
	for ri, r := range results {
		misVals[ri] = r.MisalignmentRatio
	}
	combo.MisalignmentRatioMean, combo.MisalignmentRatioStddev = MeanStddev(misVals)

	return combo
}

// expandSweepParam expands a SweepParam's range fields into Values.
func expandSweepParam(sp *SweepParam) error {
	if len(sp.Values) > 0 {
		// Already have explicit values — type-coerce them
		for i, v := range sp.Values {
			coerced, err := coerceValue(v, sp.Type)
			if err != nil {
				return fmt.Errorf("value[%d]: %w", i, err)
			}
			sp.Values[i] = coerced
		}
		return nil
	}

	// Generate values from Start/End/Step
	switch sp.Type {
	case "float64":
		if sp.Step <= 0 {
			return fmt.Errorf("step must be positive for float64 range")
		}
		for _, v := range GenerateRange(sp.Start, sp.End, sp.Step) {
			sp.Values = append(sp.Values, v)
		}
	case "int":
		if sp.Step <= 0 {
			return fmt.Errorf("step must be positive for int range")
		}
		for _, v := range GenerateIntRange(int(sp.Start), int(sp.End), int(sp.Step)) {
			sp.Values = append(sp.Values, v)
		}
	case "int64":
		if sp.Step <= 0 {
			return fmt.Errorf("step must be positive for int64 range")
		}
		for v := int64(sp.Start); v <= int64(sp.End); v += int64(sp.Step) {
			sp.Values = append(sp.Values, v)
		}
	case "bool":
		sp.Values = []interface{}{true, false}
	case "string":
		// No range generation for strings; values must be explicit
		return fmt.Errorf("string params require explicit values")
	default:
		return fmt.Errorf("unknown type %q", sp.Type)
	}
	return nil
}

// coerceValue converts a value to the appropriate Go type for the given param type.
// Returns an error if the conversion fails instead of silently defaulting to zero.
func coerceValue(v interface{}, typ string) (interface{}, error) {
	switch typ {
	case "float64":
		switch val := v.(type) {
		case float64:
			return val, nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot parse %q as float64: %w", val, err)
			}
			return f, nil
		case bool:
			if val {
				return 1.0, nil
			}
			return 0.0, nil
		}
	case "int":
		switch val := v.(type) {
		case float64:
			return int(val), nil
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, fmt.Errorf("cannot parse %q as int: %w", val, err)
			}
			return n, nil
		}
	case "int64":
		switch val := v.(type) {
		case float64:
			return int64(val), nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot parse %q as int64: %w", val, err)
			}
			return n, nil
		}
	case "bool":
		switch val := v.(type) {
		case bool:
			return val, nil
		case string:
			return strings.TrimSpace(strings.ToLower(val)) == "true", nil
		case float64:
			return val != 0, nil
		}
	case "string":
		switch val := v.(type) {
		case string:
			return strings.TrimSpace(val), nil
		default:
			return fmt.Sprintf("%v", val), nil
		}
	}
	return nil, fmt.Errorf("unsupported coercion: %T to %s", v, typ)
}

// cartesianProduct computes the Cartesian product of all SweepParam value lists.
// Returns a slice of maps, where each map represents one parameter combination.
func cartesianProduct(params []SweepParam) []map[string]interface{} {
	if len(params) == 0 {
		return nil
	}

	total := 1
	for _, p := range params {
		total *= len(p.Values)
	}

	combos := make([]map[string]interface{}, total)
	for i := range combos {
		combos[i] = make(map[string]interface{}, len(params))
	}

	repeat := 1
	for dim := len(params) - 1; dim >= 0; dim-- {
		vals := params[dim].Values
		name := params[dim].Name
		cycle := len(vals)
		for i := 0; i < total; i++ {
			combos[i][name] = vals[(i/repeat)%cycle]
		}
		repeat *= cycle
	}

	return combos
}

// toFloat64 converts an interface{} to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

// toInt converts an interface{} to int.
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case int64:
		return int(val), true
	}
	return 0, false
}
