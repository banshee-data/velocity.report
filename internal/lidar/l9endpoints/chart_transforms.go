package l9endpoints

import (
	"math"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// HeatmapBucketData represents prepared heatmap data from grid buckets.
type HeatmapBucketData struct {
	Points     []ScatterPoint `json:"points"`
	MaxValue   float64        `json:"max_value"`
	MaxAbs     float64        `json:"max_abs"`
	SensorID   string         `json:"sensor_id"`
	NumBuckets int            `json:"num_buckets"`
}

// PrepareHeatmapFromBuckets transforms grid heatmap buckets into chart-ready data.
func PrepareHeatmapFromBuckets(buckets []l3grid.CoarseBucket, sensorID string) *HeatmapBucketData {
	points := make([]ScatterPoint, 0, len(buckets))
	maxVal := 0.0
	maxAbs := 0.0

	for _, b := range buckets {
		if b.FilledCells == 0 {
			continue
		}
		val := b.MeanTimesSeen
		if val == 0 {
			val = float64(b.SettledCells)
		}
		if val > maxVal {
			maxVal = val
		}
	}
	if maxVal == 0 {
		maxVal = 1.0
	}

	for _, b := range buckets {
		if b.FilledCells == 0 {
			continue
		}
		azMid := (b.AzimuthDegStart + b.AzimuthDegEnd) / 2.0
		rRange := b.MeanRangeMeters
		if rRange == 0 {
			rRange = (b.MinRangeMeters + b.MaxRangeMeters) / 2.0
		}

		theta := azMid * math.Pi / 180.0
		x := rRange * math.Cos(theta)
		y := rRange * math.Sin(theta)

		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}

		val := b.MeanTimesSeen
		if val == 0 {
			val = float64(b.SettledCells)
		}
		points = append(points, ScatterPoint{X: x, Y: y, Value: val})
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	return &HeatmapBucketData{
		Points:     points,
		MaxValue:   maxVal,
		MaxAbs:     maxAbs,
		SensorID:   sensorID,
		NumBuckets: len(points),
	}
}

// ForegroundChartData holds foreground/background point data for charting.
type ForegroundChartData struct {
	ForegroundPoints  []ScatterPoint `json:"foreground_points"`
	BackgroundPoints  []ScatterPoint `json:"background_points"`
	MaxAbs            float64        `json:"max_abs"`
	ForegroundCount   int            `json:"foreground_count"`
	BackgroundCount   int            `json:"background_count"`
	TotalPoints       int            `json:"total_points"`
	ForegroundPercent float64        `json:"foreground_percent"`
	Timestamp         string         `json:"timestamp"`
	SensorID          string         `json:"sensor_id"`
}

// PrepareForegroundChartData transforms a foreground snapshot into chart-ready data.
func PrepareForegroundChartData(snapshot *l3grid.ForegroundSnapshot, sensorID string) *ForegroundChartData {
	fgPts := make([]ScatterPoint, 0, len(snapshot.ForegroundPoints))
	bgPts := make([]ScatterPoint, 0, len(snapshot.BackgroundPoints))
	maxAbs := 0.0

	for _, p := range snapshot.BackgroundPoints {
		if math.Abs(p.X) > maxAbs {
			maxAbs = math.Abs(p.X)
		}
		if math.Abs(p.Y) > maxAbs {
			maxAbs = math.Abs(p.Y)
		}
		bgPts = append(bgPts, ScatterPoint{X: p.X, Y: p.Y, Value: 0})
	}

	for _, p := range snapshot.ForegroundPoints {
		if math.Abs(p.X) > maxAbs {
			maxAbs = math.Abs(p.X)
		}
		if math.Abs(p.Y) > maxAbs {
			maxAbs = math.Abs(p.Y)
		}
		fgPts = append(fgPts, ScatterPoint{X: p.X, Y: p.Y, Value: 1})
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	fgPercent := 0.0
	if snapshot.TotalPoints > 0 {
		fgPercent = float64(snapshot.ForegroundCount) / float64(snapshot.TotalPoints) * 100
	}

	return &ForegroundChartData{
		ForegroundPoints:  fgPts,
		BackgroundPoints:  bgPts,
		MaxAbs:            maxAbs,
		ForegroundCount:   snapshot.ForegroundCount,
		BackgroundCount:   snapshot.BackgroundCount,
		TotalPoints:       snapshot.TotalPoints,
		ForegroundPercent: fgPercent,
		Timestamp:         snapshot.Timestamp.UTC().Format(time.RFC3339),
		SensorID:          sensorID,
	}
}

// RecentClustersData holds cluster centroid data for charting.
type RecentClustersData struct {
	Clusters    []ClusterCentroid `json:"clusters"`
	MaxAbs      float64           `json:"max_abs"`
	MaxPoints   int               `json:"max_points"`
	NumClusters int               `json:"num_clusters"`
	SensorID    string            `json:"sensor_id"`
}

// ClusterCentroid represents a cluster's centroid position and size.
type ClusterCentroid struct {
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	PointsCount int     `json:"points_count"`
}

// PrepareRecentClustersData transforms cluster records into chart-ready data.
func PrepareRecentClustersData(clusters []*l4perception.WorldCluster, sensorID string) *RecentClustersData {
	pts := make([]ClusterCentroid, 0, len(clusters))
	maxPts := 1
	maxAbs := 0.0

	for _, c := range clusters {
		if c.PointsCount > maxPts {
			maxPts = c.PointsCount
		}
	}

	for _, c := range clusters {
		x := float64(c.CentroidX)
		y := float64(c.CentroidY)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		pts = append(pts, ClusterCentroid{X: x, Y: y, PointsCount: c.PointsCount})
	}

	if maxAbs > 0 {
		maxAbs *= 1.05
	} else {
		maxAbs = 1.0
	}

	return &RecentClustersData{
		Clusters:    pts,
		MaxAbs:      maxAbs,
		MaxPoints:   maxPts,
		NumClusters: len(clusters),
		SensorID:    sensorID,
	}
}
