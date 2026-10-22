package sqlite_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// TestAliasesCompile verifies that store type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// Clip alias.
	var s sqlite.Clip
	s.SensorID = "test-sensor"
	if s.SensorID != "test-sensor" {
		t.Fatalf("Clip alias broken")
	}

	// Run alias.
	var run sqlite.Run
	run.RunID = "test-run"
	if run.RunID != "test-run" {
		t.Fatalf("Run alias broken")
	}

	// Evaluation alias.
	var eval sqlite.Evaluation
	eval.ClipID = "test-scene"
	if eval.ClipID != "test-scene" {
		t.Fatalf("Evaluation alias broken")
	}
}
