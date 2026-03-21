package sweep

import (
	"fmt"
)

// GetState returns the current state (interface for compatibility).
func (rt *HINTTuner) GetState() interface{} {
	return rt.GetHINTState()
}

// GetHINTState returns a deep copy of the current HINT state.
func (rt *HINTTuner) GetHINTState() HINTState {
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

	// Deep copy MinClassCoverage
	if state.MinClassCoverage != nil {
		state.MinClassCoverage = make(map[string]int)
		for k, v := range rt.state.MinClassCoverage {
			state.MinClassCoverage[k] = v
		}
	}

	// Deep copy round history
	if len(state.RoundHistory) > 0 {
		state.RoundHistory = make([]HINTRound, len(rt.state.RoundHistory))
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

// Stop cancels the running HINT session.
func (rt *HINTTuner) Stop() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.cancel != nil {
		rt.cancel()
		rt.cancel = nil
	}

	if rt.autoTuner != nil {
		rt.autoTuner.Stop()
	}

	rt.logger.Printf("[hint] Stopped HINT tuning session")
}

// ContinueFromLabels signals the tuner to proceed from awaiting_labels to running_sweep.
func (rt *HINTTuner) ContinueFromLabels(nextDurationMins int, addRound bool) error {
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

		// Class coverage gate (optional — skip if no requirements set)
		if len(rt.state.MinClassCoverage) > 0 {
			for className, minCount := range rt.state.MinClassCoverage {
				actual := byClass[className]
				if actual < minCount {
					return fmt.Errorf("class coverage not met: %s has %d labelled, need %d", className, actual, minCount)
				}
			}
		}

		// Temporal spread gate (optional — skip if zero)
		if rt.state.MinTemporalSpreadSecs > 0 && rt.labelQuerier != nil {
			tracks, err := rt.labelQuerier.GetRunTracks(rt.state.ReferenceRunID)
			if err != nil {
				return fmt.Errorf("failed to query tracks for temporal spread: %w", err)
			}
			var minStart, maxEnd int64
			found := false
			for _, track := range tracks {
				if track.UserLabel == "" {
					continue // Only consider labelled tracks
				}
				if !found || track.StartUnixNanos < minStart {
					minStart = track.StartUnixNanos
				}
				if !found || track.EndUnixNanos > maxEnd {
					maxEnd = track.EndUnixNanos
				}
				found = true
			}
			if found {
				spreadSecs := (float64(maxEnd) - float64(minStart)) / 1e9
				if spreadSecs < rt.state.MinTemporalSpreadSecs {
					return fmt.Errorf("temporal spread not met: %.1fs < %.1fs required", spreadSecs, rt.state.MinTemporalSpreadSecs)
				}
			}
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
		rt.logger.Printf("[hint] Added round: now %d total rounds", rt.state.TotalRounds)
	}

	// Update next sweep duration if provided
	if nextDurationMins > 0 {
		rt.state.NextSweepDuration = nextDurationMins
		rt.logger.Printf("[hint] Updated next sweep duration: %d minutes", nextDurationMins)
	}

	// Send signal
	select {
	case rt.continueCh <- continueSignal{
		NextSweepDurationMins: nextDurationMins,
		AddRound:              addRound,
	}:
		rt.logger.Printf("[hint] Sent continue signal")
		return nil
	default:
		return fmt.Errorf("continue channel blocked")
	}
}
