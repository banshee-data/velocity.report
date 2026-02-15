// Package sweep provides utilities for parameter sweeping and sampling in LiDAR background detection tuning.
package sweep

import (
	"context"
	"time"
)

// SweepBackend abstracts the operations that the sweep runner needs from the
// server. Two implementations exist:
//
//   - monitor.Client — makes HTTP requests (used by external callers or
//     cross-process deployments).
//   - monitor.DirectBackend — calls Go methods in-process, avoiding the
//     HTTP serialisation overhead and eliminating polling loops.
type SweepBackend interface {
	// SensorID returns the target sensor identifier.
	SensorID() string

	// --- Background / acceptance ---

	// FetchBuckets returns the bucket boundary configuration.
	FetchBuckets() []string

	// FetchAcceptanceMetrics returns the current acceptance counters.
	FetchAcceptanceMetrics() (map[string]interface{}, error)

	// ResetAcceptance zeroes the acceptance counters.
	ResetAcceptance() error

	// --- Grid ---

	// FetchGridStatus returns background grid statistics.
	FetchGridStatus() (map[string]interface{}, error)

	// ResetGrid clears the background grid, frame builder and tracker.
	ResetGrid() error

	// WaitForGridSettle blocks until background_count > 0 or timeout.
	WaitForGridSettle(timeout time.Duration)

	// --- Tracking ---

	// FetchTrackingMetrics returns velocity-trail alignment metrics.
	FetchTrackingMetrics() (map[string]interface{}, error)

	// --- Parameters ---

	// SetTuningParams applies a partial tuning config update.
	SetTuningParams(params map[string]interface{}) error

	// --- PCAP lifecycle ---

	// StartPCAPReplayWithConfig begins a PCAP replay.
	StartPCAPReplayWithConfig(cfg PCAPReplayConfig) error

	// StopPCAPReplay cancels the running PCAP replay.
	StopPCAPReplay() error

	// WaitForPCAPComplete blocks until the PCAP replay finishes or timeout.
	WaitForPCAPComplete(timeout time.Duration) error

	// --- Data source ---

	// GetLastAnalysisRunID returns the last analysis run ID (empty if none).
	GetLastAnalysisRunID() string
}

// PCAPReplayConfig holds configuration for starting a PCAP replay.
// This duplicates monitor.PCAPReplayConfig so the sweep package does not
// import monitor. The monitor package converts between the two.
type PCAPReplayConfig struct {
	PCAPFile         string
	StartSeconds     float64
	DurationSeconds  float64
	MaxRetries       int
	AnalysisMode     bool   // When true, preserve grid after PCAP completion
	SpeedMode        string // "fastest", "realtime", or "ratio"
	DisableRecording bool   // When true, skip VRLOG recording for this replay
}

// WaitForPCAPDone blocks on a channel until it is closed or ctx is cancelled.
// Helper used by DirectBackend; also available to tests.
func WaitForPCAPDone(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
