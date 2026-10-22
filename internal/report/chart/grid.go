package chart

// The background grid view: one coarse bucket per ring and azimuth span,
// coloured by a single settling metric. It is the operator's check on how
// much of the L3 background has filled and settled, served at
// /api/charts/grid beside the report charts (D-11, D-17) and drawn on the
// web's LiDAR debug page. It replaces the Python grid-heatmap tool's polar
// view.
//
// The chart computes nothing about the grid. The caller aggregates the
// buckets and chooses the metric; what the chart owns:
//
//   - Axes. Azimuth 0–360° across, ring index up from ring 0 at the bottom,
//     with ticks every 30° and every 5 rings.
//   - Scale. A ratio metric is drawn on a fixed 0–1 scale, so two snapshots
//     compare by colour; a count metric on 0 to its observed maximum, which
//     the colour bar states.
//   - Gaps. A bucket the metric is undefined for (a ratio over no filled
//     cells) is left blank, not drawn as zero.

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// gridRamp is a ten-stop viridis ramp. It is neutral and separate from the
// §3.3 percentile palette: the chart shows background state, not speeds.
var gridRamp = []string{
	"#440154", "#482777", "#3e4989", "#31688e", "#26828e",
	"#1f9e89", "#35b779", "#6ece58", "#b5de2b", "#fde725",
}

// colourGridEmpty fills a bucket the metric is undefined for.
const colourGridEmpty = "#e5e7eb"

// ErrGridRingsRequired is returned when the data names no rings.
var ErrGridRingsRequired = errors.New("chart: grid heatmap needs at least one ring")

// GridHeatmapCell is one coarse bucket. Value is nil where the metric is
// undefined for the bucket.
type GridHeatmapCell struct {
	Ring            int
	AzimuthStartDeg float64
	AzimuthEndDeg   float64
	Value           *float64
}

// GridHeatmapData is everything the grid chart draws.
type GridHeatmapData struct {
	// Title and Subtitle head the chart: the metric and the sensor and time.
	Title    string
	Subtitle string
	// MetricLabel names the colour bar.
	MetricLabel string
	// Ratio fixes the colour scale to 0–1; otherwise it runs to the largest
	// value present.
	Ratio bool
	Rings int
	Cells []GridHeatmapCell
	// Summary lines are drawn under the plot, one per line.
	Summary []string
}

// DefaultGridHeatmapStyle returns grid chart parameters sized for a wide web
// panel: a 360° azimuth axis needs width more than height.
func DefaultGridHeatmapStyle() ChartStyle {
	s := baseStyle()
	s.WidthMM = 240
	s.HeightMM = 120
	s.AxisLabelFontPx = 11.0
	s.AxisTickFontPx = 9.5
	s.LegendFontPx = 10.0
	return s
}

// RenderGridHeatmap draws the ring × azimuth grid as an SVG document.
func RenderGridHeatmap(data GridHeatmapData, style ChartStyle) ([]byte, error) {
	if data.Rings <= 0 {
		return nil, ErrGridRingsRequired
	}
	wPx, hPx := style.WidthMM*pxPerMM, style.HeightMM*pxPerMM
	summaryH := float64(len(data.Summary)) * (style.AxisTickFontPx + 4)
	left, right := 48.0, 92.0
	top, bottom := 44.0, 40.0+summaryH
	plotW, plotH := wPx-left-right, hPx-top-bottom

	maxValue := 1.0
	if !data.Ratio {
		maxValue = 0
		for _, cell := range data.Cells {
			if cell.Value != nil && *cell.Value > maxValue {
				maxValue = *cell.Value
			}
		}
		if maxValue == 0 {
			maxValue = 1
		}
	}

	c := NewCanvas(style.WidthMM, style.HeightMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)
	c.Text(left, 18, data.Title, fmt.Sprintf(`font-size="%.1f" font-weight="bold"`, style.AxisLabelFontPx+2))
	c.Text(left, 34, data.Subtitle, fmt.Sprintf(`font-size="%.1f" fill="gray"`, style.AxisTickFontPx))

	ringH := plotH / float64(data.Rings)
	c.Rect(left, top, plotW, plotH, `fill="`+colourGridEmpty+`"`)
	for _, cell := range data.Cells {
		if cell.Value == nil || cell.Ring < 0 || cell.Ring >= data.Rings {
			continue
		}
		x0 := left + plotW*clampUnit(cell.AzimuthStartDeg/360)
		x1 := left + plotW*clampUnit(cell.AzimuthEndDeg/360)
		y := top + plotH - float64(cell.Ring+1)*ringH
		c.Rect(x0, y, x1-x0, ringH, fmt.Sprintf(`fill="%s" class="cell"`, gridColour(*cell.Value/maxValue)))
	}

	tickAttrs := fmt.Sprintf(`font-size="%.1f" fill="gray"`, style.AxisTickFontPx)
	gridAttrs := `stroke="white" stroke-opacity="0.35" stroke-width="0.5"`
	for deg := 0; deg <= 360; deg += 30 {
		x := left + plotW*float64(deg)/360
		c.Line(x, top, x, top+plotH, gridAttrs)
		c.Text(x, top+plotH+14, strconv.Itoa(deg), tickAttrs+` text-anchor="middle"`)
	}
	for ring := 0; ring <= data.Rings; ring += 5 {
		y := top + plotH - float64(ring)*ringH
		c.Line(left, y, left+plotW, y, gridAttrs)
		c.Text(left-6, y+3, strconv.Itoa(ring), tickAttrs+` text-anchor="end"`)
	}
	labelAttrs := fmt.Sprintf(`font-size="%.1f"`, style.AxisLabelFontPx)
	c.Text(left+plotW/2, top+plotH+30, "Azimuth (degrees)", labelAttrs+` text-anchor="middle"`)
	c.Text(14, top+plotH/2, "Ring", fmt.Sprintf(`%s text-anchor="middle" transform="rotate(-90 14 %.4f)"`, labelAttrs, top+plotH/2))

	drawGridColourBar(c, left+plotW+16, top, plotH, maxValue, data, style)

	for i, line := range data.Summary {
		c.Text(left, top+plotH+48+float64(i)*(style.AxisTickFontPx+4), line, tickAttrs)
	}
	c.EndGroup()
	return c.Bytes(), nil
}

// RenderGridMessage draws a labelled message chart, for a sensor with no
// background grid to show.
func RenderGridMessage(style ChartStyle, lines ...string) []byte {
	wPx, hPx := style.WidthMM*pxPerMM, style.HeightMM*pxPerMM
	c := NewCanvas(style.WidthMM, style.HeightMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)
	for i, line := range lines {
		c.Text(wPx/2, hPx/2+float64(i)*(style.AxisLabelFontPx+4), line,
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	}
	c.EndGroup()
	return c.Bytes()
}

// drawGridColourBar draws the vertical colour key with its end values.
func drawGridColourBar(c *SVGCanvas, x, top, h, maxValue float64, data GridHeatmapData, style ChartStyle) {
	const barW = 12.0
	stepH := h / float64(len(gridRamp))
	for i, colour := range gridRamp {
		c.Rect(x, top+h-float64(i+1)*stepH, barW, stepH, `fill="`+colour+`"`)
	}
	tickAttrs := fmt.Sprintf(`font-size="%.1f" fill="gray"`, style.AxisTickFontPx)
	c.Text(x+barW+4, top+8, formatGridValue(maxValue, data.Ratio), tickAttrs)
	c.Text(x+barW+4, top+h, formatGridValue(0, data.Ratio), tickAttrs)
	c.Text(x, top+h+14, "no data", tickAttrs)
	c.Rect(x+38, top+h+5, 10, 10, `fill="`+colourGridEmpty+`"`)
	c.Text(x+barW+20, top+h/2, data.MetricLabel, fmt.Sprintf(`font-size="%.1f" text-anchor="middle" transform="rotate(90 %.4f %.4f)"`,
		style.AxisTickFontPx, x+barW+20, top+h/2))
}

// gridColour picks the ramp stop for a value already scaled to 0–1.
func gridColour(unit float64) string {
	i := int(math.Floor(clampUnit(unit) * float64(len(gridRamp))))
	if i >= len(gridRamp) {
		i = len(gridRamp) - 1
	}
	return gridRamp[i]
}

func clampUnit(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func formatGridValue(v float64, ratio bool) string {
	if ratio {
		return strconv.FormatFloat(v*100, 'f', 0, 64) + "%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
