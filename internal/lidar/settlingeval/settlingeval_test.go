package settlingeval

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

func TestSettlingThresholdsFromTuning(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	cfg.L3.EmaBaselineV1.SettlingMinCoverage = 0.7
	cfg.L3.EmaBaselineV1.SettlingMaxSpreadDelta = 0.02
	cfg.L3.EmaBaselineV1.SettlingMinRegionStability = 0.8
	cfg.L3.EmaBaselineV1.SettlingMinConfidence = 4

	got := settlingThresholdsFromTuning(cfg)
	if got.MinCoverage != 0.7 || got.MaxSpreadDelta != 0.02 || got.MinRegionStability != 0.8 || got.MinConfidence != 4 {
		t.Fatalf("thresholds = %+v", got)
	}
}

func TestBackgroundConfigFromTuningConfig(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	bg := backgroundConfigFromTuningConfig(cfg)
	if bg == nil {
		t.Fatal("expected background config, got nil")
	}
	if bg.UpdateFraction <= 0 || bg.ForegroundDBSCANEps <= 0 {
		t.Fatalf("unexpected background config: %+v", *bg)
	}
}

func TestRunUsesTuningConfigBeforeReplayFailure(t *testing.T) {
	_, err := Run(Config{PCAPFile: "/nonexistent.pcap", SensorID: "test-sensor", UDPPort: 2369})
	if err == nil {
		t.Fatal("expected replay error for nonexistent PCAP")
	}
}
