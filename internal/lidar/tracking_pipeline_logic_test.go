package lidar

import (
	"sync"
	"testing"
	"time"
)

func TestTrackingPipeline_SetExtractorMode(t *testing.T) {
	sensorID := "test-sensor"
	// Initialize BackgroundManager with basic params
	bgManager := NewBackgroundManager(sensorID, 16, 360, BackgroundParams{
		BackgroundUpdateFraction: 0.1,
	}, nil)

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bgManager,
		SensorID:          sensorID,
		ExtractorMode:     "background", // Start with default
	}

	tp := NewTrackingPipeline(cfg)

	// Helper to check extractor type
	checkType := func(expectedMode string) {
		tp.mu.RLock()
		extractor := tp.extractor
		mode := tp.config.ExtractorMode
		tp.mu.RUnlock()

		if mode != expectedMode {
			t.Errorf("Config mode expected %s, got %s", expectedMode, mode)
		}

		if extractor == nil {
			t.Errorf("Extractor is nil for mode %s", expectedMode)
			return
		}

		var isCorrectType bool
		switch expectedMode {
		case "background":
			_, isCorrectType = extractor.(*BackgroundSubtractorExtractor)
		case "velocity":
			_, isCorrectType = extractor.(*VelocityCoherentExtractor)
		case "hybrid":
			_, isCorrectType = extractor.(*HybridExtractor)
		}

		if !isCorrectType {
			t.Errorf("Extractor type does not match mode %s. Got %T", expectedMode, extractor)
		}
	}

	// Initial state
	checkType("background")

	// Switch to velocity
	tp.SetExtractorMode("velocity")
	checkType("velocity")
	if tp.GetExtractorMode() != "velocity" {
		t.Errorf("GetExtractorMode returned wrong value")
	}

	// Switch to hybrid
	tp.SetExtractorMode("hybrid")
	checkType("hybrid")

	// Switch back to background
	tp.SetExtractorMode("background")
	checkType("background")
}

func TestTrackingPipeline_Concurrency(t *testing.T) {
	// This test ensures no races between frame processing and mode switching
	sensorID := "test-sensor-conc"
	bgManager := NewBackgroundManager(sensorID, 16, 360, BackgroundParams{BackgroundUpdateFraction: 0.1}, nil)

	cfg := &TrackingPipelineConfig{
		BackgroundManager: bgManager,
		SensorID:          sensorID,
		ExtractorMode:     "background",
		// Tracker is nil, so pipeline stops after clustering
	}

	tp := NewTrackingPipeline(cfg)
	frameCb := tp.FrameCallback()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Simulate frame processing stream
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Run loop until done signal
		for {
			select {
			case <-done:
				return
			default:
				// Create a dummy frame
				// We provide empty points so it returns early but definitely passes the lock acquisition
				frame := &LiDARFrame{
					FrameID:        "test",
					Points:         []Point{},
					StartTimestamp: time.Now(),
				}
				// Also try one with points to go deeper
				frameWithPoints := &LiDARFrame{
					FrameID: "test_pts",
					Points: []Point{
						{Azimuth: 10.0, Distance: 5.0, Timestamp: time.Now()},
					},
					StartTimestamp: time.Now(),
				}

				frameCb(frame)
				frameCb(frameWithPoints)

				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Simulate user switching modes
	tags := []string{"background", "velocity", "hybrid"}
	for i := 0; i < 50; i++ {
		mode := tags[i%3]
		tp.SetExtractorMode(mode)
		time.Sleep(2 * time.Millisecond)
	}

	close(done)
	wg.Wait()
}
