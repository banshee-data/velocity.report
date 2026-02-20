package lidar

import (
	"io"
	"log"
	"strings"
	"sync"
)

// LogLevel represents a logging stream.
type LogLevel int

const (
	// LogOps routes to the ops stream: actionable warnings/errors and lifecycle events.
	LogOps LogLevel = iota
	// LogDebug routes to the debug stream: day-to-day diagnostics for troubleshooting.
	LogDebug
	// LogTrace routes to the trace stream: high-frequency packet/frame telemetry.
	LogTrace
)

// LogWriters holds the io.Writers for each logging stream.
type LogWriters struct {
	Ops   io.Writer
	Debug io.Writer
	Trace io.Writer
}

var (
	mu          sync.RWMutex
	opsLogger   *log.Logger
	debugLogger *log.Logger
	traceLogger *log.Logger
)

// Keyword lists used by the classifier to route Debugf messages.
var (
	opsKeywords   = []string{"error", "failed", "fatal", "panic", "warn", "timeout", "dropped"}
	traceKeywords = []string{"packet", "queued", "parsed", "progress", "fps=", "bandwidth", "frame="}
)

// SetLogWriters configures all three logging streams at once.
// Pass nil for any writer to disable that stream.
func SetLogWriters(w LogWriters) {
	mu.Lock()
	defer mu.Unlock()
	opsLogger = newLogger("", w.Ops)
	debugLogger = newLogger("", w.Debug)
	traceLogger = newLogger("", w.Trace)
}

// SetLogWriter configures a single logging stream.
// Pass nil to disable the stream.
func SetLogWriter(level LogLevel, w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	switch level {
	case LogOps:
		opsLogger = newLogger("", w)
	case LogDebug:
		debugLogger = newLogger("", w)
	case LogTrace:
		traceLogger = newLogger("", w)
	}
}

// SetDebugLogger is the backward-compatible shim that routes all three streams
// to a single writer. Pass nil to disable all logging.
func SetDebugLogger(w io.Writer) {
	SetLogWriters(LogWriters{Ops: w, Debug: w, Trace: w})
}

// newLogger creates a *log.Logger for a given writer, or returns nil if w is nil.
func newLogger(prefix string, w io.Writer) *log.Logger {
	if w == nil {
		return nil
	}
	return log.New(w, prefix, log.LstdFlags|log.Lmicroseconds)
}

// Opsf logs to the ops stream (actionable warnings, errors, lifecycle events).
func Opsf(format string, args ...interface{}) {
	mu.RLock()
	l := opsLogger
	mu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

// Diagf logs to the debug stream (day-to-day diagnostics, tuning context).
func Diagf(format string, args ...interface{}) {
	mu.RLock()
	l := debugLogger
	mu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

// Tracef logs to the trace stream (high-frequency packet/frame telemetry).
func Tracef(format string, args ...interface{}) {
	mu.RLock()
	l := traceLogger
	mu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

// Debugf routes through the keyword classifier for backward compatibility.
// New call sites should prefer Opsf, Diagf, or Tracef directly.
func Debugf(format string, args ...interface{}) {
	debugf(format, args...)
}

// debugf applies the classifier and dispatches to the appropriate stream.
func debugf(format string, args ...interface{}) {
	level := classifyMessage(format)
	switch level {
	case LogOps:
		Opsf(format, args...)
	case LogTrace:
		Tracef(format, args...)
	default:
		Diagf(format, args...)
	}
}

// classifyMessage applies the keyword rubric to determine stream routing.
// Ops keywords take priority, then trace; default is debug.
func classifyMessage(format string) LogLevel {
	lower := strings.ToLower(format)
	for _, kw := range opsKeywords {
		if strings.Contains(lower, kw) {
			return LogOps
		}
	}
	for _, kw := range traceKeywords {
		if strings.Contains(lower, kw) {
			return LogTrace
		}
	}
	return LogDebug
}
