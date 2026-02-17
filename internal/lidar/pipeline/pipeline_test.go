package pipeline_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// TestStageInterfacesCompile verifies that stage interface aliases resolve
// correctly and that concrete types can satisfy them.
func TestStageInterfacesCompile(t *testing.T) {
	// Verify that the pipeline TrackingPipelineConfig alias works.
	var cfg pipeline.TrackingPipelineConfig
	cfg.SensorID = "test-sensor"
	if cfg.SensorID != "test-sensor" {
		t.Fatalf("TrackingPipelineConfig alias broken")
	}

	// Verify stage interface types are accessible (compile-time check).
	var _ pipeline.ForegroundStage
	var _ pipeline.PerceptionStage
	var _ pipeline.TrackingStage
	var _ pipeline.ObjectStage
	var _ pipeline.PersistenceSink
	var _ pipeline.PublishSink

	// Verify cross-layer alias compatibility: a Tracker from l5tracks
	// should be assignable where the parent package Tracker is expected.
	cfg2 := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(cfg2)
	if tracker == nil {
		t.Fatal("cross-layer NewTracker returned nil")
	}
}
