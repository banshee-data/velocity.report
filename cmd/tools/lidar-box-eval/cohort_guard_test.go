//go:build pcap

package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
)

func TestBoxCohortProcess(t *testing.T) {
	if os.Getenv("VELOCITY_BOX_GUARD_HELPER") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("lidar-box-eval", flag.ExitOnError)
	os.Args = []string{"lidar-box-eval", "-pcap", os.Getenv("VELOCITY_TEST_CAPTURES"), "-out", os.Getenv("VELOCITY_TEST_OUTPUT"), "-rows", os.Getenv("VELOCITY_TEST_OUTPUT") + ".csv"}
	main()
}

func TestBoxCohortChecksEveryCaptureBeforeOutput(t *testing.T) {
	for _, scenario := range []string{"qualification-renamed", "unknown-second"} {
		t.Run(scenario, func(t *testing.T) {
			capture, _ := fx.Capture(t, t.TempDir(), "fixture.pcap", "synthetic capture")
			role := "development"
			if scenario == "qualification-renamed" {
				role = "qualification"
			}
			fx.ConfigureCLI(t, capture, role)
			if scenario == "qualification-renamed" {
				renamed := filepath.Join(t.TempDir(), "dev.pcap")
				if err := os.Rename(capture, renamed); err != nil {
					t.Fatal(err)
				}
				capture = renamed
			} else {
				other, _ := fx.Capture(t, t.TempDir(), "unknown.pcap", "other synthetic capture")
				capture += "," + other
			}
			out := filepath.Join(t.TempDir(), "output.json")
			cmd := exec.Command(os.Args[0], "-test.run=^TestBoxCohortProcess$")
			cmd.Env = append(os.Environ(), "VELOCITY_BOX_GUARD_HELPER=1", "VELOCITY_TEST_CAPTURES="+capture, "VELOCITY_TEST_OUTPUT="+out)
			b, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(b), "not eligible") {
				t.Fatalf("%v: %s", err, b)
			}
			for _, p := range []string{out, out + ".csv"} {
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Fatalf("output created: %v", err)
				}
			}
		})
	}
}
