package monitor

import (
	"io"
	"log"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/logutil"
)

var (
	opsLogger   *logutil.TaggedLogger
	diagLogger  *logutil.TaggedLogger
	traceLogger *logutil.TaggedLogger
)

// SetLogWriters configures the three logging streams for the monitor package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	opsLogger = logutil.NewTaggedLogger("[monitor] ", ops)
	diagLogger = logutil.NewTaggedLogger("[monitor] ", diag)
	traceLogger = logutil.NewTaggedLogger("[monitor] ", trace)
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

func opsFatalf(format string, args ...interface{}) {
	if opsLogger == nil {
		log.Fatalf(format, args...)
	}
	opsLogger.Printf(format, args...)
	os.Exit(1)
}
