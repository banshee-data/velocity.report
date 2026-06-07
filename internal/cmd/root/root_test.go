package root

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func runDispatch(t *testing.T, prog string, args []string) (code int, stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	code = Dispatch(prog, args)

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
		{"short version flag", "velocity", []string{"-v"}, 0},
		{"unknown namespace exits 2", "velocity", []string{"bogus"}, 2},
		{"data without subcommand exits 2", "velocity", []string{"data"}, 2},
		{"report without subcommand exits 2", "velocity", []string{"report"}, 2},
		{"tune without subcommand exits 2", "velocity", []string{"tune"}, 2},
		{"data with unknown subcommand exits 2", "velocity", []string{"data", "bogus"}, 2},
		{"report with unknown subcommand exits 2", "velocity", []string{"report", "bogus"}, 2},
		{"tune with unknown subcommand exits 2", "velocity", []string{"tune", "bogus"}, 2},
		{"velocity-report-local prefix routes to server", "velocity-report-local", []string{"version"}, 0},
		{"velocity-report serve prefix strips serve", "velocity-report", []string{"serve", "version"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, _ := runDispatch(t, tc.prog, tc.args)
			if code != tc.wantCode {
				t.Errorf("Dispatch(%q, %v) = %d, want %d", tc.prog, tc.args, code, tc.wantCode)
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

func TestDispatchRoutesNamespacesToApplets(t *testing.T) {
	oldServer, oldDevice, oldTune := serverMain, deviceMain, tuneMain
	defer func() {
		serverMain = oldServer
		deviceMain = oldDevice
		tuneMain = oldTune
	}()

	var calls []struct {
		name string
		args []string
	}
	serverMain = func(args []string) int {
		calls = append(calls, struct {
			name string
			args []string
		}{"server", append([]string(nil), args...)})
		return 10
	}
	deviceMain = func(args []string) int {
		calls = append(calls, struct {
			name string
			args []string
		}{"device", append([]string(nil), args...)})
		return 11
	}
	tuneMain = func(args []string) int {
		calls = append(calls, struct {
			name string
			args []string
		}{"tune", append([]string(nil), args...)})
		return 12
	}

	cases := []struct {
		args     []string
		wantCode int
		wantName string
		wantArgs []string
	}{
		{[]string{"serve", "--version"}, 10, "server", []string{"--version"}},
		{[]string{"device", "status"}, 11, "device", []string{"status"}},
		{[]string{"data", "migrate", "up"}, 10, "server", []string{"migrate", "up"}},
		{[]string{"data", "transits", "list"}, 10, "server", []string{"transits", "list"}},
		{[]string{"data", "sql", "SELECT 1"}, 10, "server", []string{"sql", "SELECT 1"}},
		{[]string{"report", "pdf", "--version"}, 10, "server", []string{"pdf", "--version"}},
		{[]string{"tune", "sweep", "--dry-run"}, 12, "tune", []string{"--dry-run"}},
	}

	for i, tc := range cases {
		if got := Dispatch("velocity", tc.args); got != tc.wantCode {
			t.Fatalf("Dispatch(%v) = %d, want %d", tc.args, got, tc.wantCode)
		}
		if calls[i].name != tc.wantName || !reflect.DeepEqual(calls[i].args, tc.wantArgs) {
			t.Fatalf("call %d = %s %v, want %s %v", i, calls[i].name, calls[i].args, tc.wantName, tc.wantArgs)
		}
	}
}

func TestVelocityCtlShimRemoved(t *testing.T) {
	code, _, stderr := runDispatch(t, "velocity-ctl", nil)
	if strings.Contains(stderr, "deprecated") {
		t.Errorf("velocity-ctl should no longer emit a deprecation warning: %q", stderr)
	}
	if code != 0 {
		t.Errorf("velocity-ctl with no args = %d, want 0", code)
	}
	if c, _, _ := runDispatch(t, "velocity-ctl", []string{"upgrade"}); c != 2 {
		t.Errorf("velocity-ctl upgrade = %d, want 2", c)
	}
}
