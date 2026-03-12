package analysis

import (
	"io"

	"github.com/banshee-data/velocity.report/internal/lidar/logutil"
)

var (
	opsLogger   *logutil.TaggedLogger
	diagLogger  *logutil.TaggedLogger
	traceLogger *logutil.TaggedLogger
)

// SetLogWriters configures the three logging streams for the analysis package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	opsLogger = logutil.NewTaggedLogger("[analysis] ", ops)
	diagLogger = logutil.NewTaggedLogger("[analysis] ", diag)
	traceLogger = logutil.NewTaggedLogger("[analysis] ", trace)
}

func opsf(format string, args ...interface{}) {
	if opsLogger != nil {
		opsLogger.Printf(format, args...)
	}
}

func diagf(format string, args ...interface{}) {
	if diagLogger != nil {
		diagLogger.Printf(format, args...)
	}
}

func tracef(format string, args ...interface{}) {
	if traceLogger != nil {
		traceLogger.Printf(format, args...)
	}
}
