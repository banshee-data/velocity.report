package monitoring

import "log"

// Logf is the package-level diagnostic logger. It defaults to log.Printf but may
// be replaced by SetLogger. Tests or production code can redirect or mute it.
var Logf func(format string, v ...interface{}) = log.Printf

// SetLogger replaces the package logger. Passing nil will set a no-op logger.
func SetLogger(f func(format string, v ...interface{})) {
	if f == nil {
		Logf = func(string, ...interface{}) {}
		return
	}
	Logf = f
}
