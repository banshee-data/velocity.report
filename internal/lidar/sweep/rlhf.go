package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LabelProgressQuerier queries label progress for a run.
type LabelProgressQuerier interface {
	GetLabelingProgress(runID string) (total, labelled int, byClass map[string]int, err error)
	GetRunTracks(runID string) ([]RLHFRunTrack, error)
	UpdateTrackLabel(runID, trackID, userLabel, qualityLabel string, confidence float32, labelerID string) error
}

// RLHFRunTrack is a minimal track representation for label carryover.
type RLHFRunTrack struct {
	TrackID        string
	StartUnixNanos int64
	EndUnixNanos   int64
	UserLabel      string
	QualityLabel   string
}

// SceneGetter retrieves scene data and sets reference runs.
type SceneGetter interface {
	GetScene(sceneID string) (*RLHFScene, error)
	SetReferenceRun(sceneID, runID string) error
}

// RLHFScene is a minimal scene representation.
type RLHFScene struct {
	SceneID           string
	SensorID          string
	PCAPFile          string
	PCAPStartSecs     *float64
	PCAPDurationSecs  *float64
	ReferenceRunID    string
	OptimalParamsJSON json.RawMessage
}

// ReferenceRunCreator creates analysis runs for RLHF reference.
type ReferenceRunCreator interface {
	CreateSweepRun(sensorID, pcapFile string, paramsJSON json.RawMessage) (string, error)
}

// RLHFSweepRequest defines the request for an RLHF tuning sweep.
type RLHFSweepRequest struct {
	SceneID            string              `json:"scene_id"`
	NumRounds          int                 `json:"num_rounds"`
	RoundDurations     []int               `json:"round_durations"`
	Params             []SweepParam        `json:"params"`
	ValuesPerParam     int                 `json:"values_per_param"`
	TopK               int                 `json:"top_k"`
	Iterations         int                 `json:"iterations"`
	Interval           string              `json:"interval"`
	SettleTime         string              `json:"settle_time"`
	Seed               string              `json:"seed"`
	SettleMode         string              `json:"settle_mode"`
	GroundTruthWeights *GroundTruthWeights `json:"ground_truth_weights"`
	AcceptanceCriteria *AcceptanceCriteria `json:"acceptance_criteria"`
	MinLabelThreshold  float64             `json:"min_label_threshold"`
	CarryOverLabels    bool                `json:"carry_over_labels"`
}

// RLHFState represents the current state of an RLHF tuning session.
type RLHFState struct {
	Status            string                 `json:"status"` // "idle","running_reference","awaiting_labels","running_sweep","completed","failed"
	Mode              string                 `json:"mode"`   // always "rlhf"
	CurrentRound      int                    `json:"current_round"`
	TotalRounds       int                    `json:"total_rounds"`
	ReferenceRunID    string                 `json:"reference_run_id,omitempty"`
	LabelProgress     *LabelProgress         `json:"label_progress,omitempty"`
	LabelDeadline     *time.Time             `json:"label_deadline,omitempty"`
	SweepDeadline     *time.Time             `json:"sweep_deadline,omitempty"`
	AutoTuneState     *AutoTuneState         `json:"auto_tune_state,omitempty"`
	Recommendation    map[string]interface{} `json:"recommendation,omitempty"`
	RoundHistory      []RLHFRound            `json:"round_history"`
	Error             string                 `json:"error,omitempty"`
	MinLabelThreshold float64                `json:"min_label_threshold"`
	LabelsCarriedOver int                    `json:"labels_carried_over"`
	NextSweepDuration int                    `json:"next_sweep_duration_mins"`
}

// LabelProgress tracks labelling progress for a reference run.
type LabelProgress struct {
	Total    int            `json:"total"`
	Labelled int            `json:"labelled"`
	Pct      float64        `json:"progress_pct"`
	ByClass  map[string]int `json:"by_class"`
}

// RLHFRound records the results of a single RLHF round.
type RLHFRound struct {
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

// RLHFTuner orchestrates human-in-the-loop parameter optimisation.
type RLHFTuner struct {
	mu sync.RWMutex

	state  RLHFState
	cancel context.CancelFunc

	// Dependencies (injected)
	autoTuner         *AutoTuner
	labelQuerier      LabelProgressQuerier
	sceneGetter       SceneGetter
	sceneStore        SceneStoreSaver
	runCreator        ReferenceRunCreator
	persister         SweepPersister
	groundTruthScorer func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error)

	// Internal coordination
	continueCh chan continueSignal
	sweepID    string

	// For testability
	pollInterval time.Duration
}

// continueSignal is sent to proceed from awaiting_labels to running_sweep.
type continueSignal struct {
	NextSweepDurationMins int  `json:"next_sweep_duration_mins"`
	AddRound              bool `json:"add_round"`
}

// NewRLHFTuner creates a new RLHF tuner with the given AutoTuner backend.
func NewRLHFTuner(autoTuner *AutoTuner) *RLHFTuner {
	return &RLHFTuner{
		autoTuner:    autoTuner,
		continueCh:   make(chan continueSignal, 1),
		pollInterval: 10 * time.Second,
		state: RLHFState{
			Status:       "idle",
			Mode:         "rlhf",
			RoundHistory: []RLHFRound{},
		},
	}
}

// SetLabelQuerier sets the label progress querier dependency.
func (rt *RLHFTuner) SetLabelQuerier(q LabelProgressQuerier) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.labelQuerier = q
}

// SetSceneGetter sets the scene getter dependency.
func (rt *RLHFTuner) SetSceneGetter(g SceneGetter) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.sceneGetter = g
}

// SetSceneStore sets the scene store dependency.
func (rt *RLHFTuner) SetSceneStore(s SceneStoreSaver) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.sceneStore = s
}

// SetRunCreator sets the reference run creator dependency.
func (rt *RLHFTuner) SetRunCreator(c ReferenceRunCreator) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.runCreator = c
}

// SetPersister sets the persistence layer.
func (rt *RLHFTuner) SetPersister(p SweepPersister) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.persister = p
}

// SetGroundTruthScorer sets the ground truth scoring function.
func (rt *RLHFTuner) SetGroundTruthScorer(scorer func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error)) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.groundTruthScorer = scorer
}

// Start initiates an RLHF tuning session.
func (rt *RLHFTuner) Start(ctx context.Context, reqInterface interface{}) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Parse request
	var req RLHFSweepRequest
	switch v := reqInterface.(type) {
	case RLHFSweepRequest:
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
	if req.SceneID == "" {
		return fmt.Errorf("scene_id is required")
	}
	if req.NumRounds < 1 || req.NumRounds > 10 {
		return fmt.Errorf("num_rounds must be between 1 and 10, got %d", req.NumRounds)
	}
	if len(req.Params) == 0 {
		return fmt.Errorf("params cannot be empty")
	}
	if len(req.Params) > 20 {
		return fmt.Errorf("too many parameters (max 20, got %d)", len(req.Params))
	}

	// Apply defaults
	if req.MinLabelThreshold == 0 {
		req.MinLabelThreshold = 0.9
	}
	if req.Iterations == 0 {
		req.Iterations = 10
	}
	if req.TopK == 0 {
		req.TopK = 3
	}
	if req.ValuesPerParam == 0 {
		req.ValuesPerParam = 5
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
	rt.state = RLHFState{
		Status:            "running_reference",
		Mode:              "rlhf",
		CurrentRound:      0,
		TotalRounds:       req.NumRounds,
		RoundHistory:      []RLHFRound{},
		MinLabelThreshold: req.MinLabelThreshold,
		NextSweepDuration: getDuration(req.RoundDurations, 0),
	}

	// Persist start if persister available
	if rt.persister != nil {
		reqJSON, err := json.Marshal(req)
		if err != nil {
			log.Printf("[rlhf] WARNING: Failed to marshal request for persistence: %v", err)
			reqJSON = []byte("{}")
		}
		if err := rt.persister.SaveSweepStart(rt.sweepID, "", "rlhf", reqJSON, time.Now(), "ground_truth", ObjectiveVersion); err != nil {
			log.Printf("[rlhf] Failed to persist sweep start: %v", err)
		}
	}

	// Launch background goroutine
	runCtx, cancel := context.WithCancel(ctx)
	rt.cancel = cancel
	go rt.run(runCtx, req)

	log.Printf("[rlhf] Started RLHF tuning session %s for scene %s (%d rounds)", rt.sweepID, req.SceneID, req.NumRounds)
	return nil
}

// GetState returns the current state (interface for compatibility).
func (rt *RLHFTuner) GetState() interface{} {
	return rt.GetRLHFState()
}

// GetRLHFState returns a deep copy of the current RLHF state.
func (rt *RLHFTuner) GetRLHFState() RLHFState {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Deep copy state
	state := rt.state

	// Deep copy label progress
	if state.LabelProgress != nil {
		lpCopy := *state.LabelProgress
		if lpCopy.ByClass != nil {
			lpCopy.ByClass = make(map[string]int)
			for k, v := range state.LabelProgress.ByClass {
				lpCopy.ByClass[k] = v
			}
		}
		state.LabelProgress = &lpCopy
	}

	// Deep copy deadlines
	if state.LabelDeadline != nil {
		t := *state.LabelDeadline
		state.LabelDeadline = &t
	}
	if state.SweepDeadline != nil {
		t := *state.SweepDeadline
		state.SweepDeadline = &t
	}

	// Deep copy auto-tune state
	if state.AutoTuneState != nil {
		atsCopy := *state.AutoTuneState
		state.AutoTuneState = &atsCopy
	}

	// Deep copy recommendation
	if state.Recommendation != nil {
		state.Recommendation = make(map[string]interface{})
		for k, v := range rt.state.Recommendation {
			state.Recommendation[k] = v
		}
	}

	// Deep copy round history
	if len(state.RoundHistory) > 0 {
		state.RoundHistory = make([]RLHFRound, len(rt.state.RoundHistory))
		for i, round := range rt.state.RoundHistory {
			roundCopy := round
			if round.LabelledAt != nil {
				t := *round.LabelledAt
				roundCopy.LabelledAt = &t
			}
			if round.LabelProgress != nil {
				lpCopy := *round.LabelProgress
				if lpCopy.ByClass != nil {
					lpCopy.ByClass = make(map[string]int)
					for k, v := range round.LabelProgress.ByClass {
						lpCopy.ByClass[k] = v
					}
				}
				roundCopy.LabelProgress = &lpCopy
			}
			if round.BestParams != nil {
				roundCopy.BestParams = make(map[string]float64)
				for k, v := range round.BestParams {
					roundCopy.BestParams[k] = v
				}
			}
			state.RoundHistory[i] = roundCopy
		}
	}

	return state
}

// Stop cancels the running RLHF session.
func (rt *RLHFTuner) Stop() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.cancel != nil {
		rt.cancel()
		rt.cancel = nil
	}

	if rt.autoTuner != nil {
		rt.autoTuner.Stop()
	}

	log.Printf("[rlhf] Stopped RLHF tuning session")
}

// ContinueFromLabels signals the tuner to proceed from awaiting_labels to running_sweep.
func (rt *RLHFTuner) ContinueFromLabels(nextDurationMins int, addRound bool) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Check state
	if rt.state.Status != "awaiting_labels" {
		return fmt.Errorf("cannot continue: not in awaiting_labels state (current: %s)", rt.state.Status)
	}

	// Check label threshold
	if rt.labelQuerier != nil && rt.state.ReferenceRunID != "" {
		total, labelled, byClass, err := rt.labelQuerier.GetLabelingProgress(rt.state.ReferenceRunID)
		if err != nil {
			return fmt.Errorf("failed to query label progress: %w", err)
		}

		pct := 0.0
		if total > 0 {
			pct = float64(labelled) / float64(total)
		}

		if pct < rt.state.MinLabelThreshold {
			return fmt.Errorf("label threshold not met: %.1f%% < %.1f%%", pct*100, rt.state.MinLabelThreshold*100)
		}

		// Update progress one more time
		rt.state.LabelProgress = &LabelProgress{
			Total:    total,
			Labelled: labelled,
			Pct:      pct * 100,
			ByClass:  byClass,
		}
	}

	// Update total rounds if requested
	if addRound {
		rt.state.TotalRounds++
		log.Printf("[rlhf] Added round: now %d total rounds", rt.state.TotalRounds)
	}

	// Update next sweep duration if provided
	if nextDurationMins > 0 {
		rt.state.NextSweepDuration = nextDurationMins
		log.Printf("[rlhf] Updated next sweep duration: %d minutes", nextDurationMins)
	}

	// Send signal
	select {
	case rt.continueCh <- continueSignal{
		NextSweepDurationMins: nextDurationMins,
		AddRound:              addRound,
	}:
		log.Printf("[rlhf] Sent continue signal")
		return nil
	default:
		return fmt.Errorf("continue channel blocked")
	}
}

// run is the main RLHF orchestration loop.
func (rt *RLHFTuner) run(ctx context.Context, req RLHFSweepRequest) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[rlhf] Panic in run loop: %v", r)
			rt.mu.Lock()
			rt.state.Status = "failed"
			rt.state.Error = fmt.Sprintf("panic: %v", r)
			rt.mu.Unlock()
		}
	}()

	// Get scene
	if rt.sceneGetter == nil {
		rt.failWithError("scene getter not configured")
		return
	}

	scene, err := rt.sceneGetter.GetScene(req.SceneID)
	if err != nil {
		rt.failWithError(fmt.Sprintf("failed to get scene: %v", err))
		return
	}

	// Load current parameters
	currentParams := make(map[string]float64)
	if len(scene.OptimalParamsJSON) > 0 {
		if err := json.Unmarshal(scene.OptimalParamsJSON, &currentParams); err != nil {
			log.Printf("[rlhf] Failed to parse optimal params, using defaults: %v", err)
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

		log.Printf("[rlhf] Starting round %d of %d", currentRound, totalRounds)

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
					log.Printf("[rlhf] Narrowed bounds for %s: [%.3f, %.3f] -> [%.3f, %.3f]",
						paramName, oldBounds[0], oldBounds[1], newStart, newEnd)
				}
			}
		}

		// Record round result
		rt.mu.Lock()
		rt.state.RoundHistory = append(rt.state.RoundHistory, RLHFRound{
			Round:      currentRound,
			BestScore:  bestScore,
			BestParams: bestParams,
		})
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
			if err := rt.sceneStore.SetOptimalParams(req.SceneID, paramsJSON); err != nil {
				log.Printf("[rlhf] Failed to persist optimal params: %v", err)
			} else {
				log.Printf("[rlhf] Applied optimal params: %s", paramsJSON)
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
	rt.mu.Unlock()

	if rt.persister != nil {
		recJSON, _ := json.Marshal(currentParams)
		roundJSON, _ := json.Marshal(rt.state.RoundHistory)
		now := time.Now()
		if err := rt.persister.SaveSweepComplete(rt.sweepID, "completed", nil, recJSON, roundJSON, now, "", nil, nil, nil, "", ""); err != nil {
			log.Printf("[rlhf] Failed to persist sweep completion: %v", err)
		}
	}

	log.Printf("[rlhf] RLHF tuning completed successfully")
}

// runRound executes a single RLHF round.
func (rt *RLHFTuner) runRound(ctx context.Context, req RLHFSweepRequest, scene *RLHFScene, round int, currentParams map[string]float64, bounds map[string][2]float64) (map[string]float64, float64, error) {
	// Phase 1: Create reference run
	rt.mu.Lock()
	rt.state.Status = "running_reference"
	rt.mu.Unlock()

	log.Printf("[rlhf] Creating reference run with current params")

	paramsJSON, err := json.Marshal(currentParams)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal params: %w", err)
	}

	if rt.runCreator == nil {
		return nil, 0, fmt.Errorf("run creator not configured")
	}

	runID, err := rt.runCreator.CreateSweepRun(scene.SensorID, scene.PCAPFile, paramsJSON)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create reference run: %w", err)
	}

	log.Printf("[rlhf] Created reference run: %s", runID)

	// Set as scene's reference run
	if err := rt.sceneGetter.SetReferenceRun(req.SceneID, runID); err != nil {
		return nil, 0, fmt.Errorf("failed to set reference run: %w", err)
	}

	rt.mu.Lock()
	rt.state.ReferenceRunID = runID
	rt.mu.Unlock()

	// Phase 2: Carry over labels if this is not the first round
	carriedOver := 0
	if round > 1 && req.CarryOverLabels && len(rt.state.RoundHistory) > 0 {
		prevRunID := rt.state.RoundHistory[len(rt.state.RoundHistory)-1].ReferenceRunID
		if prevRunID != "" {
			log.Printf("[rlhf] Carrying over labels from previous run %s", prevRunID)
			carriedOver, err = rt.carryOverLabels(prevRunID, runID)
			if err != nil {
				log.Printf("[rlhf] Failed to carry over labels: %v", err)
			} else {
				log.Printf("[rlhf] Carried over %d labels", carriedOver)
				rt.mu.Lock()
				rt.state.LabelsCarriedOver = carriedOver
				rt.mu.Unlock()
			}
		}
	}

	// Phase 3: Wait for labels
	rt.mu.Lock()
	rt.state.Status = "awaiting_labels"
	rt.mu.Unlock()

	durationMins := getDuration(req.RoundDurations, round-1)
	log.Printf("[rlhf] Awaiting labels for %d minutes (threshold: %.1f%%)", durationMins, req.MinLabelThreshold*100)

	if err := rt.waitForLabelsOrDeadline(ctx, runID, durationMins, req.MinLabelThreshold); err != nil {
		return nil, 0, fmt.Errorf("label waiting failed: %w", err)
	}

	// Record label completion time
	now := time.Now()
	rt.mu.Lock()
	if len(rt.state.RoundHistory) > 0 {
		rt.state.RoundHistory[len(rt.state.RoundHistory)-1].LabelledAt = &now
		rt.state.RoundHistory[len(rt.state.RoundHistory)-1].LabelProgress = rt.state.LabelProgress
		rt.state.RoundHistory[len(rt.state.RoundHistory)-1].LabelsCarriedOver = carriedOver
		rt.state.RoundHistory[len(rt.state.RoundHistory)-1].ReferenceRunID = runID
	}
	rt.mu.Unlock()

	// Phase 4: Run auto-tune sweep
	rt.mu.Lock()
	rt.state.Status = "running_sweep"
	sweepDuration := rt.state.NextSweepDuration
	if sweepDuration == 0 {
		sweepDuration = 60 // default
	}
	deadline := time.Now().Add(time.Duration(sweepDuration) * time.Minute)
	rt.state.SweepDeadline = &deadline
	rt.mu.Unlock()

	log.Printf("[rlhf] Running auto-tune sweep (duration: %d minutes)", sweepDuration)

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

	// Extract best result
	var bestParams map[string]float64
	var bestScore float64

	if finalState.Recommendation != nil {
		bestParams = make(map[string]float64)
		for k, v := range finalState.Recommendation {
			if k == "score" {
				if fv, ok := v.(float64); ok {
					bestScore = fv
				}
				continue
			}
			if fv, ok := v.(float64); ok {
				bestParams[k] = fv
			}
		}
	}

	log.Printf("[rlhf] Round %d complete: score=%.4f, params=%v", round, bestScore, bestParams)

	return bestParams, bestScore, nil
}

// getDuration returns the duration for the given round index.
func getDuration(durations []int, index int) int {
	if len(durations) == 0 {
		return 60 // default
	}
	if index < len(durations) {
		return durations[index]
	}
	return durations[len(durations)-1]
}

// waitForLabelsOrDeadline waits for labelling progress to reach threshold.
func (rt *RLHFTuner) waitForLabelsOrDeadline(ctx context.Context, runID string, durationMins int, threshold float64) error {
	deadline := time.Now().Add(time.Duration(durationMins) * time.Minute)

	rt.mu.Lock()
	rt.state.LabelDeadline = &deadline
	rt.mu.Unlock()

	ticker := time.NewTicker(rt.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case sig := <-rt.continueCh:
			// Manual continue signal
			log.Printf("[rlhf] Received continue signal (next_duration=%d, add_round=%v)", sig.NextSweepDurationMins, sig.AddRound)
			if sig.NextSweepDurationMins > 0 {
				rt.mu.Lock()
				rt.state.NextSweepDuration = sig.NextSweepDurationMins
				rt.mu.Unlock()
			}
			return nil

		case <-ticker.C:
			// Poll label progress
			if rt.labelQuerier == nil {
				continue
			}

			total, labelled, byClass, err := rt.labelQuerier.GetLabelingProgress(runID)
			if err != nil {
				log.Printf("[rlhf] Failed to query label progress: %v", err)
				continue
			}

			pct := 0.0
			if total > 0 {
				pct = float64(labelled) / float64(total)
			}

			rt.mu.Lock()
			rt.state.LabelProgress = &LabelProgress{
				Total:    total,
				Labelled: labelled,
				Pct:      pct * 100,
				ByClass:  byClass,
			}
			rt.mu.Unlock()

			log.Printf("[rlhf] Label progress: %d/%d (%.1f%%)", labelled, total, pct*100)

			// Check if threshold met and deadline passed
			if pct >= threshold && time.Now().After(deadline) {
				log.Printf("[rlhf] Threshold met and deadline passed, proceeding")
				return nil
			}
		}

		// Check if deadline passed without meeting threshold
		if time.Now().After(deadline) {
			rt.mu.RLock()
			currentPct := 0.0
			if rt.state.LabelProgress != nil {
				currentPct = rt.state.LabelProgress.Pct / 100
			}
			rt.mu.RUnlock()

			if currentPct < threshold {
				return fmt.Errorf("deadline expired with only %.1f%% labelled (threshold: %.1f%%)", currentPct*100, threshold*100)
			}
			return nil
		}
	}
}

// carryOverLabels transfers labels from previous run to new run based on temporal overlap.
func (rt *RLHFTuner) carryOverLabels(prevRunID, newRunID string) (int, error) {
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

		var bestMatch *RLHFRunTrack
		var bestIoU float64

		for i := range newTracks {
			newTrack := &newTracks[i]
			iou := temporalIoU(prevTrack.StartUnixNanos, prevTrack.EndUnixNanos, newTrack.StartUnixNanos, newTrack.EndUnixNanos)
			if iou > bestIoU {
				bestIoU = iou
				bestMatch = newTrack
			}
		}

		// Carry over if IoU >= 0.5
		if bestMatch != nil && bestIoU >= 0.5 {
			if err := rt.labelQuerier.UpdateTrackLabel(newRunID, bestMatch.TrackID, prevTrack.UserLabel, prevTrack.QualityLabel, 1.0, "rlhf-carryover"); err != nil {
				log.Printf("[rlhf] Failed to carry over label for track %s: %v", bestMatch.TrackID, err)
			} else {
				carried++
			}
		}
	}

	return carried, nil
}

// temporalIoU computes the intersection-over-union of two time intervals.
func temporalIoU(aStart, aEnd, bStart, bEnd int64) float64 {
	// Find intersection
	intStart := aStart
	if bStart > intStart {
		intStart = bStart
	}

	intEnd := aEnd
	if bEnd < intEnd {
		intEnd = bEnd
	}

	if intStart >= intEnd {
		return 0 // no overlap
	}

	intersection := float64(intEnd - intStart)

	// Find union
	unionStart := aStart
	if bStart < unionStart {
		unionStart = bStart
	}

	unionEnd := aEnd
	if bEnd > unionEnd {
		unionEnd = bEnd
	}

	union := float64(unionEnd - unionStart)

	if union == 0 {
		return 0
	}

	return intersection / union
}

// buildAutoTuneRequest constructs an AutoTuneRequest for the current round.
func (rt *RLHFTuner) buildAutoTuneRequest(bounds map[string][2]float64, req RLHFSweepRequest, scene *RLHFScene, round int) AutoTuneRequest {
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

	// Build base request
	autoReq := AutoTuneRequest{
		SceneID:            req.SceneID,
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
		log.Printf("[rlhf] Adjusted weights for round 1: DetectionRate x1.5, FalsePositives x0.5")
	}

	return autoReq
}

// waitForAutoTuneComplete polls the auto-tuner until completion or deadline.
func (rt *RLHFTuner) waitForAutoTuneComplete(ctx context.Context, deadline time.Time) (*AutoTuneState, error) {
	ticker := time.NewTicker(rt.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-ticker.C:
			state := rt.autoTuner.GetAutoTuneState()

			// Update our state with auto-tune progress
			rt.mu.Lock()
			rt.state.AutoTuneState = &state
			rt.mu.Unlock()

			// Check for completion
			if state.Status == SweepStatusComplete {
				return &state, nil
			}

			if state.Status == SweepStatusError {
				return &state, fmt.Errorf("auto-tune failed: %s", state.Error)
			}

			// Check deadline
			if time.Now().After(deadline) {
				rt.autoTuner.Stop()
				return nil, fmt.Errorf("auto-tune deadline exceeded")
			}
		}
	}
}

// failWithError sets the state to failed with the given error message.
func (rt *RLHFTuner) failWithError(errMsg string) {
	log.Printf("[rlhf] Failed: %s", errMsg)
	rt.mu.Lock()
	rt.state.Status = "failed"
	rt.state.Error = errMsg
	rt.mu.Unlock()

	if rt.persister != nil {
		now := time.Now()
		if err := rt.persister.SaveSweepComplete(rt.sweepID, "failed", nil, nil, nil, now, errMsg, nil, nil, nil, "", ""); err != nil {
			log.Printf("[rlhf] Failed to persist error: %v", err)
		}
	}
}
