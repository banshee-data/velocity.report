package l2frames

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
}

func TestSetLogWriters_Disable(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, nil, nil)
	SetLogWriters(nil, nil, nil)

	if opsLogger != nil {
		t.Fatal("opsLogger should be nil after SetLogWriters(nil, nil, nil)")
	}
}

func TestOpsf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	opsf("hello %s %d", "world", 42)

	output := buf.String()
	if !strings.Contains(output, "hello world 42") {
		t.Errorf("expected output to contain 'hello world 42', got %q", output)
	}
	if !strings.Contains(output, "[l2frames]") {
		t.Errorf("expected output to contain '[l2frames]' prefix, got %q", output)
	}
}

func TestOpsf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)

	// Should not panic when no logger is configured.
	opsf("this should be silently discarded: %d", 123)
}

func TestDiagf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, &buf, nil)
	defer SetLogWriters(nil, nil, nil)

	diagf("internal %s", "test")

	output := buf.String()
	if !strings.Contains(output, "internal test") {
		t.Errorf("expected output to contain 'internal test', got %q", output)
	}
}

func TestDiagf_NilLogger(t *testing.T) {
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

func TestTracef_NilLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)

	// Should not panic.
	tracef("no-op %d", 1)
}
