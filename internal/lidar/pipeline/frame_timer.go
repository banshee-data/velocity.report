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
	stages     []stageTiming
	current    string
	stageStart time.Time
	// clock supplies the current time. It is a field rather than a direct
	// time.Now() call so that tests can give a stage an exact duration.
	//
	// That matters more than it looks. time.Sleep guarantees a minimum
	// duration and not a maximum, so assertions of the form "this stage took
	// at least 2ms" are safe against a sleep, but any assertion about which
	// stage was *longest* is at the mercy of the scheduler: under load a 2ms
	// sleep can outrun a 5ms one. See newFrameTimerWithClock.
	//
	// The indirection is free in practice: the pipeline only builds a
	// frameTimer when BenchmarkMode is enabled.
	clock func() time.Time
}

type stageTiming struct {
	name     string
	duration time.Duration
}

// newFrameTimer creates a new timer reading the wall clock.
func newFrameTimer(frameID string) *frameTimer {
	return newFrameTimerWithClock(frameID, time.Now)
}

// newFrameTimerWithClock creates a timer reading the supplied clock, so a test
// can advance time by exact amounts instead of sleeping for approximate ones.
func newFrameTimerWithClock(frameID string, clock func() time.Time) *frameTimer {
	return &frameTimer{
		frameID: frameID,
		stages:  make([]stageTiming, 0, 8),
		clock:   clock,
	}
}

// Stage ends the current stage (if any) and starts a new one.
func (ft *frameTimer) Stage(name string) {
	now := ft.clock()
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
			duration: ft.clock().Sub(ft.stageStart),
		})
		ft.current = ""
	}
}

// Total returns the sum of the recorded stage durations.
//
// That is deliberately not the wall-clock span from construction to End: any
// time between creating the timer and the first Stage() call belongs to no
// stage and is not counted.
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
