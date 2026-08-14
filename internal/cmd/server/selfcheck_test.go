package server

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckDNSLocalhostResolves(t *testing.T) {
	// localhost must resolve on any machine that can run this test at all;
	// a failure here is exactly the broken-static-build case the check exists
	// to catch.
	if err := checkDNSLocalhost(t.Context()); err != nil {
		t.Fatalf("checkDNSLocalhost: %v", err)
	}
}

func TestCheckDNSLocalhostFailsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := checkDNSLocalhost(ctx)
	if err == nil {
		t.Fatal("checkDNSLocalhost with cancelled context succeeded, want error")
	}
	if !strings.Contains(err.Error(), "LookupHost(localhost)") {
		t.Errorf("error = %v, want it to name the failing lookup", err)
	}
}

func TestCheckDNSExternalFailsOnCancelledContext(t *testing.T) {
	// The success path needs the internet, so only the failure path is
	// asserted here; runSelfCheck treats this check as best-effort.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := checkDNSExternal(ctx)
	if err == nil {
		t.Fatal("checkDNSExternal with cancelled context succeeded, want error")
	}
	if !strings.Contains(err.Error(), "offline?") {
		t.Errorf("error = %v, want the offline hint", err)
	}
}

func TestCheckUDPBind(t *testing.T) {
	if err := checkUDPBind(t.Context()); err != nil {
		t.Fatalf("checkUDPBind: %v", err)
	}
}

func TestCheckTCPBind(t *testing.T) {
	if err := checkTCPBind(t.Context()); err != nil {
		t.Fatalf("checkTCPBind: %v", err)
	}
}

func TestSelfCheckReportRunClassifiesOutcomes(t *testing.T) {
	tests := []struct {
		name              string
		critical          bool
		err               error
		wantPassed        int
		wantFailed        int
		wantWarned        int
		wantOutputContain string
	}{
		{
			name:              "success passes regardless of criticality",
			critical:          true,
			err:               nil,
			wantPassed:        1,
			wantOutputContain: "PASS check-name",
		},
		{
			name:              "critical failure counts as failed",
			critical:          true,
			err:               errors.New("boom"),
			wantFailed:        1,
			wantOutputContain: "FAIL check-name: boom",
		},
		{
			name:              "non-critical failure counts as warning",
			critical:          false,
			err:               errors.New("offline"),
			wantWarned:        1,
			wantOutputContain: "WARN check-name: offline",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := &selfCheckReport{out: &out}

			r.run("check-name", tc.critical, func(context.Context) error { return tc.err })

			if r.passed != tc.wantPassed || r.failed != tc.wantFailed || r.warned != tc.wantWarned {
				t.Errorf("counters = passed:%d failed:%d warned:%d, want passed:%d failed:%d warned:%d",
					r.passed, r.failed, r.warned, tc.wantPassed, tc.wantFailed, tc.wantWarned)
			}
			if !strings.Contains(out.String(), tc.wantOutputContain) {
				t.Errorf("output = %q, want it to contain %q", out.String(), tc.wantOutputContain)
			}
		})
	}
}

func TestSelfCheckReportRunPassesDeadlineToCheck(t *testing.T) {
	var out strings.Builder
	r := &selfCheckReport{out: &out}

	var hadDeadline bool
	r.run("deadline", true, func(ctx context.Context) error {
		_, hadDeadline = ctx.Deadline()
		return nil
	})

	// Each check is bounded so a wedged resolver cannot hang the self-check.
	if !hadDeadline {
		t.Error("check ran without a deadline, want a bounded context")
	}
}

func TestRunSelfCheckReportsHeaderAndSummary(t *testing.T) {
	var out strings.Builder

	// No live interface, so the pcap live-capture check is skipped.
	code := runSelfCheck(&out, "")

	got := out.String()
	for _, want := range []string{
		"velocity-report self-check",
		"version:",
		"go:",
		"dns-localhost",
		"udp-bind",
		"tcp-bind",
		"result:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
	// The critical checks (localhost DNS, UDP bind, TCP bind) all pass on a
	// working host, and external DNS only ever warns, so the run is clean
	// even on an offline machine.
	if code != 0 {
		t.Errorf("runSelfCheck = %d, want 0; output:\n%s", code, got)
	}
}

func TestRunSelfCheckFailsWhenLiveCaptureRequestedWithoutPcap(t *testing.T) {
	// This test binary is built without the pcap tag, so requesting a live
	// capture must fail loudly rather than silently reporting success.
	var out strings.Builder

	code := runSelfCheck(&out, "eth0")

	if code != 1 {
		t.Errorf("runSelfCheck = %d, want 1 when live capture is unavailable", code)
	}
	if !strings.Contains(out.String(), "built without the pcap tag") {
		t.Errorf("output = %q, want the missing-pcap-tag diagnostic", out.String())
	}
}
