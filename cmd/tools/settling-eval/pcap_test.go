package main

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

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

func TestRunPCAPEvalUsesTuningConfigBeforeReplayFailure(t *testing.T) {
	_, err := runPCAPEval("/nonexistent.pcap", "", "test-sensor", 2369)
	if err == nil {
		t.Fatal("expected replay error for nonexistent PCAP")
	}
}
