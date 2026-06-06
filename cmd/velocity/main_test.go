package main

import (
	"os"
	"strings"
	"testing"
)

// runDispatch invokes dispatch with stdout and stderr redirected to pipes and
// returns the exit code plus the captured streams. Only safe (non-server)
// routes should be exercised through it.
func runDispatch(t *testing.T, prog string, args []string) (code int, stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	code = dispatch(prog, args)

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	outBuf := make([]byte, 8192)
	nOut, _ := rOut.Read(outBuf)
	errBuf := make([]byte, 8192)
	nErr, _ := rErr.Read(errBuf)
	return code, string(outBuf[:nOut]), string(errBuf[:nErr])
}

func TestDispatchRouting(t *testing.T) {
	cases := []struct {
		name     string
		prog     string
		args     []string
		wantCode int
	}{
		{"no args prints help", "velocity", nil, 0},
		{"help", "velocity", []string{"help"}, 0},
		{"help flag", "velocity", []string{"--help"}, 0},
		{"version", "velocity", []string{"version"}, 0},
		{"version flag", "velocity", []string{"--version"}, 0},
		{"unknown namespace exits 2", "velocity", []string{"bogus"}, 2},
		{"data without subcommand exits 2", "velocity", []string{"data"}, 2},
		{"report without subcommand exits 2", "velocity", []string{"report"}, 2},
		{"tune without subcommand exits 2", "velocity", []string{"tune"}, 2},
		{"data with unknown subcommand exits 2", "velocity", []string{"data", "bogus"}, 2},
		// argv[0] prefix routing: a suffixed dev/release name still resolves to
		// the server alias; `version` returns 0 without starting the server.
		{"velocity-report-local prefix routes to server", "velocity-report-local", []string{"version"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, _ := runDispatch(t, tc.prog, tc.args)
			if code != tc.wantCode {
				t.Errorf("dispatch(%q, %v) = %d, want %d", tc.prog, tc.args, code, tc.wantCode)
			}
		})
	}
}

func TestDispatchHelpMentionsNamespaces(t *testing.T) {
	_, stdout, _ := runDispatch(t, "velocity", nil)
	for _, ns := range []string{"serve", "device", "data", "report", "tune", "version"} {
		if !strings.Contains(stdout, ns) {
			t.Errorf("top-level help missing namespace %q\nhelp:\n%s", ns, stdout)
		}
	}
}

func TestVelocityCtlShimRemoved(t *testing.T) {
	// The velocity-ctl alias is gone: no deprecation warning, no routing into the
	// device namespace. A bare invocation falls through to the canonical help,
	// and an old `velocity-ctl <verb>` is now an unknown command.
	code, _, stderr := runDispatch(t, "velocity-ctl", nil)
	if strings.Contains(stderr, "deprecated") {
		t.Errorf("velocity-ctl should no longer emit a deprecation warning: %q", stderr)
	}
	if code != 0 {
		t.Errorf("velocity-ctl with no args = %d, want 0 (canonical help)", code)
	}
	if c, _, _ := runDispatch(t, "velocity-ctl", []string{"upgrade"}); c != 2 {
		t.Errorf("velocity-ctl upgrade = %d, want 2 (unknown command)", c)
	}
}
