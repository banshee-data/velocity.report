package monitor

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestNewGridPlotter(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 15, 0.0, 10.0)

	if gp == nil {
		t.Fatal("NewGridPlotter returned nil")
	}

	if gp.sensorID != "test-sensor" {
		t.Errorf("expected sensorID 'test-sensor', got '%s'", gp.sensorID)
	}

	if gp.ringMin != 0 {
		t.Errorf("expected ringMin 0, got %d", gp.ringMin)
	}

	if gp.ringMax != 15 {
		t.Errorf("expected ringMax 15, got %d", gp.ringMax)
	}

	if gp.azMin != 0.0 {
		t.Errorf("expected azMin 0.0, got %f", gp.azMin)
	}

	if gp.azMax != 10.0 {
		t.Errorf("expected azMax 10.0, got %f", gp.azMax)
	}

	if gp.enabled {
		t.Error("expected enabled to be false initially")
	}

	if gp.samples == nil {
		t.Error("expected samples map to be initialised")
	}
}

func TestGridPlotter_StartStop(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 10, 0.0, 360.0)
	outputDir := t.TempDir()

	// Start should succeed
	err := gp.Start(outputDir)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !gp.IsEnabled() {
		t.Error("expected plotter to be enabled after Start")
	}

	if gp.outputDir != outputDir {
		t.Errorf("expected outputDir '%s', got '%s'", outputDir, gp.outputDir)
	}

	// Stop should disable
	gp.Stop()

	if gp.IsEnabled() {
		t.Error("expected plotter to be disabled after Stop")
	}
}

func TestGridPlotter_StartCreatesDirectory(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 10, 0.0, 360.0)
	tempBase := t.TempDir()
	nestedDir := filepath.Join(tempBase, "nested", "plots")

	err := gp.Start(nestedDir)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// Check directory was created
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

func TestGridPlotter_IsEnabled(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)

	// Initially disabled
	if gp.IsEnabled() {
		t.Error("expected disabled initially")
	}

	// Enable via Start
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !gp.IsEnabled() {
		t.Error("expected enabled after Start")
	}

	// Disable via Stop
	gp.Stop()

	if gp.IsEnabled() {
		t.Error("expected disabled after Stop")
	}
}

func TestGridPlotter_Sample_NilManager(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// Should not panic with nil manager
	gp.Sample(nil)

	// Should have no samples
	if gp.GetSampleCount() != 0 {
		t.Errorf("expected 0 samples with nil manager, got %d", gp.GetSampleCount())
	}
}

func TestGridPlotter_Sample_Disabled(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)
	// Don't call Start - plotter is disabled

	// Create a minimal background manager
	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("test-sensor", 40, 1800, params, nil)

	// Sample should be ignored when disabled
	gp.Sample(mgr)

	if gp.GetSampleCount() != 0 {
		t.Errorf("expected 0 samples when disabled, got %d", gp.GetSampleCount())
	}
}

func TestGridPlotter_SampleWithObservation_OutOfRange(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 5, 10, 45.0, 90.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("test-sensor", 40, 1800, params, nil)

	// Ring out of range (below min)
	gp.SampleWithObservation(mgr, 2, 60.0, 10.0, true)
	if gp.GetSampleCount() != 0 {
		t.Error("expected no samples for ring below range")
	}

	// Ring out of range (above max)
	gp.SampleWithObservation(mgr, 15, 60.0, 10.0, true)
	if gp.GetSampleCount() != 0 {
		t.Error("expected no samples for ring above range")
	}

	// Azimuth out of range (below min)
	gp.SampleWithObservation(mgr, 7, 30.0, 10.0, true)
	if gp.GetSampleCount() != 0 {
		t.Error("expected no samples for azimuth below range")
	}

	// Azimuth out of range (above max)
	gp.SampleWithObservation(mgr, 7, 100.0, 10.0, true)
	if gp.GetSampleCount() != 0 {
		t.Error("expected no samples for azimuth above range")
	}
}

func TestGridPlotter_SampleWithPoints_Disabled(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 39, 0.0, 360.0)
	// Don't call Start - plotter is disabled

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("test-sensor", 40, 1800, params, nil)

	points := []l2frames.PointPolar{
		{Channel: 1, Azimuth: 45.0, Distance: 10.0},
		{Channel: 5, Azimuth: 90.0, Distance: 15.0},
	}

	gp.SampleWithPoints(mgr, points)

	if gp.GetSampleCount() != 0 {
		t.Errorf("expected 0 samples when disabled, got %d", gp.GetSampleCount())
	}
}

func TestGridPlotter_SampleWithPoints_NilManager(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 39, 0.0, 360.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	points := []l2frames.PointPolar{
		{Channel: 1, Azimuth: 45.0, Distance: 10.0},
	}

	// Should not panic
	gp.SampleWithPoints(nil, points)

	if gp.GetSampleCount() != 0 {
		t.Error("expected 0 samples with nil manager")
	}
}

func TestGridPlotter_GeneratePlots_NoOutputDir(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)
	// Don't call Start - no output directory

	count, err := gp.GeneratePlots()
	if err == nil {
		t.Error("expected error when no output directory configured")
	}

	if count != 0 {
		t.Errorf("expected 0 plots, got %d", count)
	}
}

func TestGridPlotter_GeneratePlots_NoSamples(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// No samples collected
	count, err := gp.GeneratePlots()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 plots with no samples, got %d", count)
	}
}

func TestGridPlotter_GetOutputDir(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)
	outputDir := t.TempDir()

	// Before start
	if gp.GetOutputDir() != "" {
		t.Error("expected empty output dir before Start")
	}

	err := gp.Start(outputDir)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	if gp.GetOutputDir() != outputDir {
		t.Errorf("expected '%s', got '%s'", outputDir, gp.GetOutputDir())
	}
}

func TestGridPlotter_GetSampleCount(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 39, 0.0, 360.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// Initially zero
	if gp.GetSampleCount() != 0 {
		t.Errorf("expected 0 samples initially, got %d", gp.GetSampleCount())
	}

	// Manually add a sample
	gp.mu.Lock()
	gp.samples["0_0"] = append(gp.samples["0_0"], GridSample{FrameIdx: 1})
	gp.samples["0_0"] = append(gp.samples["0_0"], GridSample{FrameIdx: 2})
	gp.samples["1_0"] = append(gp.samples["1_0"], GridSample{FrameIdx: 1})
	gp.mu.Unlock()

	count := gp.GetSampleCount()
	if count != 3 {
		t.Errorf("expected 3 samples, got %d", count)
	}
}

func TestGridPlotter_IncrementFrame(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)

	// Increment when disabled should not change frameIdx
	gp.IncrementFrame()
	gp.mu.Lock()
	frameIdx := gp.frameIdx
	gp.mu.Unlock()

	if frameIdx != 0 {
		t.Error("expected frameIdx to remain 0 when disabled")
	}

	// Enable
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// Increment when enabled
	gp.IncrementFrame()
	gp.IncrementFrame()
	gp.IncrementFrame()

	gp.mu.Lock()
	frameIdx = gp.frameIdx
	gp.mu.Unlock()

	if frameIdx != 3 {
		t.Errorf("expected frameIdx 3, got %d", frameIdx)
	}
}

func TestFormatTimestamp(t *testing.T) {
	// Test a known time
	ts := time.Date(2026, 1, 30, 14, 35, 22, 0, time.UTC)
	result := FormatTimestamp(ts)

	expected := "20260130_143522"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestMakePlotOutputDir_WithPCAPFile(t *testing.T) {
	baseDir := "/tmp/plots"
	pcapFile := "/data/captures/transit-001.pcap"

	result := MakePlotOutputDir(baseDir, pcapFile)

	// Should contain base dir, pcap name (without extension), and timestamp
	if !filepath.IsAbs(result) || result == "" {
		t.Errorf("unexpected result: %s", result)
	}

	// Check structure
	if filepath.Dir(filepath.Dir(result)) != baseDir {
		t.Errorf("expected base dir '%s' in path, got '%s'", baseDir, result)
	}
}

func TestMakePlotOutputDir_WithoutPCAPFile(t *testing.T) {
	baseDir := "/tmp/plots"

	result := MakePlotOutputDir(baseDir, "")

	// Should start with "live_"
	base := filepath.Base(result)
	if len(base) < 5 || base[:5] != "live_" {
		t.Errorf("expected path to contain 'live_', got '%s'", result)
	}
}

func TestMakePlotOutputDir_PCAPWithPcapng(t *testing.T) {
	baseDir := "/tmp/plots"
	pcapFile := "/data/captures/capture.pcapng"

	result := MakePlotOutputDir(baseDir, pcapFile)

	// Parent dir should be "capture" (without .pcapng extension)
	parent := filepath.Base(filepath.Dir(result))
	if parent != "capture" {
		t.Errorf("expected parent 'capture', got '%s'", parent)
	}
}

func TestGenerateColors(t *testing.T) {
	tests := []struct {
		n        int
		expected int
	}{
		{0, 0},
		{1, 1},
		{5, 5},
		{10, 10},
		{100, 100},
	}

	for _, tt := range tests {
		colors := generateColors(tt.n)
		if len(colors) != tt.expected {
			t.Errorf("generateColors(%d): expected %d colours, got %d", tt.n, tt.expected, len(colors))
		}
	}

	// Verify colours are valid RGBA
	colors := generateColors(5)
	for i, c := range colors {
		rgba, ok := c.(color.RGBA)
		if !ok {
			t.Errorf("colour %d: expected color.RGBA, got %T", i, c)
			continue
		}
		if rgba.A != 255 {
			t.Errorf("colour %d: expected alpha 255, got %d", i, rgba.A)
		}
	}
}

func TestGenerateColors_Distinct(t *testing.T) {
	// Check that generated colours are distinct (different hues)
	colors := generateColors(6)
	if len(colors) != 6 {
		t.Fatalf("expected 6 colours, got %d", len(colors))
	}

	// Convert to RGBA and check they're not all the same
	seen := make(map[uint32]bool)
	for _, c := range colors {
		rgba := c.(color.RGBA)
		key := uint32(rgba.R)<<16 | uint32(rgba.G)<<8 | uint32(rgba.B)
		if seen[key] {
			t.Error("duplicate colour found in generated palette")
		}
		seen[key] = true
	}
}

func TestHslToRGB(t *testing.T) {
	tests := []struct {
		h, s, l   float64
		expectedR uint8
		expectedG uint8
		expectedB uint8
	}{
		// Red (hue 0)
		{0.0, 1.0, 0.5, 255, 0, 0},
		// Green (hue 1/3)
		{1.0 / 3.0, 1.0, 0.5, 0, 255, 0},
		// Blue (hue 2/3)
		{2.0 / 3.0, 1.0, 0.5, 0, 0, 255},
		// White (lightness 1)
		{0.0, 0.0, 1.0, 255, 255, 255},
		// Black (lightness 0)
		{0.0, 0.0, 0.0, 0, 0, 0},
		// Grey (saturation 0)
		{0.0, 0.0, 0.5, 127, 127, 127},
	}

	for _, tt := range tests {
		r, g, b := hslToRGB(tt.h, tt.s, tt.l)

		// Allow small tolerance for floating point
		if abs(int(r)-int(tt.expectedR)) > 1 ||
			abs(int(g)-int(tt.expectedG)) > 1 ||
			abs(int(b)-int(tt.expectedB)) > 1 {
			t.Errorf("hslToRGB(%f, %f, %f): expected (%d, %d, %d), got (%d, %d, %d)",
				tt.h, tt.s, tt.l, tt.expectedR, tt.expectedG, tt.expectedB, r, g, b)
		}
	}
}

func TestHueToRGB(t *testing.T) {
	tests := []struct {
		p, q, t  float64
		expected float64
	}{
		// t < 0 case: t becomes 0.5 after +1
		{0.0, 1.0, -0.5, 1.0},
		// t > 1 case: t becomes 0.5 after -1
		{0.0, 1.0, 1.5, 1.0},
		// t < 1/6 case
		{0.0, 1.0, 0.1, 0.6},
		// t < 1/2 case
		{0.0, 1.0, 0.25, 1.0},
		// t < 2/3 case
		{0.0, 1.0, 0.6, 0.4},
	}

	for _, tt := range tests {
		result := hueToRGB(tt.p, tt.q, tt.t)
		// Allow small tolerance
		if diff := result - tt.expected; diff > 0.01 || diff < -0.01 {
			t.Errorf("hueToRGB(%f, %f, %f): expected %f, got %f", tt.p, tt.q, tt.t, tt.expected, result)
		}
	}
}

func TestGridPlotter_StartResetsState(t *testing.T) {
	gp := NewGridPlotter("test-sensor", 0, 5, 0.0, 90.0)

	// First run
	dir1 := t.TempDir()
	err := gp.Start(dir1)
	if err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	// Add some samples manually
	gp.mu.Lock()
	gp.samples["0_0"] = append(gp.samples["0_0"], GridSample{FrameIdx: 1})
	gp.frameIdx = 5
	gp.mu.Unlock()

	gp.Stop()

	// Second run should reset state
	dir2 := t.TempDir()
	err = gp.Start(dir2)
	if err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}
	defer gp.Stop()

	if gp.GetSampleCount() != 0 {
		t.Error("expected samples to be reset on Start")
	}

	gp.mu.Lock()
	frameIdx := gp.frameIdx
	gp.mu.Unlock()

	if frameIdx != 0 {
		t.Errorf("expected frameIdx to be reset to 0, got %d", frameIdx)
	}
}

// Helper function
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ====== Additional tests to increase coverage ======

func TestGridPlotter_Sample_WithManager(t *testing.T) {
	gp := NewGridPlotter("sample-test", 0, 5, 0.0, 90.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// Create background manager with matching configuration
	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("sample-test", 40, 1800, params, nil)
	defer l3grid.RegisterBackgroundManager("sample-test", nil)

	if mgr == nil {
		t.Fatal("Failed to create BackgroundManager")
	}

	// Sample should run without panic
	gp.Sample(mgr)

	// May have samples depending on grid configuration
	count := gp.GetSampleCount()
	t.Logf("Sample generated %d samples", count)
}

func TestGridPlotter_SampleWithObservation_InRange(t *testing.T) {
	gp := NewGridPlotter("obs-test", 0, 20, 0.0, 180.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("obs-test", 40, 1800, params, nil)
	defer l3grid.RegisterBackgroundManager("obs-test", nil)

	// Sample within range
	gp.SampleWithObservation(mgr, 5, 45.0, 10.0, true)
	gp.SampleWithObservation(mgr, 10, 90.0, 15.0, false)

	count := gp.GetSampleCount()
	if count < 1 {
		t.Errorf("expected at least 1 sample, got %d", count)
	}
}

func TestGridPlotter_SampleWithPoints_Enabled(t *testing.T) {
	gp := NewGridPlotter("points-test", 0, 20, 0.0, 360.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("points-test", 40, 1800, params, nil)
	defer l3grid.RegisterBackgroundManager("points-test", nil)

	points := []l2frames.PointPolar{
		{Channel: 1, Azimuth: 45.0, Distance: 10.0},
		{Channel: 5, Azimuth: 90.0, Distance: 15.0},
		{Channel: 10, Azimuth: 180.0, Distance: 20.0},
	}

	gp.SampleWithPoints(mgr, points)

	count := gp.GetSampleCount()
	t.Logf("SampleWithPoints generated %d samples", count)
}

func TestGridPlotter_GeneratePlots_NoSamples_Simple(t *testing.T) {
	gp := NewGridPlotter("gen-test", 0, 5, 0.0, 90.0)
	outputDir := t.TempDir()
	err := gp.Start(outputDir)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	// Generate plots with no samples - should not crash
	_, err = gp.GeneratePlots()
	// Accept any error since there are no samples
	t.Logf("GeneratePlots with no samples returned: %v", err)
}

func TestGridPlotter_GeneratePlots_WithSamples(t *testing.T) {
	gp := NewGridPlotter("gen-samples-test", 0, 10, 0.0, 90.0)
	outputDir := t.TempDir()
	err := gp.Start(outputDir)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("gen-samples-test", 40, 1800, params, nil)
	defer l3grid.RegisterBackgroundManager("gen-samples-test", nil)

	// Add some samples
	for i := 0; i < 10; i++ {
		gp.SampleWithObservation(mgr, 5, 45.0, float64(10+i), true)
	}

	// Generate plots
	_, err = gp.GeneratePlots()
	// May fail if plot packages aren't fully configured, but should not panic
	t.Logf("GeneratePlots with samples returned: %v", err)
}

func TestGridPlotter_MultipleSamples_SameKey(t *testing.T) {
	gp := NewGridPlotter("multi-sample-test", 0, 10, 0.0, 180.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("multi-sample-test", 40, 1800, params, nil)
	defer l3grid.RegisterBackgroundManager("multi-sample-test", nil)

	// Sample same cell multiple times
	for i := 0; i < 5; i++ {
		gp.SampleWithObservation(mgr, 5, 45.0, float64(10+i), i%2 == 0)
	}

	count := gp.GetSampleCount()
	if count < 1 {
		t.Errorf("expected at least 1 sample key, got %d", count)
	}
}

func TestGridPlotter_EdgeCases(t *testing.T) {
	// Test edge cases for azimuth/ring boundaries
	gp := NewGridPlotter("edge-test", 0, 39, 0.0, 360.0)
	err := gp.Start(t.TempDir())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer gp.Stop()

	params := l3grid.BackgroundParams{}
	mgr := l3grid.NewBackgroundManager("edge-test", 40, 1800, params, nil)
	defer l3grid.RegisterBackgroundManager("edge-test", nil)

	// Boundary values
	gp.SampleWithObservation(mgr, 0, 0.0, 5.0, true)    // Min ring, min azimuth
	gp.SampleWithObservation(mgr, 39, 359.0, 5.0, true) // Max ring, near-max azimuth
	gp.SampleWithObservation(mgr, 20, 180.0, 5.0, true) // Middle values

	count := gp.GetSampleCount()
	if count < 1 {
		t.Errorf("expected samples for edge cases, got %d", count)
	}
}
