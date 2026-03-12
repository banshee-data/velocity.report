package logutil

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/timeutil"
)

func TestNewTaggedLogger_NilWriter(t *testing.T) {
	t.Parallel()

	logger := NewTaggedLogger("[test] ", nil)
	if logger != nil {
		t.Fatal("expected nil logger for nil writer")
	}
}

func TestTaggedLoggerPrintf_FormatsExactLine(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 3, 11, 15, 4, 5, 123456000, time.UTC)
	clock := timeutil.NewMockClock(fixed)
	var buf bytes.Buffer

	logger := NewTaggedLoggerWithNow("[parse] ", &buf, clock.Now)
	logger.Printf("hello %d", 42)

	const want = "2026/03/11 15:04:05.123456 [parse] hello 42\n"
	if got := buf.String(); got != want {
		t.Fatalf("Printf() line = %q, want %q", got, want)
	}
}

func TestTaggedLoggerPrintf_UsesCurrentTimePerCall(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 3, 11, 15, 4, 5, 0, time.UTC)
	clock := timeutil.NewMockClock(fixed)
	var buf bytes.Buffer

	logger := NewTaggedLoggerWithNow("[pipeline] ", &buf, clock.Now)
	logger.Printf("first")
	clock.Advance(250 * time.Millisecond)
	logger.Printf("second")

	const want = "" +
		"2026/03/11 15:04:05.000000 [pipeline] first\n" +
		"2026/03/11 15:04:05.250000 [pipeline] second\n"
	if got := buf.String(); got != want {
		t.Fatalf("Printf() lines = %q, want %q", got, want)
	}
}

func TestNewTaggedLoggerWithNow_NilNowUsesWallClock(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewTaggedLoggerWithNow("[test] ", &buf, nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	logger.Printf("ping")
	if !strings.Contains(buf.String(), "[test] ping") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestTaggedLoggerPrintf_NilReceiverNoopSafe(t *testing.T) {
	t.Parallel()

	var tl *TaggedLogger
	// Must not panic.
	tl.Printf("should be a no-op")
}
