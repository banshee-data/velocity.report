//go:build pcap

package lidar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benchFixture is the reference multi-frame capture used by the perf gate.
const benchFixture = "../../lidar/perf/pcap/kirk0.pcapng"

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
	dir := t.TempDir()
	out := filepath.Join(dir, "bench.json")

	// Port 0 exercises auto-detection from the capture rather than assuming
	// the fixture's 2369.
	code := BenchMain([]string{
		"-pcap", benchFixture,
		"-output", dir,
		"-benchmark-output", out,
		"-port", "0",
		"-duration-seconds", "10",
		// The frame budget is a production-throughput assertion and this test
		// binary runs under -race with coverage instrumentation, several times
		// slower than the binary the perf gate measures. Enforcing it here
		// would assert something about the test harness, not the pipeline.
		"-max-frames-over-budget-pct", "100",
		"-quiet",
	})
	if code != 0 {
		t.Fatalf("BenchMain = %d, want 0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected a benchmark JSON at %s: %v", out, err)
	}
}

func TestBenchMainRejectsClusterDumpWithRepeats(t *testing.T) {
	// Each repeat would rewrite the dump, so the file would describe one
	// arbitrary run while the JSON reported the median of all of them.
	code := BenchMain([]string{
		"-pcap", benchFixture, "-output", t.TempDir(),
		"-clusters-output", filepath.Join(t.TempDir(), "clusters.jsonl"),
		"-repeat", "3", "-quiet",
	})
	if code != 2 {
		t.Errorf("BenchMain with -clusters-output and -repeat 3 = %d, want 2", code)
	}
}

func TestBenchExperimentsAcceptsOnlyBackgroundExperiments(t *testing.T) {
	got, err := benchExperiments("support_filter,no_region_overrides")
	if err != nil {
		t.Fatalf("background experiments refused: %v", err)
	}
	if len(got) != 2 || got[0] != "no_region_overrides" || got[1] != "support_filter" {
		t.Errorf("got %v, want the sorted pair", got)
	}
	if got, err := benchExperiments(""); err != nil || len(got) != 0 {
		t.Errorf("empty flag: %v, %v; want none and no error", got, err)
	}
	// A tracker experiment would run as the baseline in the bench and be
	// reported as the option, so it is refused rather than ignored.
	for _, name := range []string{"cascade", "solid_body", "not_an_experiment"} {
		if _, err := benchExperiments(name); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestBenchMainRejectsTrackerExperiment(t *testing.T) {
	if code := BenchMain([]string{"-pcap", benchFixture, "-experiment", "cascade"}); code != 2 {
		t.Errorf("BenchMain(-experiment cascade) = %d, want 2", code)
	}
}

func TestBenchMainRunsABackgroundExperiment(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bench.json")
	code := BenchMain([]string{
		"-pcap", benchFixture, "-output", dir, "-benchmark-output", out,
		"-duration-seconds", "2", "-max-frames-over-budget-pct", "100", "-quiet",
		"-experiment", "no_region_overrides",
	})
	if code != 0 {
		t.Fatalf("BenchMain = %d, want 0", code)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"experiments"`) || !strings.Contains(string(raw), `"no_region_overrides"`) {
		t.Errorf("benchmark JSON does not record the experiment:\n%s", raw)
	}
	if code := BenchMain([]string{"-pcap", benchFixture, "-experiment", "no_region_overrides,region_overrides_quiet_only"}); code != 2 {
		t.Errorf("contradictory scopes: BenchMain = %d, want 2", code)
	}
}
