//go:build pcap

package lidar

import (
	"os"
	"path/filepath"
	"testing"
)

// benchFixture is the reference multi-frame capture used by the perf gate.
const benchFixture = "../../lidar/perf/pcap/kirk0.pcapng"

// requirePCAPFixture skips unless VELOCITY_PCAP_FIXTURE_TESTS is set. These
// applets replay the whole ~200 MB capture, which takes minutes under -race.
// The flag-validation tests around them still run every time; only the replays
// are opt-in. `make test-go-coverage-gate` sets the variable so the gate still
// measures these paths, and the nightly workflow runs them.
func requirePCAPFixture(t *testing.T) {
	t.Helper()
	if os.Getenv("VELOCITY_PCAP_FIXTURE_TESTS") == "" {
		t.Skip("set VELOCITY_PCAP_FIXTURE_TESTS=1 to replay the kirk0.pcapng fixture")
	}
}

func TestBenchMainRequiresPCAP(t *testing.T) {
	// Exit code 2 is the usage-error code, distinct from 1 (run failure).
	if code := BenchMain(nil); code != 2 {
		t.Errorf("BenchMain(nil) = %d, want 2", code)
	}
}

func TestBenchMainHelpExitsZero(t *testing.T) {
	if code := BenchMain([]string{"-h"}); code != 0 {
		t.Errorf("BenchMain(-h) = %d, want 0", code)
	}
}

func TestBenchMainRejectsUnknownFlag(t *testing.T) {
	if code := BenchMain([]string{"-nope"}); code != 2 {
		t.Errorf("BenchMain(-nope) = %d, want 2", code)
	}
}

func TestBenchMainRejectsUnreadableTuningConfig(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "tuning.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("writing bad config: %v", err)
	}

	// A malformed config is a run failure (1), not a usage error (2).
	code := BenchMain([]string{"-config", bad, "-pcap", benchFixture})
	if code != 1 {
		t.Errorf("BenchMain with a malformed config = %d, want 1", code)
	}
}

func TestBenchMainRejectsMissingPCAP(t *testing.T) {
	code := BenchMain([]string{
		"-pcap", filepath.Join(t.TempDir(), "absent.pcapng"),
		"-output", t.TempDir(), "-quiet",
	})
	if code != 1 {
		t.Errorf("BenchMain with a missing capture = %d, want 1", code)
	}
}

func TestBenchMainRunsBenchmarkOverFixture(t *testing.T) {
	requirePCAPFixture(t)

	dir := t.TempDir()
	out := filepath.Join(dir, "bench.json")

	// Port 0 exercises auto-detection from the capture rather than assuming
	// the fixture's 2369.
	code := BenchMain([]string{
		"-pcap", benchFixture,
		"-output", dir,
		"-benchmark-output", out,
		"-port", "0",
		"-quiet",
	})
	if code != 0 {
		t.Fatalf("BenchMain = %d, want 0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected a benchmark JSON at %s: %v", out, err)
	}
}
