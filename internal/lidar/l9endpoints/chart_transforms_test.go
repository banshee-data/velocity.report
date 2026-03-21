package l9endpoints

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// ---------- PrepareHeatmapFromBuckets ----------

func TestPrepareHeatmapFromBuckets(t *testing.T) {
	t.Run("empty buckets", func(t *testing.T) {
		result := PrepareHeatmapFromBuckets(nil, "s1")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Points) != 0 {
			t.Errorf("expected 0 points, got %d", len(result.Points))
		}
		if result.SensorID != "s1" {
			t.Errorf("sensor_id = %q, want s1", result.SensorID)
		}
		if result.MaxAbs != 1.0 {
			t.Errorf("maxAbs = %f, want 1.0 for empty input", result.MaxAbs)
		}
	})

	t.Run("single bucket", func(t *testing.T) {
		buckets := []l3grid.CoarseBucket{
			{
				FilledCells:     10,
				MeanTimesSeen:   5.0,
				AzimuthDegStart: 0,
				AzimuthDegEnd:   10,
				MeanRangeMeters: 8.0,
			},
		}
		result := PrepareHeatmapFromBuckets(buckets, "s2")
		if len(result.Points) != 1 {
			t.Fatalf("expected 1 point, got %d", len(result.Points))
		}
		if result.MaxValue != 5.0 {
			t.Errorf("MaxValue = %f, want 5.0", result.MaxValue)
		}
		if result.NumBuckets != 1 {
			t.Errorf("NumBuckets = %d, want 1", result.NumBuckets)
		}
	})

	t.Run("bucket with zero filled cells skipped", func(t *testing.T) {
		buckets := []l3grid.CoarseBucket{
			{FilledCells: 0, MeanTimesSeen: 10},
			{FilledCells: 3, MeanTimesSeen: 2, AzimuthDegStart: 0, AzimuthDegEnd: 10, MeanRangeMeters: 5},
		}
		result := PrepareHeatmapFromBuckets(buckets, "s3")
		if len(result.Points) != 1 {
			t.Errorf("expected 1 point (zero-filled skipped), got %d", len(result.Points))
		}
	})

	t.Run("uses settled cells when mean times seen is zero", func(t *testing.T) {
		buckets := []l3grid.CoarseBucket{
			{FilledCells: 5, MeanTimesSeen: 0, SettledCells: 7, AzimuthDegStart: 0, AzimuthDegEnd: 10, MeanRangeMeters: 4},
		}
		result := PrepareHeatmapFromBuckets(buckets, "s4")
		if result.MaxValue != 7.0 {
			t.Errorf("MaxValue = %f, want 7.0 (from SettledCells)", result.MaxValue)
		}
		if len(result.Points) != 1 {
			t.Fatalf("expected 1 point, got %d", len(result.Points))
		}
		if result.Points[0].Value != 7.0 {
			t.Errorf("Point.Value = %f, want 7.0 (from SettledCells fallback)", result.Points[0].Value)
		}
	})

	t.Run("fallback range from min/max when mean range is zero", func(t *testing.T) {
		buckets := []l3grid.CoarseBucket{
			{
				FilledCells:     2,
				MeanTimesSeen:   3,
				AzimuthDegStart: 90,
				AzimuthDegEnd:   90,
				MeanRangeMeters: 0,
				MinRangeMeters:  4,
				MaxRangeMeters:  6,
			},
		}
		result := PrepareHeatmapFromBuckets(buckets, "s5")
		if len(result.Points) != 1 {
			t.Fatalf("expected 1 point, got %d", len(result.Points))
		}
		// azimuth 90° → theta = pi/2, range = 5 → x ≈ 0, y = 5
		p := result.Points[0]
		if math.Abs(p.X) > 0.01 {
			t.Errorf("X = %f, expected near 0 for azimuth 90°", p.X)
		}
		if math.Abs(p.Y-5.0) > 0.01 {
			t.Errorf("Y = %f, expected near 5 for range midpoint (4+6)/2", p.Y)
		}
	})

	t.Run("all buckets zero filled gives maxVal 1", func(t *testing.T) {
		buckets := []l3grid.CoarseBucket{
			{FilledCells: 0},
			{FilledCells: 0},
		}
		result := PrepareHeatmapFromBuckets(buckets, "s6")
		if len(result.Points) != 0 {
			t.Errorf("expected 0 points, got %d", len(result.Points))
		}
		if result.MaxAbs != 1.0 {
			t.Errorf("MaxAbs = %f, want 1.0 when no points emitted", result.MaxAbs)
		}
	})
}

func TestPrepareHeatmapFromBuckets_PolarToCartesian(t *testing.T) {
	// azimuth 0° means theta = 0 → x = range, y = 0
	buckets := []l3grid.CoarseBucket{
		{FilledCells: 1, MeanTimesSeen: 1, AzimuthDegStart: 0, AzimuthDegEnd: 0, MeanRangeMeters: 10},
	}
	result := PrepareHeatmapFromBuckets(buckets, "cart")
	p := result.Points[0]
	if math.Abs(p.X-10.0) > 0.001 {
		t.Errorf("X = %f, want 10.0 at azimuth 0", p.X)
	}
	if math.Abs(p.Y) > 0.001 {
		t.Errorf("Y = %f, want 0.0 at azimuth 0", p.Y)
	}
	// maxAbs should be 10 * 1.05
	if math.Abs(result.MaxAbs-10.5) > 0.01 {
		t.Errorf("MaxAbs = %f, want 10.5", result.MaxAbs)
	}
}

// ---------- PrepareForegroundChartData ----------

func TestPrepareForegroundChartData(t *testing.T) {
	t.Run("mixed foreground and background", func(t *testing.T) {
		snap := &l3grid.ForegroundSnapshot{
			SensorID:  "fg1",
			Timestamp: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			ForegroundPoints: []l3grid.ProjectedPoint{
				{X: 1.0, Y: 2.0},
				{X: -3.0, Y: 0.5},
			},
			BackgroundPoints: []l3grid.ProjectedPoint{
				{X: 0.5, Y: -1.0},
			},
			TotalPoints:     100,
			ForegroundCount: 20,
			BackgroundCount: 80,
		}
		result := PrepareForegroundChartData(snap, "fg1")
		if len(result.ForegroundPoints) != 2 {
			t.Errorf("foreground points = %d, want 2", len(result.ForegroundPoints))
		}
		if len(result.BackgroundPoints) != 1 {
			t.Errorf("background points = %d, want 1", len(result.BackgroundPoints))
		}
		if result.ForegroundPercent != 20.0 {
			t.Errorf("ForegroundPercent = %f, want 20.0", result.ForegroundPercent)
		}
		if result.Timestamp != "2024-01-15T12:00:00Z" {
			t.Errorf("Timestamp = %q", result.Timestamp)
		}
	})

	t.Run("empty snapshot", func(t *testing.T) {
		snap := &l3grid.ForegroundSnapshot{
			Timestamp: time.Now(),
		}
		result := PrepareForegroundChartData(snap, "fg2")
		if len(result.ForegroundPoints) != 0 {
			t.Errorf("expected 0 fg points, got %d", len(result.ForegroundPoints))
		}
		if result.MaxAbs != 1.0 {
			t.Errorf("MaxAbs = %f, want 1.0 for empty snapshot", result.MaxAbs)
		}
		if result.ForegroundPercent != 0.0 {
			t.Errorf("ForegroundPercent = %f, want 0 for zero TotalPoints", result.ForegroundPercent)
		}
	})
}

func TestPrepareForegroundChartData_MaxAbs(t *testing.T) {
	snap := &l3grid.ForegroundSnapshot{
		Timestamp:        time.Now(),
		ForegroundPoints: []l3grid.ProjectedPoint{{X: 5.0, Y: -8.0}},
		BackgroundPoints: []l3grid.ProjectedPoint{{X: -2.0, Y: 3.0}},
		TotalPoints:      10,
		ForegroundCount:  1,
		BackgroundCount:  9,
	}
	result := PrepareForegroundChartData(snap, "abs")
	// max of |5|, |-8|, |-2|, |3| = 8, so maxAbs = 8 * 1.05 = 8.4
	expected := 8.0 * 1.05
	if math.Abs(result.MaxAbs-expected) > 0.001 {
		t.Errorf("MaxAbs = %f, want %f", result.MaxAbs, expected)
	}
}

func TestPrepareForegroundChartData_ForegroundPercent(t *testing.T) {
	snap := &l3grid.ForegroundSnapshot{
		Timestamp:       time.Now(),
		TotalPoints:     200,
		ForegroundCount: 50,
		BackgroundCount: 150,
	}
	result := PrepareForegroundChartData(snap, "pct")
	if result.ForegroundPercent != 25.0 {
		t.Errorf("ForegroundPercent = %f, want 25.0", result.ForegroundPercent)
	}
}

// ---------- PrepareRecentClustersData ----------

func TestPrepareRecentClustersData(t *testing.T) {
	t.Run("empty clusters", func(t *testing.T) {
		result := PrepareRecentClustersData(nil, "c1")
		if result == nil {
			t.Fatal("expected non-nil")
		}
		if len(result.Clusters) != 0 {
			t.Errorf("clusters = %d, want 0", len(result.Clusters))
		}
		if result.MaxAbs != 1.0 {
			t.Errorf("MaxAbs = %f, want 1.0", result.MaxAbs)
		}
		if result.MaxPoints != 1 {
			t.Errorf("MaxPoints = %d, want 1 (default)", result.MaxPoints)
		}
	})

	t.Run("multiple clusters", func(t *testing.T) {
		clusters := []*l4perception.WorldCluster{
			{CentroidX: 1.0, CentroidY: 2.0, PointsCount: 10},
			{CentroidX: -5.0, CentroidY: 3.0, PointsCount: 25},
		}
		result := PrepareRecentClustersData(clusters, "c2")
		if len(result.Clusters) != 2 {
			t.Errorf("clusters = %d, want 2", len(result.Clusters))
		}
		if result.MaxPoints != 25 {
			t.Errorf("MaxPoints = %d, want 25", result.MaxPoints)
		}
		if result.NumClusters != 2 {
			t.Errorf("NumClusters = %d, want 2", result.NumClusters)
		}
		if result.SensorID != "c2" {
			t.Errorf("SensorID = %q, want c2", result.SensorID)
		}
	})
}

func TestPrepareRecentClustersData_MaxAbs(t *testing.T) {
	clusters := []*l4perception.WorldCluster{
		{CentroidX: 3.0, CentroidY: -7.0, PointsCount: 5},
	}
	result := PrepareRecentClustersData(clusters, "abs")
	// max(|3|, |-7|) = 7, maxAbs = 7 * 1.05 = 7.35
	expected := 7.0 * 1.05
	if math.Abs(result.MaxAbs-expected) > 0.001 {
		t.Errorf("MaxAbs = %f, want %f", result.MaxAbs, expected)
	}
}

func TestPrepareRecentClustersData_ClusterCoordinates(t *testing.T) {
	clusters := []*l4perception.WorldCluster{
		{CentroidX: 1.5, CentroidY: -2.5, PointsCount: 8},
	}
	result := PrepareRecentClustersData(clusters, "coord")
	c := result.Clusters[0]
	if c.X != 1.5 {
		t.Errorf("X = %f, want 1.5", c.X)
	}
	if c.Y != -2.5 {
		t.Errorf("Y = %f, want -2.5", c.Y)
	}
	if c.PointsCount != 8 {
		t.Errorf("PointsCount = %d, want 8", c.PointsCount)
	}
}
