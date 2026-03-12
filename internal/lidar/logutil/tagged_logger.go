package logutil

import (
	"fmt"
	"io"
	"log"
	"time"
)

const timestampLayout = "2006/01/02 15:04:05.000000"

// TaggedLogger emits log lines with timestamps followed by a stable package tag.
type TaggedLogger struct {
	logger *log.Logger
	tag    string
	now    func() time.Time
}

// NewTaggedLogger creates a tagged logger that uses the current wall clock.
func NewTaggedLogger(tag string, w io.Writer) *TaggedLogger {
	return NewTaggedLoggerWithNow(tag, w, time.Now)
}

// NewTaggedLoggerWithNow creates a tagged logger with an injected time source.
func NewTaggedLoggerWithNow(tag string, w io.Writer, now func() time.Time) *TaggedLogger {
	if w == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &TaggedLogger{
		logger: log.New(w, "", 0),
		tag:    tag,
		now:    now,
	}
}

// Printf writes a timestamped log line with the package tag after the timestamp.
func (tl *TaggedLogger) Printf(format string, args ...interface{}) {
	if tl == nil {
		return
	}
	msg := tl.tag + fmt.Sprintf(format, args...)
	tl.logger.Printf("%s %s", tl.now().Format(timestampLayout), msg)
}
