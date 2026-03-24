package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LabelProgressQuerier queries label progress for a run.
type LabelProgressQuerier interface {
	GetLabelingProgress(runID string) (total, labelled int, byClass map[string]int, err error)
	GetRunTracks(runID string) ([]HINTRunTrack, error)
	UpdateTrackLabel(runID, trackID, userLabel, qualityLabel string, confidence float32, labelerID, labelSource string) error
}

// HINTRunTrack is a minimal track representation for label carryover.
type HINTRunTrack struct {
	TrackID        string
	StartUnixNanos int64
	EndUnixNanos   int64
	UserLabel      string
	QualityLabel   string
}

// SceneGetter retrieves scene data and sets reference runs.
type SceneGetter interface {
	GetScene(sceneID string) (*HINTScene, error)
	SetReferenceRun(sceneID, runID string) error
}

// HINTScene is a minimal scene representation.
type HINTScene struct {
	ReplayCaseID      string
	SensorID          string
	PCAPFile          string
	PCAPStartSecs     *float64
	PCAPDurationSecs  *float64
	ReferenceRunID    string
	RecommendedParams json.RawMessage
	OptimalParamsJSON json.RawMessage // legacy fallback
}

// ReferenceRunCreator creates analysis runs for HINT reference.
type ReferenceRunCreator interface {
	CreateSweepRun(sensorID, pcapFile string, paramsJSON json.RawMessage, pcapStartSecs, pcapDurationSecs float64) (string, error)
}

// HINTSweepRequest defines the request for an HINT tuning sweep.
type HINTSweepRequest struct {
	ReplayCaseID          string              `json:"replay_case_id"`
	NumRounds             int                 `json:"num_rounds"`
	RoundDurations        []int               `json:"round_durations"`
	Params                []SweepParam        `json:"params"`
	ValuesPerParam        int                 `json:"values_per_param"`
	TopK                  int                 `json:"top_k"`
	Iterations            int                 `json:"iterations"`
	Interval              string              `json:"interval"`
	SettleTime            string              `json:"settle_time"`
	Seed                  string              `json:"seed"`
	SettleMode            string              `json:"settle_mode"`
	GroundTruthWeights    *GroundTruthWeights `json:"ground_truth_weights"`
	AcceptanceCriteria    *AcceptanceCriteria `json:"acceptance_criteria"`
	MinLabelThreshold     float64             `json:"min_label_threshold"`
	CarryOverLabels       bool                `json:"carry_over_labels"`
	MinClassCoverage      map[string]int      `json:"min_class_coverage,omitempty"`
	MinTemporalSpreadSecs float64             `json:"min_temporal_spread_secs,omitempty"`

	// TuneBackground controls whether the background grid is re-settled
	// for each exploration combo. When false (default), the grid settled
	// during the reference run is reused (SettleMode forced to "once").
	// Set to true when sweeping background-subtraction params so each
	// combo gets a fresh settle.
	TuneBackground bool `json:"tune_background,omitempty"`
}

// HINTState represents the current state of an HINT tuning session.
type HINTState struct {
	Status                string                 `json:"status"` // "idle","running_reference","awaiting_labels","running_sweep","completed","failed"
	Mode                  string                 `json:"mode"`   // always "hint"
	CurrentRound          int                    `json:"current_round"`
	TotalRounds           int                    `json:"total_rounds"`
	ReferenceRunID        string                 `json:"reference_run_id,omitempty"`
	LabelProgress         *LabelProgress         `json:"label_progress,omitempty"`
	LabelDeadline         *time.Time             `json:"label_deadline,omitempty"`
	SweepDeadline         *time.Time             `json:"sweep_deadline,omitempty"`
	AutoTuneState         *AutoTuneState         `json:"auto_tune_state,omitempty"`
	Recommendation        map[string]interface{} `json:"recommendation,omitempty"`
	RoundHistory          []HINTRound            `json:"round_history"`
	Error                 string                 `json:"error,omitempty"`
	MinLabelThreshold     float64                `json:"min_label_threshold"`
	LabelsCarriedOver     int                    `json:"labels_carried_over"`
	NextSweepDuration     int                    `json:"next_sweep_duration_mins"`
	MinClassCoverage      map[string]int         `json:"min_class_coverage,omitempty"`
	MinTemporalSpreadSecs float64                `json:"min_temporal_spread_secs,omitempty"`
	TuneBackground        bool                   `json:"tune_background"`
}

// LabelProgress tracks labelling progress for a reference run.
type LabelProgress struct {
	Total    int            `json:"total"`
	Labelled int            `json:"labelled"`
	Pct      float64        `json:"progress_pct"`
	ByClass  map[string]int `json:"by_class"`
}

// HINTRound records the results of a single HINT round.
type HINTRound struct {
	Round               int                `json:"round"`
	ReferenceRunID      string             `json:"reference_run_id"`
	LabelledAt          *time.Time         `json:"labelled_at,omitempty"`
	LabelProgress       *LabelProgress     `json:"label_progress,omitempty"`
	LabelsCarriedOver   int                `json:"labels_carried_over"`
	SweepID             string             `json:"sweep_id,omitempty"`
	BestScore           float64            `json:"best_score"`
	BestParams          map[string]float64 `json:"best_params,omitempty"`
	BestScoreComponents *ScoreComponents   `json:"best_score_components,omitempty"`
}

// defaultHINTParams returns the default set of sweep parameters for HINT
// when the caller does not specify any.
//
// When tuneBackground is false (the default), only foreground/clustering
// params are returned — the background grid settles once on the reference
// run and is reused across all exploration combos.
//
// When tuneBackground is true, background-subtraction params are also
// included since each exploration combo must re-settle the grid.
func defaultHINTParams(tuneBackground bool) []SweepParam {
	params := []SweepParam{
		{Name: "l4.dbscan_xy_v1.foreground_min_cluster_points", Type: "int", Start: 0, End: 20},
		{Name: "l4.dbscan_xy_v1.foreground_dbscan_eps", Type: "float64", Start: 0, End: 2.0},
	}
	if tuneBackground {
		params = append(params,
			SweepParam{Name: "l3.ema_baseline_v1.noise_relative", Type: "float64", Start: 0.01, End: 0.2},
			SweepParam{Name: "l3.ema_baseline_v1.closeness_multiplier", Type: "float64", Start: 1.0, End: 20.0},
			SweepParam{Name: "l3.ema_baseline_v1.background_update_fraction", Type: "float64", Start: 0.005, End: 0.1},
			SweepParam{Name: "l3.ema_baseline_v1.safety_margin_metres", Type: "float64", Start: 0, End: 2.0},
		)
	}
	return params
}

// HINTTuner orchestrates human-in-the-loop parameter optimisation.
type HINTTuner struct {
	mu sync.RWMutex

	state     HINTState
	stateCond *sync.Cond // broadcast on every status transition
	cancel    context.CancelFunc

	// Dependencies (injected)
	autoTuner         *AutoTuner
	labelQuerier      LabelProgressQuerier
	sceneGetter       SceneGetter
	sceneStore        SceneStoreSaver
	runCreator        ReferenceRunCreator
	persister         SweepPersister
	groundTruthScorer func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error)

	// Internal coordination
	continueCh    chan continueSignal
	labelUpdateCh chan struct{} // notified on every label update via API
	sweepID       string

	// For testability
	pollInterval time.Duration
	logger       printfLogger
}

// continueSignal is sent to proceed from awaiting_labels to running_sweep.
type continueSignal struct {
	NextSweepDurationMins int  `json:"next_sweep_duration_mins"`
	AddRound              bool `json:"add_round"`
}

// NewHINTTuner creates a new HINT tuner with the given AutoTuner backend.
func NewHINTTuner(autoTuner *AutoTuner) *HINTTuner {
	rt := &HINTTuner{
		autoTuner:     autoTuner,
		continueCh:    make(chan continueSignal, 1),
		labelUpdateCh: make(chan struct{}, 1),
		pollInterval:  60 * time.Second, // safety fallback; label updates arrive via channel
		logger:        opsPrintfLogger{},
		state: HINTState{
			Status:       "idle",
			Mode:         "hint",
			RoundHistory: []HINTRound{},
		},
	}
	rt.stateCond = sync.NewCond(rt.mu.RLocker())
	return rt
}

// SetLogger sets the logger for the HINT tuner. Use log.New(io.Discard, "", 0)
// in tests to suppress expected error-path log output.
func (rt *HINTTuner) SetLogger(l *log.Logger) {
	if l == nil {
		rt.logger = opsPrintfLogger{}
		return
	}
	rt.logger = l
}

// SetLabelQuerier sets the label progress querier dependency.
func (rt *HINTTuner) SetLabelQuerier(q LabelProgressQuerier) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.labelQuerier = q
}

// SetSceneGetter sets the scene getter dependency.
func (rt *HINTTuner) SetSceneGetter(g SceneGetter) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.sceneGetter = g
}

// SetSceneStore sets the scene store dependency.
func (rt *HINTTuner) SetSceneStore(s SceneStoreSaver) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.sceneStore = s
}

// SetRunCreator sets the reference run creator dependency.
func (rt *HINTTuner) SetRunCreator(c ReferenceRunCreator) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.runCreator = c
}

// SetPersister sets the persistence layer.
func (rt *HINTTuner) SetPersister(p SweepPersister) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.persister = p
}

// SetGroundTruthScorer sets the ground truth scoring function.
func (rt *HINTTuner) SetGroundTruthScorer(scorer func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error)) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.groundTruthScorer = scorer
}

// Start initiates an HINT tuning session.
func (rt *HINTTuner) Start(ctx context.Context, reqInterface interface{}) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Parse request
	var req HINTSweepRequest
	switch v := reqInterface.(type) {
	case HINTSweepRequest:
		req = v
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return fmt.Errorf("failed to unmarshal request: %w", err)
		}
	default:
		return fmt.Errorf("unsupported request type: %T", reqInterface)
	}

	// Validate request
	if req.ReplayCaseID == "" {
		return fmt.Errorf("replay_case_id is required")
	}
	if req.NumRounds < 1 || req.NumRounds > 10 {
		return fmt.Errorf("num_rounds must be between 1 and 10, got %d", req.NumRounds)
	}
	// Auto-populate default HINT sweep params if none provided.
	if len(req.Params) == 0 {
		req.Params = defaultHINTParams(req.TuneBackground)
	}
	if len(req.Params) > 20 {
		return fmt.Errorf("too many parameters (max 20, got %d)", len(req.Params))
	}

	// Apply defaults
	if req.MinLabelThreshold == 0 {
		req.MinLabelThreshold = 0.9
	}
	if req.TopK == 0 {
		req.TopK = 3
	}
	if req.ValuesPerParam == 0 {
		req.ValuesPerParam = 5
	}

	// When TuneBackground is false (default), force settle_mode to "once"
	// so the grid is settled once on the first exploration combo and reused.
	if !req.TuneBackground && req.SettleMode == "" {
		req.SettleMode = "once"
	}

	// Pre-fetch scene to derive PCAP duration and compute iterations.
	sensorID := ""
	var scenePCAPDuration float64
	if rt.sceneGetter != nil {
		if scene, err := rt.sceneGetter.GetScene(req.ReplayCaseID); err == nil && scene != nil {
			sensorID = scene.SensorID
			if scene.PCAPDurationSecs != nil && *scene.PCAPDurationSecs > 0 {
				scenePCAPDuration = *scene.PCAPDurationSecs
			}
		}
	}

	// Fit iterations to the scene duration: scene duration takes precedence
	// and we derive the number of intervals to cover the full scene window.
	if req.Iterations == 0 {
		if scenePCAPDuration > 0 {
			interval := 2.0 // default interval in seconds
			if req.Interval != "" {
				if d, err := time.ParseDuration(req.Interval); err == nil {
					interval = d.Seconds()
				}
			}
			if interval > 0 {
				req.Iterations = int(math.Floor(scenePCAPDuration / interval))
			}
		}
		if req.Iterations < 1 {
			req.Iterations = 10
		}
		rt.logger.Printf("[hint] Fitted iterations=%d from scene duration=%.1fs", req.Iterations, scenePCAPDuration)
	}

	// Check not already running
	if rt.state.Status == string(SweepStatusRunning) ||
		rt.state.Status == "running_reference" ||
		rt.state.Status == "awaiting_labels" ||
		rt.state.Status == "running_sweep" {
		return ErrSweepAlreadyRunning
	}

	// Initialize state
	rt.sweepID = uuid.New().String()
	rt.state = HINTState{
		Status:                "running_reference",
		Mode:                  "hint",
		CurrentRound:          0,
		TotalRounds:           req.NumRounds,
		RoundHistory:          []HINTRound{},
		MinLabelThreshold:     req.MinLabelThreshold,
		NextSweepDuration:     60, // default sweep duration in minutes
		MinClassCoverage:      req.MinClassCoverage,
		MinTemporalSpreadSecs: req.MinTemporalSpreadSecs,
		TuneBackground:        req.TuneBackground,
	}

	// Persist start if persister available
	if rt.persister != nil {
		reqJSON, err := json.Marshal(req)
		if err != nil {
			rt.logger.Printf("[hint] WARNING: Failed to marshal request for persistence: %v", err)
			reqJSON = []byte("{}")
		}
		if err := rt.persister.SaveSweepStart(rt.sweepID, sensorID, "hint", reqJSON, time.Now(), "ground_truth", ObjectiveVersion); err != nil {
			rt.logger.Printf("[hint] Failed to persist sweep start: %v", err)
		}
	}

	// Launch background goroutine
	runCtx, cancel := context.WithCancel(ctx)
	rt.cancel = cancel
	go rt.run(runCtx, req)

	rt.logger.Printf("[hint] Started HINT tuning session %s for replay case %s (%d rounds)", rt.sweepID, req.ReplayCaseID, req.NumRounds)
	return nil
}

// run is the main HINT orchestration loop.
func (rt *HINTTuner) run(ctx context.Context, req HINTSweepRequest) {
	defer func() {
		if r := recover(); r != nil {
			rt.logger.Printf("[hint] Panic in run loop: %v", r)
			rt.mu.Lock()
			rt.state.Status = "failed"
			rt.state.Error = fmt.Sprintf("panic: %v", r)
			rt.mu.Unlock()
			rt.stateCond.Broadcast()
		}
	}()

	// Get replay case
	if rt.sceneGetter == nil {
		rt.failWithError("replay case getter not configured")
		return
	}

	scene, err := rt.sceneGetter.GetScene(req.ReplayCaseID)
	if err != nil {
		rt.failWithError(fmt.Sprintf("failed to get replay case: %v", err))
		return
	}

	// Load current parameters (prefer immutable recommended params, fall back to legacy)
	currentParams := make(map[string]float64)
	paramsSource := scene.RecommendedParams
	if len(paramsSource) == 0 {
		paramsSource = scene.OptimalParamsJSON
	}
	if len(paramsSource) > 0 {
		if err := json.Unmarshal(paramsSource, &currentParams); err != nil {
			rt.logger.Printf("[hint] Failed to parse recommended params, using defaults: %v", err)
		}
	}

	// If no current params, use midpoints
	if len(currentParams) == 0 {
		for _, param := range req.Params {
			currentParams[param.Name] = (param.Start + param.End) / 2
		}
	}

	// Initialize bounds from request params
	bounds := make(map[string][2]float64)
	for _, param := range req.Params {
		bounds[param.Name] = [2]float64{param.Start, param.End}
	}

	// Main round loop
	for {
		// Check if we should continue (TotalRounds may be dynamically increased)
		rt.mu.Lock()
		currentRound := rt.state.CurrentRound + 1
		totalRounds := rt.state.TotalRounds
		rt.mu.Unlock()

		if currentRound > totalRounds {
			break
		}

		rt.logger.Printf("[hint] Starting round %d of %d", currentRound, totalRounds)

		// Update round number
		rt.mu.Lock()
		rt.state.CurrentRound = currentRound
		rt.mu.Unlock()

		// Run this round
		bestParams, bestScore, err := rt.runRound(ctx, req, scene, currentRound, currentParams, bounds)
		if err != nil {
			if ctx.Err() != nil {
				rt.failWithError("context cancelled")
			} else {
				rt.failWithError(fmt.Sprintf("round %d failed: %v", currentRound, err))
			}
			return
		}

		// Update current params and narrow bounds for next round
		if bestParams != nil {
			currentParams = bestParams

			// Narrow bounds: reduce range by 50% around best value
			for paramName, bestValue := range bestParams {
				if oldBounds, ok := bounds[paramName]; ok {
					rangeSize := oldBounds[1] - oldBounds[0]
					newRange := rangeSize * 0.5
					newStart := bestValue - newRange/2
					newEnd := bestValue + newRange/2

					// Clamp to original bounds
					if newStart < oldBounds[0] {
						newStart = oldBounds[0]
					}
					if newEnd > oldBounds[1] {
						newEnd = oldBounds[1]
					}

					bounds[paramName] = [2]float64{newStart, newEnd}
					rt.logger.Printf("[hint] Narrowed bounds for %s: [%.3f, %.3f] -> [%.3f, %.3f]",
						paramName, oldBounds[0], oldBounds[1], newStart, newEnd)
				}
			}
		}

		// Update round result on existing entry (appended at start of runRound)
		rt.mu.Lock()
		if idx := len(rt.state.RoundHistory) - 1; idx >= 0 {
			rt.state.RoundHistory[idx].BestScore = bestScore
			rt.state.RoundHistory[idx].BestParams = bestParams
		}
		rt.mu.Unlock()
	}

	// Apply final best params
	if len(currentParams) > 0 {
		paramsJSON, err := json.Marshal(currentParams)
		if err != nil {
			rt.failWithError(fmt.Sprintf("failed to marshal final params: %v", err))
			return
		}

		if rt.sceneStore != nil {
			if err := rt.sceneStore.SetOptimalParams(req.ReplayCaseID, paramsJSON); err != nil {
				rt.logger.Printf("[hint] Failed to persist optimal params: %v", err)
			} else {
				rt.logger.Printf("[hint] Applied optimal params: %s", paramsJSON)
			}
		}

		rt.mu.Lock()
		rt.state.Recommendation = make(map[string]interface{})
		for k, v := range currentParams {
			rt.state.Recommendation[k] = v
		}
		rt.mu.Unlock()
	}

	// Complete
	rt.mu.Lock()
	rt.state.Status = string(SweepStatusComplete)
	roundHistoryCopy := make([]HINTRound, len(rt.state.RoundHistory))
	copy(roundHistoryCopy, rt.state.RoundHistory)
	rt.mu.Unlock()
	rt.stateCond.Broadcast()

	if rt.persister != nil {
		recJSON, err := json.Marshal(currentParams)
		if err != nil {
			rt.logger.Printf("[hint] Failed to marshal recommendation: %v", err)
			recJSON = []byte("{}")
		}
		roundJSON, err := json.Marshal(roundHistoryCopy)
		if err != nil {
			rt.logger.Printf("[hint] Failed to marshal round history: %v", err)
			roundJSON = []byte("[]")
		}
		now := time.Now()
		if err := rt.persister.SaveSweepComplete(rt.sweepID, "completed", nil, recJSON, roundJSON, now, "", nil, nil, nil, "", ""); err != nil {
			rt.logger.Printf("[hint] Failed to persist sweep completion: %v", err)
		}
	}

	rt.logger.Printf("[hint] HINT tuning completed successfully")
}

// runRound executes a single HINT round.
func (rt *HINTTuner) runRound(ctx context.Context, req HINTSweepRequest, scene *HINTScene, round int, currentParams map[string]float64, bounds map[string][2]float64) (map[string]float64, float64, error) {
	// Step 1: Create reference run
	rt.setStatus("running_reference")

	rt.logger.Printf("[hint] Creating reference run with current params")

	paramsJSON, err := json.Marshal(currentParams)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal params: %w", err)
	}

	if rt.runCreator == nil {
		return nil, 0, fmt.Errorf("run creator not configured")
	}

	runID, err := rt.runCreator.CreateSweepRun(scene.SensorID, scene.PCAPFile, paramsJSON, scenePCAPStart(scene), scenePCAPDuration(scene))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create reference run: %w", err)
	}

	rt.logger.Printf("[hint] Created reference run: %s", runID)

	// Set as scene's reference run
	if err := rt.sceneGetter.SetReferenceRun(req.ReplayCaseID, runID); err != nil {
		return nil, 0, fmt.Errorf("failed to set reference run: %w", err)
	}

	rt.mu.Lock()
	rt.state.ReferenceRunID = runID
	// Append round entry now so it's available for carry-over and label updates.
	rt.state.RoundHistory = append(rt.state.RoundHistory, HINTRound{
		Round:          round,
		ReferenceRunID: runID,
	})
	rt.mu.Unlock()

	// Step 2: Carry over labels if this is not the first round
	carriedOver := 0
	rt.mu.RLock()
	prevRunID := ""
	if round > 1 && req.CarryOverLabels && len(rt.state.RoundHistory) > 1 {
		prevRunID = rt.state.RoundHistory[len(rt.state.RoundHistory)-2].ReferenceRunID
	}
	rt.mu.RUnlock()
	if prevRunID != "" {
		rt.logger.Printf("[hint] Carrying over labels from previous run %s", prevRunID)
		carriedOver, err = rt.carryOverLabels(prevRunID, runID)
		if err != nil {
			rt.logger.Printf("[hint] Failed to carry over labels: %v", err)
		} else {
			rt.logger.Printf("[hint] Carried over %d labels", carriedOver)
			rt.mu.Lock()
			rt.state.LabelsCarriedOver = carriedOver
			rt.mu.Unlock()
		}
	}

	// Step 3: Wait for labels
	rt.setStatus("awaiting_labels")

	rt.logger.Printf("[hint] Awaiting labels (threshold: %.1f%%, no deadline — continue when ready)", req.MinLabelThreshold*100)

	if err := rt.waitForLabels(ctx, runID, req.MinLabelThreshold); err != nil {
		return nil, 0, fmt.Errorf("label waiting failed: %w", err)
	}

	// Record label completion time on current round entry
	now := time.Now()
	rt.mu.Lock()
	if idx := len(rt.state.RoundHistory) - 1; idx >= 0 {
		rt.state.RoundHistory[idx].LabelledAt = &now
		rt.state.RoundHistory[idx].LabelProgress = rt.state.LabelProgress
		rt.state.RoundHistory[idx].LabelsCarriedOver = carriedOver
	}
	rt.mu.Unlock()

	// Step 4: Run auto-tune sweep
	rt.mu.Lock()
	rt.state.Status = "running_sweep"
	rt.mu.Unlock()
	rt.stateCond.Broadcast()
	rt.mu.Lock()
	sweepDuration := rt.state.NextSweepDuration
	if sweepDuration == 0 {
		sweepDuration = 60 // default
	}
	deadline := time.Now().Add(time.Duration(sweepDuration) * time.Minute)
	rt.state.SweepDeadline = &deadline
	rt.mu.Unlock()

	rt.logger.Printf("[hint] Running auto-tune sweep (duration: %d minutes)", sweepDuration)

	if rt.autoTuner == nil {
		return nil, 0, fmt.Errorf("auto-tuner not configured")
	}

	autoTuneReq := rt.buildAutoTuneRequest(bounds, req, scene, round)

	if err := rt.autoTuner.Start(ctx, autoTuneReq); err != nil {
		return nil, 0, fmt.Errorf("failed to start auto-tune: %w", err)
	}

	finalState, err := rt.waitForAutoTuneComplete(ctx, deadline)
	if err != nil {
		return nil, 0, fmt.Errorf("auto-tune failed: %w", err)
	}

	if finalState.Status != SweepStatusComplete {
		return nil, 0, fmt.Errorf("auto-tune did not complete successfully: %s", finalState.Status)
	}

	// Extract best result — only keep actual sweep param names
	// (Recommendation also contains metric keys like acceptance_rate,
	// alignment_deg, etc. which must not be passed to the next round.)
	var bestParams map[string]float64
	var bestScore float64

	if finalState.Recommendation != nil {
		// Build set of valid param names from the request.
		paramNames := make(map[string]struct{}, len(req.Params))
		for _, p := range req.Params {
			paramNames[p.Name] = struct{}{}
		}

		bestParams = make(map[string]float64)
		for k, v := range finalState.Recommendation {
			if k == "score" {
				if fv, ok := v.(float64); ok {
					bestScore = fv
				}
				continue
			}
			if _, ok := paramNames[k]; !ok {
				continue // skip metric keys
			}
			if fv, ok := v.(float64); ok {
				bestParams[k] = fv
			}
		}
	}

	rt.logger.Printf("[hint] Round %d complete: score=%.4f, params=%v", round, bestScore, bestParams)

	return bestParams, bestScore, nil
}

// carryOverLabels transfers labels from previous run to new run based on temporal overlap.
func (rt *HINTTuner) carryOverLabels(prevRunID, newRunID string) (int, error) {
	if rt.labelQuerier == nil {
		return 0, fmt.Errorf("label querier not configured")
	}

	// Get tracks from both runs
	prevTracks, err := rt.labelQuerier.GetRunTracks(prevRunID)
	if err != nil {
		return 0, fmt.Errorf("failed to get previous tracks: %w", err)
	}

	newTracks, err := rt.labelQuerier.GetRunTracks(newRunID)
	if err != nil {
		return 0, fmt.Errorf("failed to get new tracks: %w", err)
	}

	carried := 0

	// For each labelled track in previous run, find best match in new run
	for _, prevTrack := range prevTracks {
		if prevTrack.UserLabel == "" {
			continue // not labelled
		}

		var bestMatch *HINTRunTrack
		var bestIoU float64

		for i := range newTracks {
			newTrack := &newTracks[i]
			iou := temporalIoU(prevTrack.StartUnixNanos, prevTrack.EndUnixNanos, newTrack.StartUnixNanos, newTrack.EndUnixNanos)
			if iou > bestIoU {
				bestIoU = iou
				bestMatch = newTrack
			}
		}

		// Carry over if IoU >= 0.5, using IoU as confidence
		if bestMatch != nil && bestIoU >= 0.5 {
			if err := rt.labelQuerier.UpdateTrackLabel(newRunID, bestMatch.TrackID, prevTrack.UserLabel, prevTrack.QualityLabel, float32(bestIoU), "hint-carryover", "carried_over"); err != nil {
				rt.logger.Printf("[hint] Failed to carry over label for track %s: %v", bestMatch.TrackID, err)
			} else {
				carried++
			}
		}
	}

	return carried, nil
}

// buildAutoTuneRequest constructs an AutoTuneRequest for the current round.
func (rt *HINTTuner) buildAutoTuneRequest(bounds map[string][2]float64, req HINTSweepRequest, scene *HINTScene, round int) AutoTuneRequest {
	// Build params with current bounds
	params := make([]SweepParam, 0, len(req.Params))
	for _, param := range req.Params {
		if newBounds, ok := bounds[param.Name]; ok {
			params = append(params, SweepParam{
				Name:  param.Name,
				Type:  param.Type,
				Start: newBounds[0],
				End:   newBounds[1],
			})
		} else {
			params = append(params, param)
		}
	}

	// Build base request — always use the scene's PCAP so the exploration
	// sweep replays the same capture window as the reference run.
	autoReq := AutoTuneRequest{
		ReplayCaseID:       req.ReplayCaseID,
		Objective:          "ground_truth",
		Params:             params,
		ValuesPerParam:     req.ValuesPerParam,
		TopK:               req.TopK,
		MaxRounds:          1,
		Iterations:         req.Iterations,
		Interval:           req.Interval,
		SettleTime:         req.SettleTime,
		Seed:               req.Seed,
		SettleMode:         req.SettleMode,
		GroundTruthWeights: req.GroundTruthWeights,
		AcceptanceCriteria: req.AcceptanceCriteria,
		DataSource:         "pcap",
		PCAPFile:           scene.PCAPFile,
		PCAPStartSecs:      scenePCAPStart(scene),
		PCAPDurationSecs:   scenePCAPDuration(scene),
	}

	// Apply defaults
	if autoReq.GroundTruthWeights == nil {
		defaults := DefaultGroundTruthWeights()
		autoReq.GroundTruthWeights = &defaults
	}

	// Adjust weights for first round: favour recall over precision
	if round == 1 {
		weights := *autoReq.GroundTruthWeights
		weights.DetectionRate *= 1.5
		weights.FalsePositives *= 0.5
		autoReq.GroundTruthWeights = &weights
		rt.logger.Printf("[hint] Adjusted weights for round 1: DetectionRate x1.5, FalsePositives x0.5")
	}

	return autoReq
}
