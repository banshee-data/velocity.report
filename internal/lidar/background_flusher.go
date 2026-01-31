// Package lidar provides LiDAR processing and background subtraction.
package lidar

import (
	"context"
	"log"
	"sync"
	"time"
)

// Persister is an interface for types that can persist their state.
// BackgroundManager implements this interface.
type Persister interface {
	Persist(store BgStore, reason string) error
}

// BackgroundFlusher periodically flushes a BackgroundManager to the database.
// It provides context-aware lifecycle management for background grid persistence.
type BackgroundFlusher struct {
	manager  Persister
	store    BgStore
	interval time.Duration
	reason   string
	logger   *log.Logger
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// BackgroundFlusherConfig contains configuration for BackgroundFlusher.
type BackgroundFlusherConfig struct {
	// Manager is the Persister to flush (typically a BackgroundManager)
	Manager Persister
	// Store is the database store for persistence
	Store BgStore
	// Interval is how often to flush (e.g., 60*time.Second)
	Interval time.Duration
	// Reason is the reason string to use for flushes (e.g., "periodic_flush")
	Reason string
	// Logger is optional; if nil, uses log.Default()
	Logger *log.Logger
}

// NewBackgroundFlusher creates a new BackgroundFlusher.
func NewBackgroundFlusher(cfg BackgroundFlusherConfig) *BackgroundFlusher {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	reason := cfg.Reason
	if reason == "" {
		reason = "periodic_flush"
	}
	return &BackgroundFlusher{
		manager:  cfg.Manager,
		store:    cfg.Store,
		interval: cfg.Interval,
		reason:   reason,
		logger:   logger,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Run starts the periodic flushing loop. It blocks until the context is
// cancelled or Stop() is called. Returns nil on clean shutdown.
func (f *BackgroundFlusher) Run(ctx context.Context) error {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return nil // already running
	}
	f.running = true
	f.stopCh = make(chan struct{})
	f.doneCh = make(chan struct{})
	f.mu.Unlock()

	defer func() {
		close(f.doneCh)
		f.mu.Lock()
		f.running = false
		f.mu.Unlock()
	}()

	if f.interval <= 0 {
		f.logger.Printf("BackgroundFlusher: interval is zero or negative, not starting")
		return nil
	}

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	f.logger.Printf("BackgroundFlusher started: interval=%v", f.interval)

	for {
		select {
		case <-ctx.Done():
			f.logger.Printf("BackgroundFlusher stopping due to context cancellation")
			f.flushFinal()
			return nil
		case <-f.stopCh:
			f.logger.Printf("BackgroundFlusher stopping due to Stop() call")
			f.flushFinal()
			return nil
		case <-ticker.C:
			f.flush()
		}
	}
}

// Stop requests the flusher to stop. It is safe to call multiple times.
func (f *BackgroundFlusher) Stop() {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return
	}
	select {
	case <-f.stopCh:
		// already closed
	default:
		close(f.stopCh)
	}
	f.mu.Unlock()

	// Wait for completion
	<-f.doneCh
}

// IsRunning returns whether the flusher is currently running.
func (f *BackgroundFlusher) IsRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

// flush performs a single flush operation.
func (f *BackgroundFlusher) flush() {
	if f.manager == nil || f.store == nil {
		return
	}
	if err := f.manager.Persist(f.store, f.reason); err != nil {
		f.logger.Printf("BackgroundFlusher: error flushing: %v", err)
	} else {
		f.logger.Printf("BackgroundFlusher: grid flushed to database")
	}
}

// flushFinal performs a final flush before shutdown.
func (f *BackgroundFlusher) flushFinal() {
	if f.manager == nil || f.store == nil {
		return
	}
	if err := f.manager.Persist(f.store, "final_flush"); err != nil {
		f.logger.Printf("BackgroundFlusher: error during final flush: %v", err)
	} else {
		f.logger.Printf("BackgroundFlusher: final grid flushed to database")
	}
}

// FlushNow triggers an immediate flush outside the regular interval.
func (f *BackgroundFlusher) FlushNow() {
	f.flush()
}
