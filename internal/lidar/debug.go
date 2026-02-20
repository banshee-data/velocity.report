package lidar

import (
	"io"
	"log"
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

// SetLogWriters configures all three logging streams at once.
// Pass nil for any writer to disable that stream.
func SetLogWriters(w LogWriters) {
	mu.Lock()
	defer mu.Unlock()
	opsLogger = newLogger("[lidar] ", w.Ops)
	debugLogger = newLogger("[lidar] ", w.Debug)
	traceLogger = newLogger("[lidar] ", w.Trace)
}

// SetLogWriter configures a single logging stream.
// Pass nil to disable the stream.
func SetLogWriter(level LogLevel, w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	switch level {
	case LogOps:
		opsLogger = newLogger("[lidar] ", w)
	case LogDebug:
		debugLogger = newLogger("[lidar] ", w)
	case LogTrace:
		traceLogger = newLogger("[lidar] ", w)
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
