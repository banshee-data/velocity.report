package sqlite_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// TestAliasesCompile verifies that store type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// Scene alias.
	var s sqlite.Scene
	s.SensorID = "test-sensor"
	if s.SensorID != "test-sensor" {
		t.Fatalf("Scene alias broken")
	}

	// AnalysisRun alias.
	var run sqlite.AnalysisRun
	run.RunID = "test-run"
	if run.RunID != "test-run" {
		t.Fatalf("AnalysisRun alias broken")
	}

	// Evaluation alias.
	var eval sqlite.Evaluation
	eval.SceneID = "test-scene"
	if eval.SceneID != "test-scene" {
		t.Fatalf("Evaluation alias broken")
	}
}
