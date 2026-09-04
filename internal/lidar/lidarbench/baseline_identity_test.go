//go:build pcap
// +build pcap

package lidarbench

import (
	"strings"
	"testing"
)

// fullWork is a plausible set of counters for a full-profile run over kirk0.
func fullWork() WorkCounters {
	return WorkCounters{
		Frames:           832,
		ForegroundPoints: 1_613_914,
		BackgroundPoints: 55_762_542,
		Clusters:         13_854,
		ConfirmedTracks:  12,
	}
}

func comparableResult() *BenchmarkResult {
	return &BenchmarkResult{
		PCAPFile:          "kirk0.pcapng",
		Profile:           "full",
		TuningFingerprint: "abc123def456",
		Metrics:           PerformanceMetrics{Work: fullWork()},
	}
}

// TestIdentityAcceptsMatchingRuns establishes the baseline behaviour the other
// cases deviate from: two runs of the same workload compare normally.
func TestIdentityAcceptsMatchingRuns(t *testing.T) {
	if err := checkWorkloadIdentity(comparableResult(), comparableResult()); err != nil {
		t.Errorf("identical workloads were refused: %v", err)
	}
}

// TestIdentityRefusesTheJune2026Baseline is the regression test for the
// failure that motivated all of this. That baseline recorded 832 frames, zero
// foreground points and zero clusters — a pipeline that stopped at L3 — and
// the comparator reported its successor as a 7028% heap regression rather than
// saying the two runs were not comparable.
func TestIdentityRefusesTheJune2026Baseline(t *testing.T) {
	stale := &BenchmarkResult{
		PCAPFile: "kirk0.pcapng",
		// No profile and no fingerprint: the schema predates both.
		Metrics: PerformanceMetrics{Work: WorkCounters{
			Frames:           832,
			ForegroundPoints: 0,
			BackgroundPoints: 57_376_456,
			Clusters:         0,
		}},
	}

	err := checkWorkloadIdentity(stale, comparableResult())
	if err == nil {
		t.Fatal("the stale baseline was accepted as comparable")
	}

	msg := err.Error()
	for _, want := range []string{"profile", "fingerprint", "foreground_points", "clusters"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q should name %q", msg, want)
		}
	}
}

func TestIdentityRefusalReasons(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*BenchmarkResult)
		wantText string
	}{
		{
			name:     "different profile",
			mutate:   func(b *BenchmarkResult) { b.Profile = "l3-only" },
			wantText: "profile",
		},
		{
			name:     "different tuning",
			mutate:   func(b *BenchmarkResult) { b.TuningFingerprint = "999999999999" },
			wantText: "tuning fingerprint",
		},
		{
			name:     "different capture",
			mutate:   func(b *BenchmarkResult) { b.PCAPFile = "other.pcapng" },
			wantText: "capture",
		},
		{
			name:     "missing profile",
			mutate:   func(b *BenchmarkResult) { b.Profile = "" },
			wantText: "predates pipeline profiles",
		},
		{
			name:     "missing fingerprint",
			mutate:   func(b *BenchmarkResult) { b.TuningFingerprint = "" },
			wantText: "no tuning fingerprint",
		},
		{
			name: "clusters collapsed to zero",
			mutate: func(b *BenchmarkResult) {
				b.Metrics.Work.Clusters = 0
			},
			wantText: "clusters",
		},
		{
			name: "foreground moved beyond tolerance",
			mutate: func(b *BenchmarkResult) {
				b.Metrics.Work.ForegroundPoints = 1_000_000
			},
			wantText: "foreground_points",
		},
		{
			name: "frame count changed",
			mutate: func(b *BenchmarkResult) {
				b.Metrics.Work.Frames = 400
			},
			wantText: "frames",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseline := comparableResult()
			tc.mutate(baseline)

			err := checkWorkloadIdentity(baseline, comparableResult())
			if err == nil {
				t.Fatalf("expected a refusal for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("refusal %q should mention %q", err, tc.wantText)
			}
		})
	}
}

// TestIdentityToleratesSmallWorkDrift checks the tolerance does its job. Work
// counts are deterministic for a given code, config and capture, but a genuine
// small detection change should not have to be re-baselined as an emergency.
func TestIdentityToleratesSmallWorkDrift(t *testing.T) {
	baseline := comparableResult()
	current := comparableResult()
	// +5%, inside the 10% tolerance.
	current.Metrics.Work.Clusters = 14_546

	if err := checkWorkloadIdentity(baseline, current); err != nil {
		t.Errorf("a 5%% cluster drift was refused: %v", err)
	}
}

// TestIdentityRefusesJustBeyondTolerance pins the other side of the boundary,
// so the tolerance cannot silently widen.
func TestIdentityRefusesJustBeyondTolerance(t *testing.T) {
	baseline := comparableResult()
	current := comparableResult()
	// +15%, outside the 10% tolerance.
	current.Metrics.Work.Clusters = 15_932

	if err := checkWorkloadIdentity(baseline, current); err == nil {
		t.Error("a 15% cluster drift was accepted as the same workload")
	}
}

// TestIdentitySkipsCaptureCheckWhenUnnamed covers the arm where one side has
// no capture recorded, which older documents and hand-built fixtures hit.
func TestIdentitySkipsCaptureCheckWhenUnnamed(t *testing.T) {
	baseline := comparableResult()
	baseline.PCAPFile = ""

	if err := checkWorkloadIdentity(baseline, comparableResult()); err != nil {
		t.Errorf("an unnamed capture should not by itself refuse the comparison: %v", err)
	}
}

// TestFrameBudgetCheck covers the absolute gate. It is the only check here
// that needs no baseline: it asks whether the pipeline keeps up with the
// sensor, which no comparison against a previous run can answer.
func TestFrameBudgetCheck(t *testing.T) {
	tests := []struct {
		name       string
		budget     FrameBudget
		maxOverPct float64
		wantErr    bool
	}{
		{
			name:       "no frames over budget",
			budget:     FrameBudget{BudgetMs: 98, FramesOver: 0},
			maxOverPct: 1.0,
			wantErr:    false,
		},
		{
			name:       "within the allowance",
			budget:     FrameBudget{BudgetMs: 98, FramesOver: 4, FramesOverPct: 0.48, WorstMs: 110},
			maxOverPct: 1.0,
			wantErr:    false,
		},
		{
			name:       "beyond the allowance",
			budget:     FrameBudget{BudgetMs: 98, FramesOver: 40, FramesOverPct: 4.8, WorstMs: 320},
			maxOverPct: 1.0,
			wantErr:    true,
		},
		{
			name:       "exactly at the allowance is allowed",
			budget:     FrameBudget{BudgetMs: 98, FramesOver: 8, FramesOverPct: 1.0, WorstMs: 99},
			maxOverPct: 1.0,
			wantErr:    false,
		},
		{
			name:       "a disabled budget never fails",
			budget:     FrameBudget{BudgetMs: 0, FramesOver: 100, FramesOverPct: 50},
			maxOverPct: 1.0,
			wantErr:    false,
		},
		{
			name:       "a zero allowance fails on one frame",
			budget:     FrameBudget{BudgetMs: 98, FramesOver: 1, FramesOverPct: 0.12, WorstMs: 105},
			maxOverPct: 0,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkFrameBudget(tc.budget, tc.maxOverPct)
			if tc.wantErr && err == nil {
				t.Fatal("expected the budget check to fail")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected budget failure: %v", err)
			}
		})
	}
}

// TestFrameBudgetErrorIsActionable checks the failure names the numbers an
// operator needs: how many frames, how bad the worst was, and against what.
func TestFrameBudgetErrorIsActionable(t *testing.T) {
	err := checkFrameBudget(FrameBudget{
		BudgetMs: 98, FramesOver: 40, FramesOverPct: 4.8, WorstMs: 320.5,
	}, 1.0)
	if err == nil {
		t.Fatal("expected a failure")
	}
	for _, want := range []string{"40 frames", "4.80%", "98 ms", "320.5 ms"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err, want)
		}
	}
}

func TestComputeFrameBudget(t *testing.T) {
	tests := []struct {
		name        string
		frameTimes  []float64
		budgetMs    float64
		wantOver    int
		wantOverPct float64
		wantWorst   float64
	}{
		{
			name:       "no frames",
			frameTimes: nil,
			budgetMs:   98,
		},
		{
			name:        "all within budget",
			frameTimes:  []float64{5, 10, 40, 97.9},
			budgetMs:    98,
			wantOver:    0,
			wantOverPct: 0,
			wantWorst:   97.9,
		},
		{
			name:        "one over",
			frameTimes:  []float64{5, 10, 98.1, 40},
			budgetMs:    98,
			wantOver:    1,
			wantOverPct: 25,
			wantWorst:   98.1,
		},
		{
			name:        "exactly at budget is not over",
			frameTimes:  []float64{98, 98},
			budgetMs:    98,
			wantOver:    0,
			wantOverPct: 0,
			wantWorst:   98,
		},
		{
			name:        "a zero budget disables counting but still tracks the worst",
			frameTimes:  []float64{5, 500},
			budgetMs:    0,
			wantOver:    0,
			wantOverPct: 0,
			wantWorst:   500,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeFrameBudget(tc.frameTimes, tc.budgetMs)
			if got.FramesOver != tc.wantOver {
				t.Errorf("FramesOver = %d, want %d", got.FramesOver, tc.wantOver)
			}
			if got.FramesOverPct != tc.wantOverPct {
				t.Errorf("FramesOverPct = %v, want %v", got.FramesOverPct, tc.wantOverPct)
			}
			if got.WorstMs != tc.wantWorst {
				t.Errorf("WorstMs = %v, want %v", got.WorstMs, tc.wantWorst)
			}
			if got.BudgetMs != tc.budgetMs {
				t.Errorf("BudgetMs = %v, want %v", got.BudgetMs, tc.budgetMs)
			}
		})
	}
}

// TestZeroBaselineIsNoLongerSkipped is the other half of the June 2026
// failure. cluster_time_ms and tracking_time_ms were both zero in that
// baseline, and the comparator skipped every metric whose baseline was zero —
// so the two numbers that would have said "clustering is not running" were the
// two it ignored.
func TestZeroBaselineIsNoLongerSkipped(t *testing.T) {
	diff, verdict := classifyMetricChange("cluster_time_ms", 0, 5410, true, 0.30)

	if verdict != verdictRegression {
		t.Errorf("verdict = %v, want a regression when a cost appears from nothing", verdict)
	}
	if diff.BaselineValue != 0 || diff.CurrentValue != 5410 {
		t.Errorf("diff = %+v, want the raw values preserved", diff)
	}
}

func TestClassifyMetricChange(t *testing.T) {
	tests := []struct {
		name        string
		baseline    float64
		current     float64
		higherIsBad bool
		want        metricVerdict
	}{
		{"both zero is unchanged", 0, 0, true, verdictUnchanged},
		{"zero to non-zero, higher is bad", 0, 100, true, verdictRegression},
		{"zero to non-zero, higher is good", 0, 100, false, verdictImprovement},
		{"within threshold", 100, 110, true, verdictUnchanged},
		{"beyond threshold, higher is bad", 100, 150, true, verdictRegression},
		{"improvement, higher is bad", 100, 50, true, verdictImprovement},
		{"beyond threshold, higher is good", 100, 50, false, verdictRegression},
		{"improvement, higher is good", 100, 150, false, verdictImprovement},
		{"collapse to zero, higher is bad", 100, 0, true, verdictImprovement},
		{"collapse to zero, higher is good", 100, 0, false, verdictRegression},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, got := classifyMetricChange("m", tc.baseline, tc.current, tc.higherIsBad, 0.30)
			if got != tc.want {
				t.Errorf("verdict = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIdentityErrorMessage checks the wording distinguishes a refusal from a
// regression. They mean different things and lead to different actions: one is
// "the pipeline got slower", the other "the question was never well formed".
func TestIdentityErrorMessage(t *testing.T) {
	err := &identityError{reasons: []string{"profile \"full\" vs \"detect\"", "clusters 0 vs 13854"}}
	msg := err.Error()

	if !strings.HasPrefix(msg, "baseline workload mismatch:") {
		t.Errorf("message %q should open by naming the mismatch", msg)
	}
	if !strings.Contains(msg, "; ") {
		t.Errorf("message %q should list every reason, not just the first", msg)
	}
}

// TestIdentityRefusesDifferentPlatform covers the hardware arm. A committed
// local baseline is otherwise a trap: the next person to run the gate on a
// different architecture reads timing noise as a regression.
func TestIdentityRefusesDifferentPlatform(t *testing.T) {
	baseline := comparableResult()
	baseline.SystemInfo = SystemInfo{GOOS: "darwin", GOARCH: "arm64", NumCPU: 10}

	current := comparableResult()
	current.SystemInfo = SystemInfo{GOOS: "linux", GOARCH: "amd64", NumCPU: 4}

	err := checkWorkloadIdentity(baseline, current)
	if err == nil {
		t.Fatal("a baseline from another platform was accepted")
	}
	if !strings.Contains(err.Error(), "platform") {
		t.Errorf("refusal %q should name the platform mismatch", err)
	}
}

// TestIdentityWarnsButAcceptsDifferentCPUCount checks the softer arm. A
// different core count on the same platform shifts the numbers without making
// the comparison meaningless, so it warns rather than refusing — otherwise
// every runner-size change would demand a re-baseline.
func TestIdentityWarnsButAcceptsDifferentCPUCount(t *testing.T) {
	baseline := comparableResult()
	baseline.SystemInfo = SystemInfo{GOOS: "linux", GOARCH: "amd64", NumCPU: 4, GoVersion: "go1.26.4"}

	current := comparableResult()
	current.SystemInfo = SystemInfo{GOOS: "linux", GOARCH: "amd64", NumCPU: 8, GoVersion: "go1.26.5"}

	if err := checkWorkloadIdentity(baseline, current); err != nil {
		t.Errorf("a CPU count or Go version change should warn, not refuse: %v", err)
	}
}

// TestIdentitySkipsPlatformCheckWhenUnrecorded covers documents that carry no
// system info, which hand-built fixtures and the oldest baselines hit.
func TestIdentitySkipsPlatformCheckWhenUnrecorded(t *testing.T) {
	baseline := comparableResult()
	baseline.SystemInfo = SystemInfo{}

	if err := checkWorkloadIdentity(baseline, comparableResult()); err != nil {
		t.Errorf("missing system info should not by itself refuse: %v", err)
	}
}
