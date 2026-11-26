//go:build pcap

package lidar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
)

// Exercise the actual shared entry points (also used by the standalone tools).
// Invalid packet bytes prove a denial occurs before port detection or replay.
func TestCohortGuardBeforeReplay(t *testing.T) {
	for _, tool := range []string{"benchmark", "settling"} {
		for _, scenario := range []string{"qualification", "validation", "unknown", "renamed", "held-out", "missing-policy"} {
			t.Run(tool+"/"+scenario, func(t *testing.T) {
				capture, hash := fx.Capture(t, t.TempDir(), "fixture.pcap", "authored non-packet fixture")
				role := "development"
				if scenario == "qualification" || scenario == "renamed" {
					role = "qualification"
				}
				if scenario == "validation" {
					role = "validation"
				}
				root := fx.ConfigureCLI(t, capture, role)
				switch scenario {
				case "unknown":
					capture, _ = fx.Capture(t, t.TempDir(), "unknown.pcap", "different synthetic bytes")
				case "renamed":
					renamed := filepath.Join(t.TempDir(), "looks-development.pcap")
					if err := os.Rename(capture, renamed); err != nil {
						t.Fatal(err)
					}
					capture = renamed
				case "held-out":
					p := fx.Pack(t, root, fx.PackOptions{Capture: "fixture", Run: "old-run", PCAPBasename: filepath.Base(capture)})
					fx.Bind(t, fx.LabelSetDir(t, p), filepath.Base(capture), hash)
					fx.Freeze(t, root, "held-out.json", p, annotation.SplitRoleHeldOut, nil)
				case "missing-policy":
					t.Setenv("VELOCITY_COHORT_MANIFEST", "")
				}
				output := filepath.Join(t.TempDir(), "report.json")
				var code int
				text := captureStderr(t, func() {
					if tool == "benchmark" {
						code = BenchMain([]string{"-pcap", capture, "-benchmark-output", output})
					} else {
						code = SettlingEvalMain([]string{"-output", output, capture})
					}
				})
				if code != 1 || !strings.Contains(text, "not eligible") {
					t.Fatalf("code=%d stderr=%s", code, text)
				}
				reason := map[string]string{
					"qualification":  "blind qualification cohort",
					"validation":     "validation cohort",
					"unknown":        "not in the cohort manifest",
					"renamed":        "blind qualification cohort",
					"held-out":       "held out by frozen split",
					"missing-policy": "no cohort manifest is configured",
				}[scenario]
				if !strings.Contains(text, reason) {
					t.Fatalf("missing reason %q: %s", reason, text)
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("output created: %v", err)
				}
			})
		}
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = f
	defer func() { os.Stderr = old; f.Close() }()
	fn()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
