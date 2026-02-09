package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

// SceneStoreSaver is the interface for saving optimal parameters to a scene.
// This is defined here to avoid circular imports with the lidar package.
type SceneStoreSaver interface {
	SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error
}

const (
	// singleValueMarginRatio is the fraction of a single value to use as margin when narrowing
	// bounds around a single result (e.g., if value is 0.05, margin = 0.05 * 0.1 = 0.005).
	singleValueMarginRatio = 0.1

	// minMargin is the minimum absolute margin to add around a single value when narrowing bounds.
	minMargin = 0.001

	// defaultMarginSteps is the number of grid steps to add as margin on each side when narrowing bounds.
	defaultMarginSteps = 1.0

	// maxValuesPerParam limits the number of grid points generated for a single parameter
	// to avoid excessive memory allocation from untrusted input.
	maxValuesPerParam = 10000
)

// GroundTruthWeights holds the weights for computing composite ground truth scores.
// These weights control the relative importance of each metric in the overall score.
// This is defined here to avoid circular imports between sweep and lidar packages.
type GroundTruthWeights struct {
	DetectionRate     float64 `json:"detection_rate"`      // w1: Weight for detection rate (matched good tracks)
	Fragmentation     float64 `json:"fragmentation"`       // w2: Penalty for track splits
	FalsePositives    float64 `json:"false_positives"`     // w3: Penalty for unmatched candidate tracks
	VelocityCoverage  float64 `json:"velocity_coverage"`   // w4: Bonus for tracks with velocity data
	QualityPremium    float64 `json:"quality_premium"`     // w5: Bonus for "perfect" quality tracks
	TruncationRate    float64 `json:"truncation_rate"`     // w6: Penalty for truncated tracks
	VelocityNoiseRate float64 `json:"velocity_noise_rate"` // w7: Penalty for noisy velocity tracks
	StoppedRecovery   float64 `json:"stopped_recovery"`    // w8: Bonus for stopped vehicle recovery
}

// DefaultGroundTruthWeights returns the default weights from the design doc.
func DefaultGroundTruthWeights() GroundTruthWeights {
	return GroundTruthWeights{
		DetectionRate:     1.0,
		Fragmentation:     5.0,
		FalsePositives:    2.0,
		VelocityCoverage:  0.5,
		QualityPremium:    0.3,
		TruncationRate:    0.4,
		VelocityNoiseRate: 0.4,
		StoppedRecovery:   0.2,
	}
}

// AutoTuneRequest defines the parameters for an auto-tuning run.
type AutoTuneRequest struct {
	Params           []SweepParam      `json:"params"`
	MaxRounds        int               `json:"max_rounds"`
	ValuesPerParam   int               `json:"values_per_param"`
	TopK             int               `json:"top_k"`
	Objective        string            `json:"objective"` // "acceptance", "weighted", "ground_truth"
	Weights          *ObjectiveWeights `json:"weights,omitempty"`
	Iterations       int               `json:"iterations"`
	SettleTime       string            `json:"settle_time"`
	Interval         string            `json:"interval"`
	Seed             string            `json:"seed"`
	DataSource       string            `json:"data_source"`
	PCAPFile         string            `json:"pcap_file,omitempty"`
	PCAPStartSecs    float64           `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs float64           `json:"pcap_duration_secs,omitempty"`
	SettleMode       string            `json:"settle_mode,omitempty"`

	// Phase 5: Ground truth evaluation support
	SceneID            string              `json:"scene_id,omitempty"`             // When set, enables ground truth evaluation
	GroundTruthWeights *GroundTruthWeights `json:"ground_truth_weights,omitempty"` // Weights for ground truth scoring
}

// RoundSummary holds the results of one round of auto-tuning.
type RoundSummary struct {
	Round      int                    `json:"round"`
	Bounds     map[string][2]float64  `json:"bounds"`
	BestScore  float64                `json:"best_score"`
	BestParams map[string]interface{} `json:"best_params"`
	NumCombos  int                    `json:"num_combos"`
	TopK       []ScoredResult         `json:"top_k"`
}

// AutoTuneState holds the current state and results of an auto-tuning run.
type AutoTuneState struct {
	Status          SweepStatus            `json:"status"`
	Mode            string                 `json:"mode"` // always "auto"
	Round           int                    `json:"round"`
	TotalRounds     int                    `json:"total_rounds"`
	CompletedCombos int                    `json:"completed_combos"`
	TotalCombos     int                    `json:"total_combos"`
	RoundResults    []RoundSummary         `json:"round_results"`
	Results         []ComboResult          `json:"results"`
	Recommendation  map[string]interface{} `json:"recommendation,omitempty"`
	Error           string                 `json:"error,omitempty"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
}

// AutoTuner orchestrates iterative parameter sweep rounds.
type AutoTuner struct {
	runner *Runner
	mu     sync.RWMutex
	state  AutoTuneState
	cancel context.CancelFunc

	// Phase 5: Ground truth scoring support
	// When set, this function is called to score each combination's results using ground truth evaluation.
	// The function receives the scene_id and candidate run_id, and returns the composite ground truth score.
	groundTruthScorer func(sceneID, candidateRunID string) (float64, error)

	// Phase 5: Scene store for saving optimal params when ground truth mode completes
	sceneStore SceneStoreSaver
}

// NewAutoTuner creates a new AutoTuner wrapping the given Runner.
func NewAutoTuner(runner *Runner) *AutoTuner {
	return &AutoTuner{
		runner: runner,
		state: AutoTuneState{
			Status: SweepStatusIdle,
			Mode:   "auto",
		},
	}
}

// SetGroundTruthScorer sets the ground truth scoring function for label-aware auto-tuning.
// This function will be called to evaluate each combination's results against reference ground truth
// when objective is "ground_truth" and scene_id is set.
func (at *AutoTuner) SetGroundTruthScorer(scorer func(sceneID, candidateRunID string) (float64, error)) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.groundTruthScorer = scorer
}

// SetSceneStore sets the scene store for saving optimal parameters after auto-tuning completes.
func (at *AutoTuner) SetSceneStore(store SceneStoreSaver) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.sceneStore = store
}

// Start begins an auto-tuning run. Implements the AutoTuneRunner interface.
// It accepts an interface{} which should be a map or AutoTuneRequest.
func (at *AutoTuner) Start(ctx context.Context, reqInterface interface{}) error {
	var req AutoTuneRequest

	switch v := reqInterface.(type) {
	case AutoTuneRequest:
		req = v
	case map[string]interface{}:
		// Re-marshal via JSON for consistency
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshalling auto-tune request: %w", err)
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return fmt.Errorf("parsing auto-tune request: %w", err)
		}
	default:
		return fmt.Errorf("invalid request type: %T", reqInterface)
	}

	return at.start(ctx, req)
}

// start is the internal implementation that does the actual work.
func (at *AutoTuner) start(ctx context.Context, req AutoTuneRequest) error {
	// Validate AutoTuner configuration before starting background work.
	if at.runner == nil {
		return fmt.Errorf("auto-tuner runner is not configured")
	}

	// Guard nil context
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate and apply defaults
	req = applyAutoTuneDefaults(req)

	// Phase 5: Validate ground truth configuration
	if req.Objective == "ground_truth" {
		if req.SceneID == "" {
			return fmt.Errorf("ground_truth objective requires scene_id to be set")
		}
		if at.groundTruthScorer == nil {
			return fmt.Errorf("ground_truth objective requires a ground truth scorer to be configured")
		}
		// Apply default ground truth weights if none provided
		if req.GroundTruthWeights == nil {
			defaultWeights := DefaultGroundTruthWeights()
			req.GroundTruthWeights = &defaultWeights
		}
	}

	if req.MaxRounds > 10 {
		return fmt.Errorf("max_rounds must not exceed 10, got %d", req.MaxRounds)
	}

	if req.ValuesPerParam < 2 {
		return fmt.Errorf("values_per_param must be at least 2, got %d", req.ValuesPerParam)
	}
	if req.ValuesPerParam > 20 {
		return fmt.Errorf("values_per_param must not exceed 20, got %d", req.ValuesPerParam)
	}

	if req.TopK > 50 {
		return fmt.Errorf("top_k must not exceed 50, got %d", req.TopK)
	}

	if len(req.Params) == 0 {
		return fmt.Errorf("no parameters specified for auto-tuning")
	}

	// Limit maximum number of parameters to prevent excessive memory allocation.
	const maxParams = 10
	if len(req.Params) > maxParams {
		return fmt.Errorf("too many parameters: maximum %d allowed, got %d", maxParams, len(req.Params))
	}

	// Validate that each param has start/end bounds
	for i, p := range req.Params {
		if p.Start >= p.End {
			return fmt.Errorf("param %q: start must be less than end", p.Name)
		}
		// Ensure type is numeric for auto-tuning
		switch p.Type {
		case "float64", "int", "int64":
			// OK
		default:
			return fmt.Errorf("param %q: auto-tuning only supports numeric types (float64, int, int64), got %q", p.Name, p.Type)
		}
		req.Params[i] = p
	}

	// Now acquire lock for state modification
	at.mu.Lock()
	if at.state.Status == SweepStatusRunning {
		at.mu.Unlock()
		return ErrSweepAlreadyRunning
	}

	now := time.Now()
	at.state = AutoTuneState{
		Status:       SweepStatusRunning,
		Mode:         "auto",
		StartedAt:    &now,
		TotalRounds:  req.MaxRounds,
		RoundResults: make([]RoundSummary, 0, req.MaxRounds),
		Results:      make([]ComboResult, 0),
	}

	runCtx, cancel := context.WithCancel(ctx)
	at.cancel = cancel
	at.mu.Unlock()

	// Run auto-tuning in background
	go at.run(runCtx, req)

	return nil
}

// GetAutoTuneState returns the current auto-tuning state as a typed value.
// The returned state is a deep copy safe for concurrent use.
func (at *AutoTuner) GetAutoTuneState() AutoTuneState {
	at.mu.RLock()
	defer at.mu.RUnlock()
	// Return a deep copy to avoid race conditions on nested maps/slices.
	state := at.state

	// Deep-copy RoundResults (contains nested maps and slices)
	roundResults := make([]RoundSummary, len(at.state.RoundResults))
	for i, rs := range at.state.RoundResults {
		roundResults[i] = RoundSummary{
			Round:     rs.Round,
			Bounds:    copyBounds(rs.Bounds),
			BestScore: rs.BestScore,
			NumCombos: rs.NumCombos,
		}
		roundResults[i].BestParams = copyParamValues(rs.BestParams)
		topK := make([]ScoredResult, len(rs.TopK))
		for j, sr := range rs.TopK {
			topK[j] = sr
			topK[j].ParamValues = copyParamValues(sr.ParamValues)
		}
		roundResults[i].TopK = topK
	}
	state.RoundResults = roundResults

	// Deep-copy Results
	results := make([]ComboResult, len(at.state.Results))
	for i, r := range at.state.Results {
		results[i] = r
		results[i].ParamValues = copyParamValues(r.ParamValues)
	}
	state.Results = results

	// Deep-copy Recommendation
	state.Recommendation = copyParamValues(at.state.Recommendation)

	return state
}

// GetState implements the AutoTuneRunner interface.
// It returns the auto-tuning state as interface{}.
func (at *AutoTuner) GetState() interface{} {
	return at.GetAutoTuneState()
}

// Stop cancels a running auto-tune.
func (at *AutoTuner) Stop() {
	at.mu.Lock()
	if at.cancel != nil {
		at.cancel()
	}
	at.mu.Unlock()
}

// run executes the auto-tuning algorithm.
func (at *AutoTuner) run(ctx context.Context, req AutoTuneRequest) {
	log.Printf("[sweep] Auto-tuner started: %d rounds, %d values/param, top %d", req.MaxRounds, req.ValuesPerParam, req.TopK)

	// Set up objective weights
	var weights ObjectiveWeights
	if req.Weights != nil {
		weights = *req.Weights
	} else if req.Objective == "acceptance" {
		// Acceptance-only optimization
		weights = ObjectiveWeights{
			Acceptance:   1.0,
			Misalignment: 0.0,
			Alignment:    0.0,
			NonzeroCells: 0.0,
		}
	} else {
		// Default weighted objective
		weights = DefaultObjectiveWeights()
	}

	// Track current bounds for each parameter (start with initial bounds from request)
	currentBounds := make(map[string][2]float64)
	for _, p := range req.Params {
		currentBounds[p.Name] = [2]float64{p.Start, p.End}
	}

	var allResults []ComboResult
	var overallBest *ScoredResult

	for round := 1; round <= req.MaxRounds; round++ {
		select {
		case <-ctx.Done():
			at.setError("auto-tune cancelled")
			return
		default:
		}

		log.Printf("[sweep] Auto-tune round %d/%d", round, req.MaxRounds)

		at.mu.Lock()
		at.state.Round = round
		at.mu.Unlock()

		// Generate parameter grid from current bounds
		gridParams := make([]SweepParam, len(req.Params))
		totalCombos := 1
		for i, p := range req.Params {
			bounds := currentBounds[p.Name]

			// Convert to interface{} slice, using integer grid for int/int64 types
			var ifaceValues []interface{}
			switch p.Type {
			case "int", "int64":
				intValues := generateIntGrid(bounds[0], bounds[1], req.ValuesPerParam)
				ifaceValues = make([]interface{}, len(intValues))
				for j, v := range intValues {
					if p.Type == "int64" {
						ifaceValues[j] = int64(v)
					} else {
						ifaceValues[j] = v
					}
				}
			default:
				values := generateGrid(bounds[0], bounds[1], req.ValuesPerParam)
				ifaceValues = make([]interface{}, len(values))
				for j, v := range values {
					ifaceValues[j] = v
				}
			}

			gridParams[i] = SweepParam{
				Name:   p.Name,
				Type:   p.Type,
				Values: ifaceValues,
			}
			totalCombos *= len(ifaceValues)
		}

		// Enforce max combinations per round (same as regular sweep)
		if totalCombos > 1000 {
			at.setError(fmt.Sprintf("round %d would generate %d combinations (max 1000)", round, totalCombos))
			return
		}

		at.mu.Lock()
		at.state.TotalCombos = totalCombos
		at.state.CompletedCombos = 0
		at.mu.Unlock()

		// Build a SweepRequest for this round
		sweepReq := SweepRequest{
			Mode:             "params",
			Params:           gridParams,
			Iterations:       req.Iterations,
			Interval:         req.Interval,
			SettleTime:       req.SettleTime,
			Seed:             req.Seed,
			DataSource:       req.DataSource,
			PCAPFile:         req.PCAPFile,
			PCAPStartSecs:    req.PCAPStartSecs,
			PCAPDurationSecs: req.PCAPDurationSecs,
			SettleMode:       req.SettleMode,
		}

		// Start the sweep for this round
		if err := at.runner.StartWithRequest(ctx, sweepReq); err != nil {
			at.setError(fmt.Sprintf("round %d: failed to start sweep: %v", round, err))
			return
		}

		// Poll for completion
		if err := at.waitForSweepComplete(ctx); err != nil {
			at.setError(fmt.Sprintf("round %d: %v", round, err))
			return
		}

		// Get results from the runner
		sweepState := at.runner.GetSweepState()
		if sweepState.Status == SweepStatusError {
			at.setError(fmt.Sprintf("round %d: sweep failed: %s", round, sweepState.Error))
			return
		}

		roundResults := sweepState.Results
		if len(roundResults) == 0 {
			at.setError(fmt.Sprintf("round %d: no results returned", round))
			return
		}

		// Score and rank results
		var scored []ScoredResult
		if req.Objective == "ground_truth" {
			// Phase 5: Ground truth evaluation mode
			// Each combo should have created an analysis run with ID stored in result.RunID
			scored = make([]ScoredResult, len(roundResults))
			for i, result := range roundResults {
				// Copy common fields first
				scored[i].ParamValues = result.ParamValues
				scored[i].OverallAcceptMean = result.OverallAcceptMean
				scored[i].MisalignmentRatioMean = result.MisalignmentRatioMean
				scored[i].AlignmentDegMean = result.AlignmentDegMean
				scored[i].NonzeroCellsMean = result.NonzeroCellsMean

				// Then evaluate score
				if result.RunID == "" {
					// No run ID - log warning and give score 0
					log.Printf("[sweep] WARNING: combo %d has no RunID; cannot evaluate with ground truth. Assigning score 0.", i)
					scored[i].Score = 0.0
				} else {
					// Call ground truth scorer
					score, err := at.groundTruthScorer(req.SceneID, result.RunID)
					if err != nil {
						log.Printf("[sweep] ERROR: scoring combo %d (run %s) with ground truth: %v. Assigning score 0.", i, result.RunID, err)
						scored[i].Score = 0.0
					} else {
						scored[i].Score = score
					}
				}
			}
			// Sort by ground truth score (highest = best)
			scored = sortScoredResults(scored)
		} else {
			// Standard objective-based scoring
			scored = RankResults(roundResults, weights)
		}

		// Update overall best
		if overallBest == nil || scored[0].Score > overallBest.Score {
			overallBest = &scored[0]
		}

		// Select top K
		topK := scored
		if len(topK) > req.TopK {
			topK = scored[:req.TopK]
		}

		// Store round summary
		roundSummary := RoundSummary{
			Round:      round,
			Bounds:     copyBounds(currentBounds),
			BestScore:  scored[0].Score,
			BestParams: scored[0].ParamValues,
			NumCombos:  len(roundResults),
			TopK:       topK,
		}

		at.mu.Lock()
		at.state.RoundResults = append(at.state.RoundResults, roundSummary)
		at.mu.Unlock()

		allResults = append(allResults, roundResults...)

		// Narrow bounds for next round (unless this is the last round)
		if round < req.MaxRounds {
			for _, p := range req.Params {
				start, end := narrowBounds(topK, p.Name, req.ValuesPerParam)

				// Clamp to original bounds
				origBounds := [2]float64{p.Start, p.End}
				if start < origBounds[0] {
					start = origBounds[0]
				}
				if end > origBounds[1] {
					end = origBounds[1]
				}

				currentBounds[p.Name] = [2]float64{start, end}
			}

			log.Printf("[sweep] Narrowed bounds for round %d: %v", round+1, currentBounds)
		}
	}

	// Build final recommendation
	recommendation := make(map[string]interface{})
	if overallBest != nil {
		for k, v := range overallBest.ParamValues {
			recommendation[k] = v
		}
		recommendation["score"] = overallBest.Score
		recommendation["acceptance_rate"] = overallBest.OverallAcceptMean
		recommendation["misalignment_ratio"] = overallBest.MisalignmentRatioMean
		recommendation["alignment_deg"] = overallBest.AlignmentDegMean
		recommendation["nonzero_cells"] = overallBest.NonzeroCellsMean
	}

	now := time.Now()
	at.mu.Lock()
	at.state.Status = SweepStatusComplete
	at.state.CompletedAt = &now
	at.state.Recommendation = recommendation
	at.state.Results = allResults
	at.mu.Unlock()

	log.Printf("[sweep] Auto-tune complete: recommendation=%v, score=%.4f", overallBest.ParamValues, overallBest.Score)

	// Phase 5: Save optimal params to scene when ground truth mode is enabled
	if req.SceneID != "" && req.Objective == "ground_truth" && overallBest != nil && at.sceneStore != nil {
		// Extract just the parameter values (without score/metrics) for storage
		optimalParams := make(map[string]interface{})
		for k, v := range overallBest.ParamValues {
			optimalParams[k] = v
		}

		paramsJSON, err := json.Marshal(optimalParams)
		if err != nil {
			log.Printf("[sweep] Error marshalling optimal params for scene %s: %v", req.SceneID, err)
		} else {
			if err := at.sceneStore.SetOptimalParams(req.SceneID, paramsJSON); err != nil {
				log.Printf("[sweep] Error saving optimal params for scene %s: %v", req.SceneID, err)
			} else {
				log.Printf("[sweep] Saved optimal params for scene %s: %s", req.SceneID, string(paramsJSON))
			}
		}
	}
}

// waitForSweepComplete polls the runner until the sweep completes or fails.
func (at *AutoTuner) waitForSweepComplete(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled")
		case <-ticker.C:
			state := at.runner.GetSweepState()

			// Update progress
			at.mu.Lock()
			at.state.CompletedCombos = state.CompletedCombos
			at.mu.Unlock()

			switch state.Status {
			case SweepStatusComplete:
				return nil
			case SweepStatusError:
				return fmt.Errorf("sweep error: %s", state.Error)
			case SweepStatusRunning:
				// Continue polling
			default:
				// Idle or unknown status - should not happen
				return fmt.Errorf("unexpected sweep status: %s", state.Status)
			}
		}
	}
}

// setError sets an error state and marks the auto-tune as failed.
func (at *AutoTuner) setError(msg string) {
	log.Printf("[sweep] Auto-tune error: %s", msg)
	now := time.Now()
	at.mu.Lock()
	at.state.Status = SweepStatusError
	at.state.Error = msg
	at.state.CompletedAt = &now
	at.mu.Unlock()
}

// narrowBounds computes narrowed parameter bounds from the top K results.
// For each parameter, finds min/max across top K, adds a margin of 1 step.
func narrowBounds(topK []ScoredResult, paramName string, valuesPerParam int) (start, end float64) {
	if len(topK) == 0 {
		return 0, 0
	}

	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for _, r := range topK {
		val, ok := r.ParamValues[paramName]
		if !ok {
			continue
		}

		// Convert to float64 for comparison
		var fval float64
		switch v := val.(type) {
		case float64:
			fval = v
		case int:
			fval = float64(v)
		case int64:
			fval = float64(v)
		default:
			continue
		}

		if fval < minVal {
			minVal = fval
		}
		if fval > maxVal {
			maxVal = fval
		}
	}

	// If no numeric values were found for this parameter, do not narrow bounds.
	if math.IsInf(minVal, 1) && math.IsInf(maxVal, -1) {
		return 0, 0
	}

	// If we only have one result, or all results have the same value
	if minVal == maxVal {
		// Add a small margin around the single value
		margin := math.Abs(minVal) * singleValueMarginRatio
		if margin < minMargin {
			margin = minMargin
		}
		return minVal - margin, maxVal + margin
	}

	// Calculate step size based on the range and number of values
	rangeSize := maxVal - minVal
	step := rangeSize / float64(valuesPerParam-1)

	// Add margin on each side (1 step by default)
	return minVal - step*defaultMarginSteps, maxVal + step*defaultMarginSteps
}

// generateGrid creates N evenly-spaced values between start and end (inclusive).
func generateGrid(start, end float64, n int) []float64 {
	if n <= 0 {
		return []float64{}
	}
	if n == 1 {
		// Return midpoint
		return []float64{(start + end) / 2.0}
	}

	// Enforce an upper bound to prevent excessive memory allocation from untrusted input.
	// Clamp to safe maximum before allocation to prevent DoS attacks.
	if n > maxValuesPerParam {
		n = maxValuesPerParam
	}
	// Additional safety check: ensure n is within safe bounds after clamping
	if n < 0 || n > maxValuesPerParam {
		return []float64{}
	}

	grid := make([]float64, n)
	step := (end - start) / float64(n-1)
	for i := 0; i < n; i++ {
		grid[i] = start + step*float64(i)
	}
	return grid
}

// copyBounds creates a deep copy of a bounds map.
func copyBounds(bounds map[string][2]float64) map[string][2]float64 {
	result := make(map[string][2]float64)
	for k, v := range bounds {
		result[k] = v
	}
	return result
}

// copyParamValues creates a deep copy of a parameter values map.
func copyParamValues(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// applyAutoTuneDefaults applies default values for unset fields on an AutoTuneRequest.
// This is exported as a helper so tests can exercise the defaulting logic directly.
func applyAutoTuneDefaults(req AutoTuneRequest) AutoTuneRequest {
	if req.MaxRounds <= 0 {
		req.MaxRounds = 3
	}
	if req.ValuesPerParam <= 0 {
		req.ValuesPerParam = 5
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.Objective == "" {
		req.Objective = "acceptance"
	}
	return req
}

// generateIntGrid creates N evenly-spaced integer values between start and end (inclusive).
// Values are rounded to the nearest integer and deduplicated while preserving order.
// Both endpoints are always included (if they map to distinct integers).
func generateIntGrid(start, end float64, n int) []int {
	if n <= 0 {
		return []int{}
	}

	intStart := int(math.Round(start))
	intEnd := int(math.Round(end))

	if n == 1 {
		return []int{(intStart + intEnd) / 2}
	}

	// Enforce an upper bound to prevent excessive memory allocation.
	if n > maxValuesPerParam {
		n = maxValuesPerParam
	}
	// Additional safety check: ensure n is within safe bounds after clamping
	if n < 0 || n > maxValuesPerParam {
		return []int{}
	}

	// Generate float grid, round, and deduplicate
	floatGrid := generateGrid(start, end, n)
	seen := make(map[int]bool, len(floatGrid))
	result := make([]int, 0, len(floatGrid))
	for _, v := range floatGrid {
		iv := int(math.Round(v))
		if !seen[iv] {
			seen[iv] = true
			result = append(result, iv)
		}
	}

	// Ensure endpoints are included
	if !seen[intStart] {
		result = append([]int{intStart}, result...)
	}
	if !seen[intEnd] {
		result = append(result, intEnd)
	}

	return result
}

// sortScoredResults sorts scored results by score in descending order (highest first).
// Returns a new sorted slice, leaving the original unchanged.
func sortScoredResults(scored []ScoredResult) []ScoredResult {
	// Create a copy to avoid modifying the input
	result := make([]ScoredResult, len(scored))
	copy(result, scored)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	return result
}
