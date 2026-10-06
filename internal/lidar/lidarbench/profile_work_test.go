//go:build pcap
// +build pcap

package lidarbench

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

// TestProfileWorkCounters drives the real pipeline over the fixture capture at
// each profile and checks the work counters say which layers ran. These are the
// numbers the baseline comparator uses as workload identity, so they have to
// mean what they claim: a profile whose counters look like another profile's
// would let two incomparable runs be compared.
func TestProfileWorkCounters(t *testing.T) {
	tests := []struct {
		profile      config.Profile
		wantClusters bool
	}{
		{config.ProfileL3Only, false},
		{config.ProfileDetect, true},
		{config.ProfileFull, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			cfg := benchConfig(t)
			tuning := config.MustLoadDefaultConfig()
			if err := tuning.ApplyProfile(tc.profile); err != nil {
				t.Fatalf("ApplyProfile(%s): %v", tc.profile, err)
			}
			cfg.Tuning = tuning

			_, metrics, err := runBenchmark(cfg)
			if err != nil {
				t.Fatalf("runBenchmark: %v", err)
			}

			work := metrics.Work
			if work.Frames == 0 {
				t.Fatal("no frames processed; the fixture or port is wrong")
			}
			if work.ForegroundPoints == 0 {
				t.Error("no foreground points: L3 must run in every profile")
			}

			if got := work.Clusters > 0; got != tc.wantClusters {
				t.Errorf("clusters produced = %v (%d), want %v",
					got, work.Clusters, tc.wantClusters)
			}
			if !tc.wantClusters && metrics.ClusterTimeMs != 0 {
				t.Errorf("cluster_time_ms = %d under %s, want 0",
					metrics.ClusterTimeMs, tc.profile)
			}
			if tc.profile == config.ProfileDetect && metrics.TrackingTimeMs != 0 {
				t.Errorf("tracking_time_ms = %d under detect, want 0", metrics.TrackingTimeMs)
			}
		})
	}
}

// TestHeapAllocIsStableAcrossRuns is the assertion that would have caught the
// metric which produced the "+6763% heap regression" headline. Read without a
// forced collection, heap_alloc_bytes was whatever the heap happened to be
// mid-cycle and spanned 18.8-40.0 MB across identical runs.
func TestHeapAllocIsStableAcrossRuns(t *testing.T) {
	cfg := benchConfig(t)

	_, first, err := runBenchmark(cfg)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	_, second, err := runBenchmark(cfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if first.HeapAllocBytes == 0 || second.HeapAllocBytes == 0 {
		t.Fatal("heap_alloc_bytes was zero; the measurement is not being taken")
	}

	// Live heap after a collection is not bit-identical run to run — a few
	// runtime-internal allocations differ — but it must be the same number to
	// within a few per cent, not a multiple.
	lo, hi := first.HeapAllocBytes, second.HeapAllocBytes
	if lo > hi {
		lo, hi = hi, lo
	}
	drift := float64(hi-lo) / float64(lo)
	if drift > 0.05 {
		t.Errorf("heap_alloc_bytes drifted %.1f%% between identical runs (%d vs %d); "+
			"the forced collection before ReadMemStats is missing or ineffective",
			drift*100, first.HeapAllocBytes, second.HeapAllocBytes)
	}
}

// TestBenchProfileResolution covers the override precedence: an explicit
// -profile beats the config, and an absent one falls back to it.
// TestBenchProfileFollowsTheConfig checks the benchmark reads its depth from
// the same engine selectors the pipeline gates on. A -profile flag reaches the
// benchmark by having already disabled layers in this config, so there is no
// second switch that could disagree with it.
func TestBenchProfileFollowsTheConfig(t *testing.T) {
	tests := []struct {
		name  string
		apply config.Profile
		want  config.Profile
	}{
		{"defaults are full depth", "", config.ProfileFull},
		{"reduced to detect", config.ProfileDetect, config.ProfileDetect},
		{"reduced to l3-only", config.ProfileL3Only, config.ProfileL3Only},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tuning := config.MustLoadDefaultConfig()
			if tc.apply != "" {
				if err := tuning.ApplyProfile(tc.apply); err != nil {
					t.Fatalf("ApplyProfile(%s): %v", tc.apply, err)
				}
			}
			if got := benchProfile(Config{Tuning: tuning}); got != tc.want {
				t.Errorf("benchProfile() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBenchProfileWithoutConfigIsFull covers the arm where no tuning config
// was loaded at all, which only tests and hand-built Configs hit.
func TestBenchProfileWithoutConfigIsFull(t *testing.T) {
	if got := benchProfile(Config{}); got != config.ProfileFull {
		t.Errorf("benchProfile() = %q with no config, want %q", got, config.ProfileFull)
	}
}

// TestTuningFingerprintIsRecorded checks the identity field is populated from
// the loaded config, and empty when there is no config to fingerprint.
func TestTuningFingerprintIsRecorded(t *testing.T) {
	if got := tuningFingerprint(Config{}); got != "" {
		t.Errorf("tuningFingerprint with no config = %q, want empty", got)
	}

	cfg := Config{Tuning: config.MustLoadDefaultConfig()}
	got := tuningFingerprint(cfg)
	if got == "" {
		t.Fatal("a loaded config produced no fingerprint")
	}
	if got != cfg.Tuning.Fingerprint() {
		t.Errorf("tuningFingerprint = %q, want the config's own %q", got, cfg.Tuning.Fingerprint())
	}
}
