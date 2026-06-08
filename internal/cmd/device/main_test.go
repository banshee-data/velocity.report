package device

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMainUsage(t *testing.T) {
	if usage == "" {
		t.Fatal("usage string should not be empty")
	}
}

func TestRunVersion(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runVersion()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output == "" {
		t.Fatal("runVersion produced no output")
	}
	if !strings.Contains(output, "velocity") {
		t.Errorf("expected 'velocity' in output, got: %s", output)
	}
}

func captureDeviceMain(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	code = Main(args)

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var outBuf, errBuf bytes.Buffer
	_, _ = outBuf.ReadFrom(rOut)
	_, _ = errBuf.ReadFrom(rErr)
	return code, outBuf.String(), errBuf.String()
}

func TestMainRoutesHelpAndVersionWithoutSideEffects(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"root help", []string{"help"}, 0},
		{"root help flag", []string{"--help"}, 0},
		{"root short help flag", []string{"-h"}, 0},
		{"root version", []string{"version"}, 0},
		{"root version flag", []string{"--version"}, 0},
		{"root short version flag", []string{"-v"}, 0},
		{"check help", []string{"check", "--help"}, 0},
		{"upgrade help", []string{"upgrade", "--help"}, 0},
		{"backup help", []string{"backup", "--help"}, 0},
		{"rollback help", []string{"rollback", "--help"}, 0},
		{"status help", []string{"status", "--help"}, 0},
		{"tailscale help", []string{"tailscale", "--help"}, 0},
		{"install help", []string{"install", "--help"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := captureDeviceMain(t, tc.args)
			if code != tc.wantCode {
				t.Fatalf("Main(%v) = %d, want %d; stderr=%q", tc.args, code, tc.wantCode, stderr)
			}
		})
	}
}

func TestMainReportsUsageAndUnknownCommands(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"no args", nil, "velocity device"},
		{"unknown root command", []string{"bogus"}, "unknown command"},
		{"check parse error", []string{"check", "--bogus"}, "check failed"},
		{"upgrade parse error", []string{"upgrade", "--bogus"}, "upgrade failed"},
		{"backup parse error", []string{"backup", "--bogus"}, "backup failed"},
		{"rollback parse error", []string{"rollback", "--bogus"}, "rollback failed"},
		{"status parse error", []string{"status", "--bogus"}, "status failed"},
		{"tailscale unknown", []string{"tailscale", "bogus"}, "tailscale:"},
		{"install unknown", []string{"install", "bogus"}, "install:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := captureDeviceMain(t, tc.args)
			if code != 1 {
				t.Fatalf("Main(%v) = %d, want 1; stderr=%q", tc.args, code, stderr)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Fatalf("stderr %q does not contain %q", stderr, tc.wantStderr)
			}
		})
	}
}
