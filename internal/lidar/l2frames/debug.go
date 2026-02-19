package l2frames

import (
	"io"
	"log"
)

var debugLogger *log.Logger

// SetDebugLogger installs a debug logger that receives verbose LiDAR diagnostics.
// Pass nil to disable debug logging.
func SetDebugLogger(w io.Writer) {
	if w == nil {
		debugLogger = nil
		return
	}
	debugLogger = log.New(w, "", log.LstdFlags|log.Lmicroseconds)
}

// debugf logs formatted debug messages when a debug logger is configured.
func debugf(format string, args ...interface{}) {
	if debugLogger != nil {
		debugLogger.Printf(format, args...)
	}
}

// Debugf is an exported helper for callers outside the lidar package.
func Debugf(format string, args ...interface{}) {
	debugf(format, args...)
}
