package l3grid

import (
	"io"
	"log"
)

var (
	opsLogger   *log.Logger
	diagLogger  *log.Logger
	traceLogger *log.Logger
)

// SetLogWriters configures the three logging streams for the l3grid package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	opsLogger = newLogger("[l3grid] ", ops)
	diagLogger = newLogger("[l3grid] ", diag)
	traceLogger = newLogger("[l3grid] ", trace)
}

func newLogger(prefix string, w io.Writer) *log.Logger {
	if w == nil {
		return nil
	}
	return log.New(w, prefix, log.LstdFlags|log.Lmicroseconds)
}

// opsf logs to the ops stream (actionable warnings, errors, data loss).
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

// tracef logs to the trace stream (high-frequency packet/frame telemetry).
func tracef(format string, args ...interface{}) {
	if traceLogger != nil {
		traceLogger.Printf(format, args...)
	}
}
