package recorder

import (
	"strings"
	"sync"
	"testing"
)

// syncBuffer is a concurrency-safe io.Writer for capturing log output.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// restoreLoggers snapshots the package-level loggers and puts them back after
// the test, so wiring writers here does not leak into other tests.
func restoreLoggers(t *testing.T) {
	t.Helper()
	ops, diag, trace := opsLogger, diagLogger, traceLogger
	t.Cleanup(func() {
		opsLogger, diagLogger, traceLogger = ops, diag, trace
	})
}

func TestSetLogWritersRoutesEachStream(t *testing.T) {
	restoreLoggers(t)
	var ops, diag, trace syncBuffer

	SetLogWriters(&ops, &diag, &trace)
	opsf("ops %d", 1)
	diagf("diag %d", 2)
	tracef("trace %d", 3)

	for _, tc := range []struct {
		name string
		buf  *syncBuffer
		want string
	}{
		{"ops", &ops, "ops 1"},
		{"diag", &diag, "diag 2"},
		{"trace", &trace, "trace 3"},
	} {
		got := tc.buf.String()
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s stream = %q, want it to contain %q", tc.name, got, tc.want)
		}
		// Every stream carries the package tag so interleaved logs stay
		// attributable.
		if !strings.Contains(got, "[recorder]") {
			t.Errorf("%s stream = %q, want the [recorder] tag", tc.name, got)
		}
	}
}

func TestSetLogWritersKeepsStreamsSeparate(t *testing.T) {
	restoreLoggers(t)
	var ops, diag, trace syncBuffer

	SetLogWriters(&ops, &diag, &trace)
	opsf("only ops")

	if diag.String() != "" {
		t.Errorf("diag stream = %q, want empty", diag.String())
	}
	if trace.String() != "" {
		t.Errorf("trace stream = %q, want empty", trace.String())
	}
}

func TestSetLogWritersNilWriterDisablesStream(t *testing.T) {
	restoreLoggers(t)
	var ops syncBuffer

	// nil diag/trace is the ops-only configuration the server uses by
	// default; writing to a disabled stream must be a no-op, not a panic.
	SetLogWriters(&ops, nil, nil)
	diagf("dropped")
	tracef("dropped")
	opsf("kept")

	if got := ops.String(); !strings.Contains(got, "kept") {
		t.Errorf("ops stream = %q, want it to contain \"kept\"", got)
	}
}

func TestLogHelpersAreNoOpsBeforeConfiguration(t *testing.T) {
	restoreLoggers(t)
	// A package used before SetLogWriters runs must not panic on a nil logger.
	opsLogger, diagLogger, traceLogger = nil, nil, nil

	opsf("ops")
	diagf("diag")
	tracef("trace")
}
