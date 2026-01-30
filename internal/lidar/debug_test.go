package lidar

import (
	"bytes"
	"strings"
	"testing"
)

// TestSetDebugLogger tests installing a debug logger
func TestSetDebugLogger(t *testing.T) {
	// Save original state
	originalLogger := debugLogger
	defer func() {
		debugLogger = originalLogger
	}()

	// Test setting a debug logger
	var buf bytes.Buffer
	SetDebugLogger(&buf)

	if debugLogger == nil {
		t.Error("SetDebugLogger() failed to set debugLogger")
	}

	// Test logging with the installed logger
	debugf("test message: %d", 42)

	output := buf.String()
	if !strings.Contains(output, "test message: 42") {
		t.Errorf("Debug output = %q, want to contain 'test message: 42'", output)
	}

	// Test setting to nil (disable logging)
	SetDebugLogger(nil)

	if debugLogger != nil {
		t.Error("SetDebugLogger(nil) failed to clear debugLogger")
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
	// Save original state
	originalLogger := debugLogger
	defer func() {
		debugLogger = originalLogger
	}()

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
	// Save original state
	originalLogger := debugLogger
	defer func() {
		debugLogger = originalLogger
	}()

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
	// Save original state
	originalLogger := debugLogger
	defer func() {
		debugLogger = originalLogger
	}()

	var buf bytes.Buffer
	SetDebugLogger(&buf)

	// Run concurrent logging operations
	done := make(chan bool)
	numGoroutines := 10
	messagesPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				Debugf("goroutine %d message %d", id, j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

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
	// Save original state
	originalLogger := debugLogger
	defer func() {
		debugLogger = originalLogger
	}()

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
