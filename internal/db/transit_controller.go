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
}

// NewTransitController creates a new controller for the transit worker.
func NewTransitController(worker *TransitWorker) *TransitController {
	return &TransitController{
		worker:  worker,
		enabled: true, // Default to enabled on boot
		// Buffered channel of size 1 to coalesce multiple rapid trigger requests.
		// If a trigger is already pending, subsequent triggers are skipped.
		manualTrigger: make(chan struct{}, 1),
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
	}
}

// Run starts the transit worker loop. This should be called in a goroutine.
// It will run periodically based on the worker's Interval, but only when enabled.
// It also responds to manual triggers from the UI.
func (tc *TransitController) Run(ctx context.Context) error {
	ticker := time.NewTicker(tc.worker.Interval)
	defer ticker.Stop()

	// Run once immediately on startup if enabled
	if tc.IsEnabled() {
		if err := tc.worker.RunOnce(ctx); err != nil {
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
				if err := tc.worker.RunOnce(ctx); err != nil {
					log.Printf("Transit worker periodic run error: %v", err)
				} else {
					log.Printf("Transit worker completed periodic run")
				}
			} else {
				log.Printf("Transit worker skipped (disabled)")
			}
		case <-tc.manualTrigger:
			// Manual trigger from UI
			if tc.IsEnabled() {
				log.Printf("Transit worker manual run triggered")
				if err := tc.worker.RunOnce(ctx); err != nil {
					log.Printf("Transit worker manual run error: %v", err)
				} else {
					log.Printf("Transit worker completed manual run")
				}
			} else {
				log.Printf("Transit worker manual run skipped (disabled)")
			}
		case <-ctx.Done():
			log.Printf("Transit worker terminated")
			return ctx.Err()
		}
	}
}
