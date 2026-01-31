// Package monitor provides chart data preparation utilities for LiDAR visualisation.
// This file separates data transformation from eCharts rendering for improved testability.
package monitor

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// ScatterPoint represents a single point in an XY scatter chart.
type ScatterPoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Value float64 `json:"value"`
}

// PolarChartData holds prepared data for rendering a polar/scatter chart.
type PolarChartData struct {
	Points    []ScatterPoint `json:"points"`
	MaxAbs    float64        `json:"max_abs"`
	MaxValue  float64        `json:"max_value"`
	SensorID  string         `json:"sensor_id"`
	Stride    int            `json:"stride"`
	NumPoints int            `json:"num_points"`
}

// HeatmapCell represents a single cell in a heatmap chart.
type HeatmapCell struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Value float64 `json:"value"`
}

// HeatmapChartData holds prepared data for rendering a heatmap chart.
type HeatmapChartData struct {
	Cells    []HeatmapCell `json:"cells"`
	XLabels  []string      `json:"x_labels"`
	YLabels  []string      `json:"y_labels"`
	MaxValue float64       `json:"max_value"`
	MinValue float64       `json:"min_value"`
	SensorID string        `json:"sensor_id"`
	NumCells int           `json:"num_cells"`
}

// ClusterPoint represents a point belonging to a cluster.
type ClusterPoint struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	ClusterID int     `json:"cluster_id"`
}

// ClustersChartData holds prepared data for rendering a clusters chart.
type ClustersChartData struct {
	Points      []ClusterPoint `json:"points"`
	NumClusters int            `json:"num_clusters"`
	MaxAbs      float64        `json:"max_abs"`
	SensorID    string         `json:"sensor_id"`
}

// TrafficMetrics holds traffic statistics for chart display.
type TrafficMetrics struct {
	PacketsPerSec float64 `json:"packets_per_sec"`
	MBPerSec      float64 `json:"mb_per_sec"`
	PointsPerSec  float64 `json:"points_per_sec"`
	DroppedCount  int64   `json:"dropped_count"`
	Timestamp     string  `json:"timestamp"`
}

// PreparePolarChartData transforms background grid cells into scatter chart data.
// It handles coordinate conversion from polar to Cartesian and downsampling.
func PreparePolarChartData(cells []lidar.ExportedCell, sensorID string, maxPoints int) *PolarChartData {
	if len(cells) == 0 {
		return &PolarChartData{
			Points:   []ScatterPoint{},
			SensorID: sensorID,
		}
	}

	if maxPoints <= 0 {
		maxPoints = 8000
	}

	// Downsample by stride to stay within maxPoints
	stride := 1
	if len(cells) > maxPoints {
		stride = int(math.Ceil(float64(len(cells)) / float64(maxPoints)))
	}

	points := make([]ScatterPoint, 0, len(cells)/stride+1)
	maxAbs := 0.0
	maxValue := 0.0

	for i := 0; i < len(cells); i += stride {
		c := cells[i]
		theta := float64(c.AzimuthDeg) * math.Pi / 180.0
		x := float64(c.Range) * math.Cos(theta)
		y := float64(c.Range) * math.Sin(theta)

		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}

		value := float64(c.TimesSeen)
		if value > maxValue {
			maxValue = value
		}

		points = append(points, ScatterPoint{X: x, Y: y, Value: value})
	}

	// Add padding so points at the edges are visible
	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	if maxValue == 0 {
		maxValue = 1
	}

	return &PolarChartData{
		Points:    points,
		MaxAbs:    maxAbs,
		MaxValue:  maxValue,
		SensorID:  sensorID,
		Stride:    stride,
		NumPoints: len(points),
	}
}

// PrepareHeatmapChartData transforms 2D grid data into heatmap chart data.
// xSize and ySize define the grid dimensions.
func PrepareHeatmapChartData(values [][]float64, xLabels, yLabels []string, sensorID string) *HeatmapChartData {
	if len(values) == 0 {
		return &HeatmapChartData{
			Cells:    []HeatmapCell{},
			XLabels:  xLabels,
			YLabels:  yLabels,
			SensorID: sensorID,
		}
	}

	cells := make([]HeatmapCell, 0)
	maxValue := math.Inf(-1)
	minValue := math.Inf(1)

	for y, row := range values {
		for x, val := range row {
			cells = append(cells, HeatmapCell{X: x, Y: y, Value: val})
			if val > maxValue {
				maxValue = val
			}
			if val < minValue {
				minValue = val
			}
		}
	}

	if math.IsInf(maxValue, -1) {
		maxValue = 0
	}
	if math.IsInf(minValue, 1) {
		minValue = 0
	}

	return &HeatmapChartData{
		Cells:    cells,
		XLabels:  xLabels,
		YLabels:  yLabels,
		MaxValue: maxValue,
		MinValue: minValue,
		SensorID: sensorID,
		NumCells: len(cells),
	}
}

// PrepareClustersChartData transforms cluster data into chart-ready format.
func PrepareClustersChartData(clusters [][]lidar.ExportedCell, sensorID string) *ClustersChartData {
	points := make([]ClusterPoint, 0)
	maxAbs := 0.0

	for clusterID, cluster := range clusters {
		for _, c := range cluster {
			theta := float64(c.AzimuthDeg) * math.Pi / 180.0
			x := float64(c.Range) * math.Cos(theta)
			y := float64(c.Range) * math.Sin(theta)

			if math.Abs(x) > maxAbs {
				maxAbs = math.Abs(x)
			}
			if math.Abs(y) > maxAbs {
				maxAbs = math.Abs(y)
			}

			points = append(points, ClusterPoint{X: x, Y: y, ClusterID: clusterID})
		}
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	return &ClustersChartData{
		Points:      points,
		NumClusters: len(clusters),
		MaxAbs:      maxAbs,
		SensorID:    sensorID,
	}
}

// PrepareTrafficMetrics transforms packet statistics into chart-ready format.
func PrepareTrafficMetrics(snap *StatsSnapshot) *TrafficMetrics {
	if snap == nil {
		return &TrafficMetrics{}
	}

	return &TrafficMetrics{
		PacketsPerSec: snap.PacketsPerSec,
		MBPerSec:      snap.MBPerSec,
		PointsPerSec:  snap.PointsPerSec,
		DroppedCount:  snap.DroppedCount,
		Timestamp:     snap.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}
}
