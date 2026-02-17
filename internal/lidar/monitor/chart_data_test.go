package monitor

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestPreparePolarChartData_Empty(t *testing.T) {
	result := PreparePolarChartData(nil, "test-sensor", 1000)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Points) != 0 {
		t.Errorf("expected empty points, got %d", len(result.Points))
	}
	if result.SensorID != "test-sensor" {
		t.Errorf("expected sensor ID 'test-sensor', got %q", result.SensorID)
	}
}

func TestPreparePolarChartData_SinglePoint(t *testing.T) {
	cells := []l3grid.ExportedCell{
		{AzimuthDeg: 0, Range: 10, TimesSeen: 5},
	}

	result := PreparePolarChartData(cells, "sensor-1", 1000)

	if len(result.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result.Points))
	}

	p := result.Points[0]
	// At azimuth 0, X should be range, Y should be 0
	if math.Abs(p.X-10) > 0.001 {
		t.Errorf("expected X=10, got %f", p.X)
	}
	if math.Abs(p.Y) > 0.001 {
		t.Errorf("expected Y=0, got %f", p.Y)
	}
	if p.Value != 5 {
		t.Errorf("expected Value=5, got %f", p.Value)
	}
}

func TestPreparePolarChartData_MultiplePoints(t *testing.T) {
	cells := []l3grid.ExportedCell{
		{AzimuthDeg: 0, Range: 10, TimesSeen: 1},
		{AzimuthDeg: 90, Range: 10, TimesSeen: 2},
		{AzimuthDeg: 180, Range: 10, TimesSeen: 3},
		{AzimuthDeg: 270, Range: 10, TimesSeen: 4},
	}

	result := PreparePolarChartData(cells, "sensor-1", 1000)

	if len(result.Points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(result.Points))
	}

	// Check that MaxValue is correctly computed
	if result.MaxValue != 4 {
		t.Errorf("expected MaxValue=4, got %f", result.MaxValue)
	}

	// Check stride is 1 (no downsampling needed)
	if result.Stride != 1 {
		t.Errorf("expected Stride=1, got %d", result.Stride)
	}
}

func TestPreparePolarChartData_Downsampling(t *testing.T) {
	// Create more points than maxPoints
	cells := make([]l3grid.ExportedCell, 100)
	for i := range cells {
		cells[i] = l3grid.ExportedCell{AzimuthDeg: float32(i * 3), Range: 10, TimesSeen: uint32(i)}
	}

	result := PreparePolarChartData(cells, "sensor-1", 10)

	// With 100 points and maxPoints=10, stride should be ceil(100/10) = 10
	if result.Stride != 10 {
		t.Errorf("expected Stride=10, got %d", result.Stride)
	}

	// Should have about 10 points
	if len(result.Points) > 15 {
		t.Errorf("expected ~10 points, got %d", len(result.Points))
	}
}

func TestPreparePolarChartData_PolarToCartesian(t *testing.T) {
	tests := []struct {
		azimuth float32
		rangeM  float32
		wantX   float64
		wantY   float64
	}{
		{0, 10, 10, 0},    // East
		{90, 10, 0, 10},   // North
		{180, 10, -10, 0}, // West
		{270, 10, 0, -10}, // South
		{45, 10, 10 * math.Cos(math.Pi/4), 10 * math.Sin(math.Pi/4)}, // NE
	}

	for _, tc := range tests {
		cells := []l3grid.ExportedCell{
			{AzimuthDeg: tc.azimuth, Range: tc.rangeM, TimesSeen: 1},
		}
		result := PreparePolarChartData(cells, "test", 1000)

		if len(result.Points) != 1 {
			t.Fatalf("expected 1 point for azimuth %f", tc.azimuth)
		}

		p := result.Points[0]
		if math.Abs(p.X-tc.wantX) > 0.001 {
			t.Errorf("azimuth %f: expected X=%f, got %f", tc.azimuth, tc.wantX, p.X)
		}
		if math.Abs(p.Y-tc.wantY) > 0.001 {
			t.Errorf("azimuth %f: expected Y=%f, got %f", tc.azimuth, tc.wantY, p.Y)
		}
	}
}

func TestPreparePolarChartData_ZeroMaxPoints(t *testing.T) {
	cells := []l3grid.ExportedCell{
		{AzimuthDeg: 0, Range: 10, TimesSeen: 1},
	}

	// maxPoints <= 0 should default to 8000
	result := PreparePolarChartData(cells, "test", 0)

	if result.Stride != 1 {
		t.Errorf("expected Stride=1 with default maxPoints, got %d", result.Stride)
	}
}

func TestPrepareHeatmapChartData_Empty(t *testing.T) {
	result := PrepareHeatmapChartData(nil, []string{"a", "b"}, []string{"1", "2"}, "test")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Cells) != 0 {
		t.Errorf("expected empty cells, got %d", len(result.Cells))
	}
}

func TestPrepareHeatmapChartData_2x2Grid(t *testing.T) {
	values := [][]float64{
		{1.0, 2.0},
		{3.0, 4.0},
	}
	xLabels := []string{"col0", "col1"}
	yLabels := []string{"row0", "row1"}

	result := PrepareHeatmapChartData(values, xLabels, yLabels, "sensor-1")

	if len(result.Cells) != 4 {
		t.Fatalf("expected 4 cells, got %d", len(result.Cells))
	}
	if result.MaxValue != 4.0 {
		t.Errorf("expected MaxValue=4, got %f", result.MaxValue)
	}
	if result.MinValue != 1.0 {
		t.Errorf("expected MinValue=1, got %f", result.MinValue)
	}
	if result.NumCells != 4 {
		t.Errorf("expected NumCells=4, got %d", result.NumCells)
	}
}

func TestPrepareHeatmapChartData_CellPositions(t *testing.T) {
	values := [][]float64{
		{10.0, 20.0, 30.0},
	}

	result := PrepareHeatmapChartData(values, nil, nil, "test")

	// Should have 3 cells at positions (0,0), (1,0), (2,0)
	if len(result.Cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(result.Cells))
	}

	for i, cell := range result.Cells {
		if cell.Y != 0 {
			t.Errorf("cell %d: expected Y=0, got %d", i, cell.Y)
		}
		if cell.X != i {
			t.Errorf("cell %d: expected X=%d, got %d", i, i, cell.X)
		}
	}
}

func TestPrepareClustersChartData_Empty(t *testing.T) {
	result := PrepareClustersChartData(nil, "test")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Points) != 0 {
		t.Errorf("expected empty points, got %d", len(result.Points))
	}
	if result.NumClusters != 0 {
		t.Errorf("expected 0 clusters, got %d", result.NumClusters)
	}
}

func TestPrepareClustersChartData_MultipleClusters(t *testing.T) {
	clusters := [][]l3grid.ExportedCell{
		{{AzimuthDeg: 0, Range: 5, TimesSeen: 1}},
		{{AzimuthDeg: 90, Range: 10, TimesSeen: 1}, {AzimuthDeg: 91, Range: 10, TimesSeen: 1}},
	}

	result := PrepareClustersChartData(clusters, "sensor-1")

	if result.NumClusters != 2 {
		t.Errorf("expected 2 clusters, got %d", result.NumClusters)
	}
	if len(result.Points) != 3 {
		t.Errorf("expected 3 total points, got %d", len(result.Points))
	}

	// Check cluster IDs are assigned correctly
	if result.Points[0].ClusterID != 0 {
		t.Errorf("first point should have ClusterID=0, got %d", result.Points[0].ClusterID)
	}
	if result.Points[1].ClusterID != 1 || result.Points[2].ClusterID != 1 {
		t.Error("cluster 1 points should have ClusterID=1")
	}
}

func TestPrepareTrafficMetrics_Nil(t *testing.T) {
	result := PrepareTrafficMetrics(nil)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.PacketsPerSec != 0 {
		t.Errorf("expected 0 PacketsPerSec, got %f", result.PacketsPerSec)
	}
}

func TestPrepareTrafficMetrics_WithData(t *testing.T) {
	snap := &StatsSnapshot{
		Timestamp:     time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC),
		PacketsPerSec: 100.5,
		MBPerSec:      1.5,
		PointsPerSec:  5000.0,
		DroppedCount:  10,
	}

	result := PrepareTrafficMetrics(snap)

	if result.PacketsPerSec != 100.5 {
		t.Errorf("expected PacketsPerSec=100.5, got %f", result.PacketsPerSec)
	}
	if result.MBPerSec != 1.5 {
		t.Errorf("expected MBPerSec=1.5, got %f", result.MBPerSec)
	}
	if result.PointsPerSec != 5000.0 {
		t.Errorf("expected PointsPerSec=5000, got %f", result.PointsPerSec)
	}
	if result.DroppedCount != 10 {
		t.Errorf("expected DroppedCount=10, got %d", result.DroppedCount)
	}
	if result.Timestamp != "2026-01-31T12:00:00Z" {
		t.Errorf("expected timestamp '2026-01-31T12:00:00Z', got %q", result.Timestamp)
	}
}

func TestPreparePolarChartData_MaxAbsPadding(t *testing.T) {
	cells := []l3grid.ExportedCell{
		{AzimuthDeg: 0, Range: 100, TimesSeen: 1},
	}

	result := PreparePolarChartData(cells, "test", 1000)

	// MaxAbs should be 100 * 1.05 = 105
	expected := 100.0 * 1.05
	if math.Abs(result.MaxAbs-expected) > 0.001 {
		t.Errorf("expected MaxAbs=%f, got %f", expected, result.MaxAbs)
	}
}

func TestPreparePolarChartData_ZeroTimesSeen(t *testing.T) {
	cells := []l3grid.ExportedCell{
		{AzimuthDeg: 0, Range: 10, TimesSeen: 0},
	}

	result := PreparePolarChartData(cells, "test", 1000)

	// MaxValue should default to 1 when all values are 0
	if result.MaxValue != 1 {
		t.Errorf("expected MaxValue=1 for zero data, got %f", result.MaxValue)
	}
}
