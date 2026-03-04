package pipeline

import (
	"fmt"
	"strings"
	"time"
)

// frameTimer is a lightweight per-frame stopwatch that records stage durations.
// It produces a structured log-friendly string for pipeline performance tracing.
//
// Usage:
//
//	ft := newFrameTimer("F0042")
//	ft.Stage("foreground")
//	// ... do foreground extraction ...
//	ft.Stage("cluster")
//	// ... do DBSCAN ...
//	ft.End()
//	tracef("[Pipeline] frame=%s total=%.1fms %s", ft.frameID, ft.TotalMs(), ft.Format())
type frameTimer struct {
	frameID    string
	frameStart time.Time
	stages     []stageTiming
	current    string
	stageStart time.Time
}

type stageTiming struct {
	name     string
	duration time.Duration
}

// newFrameTimer creates a new timer. The clock starts immediately.
func newFrameTimer(frameID string) *frameTimer {
	now := time.Now()
	return &frameTimer{
		frameID:    frameID,
		frameStart: now,
		stages:     make([]stageTiming, 0, 8),
	}
}

// Stage ends the current stage (if any) and starts a new one.
func (ft *frameTimer) Stage(name string) {
	now := time.Now()
	if ft.current != "" {
		ft.stages = append(ft.stages, stageTiming{
			name:     ft.current,
			duration: now.Sub(ft.stageStart),
		})
	}
	ft.current = name
	ft.stageStart = now
}

// End closes the final stage. Must be called to capture the last stage's duration.
func (ft *frameTimer) End() {
	if ft.current != "" {
		ft.stages = append(ft.stages, stageTiming{
			name:     ft.current,
			duration: time.Since(ft.stageStart),
		})
		ft.current = ""
	}
}

// Total returns the wall-clock duration from timer creation to the last End() call.
func (ft *frameTimer) Total() time.Duration {
	var total time.Duration
	for _, s := range ft.stages {
		total += s.duration
	}
	return total
}

// TotalMs returns Total() in milliseconds as a float64 for log formatting.
func (ft *frameTimer) TotalMs() float64 {
	return float64(ft.Total().Nanoseconds()) / 1e6
}

// SlowestStage returns the name and duration of the longest stage.
// Returns ("", 0) if no stages were recorded.
func (ft *frameTimer) SlowestStage() (string, time.Duration) {
	var maxName string
	var maxDur time.Duration
	for _, s := range ft.stages {
		if s.duration > maxDur {
			maxName = s.name
			maxDur = s.duration
		}
	}
	return maxName, maxDur
}

// Format returns a space-separated key=value string of stage timings.
// Example: "foreground=2.1ms cluster=8.3ms track=4.1ms classify=1.2ms"
func (ft *frameTimer) Format() string {
	if len(ft.stages) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range ft.stages {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%.1fms", s.name, float64(s.duration.Nanoseconds())/1e6)
	}
	return b.String()
}
