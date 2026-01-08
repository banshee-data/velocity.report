package monitor

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
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
func (gp *GridPlotter) Sample(mgr *lidar.BackgroundManager) {
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
func (gp *GridPlotter) SampleWithObservation(mgr *lidar.BackgroundManager, ring int, azDeg, obsDist float64, isBg bool) {
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
func (gp *GridPlotter) SampleWithPoints(mgr *lidar.BackgroundManager, points []lidar.PointPolar) {
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

// GeneratePlots creates PNG files for each ring, showing BG and FG values over time.
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
		fmt.Sscanf(key, "%d_%d", &ring, &azBin)

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

	// Create plot for background average
	pBg := plot.New()
	pBg.Title.Text = fmt.Sprintf("Ring %d - Background Average (EMA)", ring)
	pBg.X.Label.Text = "Frame"
	pBg.Y.Label.Text = "Distance (m)"

	// Create plot for locked baseline
	pLocked := plot.New()
	pLocked.Title.Text = fmt.Sprintf("Ring %d - Locked Baseline", ring)
	pLocked.X.Label.Text = "Frame"
	pLocked.Y.Label.Text = "Distance (m)"

	// Create plot for observation distance (foreground points)
	pObs := plot.New()
	pObs.Title.Text = fmt.Sprintf("Ring %d - Observation Distance", ring)
	pObs.X.Label.Text = "Frame"
	pObs.Y.Label.Text = "Distance (m)"

	// Create plot for recent foreground count
	pFg := plot.New()
	pFg.Title.Text = fmt.Sprintf("Ring %d - Recent Foreground Count", ring)
	pFg.X.Label.Text = "Frame"
	pFg.Y.Label.Text = "RecFg Count"

	// Get grid resolution for azimuth labels
	mgr := lidar.GetBackgroundManager(gp.sensorID)
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

	for i, azBin := range sortedAzBins {
		samples := azBins[azBin]
		if len(samples) == 0 {
			continue
		}

		// Sort by frame index
		sort.Slice(samples, func(a, b int) bool {
			return samples[a].FrameIdx < samples[b].FrameIdx
		})

		// Create XY data, skipping initial zero values for BG to show more range
		bgPts := make(plotter.XYs, 0, len(samples))
		lockedPts := make(plotter.XYs, 0, len(samples))
		obsPts := make(plotter.XYs, 0, len(samples))
		fgPts := make(plotter.XYs, 0, len(samples))
		for _, s := range samples {
			// Skip initial zeros for BG average (uninitialized cells)
			if s.BgAverage > 0 {
				bgPts = append(bgPts, plotter.XY{X: float64(s.FrameIdx), Y: s.BgAverage})
			}
			// Only include locked baseline when established (non-zero)
			if s.LockedBaseline > 0 {
				lockedPts = append(lockedPts, plotter.XY{X: float64(s.FrameIdx), Y: s.LockedBaseline})
			}
			// Only include observation distance when non-zero (point was observed)
			if s.ObsDist > 0 {
				obsPts = append(obsPts, plotter.XY{X: float64(s.FrameIdx), Y: s.ObsDist})
			}
			// Always include RecFg count
			fgPts = append(fgPts, plotter.XY{X: float64(s.FrameIdx), Y: float64(s.RecFg)})
		}

		azLabel := fmt.Sprintf("%.1f°", float64(azBin)*azBinRes)

		// Add BG line if we have data
		if len(bgPts) > 0 {
			bgLine, err := plotter.NewLine(bgPts)
			if err != nil {
				return err
			}
			bgLine.Color = colors[i]
			bgLine.Width = vg.Points(1)
			pBg.Add(bgLine)
			pBg.Legend.Add(azLabel, bgLine)
		}

		// Add locked baseline line if we have data
		if len(lockedPts) > 0 {
			lockedLine, err := plotter.NewLine(lockedPts)
			if err != nil {
				return err
			}
			lockedLine.Color = colors[i]
			lockedLine.Width = vg.Points(1)
			pLocked.Add(lockedLine)
			pLocked.Legend.Add(azLabel, lockedLine)
		}

		// Add observation distance line if we have data
		if len(obsPts) > 0 {
			obsLine, err := plotter.NewLine(obsPts)
			if err != nil {
				return err
			}
			obsLine.Color = colors[i]
			obsLine.Width = vg.Points(1)
			pObs.Add(obsLine)
			pObs.Legend.Add(azLabel, obsLine)
		}

		// Add FG line
		if len(fgPts) > 0 {
			fgLine, err := plotter.NewLine(fgPts)
			if err != nil {
				return err
			}
			fgLine.Color = colors[i]
			fgLine.Width = vg.Points(1)
			pFg.Add(fgLine)
			pFg.Legend.Add(azLabel, fgLine)
		}
	}

	// Configure legends
	pBg.Legend.Top = true
	pBg.Legend.Left = false
	pBg.Legend.XOffs = -10
	pBg.Legend.YOffs = -10

	pLocked.Legend.Top = true
	pLocked.Legend.Left = false
	pLocked.Legend.XOffs = -10
	pLocked.Legend.YOffs = -10

	pObs.Legend.Top = true
	pObs.Legend.Left = false
	pObs.Legend.XOffs = -10
	pObs.Legend.YOffs = -10

	pFg.Legend.Top = true
	pFg.Legend.Left = false
	pFg.Legend.XOffs = -10
	pFg.Legend.YOffs = -10

	// Save plots
	bgFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_bg_avg.png", ring))
	if err := pBg.Save(14*vg.Inch, 6*vg.Inch, bgFile); err != nil {
		return fmt.Errorf("save bg plot: %w", err)
	}

	lockedFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_locked.png", ring))
	if err := pLocked.Save(14*vg.Inch, 6*vg.Inch, lockedFile); err != nil {
		return fmt.Errorf("save locked baseline plot: %w", err)
	}

	obsFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_obs_dist.png", ring))
	if err := pObs.Save(14*vg.Inch, 6*vg.Inch, obsFile); err != nil {
		return fmt.Errorf("save obs plot: %w", err)
	}

	fgFile := filepath.Join(gp.outputDir, fmt.Sprintf("ring_%02d_recfg.png", ring))
	if err := pFg.Save(14*vg.Inch, 6*vg.Inch, fgFile); err != nil {
		return fmt.Errorf("save fg plot: %w", err)
	}

	return nil
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

// Convenience function to suppress unused import warnings
var _ = math.Abs
