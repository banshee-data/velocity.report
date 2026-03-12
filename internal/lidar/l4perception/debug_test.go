package l4perception

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
	if diagLogger != nil || traceLogger != nil {
		t.Fatal("diag/trace loggers should be nil when passed nil writers")
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

func TestDiagf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, &buf, nil)
	defer SetLogWriters(nil, nil, nil)

	diagf("clustered %d points", 42)

	output := buf.String()
	if !strings.Contains(output, "clustered 42 points") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l4perception]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}

func TestOpsf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	opsf("cluster count %d", 5)

	output := buf.String()
	if !strings.Contains(output, "cluster count 5") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l4perception]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}

func TestOpsf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Must not panic.
	opsf("no-op %d", 1)
}

func TestTracef_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, nil, &buf)
	defer SetLogWriters(nil, nil, nil)

	tracef("point %d", 7)

	output := buf.String()
	if !strings.Contains(output, "point 7") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l4perception]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}

func TestTracef_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Must not panic.
	tracef("no-op %d", 1)
}
