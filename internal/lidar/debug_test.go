package lidar

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// TestSetLegacyLogger tests the backward-compatible shim that routes all streams
// to a single writer.
func TestSetLegacyLogger(t *testing.T) {
	defer resetLoggers()

	var buf bytes.Buffer
	SetLegacyLogger(&buf)

	mu.RLock()
	hasOps := opsLogger != nil
	hasDiag := diagLogger != nil
	hasTrace := traceLogger != nil
	mu.RUnlock()

	if !hasOps || !hasDiag || !hasTrace {
		t.Error("SetLegacyLogger() should configure all three streams")
	}

	// Test logging with each stream
	Opsf("ops message: %d", 1)
	Diagf("diag message: %d", 2)
	Tracef("trace message: %d", 3)

	output := buf.String()
	if !strings.Contains(output, "ops message: 1") {
		t.Errorf("SetLegacyLogger output missing ops message, got: %q", output)
	}
	if !strings.Contains(output, "diag message: 2") {
		t.Errorf("SetLegacyLogger output missing diag message, got: %q", output)
	}
	if !strings.Contains(output, "trace message: 3") {
		t.Errorf("SetLegacyLogger output missing trace message, got: %q", output)
	}

	// Test setting to nil (disable logging)
	SetLegacyLogger(nil)

	mu.RLock()
	nilOps := opsLogger == nil
	nilDiag := diagLogger == nil
	nilTrace := traceLogger == nil
	mu.RUnlock()

	if !nilOps || !nilDiag || !nilTrace {
		t.Error("SetLegacyLogger(nil) should clear all three streams")
	}

	// Test logging when disabled (should not panic)
	buf.Reset()
	Opsf("should not appear")
	Diagf("should not appear")
	Tracef("should not appear")

	if buf.Len() > 0 {
		t.Errorf("Output after disabling = %q, want empty", buf.String())
	}
}

// TestExplicitStreams tests that Opsf, Diagf, Tracef route to correct streams.
func TestExplicitStreams(t *testing.T) {
	defer resetLoggers()

	tests := []struct {
		name         string
		setupLogger  bool
		logFunc      func(string, ...interface{})
		format       string
		args         []interface{}
		wantContains string
		wantEmpty    bool
	}{
		{
			name:         "Opsf with logger enabled",
			setupLogger:  true,
			logFunc:      Opsf,
			format:       "error: %s failed",
			args:         []interface{}{"connection"},
			wantContains: "error: connection failed",
		},
		{
			name:         "Diagf with logger enabled",
			setupLogger:  true,
			logFunc:      Diagf,
			format:       "processing frame %d with %d points",
			args:         []interface{}{123, 45678},
			wantContains: "processing frame 123 with 45678 points",
		},
		{
			name:         "Tracef with logger enabled",
			setupLogger:  true,
			logFunc:      Tracef,
			format:       "packet=%d parsed",
			args:         []interface{}{42},
			wantContains: "packet=42 parsed",
		},
		{
			name:        "Opsf with logger disabled",
			setupLogger: false,
			logFunc:     Opsf,
			format:      "this should not appear",
			args:        []interface{}{},
			wantEmpty:   true,
		},
		{
			name:         "special characters",
			setupLogger:  true,
			logFunc:      Diagf,
			format:       "sensor: %s, value: %f%%",
			args:         []interface{}{"sensor-01", 95.5},
			wantContains: "sensor: sensor-01, value: 95.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			if tt.setupLogger {
				SetLegacyLogger(&buf)
			} else {
				SetLegacyLogger(nil)
			}

			tt.logFunc(tt.format, tt.args...)

			output := buf.String()

			if tt.wantEmpty {
				if len(output) > 0 {
					t.Errorf("Expected no output, got: %q", output)
				}
			} else if !strings.Contains(output, tt.wantContains) {
				t.Errorf("Output %q does not contain expected string %q", output, tt.wantContains)
			}
		})
	}
}

// TestThreadSafety tests concurrent access to all three streams.
func TestThreadSafety(t *testing.T) {
	defer resetLoggers()

	var ops, diag, trace bytes.Buffer
	SetLogWriters(LogWriters{Ops: &ops, Diag: &diag, Trace: &trace})

	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				switch j % 3 {
				case 0:
					Opsf("goroutine %d ops %d", id, j)
				case 1:
					Diagf("goroutine %d diag %d", id, j)
				case 2:
					Tracef("goroutine %d trace %d", id, j)
				}
			}
		}(i)
	}

	wg.Wait()

	if ops.Len() == 0 {
		t.Error("Expected ops output from concurrent goroutines, got none")
	}
	if diag.Len() == 0 {
		t.Error("Expected diag output from concurrent goroutines, got none")
	}
	if trace.Len() == 0 {
		t.Error("Expected trace output from concurrent goroutines, got none")
	}
}

// TestFormattingEdgeCases tests edge cases in formatting
func TestFormattingEdgeCases(t *testing.T) {
	defer resetLoggers()

	tests := []struct {
		name   string
		format string
		args   []interface{}
		want   string
	}{
		{
			name:   "no arguments",
			format: "simple message",
			args:   []interface{}{},
			want:   "simple message",
		},
		{
			name:   "empty format",
			format: "",
			args:   []interface{}{},
			want:   "",
		},
		{
			name:   "multiple format specifiers",
			format: "%s: %d points, %.1f meters, %t active",
			args:   []interface{}{"sensor-01", 1000, 15.5, true},
			want:   "sensor-01: 1000 points, 15.5 meters, true active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			SetLegacyLogger(&buf)

			Diagf(tt.format, tt.args...)

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("Diagf() output = %q, want to contain %q", output, tt.want)
			}
		})
	}
}

// TestSetLogWriters tests configuring all three streams independently.
func TestSetLogWriters(t *testing.T) {
	defer resetLoggers()

	var ops, diag, trace bytes.Buffer
	SetLogWriters(LogWriters{Ops: &ops, Diag: &diag, Trace: &trace})

	Opsf("ops event: %s", "restart")
	Diagf("diag event: %d", 42)
	Tracef("trace event: fps=%.1f", 30.0)

	if !strings.Contains(ops.String(), "ops event: restart") {
		t.Errorf("Opsf output = %q, want to contain 'ops event: restart'", ops.String())
	}
	if !strings.Contains(diag.String(), "diag event: 42") {
		t.Errorf("Diagf output = %q, want to contain 'diag event: 42'", diag.String())
	}
	if !strings.Contains(trace.String(), "trace event: fps=30.0") {
		t.Errorf("Tracef output = %q, want to contain 'trace event: fps=30.0'", trace.String())
	}

	// Verify package prefix is present on every line
	for _, line := range strings.Split(strings.TrimSpace(ops.String()), "\n") {
		if !strings.Contains(line, "[lidar] ") {
			t.Errorf("Ops line missing [lidar] prefix: %q", line)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(diag.String()), "\n") {
		if !strings.Contains(line, "[lidar] ") {
			t.Errorf("Diag line missing [lidar] prefix: %q", line)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(trace.String()), "\n") {
		if !strings.Contains(line, "[lidar] ") {
			t.Errorf("Trace line missing [lidar] prefix: %q", line)
		}
	}

	// Verify no cross-contamination
	if strings.Contains(ops.String(), "diag event") || strings.Contains(ops.String(), "trace event") {
		t.Errorf("Ops stream received non-ops messages: %q", ops.String())
	}
	if strings.Contains(diag.String(), "ops event") || strings.Contains(diag.String(), "trace event") {
		t.Errorf("Diag stream received non-diag messages: %q", diag.String())
	}
	if strings.Contains(trace.String(), "ops event") || strings.Contains(trace.String(), "diag event") {
		t.Errorf("Trace stream received non-trace messages: %q", trace.String())
	}
}

// TestSetLogWriter tests configuring individual streams.
func TestSetLogWriter(t *testing.T) {
	defer resetLoggers()

	var ops bytes.Buffer
	SetLogWriter(LogOps, &ops)

	Opsf("ops only: %s", "alert")
	Diagf("should be silent")
	Tracef("should be silent too")

	if !strings.Contains(ops.String(), "ops only: alert") {
		t.Errorf("Opsf output = %q, want to contain 'ops only: alert'", ops.String())
	}
}

// TestSetLogWriterInvalidLevel tests that an invalid LogLevel panics.
func TestSetLogWriterInvalidLevel(t *testing.T) {
	defer resetLoggers()
	defer func() {
		if r := recover(); r == nil {
			t.Error("SetLogWriter with invalid LogLevel should panic")
		}
	}()

	var buf bytes.Buffer
	SetLogWriter(LogLevel(99), &buf)
}

// TestNilWriterSafety tests that nil writers do not cause panics.
func TestNilWriterSafety(t *testing.T) {
	defer resetLoggers()

	// All nil
	SetLogWriters(LogWriters{})
	Opsf("should not panic: %s", "nil ops")
	Diagf("should not panic: %s", "nil diag")
	Tracef("should not panic: %s", "nil trace")

	// Partial nil
	var buf bytes.Buffer
	SetLogWriters(LogWriters{Ops: &buf})
	Opsf("ops ok")
	Diagf("silent")
	Tracef("silent")
}

// TestConcurrentStreamWrites tests concurrent writes across all three streams.
func TestConcurrentStreamWrites(t *testing.T) {
	defer resetLoggers()

	var ops, diag, trace bytes.Buffer
	SetLogWriters(LogWriters{Ops: &ops, Diag: &diag, Trace: &trace})

	var wg sync.WaitGroup
	n := 50

	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			Opsf("ops %d", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			Diagf("diag %d", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			Tracef("trace %d", i)
		}
	}()

	wg.Wait()

	if ops.Len() == 0 {
		t.Error("expected ops output from concurrent writes")
	}
	if diag.Len() == 0 {
		t.Error("expected diag output from concurrent writes")
	}
	if trace.Len() == 0 {
		t.Error("expected trace output from concurrent writes")
	}
}

// resetLoggers clears all loggers to a clean state for test isolation.
func resetLoggers() {
	mu.Lock()
	opsLogger = nil
	diagLogger = nil
	traceLogger = nil
	mu.Unlock()
}
