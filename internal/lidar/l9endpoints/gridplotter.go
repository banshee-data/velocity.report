package l9endpoints

import (
	"fmt"
	"html"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// GridPlotter records grid cell states over time for visualization.
// It samples the BackgroundManager's grid on each call to Sample(),
// accumulating time series data that can be plotted after a run.
type GridPlotter struct {
	mu        sync.Mutex
	enabled   bool
	outputDir string
	sensorID  string

	// Ring/azimuth range to capture (all 0.1° increments within range)
	ringMin int
	ringMax int
	azMin   float64
	azMax   float64

	// samples holds per-cell time series. Key = "ring_azBin" (e.g., "15_3200")
	samples map[string][]GridSample

	// startTime is the timestamp of the first sample, used for x-axis
	startTime time.Time
	frameIdx  int
}

// GridSample represents one snapshot of a cell's state
type GridSample struct {
	FrameIdx  int
	Timestamp time.Time
	// Background values
	BgAverage float64
	BgSpread  float64
	BgSeen    int
	// Locked baseline values (stable reference)
	LockedBaseline float64
	LockedSpread   float64
	LockedAtCount  int
	// Most recent observation values (from foreground extraction)
	ObsDist float64 // Actual observed distance for this frame
	Diff    float64
	RecFg   int
	Frozen  bool
	IsBg    bool
}

// NewGridPlotter creates a plotter for the given sensor with specified range.
func NewGridPlotter(sensorID string, ringMin, ringMax int, azMin, azMax float64) *GridPlotter {
	return &GridPlotter{
		sensorID: sensorID,
		ringMin:  ringMin,
		ringMax:  ringMax,
		azMin:    azMin,
		azMax:    azMax,
		samples:  make(map[string][]GridSample),
	}
}

// Start initializes the plotter for a new run.
// outputDir should be a timestamped directory (e.g., "plots/transit-001/20260107_173129")
func (gp *GridPlotter) Start(outputDir string) error {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	gp.outputDir = outputDir
	gp.enabled = true
	gp.startTime = time.Time{}
	gp.frameIdx = 0
	gp.samples = make(map[string][]GridSample)
	return nil
}

// Stop disables sampling. Call GeneratePlots() to produce output files.
func (gp *GridPlotter) Stop() {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	gp.enabled = false
}

// IsEnabled returns true if the plotter is currently recording.
func (gp *GridPlotter) IsEnabled() bool {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	return gp.enabled
}

// Sample captures the current grid state for all cells in the configured range.
// Call this once per frame during PCAP replay or live processing.
func (gp *GridPlotter) Sample(mgr *l3grid.BackgroundManager) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if !gp.enabled || mgr == nil || mgr.Grid == nil {
		return
	}

	g := mgr.Grid
	now := time.Now()

	if gp.startTime.IsZero() {
		gp.startTime = now
	}
	gp.frameIdx++

	// Grid resolution: how many degrees per azimuth bin
	azBinRes := 360.0 / float64(g.AzimuthBins)

	// Iterate over rings in range
	for ring := gp.ringMin; ring <= gp.ringMax && ring < g.Rings; ring++ {
		// Iterate over azimuth bins in range (0.1° resolution means we capture all bins)
		for azBin := 0; azBin < g.AzimuthBins; azBin++ {
			azDeg := float64(azBin) * azBinRes

			// Check if azimuth is in range
			if azDeg < gp.azMin || azDeg > gp.azMax {
				continue
			}

			idx := ring*g.AzimuthBins + azBin
			if idx >= len(g.Cells) {
				continue
			}

			cell := g.Cells[idx]
			key := fmt.Sprintf("%d_%d", ring, azBin)

			// Compute diff from last observed distance if we have one
			sample := GridSample{
				FrameIdx:       gp.frameIdx,
				Timestamp:      now,
				BgAverage:      float64(cell.AverageRangeMeters),
				BgSpread:       float64(cell.RangeSpreadMeters),
				BgSeen:         int(cell.TimesSeenCount),
				LockedBaseline: float64(cell.LockedBaseline),
				LockedSpread:   float64(cell.LockedSpread),
				LockedAtCount:  int(cell.LockedAtCount),
				RecFg:          int(cell.RecentForegroundCount),
				Frozen:         cell.FrozenUntilUnixNanos > now.UnixNano(),
			}

			gp.samples[key] = append(gp.samples[key], sample)
		}
	}
}

// SampleWithObservation records both background state and a specific observation.
// Use this when you have access to the current point being processed.
func (gp *GridPlotter) SampleWithObservation(mgr *l3grid.BackgroundManager, ring int, azDeg, obsDist float64, isBg bool) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if !gp.enabled || mgr == nil || mgr.Grid == nil {
		return
	}

	// Check range
	if ring < gp.ringMin || ring > gp.ringMax {
		return
	}
	if azDeg < gp.azMin || azDeg > gp.azMax {
		return
	}

	g := mgr.Grid
	now := time.Now()

	if gp.startTime.IsZero() {
		gp.startTime = now
	}

	azBinRes := 360.0 / float64(g.AzimuthBins)
	azBin := int(azDeg / azBinRes)
	if azBin >= g.AzimuthBins {
		azBin = g.AzimuthBins - 1
	}

	idx := ring*g.AzimuthBins + azBin
	if idx >= len(g.Cells) {
		return
	}

	cell := g.Cells[idx]
	key := fmt.Sprintf("%d_%d", ring, azBin)

	diff := obsDist - float64(cell.AverageRangeMeters)

	sample := GridSample{
		FrameIdx:  gp.frameIdx,
		Timestamp: now,
		BgAverage: float64(cell.AverageRangeMeters),
		BgSpread:  float64(cell.RangeSpreadMeters),
		BgSeen:    int(cell.TimesSeenCount),
		ObsDist:   obsDist,
		Diff:      diff,
		RecFg:     int(cell.RecentForegroundCount),
		Frozen:    cell.FrozenUntilUnixNanos > now.UnixNano(),
		IsBg:      isBg,
	}

	gp.samples[key] = append(gp.samples[key], sample)
}

// SampleWithPoints captures grid state and actual observations from the points array.
// This is called per-frame with the full point cloud after foreground extraction.
func (gp *GridPlotter) SampleWithPoints(mgr *l3grid.BackgroundManager, points []l2frames.PointPolar) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if !gp.enabled || mgr == nil || mgr.Grid == nil {
		return
	}

	g := mgr.Grid
	now := time.Now()

	if gp.startTime.IsZero() {
		gp.startTime = now
	}
	gp.frameIdx++

	// Grid resolution: how many degrees per azimuth bin
	azBinRes := 360.0 / float64(g.AzimuthBins)

	// Create a map to collect observations per cell for this frame
	cellObs := make(map[string][]float64) // key = "ring_azBin", value = distances

	// Collect all observations in range
	for _, p := range points {
		ring := p.Channel - 1 // Channel is 1-based
		if ring < gp.ringMin || ring > gp.ringMax {
			continue
		}
		if p.Azimuth < gp.azMin || p.Azimuth > gp.azMax {
			continue
		}

		azBin := int(p.Azimuth / azBinRes)
		if azBin >= g.AzimuthBins {
			azBin = g.AzimuthBins - 1
		}

		key := fmt.Sprintf("%d_%d", ring, azBin)
		cellObs[key] = append(cellObs[key], p.Distance)
	}

	// Now sample all cells in range, including observation data
	for ring := gp.ringMin; ring <= gp.ringMax && ring < g.Rings; ring++ {
		for azBin := 0; azBin < g.AzimuthBins; azBin++ {
			azDeg := float64(azBin) * azBinRes

			if azDeg < gp.azMin || azDeg > gp.azMax {
				continue
			}

			idx := ring*g.AzimuthBins + azBin
			if idx >= len(g.Cells) {
				continue
			}

			cell := g.Cells[idx]
			key := fmt.Sprintf("%d_%d", ring, azBin)

			// Compute average observed distance for this cell in this frame
			var obsDist float64
			if obs, hasObs := cellObs[key]; hasObs && len(obs) > 0 {
				sum := 0.0
				for _, d := range obs {
					sum += d
				}
				obsDist = sum / float64(len(obs))
			}

			sample := GridSample{
				FrameIdx:       gp.frameIdx,
				Timestamp:      now,
				BgAverage:      float64(cell.AverageRangeMeters),
				BgSpread:       float64(cell.RangeSpreadMeters),
				BgSeen:         int(cell.TimesSeenCount),
				LockedBaseline: float64(cell.LockedBaseline),
				LockedSpread:   float64(cell.LockedSpread),
				LockedAtCount:  int(cell.LockedAtCount),
				ObsDist:        obsDist,
				RecFg:          int(cell.RecentForegroundCount),
				Frozen:         cell.FrozenUntilUnixNanos > now.UnixNano(),
			}

			gp.samples[key] = append(gp.samples[key], sample)
		}
	}
}

// GeneratePlots creates SVG files for each ring, showing BG and FG values over time.
// Returns the number of plots generated and any error.
func (gp *GridPlotter) GeneratePlots() (int, error) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if gp.outputDir == "" {
		return 0, fmt.Errorf("no output directory configured")
	}

	if len(gp.samples) == 0 {
		return 0, nil
	}

	// Group samples by ring
	byRing := make(map[int]map[int][]GridSample) // ring -> azBin -> samples
	for key, samples := range gp.samples {
		var ring, azBin int
		if _, err := fmt.Sscanf(key, "%d_%d", &ring, &azBin); err != nil {
			// Skip malformed keys
			continue
		}

		if byRing[ring] == nil {
			byRing[ring] = make(map[int][]GridSample)
		}
		byRing[ring][azBin] = samples
	}

	plotCount := 0
	for ring, azBins := range byRing {
		if err := gp.generateRingPlot(ring, azBins); err != nil {
			return plotCount, fmt.Errorf("ring %d: %w", ring, err)
		}
		plotCount++
	}

	return plotCount, nil
}

// generateRingPlot creates plots for a ring: BG average, locked baseline, observation distance, and RecFg count.
func (gp *GridPlotter) generateRingPlot(ring int, azBins map[int][]GridSample) error {
	if len(azBins) == 0 {
		return nil
	}

	// Get grid resolution for azimuth labels
	mgr := l3grid.GetBackgroundManager(gp.sensorID)
	azBinRes := 0.2 // default
	if mgr != nil && mgr.Grid != nil {
		azBinRes = 360.0 / float64(mgr.Grid.AzimuthBins)
	}

	// Sort azimuth bins for consistent legend
	var sortedAzBins []int
	for azBin := range azBins {
		sortedAzBins = append(sortedAzBins, azBin)
	}
	sort.Ints(sortedAzBins)

	// Color palette
	colors := generateColors(len(sortedAzBins))

	var bgSeries []gridSeries
	var lockedSeries []gridSeries
	var obsSeries []gridSeries
	var fgSeries []gridSeries

	for i, azBin := range sortedAzBins {
		samples := azBins[azBin]
		if len(samples) == 0 {
			continue
		}

		// Sort by frame index
		sort.Slice(samples, func(a, b int) bool {
			return samples[a].FrameIdx < samples[b].FrameIdx
		})

		bgPts := make([]gridPoint, 0, len(samples))
		lockedPts := make([]gridPoint, 0, len(samples))
		obsPts := make([]gridPoint, 0, len(samples))
		fgPts := make([]gridPoint, 0, len(samples))
		for _, s := range samples {
			// Skip initial zeros for BG average (uninitialized cells)
			if s.BgAverage > 0 {
				bgPts = append(bgPts, gridPoint{X: float64(s.FrameIdx), Y: s.BgAverage})
			}
			// Only include locked baseline when established (non-zero)
			if s.LockedBaseline > 0 {
				lockedPts = append(lockedPts, gridPoint{X: float64(s.FrameIdx), Y: s.LockedBaseline})
			}
			// Only include observation distance when non-zero (point was observed)
			if s.ObsDist > 0 {
				obsPts = append(obsPts, gridPoint{X: float64(s.FrameIdx), Y: s.ObsDist})
			}
			// Always include RecFg count
			fgPts = append(fgPts, gridPoint{X: float64(s.FrameIdx), Y: float64(s.RecFg)})
		}

		azLabel := fmt.Sprintf("%.1f deg", float64(azBin)*azBinRes)

		if len(bgPts) > 0 {
			bgSeries = append(bgSeries, gridSeries{Label: azLabel, Color: colors[i], Points: bgPts})
		}
		if len(lockedPts) > 0 {
			lockedSeries = append(lockedSeries, gridSeries{Label: azLabel, Color: colors[i], Points: lockedPts})
		}
		if len(obsPts) > 0 {
			obsSeries = append(obsSeries, gridSeries{Label: azLabel, Color: colors[i], Points: obsPts})
		}
		if len(fgPts) > 0 {
			fgSeries = append(fgSeries, gridSeries{Label: azLabel, Color: colors[i], Points: fgPts})
		}
	}

	bgFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_bg_avg.svg", ring))
	if err := writeGridSVG(bgFile, fmt.Sprintf("Ring %d - Background Average (EMA)", ring), "Distance (m)", bgSeries); err != nil {
		return fmt.Errorf("save bg plot: %w", err)
	}

	lockedFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_locked.svg", ring))
	if err := writeGridSVG(lockedFile, fmt.Sprintf("Ring %d - Locked Baseline", ring), "Distance (m)", lockedSeries); err != nil {
		return fmt.Errorf("save locked baseline plot: %w", err)
	}

	obsFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_obs_dist.svg", ring))
	if err := writeGridSVG(obsFile, fmt.Sprintf("Ring %d - Observation Distance", ring), "Distance (m)", obsSeries); err != nil {
		return fmt.Errorf("save obs plot: %w", err)
	}

	fgFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_recfg.svg", ring))
	if err := writeGridSVG(fgFile, fmt.Sprintf("Ring %d - Recent Foreground Count", ring), "RecFg Count", fgSeries); err != nil {
		return fmt.Errorf("save fg plot: %w", err)
	}

	return nil
}

type gridPoint struct {
	X float64
	Y float64
}

type gridSeries struct {
	Label  string
	Color  color.Color
	Points []gridPoint
}

func writeGridSVG(path, title, yLabel string, series []gridSeries) error {
	const (
		width  = 1400.0
		height = 600.0
		left   = 72.0
		right  = 220.0
		top    = 56.0
		bottom = 64.0
	)

	xMin, xMax, yMin, yMax := gridBounds(series)
	if xMax <= xMin {
		xMax = xMin + 1
	}
	if yMax <= yMin {
		yMax = yMin + 1
	}
	yPad := (yMax - yMin) * 0.08
	if yPad == 0 {
		yPad = 1
	}
	yMin -= yPad
	yMax += yPad

	plotW := width - left - right
	plotH := height - top - bottom
	xScale := func(x float64) float64 {
		return left + ((x - xMin) / (xMax - xMin) * plotW)
	}
	yScale := func(y float64) float64 {
		return top + plotH - ((y - yMin) / (yMax - yMin) * plotH)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, width, height, width, height)
	b.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)
	fmt.Fprintf(&b, `<text x="%.0f" y="28" font-family="sans-serif" font-size="20" font-weight="700">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#555">Frame</text>`, left+plotW/2-20, height-18)
	fmt.Fprintf(&b, `<text x="18" y="%.0f" font-family="sans-serif" font-size="12" fill="#555" transform="rotate(-90 18 %.0f)">%s</text>`, top+plotH/2+40, top+plotH/2+40, html.EscapeString(yLabel))
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="#fbfbfb" stroke="#d9d9d9"/>`, left, top, plotW, plotH)

	for i := 0; i <= 5; i++ {
		x := left + float64(i)*plotW/5
		y := top + float64(i)*plotH/5
		fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="#eeeeee"/>`, x, top, x, top+plotH)
		fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="#eeeeee"/>`, left, y, left+plotW, y)
		xVal := xMin + float64(i)*(xMax-xMin)/5
		yVal := yMax - float64(i)*(yMax-yMin)/5
		fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" text-anchor="middle" font-family="sans-serif" font-size="11" fill="#555">%.0f</text>`, x, top+plotH+18, xVal)
		fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" text-anchor="end" font-family="sans-serif" font-size="11" fill="#555">%.2f</text>`, left-8, y+4, yVal)
	}

	for _, s := range series {
		if len(s.Points) == 0 {
			continue
		}
		colour := svgColor(s.Color)
		b.WriteString(`<polyline fill="none" stroke="`)
		b.WriteString(colour)
		b.WriteString(`" stroke-width="1.5" points="`)
		for _, p := range s.Points {
			fmt.Fprintf(&b, "%.2f,%.2f ", xScale(p.X), yScale(p.Y))
		}
		b.WriteString(`"/>`)
	}

	legendX := left + plotW + 24
	legendY := top + 18
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="13" font-weight="700">Azimuth</text>`, legendX, legendY-16)
	for i, s := range series {
		if i >= 24 {
			fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="11" fill="#555">+%d more</text>`, legendX, legendY+float64(i)*18, len(series)-i)
			break
		}
		y := legendY + float64(i)*18
		colour := svgColor(s.Color)
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="%s" stroke-width="2"/>`, legendX, y, legendX+18, y, colour)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="11" fill="#333">%s</text>`, legendX+24, y+4, html.EscapeString(s.Label))
	}

	b.WriteString(`</svg>`)
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func gridBounds(series []gridSeries) (xMin, xMax, yMin, yMax float64) {
	xMin = math.Inf(1)
	yMin = math.Inf(1)
	xMax = math.Inf(-1)
	yMax = math.Inf(-1)
	for _, s := range series {
		for _, p := range s.Points {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) {
				continue
			}
			xMin = math.Min(xMin, p.X)
			xMax = math.Max(xMax, p.X)
			yMin = math.Min(yMin, p.Y)
			yMax = math.Max(yMax, p.Y)
		}
	}
	if math.IsInf(xMin, 0) {
		return 0, 1, 0, 1
	}
	return xMin, xMax, yMin, yMax
}

func svgColor(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// generateColors creates a palette of distinct colors for azimuth lines
func generateColors(n int) []color.Color {
	if n <= 0 {
		return nil
	}

	colors := make([]color.Color, n)
	for i := 0; i < n; i++ {
		hue := float64(i) / float64(n)
		r, g, b := hslToRGB(hue, 0.7, 0.5)
		colors[i] = color.RGBA{R: r, G: g, B: b, A: 255}
	}
	return colors
}

// hslToRGB converts HSL to RGB (0-255 range)
func hslToRGB(h, s, l float64) (r, g, b uint8) {
	var rf, gf, bf float64

	if s == 0 {
		rf, gf, bf = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q
		rf = hueToRGB(p, q, h+1.0/3.0)
		gf = hueToRGB(p, q, h)
		bf = hueToRGB(p, q, h-1.0/3.0)
	}

	return uint8(rf * 255), uint8(gf * 255), uint8(bf * 255)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

// GetOutputDir returns the current output directory for plots.
func (gp *GridPlotter) GetOutputDir() string {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	return gp.outputDir
}

// GetSampleCount returns the total number of samples collected.
func (gp *GridPlotter) GetSampleCount() int {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	count := 0
	for _, samples := range gp.samples {
		count += len(samples)
	}
	return count
}

// IncrementFrame should be called once per frame to track frame boundaries.
func (gp *GridPlotter) IncrementFrame() {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	if gp.enabled {
		gp.frameIdx++
	}
}

// FormatTimestamp generates a timestamp string for directory naming.
func FormatTimestamp(t time.Time) string {
	return t.Format("20060102_150405")
}

// MakePlotOutputDir creates a timestamped output directory for plots.
// For PCAP files: plots/<pcap_basename>/<timestamp>
// For live data: plots/live_<timestamp>
func MakePlotOutputDir(baseDir, pcapFile string) string {
	ts := FormatTimestamp(time.Now())
	if pcapFile != "" {
		// Use PCAP basename without extension
		base := filepath.Base(pcapFile)
		ext := filepath.Ext(base)
		name := base[:len(base)-len(ext)]
		return filepath.Join(baseDir, name, ts)
	}
	return filepath.Join(baseDir, "live_"+ts)
}
