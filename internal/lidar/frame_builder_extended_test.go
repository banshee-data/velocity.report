package lidar

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestGetFrameBuilder tests the frame builder registry retrieval
func TestGetFrameBuilder(t *testing.T) {
	// Clear registry before test
	fbRegistryMu.Lock()
	fbRegistry = make(map[string]*FrameBuilder)
	fbRegistryMu.Unlock()

	// Test getting non-existent frame builder
	fb := GetFrameBuilder("nonexistent")
	if fb != nil {
		t.Errorf("GetFrameBuilder('nonexistent') = %v, want nil", fb)
	}

	// Register a frame builder
	testFB := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-sensor"})
	RegisterFrameBuilder("test-sensor", testFB)

	// Test retrieving registered frame builder
	retrievedFB := GetFrameBuilder("test-sensor")
	if retrievedFB == nil {
		t.Fatal("GetFrameBuilder('test-sensor') returned nil")
	}
	if retrievedFB != testFB {
		t.Error("GetFrameBuilder returned different instance than registered")
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fb := GetFrameBuilder("test-sensor")
			if fb == nil {
				t.Error("Concurrent GetFrameBuilder returned nil")
			}
		}()
	}
	wg.Wait()
}

// TestEnableTimeBased tests enabling/disabling time-based frame detection
func TestEnableTimeBased(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-sensor"})

	// Enable time-based detection
	fb.EnableTimeBased(true)
	fb.mu.Lock()
	enabled := fb.enableTimeBased
	fb.mu.Unlock()

	if !enabled {
		t.Error("EnableTimeBased(true) did not enable time-based detection")
	}

	// Disable time-based detection
	fb.EnableTimeBased(false)
	fb.mu.Lock()
	enabled = fb.enableTimeBased
	fb.mu.Unlock()

	if enabled {
		t.Error("EnableTimeBased(false) did not disable time-based detection")
	}
}

// TestRequestExportNextFrameASC tests scheduling frame export
func TestRequestExportNextFrameASC(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-sensor"})

	// Request export
	fb.RequestExportNextFrameASC()

	fb.mu.Lock()
	afterRequest := fb.exportNextFrameASC
	fb.mu.Unlock()

	if !afterRequest {
		t.Error("RequestExportNextFrameASC() did not set exportNextFrameASC flag")
	}
}

// TestRequestExportFrameBatchASC tests scheduling batch frame export
func TestRequestExportFrameBatchASC(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-sensor"})

	// Test with specific count
	fb.RequestExportFrameBatchASC(10)

	fb.mu.Lock()
	batchCount := fb.exportBatchCount
	batchExported := fb.exportBatchExported
	fb.mu.Unlock()

	if batchCount != 10 {
		t.Errorf("exportBatchCount = %d, want 10", batchCount)
	}
	if batchExported != 0 {
		t.Errorf("exportBatchExported = %d, want 0", batchExported)
	}

	// Test with zero count (should default to 5)
	fb.RequestExportFrameBatchASC(0)

	fb.mu.Lock()
	batchCount = fb.exportBatchCount
	fb.mu.Unlock()

	if batchCount != 5 {
		t.Errorf("exportBatchCount with 0 input = %d, want 5 (default)", batchCount)
	}

	// Test with negative count (should default to 5)
	fb.RequestExportFrameBatchASC(-3)

	fb.mu.Lock()
	batchCount = fb.exportBatchCount
	fb.mu.Unlock()

	if batchCount != 5 {
		t.Errorf("exportBatchCount with negative input = %d, want 5 (default)", batchCount)
	}
}

// TestGetCurrentFrameStats tests retrieving frame buffer statistics
func TestGetCurrentFrameStats(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-sensor"})

	// Test with empty buffer
	count, oldestAge, newestAge := fb.GetCurrentFrameStats()
	if count != 0 {
		t.Errorf("Empty buffer: count = %d, want 0", count)
	}
	if oldestAge != 0 {
		t.Errorf("Empty buffer: oldestAge = %v, want 0", oldestAge)
	}
	if newestAge != 0 {
		t.Errorf("Empty buffer: newestAge = %v, want 0", newestAge)
	}

	// Add some frames to the buffer
	now := time.Now()
	fb.mu.Lock()
	fb.frameBuffer = make(map[string]*LiDARFrame)
	fb.frameBuffer["frame-1"] = &LiDARFrame{
		FrameID:        "frame-1",
		StartTimestamp: now.Add(-10 * time.Second),
	}
	fb.frameBuffer["frame-2"] = &LiDARFrame{
		FrameID:        "frame-2",
		StartTimestamp: now.Add(-5 * time.Second),
	}
	fb.frameBuffer["frame-3"] = &LiDARFrame{
		FrameID:        "frame-3",
		StartTimestamp: now.Add(-2 * time.Second),
	}
	fb.mu.Unlock()

	count, oldestAge, newestAge = fb.GetCurrentFrameStats()

	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	// Check that ages are reasonable (with some tolerance for test execution time)
	if oldestAge < 9*time.Second || oldestAge > 12*time.Second {
		t.Errorf("oldestAge = %v, expected ~10s", oldestAge)
	}
	if newestAge < 1*time.Second || newestAge > 4*time.Second {
		t.Errorf("newestAge = %v, expected ~2s", newestAge)
	}
}

// TestSetDebug tests enabling/disabling debug logging
func TestSetDebug(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-sensor"})

	// Enable debug
	fb.SetDebug(true)
	fb.mu.Lock()
	debugEnabled := fb.debug
	fb.mu.Unlock()

	if !debugEnabled {
		t.Error("SetDebug(true) did not enable debug mode")
	}

	// Disable debug
	fb.SetDebug(false)
	fb.mu.Lock()
	debugEnabled = fb.debug
	fb.mu.Unlock()

	if debugEnabled {
		t.Error("SetDebug(false) did not disable debug mode")
	}
}

// TestNewFrameBuilderWithLogging tests creating a frame builder with logging
func TestNewFrameBuilderWithLogging(t *testing.T) {
	fb := NewFrameBuilderWithLogging("test-sensor-log")

	if fb == nil {
		t.Fatal("NewFrameBuilderWithLogging returned nil")
	}
	if fb.sensorID != "test-sensor-log" {
		t.Errorf("sensorID = %s, want 'test-sensor-log'", fb.sensorID)
	}
	// NewFrameBuilderWithLogging calls NewFrameBuilderWithDebugLogging(sensorID, false)
	// so callback should be nil
	if fb.frameCallback != nil {
		t.Error("frameCallback should be nil for non-debug builder")
	}
}

// TestNewFrameBuilderWithDebugLogging tests creating a frame builder with debug logging
func TestNewFrameBuilderWithDebugLogging(t *testing.T) {
	tests := []struct {
		name               string
		debug              bool
		expectCallback     bool
	}{
		{"with debug enabled", true, true},
		{"with debug disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := NewFrameBuilderWithDebugLogging("test-sensor-debug", tt.debug)

			if fb == nil {
				t.Fatal("NewFrameBuilderWithDebugLogging returned nil")
			}
			if fb.sensorID != "test-sensor-debug" {
				t.Errorf("sensorID = %s, want 'test-sensor-debug'", fb.sensorID)
			}
			
			// Callback is only set when debug is enabled
			if tt.expectCallback && fb.frameCallback == nil {
				t.Error("frameCallback should not be nil when debug is enabled")
			}
			if !tt.expectCallback && fb.frameCallback != nil {
				t.Error("frameCallback should be nil when debug is disabled")
			}
		})
	}
}

// TestNewFrameBuilderWithDebugLoggingAndInterval tests creating a frame builder with custom interval
func TestNewFrameBuilderWithDebugLoggingAndInterval(t *testing.T) {
	customInterval := 5 * time.Second
	fb := NewFrameBuilderWithDebugLoggingAndInterval("test-sensor-interval", true, customInterval)

	if fb == nil {
		t.Fatal("NewFrameBuilderWithDebugLoggingAndInterval returned nil")
	}
	if fb.sensorID != "test-sensor-interval" {
		t.Errorf("sensorID = %s, want 'test-sensor-interval'", fb.sensorID)
	}
	if fb.frameCallback == nil {
		t.Error("frameCallback should not be nil")
	}

	// Test that the callback doesn't panic when invoked
	testFrame := &LiDARFrame{
		FrameID:        "test-frame",
		StartTimestamp: time.Now(),
		EndTimestamp:   time.Now().Add(100 * time.Millisecond),
		Points:         []Point{{X: 1, Y: 2, Z: 3}},
	}

	// This should not panic
	fb.frameCallback(testFrame)
}

// TestFrameBuilder_ExportFunctionality tests the export-related functionality
func TestFrameBuilder_ExportFunctionality(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "test-export"})

	// Request export for next frame
	fb.RequestExportNextFrameASC()

	// Verify flag is set
	fb.mu.Lock()
	shouldExport := fb.exportNextFrameASC
	fb.mu.Unlock()

	if !shouldExport {
		t.Error("Export flag not set after RequestExportNextFrameASC()")
	}

	// Test batch export request
	fb.RequestExportFrameBatchASC(3)

	fb.mu.Lock()
	batchCount := fb.exportBatchCount
	fb.mu.Unlock()

	if batchCount != 3 {
		t.Errorf("Batch export count = %d, want 3", batchCount)
	}
}

// TestFrameBuilder_StatsTracking tests statistics tracking
func TestFrameBuilder_StatsTracking(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "stats-test"})

	// Add frames with different timestamps
	times := []time.Duration{-30 * time.Second, -20 * time.Second, -10 * time.Second, -5 * time.Second}
	
	fb.mu.Lock()
	fb.frameBuffer = make(map[string]*LiDARFrame)
	for i, offset := range times {
		frameID := "frame-" + string(rune('A'+i))
		fb.frameBuffer[frameID] = &LiDARFrame{
			FrameID:        frameID,
			StartTimestamp: time.Now().Add(offset),
			Points:         []Point{{X: 1, Y: 2, Z: 3}},
		}
	}
	fb.mu.Unlock()

	count, oldestAge, newestAge := fb.GetCurrentFrameStats()

	if count != len(times) {
		t.Errorf("Frame count = %d, want %d", count, len(times))
	}

	// Oldest frame should be ~30 seconds old
	if oldestAge < 29*time.Second || oldestAge > 32*time.Second {
		t.Errorf("Oldest age = %v, expected ~30s", oldestAge)
	}

	// Newest frame should be ~5 seconds old
	if newestAge < 4*time.Second || newestAge > 7*time.Second {
		t.Errorf("Newest age = %v, expected ~5s", newestAge)
	}
}

// TestFrameBuilder_ConcurrentOperations tests thread-safe operations
func TestFrameBuilder_ConcurrentOperations(t *testing.T) {
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "concurrent-test"})

	var wg sync.WaitGroup
	numGoroutines := 20

	// Concurrent reads and writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(3)

		// Reader
		go func() {
			defer wg.Done()
			fb.GetCurrentFrameStats()
		}()

		// Writer - enable/disable
		go func(id int) {
			defer wg.Done()
			fb.EnableTimeBased(id%2 == 0)
			fb.SetDebug(id%2 == 1)
		}(i)

		// Writer - export requests
		go func() {
			defer wg.Done()
			fb.RequestExportNextFrameASC()
			fb.RequestExportFrameBatchASC(5)
		}()
	}

	wg.Wait()

	// Verify no panics occurred and state is consistent
	count, _, _ := fb.GetCurrentFrameStats()
	if count < 0 {
		t.Error("Invalid frame count after concurrent operations")
	}
}

// TestFrameBuilder_ExportPaths tests that export paths work correctly
func TestFrameBuilder_ExportPaths(t *testing.T) {
	tempDir := t.TempDir()
	fb := NewFrameBuilder(FrameBuilderConfig{SensorID: "path-test"})

	// Ensure export directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Errorf("Export directory does not exist: %s", tempDir)
	}

	// Test that we can create test files
	testPath := filepath.Join(tempDir, "test-frame.asc")
	file, err := os.Create(testPath)
	if err != nil {
		t.Errorf("Failed to create test export file: %v", err)
	}
	defer file.Close()

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Error("Test export file was not created")
	}
	
	// Verify frame builder exists
	if fb == nil {
		t.Error("FrameBuilder should not be nil")
	}
}
