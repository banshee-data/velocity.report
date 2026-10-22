package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/report/chart"
)

// gridMetric is one settling metric the grid chart can colour by.
type gridMetric struct {
	label string
	ratio bool
	// value returns the bucket's value, false where it is undefined.
	value func(b l3grid.CoarseBucket) (float64, bool)
}

// gridMetrics are the metrics GET /api/charts/grid accepts, keyed by their
// query value. Each matches the definition the Python grid-heatmap tool used.
var gridMetrics = map[string]gridMetric{
	"fill_rate": {"Fill rate", true, func(b l3grid.CoarseBucket) (float64, bool) {
		return ratioOf(b.FilledCells, b.TotalCells)
	}},
	"settle_rate": {"Settle rate", true, func(b l3grid.CoarseBucket) (float64, bool) {
		return ratioOf(b.SettledCells, b.FilledCells)
	}},
	"unsettled_ratio": {"Unsettled ratio", true, func(b l3grid.CoarseBucket) (float64, bool) {
		return ratioOf(b.FilledCells-b.SettledCells, b.FilledCells)
	}},
	"frozen_ratio": {"Frozen ratio", true, func(b l3grid.CoarseBucket) (float64, bool) {
		return ratioOf(b.FrozenCells, b.TotalCells)
	}},
	"mean_times_seen": {"Mean times seen", false, func(b l3grid.CoarseBucket) (float64, bool) {
		return b.MeanTimesSeen, b.FilledCells > 0
	}},
}

func ratioOf(num, den int) (float64, bool) {
	if den <= 0 {
		return 0, false
	}
	return float64(num) / float64(den), true
}

// gridMetricNames lists the accepted metric values, for error messages.
func gridMetricNames() string {
	names := make([]string, 0, len(gridMetrics))
	for name := range gridMetrics {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// handleGridChart serves the background grid as SVG.
//
//	GET /api/charts/grid?sensor_id=S&metric=settle_rate&azimuth_bucket_deg=3&settled_threshold=5
//
// A sensor with no background grid yet gets a labelled message chart, not an
// error, so the web panel always has something to show.
func (ws *Server) handleGridChart(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sensorID := q.Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}
	metricName := q.Get("metric")
	if metricName == "" {
		metricName = "settle_rate"
	}
	metric, ok := gridMetrics[metricName]
	if !ok {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'metric'. Must be one of: %s", gridMetricNames()))
		return
	}
	azBucketDeg := 3.0
	if v := q.Get("azimuth_bucket_deg"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil || parsed < 1 || parsed > 90 {
			ws.writeJSONError(w, http.StatusBadRequest, "'azimuth_bucket_deg' must be a number from 1 to 90")
			return
		}
		azBucketDeg = parsed
	}
	settledThreshold := uint32(5)
	if v := q.Get("settled_threshold"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, "'settled_threshold' must be a non-negative integer")
			return
		}
		settledThreshold = uint32(parsed)
	}

	style := chart.DefaultGridHeatmapStyle()
	var svg []byte
	heatmap := l3grid.GetBackgroundManager(sensorID).GetGridHeatmap(azBucketDeg, settledThreshold)
	if heatmap == nil || len(heatmap.Buckets) == 0 {
		svg = chart.RenderGridMessage(style, "No background grid yet", "for sensor "+sensorID)
	} else {
		var err error
		svg, err = chart.RenderGridHeatmap(gridChartData(heatmap, metric), style)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not render grid chart: %v", err))
			return
		}
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(svg)
}

// gridChartData maps the aggregated grid onto the chart's input.
func gridChartData(h *l3grid.GridHeatmap, metric gridMetric) chart.GridHeatmapData {
	rings := 0
	cells := make([]chart.GridHeatmapCell, 0, len(h.Buckets))
	for _, b := range h.Buckets {
		if b.Ring+1 > rings {
			rings = b.Ring + 1
		}
		cell := chart.GridHeatmapCell{Ring: b.Ring, AzimuthStartDeg: b.AzimuthDegStart, AzimuthEndDeg: b.AzimuthDegEnd}
		if v, ok := metric.value(b); ok {
			cell.Value = &v
		}
		cells = append(cells, cell)
	}
	return chart.GridHeatmapData{
		Title:       "Background grid: " + metric.label,
		Subtitle:    fmt.Sprintf("%s at %s", h.SensorID, h.Timestamp.UTC().Format(time.RFC3339)),
		MetricLabel: metric.label,
		Ratio:       metric.ratio,
		Rings:       rings,
		Cells:       cells,
		Summary:     gridSummary(h.Summary),
	}
}

// gridSummary writes the grid totals under the chart, skipping any the
// aggregate does not carry.
func gridSummary(s map[string]interface{}) []string {
	count := func(key string) (int, bool) {
		switch v := s[key].(type) {
		case int:
			return v, true
		case float64:
			return int(v), true
		}
		return 0, false
	}
	share := func(key string) string {
		if v, ok := s[key].(float64); ok {
			return fmt.Sprintf(" (%.1f%%)", v*100)
		}
		return ""
	}
	var lines []string
	if n, ok := count("total_filled"); ok {
		lines = append(lines, fmt.Sprintf("Filled: %d%s", n, share("fill_rate")))
	}
	if n, ok := count("total_settled"); ok {
		lines = append(lines, fmt.Sprintf("Settled: %d%s", n, share("settle_rate")))
	}
	if n, ok := count("total_frozen"); ok {
		lines = append(lines, fmt.Sprintf("Frozen: %d", n))
	}
	if len(lines) == 0 {
		return nil
	}
	return []string{strings.Join(lines, " · ")}
}
