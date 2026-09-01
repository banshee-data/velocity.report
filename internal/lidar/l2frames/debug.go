package l2frames

import (
	"io"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/logutil"
)

var (
	// logMu guards the three loggers. They are reconfigured at runtime while
	// other goroutines are logging, so the pointers cannot be read and written
	// unsynchronised. internal/lidar/debug.go guards its equivalents the same way.
	logMu       sync.RWMutex
	opsLogger   *logutil.TaggedLogger
	diagLogger  *logutil.TaggedLogger
	traceLogger *logutil.TaggedLogger
)

// SetLogWriters configures the three logging streams for the l2frames package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	logMu.Lock()
	defer logMu.Unlock()
	opsLogger = logutil.NewTaggedLogger("[l2frames] ", ops)
	diagLogger = logutil.NewTaggedLogger("[l2frames] ", diag)
	traceLogger = logutil.NewTaggedLogger("[l2frames] ", trace)
}

// opsf logs to the ops stream (actionable warnings, errors, data loss).
func opsf(format string, args ...interface{}) {
	logMu.RLock()
	l := opsLogger
	logMu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

// diagf logs to the diag stream (day-to-day diagnostics, tuning context).
func diagf(format string, args ...interface{}) {
	logMu.RLock()
	l := diagLogger
	logMu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

// tracef logs to the trace stream (high-frequency packet/frame telemetry).
func tracef(format string, args ...interface{}) {
	logMu.RLock()
	l := traceLogger
	logMu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

// DO NOT add Debugf, that's an anti-pattern. match callsite needs to opsf,diagf,tracef
