package lidar

import (
	"io"
	"log"
	"sync"
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
	opsWriter   io.Writer
	diagWriter  io.Writer
	traceWriter io.Writer
)

// SetLogWriters configures all three logging streams at once.
// Pass nil for any writer to disable that stream.
func SetLogWriters(w LogWriters) {
	mu.Lock()
	defer mu.Unlock()
	opsWriter = w.Ops
	diagWriter = w.Diag
	traceWriter = w.Trace
	opsLogger = newLogger("[lidar] ", w.Ops)
	diagLogger = newLogger("[lidar] ", w.Diag)
	traceLogger = newLogger("[lidar] ", w.Trace)
}

// SubLogger returns an ops-level Printf-compatible function with a combined
// tag prefix, e.g. SubLogger("pcap") yields "[lidar/pcap] " prefix.
// Returns a no-op if the ops writer is nil.
func SubLogger(subtag string) func(format string, args ...interface{}) {
	mu.RLock()
	w := opsWriter
	mu.RUnlock()
	if w == nil {
		return func(string, ...interface{}) {}
	}
	l := log.New(w, "[lidar/"+subtag+"] ", log.LstdFlags|log.Lmicroseconds)
	return l.Printf
}

// SubLoggers returns ops/diag/trace Printf-compatible functions with a
// combined tag prefix, e.g. SubLoggers("vis") produces "[lidar/vis] ".
// Any stream without a writer returns a no-op.
func SubLoggers(subtag string) (ops, diag, trace func(format string, args ...interface{})) {
	mu.RLock()
	ow, dw, tw := opsWriter, diagWriter, traceWriter
	mu.RUnlock()
	noop := func(string, ...interface{}) {}
	prefix := "[lidar/" + subtag + "] "
	flags := log.LstdFlags | log.Lmicroseconds
	if ow != nil {
		ops = log.New(ow, prefix, flags).Printf
	} else {
		ops = noop
	}
	if dw != nil {
		diag = log.New(dw, prefix, flags).Printf
	} else {
		diag = noop
	}
	if tw != nil {
		trace = log.New(tw, prefix, flags).Printf
	} else {
		trace = noop
	}
	return
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
