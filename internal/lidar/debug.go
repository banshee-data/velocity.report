package lidar

import (
	"fmt"
	"io"
	"log"
	"sync"
)

// LogLevel represents a logging stream.
type LogLevel int

const (
	// LogOps routes to the ops stream: actionable warnings/errors and lifecycle events.
	LogOps LogLevel = iota
	// LogDiag routes to the diag stream: day-to-day diagnostics for troubleshooting.
	LogDiag
	// LogTrace routes to the trace stream: high-frequency packet/frame telemetry.
	LogTrace
)

// LogWriters holds the io.Writers for each logging stream.
type LogWriters struct {
	Ops   io.Writer
	Diag  io.Writer
	Trace io.Writer
}

var (
	mu          sync.RWMutex
	opsLogger   *log.Logger
	diagLogger  *log.Logger
	traceLogger *log.Logger
)

// SetLogWriters configures all three logging streams at once.
// Pass nil for any writer to disable that stream.
func SetLogWriters(w LogWriters) {
	mu.Lock()
	defer mu.Unlock()
	opsLogger = newLogger("[lidar] ", w.Ops)
	diagLogger = newLogger("[lidar] ", w.Diag)
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
	case LogDiag:
		diagLogger = newLogger("[lidar] ", w)
	case LogTrace:
		traceLogger = newLogger("[lidar] ", w)
	default:
		panic(fmt.Sprintf("lidar.SetLogWriter: unknown LogLevel %d", level))
	}
}

// SetLegacyLogger is the backward-compatible shim that routes all three streams
// to a single writer. Used by the VELOCITY_DEBUG_LOG fallback.
// Pass nil to disable all logging.
func SetLegacyLogger(w io.Writer) {
	SetLogWriters(LogWriters{Ops: w, Diag: w, Trace: w})
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

// Diagf logs to the diag stream (day-to-day diagnostics, tuning context).
func Diagf(format string, args ...interface{}) {
	mu.RLock()
	l := diagLogger
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
