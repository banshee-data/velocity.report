package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SweepStatus represents the current state of a sweep run
type SweepStatus string

const (
	SweepStatusIdle     SweepStatus = "idle"
	SweepStatusRunning  SweepStatus = "running"
	SweepStatusComplete SweepStatus = "complete"
	SweepStatusError    SweepStatus = "error"

	// ObjectiveVersion is the current version of the objective/scoring system
	ObjectiveVersion = "v1"
)

// ErrSweepAlreadyRunning is returned when a sweep is already in progress.
var ErrSweepAlreadyRunning = fmt.Errorf("sweep already in progress")

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

	// EnableRecording enables VRLOG recording during PCAP replays.
	// Only RLHF tuning runs and manual replays should set this to true;
	// regular multi-combo sweeps leave it false to avoid generating a
	// VRLOG file per combination.
	EnableRecording bool `json:"enable_recording,omitempty"`
}

// ComboResult holds the summary result for one parameter combination
type ComboResult struct {
	RunID string `json:"run_id,omitempty"` // Analysis run ID (when ground truth mode is active)

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
	HeadingJitterDegMean    float64 `json:"heading_jitter_deg_mean"`
	HeadingJitterDegStddev  float64 `json:"heading_jitter_deg_stddev"`
	SpeedJitterMpsMean      float64 `json:"speed_jitter_mps_mean"`
	SpeedJitterMpsStddev    float64 `json:"speed_jitter_mps_stddev"`
	FragmentationRatioMean  float64 `json:"fragmentation_ratio_mean"`

	// Scene-level foreground capture metrics
	ForegroundCaptureMean   float64 `json:"foreground_capture_mean"`
	ForegroundCaptureStddev float64 `json:"foreground_capture_stddev"`
	UnboundedPointMean      float64 `json:"unbounded_point_mean"`
	UnboundedPointStddev    float64 `json:"unbounded_point_stddev"`
	EmptyBoxRatioMean       float64 `json:"empty_box_ratio_mean"`
	EmptyBoxRatioStddev     float64 `json:"empty_box_ratio_stddev"`
}

// AnalysisRunCreator creates analysis runs for sweep combinations.
// Defined as an interface to avoid circular imports with the lidar package.
type AnalysisRunCreator interface {
	CreateSweepRun(sensorID, pcapFile string, paramsJSON json.RawMessage) (string, error)
}

// SweepPersister persists sweep results to a database.
// Defined as an interface to avoid circular imports with the lidar package.
type SweepPersister interface {
	SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time, objectiveName, objectiveVersion string) error
	SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error
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
	backend   SweepBackend
	mu        sync.RWMutex
	state     SweepState
	cancel    context.CancelFunc
	persister SweepPersister
	sweepID   string // current sweep's unique ID
	logger    *log.Logger
}

// NewRunner creates a new sweep runner with the given backend.
func NewRunner(backend SweepBackend) *Runner {
	return &Runner{
		backend: backend,
		logger:  log.Default(),
		state:   SweepState{Status: SweepStatusIdle},
	}
}

// SetLogger sets the logger for the Runner. Use log.New(io.Discard, "", 0)
// in tests to suppress expected error-path log output.
func (r *Runner) SetLogger(l *log.Logger) {
	r.logger = l
}

// discardRunnerLogger returns a logger that discards all output.
func discardRunnerLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// SetPersister sets the database persister for saving sweep results.
func (r *Runner) SetPersister(p SweepPersister) {
	r.persister = p
}

// GetSweepID returns the current sweep's unique ID.
func (r *Runner) GetSweepID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sweepID
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

	// Validate backend is configured
	if r.backend == nil {
		return fmt.Errorf("sweep runner has no backend configured")
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
		return ErrSweepAlreadyRunning
	}

	now := time.Now()
	r.sweepID = uuid.New().String()
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

	// Persist sweep start to database
	if r.persister != nil {
		reqJSON, err := json.Marshal(req)
		if err != nil {
			r.logger.Printf("[sweep] WARNING: Failed to marshal sweep request for persistence: %v", err)
			reqJSON = []byte("{}")
		}
		if err := r.persister.SaveSweepStart(r.sweepID, r.backend.SensorID(), "sweep", reqJSON, now, "manual", ObjectiveVersion); err != nil {
			r.logger.Printf("[sweep] WARNING: Failed to persist sweep start: %v", err)
		}
	}

	// Run sweep in background
	go r.run(sweepCtx, req, noiseCombos, closenessCombos, neighbourCombos, interval, settleTime)

	return nil
}

// startGeneric handles the generic N-dimensional parameter sweep.
func (r *Runner) startGeneric(ctx context.Context, req SweepRequest, interval, settleTime time.Duration) error {
	// Limit maximum number of parameters to prevent excessive memory allocation during Cartesian product
	const maxParams = 10
	if len(req.Params) > maxParams {
		return fmt.Errorf("too many parameters: maximum %d allowed, got %d", maxParams, len(req.Params))
	}

	// Expand SweepParam values from ranges
	for i := range req.Params {
		if err := expandSweepParam(&req.Params[i]); err != nil {
			return fmt.Errorf("param %q: %w", req.Params[i].Name, err)
		}
		if len(req.Params[i].Values) == 0 {
			return fmt.Errorf("param %q has no values", req.Params[i].Name)
		}
	}

	// Compute Cartesian product - validates size before allocation to prevent DoS
	combos, err := cartesianProduct(req.Params)
	if err != nil {
		return err
	}
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
		return ErrSweepAlreadyRunning
	}

	now := time.Now()
	r.sweepID = uuid.New().String()
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

	// Persist sweep start to database
	if r.persister != nil {
		reqJSON, err := json.Marshal(req)
		if err != nil {
			r.logger.Printf("[sweep] WARNING: Failed to marshal sweep request for persistence: %v", err)
			reqJSON = []byte("{}")
		}
		if err := r.persister.SaveSweepStart(r.sweepID, r.backend.SensorID(), "sweep", reqJSON, now, "manual", ObjectiveVersion); err != nil {
			r.logger.Printf("[sweep] WARNING: Failed to persist sweep start: %v", err)
		}
	}

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

	buckets := r.backend.FetchBuckets()
	sampler := NewSampler(r.backend, buckets, interval)

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
					errMsg := fmt.Sprintf("sweep stopped at combination %d/%d: %v", comboNum, totalCombos, ctx.Err())
					r.state.Error = errMsg
					now := time.Now()
					r.state.CompletedAt = &now
					r.mu.Unlock()
					r.persistComplete("error", errMsg, nil)
					return
				default:
				}

				comboNum++
				r.logger.Printf("[sweep] Combination %d/%d: noise=%.4f, closeness=%.2f, neighbour=%d",
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
				tuningParams := map[string]interface{}{
					"noise_relative":              noise,
					"closeness_multiplier":        closeness,
					"neighbor_confirmation_count": neighbour,
					"seed_from_first":             seed,
				}
				if err := r.backend.SetTuningParams(tuningParams); err != nil {
					r.logger.Printf("[sweep] ERROR: Failed to set params: %v", err)
					errMsg := fmt.Sprintf("combo %d: failed to set params: %v", comboNum, err)
					r.mu.Lock()
					r.state.Status = SweepStatusError
					r.state.Error = errMsg
					r.mu.Unlock()
					r.persistComplete("error", errMsg, nil)
					return
				}

				if isPCAP {
					// Reset acceptance counters before each PCAP combination so metrics
					// reflect only this combination's data (mirrors live mode at line 526).
					if err := r.backend.ResetAcceptance(); err != nil {
						r.logger.Printf("[sweep] WARNING: Failed to reset acceptance before PCAP: %v", err)
						r.addWarning(fmt.Sprintf("combo %d: reset acceptance failed: %v", comboNum+1, err))
					}

					// PCAP mode: replay per-combination with analysis_mode so grid is preserved after completion.
					// Use "realtime" speed to ensure the full tracking pipeline (BackgroundManager,
					// ForegroundForwarder, warmup) runs — "fastest" mode skips foreground extraction
					// and produces 0 tracks.
					if err := r.backend.StartPCAPReplayWithConfig(PCAPReplayConfig{
						PCAPFile:         req.PCAPFile,
						StartSeconds:     req.PCAPStartSecs,
						DurationSeconds:  req.PCAPDurationSecs,
						MaxRetries:       30,
						AnalysisMode:     true,
						SpeedMode:        "realtime",
						DisableRecording: !req.EnableRecording,
					}); err != nil {
						r.logger.Printf("[sweep] ERROR: Failed to start PCAP for combo %d: %v", comboNum, err)
						r.addWarning(fmt.Sprintf("combo %d: failed to start PCAP (skipped): %v", comboNum, err))
						continue
					}

					// Wait for PCAP replay to finish so all data is processed
					if err := r.backend.WaitForPCAPComplete(120 * time.Second); err != nil {
						r.logger.Printf("[sweep] WARNING: PCAP wait timeout for combo %d: %v", comboNum, err)
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
					if err := r.backend.ResetGrid(); err != nil {
						r.logger.Printf("[sweep] WARNING: Grid reset failed: %v", err)
						r.addWarning(fmt.Sprintf("combo %d: grid reset failed: %v", comboNum+1, err))
					}

					if err := r.backend.ResetAcceptance(); err != nil {
						r.logger.Printf("[sweep] WARNING: Failed to reset acceptance: %v", err)
						r.addWarning(fmt.Sprintf("combo %d: reset acceptance failed: %v", comboNum+1, err))
					}

					// Settle: full settle for first combo, short wait for subsequent in "once" mode
					if settleOnce && comboNum > 1 {
						r.backend.WaitForGridSettle(regionRestoreWait)
					} else {
						r.backend.WaitForGridSettle(settleTime)
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

				// Capture analysis run ID from the server (set during PCAP replay)
				if isPCAP {
					combo.RunID = r.backend.GetLastAnalysisRunID()
				}

				// Update state
				r.mu.Lock()
				r.state.Results = append(r.state.Results, combo)
				r.state.CompletedCombos = comboNum
				r.state.CurrentCombo = &combo
				r.mu.Unlock()

				// Release PCAP slot after each combo so the next combo can start
				// a fresh replay. Without this, currentSource stays DataSourcePCAP
				// and subsequent StartPCAPForSweep calls spin on the conflict retry
				// loop until they time out.
				if isPCAP {
					if err := r.backend.StopPCAPReplay(); err != nil {
						r.logger.Printf("[sweep] WARNING: Failed to stop PCAP after combo %d: %v", comboNum, err)
					}
				}
			}
		}
	}

	// Clean up: stop any lingering PCAP replay (covers early exits / errors)
	if isPCAP {
		if err := r.backend.StopPCAPReplay(); err != nil {
			r.logger.Printf("[sweep] WARNING: Failed to stop PCAP: %v", err)
		}
	}

	r.mu.Lock()
	r.state.Status = SweepStatusComplete
	now := time.Now()
	r.state.CompletedAt = &now
	r.mu.Unlock()
	r.logger.Printf("[sweep] Sweep complete: %d combinations evaluated", comboNum)

	// Persist completion to database
	r.persistComplete("complete", "", nil)
}

// runGeneric executes the generic N-dimensional sweep.
func (r *Runner) runGeneric(ctx context.Context, req SweepRequest, combos []map[string]interface{}, interval, settleTime time.Duration) {
	isPCAP := req.DataSource == "pcap" && req.PCAPFile != ""
	settleOnce := req.SettleMode == "once"
	const regionRestoreWait = 2 * time.Second
	// Maximum number of tuning parameters to prevent overflow in map allocation (CWE-770)
	const maxTuningParams = 50

	buckets := r.backend.FetchBuckets()
	sampler := NewSampler(r.backend, buckets, interval)

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
			errMsg := fmt.Sprintf("sweep stopped at combination %d/%d: %v", comboNum+1, totalCombos, ctx.Err())
			r.state.Error = errMsg
			now := time.Now()
			r.state.CompletedAt = &now
			r.mu.Unlock()
			r.persistComplete("error", errMsg, nil)
			return
		default:
		}

		r.logger.Printf("[sweep] Combination %d/%d: %v", comboNum+1, totalCombos, paramValues)

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
		// Validate parameter count to prevent overflow (CWE-770)
		if len(paramValues) >= maxTuningParams {
			r.logger.Printf("[sweep] WARNING: Parameter count %d exceeds maximum %d, clamping", len(paramValues), maxTuningParams-1)
			r.addWarning(fmt.Sprintf("combo %d: parameter count %d exceeds maximum %d (skipped)", comboNum+1, len(paramValues), maxTuningParams-1))
			continue
		}
		// Allocate with compile-time constant to satisfy static analysis
		tuningParams := make(map[string]interface{}, maxTuningParams)
		for k, v := range paramValues {
			tuningParams[k] = v
		}
		// Always include seed_from_first unless the sweep is explicitly sweeping it
		if _, hasSeed := tuningParams["seed_from_first"]; !hasSeed {
			tuningParams["seed_from_first"] = seed
		}

		// Set parameters FIRST (before reset, so new config is active)
		if err := r.backend.SetTuningParams(tuningParams); err != nil {
			r.logger.Printf("[sweep] ERROR: Failed to set params: %v", err)
			r.addWarning(fmt.Sprintf("combo %d: failed to set params (skipped): %v", comboNum+1, err))
			continue
		}

		if isPCAP {
			// PCAP mode: replay per-combination with analysis_mode so grid is preserved after completion.
			// Starting PCAP internally resets all state (grid, frame builder), so no separate reset needed.
			// Use "realtime" speed to ensure the full tracking pipeline (BackgroundManager,
			// ForegroundForwarder, warmup) runs — "fastest" mode skips foreground extraction
			// and produces 0 tracks.
			if err := r.backend.StartPCAPReplayWithConfig(PCAPReplayConfig{
				PCAPFile:         req.PCAPFile,
				StartSeconds:     req.PCAPStartSecs,
				DurationSeconds:  req.PCAPDurationSecs,
				MaxRetries:       30,
				AnalysisMode:     true,
				SpeedMode:        "realtime",
				DisableRecording: !req.EnableRecording,
			}); err != nil {
				r.logger.Printf("[sweep] ERROR: Failed to start PCAP for combo %d: %v", comboNum+1, err)
				r.addWarning(fmt.Sprintf("combo %d: failed to start PCAP (skipped): %v", comboNum+1, err))
				continue
			}

			// Wait for PCAP replay to finish so all data is processed
			if err := r.backend.WaitForPCAPComplete(120 * time.Second); err != nil {
				r.logger.Printf("[sweep] WARNING: PCAP wait timeout for combo %d: %v", comboNum+1, err)
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
			if err := r.backend.ResetGrid(); err != nil {
				r.logger.Printf("[sweep] WARNING: Grid reset failed: %v", err)
				r.addWarning(fmt.Sprintf("combo %d: grid reset failed: %v", comboNum+1, err))
			}

			if err := r.backend.ResetAcceptance(); err != nil {
				r.logger.Printf("[sweep] WARNING: Failed to reset acceptance: %v", err)
				r.addWarning(fmt.Sprintf("combo %d: reset acceptance failed: %v", comboNum+1, err))
			}

			// Settle: full settle for first combo, short wait for subsequent in "once" mode
			if settleOnce && comboNum > 0 {
				r.backend.WaitForGridSettle(regionRestoreWait)
			} else {
				r.backend.WaitForGridSettle(settleTime)
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

		// Capture analysis run ID from the server (set during PCAP replay)
		if isPCAP {
			combo.RunID = r.backend.GetLastAnalysisRunID()
		}

		// Update state
		r.mu.Lock()
		r.state.Results = append(r.state.Results, combo)
		r.state.CompletedCombos = comboNum + 1
		r.state.CurrentCombo = &combo
		r.mu.Unlock()

		// Release PCAP slot after each combo so the next combo can start
		// a fresh replay. Without this, currentSource stays DataSourcePCAP
		// and subsequent StartPCAPForSweep calls spin on the conflict retry
		// loop until they time out.
		if isPCAP {
			if err := r.backend.StopPCAPReplay(); err != nil {
				r.logger.Printf("[sweep] WARNING: Failed to stop PCAP after combo %d: %v", comboNum+1, err)
			}
		}
	}

	// Clean up: stop any lingering PCAP replay (covers early exits / errors)
	if isPCAP {
		if err := r.backend.StopPCAPReplay(); err != nil {
			r.logger.Printf("[sweep] WARNING: Failed to stop PCAP: %v", err)
		}
	}

	r.mu.Lock()
	r.state.Status = SweepStatusComplete
	now := time.Now()
	r.state.CompletedAt = &now
	r.mu.Unlock()
	r.logger.Printf("[sweep] Sweep complete: %d combinations evaluated", len(combos))

	// Persist completion to database
	r.persistComplete("complete", "", nil)
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

	// Track health: heading jitter
	jitterVals := make([]float64, len(results))
	for ri, r := range results {
		jitterVals[ri] = r.HeadingJitterDeg
	}
	combo.HeadingJitterDegMean, combo.HeadingJitterDegStddev = MeanStddev(jitterVals)

	// Track health: speed jitter
	speedJitterVals := make([]float64, len(results))
	for ri, r := range results {
		speedJitterVals[ri] = r.SpeedJitterMps
	}
	combo.SpeedJitterMpsMean, combo.SpeedJitterMpsStddev = MeanStddev(speedJitterVals)

	// Track health: fragmentation ratio
	fragVals := make([]float64, len(results))
	for ri, r := range results {
		fragVals[ri] = r.FragmentationRatio
	}
	combo.FragmentationRatioMean, _ = MeanStddev(fragVals)

	// Scene-level: foreground capture ratio
	capVals := make([]float64, len(results))
	for ri, r := range results {
		capVals[ri] = r.ForegroundCaptureRatio
	}
	combo.ForegroundCaptureMean, combo.ForegroundCaptureStddev = MeanStddev(capVals)

	// Scene-level: unbounded point ratio
	unbVals := make([]float64, len(results))
	for ri, r := range results {
		unbVals[ri] = r.UnboundedPointRatio
	}
	combo.UnboundedPointMean, combo.UnboundedPointStddev = MeanStddev(unbVals)

	// Scene-level: empty box ratio
	ebVals := make([]float64, len(results))
	for ri, r := range results {
		ebVals[ri] = r.EmptyBoxRatio
	}
	combo.EmptyBoxRatioMean, combo.EmptyBoxRatioStddev = MeanStddev(ebVals)

	return combo
}

// persistComplete saves the final sweep state to the database.
func (r *Runner) persistComplete(status, errMsg string, recommendation json.RawMessage) {
	if r.persister == nil || r.sweepID == "" {
		return
	}

	r.mu.RLock()
	state := r.state
	results := make([]ComboResult, len(state.Results))
	copy(results, state.Results)
	r.mu.RUnlock()

	resultsJSON, err := json.Marshal(results)
	if err != nil {
		r.logger.Printf("[sweep] WARNING: Failed to marshal sweep results for persistence: %v", err)
		resultsJSON = []byte("[]")
	}
	now := time.Now()
	if err := r.persister.SaveSweepComplete(r.sweepID, status, resultsJSON, recommendation, nil, now, errMsg, nil, nil, nil, "", ""); err != nil {
		r.logger.Printf("[sweep] WARNING: Failed to persist sweep completion: %v", err)
	}
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
		case int:
			return val, nil
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
		case int64:
			return val, nil
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
// Returns an error if the total number of combinations exceeds safe limits to prevent DoS attacks.
func cartesianProduct(params []SweepParam) ([]map[string]interface{}, error) {
	if len(params) == 0 {
		return nil, nil
	}

	// Validate total combinations before allocating memory to prevent DoS attacks.
	// Using int64 to detect overflow during multiplication.
	const maxCombos = 10000 // Hard limit to prevent excessive memory allocation
	total := int64(1)
	for _, p := range params {
		if len(p.Values) <= 0 {
			continue
		}
		total *= int64(len(p.Values))
		// Check for overflow or excessive combinations during computation
		if total > maxCombos || total < 0 {
			return nil, fmt.Errorf("parameter combinations would exceed safe limit of %d (detected: %d parameters with potential for >%d combinations)", maxCombos, len(params), maxCombos)
		}
	}

	if total == 0 {
		return nil, nil
	}

	combos := make([]map[string]interface{}, total)
	for i := range combos {
		combos[i] = make(map[string]interface{}, len(params))
	}

	repeat := int64(1)
	for dim := len(params) - 1; dim >= 0; dim-- {
		vals := params[dim].Values
		name := params[dim].Name
		cycle := int64(len(vals))
		for i := int64(0); i < total; i++ {
			combos[i][name] = vals[(i/repeat)%cycle]
		}
		repeat *= cycle
	}

	return combos, nil
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
