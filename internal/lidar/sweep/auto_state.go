package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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

// Suspend cancels a running auto-tune and saves a checkpoint to the database
// so it can later be resumed with Resume(). The sweep status is set to "suspended".
func (at *AutoTuner) Suspend() error {
	at.mu.Lock()
	if at.state.Status != SweepStatusRunning {
		at.mu.Unlock()
		return fmt.Errorf("cannot suspend: sweep is not running (status=%s)", at.state.Status)
	}

	// Capture checkpoint data under lock
	round := checkpointRoundForSuspend(at.state)
	results := make([]ComboResult, len(at.state.Results))
	copy(results, at.state.Results)
	bounds := copyBounds(at.currentBounds)

	// Cancel the running goroutine
	if at.cancel != nil {
		at.cancel()
	}
	at.mu.Unlock()

	// Wait briefly for the run goroutine to process cancellation
	// and set its own error state; we'll override with suspended.
	time.Sleep(200 * time.Millisecond)

	// Persist checkpoint to database
	if at.persister != nil && at.sweepID != "" {
		boundsJSON, err := json.Marshal(bounds)
		if err != nil {
			at.logger.Printf("[sweep] WARNING: Failed to marshal bounds for checkpoint: %v", err)
			boundsJSON = []byte("{}")
		}

		resultsJSON, err := json.Marshal(results)
		if err != nil {
			at.logger.Printf("[sweep] WARNING: Failed to marshal results for checkpoint: %v", err)
			resultsJSON = []byte("[]")
		}

		var reqJSON json.RawMessage
		if at.lastRequest != nil {
			reqJSON, err = json.Marshal(at.lastRequest)
			if err != nil {
				at.logger.Printf("[sweep] WARNING: Failed to marshal request for checkpoint: %v", err)
			}
		}

		if err := at.persister.SaveSweepCheckpoint(at.sweepID, round, boundsJSON, resultsJSON, reqJSON); err != nil {
			at.logger.Printf("[sweep] WARNING: Failed to save checkpoint: %v", err)
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}
	}

	// Override the error status set by cancellation with suspended
	at.mu.Lock()
	at.state.Status = SweepStatusSuspended
	at.state.Error = ""
	at.mu.Unlock()

	at.logger.Printf("[sweep] Auto-tune suspended at round %d with %d results", round, len(results))
	return nil
}

// GetSuspendedSweepID returns the sweep ID if the current state is suspended,
// or empty string otherwise. Used by the HTTP handler to pass the sweep ID to Resume.
func (at *AutoTuner) GetSuspendedSweepID() string {
	at.mu.RLock()
	defer at.mu.RUnlock()
	if at.state.Status == SweepStatusSuspended {
		return at.sweepID
	}
	return ""
}

// Resume restores a suspended auto-tune from its database checkpoint and
// re-runs the remaining rounds. If sweepID is non-empty it is used to look up
// the checkpoint (allowing resume after a server restart when in-memory state
// has been lost). If sweepID is empty the in-memory suspended sweep ID is
// used instead.
func (at *AutoTuner) Resume(ctx context.Context, sweepID string) error {
	at.mu.Lock()
	if at.state.Status == SweepStatusRunning {
		at.mu.Unlock()
		return ErrSweepAlreadyRunning
	}

	// Prefer an explicit sweepID (from the DB). Fall back to in-memory state.
	if sweepID == "" {
		sweepID = at.sweepID
	}
	if sweepID == "" {
		at.mu.Unlock()
		return fmt.Errorf("no suspended sweep to resume")
	}
	at.mu.Unlock()

	if at.persister == nil {
		return fmt.Errorf("cannot resume: no persister configured")
	}

	// Load checkpoint from database
	checkpointRound, boundsJSON, resultsJSON, reqJSON, err := at.persister.LoadSweepCheckpoint(sweepID)
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Deserialise the original request
	var req AutoTuneRequest
	if len(reqJSON) > 0 {
		if err := json.Unmarshal(reqJSON, &req); err != nil {
			return fmt.Errorf("failed to unmarshal checkpoint request: %w", err)
		}
	} else if at.lastRequest != nil {
		req = *at.lastRequest
	} else {
		return fmt.Errorf("no request found in checkpoint; cannot resume")
	}

	// Deserialise accumulated results
	var priorResults []ComboResult
	if len(resultsJSON) > 0 {
		if err := json.Unmarshal(resultsJSON, &priorResults); err != nil {
			return fmt.Errorf("failed to unmarshal checkpoint results: %w", err)
		}
	}

	// Deserialize checkpoint bounds (for resuming mid-round with narrowed bounds).
	var checkpointBounds map[string][2]float64
	if len(boundsJSON) > 0 {
		if err := json.Unmarshal(boundsJSON, &checkpointBounds); err != nil {
			return fmt.Errorf("failed to unmarshal checkpoint bounds: %w", err)
		}
	}

	// Apply defaults
	req = applyAutoTuneDefaults(req)

	startRound := checkpointRound
	if startRound <= 0 {
		startRound = 1
	}
	if startRound > req.MaxRounds {
		return fmt.Errorf("checkpoint round %d > max rounds %d; nothing to resume", startRound, req.MaxRounds)
	}

	at.mu.Lock()
	now := time.Now()
	at.sweepID = sweepID
	at.cumulativeBase = len(priorResults)
	at.lastRequest = &req
	at.state = AutoTuneState{
		Status:              SweepStatusRunning,
		Mode:                "auto",
		StartedAt:           &now,
		Round:               startRound,
		TotalRounds:         req.MaxRounds,
		Results:             priorResults,
		RoundResults:        make([]RoundSummary, 0),
		CumulativeCompleted: len(priorResults),
	}
	if len(checkpointBounds) > 0 {
		at.currentBounds = copyBounds(checkpointBounds)
	} else {
		at.currentBounds = boundsFromParams(req.Params)
	}

	runCtx, cancel := context.WithCancel(ctx)
	at.cancel = cancel
	at.mu.Unlock()

	at.logger.Printf("[sweep] Auto-tune resuming from round %d with %d prior results (sweepID=%s)", startRound, len(priorResults), sweepID)

	// Run remaining rounds in background
	go at.runFromRound(runCtx, req, startRound, priorResults, checkpointBounds)

	return nil
}
