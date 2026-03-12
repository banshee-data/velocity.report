package lidar

import (
	"io"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/logutil"
)

// LogWriters holds the io.Writers for each logging stream.
type LogWriters struct {
	Ops   io.Writer
	Diag  io.Writer
	Trace io.Writer
}

var (
	mu          sync.RWMutex
	opsLogger   *logutil.TaggedLogger
	diagLogger  *logutil.TaggedLogger
	traceLogger *logutil.TaggedLogger
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
	opsLogger = logutil.NewTaggedLogger("[lidar] ", w.Ops)
	diagLogger = logutil.NewTaggedLogger("[lidar] ", w.Diag)
	traceLogger = logutil.NewTaggedLogger("[lidar] ", w.Trace)
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
	tl := logutil.NewTaggedLogger("[lidar/"+subtag+"] ", w)
	return tl.Printf
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
		tl := logutil.NewTaggedLogger(tag, ow)
		ops = tl.Printf
	} else {
		ops = noop
	}
	if dw != nil {
		tl := logutil.NewTaggedLogger(tag, dw)
		diag = tl.Printf
	} else {
		diag = noop
	}
	if tw != nil {
		tl := logutil.NewTaggedLogger(tag, tw)
		trace = tl.Printf
	} else {
		trace = noop
	}
	return
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
