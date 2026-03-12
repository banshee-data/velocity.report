package l6objects

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

func TestTracef_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogWriters(nil, nil, &buf)
	defer SetLogWriters(nil, nil, nil)

	tracef("classified %s", "car")

	output := buf.String()
	if !strings.Contains(output, "classified car") {
		t.Fatalf("expected output to contain message, got %q", output)
	}
	if !strings.Contains(output, "[l6objects]") {
		t.Fatalf("expected output to contain package prefix, got %q", output)
	}
}
