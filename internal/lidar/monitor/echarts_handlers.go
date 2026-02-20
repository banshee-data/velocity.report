package monitor

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// handleBackgroundGridPolar renders a quick polar plot (HTML) of the background grid using go-echarts.
// This is a debugging-only endpoint (no auth) to visually compare grid vs observations without the Svelte UI.
// Query params:
//   - sensor_id (optional; defaults to configured sensor)
//   - max_points (optional; default 8000) to reduce payload size
func (ws *WebServer) handleBackgroundGridPolar(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	maxPoints := 8000
	if mp := r.URL.Query().Get("max_points"); mp != "" {
		if v, err := strconv.Atoi(mp); err == nil && v > 100 && v <= 50000 {
			maxPoints = v
		}
	}

	cells := bm.GetGridCells()
	if len(cells) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no background cells available")
		return
	}

	// Downsample by stride to stay within maxPoints
	stride := 1
	if len(cells) > maxPoints {
		stride = int(math.Ceil(float64(len(cells)) / float64(maxPoints)))
	}

	data := make([]opts.ScatterData, 0, len(cells)/stride+1)
	maxAbs := 0.0
	maxSeen := float64(0)
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

		seen := float64(c.TimesSeen)
		if seen > maxSeen {
			maxSeen = seen
		}

		data = append(data, opts.ScatterData{Value: []interface{}{x, y, seen}})
	}

	// Add a small padding so points at the edges are visible
	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	if maxSeen == 0 {
		maxSeen = 1
	}

	// Force a square plot by using equal width/height and symmetric axis ranges
	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Background (Polar->XY)", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "LiDAR Background Grid", Subtitle: fmt.Sprintf("sensor=%s points=%d stride=%d", sensorID, len(data), stride)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxSeen),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)

	scatter.AddSeries("background", data, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 3}))

	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render chart: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleLidarDebugDashboard renders a simple dashboard with iframes to the debug charts.
func (ws *WebServer) handleLidarDebugDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}
	safeSensorID := html.EscapeString(sensorID)
	qs := ""
	if sensorID != "" {
		qs = "?sensor_id=" + url.QueryEscape(sensorID)
	}
	safeQs := html.EscapeString(qs)

	doc := fmt.Sprintf(dashboardHTML, safeSensorID, safeQs)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(doc))
}

// handleSweepDashboard renders the parameter sweep dashboard with ECharts visualisations.
func (ws *WebServer) handleSweepDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}
	// Use html.EscapeString because the value is interpolated into an HTML
	// attribute (meta tag) in sweep_dashboard.html — the JS now reads
	// sensorId from the DOM instead of a string literal.
	safeSensorID := html.EscapeString(sensorID)

	doc := fmt.Sprintf(sweepDashboardHTML, safeSensorID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(doc))
}

// handleTrafficChart renders a simple bar chart of packet/point throughput.
func (ws *WebServer) handleTrafficChart(w http.ResponseWriter, r *http.Request) {
	if ws.stats == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no packet stats available")
		return
	}

	snap := ws.stats.GetLatestSnapshot()
	if snap == nil {
		snap = &StatsSnapshot{Timestamp: time.Now()}
	}

	x := []string{"Packets/s", "MB/s", "Points/s", "Dropped (recent)"}
	y := []opts.BarData{
		{Value: snap.PacketsPerSec},
		{Value: snap.MBPerSec},
		{Value: snap.PointsPerSec},
		{Value: snap.DroppedCount},
	}

	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "720px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "LiDAR Traffic", Subtitle: snap.Timestamp.Format(time.RFC3339)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
	)
	bar.SetXAxis(x).
		AddSeries("traffic", y,
			charts.WithLabelOpts(opts.Label{Show: opts.Bool(true), Position: "top"}),
		)

	page := components.NewPage()
	page.SetAssetsHost(echartsAssetsPrefix)
	page.AddCharts(bar)

	var buf bytes.Buffer
	if err := page.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("render error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleBackgroundGridHeatmapChart renders a coarse heatmap (as colored scatter)
// using the aggregated buckets returned by GetGridHeatmap.
func (ws *WebServer) handleBackgroundGridHeatmapChart(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil || bm.Grid == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
		return
	}

	azBucketDeg := 3.0
	if v := r.URL.Query().Get("azimuth_bucket_deg"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 1.0 {
			azBucketDeg = parsed
		}
	}
	settledThreshold := uint32(5)
	if v := r.URL.Query().Get("settled_threshold"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			settledThreshold = uint32(parsed)
		}
	}

	heatmap := bm.GetGridHeatmap(azBucketDeg, settledThreshold)
	if heatmap == nil || len(heatmap.Buckets) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no heatmap buckets available")
		return
	}

	// Build scatter points colored by MeanTimesSeen (or settled cells)
	points := make([]opts.ScatterData, 0, len(heatmap.Buckets))
	maxVal := 0.0
	for _, b := range heatmap.Buckets {
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

	maxAbs := 0.0
	for _, b := range heatmap.Buckets {
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

		norm := b.MeanTimesSeen / maxVal
		if norm < 0 {
			norm = 0
		}
		if norm > 1 {
			norm = 1
		}

		pt := opts.ScatterData{Value: []interface{}{x, y, b.MeanTimesSeen}}
		points = append(points, pt)
	}

	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Background Heatmap", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "LiDAR Background Heatmap", Subtitle: fmt.Sprintf("sensor=%s buckets=%d az=%g", sensorID, len(points), azBucketDeg)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxVal),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)
	scatter.AddSeries("heatmap", points, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 10}))

	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render heatmap chart: %v", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleClustersChart renders recent clusters as scatter points (color by point count).
func (ws *WebServer) handleClustersChart(w http.ResponseWriter, r *http.Request) {
	if ws.trackAPI == nil || ws.trackAPI.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "track DB not configured")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	// time window (seconds)
	var startNanos, endNanos int64
	if s := r.URL.Query().Get("start"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			startNanos = parsed * 1e9
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if parsed, err := strconv.ParseInt(e, 10, 64); err == nil {
			endNanos = parsed * 1e9
		}
	}
	if endNanos == 0 {
		endNanos = time.Now().UnixNano()
	}
	if startNanos == 0 {
		startNanos = endNanos - int64(time.Hour)
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	clusters, err := sqlite.GetRecentClusters(ws.trackAPI.db, sensorID, startNanos, endNanos, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get clusters: %v", err))
		return
	}

	pts := make([]opts.ScatterData, 0, len(clusters))
	maxPts := 1
	for _, c := range clusters {
		if c.PointsCount > maxPts {
			maxPts = c.PointsCount
		}
	}
	maxAbs := 0.0
	for _, c := range clusters {
		x := float64(c.CentroidX)
		y := float64(c.CentroidY)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		pt := opts.ScatterData{Value: []interface{}{x, y, c.PointsCount}}
		pts = append(pts, pt)
	}
	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Clusters", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "Recent Clusters", Subtitle: fmt.Sprintf("sensor=%s count=%d", sensorID, len(pts))}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxPts),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)
	scatter.AddSeries("clusters", pts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 10}))
	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render clusters chart: %v", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleTracksChart renders current track positions (and optionally recent observations) as a scatter overlay.
func (ws *WebServer) handleTracksChart(w http.ResponseWriter, r *http.Request) {
	if ws.trackAPI == nil || ws.trackAPI.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "track DB not configured")
		return
	}
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	state := r.URL.Query().Get("state")
	tracks, err := sqlite.GetActiveTracks(ws.trackAPI.db, sensorID, state)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get tracks: %v", err))
		return
	}

	pts := make([]opts.ScatterData, 0, len(tracks))
	maxAbs := 0.0
	maxObs := 0
	for _, t := range tracks {
		x := float64(t.X)
		y := float64(t.Y)
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		if t.ObservationCount > maxObs {
			maxObs = t.ObservationCount
		}
		pt := opts.ScatterData{Value: []interface{}{x, y, t.ObservationCount}}
		pts = append(pts, pt)
	}
	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}
	if maxObs == 0 {
		maxObs = 1
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Tracks", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "Active Tracks", Subtitle: fmt.Sprintf("sensor=%s count=%d", sensorID, len(pts))}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Show:       opts.Bool(true),
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(maxObs),
			Dimension:  "2",
			InRange:    &opts.VisualMapInRange{Color: []string{"#440154", "#482777", "#3e4989", "#31688e", "#26828e", "#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725"}},
		}),
	)
	scatter.AddSeries("tracks", pts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 8}))
	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render tracks chart: %v", err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleForegroundFrameChart renders the most recent foreground frame cached from the pipeline.
// Points are pulled from the in-memory foreground snapshot instead of the track observations table
// so the chart reflects the full per-point mask output (point density, foreground fraction, etc.).
func (ws *WebServer) handleForegroundFrameChart(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	snapshot := l3grid.GetForegroundSnapshot(sensorID)
	if snapshot == nil || (len(snapshot.ForegroundPoints) == 0 && len(snapshot.BackgroundPoints) == 0) {
		ws.writeJSONError(w, http.StatusNotFound, "no foreground snapshot available")
		return
	}

	fgPts := make([]opts.ScatterData, 0, len(snapshot.ForegroundPoints))
	bgPts := make([]opts.ScatterData, 0, len(snapshot.BackgroundPoints))
	maxAbs := 0.0

	for _, p := range snapshot.BackgroundPoints {
		x := p.X
		y := p.Y
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		bgPts = append(bgPts, opts.ScatterData{Value: []interface{}{x, y}})
	}

	for _, p := range snapshot.ForegroundPoints {
		x := p.X
		y := p.Y
		if math.Abs(x) > maxAbs {
			maxAbs = math.Abs(x)
		}
		if math.Abs(y) > maxAbs {
			maxAbs = math.Abs(y)
		}
		fgPts = append(fgPts, opts.ScatterData{Value: []interface{}{x, y}})
	}

	pad := maxAbs * 1.05
	if pad == 0 {
		pad = 1.0
	}

	fraction := 0.0
	if snapshot.TotalPoints > 0 {
		fraction = float64(snapshot.ForegroundCount) / float64(snapshot.TotalPoints)
	}

	subtitle := fmt.Sprintf(
		"sensor=%s ts=%s fg=%d bg=%d total=%d (%.2f%% fg)",
		sensorID,
		snapshot.Timestamp.UTC().Format(time.RFC3339),
		snapshot.ForegroundCount,
		snapshot.BackgroundCount,
		snapshot.TotalPoints,
		fraction*100,
	)

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "LiDAR Foreground Frame", Theme: "dark", Width: "900px", Height: "900px", AssetsHost: echartsAssetsPrefix}),
		charts.WithTitleOpts(opts.Title{Title: "Foreground vs Background", Subtitle: subtitle}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true)}),
		charts.WithXAxisOpts(opts.XAxis{Min: -pad, Max: pad, Name: "X (m)", NameLocation: "middle", NameGap: 25}),
		charts.WithYAxisOpts(opts.YAxis{Min: -pad, Max: pad, Name: "Y (m)", NameLocation: "middle", NameGap: 30}),
	)

	scatter.AddSeries("background", bgPts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 4}), charts.WithItemStyleOpts(opts.ItemStyle{Color: "#9e9e9e"}))
	scatter.AddSeries("foreground", fgPts, charts.WithScatterChartOpts(opts.ScatterChart{SymbolSize: 6}), charts.WithItemStyleOpts(opts.ItemStyle{Color: "#ff5252"}))

	var buf bytes.Buffer
	if err := scatter.Render(&buf); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to render foreground chart: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// handleBackgroundRegionsDashboard renders an HTML visualization of the background regions
func (ws *WebServer) handleBackgroundRegionsDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	escapedSensorID := html.EscapeString(sensorID)

	// Use escapedSensorID for both instances (title and meta tag)
	// The template has been updated to use %[1]s in both places and handle decoding via DOM
	doc := fmt.Sprintf(regionsDashboardHTML, escapedSensorID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(doc))
}
