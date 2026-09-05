//go:build pcap

package lidarbench

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// fixturePCAP is the reference multi-frame capture (UDP 2369). The 20Hz
// capture in the same tree is degenerate — one frame — and cannot drive the
// tracking pipeline.
const (
	fixturePCAP = "../perf/pcap/kirk0.pcapng"
	fixturePort = 2369
)

// benchConfig returns a Config pointed at the fixture, writing into a temp dir.
//
// The tuning config is loaded rather than left nil, because every real caller
// has one and the workload identity depends on it: without a config there is no
// fingerprint, and a benchmark that cannot state which tuning it measured is
// refused for comparison — correctly, but it makes the fixture unable to
// exercise the comparison path at all.
//
// MaxFramesOverBudgetPct is deliberately left nil. The frame budget is a
// statement about production throughput, and these tests run under -race with
// coverage instrumentation, where the pipeline is several times slower than the
// binary the perf gate measures.
func benchConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		PCAPFile:        filepath.Clean(fixturePCAP),
		OutputDir:       t.TempDir(),
		SensorID:        "test-sensor",
		UDPPort:         fixturePort,
		DurationSeconds: 10,
		Tuning:          deterministicTuning(t),
		Quiet:           true,
	}
}

// deterministicTuning removes the wall-clock term from settling so a replay
// does the same work however fast the machine runs it.
//
// Settling ends on a frame minimum, a duration ceiling, or measured
// convergence, and the duration is measured against the clock. On a bounded
// ten-second replay under -race that makes the workload a function of machine
// speed: two runs of identical code produced 29,333 and 42,247 foreground
// points, a 44% spread that no sane identity tolerance would accept. Zeroing
// the ceiling leaves the frame minimum in charge, which is a property of the
// capture rather than of the runner.
//
// The live pipeline keeps the ceiling; this is a fixture, not a default. The
// underlying time-domain boundary is the clock abstraction plan's business.
func deterministicTuning(t *testing.T) *config.TuningConfig {
	t.Helper()

	cfg := config.MustLoadDefaultConfig()
	cfg.L3.EmaBaselineV1.WarmupDurationNanos = 0
	return cfg
}

func TestNormaliseReplayDuration(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   float64
		want float64
	}{
		{name: "zero value means full capture", in: 0, want: -1},
		{name: "explicit full capture", in: -1, want: -1},
		{name: "bounded replay", in: 1.5, want: 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normaliseReplayDuration(tc.in); got != tc.want {
				t.Errorf("normaliseReplayDuration(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestRunProducesBenchmarkResult drives the whole benchmark: PCAP decode,
// frame assembly, the L3–L6 pipeline, metric assembly and JSON output. It is
// the only path that reaches runBenchmark, the analysis frame builder, and the
// summary printers.
func TestRunProducesBenchmarkResult(t *testing.T) {
	cfg := benchConfig(t)
	cfg.BenchmarkOutput = filepath.Join(cfg.OutputDir, "bench.json")

	if code := Run(cfg); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}

	raw, err := os.ReadFile(cfg.BenchmarkOutput)
	if err != nil {
		t.Fatalf("reading benchmark output: %v", err)
	}
	var got BenchmarkResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding benchmark output: %v", err)
	}

	if got.PCAPFile == "" {
		t.Error("PCAPFile is empty, want the replayed capture")
	}
	// One frame-time sample is recorded per processed frame.
	if got.Metrics.FrameTimeStats.Samples <= 0 {
		t.Errorf("FrameTimeStats.Samples = %d, want > 0", got.Metrics.FrameTimeStats.Samples)
	}
	// Throughput is derived from counts over elapsed time, so a zero here
	// means the timing instrumentation did not run.
	if got.Metrics.FramesPerSecond <= 0 {
		t.Errorf("FramesPerSecond = %v, want > 0", got.Metrics.FramesPerSecond)
	}
	if got.Metrics.PacketsPerSecond <= 0 {
		t.Errorf("PacketsPerSecond = %v, want > 0", got.Metrics.PacketsPerSecond)
	}
	if got.Metrics.PointsPerSecond <= 0 {
		t.Errorf("PointsPerSecond = %v, want > 0", got.Metrics.PointsPerSecond)
	}
	if got.Metrics.WallClockMs <= 0 {
		t.Errorf("WallClockMs = %d, want > 0", got.Metrics.WallClockMs)
	}
	// System info is recorded so a baseline can be attributed to a host.
	if got.SystemInfo.GOOS == "" || got.SystemInfo.NumCPU == 0 {
		t.Errorf("SystemInfo = %+v, want it populated", got.SystemInfo)
	}
}

// runOnceForBaseline replays the fixture and returns the document it produced,
// so a test baseline can carry the same workload identity as the run it will be
// compared against. Hand-built identity fields would only prove that the
// refusal path works.
func runOnceForBaseline(t *testing.T, cfg Config) BenchmarkResult {
	t.Helper()

	out := filepath.Join(t.TempDir(), "seed.json")
	seedCfg := cfg
	seedCfg.BenchmarkOutput = out
	seedCfg.CompareBaseline = ""
	if code := Run(seedCfg); code != 0 {
		t.Fatalf("seed run returned %d, want 0", code)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading seed run: %v", err)
	}
	var seed BenchmarkResult
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatalf("decoding seed run: %v", err)
	}
	return seed
}

// workDriftAllowance loosens the work-counter tolerance for tests that replay
// the fixture twice.
//
// The production tolerance is 10%, which an uninstrumented full-capture run
// clears with three orders of magnitude to spare. These tests run a bounded
// ten-second replay under -race, where L3's remaining wall-clock terms mean a
// slower run settles at a different point in the capture: two runs of identical
// code produced foreground counts 20-44% apart. That is a property of the
// harness, not of the pipeline, and the identity mechanism itself is pinned
// deterministically in baseline_identity_test.go.
const workDriftAllowance = 1.0

// readResult decodes a benchmark document written by Run.
func readResult(t *testing.T, path string) BenchmarkResult {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading benchmark output: %v", err)
	}
	var got BenchmarkResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding benchmark output: %v", err)
	}
	return got
}

// writeBaseline marshals a baseline document to a temp path.
func writeBaseline(t *testing.T, dir string, seed BenchmarkResult) string {
	t.Helper()

	path := filepath.Join(dir, "baseline.json")
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		t.Fatalf("marshalling baseline: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing baseline: %v", err)
	}
	return path
}

// TestRunComparesAgainstBaseline covers the comparison path and its exit code:
// a baseline whose metrics are far better than any real run must be reported as
// a regression, which Run signals with exit code 1.
//
// The baseline is seeded from a real run so its workload identity matches, and
// only the timings are then made unreachable. Building the identity by hand
// meant the comparison was refused rather than performed, and the test passed
// on the wrong exit code — a regression could have stopped being detected
// without this failing.
func TestRunComparesAgainstBaseline(t *testing.T) {
	cfg := benchConfig(t)

	seed := runOnceForBaseline(t, cfg)
	seed.Metrics.WallClockMs = 1
	seed.Metrics.FrameTimeStats.AvgMs = 0.001
	seed.Metrics.FrameTimeStats.P95Ms = 0.001
	seed.Metrics.HeapAllocBytes = 1
	seed.Metrics.TotalAllocBytes = 1
	seed.Metrics.ClusterTimeMs = 1

	cfg.CompareBaseline = writeBaseline(t, t.TempDir(), seed)
	cfg.RegressionThreshold = 1.0 // 1% — any slowdown trips it
	cfg.WorkTolerance = workDriftAllowance
	cfg.BenchmarkOutput = filepath.Join(cfg.OutputDir, "bench.json")

	if code := Run(cfg); code != 1 {
		t.Fatalf("Run() = %d, want 1 when the baseline comparison finds a regression", code)
	}

	// A refusal also exits 1. Read the recorded comparison to prove the run
	// was compared and found slower, rather than declined as incomparable.
	got := readResult(t, cfg.BenchmarkOutput)
	if got.Comparison == nil {
		t.Fatal("no comparison recorded: the baseline was refused, not compared")
	}
	if len(got.Comparison.Regressions) == 0 {
		t.Errorf("comparison recorded no regressions: %+v", got.Comparison)
	}
}

// TestRunAcceptsAMatchingBaseline is the other half: the same workload against
// its own numbers must not be reported as a regression. Without it, a change
// that made every comparison fail would still satisfy the test above.
func TestRunAcceptsAMatchingBaseline(t *testing.T) {
	cfg := benchConfig(t)

	seed := runOnceForBaseline(t, cfg)
	cfg.CompareBaseline = writeBaseline(t, t.TempDir(), seed)
	cfg.RegressionThreshold = 10.0 // 1000%: nothing here should trip it
	cfg.WorkTolerance = workDriftAllowance
	cfg.BenchmarkOutput = filepath.Join(cfg.OutputDir, "bench.json")

	if code := Run(cfg); code != 0 {
		t.Fatalf("Run() = %d, want 0 comparing a workload against its own numbers", code)
	}

	got := readResult(t, cfg.BenchmarkOutput)
	if got.Comparison == nil {
		t.Fatal("no comparison recorded: the baseline was refused, not compared")
	}
	if len(got.Comparison.Regressions) != 0 {
		t.Errorf("unexpected regressions against its own numbers: %+v", got.Comparison.Regressions)
	}
}

// TestRunRefusesAStaleBaseline covers the refusal path with the shape that
// caused it to be written: a baseline recording no foreground and no clusters,
// which is what the committed CI baseline held for three months.
func TestRunRefusesAStaleBaseline(t *testing.T) {
	cfg := benchConfig(t)

	stale := BenchmarkResult{
		Version:  "1.0",
		PCAPFile: "kirk0.pcapng",
		Metrics: PerformanceMetrics{
			WallClockMs:    1000,
			FrameTimeStats: FrameTimeStats{AvgMs: 5, P95Ms: 5},
		},
	}
	cfg.CompareBaseline = writeBaseline(t, t.TempDir(), stale)
	cfg.RegressionThreshold = 1.0

	if code := Run(cfg); code != 1 {
		t.Errorf("Run() = %d, want 1 when the baseline measured a different workload", code)
	}
}

// TestRunFailsOnAnUnreadableBaseline pins the behaviour for a comparison that
// was asked for and could not happen. It used to be logged as a warning, and
// -quiet sends log output to io.Discard, so the perf gate could skip the
// comparison entirely and still pass.
func TestRunFailsOnAnUnreadableBaseline(t *testing.T) {
	tests := []struct {
		name  string
		write func(t *testing.T, dir string) string
	}{
		{
			name: "missing file",
			write: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "absent.json")
			},
		},
		{
			name: "malformed json",
			write: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "broken.json")
				if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
					t.Fatalf("writing baseline: %v", err)
				}
				return path
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := benchConfig(t)
			cfg.CompareBaseline = tc.write(t, t.TempDir())

			if code := Run(cfg); code != 1 {
				t.Errorf("Run() = %d, want 1 when the baseline cannot be read", code)
			}
		})
	}
}

func TestRunRejectsMissingPCAPPath(t *testing.T) {
	cfg := benchConfig(t)
	cfg.PCAPFile = ""

	if code := Run(cfg); code != 1 {
		t.Errorf("Run() with no PCAP file = %d, want 1", code)
	}
}

func TestRunRejectsNonexistentPCAP(t *testing.T) {
	cfg := benchConfig(t)
	cfg.PCAPFile = filepath.Join(t.TempDir(), "absent.pcapng")

	if code := Run(cfg); code != 1 {
		t.Errorf("Run() with a missing PCAP = %d, want 1", code)
	}
}

func TestRunRejectsUncreatableOutputDir(t *testing.T) {
	// A file where the output directory should be makes MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}

	cfg := benchConfig(t)
	cfg.OutputDir = filepath.Join(blocker, "nested")

	if code := Run(cfg); code != 1 {
		t.Errorf("Run() with an uncreatable output dir = %d, want 1", code)
	}
}

func TestRunReportsCorruptPCAP(t *testing.T) {
	pcap := filepath.Join(t.TempDir(), "corrupt.pcap")
	if err := os.WriteFile(pcap, []byte("not a pcap"), 0o644); err != nil {
		t.Fatalf("writing corrupt PCAP: %v", err)
	}
	cfg := benchConfig(t)
	cfg.PCAPFile = pcap
	if code := Run(cfg); code != 1 {
		t.Errorf("Run() with corrupt PCAP = %d, want 1", code)
	}
}

func TestRunBenchmarkReportsParserConfigError(t *testing.T) {
	original := loadBenchmarkPandarConfig
	t.Cleanup(func() { loadBenchmarkPandarConfig = original })
	loadBenchmarkPandarConfig = func() (*parse.Pandar40PConfig, error) {
		return nil, errors.New("parser config failed")
	}
	if _, _, err := runBenchmark(benchConfig(t)); err == nil {
		t.Fatal("expected parser config error")
	}
}

func TestHandleBenchmarkOutputReportsUnwritableDestination(t *testing.T) {
	// A path inside a nonexistent directory fails the final WriteFile, which
	// must surface as a failing exit code rather than a silent success.
	cfg := Config{
		PCAPFile:        "fixture.pcapng",
		OutputDir:       t.TempDir(),
		BenchmarkOutput: filepath.Join(t.TempDir(), "absent", "bench.json"),
		Quiet:           true,
	}

	if code := handleBenchmarkOutput(cfg, &result{}, &PerformanceMetrics{}, nil); code != 1 {
		t.Errorf("handleBenchmarkOutput = %d, want 1 for an unwritable destination", code)
	}
}

func TestHandleBenchmarkOutputDerivesFilenameFromPCAP(t *testing.T) {
	// With no explicit -benchmark-output the name is derived from the capture,
	// so repeated runs over different PCAPs do not overwrite each other.
	dir := t.TempDir()
	cfg := Config{PCAPFile: "/captures/kirk0.pcapng", OutputDir: dir, Quiet: true}

	if code := handleBenchmarkOutput(cfg, &result{}, &PerformanceMetrics{}, nil); code != 0 {
		t.Fatalf("handleBenchmarkOutput = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "kirk0_benchmark.json")); err != nil {
		t.Errorf("expected kirk0_benchmark.json in the output dir: %v", err)
	}
}

func TestHandleBenchmarkOutputPrintsSummaryWhenNotQuiet(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{PCAPFile: "/captures/kirk0.pcapng", OutputDir: dir}

	if code := handleBenchmarkOutput(cfg, &result{}, &PerformanceMetrics{}, nil); code != 0 {
		t.Errorf("handleBenchmarkOutput = %d, want 0", code)
	}
}

func TestCompareWithBaselineToleratesUnusableBaseline(t *testing.T) {
	// A missing or malformed baseline is a warning, not a failure: the
	// benchmark still runs and reports, it just has nothing to diff against.
	t.Run("missing file", func(t *testing.T) {
		comparison, regressed := compareWithBaseline(
			filepath.Join(t.TempDir(), "absent.json"), &PerformanceMetrics{}, 0.05)
		if comparison != nil {
			t.Errorf("comparison = %+v, want nil", comparison)
		}
		if regressed {
			t.Error("regressed = true, want false when there is no baseline")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "baseline.json")
		if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("writing baseline: %v", err)
		}
		comparison, regressed := compareWithBaseline(bad, &PerformanceMetrics{}, 0.05)
		if comparison != nil {
			t.Errorf("comparison = %+v, want nil", comparison)
		}
		if regressed {
			t.Error("regressed = true, want false for an unparseable baseline")
		}
	})
}

func TestAnalysisStatsAccumulate(t *testing.T) {
	var s analysisStats

	s.AddPacket(10)
	s.AddPacket(20)
	s.AddPoints(100)
	s.AddPoints(50)
	s.AddDropped()
	s.LogStats(true)

	packets, points, _ := s.getStats()
	if packets != 2 {
		t.Errorf("packets = %d, want 2", packets)
	}
	if points != 150 {
		t.Errorf("points = %d, want 150", points)
	}
}

func TestAnalysisFrameBuilderDirectBranches(t *testing.T) {
	t.Run("empty packet", func(t *testing.T) {
		fb := newAnalysisFrameBuilder(Config{SensorID: "bench-empty"}, &result{})
		fb.AddPointsPolar(nil)
		if len(fb.points) != 0 {
			t.Fatalf("empty packet added %d points", len(fb.points))
		}
	})

	t.Run("unavailable background mask", func(t *testing.T) {
		fb := &analysisFrameBuilder{
			bgManager: &l3grid.BackgroundManager{},
			res:       &result{},
			points:    []l2frames.PointPolar{{Channel: 1, Distance: 5}},
		}
		fb.processCurrentFrame()
		if len(fb.frameTimes) != 1 {
			t.Fatalf("frame timing samples = %d, want 1", len(fb.frameTimes))
		}
	})

	t.Run("foreground clustering and tracking", func(t *testing.T) {
		res := &result{}
		fb := newAnalysisFrameBuilder(Config{SensorID: "bench-direct-foreground"}, res)
		for i := range fb.bgManager.Grid.Cells {
			fb.bgManager.Grid.Cells[i].AverageRangeMeters = 10
			fb.bgManager.Grid.Cells[i].TimesSeenCount = 100
		}
		fb.bgManager.Grid.SettlingComplete = true
		fb.bgManager.HasSettled = true
		params := fb.bgManager.GetParams()
		params.WarmupMinFrames = 0
		params.WarmupDurationNanos = 0
		params.NeighbourConfirmationCount = 0
		params.ForegroundMinClusterPoints = 3
		params.ForegroundDBSCANEps = 1
		if err := fb.bgManager.SetParams(params); err != nil {
			t.Fatalf("SetParams() error: %v", err)
		}

		base := time.Unix(20_000, 0)
		for frameIndex := 0; frameIndex < 7; frameIndex++ {
			fb.points = fb.points[:0]
			for i := 0; i < 20; i++ {
				fb.points = append(fb.points, l2frames.PointPolar{
					Channel:   1,
					Azimuth:   180 + float64(i)*0.01,
					Elevation: float64(i%3) * 0.01,
					Distance:  5 + float64(i)*0.005,
					Timestamp: base.Add(time.Duration(frameIndex) * 50 * time.Millisecond).UnixNano(),
				})
			}
			fb.frameStartTime = base.Add(time.Duration(frameIndex) * 50 * time.Millisecond)
			fb.processCurrentFrame()
		}
		if res.TotalClusters == 0 {
			t.Fatal("direct foreground frames produced no clusters")
		}
	})

	t.Run("foreground without clusters", func(t *testing.T) {
		res := &result{}
		fb := newAnalysisFrameBuilder(Config{SensorID: "bench-direct-no-clusters"}, res)
		for i := range fb.bgManager.Grid.Cells {
			fb.bgManager.Grid.Cells[i].AverageRangeMeters = 10
			fb.bgManager.Grid.Cells[i].TimesSeenCount = 100
		}
		fb.bgManager.Grid.SettlingComplete = true
		fb.bgManager.HasSettled = true
		params := fb.bgManager.GetParams()
		params.WarmupMinFrames = 0
		params.WarmupDurationNanos = 0
		params.NeighbourConfirmationCount = 0
		params.ForegroundMinClusterPoints = 999
		params.ForegroundDBSCANEps = 1
		if err := fb.bgManager.SetParams(params); err != nil {
			t.Fatalf("SetParams() error: %v", err)
		}
		base := time.Unix(30_000, 0)
		for i := 0; i < 20; i++ {
			fb.points = append(fb.points, l2frames.PointPolar{
				Channel:   1,
				Azimuth:   180 + float64(i)*0.01,
				Distance:  5 + float64(i)*0.005,
				Timestamp: base.UnixNano(),
			})
		}
		fb.frameStartTime = base
		fb.processCurrentFrame()
		if res.TotalClusters != 0 || len(fb.frameTimes) != 1 {
			t.Fatalf("clusters=%d frameTimes=%d, want 0 and 1", res.TotalClusters, len(fb.frameTimes))
		}
	})
}

func TestGetSystemInfoReadsAndAbbreviatesRevision(t *testing.T) {
	original := readBenchmarkBuildInfo
	t.Cleanup(func() { readBenchmarkBuildInfo = original })
	readBenchmarkBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{
			Key:   "vcs.revision",
			Value: "1234567890abcdef",
		}}}, true
	}
	if got := getSystemInfo().CommitHash; got != "1234567890ab" {
		t.Fatalf("CommitHash = %q, want abbreviated revision", got)
	}
}

func TestCompareWithBaselineReportsImprovements(t *testing.T) {
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	baseline := BenchmarkResult{Metrics: PerformanceMetrics{
		WallClockMs:     100,
		FramesPerSecond: 100,
		HeapAllocBytes:  100,
		TotalAllocBytes: 100,
		ClusterTimeMs:   100,
		TrackingTimeMs:  100,
		FrameTimeStats:  FrameTimeStats{AvgMs: 100, P95Ms: 100},
	}}
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if err := os.WriteFile(baselinePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	current := &PerformanceMetrics{
		WallClockMs:     50,
		FramesPerSecond: 200,
		HeapAllocBytes:  50,
		TotalAllocBytes: 50,
		ClusterTimeMs:   50,
		TrackingTimeMs:  50,
		FrameTimeStats:  FrameTimeStats{AvgMs: 50, P95Ms: 50},
	}
	comparison, regressed := compareWithBaseline(baselinePath, current, 0.1)
	if regressed {
		t.Fatal("improved metrics were reported as a regression")
	}
	// Seven gated metrics, all halved. frames_per_second is not among them:
	// it is derived from wall_clock_ms and would double-count.
	if comparison == nil || len(comparison.Improvements) != 7 {
		t.Fatalf("improvements = %+v, want all seven gated metrics", comparison)
	}
}

func TestMsSinceIsNonNegative(t *testing.T) {
	// msSince converts an elapsed duration to fractional milliseconds; a
	// freshly taken start time must never produce a negative reading.
	if got := msSince(time.Now()); got < 0 {
		t.Errorf("msSince(now) = %v, want >= 0", got)
	}
}

func TestGetSystemInfoPopulatesHostDetails(t *testing.T) {
	info := getSystemInfo()

	if info.GOOS == "" {
		t.Error("GOOS is empty")
	}
	if info.GOARCH == "" {
		t.Error("GOARCH is empty")
	}
	if info.NumCPU <= 0 {
		t.Errorf("NumCPU = %d, want > 0", info.NumCPU)
	}
	if info.GoVersion == "" {
		t.Error("GoVersion is empty")
	}
}

func TestTuningOrEmbeddedFallsBackToDefaults(t *testing.T) {
	// A nil tuning config must resolve to the embedded defaults so the
	// benchmarked pipeline matches live observation.
	got := tuningOrEmbedded(nil)
	if got == nil {
		t.Fatal("tuningOrEmbedded(nil) = nil, want the embedded defaults")
	}
}

func TestTuningOrEmbeddedPassesThroughSuppliedConfig(t *testing.T) {
	supplied := tuningOrEmbedded(nil) // a valid config to hand straight back
	if supplied == nil {
		t.Fatal("could not obtain a tuning config to test with")
	}

	if got := tuningOrEmbedded(supplied); got != supplied {
		t.Error("tuningOrEmbedded(cfg) did not return the supplied config unchanged")
	}
}

func TestWrapProgressOnlyWrapsWhenIntervalSet(t *testing.T) {
	var inner analysisStats

	t.Run("no interval returns the inner stats unchanged", func(t *testing.T) {
		cfg := Config{PCAPFile: filepath.Clean(fixturePCAP)}
		if got := wrapProgress(cfg, &inner, "test"); got != &inner {
			t.Error("wrapProgress with ProgressSecs=0 wrapped the stats, want passthrough")
		}
	})

	t.Run("interval wraps with progress reporting", func(t *testing.T) {
		cfg := Config{PCAPFile: filepath.Clean(fixturePCAP), ProgressSecs: 0.5}
		got := wrapProgress(cfg, &inner, "test")
		if got == nil {
			t.Fatal("wrapProgress returned nil")
		}
		if got == network.PacketStatsInterface(&inner) {
			t.Error("wrapProgress with ProgressSecs>0 returned the inner stats, want a wrapper")
		}
	})

	t.Run("missing file still wraps", func(t *testing.T) {
		// The size lookup is best-effort: a stat failure leaves size at 0
		// rather than disabling progress entirely.
		cfg := Config{PCAPFile: filepath.Join(t.TempDir(), "absent.pcapng"), ProgressSecs: 0.5}
		if got := wrapProgress(cfg, &inner, "test"); got == nil {
			t.Error("wrapProgress returned nil for an unstattable file")
		}
	})
}

func TestFrameBuilderNoOpMethods(t *testing.T) {
	// SetMotorSpeed and LogStats satisfy the network interfaces but carry no
	// benchmark meaning; they must remain safe no-ops.
	fb := newAnalysisFrameBuilder(Config{SensorID: "test"}, &result{})
	fb.SetMotorSpeed(600)

	var stats analysisStats
	stats.LogStats(false)
	stats.LogStats(true)
}

func TestPrintBenchmarkSummary(t *testing.T) {
	// Printed to stdout for the operator; exercised here to prove it formats
	// a fully-populated metrics struct without panicking on zero values.
	printBenchmarkSummary(&BenchmarkResult{
		Profile:           "full",
		TuningFingerprint: "abc123def456",
		Metrics: PerformanceMetrics{
			WallClockMs:      1234,
			FramesPerSecond:  19.8,
			PacketsPerSecond: 1500,
			FrameTimeStats: FrameTimeStats{
				AvgMs: 1.5, P50Ms: 1.4, P95Ms: 2.2, P99Ms: 3.1, Samples: 200,
			},
			PipelineTimeMs: 900, ClusterTimeMs: 300,
			TrackingTimeMs: 200, ClassifyTimeMs: 100,
			HeapAllocBytes: 5 << 20, TotalAllocBytes: 50 << 20,
			NumGC: 7, GCPauseNs: 1_500_000,
			Work:        WorkCounters{Frames: 200, ForegroundPoints: 4000, Clusters: 30, ConfirmedTracks: 4},
			FrameBudget: FrameBudget{BudgetMs: 98, FramesOver: 2, FramesOverPct: 1.0, WorstMs: 120},
		},
		RepeatSpread: &RepeatSpread{Runs: 3, WallClockMs: []int64{1200, 1234, 1300}, MedianMs: 1234, SpreadPct: 8.3},
	})
	printBenchmarkSummary(&BenchmarkResult{})
}

func TestPrintComparisonSummary(t *testing.T) {
	t.Run("regressions and improvements", func(t *testing.T) {
		printComparisonSummary(&BenchmarkComparison{
			BaselineFile: "baseline.json",
			Regressions: []MetricDifference{
				{Metric: "frames_per_second", BaselineValue: 20, CurrentValue: 15, ChangePercent: -25},
			},
			Improvements: []MetricDifference{
				{Metric: "wall_clock_ms", BaselineValue: 2000, CurrentValue: 1000, ChangePercent: -50},
			},
		}, 0.05)
	})

	t.Run("no significant changes", func(t *testing.T) {
		printComparisonSummary(&BenchmarkComparison{BaselineFile: "baseline.json"}, 0.05)
	})
}
