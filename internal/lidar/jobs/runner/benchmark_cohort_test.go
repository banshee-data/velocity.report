//go:build pcap

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

func TestBenchmarkExecutorCannotBypassCohortGuard(t *testing.T) {
	capture, _ := fx.Capture(t, t.TempDir(), "fixture.pcap", "synthetic qualification bytes")
	fx.ConfigureCLI(t, capture, "qualification")
	out := filepath.Join(t.TempDir(), "bundle")
	// No valid tuning or logger is supplied: policy must reject before either
	// can be consumed, independently of the worker's verified job manifest.
	_, err := (benchmarkExecutor{}).Run(context.Background(), jobs.JobRequest{}, Env{
		Captures: []Resolved{{Path: capture}}, BundleDir: out,
	})
	if err == nil || !strings.Contains(err.Error(), "not eligible") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("created output: %v", err)
	}
}
