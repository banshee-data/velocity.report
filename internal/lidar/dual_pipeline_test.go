package lidar

import (
	"testing"
	"time"
)

func TestNewDualExtractionPipeline(t *testing.T) {
	config := DefaultDualPipelineConfig()
	pipeline := NewDualExtractionPipeline(config)

	if pipeline == nil {
		t.Fatal("NewDualExtractionPipeline returned nil")
	}

	if pipeline.bgTracker == nil {
		t.Error("Background tracker not initialized")
	}

	if pipeline.vcTracker == nil {
		t.Error("Velocity coherent tracker not initialized")
	}
}

func TestDualExtractionPipeline_GetConfig(t *testing.T) {
	config := DefaultDualPipelineConfig()
	config.ActiveAlgorithm = AlgorithmVelocityCoherent
	pipeline := NewDualExtractionPipeline(config)

	gotConfig := pipeline.GetConfig()

	if gotConfig.ActiveAlgorithm != AlgorithmVelocityCoherent {
		t.Errorf("Expected algorithm %s, got %s", AlgorithmVelocityCoherent, gotConfig.ActiveAlgorithm)
	}
}

func TestDualExtractionPipeline_UpdateConfig(t *testing.T) {
	config := DefaultDualPipelineConfig()
	config.ActiveAlgorithm = AlgorithmBackgroundSubtraction
	pipeline := NewDualExtractionPipeline(config)

	// Update to velocity coherent
	newConfig := config
	newConfig.ActiveAlgorithm = AlgorithmVelocityCoherent
	pipeline.UpdateConfig(newConfig)

	gotConfig := pipeline.GetConfig()
	if gotConfig.ActiveAlgorithm != AlgorithmVelocityCoherent {
		t.Errorf("Expected algorithm %s after update, got %s", AlgorithmVelocityCoherent, gotConfig.ActiveAlgorithm)
	}
}

func TestDualExtractionPipeline_ProcessFrame_BackgroundSubtraction(t *testing.T) {
	config := DefaultDualPipelineConfig()
	config.ActiveAlgorithm = AlgorithmBackgroundSubtraction
	config.BackgroundSubtractionEnabled = true
	pipeline := NewDualExtractionPipeline(config)

	// Create some clusters
	clusters := []WorldCluster{
		{
			ClusterID:   1,
			CentroidX:   5.0,
			CentroidY:   0.0,
			CentroidZ:   0.5,
			PointsCount: 2,
		},
	}

	// Process frame - should not panic
	pipeline.ProcessFrame(nil, clusters, time.Now(), "test-sensor")

	stats := pipeline.GetStats()
	if stats.BGFramesProcessed != 1 {
		t.Errorf("Expected 1 BG frame processed, got %d", stats.BGFramesProcessed)
	}
}

func TestDualExtractionPipeline_ProcessFrame_VelocityCoherent(t *testing.T) {
	config := DefaultDualPipelineConfig()
	config.ActiveAlgorithm = AlgorithmVelocityCoherent
	config.VelocityCoherentEnabled = true
	pipeline := NewDualExtractionPipeline(config)

	// Create some world points
	points := []WorldPoint{
		{X: 5.0, Y: 0.0, Z: 0.5},
		{X: 5.1, Y: 0.1, Z: 0.5},
		{X: 5.2, Y: -0.1, Z: 0.5},
	}

	// Process frame - should not panic
	pipeline.ProcessFrame(points, nil, time.Now(), "test-sensor")

	stats := pipeline.GetStats()
	if stats.VCFramesProcessed != 1 {
		t.Errorf("Expected 1 VC frame processed, got %d", stats.VCFramesProcessed)
	}
}

func TestDualExtractionPipeline_ProcessFrame_Dual(t *testing.T) {
	config := DefaultDualPipelineConfig()
	config.ActiveAlgorithm = AlgorithmDual
	config.BackgroundSubtractionEnabled = true
	config.VelocityCoherentEnabled = true
	pipeline := NewDualExtractionPipeline(config)

	// Create points and clusters
	points := []WorldPoint{
		{X: 5.0, Y: 0.0, Z: 0.5},
		{X: 5.1, Y: 0.1, Z: 0.5},
	}
	clusters := []WorldCluster{
		{
			ClusterID:   1,
			CentroidX:   5.0,
			CentroidY:   0.0,
			CentroidZ:   0.5,
			PointsCount: 2,
		},
	}

	// Process frame - should process with both algorithms
	pipeline.ProcessFrame(points, clusters, time.Now(), "test-sensor")

	stats := pipeline.GetStats()
	if stats.BGFramesProcessed != 1 {
		t.Errorf("Expected 1 BG frame processed in dual mode, got %d", stats.BGFramesProcessed)
	}
	if stats.VCFramesProcessed != 1 {
		t.Errorf("Expected 1 VC frame processed in dual mode, got %d", stats.VCFramesProcessed)
	}
}

func TestDualExtractionPipeline_GetBGTracker(t *testing.T) {
	config := DefaultDualPipelineConfig()
	pipeline := NewDualExtractionPipeline(config)

	tracker := pipeline.GetBGTracker()
	if tracker == nil {
		t.Error("GetBGTracker returned nil")
	}
}

func TestDualExtractionPipeline_GetVCTracker(t *testing.T) {
	config := DefaultDualPipelineConfig()
	pipeline := NewDualExtractionPipeline(config)

	tracker := pipeline.GetVCTracker()
	if tracker == nil {
		t.Error("GetVCTracker returned nil")
	}
}

func TestDualExtractionPipeline_GetStats(t *testing.T) {
	config := DefaultDualPipelineConfig()
	pipeline := NewDualExtractionPipeline(config)

	stats := pipeline.GetStats()

	// All stats should be zero initially
	if stats.BGFramesProcessed != 0 {
		t.Errorf("Expected 0 BG frames processed, got %d", stats.BGFramesProcessed)
	}
	if stats.VCFramesProcessed != 0 {
		t.Errorf("Expected 0 VC frames processed, got %d", stats.VCFramesProcessed)
	}
}

func TestDualExtractionPipeline_ConcurrentAccess(t *testing.T) {
	config := DefaultDualPipelineConfig()
	config.ActiveAlgorithm = AlgorithmDual
	pipeline := NewDualExtractionPipeline(config)

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				_ = pipeline.GetConfig()
				_ = pipeline.GetStats()
				_ = pipeline.GetBGTracker()
				_ = pipeline.GetVCTracker()
			}
			done <- true
		}()
	}

	// Concurrent writers
	go func() {
		for j := 0; j < 50; j++ {
			points := []WorldPoint{{X: float64(j), Y: 0.0, Z: 0.5}}
			clusters := []WorldCluster{{ClusterID: int64(j), PointsCount: 1}}
			pipeline.ProcessFrame(points, clusters, time.Now(), "test")
		}
		done <- true
	}()

	// Config updater
	go func() {
		for j := 0; j < 50; j++ {
			cfg := pipeline.GetConfig()
			if j%2 == 0 {
				cfg.ActiveAlgorithm = AlgorithmBackgroundSubtraction
			} else {
				cfg.ActiveAlgorithm = AlgorithmVelocityCoherent
			}
			pipeline.UpdateConfig(cfg)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 7; i++ {
		<-done
	}
}

func TestTrackingAlgorithm_String(t *testing.T) {
	tests := []struct {
		alg      TrackingAlgorithm
		expected string
	}{
		{AlgorithmBackgroundSubtraction, "background_subtraction"},
		{AlgorithmVelocityCoherent, "velocity_coherent"},
		{AlgorithmDual, "dual"},
	}

	for _, tt := range tests {
		if got := tt.alg.String(); got != tt.expected {
			t.Errorf("Algorithm %v String() = %s, expected %s", tt.alg, got, tt.expected)
		}
	}
}

func TestTrackingAlgorithm_IsValid(t *testing.T) {
	tests := []struct {
		alg      TrackingAlgorithm
		expected bool
	}{
		{AlgorithmBackgroundSubtraction, true},
		{AlgorithmVelocityCoherent, true},
		{AlgorithmDual, true},
		{TrackingAlgorithm("unknown"), false},
		{TrackingAlgorithm(""), false},
	}

	for _, tt := range tests {
		if got := tt.alg.IsValid(); got != tt.expected {
			t.Errorf("Algorithm %v IsValid() = %v, expected %v", tt.alg, got, tt.expected)
		}
	}
}

func TestDefaultDualPipelineConfig(t *testing.T) {
	config := DefaultDualPipelineConfig()

	if config.ActiveAlgorithm != AlgorithmBackgroundSubtraction {
		t.Errorf("Default algorithm should be background_subtraction, got %s", config.ActiveAlgorithm)
	}

	if !config.BackgroundSubtractionEnabled {
		t.Error("Background subtraction should be enabled by default")
	}

	if config.VelocityCoherentEnabled {
		t.Error("Velocity coherent should be disabled by default")
	}
}
