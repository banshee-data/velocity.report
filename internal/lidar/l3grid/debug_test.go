package l3grid

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetDebugLogger_Enable(t *testing.T) {
	var buf bytes.Buffer
	SetDebugLogger(&buf)
	defer SetDebugLogger(nil)

	if debugLogger == nil {
		t.Fatal("debugLogger should be non-nil after SetDebugLogger with a writer")
	}
}

func TestSetDebugLogger_Disable(t *testing.T) {
	var buf bytes.Buffer
	SetDebugLogger(&buf)
	SetDebugLogger(nil)

	if debugLogger != nil {
		t.Fatal("debugLogger should be nil after SetDebugLogger(nil)")
	}
}

func TestDebugf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetDebugLogger(&buf)
	defer SetDebugLogger(nil)

	Debugf("hello %s %d", "world", 42)

	output := buf.String()
	if !strings.Contains(output, "hello world 42") {
		t.Errorf("expected output to contain 'hello world 42', got %q", output)
	}
	if !strings.Contains(output, "[l3grid]") {
		t.Errorf("expected output to contain '[l3grid]' prefix, got %q", output)
	}
}

func TestDebugf_WithoutLogger(t *testing.T) {
	SetDebugLogger(nil)

	// Should not panic when no logger is configured.
	Debugf("this should be silently discarded: %d", 123)
}

func TestDebugf_Internal(t *testing.T) {
	var buf bytes.Buffer
	SetDebugLogger(&buf)
	defer SetDebugLogger(nil)

	debugf("internal %s", "test")

	output := buf.String()
	if !strings.Contains(output, "internal test") {
		t.Errorf("expected output to contain 'internal test', got %q", output)
	}
}

func TestDebugf_Internal_NilLogger(t *testing.T) {
	SetDebugLogger(nil)

	// Should not panic.
	debugf("no-op %d", 1)
}
