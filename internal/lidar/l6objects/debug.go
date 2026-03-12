package l6objects

import (
	"io"

	"github.com/banshee-data/velocity.report/internal/lidar/logutil"
)

var (
	opsLogger   *logutil.TaggedLogger
	diagLogger  *logutil.TaggedLogger
	traceLogger *logutil.TaggedLogger
)

// SetLogWriters configures the three logging streams for the l6objects package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	opsLogger = logutil.NewTaggedLogger("[l6objects] ", ops)
	diagLogger = logutil.NewTaggedLogger("[l6objects] ", diag)
	traceLogger = logutil.NewTaggedLogger("[l6objects] ", trace)
}

// opsf logs to the ops stream (actionable warnings, errors, lifecycle events).
func opsf(format string, args ...interface{}) {
	if opsLogger != nil {
		opsLogger.Printf(format, args...)
	}
}

// diagf logs to the diag stream (day-to-day diagnostics, tuning context).
func diagf(format string, args ...interface{}) {
	if diagLogger != nil {
		diagLogger.Printf(format, args...)
	}
}

// tracef logs to the trace stream (high-frequency algorithm telemetry).
func tracef(format string, args ...interface{}) {
	if traceLogger != nil {
		traceLogger.Printf(format, args...)
	}
}

// DO NOT add Debugf, that's an anti-pattern. Each callsite needs to use opsf, diagf, or tracef.
