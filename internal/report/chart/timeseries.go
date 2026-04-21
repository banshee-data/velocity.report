package chart

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// TimeSeriesPoint holds speed percentiles and count for one time period.
type TimeSeriesPoint struct {
	StartTime time.Time
	P50Speed  float64 // NaN = missing
	P85Speed  float64
	P98Speed  float64
	MaxSpeed  float64
	Count     int
}

// TimeSeriesData is the input for a time-series chart.
type TimeSeriesData struct {
	Points []TimeSeriesPoint
	Units  string
	Title  string
}

// XTick represents a labelled tick on the X axis.
type XTick struct {
	Index int
	Label string
}

// DayBoundaries returns indices where the calendar date changes.
// Index 0 is always included. Uses Year/Month/Day comparison rather
// than Truncate, so it respects the timezone embedded in each point.
func DayBoundaries(pts []TimeSeriesPoint) []int {
	if len(pts) == 0 {
		return nil
	}
	boundaries := []int{0}
	py, pm, pd := pts[0].StartTime.Date()
	for i := 1; i < len(pts); i++ {
		y, m, d := pts[i].StartTime.Date()
		if y != py || m != pm || d != pd {
			boundaries = append(boundaries, i)
			py, pm, pd = y, m, d
		}
	}
	return boundaries
}

// ApplyCountMask returns a copy of pts where all speed fields are NaN
// when Count < threshold. The original slice is not modified.
func ApplyCountMask(pts []TimeSeriesPoint, threshold int) []TimeSeriesPoint {
	out := make([]TimeSeriesPoint, len(pts))
	copy(out, pts)
	for i := range out {
		if out[i].Count < threshold {
			out[i].P50Speed = math.NaN()
			out[i].P85Speed = math.NaN()
			out[i].P98Speed = math.NaN()
			out[i].MaxSpeed = math.NaN()
		}
	}
	return out
}

// XTicks generates duration-aware tick labels for the time-series X axis.
// Tick cadence is chosen from the total span so the chart shows 6-10 labels
// regardless of range length (per DESIGN.md §4.1).
func XTicks(pts []TimeSeriesPoint, boundaries []int) []XTick {
	if len(pts) < 2 {
		if len(pts) == 1 {
			return []XTick{{Index: 0, Label: pts[0].StartTime.Format("Jan 02\n15:04")}}
		}
		return nil
	}

	first := pts[0].StartTime
	last := pts[len(pts)-1].StartTime
	span := last.Sub(first)

	// Choose a cadence that targets ~8 labels across the span.
	cadence := pickTickCadence(span)

	// Walk points and emit a tick whenever the rounded cadence boundary
	// advances. This is robust to irregular sampling and day-of-month jumps.
	var ticks []XTick
	prevBucket := int64(-1)
	for i, pt := range pts {
		bucket := cadence.bucket(pt.StartTime)
		if bucket == prevBucket {
			continue
		}
		prevBucket = bucket
		ticks = append(ticks, XTick{Index: i, Label: cadence.format(pt.StartTime)})
	}
	return ticks
}

// tickCadence describes an X-axis tick stride.
type tickCadence struct {
	// bucket returns a strictly-increasing integer key that changes when a
	// new tick should be emitted.
	bucket func(t time.Time) int64
	// format produces the label shown at the tick.
	format func(t time.Time) string
}

func pickTickCadence(span time.Duration) tickCadence {
	switch {
	case span <= 12*time.Hour:
		return tickCadence{
			bucket: func(t time.Time) int64 { return t.Unix() / (60 * 60) },
			format: func(t time.Time) string { return t.Format("15:04") },
		}
	case span <= 36*time.Hour:
		return tickCadence{
			bucket: func(t time.Time) int64 { return t.Unix() / (3 * 60 * 60) },
			format: func(t time.Time) string { return t.Format("Jan 02\n15:04") },
		}
	case span <= 4*24*time.Hour:
		return tickCadence{
			bucket: func(t time.Time) int64 { return t.Unix() / (6 * 60 * 60) },
			format: func(t time.Time) string { return t.Format("Jan 02\n15:04") },
		}
	case span <= 14*24*time.Hour:
		// Daily.
		return tickCadence{
			bucket: func(t time.Time) int64 {
				y, m, d := t.Date()
				return int64(y*10000 + int(m)*100 + d)
			},
			format: func(t time.Time) string { return t.Format("Jan 02") },
		}
	case span <= 90*24*time.Hour:
		// Weekly (ISO week).
		return tickCadence{
			bucket: func(t time.Time) int64 {
				y, w := t.ISOWeek()
				return int64(y*100 + w)
			},
			format: func(t time.Time) string { return t.Format("Jan 02") },
		}
	default:
		// Monthly.
		return tickCadence{
			bucket: func(t time.Time) int64 {
				y, m, _ := t.Date()
				return int64(y*100 + int(m))
			},
			format: func(t time.Time) string { return t.Format("Jan 2006") },
		}
	}
}

// RenderTimeSeries produces a time-series SVG chart with speed percentile
// lines, count bars, low-sample highlights, and day boundary markers.
func RenderTimeSeries(data TimeSeriesData, style ChartStyle) ([]byte, error) {
	if len(data.Points) == 0 {
		return renderTimeSeriesNoData(style), nil
	}

	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM

	c := NewCanvas(style.WidthMM, style.HeightMM)

	// Embed font.
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())

	// Layout. Leave ~7% on each side for y-axis tick labels and a tall
	// enough bottom strip for two-line date/time tick labels plus the legend.
	tickLabelBlock := 2.6 * style.AxisTickFontPx // two-line labels
	legendBlock := style.LegendFontPx + 6
	bottomMargin := tickLabelBlock + legendBlock + 4

	leftPx := 0.07 * wPx
	rightPx := 0.93 * wPx
	topPx := 0.04 * hPx
	if data.Title != "" {
		topPx = style.AxisLabelFontPx*1.6 + 4
	}
	bottomPx := hPx - bottomMargin

	plotW := rightPx - leftPx
	plotH := bottomPx - topPx
	n := len(data.Points)

	boundaries := DayBoundaries(data.Points)

	// Compute speed and count ranges.
	var maxSpeed float64
	var maxCount int
	for _, pt := range data.Points {
		for _, s := range []float64{pt.P50Speed, pt.P85Speed, pt.P98Speed, pt.MaxSpeed} {
			if !math.IsNaN(s) && s > maxSpeed {
				maxSpeed = s
			}
		}
		if pt.Count > maxCount {
			maxCount = pt.Count
		}
	}
	if maxSpeed == 0 {
		maxSpeed = 1
	}
	if maxCount == 0 {
		maxCount = 1
	}

	// Y-scale functions.
	speedScale := plotH / (maxSpeed * 1.1)
	countScale := plotH / (float64(maxCount) * style.CountAxisScale)

	xOf := func(i int) float64 {
		if n == 1 {
			return leftPx + plotW/2
		}
		return leftPx + (float64(i)+0.5)/float64(n)*plotW
	}
	speedYOf := func(v float64) float64 {
		return bottomPx - v*speedScale
	}
	countYOf := func(cnt int) float64 {
		return bottomPx - float64(cnt)*countScale
	}

	barSlotW := plotW / float64(n)

	// Count bars.
	c.BeginGroup(`class="count-bars"`)
	for i, pt := range data.Points {
		if pt.Count == 0 {
			continue
		}
		barW := barSlotW * style.BarWidthFraction
		x := xOf(i) - barW/2
		y := countYOf(pt.Count)
		h := bottomPx - y
		c.Rect(x, y, barW, h,
			fmt.Sprintf(`fill="%s" fill-opacity="0.25"`, style.ColourCountBar))
	}
	c.EndGroup()

	// Low-sample background bars.
	c.BeginGroup(`class="low-sample"`)
	for i, pt := range data.Points {
		if pt.Count < style.LowSampleThreshold && pt.Count >= style.CountMissingThreshold {
			bgW := barSlotW * style.BarWidthBGFraction
			x := xOf(i) - bgW/2
			c.Rect(x, topPx, bgW, plotH,
				fmt.Sprintf(`fill="%s" fill-opacity="0.25"`, style.ColourLowSample))
		}
	}
	c.EndGroup()

	// Day boundary vertical markers (visual guide only, never break lines).
	c.BeginGroup(`class="day-boundaries"`)
	for _, b := range boundaries {
		if b == 0 {
			continue
		}
		x := leftPx + float64(b)/float64(n)*plotW
		c.Line(x, topPx, x, bottomPx,
			`stroke="gray" stroke-dasharray="4 2" stroke-width="0.5" opacity="0.3"`)
	}
	c.EndGroup()

	// Percentile lines — one continuous polyline per series. NaN values
	// (missing data) break the line; day boundaries do not.
	type seriesDef struct {
		colour string
		marker string
		field  func(TimeSeriesPoint) float64
		label  string
		dashed bool
	}
	series := []seriesDef{
		{style.ColourP50, "triangle", func(p TimeSeriesPoint) float64 { return p.P50Speed }, "p50", false},
		{style.ColourP85, "square", func(p TimeSeriesPoint) float64 { return p.P85Speed }, "p85", false},
		{style.ColourP98, "circle", func(p TimeSeriesPoint) float64 { return p.P98Speed }, "p98", false},
		{style.ColourMax, "x", func(p TimeSeriesPoint) float64 { return p.MaxSpeed }, "max", true},
	}

	for _, s := range series {
		lineAttrs := fmt.Sprintf(
			`fill="none" stroke="%s" stroke-width="%.1f"`,
			s.colour, style.LineWidthPx)
		if s.dashed {
			lineAttrs += ` stroke-dasharray="6 3"`
		}

		c.BeginGroup(fmt.Sprintf(`class="series-%s"`, s.label))

		var pts [][2]float64
		for i := 0; i < n; i++ {
			val := s.field(data.Points[i])
			if math.IsNaN(val) {
				if len(pts) > 1 {
					c.Polyline(pts, lineAttrs)
				}
				pts = pts[:0]
				continue
			}
			pts = append(pts, [2]float64{xOf(i), speedYOf(val)})
		}
		if len(pts) > 1 {
			c.Polyline(pts, lineAttrs)
		}

		// Markers.
		for i, pt := range data.Points {
			val := s.field(pt)
			if math.IsNaN(val) {
				continue
			}
			px := xOf(i)
			py := speedYOf(val)
			r := style.MarkerRadiusPx

			switch s.marker {
			case "triangle":
				triPts := fmt.Sprintf("%.4f,%.4f %.4f,%.4f %.4f,%.4f",
					px, py-r, px-r*0.866, py+r*0.5, px+r*0.866, py+r*0.5)
				fmt.Fprintf(&c.buf,
					`<polygon points="%s" fill="%s" stroke="%s" stroke-width="0.5"/>`+"\n",
					triPts, s.colour, s.colour)
			case "square":
				c.Rect(px-r/2, py-r/2, r, r,
					fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="0.5"`, s.colour, s.colour))
			case "circle":
				c.Circle(px, py, r/2,
					fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="0.5"`, s.colour, s.colour))
			case "x":
				half := r * 0.6
				xAttrs := fmt.Sprintf(`stroke="%s" stroke-width="1.5"`, s.colour)
				c.Line(px-half, py-half, px+half, py+half, xAttrs)
				c.Line(px-half, py+half, px+half, py-half, xAttrs)
			}
		}

		c.EndGroup()
	}

	// X-axis ticks.
	ticks := XTicks(data.Points, boundaries)
	c.BeginGroup(`class="x-axis"`)
	for _, t := range ticks {
		x := xOf(t.Index)
		parts := strings.Split(t.Label, "\n")
		for j, part := range parts {
			y := bottomPx + style.AxisTickFontPx*(float64(j)+1) + 3
			c.Text(x, y, part,
				fmt.Sprintf(
					`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle"`,
					style.AxisTickFontPx))
		}
		// Small tick mark on the plot border.
		c.Line(x, bottomPx, x, bottomPx+3, `stroke="black" stroke-width="0.5"`)
	}
	c.EndGroup()

	// Speed Y-axis label + ticks.
	c.BeginGroup(`class="y-axis"`)
	nSpeedTicks := 5
	for i := 0; i <= nSpeedTicks; i++ {
		val := maxSpeed * 1.1 * float64(i) / float64(nSpeedTicks)
		y := speedYOf(val)
		c.Line(leftPx-4, y, leftPx, y, `stroke="black" stroke-width="0.5"`)
		c.Text(leftPx-6, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", val),
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="end"`, style.AxisTickFontPx))
	}
	// Rotated "Speed (units)" label along the left edge.
	labelX := leftPx - style.AxisTickFontPx*3.2
	labelY := (topPx + bottomPx) / 2
	c.Text(labelX, labelY,
		fmt.Sprintf("Speed (%s)", data.Units),
		fmt.Sprintf(
			`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle" transform="rotate(-90 %.2f %.2f)"`,
			style.AxisLabelFontPx, labelX, labelY))
	c.EndGroup()

	// Count Y-axis ticks (right side).
	c.BeginGroup(`class="count-axis"`)
	nCountTicks := 4
	for i := 0; i <= nCountTicks; i++ {
		val := float64(maxCount) * style.CountAxisScale * float64(i) / float64(nCountTicks)
		y := bottomPx - val*countScale
		c.Line(rightPx, y, rightPx+4, y, `stroke="black" stroke-width="0.5"`)
		c.Text(rightPx+6, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", val),
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="start"`, style.AxisTickFontPx))
	}
	countLabelX := rightPx + style.AxisTickFontPx*3.2
	c.Text(countLabelX, labelY, "Count",
		fmt.Sprintf(
			`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle" transform="rotate(90 %.2f %.2f)"`,
			style.AxisLabelFontPx, countLabelX, labelY))
	c.EndGroup()

	// Plot border.
	c.Line(leftPx, topPx, leftPx, bottomPx, `stroke="black" stroke-width="0.5"`)
	c.Line(leftPx, bottomPx, rightPx, bottomPx, `stroke="black" stroke-width="0.5"`)
	c.Line(rightPx, topPx, rightPx, bottomPx, `stroke="black" stroke-width="0.5"`)
	c.Line(leftPx, topPx, rightPx, topPx, `stroke="black" stroke-width="0.5"`)

	// Legend along the bottom. Evenly spaced across the plot width.
	legY := hPx - style.LegendFontPx/2 - 2
	legItems := len(series) + 1 // + low-sample swatch
	legStep := plotW / float64(legItems)
	for i, s := range series {
		x := leftPx + legStep*(float64(i)+0.5) - legStep/2
		lineA := fmt.Sprintf(`stroke="%s" stroke-width="%.1f"`, s.colour, style.LineWidthPx)
		if s.dashed {
			lineA += ` stroke-dasharray="6 3"`
		}
		c.Line(x, legY, x+16, legY, lineA)
		c.Text(x+20, legY+style.LegendFontPx/3, s.label,
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))
	}
	// Low-sample swatch.
	swX := leftPx + legStep*(float64(len(series))+0.5) - legStep/2
	c.Rect(swX, legY-style.LegendFontPx/2, 14, style.LegendFontPx,
		fmt.Sprintf(`fill="%s" fill-opacity="0.25"`, style.ColourLowSample))
	c.Text(swX+18, legY+style.LegendFontPx/3,
		fmt.Sprintf("low sample (<%d)", style.LowSampleThreshold),
		fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))

	// Title.
	if data.Title != "" {
		c.Text(wPx/2, style.AxisLabelFontPx+2, data.Title,
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle" font-weight="bold"`,
				style.AxisLabelFontPx*1.2))
	}

	return c.Bytes(), nil
}

func renderTimeSeriesNoData(style ChartStyle) []byte {
	c := NewCanvas(style.WidthMM, style.HeightMM)
	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM
	c.Text(wPx/2, hPx/2, "No data",
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	return c.Bytes()
}
