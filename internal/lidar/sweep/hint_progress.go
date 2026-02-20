package sweep

import (
	"context"
	"fmt"
	"time"
)

// setStatus updates the HINT status and broadcasts to any long-poll waiters.
// Caller must NOT hold rt.mu.
func (rt *HINTTuner) setStatus(status string) {
	rt.mu.Lock()
	rt.state.Status = status
	rt.mu.Unlock()
	rt.stateCond.Broadcast()
}

// WaitForChange blocks until the HINT state changes (status transition
// or label-progress update) compared to the caller's last-seen snapshot,
// or ctx is cancelled.  Returns the current state.
func (rt *HINTTuner) WaitForChange(ctx context.Context, lastStatus string) interface{} {
	// Use a goroutine to unblock the Cond.Wait when the context is done.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			rt.stateCond.Broadcast()
		case <-done:
		}
	}()

	// Block once and then return — any broadcast (status change OR
	// label-progress update) counts as a change the client should see.
	rt.mu.RLock()
	if rt.state.Status == lastStatus && ctx.Err() == nil {
		rt.stateCond.Wait()
	}
	rt.mu.RUnlock()
	close(done)

	return rt.GetState()
}

// NotifyLabelUpdate is called by the label-update HTTP handler to wake the
// HINT waiter immediately instead of waiting for the next poll tick.
func (rt *HINTTuner) NotifyLabelUpdate() {
	select {
	case rt.labelUpdateCh <- struct{}{}:
	default:
		// Already pending — no need to queue another.
	}
}

// waitForLabels waits for the user to label tracks and click continue.
// There is no deadline — the session stays in awaiting_labels until the
// user explicitly continues via the dashboard.
func (rt *HINTTuner) waitForLabels(ctx context.Context, runID string, threshold float64) error {
	rt.mu.Lock()
	rt.state.LabelDeadline = nil // no deadline — user continues when ready
	rt.mu.Unlock()

	// Safety-net ticker: re-query progress periodically in case a label
	// update notification was missed.  The primary trigger is labelUpdateCh.
	ticker := time.NewTicker(rt.pollInterval)
	defer ticker.Stop()

	// Helper: query label progress from the DB, update state, and broadcast.
	refreshProgress := func() {
		if rt.labelQuerier == nil {
			return
		}
		total, labelled, byClass, err := rt.labelQuerier.GetLabelingProgress(runID)
		if err != nil {
			rt.logger.Printf("[hint] Failed to query label progress: %v", err)
			return
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
		rt.stateCond.Broadcast()
		rt.logger.Printf("[hint] Label progress: %d/%d (%.1f%%)", labelled, total, pct*100)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case sig := <-rt.continueCh:
			// Manual continue signal from the dashboard
			rt.logger.Printf("[hint] Received continue signal (next_duration=%d, add_round=%v)", sig.NextSweepDurationMins, sig.AddRound)
			if sig.NextSweepDurationMins > 0 {
				rt.mu.Lock()
				rt.state.NextSweepDuration = sig.NextSweepDurationMins
				rt.mu.Unlock()
			}
			return nil

		case <-rt.labelUpdateCh:
			// A label was just saved — immediately refresh progress.
			refreshProgress()

		case <-ticker.C:
			// Periodic safety-net refresh.
			refreshProgress()
		}
	}
}

// waitForAutoTuneComplete polls the auto-tuner until completion or deadline.
func (rt *HINTTuner) waitForAutoTuneComplete(ctx context.Context, deadline time.Time) (*AutoTuneState, error) {
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
