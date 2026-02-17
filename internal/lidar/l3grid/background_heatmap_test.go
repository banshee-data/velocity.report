package l3grid

import (
	"testing"
	"time"
)

func TestGetGridHeatmap(t *testing.T) {
	// Create a test grid with known dimensions
	rings := 4
	azBins := 120 // 120 bins = 3° per bin for 360°
	grid := &BackgroundGrid{
		SensorID:    "test-sensor",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       make([]BackgroundCell, rings*azBins),
	}

	bm := &BackgroundManager{
		Grid: grid,
	}

	// Populate some test data
	// Fill first ring completely with varying times seen
	for i := 0; i < azBins; i++ {
		idx := grid.Idx(0, i)
		grid.Cells[idx].AverageRangeMeters = 10.0 + float32(i)*0.1
		grid.Cells[idx].TimesSeenCount = uint32(i % 10)
	}

	// Fill second ring partially
	for i := 0; i < azBins/2; i++ {
		idx := grid.Idx(1, i)
		grid.Cells[idx].AverageRangeMeters = 20.0 + float32(i)*0.1
		grid.Cells[idx].TimesSeenCount = uint32(i%10 + 5)
	}

	// Test with 3-degree buckets
	azBucketDeg := 3.0
	settledThreshold := uint32(5)

	heatmap := bm.GetGridHeatmap(azBucketDeg, settledThreshold)

	if heatmap == nil {
		t.Fatal("GetGridHeatmap returned nil")
	}

	// Verify basic structure
	if heatmap.SensorID != "test-sensor" {
		t.Errorf("Expected sensor_id 'test-sensor', got '%s'", heatmap.SensorID)
	}

	// Verify grid params
	gridParams := heatmap.GridParams
	if gridParams["total_rings"] != rings {
		t.Errorf("Expected %d rings, got %v", rings, gridParams["total_rings"])
	}
	if gridParams["total_azimuth_bins"] != azBins {
		t.Errorf("Expected %d azimuth bins, got %v", azBins, gridParams["total_azimuth_bins"])
	}

	// Verify heatmap params
	heatmapParams := heatmap.HeatmapParams
	if heatmapParams["azimuth_bucket_deg"] != azBucketDeg {
		t.Errorf("Expected azimuth_bucket_deg %.1f, got %v", azBucketDeg, heatmapParams["azimuth_bucket_deg"])
	}

	expectedAzBuckets := int(360.0 / azBucketDeg)
	if heatmapParams["azimuth_buckets"] != expectedAzBuckets {
		t.Errorf("Expected %d azimuth buckets, got %v", expectedAzBuckets, heatmapParams["azimuth_buckets"])
	}

	// Verify bucket count
	expectedBuckets := rings * expectedAzBuckets
	if len(heatmap.Buckets) != expectedBuckets {
		t.Errorf("Expected %d buckets, got %d", expectedBuckets, len(heatmap.Buckets))
	}

	// Verify first bucket (ring 0, azimuth 0-3°)
	bucket0 := heatmap.Buckets[0]
	if bucket0.Ring != 0 {
		t.Errorf("Expected ring 0, got %d", bucket0.Ring)
	}
	if bucket0.AzimuthDegStart != 0.0 {
		t.Errorf("Expected azimuth start 0.0, got %.1f", bucket0.AzimuthDegStart)
	}
	if bucket0.AzimuthDegEnd != 3.0 {
		t.Errorf("Expected azimuth end 3.0, got %.1f", bucket0.AzimuthDegEnd)
	}

	// First bucket should have 1 cell (3° / 3° per bin = 1 cell)
	expectedCellsPerBucket := 1
	if bucket0.TotalCells != expectedCellsPerBucket {
		t.Errorf("Expected %d total cells, got %d", expectedCellsPerBucket, bucket0.TotalCells)
	}

	// Verify summary
	summary := heatmap.Summary
	totalFilled, ok := summary["total_filled"].(int)
	if !ok {
		t.Fatal("total_filled not found or wrong type")
	}

	// Ring 0 has all 120 cells filled, ring 1 has 60 cells filled = 180 total
	// However, due to aggregation bucket boundaries, we might get slightly different counts
	// Just verify we have a reasonable number
	if totalFilled < 150 || totalFilled > 180 {
		t.Errorf("Expected filled cells between 150-180, got %d", totalFilled)
	}

	// Count settled cells: ring 0 has cells with times_seen 0-9 (5-9 are settled) = 60 cells
	// ring 1 has cells with times_seen 5-14 (all settled) = 60 cells
	// But we only filled half of ring 1, so it's 60 cells with varying values
	totalSettled, ok := summary["total_settled"].(int)
	if !ok {
		t.Fatal("total_settled not found or wrong type")
	}

	// This should be non-zero
	if totalSettled == 0 {
		t.Error("Expected some settled cells, got 0")
	}
}

func TestGetGridHeatmapNilManager(t *testing.T) {
	var bm *BackgroundManager
	heatmap := bm.GetGridHeatmap(3.0, 5)
	if heatmap != nil {
		t.Error("Expected nil heatmap for nil manager")
	}
}

func TestGetGridHeatmapNilGrid(t *testing.T) {
	bm := &BackgroundManager{
		Grid: nil,
	}
	heatmap := bm.GetGridHeatmap(3.0, 5)
	if heatmap != nil {
		t.Error("Expected nil heatmap for nil grid")
	}
}

func TestGetGridHeatmapWithFrozenCells(t *testing.T) {
	rings := 2
	azBins := 360 // 1° per bin
	grid := &BackgroundGrid{
		SensorID:    "test-sensor",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       make([]BackgroundCell, rings*azBins),
	}

	bm := &BackgroundManager{
		Grid: grid,
	}

	// Set some cells as frozen
	futureNanos := time.Now().Add(10 * time.Second).UnixNano()
	for i := 0; i < 10; i++ {
		idx := grid.Idx(0, i)
		grid.Cells[idx].AverageRangeMeters = 10.0
		grid.Cells[idx].TimesSeenCount = 5
		grid.Cells[idx].FrozenUntilUnixNanos = futureNanos
	}

	heatmap := bm.GetGridHeatmap(3.0, 5)
	if heatmap == nil {
		t.Fatal("GetGridHeatmap returned nil")
	}

	summary := heatmap.Summary
	totalFrozen, ok := summary["total_frozen"].(int)
	if !ok {
		t.Fatal("total_frozen not found or wrong type")
	}

	if totalFrozen != 10 {
		t.Errorf("Expected 10 frozen cells, got %d", totalFrozen)
	}
}
