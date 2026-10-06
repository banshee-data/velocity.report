package l2frames

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Frame drops are bursty: the tracking pipeline stalls, the callback queue
// fills, and every frame arriving behind it is lost until the backlog clears.
// Logging each one reports a single stall dozens of times — 84 lines for two
// stalls on 2026-08-26 — and buries the useful facts, which are how many were
// lost and over what span.

func TestDroppedFrameReportSummarisesABurst(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	fb := &FrameBuilder{}

	// A burst arriving well inside the reporting interval emits nothing yet.
	for i := 0; i < 40; i++ {
		fb.reportDroppedFrame("frame-"+string(rune('a'+i%26)), uint64(i+1))
	}
	if got := opsBuf.String(); got != "" {
		t.Errorf("a burst inside the interval logged early:\n%s", got)
	}

	// Once the interval has elapsed, the next drop reports the whole burst.
	fb.dropReportMu.Lock()
	fb.dropReportStart = time.Now().Add(-2 * dropReportInterval)
	fb.dropReportMu.Unlock()

	fb.reportDroppedFrame("frame-last", 41)

	out := opsBuf.String()
	if !strings.Contains(out, "Dropped 41 frames") {
		t.Errorf("summary did not report the burst size:\n%s", out)
	}
	if !strings.Contains(out, "the tracking pipeline is not keeping up") {
		t.Errorf("summary did not say what a full callback queue means:\n%s", out)
	}
	if strings.Count(out, "Dropped") != 1 {
		t.Errorf("expected exactly one summary line, got:\n%s", out)
	}
}

func TestDroppedFrameReportResetsBetweenBursts(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	fb := &FrameBuilder{}
	fb.reportDroppedFrame("a", 1)
	fb.dropReportMu.Lock()
	fb.dropReportStart = time.Now().Add(-2 * dropReportInterval)
	fb.dropReportMu.Unlock()
	fb.reportDroppedFrame("b", 2)

	opsBuf.Reset()

	// A second burst must be counted from zero, not carry the first one's total.
	fb.reportDroppedFrame("c", 3)
	fb.dropReportMu.Lock()
	fb.dropReportStart = time.Now().Add(-2 * dropReportInterval)
	fb.dropReportMu.Unlock()
	fb.reportDroppedFrame("d", 4)

	if out := opsBuf.String(); !strings.Contains(out, "Dropped 2 frames") {
		t.Errorf("second burst did not start from zero:\n%s", out)
	}
}

// A burst that ends inside the reporting interval still has to be reported.
func TestFlushDroppedFrameReportEmitsAPendingBurst(t *testing.T) {
	var opsBuf bytes.Buffer
	SetLogWriters(&opsBuf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	fb := &FrameBuilder{}
	for i := 0; i < 7; i++ {
		fb.reportDroppedFrame("frame", uint64(i+1))
	}
	if opsBuf.String() != "" {
		t.Fatal("burst logged before the interval elapsed")
	}

	fb.FlushDroppedFrameReport()

	if out := opsBuf.String(); !strings.Contains(out, "Dropped 7 frames") {
		t.Errorf("flush did not report the pending burst:\n%s", out)
	}

	opsBuf.Reset()
	fb.FlushDroppedFrameReport()
	if out := opsBuf.String(); out != "" {
		t.Errorf("flush with nothing pending logged anyway:\n%s", out)
	}
}
