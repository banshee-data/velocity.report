package parse

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetLogWriters_Enable(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	if opsLogger == nil {
		t.Fatal("opsLogger should be non-nil after SetLogWriters with a writer")
	}
	if diagLogger != nil {
		t.Fatal("diagLogger should be nil when passed nil writer")
	}
	if traceLogger != nil {
		t.Fatal("traceLogger should be nil when passed nil writer")
	}
}

func TestSetLogWriters_AllStreams(t *testing.T) {
	var ops, diag, trace bytes.Buffer
	SetLogWriters(&ops, &diag, &trace)
	defer SetLogWriters(nil, nil, nil)

	if opsLogger == nil || diagLogger == nil || traceLogger == nil {
		t.Fatal("all loggers should be non-nil")
	}
}

func TestSetLogWriters_Disable(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, &buf, &buf)
	SetLogWriters(nil, nil, nil)

	if opsLogger != nil || diagLogger != nil || traceLogger != nil {
		t.Fatal("all loggers should be nil after SetLogWriters(nil, nil, nil)")
	}
}

func TestNewLogger_NilWriter(t *testing.T) {
	logger := newLogger("[test] ", nil)
	if logger != nil {
		t.Error("expected nil logger for nil writer")
	}
}

func TestNewLogger_NonNilWriter(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger("[test] ", &buf)
	if logger == nil {
		t.Fatal("expected non-nil logger for non-nil writer")
	}
	logger.Printf("hello %d", 42)
	if !strings.Contains(buf.String(), "hello 42") {
		t.Errorf("expected output to contain 'hello 42', got %q", buf.String())
	}
}

func TestOpsf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	opsf("test %s %d", "msg", 1)

	output := buf.String()
	if !strings.Contains(output, "test msg 1") {
		t.Errorf("expected output to contain 'test msg 1', got %q", output)
	}
	if !strings.Contains(output, "[parse]") {
		t.Errorf("expected output to contain '[parse]' prefix, got %q", output)
	}
}

func TestOpsf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Should not panic when no logger is configured.
	opsf("silently discarded: %d", 123)
}

func TestDiagf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, &buf, nil)
	defer SetLogWriters(nil, nil, nil)

	diagf("diag %s", "event")

	output := buf.String()
	if !strings.Contains(output, "diag event") {
		t.Errorf("expected output to contain 'diag event', got %q", output)
	}
}

func TestDiagf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Should not panic.
	diagf("no-op %d", 1)
}

func TestTracef_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, nil, &buf)
	defer SetLogWriters(nil, nil, nil)

	tracef("trace %s", "event")

	output := buf.String()
	if !strings.Contains(output, "trace event") {
		t.Errorf("expected output to contain 'trace event', got %q", output)
	}
}

func TestTracef_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Should not panic.
	tracef("no-op %d", 1)
}
