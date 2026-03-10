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

// taggedLogger pairs a standard log.Logger (no prefix) with a tag string
// that is prepended to the format string at each call site. This ensures the
// tag appears after the timestamp, not before it.
type taggedLogger struct {
	logger *log.Logger
	tag    string
}

func (tl *taggedLogger) printf(format string, args ...interface{}) {
	tl.logger.Printf(tl.tag+format, args...)
}

var (
	mu          sync.RWMutex
	opsLogger   *taggedLogger
	diagLogger  *taggedLogger
	traceLogger *taggedLogger
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
	opsLogger = newTaggedLogger("[lidar] ", w.Ops)
	diagLogger = newTaggedLogger("[lidar] ", w.Diag)
	traceLogger = newTaggedLogger("[lidar] ", w.Trace)
}

// SubLogger returns an ops-level Printf-compatible function with a combined
// tag prefix, e.g. SubLogger("pcap") yields "[lidar/pcap] " tag.
// Returns a no-op if the ops writer is nil.
func SubLogger(subtag string) func(format string, args ...interface{}) {
	mu.RLock()
	w := opsWriter
	mu.RUnlock()
	if w == nil {
		return func(string, ...interface{}) {}
	}
	tl := newTaggedLogger("[lidar/"+subtag+"] ", w)
	return tl.printf
}

// SubLoggers returns ops/diag/trace Printf-compatible functions with a
// combined tag prefix, e.g. SubLoggers("vis") produces "[lidar/vis] ".
// Any stream without a writer returns a no-op.
func SubLoggers(subtag string) (ops, diag, trace func(format string, args ...interface{})) {
	mu.RLock()
	ow, dw, tw := opsWriter, diagWriter, traceWriter
	mu.RUnlock()
	noop := func(string, ...interface{}) {}
	tag := "[lidar/" + subtag + "] "
	if ow != nil {
		tl := newTaggedLogger(tag, ow)
		ops = tl.printf
	} else {
		ops = noop
	}
	if dw != nil {
		tl := newTaggedLogger(tag, dw)
		diag = tl.printf
	} else {
		diag = noop
	}
	if tw != nil {
		tl := newTaggedLogger(tag, tw)
		trace = tl.printf
	} else {
		trace = noop
	}
	return
}

// newTaggedLogger creates a taggedLogger for a given writer, or returns nil
// if w is nil. The tag is prepended to each log message after the timestamp.
func newTaggedLogger(tag string, w io.Writer) *taggedLogger {
	if w == nil {
		return nil
	}
	return &taggedLogger{
		logger: log.New(w, "", log.LstdFlags|log.Lmicroseconds),
		tag:    tag,
	}
}

// Opsf logs to the ops stream (actionable warnings, errors, lifecycle events).
func Opsf(format string, args ...interface{}) {
	mu.RLock()
	l := opsLogger
	mu.RUnlock()
	if l != nil {
		l.printf(format, args...)
	}
}

// Diagf logs to the diag stream (day-to-day diagnostics, tuning context).
func Diagf(format string, args ...interface{}) {
	mu.RLock()
	l := diagLogger
	mu.RUnlock()
	if l != nil {
		l.printf(format, args...)
	}
}

// Tracef logs to the trace stream (high-frequency packet/frame telemetry).
func Tracef(format string, args ...interface{}) {
	mu.RLock()
	l := traceLogger
	mu.RUnlock()
	if l != nil {
		l.printf(format, args...)
	}
}
