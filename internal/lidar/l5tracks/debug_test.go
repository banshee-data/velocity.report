package l5tracks

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

func TestOpsf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(&buf, nil, nil)
	defer SetLogWriters(nil, nil, nil)

	opsf("track %s deleted", "trk_1")

	output := buf.String()
	if !strings.Contains(output, "track trk_1 deleted") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l5tracks]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}

func TestOpsf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Must not panic.
	opsf("no-op %d", 1)
}

func TestDiagf_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, &buf, nil)
	defer SetLogWriters(nil, nil, nil)

	diagf("association cost %f", 0.42)

	output := buf.String()
	if !strings.Contains(output, "association cost") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l5tracks]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}

func TestDiagf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Must not panic.
	diagf("no-op %d", 1)
}

func TestTracef_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, nil, &buf)
	defer SetLogWriters(nil, nil, nil)

	tracef("kalman step %d", 3)

	output := buf.String()
	if !strings.Contains(output, "kalman step 3") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l5tracks]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}

func TestTracef_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
	// Must not panic.
	tracef("no-op %d", 1)
}
