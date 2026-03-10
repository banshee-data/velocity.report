package pipeline

import (
	"io"
	"log"
)

type taggedLogger struct {
	logger *log.Logger
	tag    string
}

var (
	opsLogger   *taggedLogger
	diagLogger  *taggedLogger
	traceLogger *taggedLogger
)

// SetLogWriters configures the three logging streams for the pipeline package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	opsLogger = newTaggedLogger("[pipeline] ", ops)
	diagLogger = newTaggedLogger("[pipeline] ", diag)
	traceLogger = newTaggedLogger("[pipeline] ", trace)
}

func newTaggedLogger(tag string, w io.Writer) *taggedLogger {
	if w == nil {
		return nil
	}
	return &taggedLogger{
		logger: log.New(w, "", log.LstdFlags|log.Lmicroseconds),
		tag:    tag,
	}
}

// opsf logs to the ops stream (actionable warnings, errors, data loss).
func opsf(format string, args ...interface{}) {
	if opsLogger != nil {
		opsLogger.logger.Printf(opsLogger.tag+format, args...)
	}
}

// diagf logs to the diag stream (day-to-day diagnostics, tuning context).
func diagf(format string, args ...interface{}) {
	if diagLogger != nil {
		diagLogger.logger.Printf(diagLogger.tag+format, args...)
	}
}

// tracef logs to the trace stream (high-frequency packet/frame telemetry).
func tracef(format string, args ...interface{}) {
	if traceLogger != nil {
		traceLogger.logger.Printf(traceLogger.tag+format, args...)
	}
}

// DO NOT add Debugf, that's an anti-pattern. match callsite needs to opsf,diagf,tracef
