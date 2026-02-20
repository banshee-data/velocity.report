package lidar

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// TestSetDebugLogger tests the backward-compatible shim that routes all streams
// to a single writer.
func TestSetDebugLogger(t *testing.T) {
	defer resetLoggers()

	var buf bytes.Buffer
	SetDebugLogger(&buf)

	mu.RLock()
	hasOps := opsLogger != nil
	hasDebug := debugLogger != nil
	hasTrace := traceLogger != nil
	mu.RUnlock()

	if !hasOps || !hasDebug || !hasTrace {
		t.Error("SetDebugLogger() should configure all three streams")
	}

	// Test logging with the installed logger
	debugf("test message: %d", 42)

	output := buf.String()
	if !strings.Contains(output, "test message: 42") {
		t.Errorf("Debug output = %q, want to contain 'test message: 42'", output)
	}

	// Test setting to nil (disable logging)
	SetDebugLogger(nil)

	mu.RLock()
	nilOps := opsLogger == nil
	nilDebug := debugLogger == nil
	nilTrace := traceLogger == nil
	mu.RUnlock()

	if !nilOps || !nilDebug || !nilTrace {
		t.Error("SetDebugLogger(nil) should clear all three streams")
	}

	// Test logging when disabled (should not panic)
	buf.Reset()
	debugf("should not appear")

	if buf.Len() > 0 {
		t.Errorf("Debug output after disabling = %q, want empty", buf.String())
	}
}

// TestDebugf tests the internal debugf function
func TestDebugf(t *testing.T) {
	defer resetLoggers()

	tests := []struct {
		name         string
		setupLogger  bool
		format       string
		args         []interface{}
		wantContains string
		wantEmpty    bool
	}{
		{
			name:         "with logger enabled",
			setupLogger:  true,
			format:       "processing frame %d with %d points",
			args:         []interface{}{123, 45678},
			wantContains: "processing frame 123 with 45678 points",
		},
		{
			name:        "with logger disabled",
			setupLogger: false,
			format:      "this should not appear",
			args:        []interface{}{},
			wantEmpty:   true,
		},
		{
			name:         "with special characters",
			setupLogger:  true,
			format:       "sensor: %s, value: %f%%",
			args:         []interface{}{"sensor-01", 95.5},
			wantContains: "sensor: sensor-01, value: 95.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			if tt.setupLogger {
				SetDebugLogger(&buf)
			} else {
				SetDebugLogger(nil)
			}

			debugf(tt.format, tt.args...)

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

// TestDebugf_Exported tests the exported Debugf function
func TestDebugf_Exported(t *testing.T) {
	defer resetLoggers()

	var buf bytes.Buffer
	SetDebugLogger(&buf)

	// Test exported Debugf function
	Debugf("exported debug: %s = %d", "count", 999)

	output := buf.String()
	if !strings.Contains(output, "exported debug: count = 999") {
		t.Errorf("Debugf() output = %q, want to contain 'exported debug: count = 999'", output)
	}
}

// TestDebugLogger_ThreadSafety tests concurrent access to debug logger
func TestDebugLogger_ThreadSafety(t *testing.T) {
	defer resetLoggers()

	var buf bytes.Buffer
	SetDebugLogger(&buf)

	// Run concurrent logging operations
	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				Debugf("goroutine %d message %d", id, j)
			}
		}(i)
	}

	wg.Wait()

	// Verify we got output (exact content may vary due to concurrent writes)
	output := buf.String()
	if len(output) == 0 {
		t.Error("Expected debug output from concurrent goroutines, got none")
	}

	// Verify some expected strings are present
	if !strings.Contains(output, "goroutine") || !strings.Contains(output, "message") {
		t.Errorf("Expected concurrent log output to contain 'goroutine' and 'message', got: %q", output)
	}
}

// TestDebugLogger_FormattingEdgeCases tests edge cases in formatting
func TestDebugLogger_FormattingEdgeCases(t *testing.T) {
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
			SetDebugLogger(&buf)

			debugf(tt.format, tt.args...)

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("debugf() output = %q, want to contain %q", output, tt.want)
			}
		})
	}
}

// --- New tests for the three-stream model ---

// TestSetLogWriters tests configuring all three streams independently.
func TestSetLogWriters(t *testing.T) {
	defer resetLoggers()

	var ops, dbg, trace bytes.Buffer
	SetLogWriters(LogWriters{Ops: &ops, Debug: &dbg, Trace: &trace})

	Opsf("ops event: %s", "restart")
	Diagf("diag event: %d", 42)
	Tracef("trace event: fps=%.1f", 30.0)

	if !strings.Contains(ops.String(), "ops event: restart") {
		t.Errorf("Opsf output = %q, want to contain 'ops event: restart'", ops.String())
	}
	if !strings.Contains(dbg.String(), "diag event: 42") {
		t.Errorf("Diagf output = %q, want to contain 'diag event: 42'", dbg.String())
	}
	if !strings.Contains(trace.String(), "trace event: fps=30.0") {
		t.Errorf("Tracef output = %q, want to contain 'trace event: fps=30.0'", trace.String())
	}

	// Verify no cross-contamination
	if strings.Contains(ops.String(), "diag event") || strings.Contains(ops.String(), "trace event") {
		t.Errorf("Ops stream received non-ops messages: %q", ops.String())
	}
	if strings.Contains(dbg.String(), "ops event") || strings.Contains(dbg.String(), "trace event") {
		t.Errorf("Debug stream received non-debug messages: %q", dbg.String())
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

// TestClassifierRouting tests that Debugf routes through the keyword classifier.
func TestClassifierRouting(t *testing.T) {
	defer resetLoggers()

	tests := []struct {
		name       string
		format     string
		wantStream string // "ops", "debug", or "trace"
	}{
		// Ops keywords
		{name: "error keyword", format: "Error forwarding packet: %v", wantStream: "ops"},
		{name: "failed keyword", format: "connection failed: retry", wantStream: "ops"},
		{name: "dropped keyword", format: "Dropped 5 forwarded packets", wantStream: "ops"},
		{name: "timeout keyword", format: "sensor timeout after 30s", wantStream: "ops"},
		{name: "warn keyword", format: "warning: buffer near capacity", wantStream: "ops"},
		{name: "fatal keyword", format: "fatal: cannot open device", wantStream: "ops"},
		{name: "panic keyword", format: "recovered from panic in handler", wantStream: "ops"},

		// Trace keywords
		{name: "packet keyword", format: "PCAP parsed points: packet=%d", wantStream: "trace"},
		{name: "fps keyword", format: "Stats: fps=30.1 frames=%d", wantStream: "trace"},
		{name: "progress keyword", format: "PCAP real-time replay progress: 45%%", wantStream: "trace"},
		{name: "queued keyword", format: "queued 128 points for processing", wantStream: "trace"},
		{name: "parsed keyword", format: "parsed 1024 bytes from UDP", wantStream: "trace"},
		{name: "bandwidth keyword", format: "bandwidth utilisation: 85%%", wantStream: "trace"},
		{name: "frame= keyword", format: "frame=42 completed with 1200 points", wantStream: "trace"},

		// Debug (default)
		{name: "cluster count", format: "Clustered into %d objects", wantStream: "debug"},
		{name: "track count", format: "%d confirmed tracks active", wantStream: "debug"},
		{name: "state transition", format: "background settling complete", wantStream: "debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ops, dbg, trace bytes.Buffer
			SetLogWriters(LogWriters{Ops: &ops, Debug: &dbg, Trace: &trace})

			Debugf(tt.format, 1) // pass dummy arg

			switch tt.wantStream {
			case "ops":
				if ops.Len() == 0 {
					t.Errorf("expected ops output for %q, got none", tt.format)
				}
				if dbg.Len() > 0 || trace.Len() > 0 {
					t.Errorf("expected only ops output, got debug=%q trace=%q", dbg.String(), trace.String())
				}
			case "trace":
				if trace.Len() == 0 {
					t.Errorf("expected trace output for %q, got none", tt.format)
				}
				if ops.Len() > 0 || dbg.Len() > 0 {
					t.Errorf("expected only trace output, got ops=%q debug=%q", ops.String(), dbg.String())
				}
			case "debug":
				if dbg.Len() == 0 {
					t.Errorf("expected debug output for %q, got none", tt.format)
				}
				if ops.Len() > 0 || trace.Len() > 0 {
					t.Errorf("expected only debug output, got ops=%q trace=%q", ops.String(), trace.String())
				}
			}
		})
	}
}

// TestClassifyMessage tests the internal classifier directly.
func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		format string
		want   LogLevel
	}{
		{"Error: something broke", LogOps},
		{"CONNECTION FAILED", LogOps},
		{"packet received from sensor", LogTrace},
		{"fps=29.97 frames rendered", LogTrace},
		{"normal diagnostic line", LogDebug},
		{"", LogDebug},
	}

	for _, tt := range tests {
		got := classifyMessage(tt.format)
		if got != tt.want {
			t.Errorf("classifyMessage(%q) = %d, want %d", tt.format, got, tt.want)
		}
	}
}

// TestOpsKeywordPriority verifies that ops keywords take priority over trace keywords
// when both appear in the same message.
func TestOpsKeywordPriority(t *testing.T) {
	defer resetLoggers()

	var ops, dbg, trace bytes.Buffer
	SetLogWriters(LogWriters{Ops: &ops, Debug: &dbg, Trace: &trace})

	// "Error" (ops) + "packet" (trace) — ops should win
	Debugf("Error forwarding packet: connection reset")

	if ops.Len() == 0 {
		t.Error("expected ops output when both ops and trace keywords present")
	}
	if trace.Len() > 0 {
		t.Error("trace should not receive message when ops keyword is present")
	}
}

// TestNilWriterSafety tests that nil writers do not cause panics.
func TestNilWriterSafety(t *testing.T) {
	defer resetLoggers()

	// All nil
	SetLogWriters(LogWriters{})
	Opsf("should not panic: %s", "nil ops")
	Diagf("should not panic: %s", "nil debug")
	Tracef("should not panic: %s", "nil trace")
	Debugf("should not panic: %s", "nil debugf")

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

	var ops, dbg, trace bytes.Buffer
	SetLogWriters(LogWriters{Ops: &ops, Debug: &dbg, Trace: &trace})

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
	if dbg.Len() == 0 {
		t.Error("expected debug output from concurrent writes")
	}
	if trace.Len() == 0 {
		t.Error("expected trace output from concurrent writes")
	}
}

// resetLoggers clears all loggers to a clean state for test isolation.
func resetLoggers() {
	mu.Lock()
	opsLogger = nil
	debugLogger = nil
	traceLogger = nil
	mu.Unlock()
}
