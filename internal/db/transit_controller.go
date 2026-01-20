package db

import (
	"context"
	"log"
	"sync"
	"time"
)

// TransitController manages the state and execution of the transit worker.
// It provides thread-safe control over whether the transit worker runs,
// and supports manual triggering from the UI.
type TransitController struct {
	worker        *TransitWorker
	enabled       bool
	mu            sync.RWMutex
	manualTrigger chan struct{}
	fullHistory   chan struct{}

	// Status tracking
	lastRunAt    time.Time
	lastRunError error
	runCount     int64
	currentRun   *TransitRunInfo
	lastRun      *TransitRunInfo
}

// TransitRunInfo captures details about a single transit worker run.
type TransitRunInfo struct {
	Trigger    string    `json:"trigger,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// TransitStatus represents the current state of the transit worker.
type TransitStatus struct {
	Enabled      bool            `json:"enabled"`
	LastRunAt    time.Time       `json:"last_run_at"`
	LastRunError string          `json:"last_run_error,omitempty"`
	RunCount     int64           `json:"run_count"`
	IsHealthy    bool            `json:"is_healthy"`
	CurrentRun   *TransitRunInfo `json:"current_run,omitempty"`
	LastRun      *TransitRunInfo `json:"last_run,omitempty"`
}

// NewTransitController creates a new controller for the transit worker.
func NewTransitController(worker *TransitWorker) *TransitController {
	return &TransitController{
		worker:  worker,
		enabled: true, // Default to enabled on boot
		// Buffered channel of size 1 to coalesce multiple rapid trigger requests.
		// If a trigger is already pending, subsequent triggers are skipped.
		manualTrigger: make(chan struct{}, 1),
		fullHistory:   make(chan struct{}, 1),
	}
}

// IsEnabled returns whether the transit worker is currently enabled.
func (tc *TransitController) IsEnabled() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.enabled
}

// SetEnabled sets whether the transit worker should run.
// If enabling, it also triggers an immediate run.
func (tc *TransitController) SetEnabled(enabled bool) {
	tc.mu.Lock()
	tc.enabled = enabled
	tc.mu.Unlock()

	if enabled {
		// Trigger immediate run when enabling
		tc.TriggerManualRun()
	}
}

// TriggerManualRun triggers a manual run of the transit worker.
// This is non-blocking and safe to call multiple times.
func (tc *TransitController) TriggerManualRun() {
	select {
	case tc.manualTrigger <- struct{}{}:
		// Trigger sent
	default:
		// Channel already has a pending trigger, skip
		log.Printf("Transit worker manual trigger skipped (already pending)")
	}
}

// TriggerFullHistoryRun triggers a full-history run of the transit worker.
// This is non-blocking and safe to call multiple times.
func (tc *TransitController) TriggerFullHistoryRun() {
	select {
	case tc.fullHistory <- struct{}{}:
		// Trigger sent
	default:
		// Channel already has a pending trigger, skip
		log.Printf("Transit worker full-history trigger skipped (already pending)")
	}
}

// GetStatus returns the current status of the transit worker.
func (tc *TransitController) GetStatus() TransitStatus {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	status := TransitStatus{
		Enabled:   tc.enabled,
		LastRunAt: tc.lastRunAt,
		RunCount:  tc.runCount,
		IsHealthy: true,
	}

	if tc.lastRunError != nil {
		status.LastRunError = tc.lastRunError.Error()
		status.IsHealthy = false
	}
	if tc.currentRun != nil {
		runCopy := *tc.currentRun
		status.CurrentRun = &runCopy
	}
	if tc.lastRun != nil {
		runCopy := *tc.lastRun
		status.LastRun = &runCopy
	}

	// Consider unhealthy if enabled but hasn't run in 2x the interval
	if tc.enabled && !tc.lastRunAt.IsZero() {
		expectedInterval := tc.worker.Interval * 2
		if time.Since(tc.lastRunAt) > expectedInterval {
			status.IsHealthy = false
		}
	}

	return status
}

func (tc *TransitController) startRun(trigger string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.currentRun = &TransitRunInfo{
		Trigger:   trigger,
		StartedAt: time.Now(),
	}
}

func (tc *TransitController) finishRun(err error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()
	if tc.currentRun == nil {
		tc.currentRun = &TransitRunInfo{
			Trigger:   "unknown",
			StartedAt: now,
		}
	}
	tc.currentRun.FinishedAt = now
	tc.currentRun.DurationMs = now.Sub(tc.currentRun.StartedAt).Milliseconds()
	if err != nil {
		tc.currentRun.Error = err.Error()
	}

	tc.lastRun = tc.currentRun
	tc.currentRun = nil

	tc.lastRunAt = now
	tc.lastRunError = err
	tc.runCount++
}

// recordRun updates the status after a worker run.
func (tc *TransitController) recordRun(err error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()
	run := &TransitRunInfo{
		Trigger:    "unknown",
		StartedAt:  now,
		FinishedAt: now,
		DurationMs: 0,
	}
	if err != nil {
		run.Error = err.Error()
	}
	tc.lastRun = run

	tc.lastRunAt = now
	tc.lastRunError = err
	tc.runCount++
}

// Run starts the transit worker loop. This should be called in a goroutine.
// It will run periodically based on the worker's Interval, but only when enabled.
// It also responds to manual triggers from the UI.
func (tc *TransitController) Run(ctx context.Context) error {
	ticker := time.NewTicker(tc.worker.Interval)
	defer ticker.Stop()
	log.Printf("Transit worker loop started: enabled=%t interval=%s window=%s", tc.IsEnabled(), tc.worker.Interval, tc.worker.Window)

	// Run once immediately on startup if enabled
	if tc.IsEnabled() {
		tc.startRun("initial")
		err := tc.worker.RunOnce(ctx)
		tc.finishRun(err)
		if err != nil {
			log.Printf("Transit worker initial run error: %v", err)
		} else {
			log.Printf("Transit worker completed initial run")
		}
	}

	for {
		select {
		case <-ticker.C:
			// Check if enabled before running
			if tc.IsEnabled() {
				log.Printf("Transit worker scheduled tick: last_run_at=%v run_count=%d", tc.lastRunAt, tc.runCount)
				tc.startRun("periodic")
				err := tc.worker.RunOnce(ctx)
				tc.finishRun(err)
				if err != nil {
					log.Printf("Transit worker periodic run error: %v", err)
				} else {
					log.Printf("Transit worker completed periodic run")
				}
			} else {
				log.Printf("Transit worker skipped (disabled): last_run_at=%v run_count=%d", tc.lastRunAt, tc.runCount)
			}
		case <-tc.manualTrigger:
			// Manual trigger from UI
			if tc.IsEnabled() {
				log.Printf("Transit worker manual run triggered")
				tc.startRun("manual")
				err := tc.worker.RunOnce(ctx)
				tc.finishRun(err)
				if err != nil {
					log.Printf("Transit worker manual run error: %v", err)
				} else {
					log.Printf("Transit worker completed manual run")
				}
			} else {
				log.Printf("Transit worker manual run skipped (disabled): last_run_at=%v run_count=%d", tc.lastRunAt, tc.runCount)
			}
		case <-tc.fullHistory:
			// Full-history trigger from UI
			if tc.IsEnabled() {
				log.Printf("Transit worker full-history run triggered")
				tc.startRun("full-history")
				err := tc.worker.RunFullHistory(ctx)
				tc.finishRun(err)
				if err != nil {
					log.Printf("Transit worker full-history run error: %v", err)
				} else {
					log.Printf("Transit worker completed full-history run")
				}
			} else {
				log.Printf("Transit worker full-history run skipped (disabled): last_run_at=%v run_count=%d", tc.lastRunAt, tc.runCount)
			}
		case <-ctx.Done():
			log.Printf("Transit worker terminated")
			return ctx.Err()
		}
	}
}
