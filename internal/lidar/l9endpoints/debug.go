package l9endpoints

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

// SetLogWriters configures the three logging streams for the visualiser package.
// Pass nil for any writer to disable that stream.
func SetLogWriters(ops, diag, trace io.Writer) {
	logMu.Lock()
	defer logMu.Unlock()
	opsLogger = logutil.NewTaggedLogger("[visualiser] ", ops)
	diagLogger = logutil.NewTaggedLogger("[visualiser] ", diag)
	traceLogger = logutil.NewTaggedLogger("[visualiser] ", trace)
}

func opsf(format string, args ...interface{}) {
	logMu.RLock()
	l := opsLogger
	logMu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

func diagf(format string, args ...interface{}) {
	logMu.RLock()
	l := diagLogger
	logMu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}

func tracef(format string, args ...interface{}) {
	logMu.RLock()
	l := traceLogger
	logMu.RUnlock()
	if l != nil {
		l.Printf(format, args...)
	}
}
