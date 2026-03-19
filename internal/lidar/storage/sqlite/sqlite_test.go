package sqlite_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// TestAliasesCompile verifies that store type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// ReplayCase alias.
	var s sqlite.ReplayCase
	s.SensorID = "test-sensor"
	if s.SensorID != "test-sensor" {
		t.Fatalf("ReplayCase alias broken")
	}

	// AnalysisRun alias.
	var run sqlite.AnalysisRun
	run.RunID = "test-run"
	if run.RunID != "test-run" {
		t.Fatalf("AnalysisRun alias broken")
	}

	// Evaluation alias.
	var eval sqlite.Evaluation
	eval.ReplayCaseID = "test-scene"
	if eval.ReplayCaseID != "test-scene" {
		t.Fatalf("Evaluation alias broken")
	}
}
