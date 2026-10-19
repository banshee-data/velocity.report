//go:build pcap

package lidarbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPUSlug(t *testing.T) {
	for model, want := range map[string]string{
		"AMD EPYC 7763 64-Core Processor":               "amd-epyc-7763",
		"AMD EPYC 9V74 80-Core Processor":               "amd-epyc-9v74",
		"Intel(R) Xeon(R) Platinum 8370C CPU @ 2.80GHz": "intel-xeon-platinum-8370c",
		"Apple M1 Pro":                   "apple-m1-pro",
		"Raspberry Pi 5 Model B Rev 1.0": "raspberry-pi-5-model-b",
		"Raspberry Pi 4 Model B Rev 1.4": "raspberry-pi-4-model-b",
		"":                               "",
	} {
		if got := cpuSlug(model); got != want {
			t.Errorf("cpuSlug(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestPerCPUBaselinePath(t *testing.T) {
	got := perCPUBaselinePath("internal/lidar/perf/baseline/baseline-kirk0-full-ci.json", "AMD EPYC 7763 64-Core Processor")
	if want := "internal/lidar/perf/baseline/baseline-kirk0-full-ci-amd-epyc-7763.json"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := perCPUBaselinePath("baseline-kirk0-full-ci.json", ""); got != "" {
		t.Errorf("an unnamed CPU gave %q, want no per-CPU path", got)
	}
}

// writeBaselineAt writes doc to path, for baselines that must carry a given name.
func writeBaselineAt(t *testing.T, path string, doc *BenchmarkResult) {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestChooseBaseline: a run compares against the baseline captured on its own
// CPU when one exists and measured the same workload, and against the class
// baseline otherwise, saying which and why.
func TestChooseBaseline(t *testing.T) {
	const epyc = "AMD EPYC 7763 64-Core Processor"
	info := SystemInfo{GOOS: "linux", GOARCH: "amd64", HostClass: HostClassCI, NumCPU: 4, GoVersion: "go1.27.1"}
	run := func(model string) *BenchmarkResult {
		r := comparableResult()
		r.SystemInfo = info
		r.SystemInfo.CPUModel = model
		return r
	}
	setup := func(t *testing.T, cpuDoc *BenchmarkResult) string {
		dir := t.TempDir()
		class := filepath.Join(dir, "baseline-kirk0-full-ci.json")
		writeBaselineAt(t, class, run("AMD EPYC 9V74 80-Core Processor"))
		if cpuDoc != nil {
			writeBaselineAt(t, filepath.Join(dir, "baseline-kirk0-full-ci-amd-epyc-7763.json"), cpuDoc)
		}
		return class
	}

	t.Run("this CPU's baseline", func(t *testing.T) {
		class := setup(t, run(epyc))
		got, err := chooseBaseline(class, run(epyc), DefaultWorkTolerance)
		if err != nil || got.Identity != nil {
			t.Fatalf("err %v, identity %v", err, got.Identity)
		}
		if filepath.Base(got.Path) != "baseline-kirk0-full-ci-amd-epyc-7763.json" || !strings.Contains(got.Notice, "this runner's CPU") {
			t.Errorf("chose %s: %s", got.Path, got.Notice)
		}
	})
	t.Run("no baseline for this CPU", func(t *testing.T) {
		class := setup(t, nil)
		got, err := chooseBaseline(class, run(epyc), DefaultWorkTolerance)
		if err != nil || got.Path != class || got.Identity != nil {
			t.Fatalf("chose %s, err %v, identity %v", got.Path, err, got.Identity)
		}
		if !strings.Contains(got.Notice, "no baseline for this runner's CPU") {
			t.Errorf("notice %q", got.Notice)
		}
	})
	t.Run("this CPU's baseline from another tuning", func(t *testing.T) {
		stale := run(epyc)
		stale.TuningFingerprint = "000000000000"
		class := setup(t, stale)
		got, err := chooseBaseline(class, run(epyc), DefaultWorkTolerance)
		if err != nil || got.Path != class || got.Identity != nil {
			t.Fatalf("chose %s, err %v, identity %v", got.Path, err, got.Identity)
		}
		if !strings.Contains(got.Notice, "measured a different workload") {
			t.Errorf("notice %q", got.Notice)
		}
	})
	t.Run("an unnamed CPU", func(t *testing.T) {
		class := setup(t, run(epyc))
		got, err := chooseBaseline(class, run(""), DefaultWorkTolerance)
		if err != nil || got.Path != class {
			t.Fatalf("chose %s, err %v", got.Path, err)
		}
		if !strings.Contains(got.Notice, "not named") {
			t.Errorf("notice %q", got.Notice)
		}
	})
	t.Run("an unreadable per-CPU baseline fails", func(t *testing.T) {
		class := setup(t, nil)
		if err := os.WriteFile(filepath.Join(filepath.Dir(class), "baseline-kirk0-full-ci-amd-epyc-7763.json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := chooseBaseline(class, run(epyc), DefaultWorkTolerance); err == nil {
			t.Fatal("a malformed per-CPU baseline was passed over in silence")
		}
	})
	t.Run("the class baseline's refusal stands", func(t *testing.T) {
		class := setup(t, nil)
		current := run(epyc)
		current.TuningFingerprint = "ffffffffffff"
		got, err := chooseBaseline(class, current, DefaultWorkTolerance)
		if err != nil || got.Identity == nil {
			t.Fatalf("err %v, identity %v; want the class baseline refused", err, got.Identity)
		}
	})
}

// TestHandleBenchmarkOutputWritesAPerCPUCopy: a capture with -per-cpu-copy
// writes the class baseline and, beside it, the same document under this
// host's CPU name.
func TestHandleBenchmarkOutputWritesAPerCPUCopy(t *testing.T) {
	model := cpuModel()
	if model == "" {
		t.Skip("this host's CPU is not named")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "baseline-kirk0-full-mac.json")
	cfg := Config{PCAPFile: "kirk0.pcapng", OutputDir: dir, BenchmarkOutput: out, PerCPUCopy: true, Quiet: true}
	if code := handleBenchmarkOutput(cfg, &result{}, &PerformanceMetrics{}, nil); code != 0 {
		t.Fatalf("handleBenchmarkOutput = %d, want 0", code)
	}
	class, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(perCPUBaselinePath(out, model))
	if err != nil {
		t.Fatalf("no per-CPU copy: %v", err)
	}
	if string(class) != string(copied) {
		t.Error("the per-CPU copy differs from the class baseline")
	}
}
