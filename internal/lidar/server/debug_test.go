package server

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
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
	if !strings.Contains(output, "[monitor]") {
		t.Errorf("expected output to contain '[monitor]' prefix, got %q", output)
	}
}

func TestOpsf_WithoutLogger(t *testing.T) {
	SetLogWriters(nil, nil, nil)
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
	tracef("no-op %d", 1)
}

// TestOpsFatalf_Exits covers opsFatalf, which terminates the process on both
// of its branches, so it can only be observed from a child process. The test
// re-executes this binary with VELOCITY_TEST_FATAL set to select the branch:
//
//	"configured"   — a logger is wired, so it logs then calls os.Exit(1)
//	"unconfigured" — no logger, so it falls through to log.Fatalf
func TestOpsFatalf_Exits(t *testing.T) {
	if mode := os.Getenv("VELOCITY_TEST_FATAL"); mode != "" {
		if mode == "configured" {
			SetLogWriters(os.Stdout, nil, nil)
		} else {
			SetLogWriters(nil, nil, nil)
		}
		opsFatalf("fatal: %s", mode)
		t.Fatal("opsFatalf returned, want process exit")
	}

	for _, mode := range []string{"configured", "unconfigured"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestOpsFatalf_Exits$")
			cmd.Env = append(os.Environ(), "VELOCITY_TEST_FATAL="+mode)
			out, err := cmd.CombinedOutput()

			if err == nil {
				t.Fatalf("subprocess exited 0, want non-zero; output:\n%s", out)
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("subprocess error = %v, want *exec.ExitError; output:\n%s", err, out)
			}
			if code := exitErr.ExitCode(); code != 1 {
				t.Errorf("exit code = %d, want 1; output:\n%s", code, out)
			}
			if !strings.Contains(string(out), "fatal: "+mode) {
				t.Errorf("output missing the fatal message; got:\n%s", out)
			}
		})
	}
}
