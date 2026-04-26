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
	// P98Reference is the aggregate p98 value for the full range, drawn as a
	// horizontal dashed red reference line across the plot. NaN = not drawn.
	P98Reference float64
	// MaxReference is the aggregate max speed for the full range, drawn as a
	// horizontal dashed black reference line across the plot. NaN = not drawn.
	MaxReference float64
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
func XTicks(pts []TimeSeriesPoint) []XTick {
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
		// 2h cadence → ≤6 ticks over 12h.
		return tickCadence{
			bucket: func(t time.Time) int64 { return t.Unix() / (2 * 60 * 60) },
			format: func(t time.Time) string { return t.Format("15:04") },
		}
	case span <= 48*time.Hour:
		// 6h cadence → ≤8 ticks over 48h. Single-line labels avoid overlap.
		return tickCadence{
			bucket: func(t time.Time) int64 { return t.Unix() / (6 * 60 * 60) },
			format: func(t time.Time) string { return t.Format("Jan 02 15:04") },
		}
	case span <= 7*24*time.Hour:
		// 12h cadence for 2-7 day ranges → ≤14 ticks at weekly max.
		return tickCadence{
			bucket: func(t time.Time) int64 { return t.Unix() / (12 * 60 * 60) },
			format: func(t time.Time) string { return t.Format("Jan 02 15:04") },
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

	// Mask speed values for buckets below the missing-count threshold so
	// low-sample periods render as gaps in the percentile lines. Count and
	// StartTime are preserved; count bars, low-sample highlights, and hover
	// tooltips continue to reflect the original (unmasked) data.
	maskedPts := ApplyCountMask(data.Points, style.CountMissingThreshold)
	timeGaps := detectTimeGaps(data.Points)

	// Decide whether x-axis labels need rotation based on time span.
	rotateXLabels := false
	if len(data.Points) >= 2 {
		span := data.Points[len(data.Points)-1].StartTime.Sub(data.Points[0].StartTime)
		// Estimate label width based on the cadence format for this span.
		var labelChars float64
		switch {
		case span <= 12*time.Hour:
			labelChars = 5 // "15:04"
		case span <= 7*24*time.Hour:
			labelChars = 12 // "Jan 02 15:04"
		default:
			labelChars = 6 // "Jan 02"
		}
		estLabelWidthPx := labelChars * 0.7 * style.AxisTickFontPx
		tentativePlotW := (0.93 - 0.12) * wPx
		previewTicks := XTicks(data.Points)
		if len(previewTicks) > 1 {
			tickSpacing := tentativePlotW / float64(len(previewTicks))
			rotateXLabels = tickSpacing < estLabelWidthPx
		}
	}

	// Layout. Leave ~7% on each side for y-axis tick labels and a tall
	// enough bottom strip for date/time tick labels plus the legend.
	tickLabelBlock := 2.6 * style.AxisTickFontPx // two-line labels
	if rotateXLabels {
		tickLabelBlock = 6.3 * style.AxisTickFontPx // rotated label vertical extent
	}
	legendBlock := style.LegendFontPx + 6
	bottomMargin := tickLabelBlock + legendBlock + 4

	leftPx := 0.12 * wPx
	rightPx := 0.93 * wPx
	topPx := 0.04 * hPx
	if data.Title != "" {
		topPx = style.AxisLabelFontPx*1.6 + 4
	}
	bottomPx := hPx - bottomMargin

	plotW := rightPx - leftPx
	plotH := bottomPx - topPx
	n := len(data.Points)

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

	// Y-scale: speed axis rounds to the next nice-step ceiling so tick labels
	// are always clean round numbers. Step is 5 for low ranges, 10 for higher
	// ones (e.g. 60 mph), targeting ~6 ticks.
	rawSpeedMax := maxSpeed * 1.1
	speedStep := niceStep(rawSpeedMax, 6)
	if speedStep < 5 {
		speedStep = 5
	}
	speedNiceMax := math.Ceil(rawSpeedMax/speedStep) * speedStep
	if speedNiceMax < speedStep {
		speedNiceMax = speedStep
	}
	speedScale := plotH / speedNiceMax

	// Count axis uses a magnitude-appropriate step giving ~4 clean labels.
	countAxisMax := float64(maxCount) * style.CountAxisScale
	countStep := niceStep(countAxisMax, 4)
	countNiceMax := math.Ceil(countAxisMax/countStep) * countStep
	if countNiceMax <= 0 {
		countNiceMax = countStep
	}
	countScale := plotH / countNiceMax

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

	// Gap dividers: a dashed vertical line where a significant gap begins —
	// either the start of a low-sample/missing NaN run, or a significant time
	// jump between consecutive data points (e.g. overnight hours with no data).
	// These are the only vertical dividers on the chart.
	c.BeginGroup(`class="gap-dividers"`)
	inNaNGap := false
	for i := range n {
		isNaN := math.IsNaN(maskedPts[i].P50Speed)
		if timeGaps[i] {
			// Time gap: place divider midway between the surrounding points.
			x := (xOf(i-1) + xOf(i)) / 2
			c.Line(x, topPx, x, bottomPx,
				`stroke="#999" stroke-dasharray="3 3" stroke-width="0.8" opacity="0.6"`)
			inNaNGap = false
		} else if !inNaNGap && isNaN && i > 0 {
			x := leftPx + float64(i)/float64(n)*plotW
			c.Line(x, topPx, x, bottomPx,
				`stroke="#999" stroke-dasharray="3 3" stroke-width="0.8" opacity="0.6"`)
		}
		inNaNGap = isNaN
	}
	c.EndGroup()

	// P98 aggregate reference line (horizontal dashed red across the plot).
	// Drawn before the series so the time-varying p98 line renders on top.
	drawRefLine := !math.IsNaN(data.P98Reference) && data.P98Reference > 0
	refY := 0.0
	if drawRefLine {
		refY = speedYOf(data.P98Reference)
		c.BeginGroup(`class="p98-reference"`)
		c.Line(leftPx, refY, rightPx, refY,
			fmt.Sprintf(`stroke="%s" stroke-dasharray="6 3" stroke-width="1.2" opacity="0.75"`, style.ColourP98))
		c.EndGroup()
	}

	// Max aggregate reference line (horizontal dashed black across the plot).
	drawMaxRefLine := !math.IsNaN(data.MaxReference) && data.MaxReference > 0
	maxRefY := 0.0
	if drawMaxRefLine {
		maxRefY = speedYOf(data.MaxReference)
		c.BeginGroup(`class="max-reference"`)
		c.Line(leftPx, maxRefY, rightPx, maxRefY,
			fmt.Sprintf(`stroke="%s" stroke-dasharray="1 3" stroke-width="0.8" opacity="0.55"`, ColourMax))
		c.EndGroup()
	}

	// Percentile lines — one polyline per contiguous run of non-NaN samples.
	// NaN values (missing data or low-sample buckets masked below
	// CountMissingThreshold) break the line into separate segments. Day
	// boundaries do not interrupt the line.
	type seriesDef struct {
		colour string
		marker string // "triangle" "square" "circle" "x" or "" for no marker
		field  func(TimeSeriesPoint) float64
		label  string
		dash   string // SVG stroke-dasharray, empty = solid
	}
	series := []seriesDef{
		{style.ColourP50, "triangle", func(p TimeSeriesPoint) float64 { return p.P50Speed }, "p50", ""},
		{style.ColourP85, "square", func(p TimeSeriesPoint) float64 { return p.P85Speed }, "p85", ""},
		{style.ColourP98, "circle", func(p TimeSeriesPoint) float64 { return p.P98Speed }, "p98", ""},
		{style.ColourMax, "", func(p TimeSeriesPoint) float64 { return p.MaxSpeed }, "max", "1 3"},
	}

	for _, s := range series {
		lineAttrs := fmt.Sprintf(
			`fill="none" stroke="%s" stroke-width="%.1f"`,
			s.colour, style.LineWidthPx)
		if s.dash != "" {
			lineAttrs += fmt.Sprintf(` stroke-dasharray="%s"`, s.dash)
		}

		c.BeginGroup(fmt.Sprintf(`class="series-%s"`, s.label))

		// Emit one polyline per contiguous run of non-NaN samples. NaN-masked
		// buckets and time gaps (detected missing periods) both break the line.
		var segment [][2]float64
		flushSegment := func() {
			if len(segment) > 1 {
				c.Polyline(segment, lineAttrs)
			}
			segment = segment[:0]
		}
		for i := range n {
			if timeGaps[i] {
				flushSegment()
			}
			val := s.field(maskedPts[i])
			if math.IsNaN(val) {
				flushSegment()
				continue
			}
			segment = append(segment, [2]float64{xOf(i), speedYOf(val)})
		}
		flushSegment()

		// Markers — skip masked/NaN positions so they match the line gaps.
		for i, pt := range maskedPts {
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
	ticks := XTicks(data.Points)
	c.BeginGroup(`class="x-axis"`)
	for _, t := range ticks {
		x := xOf(t.Index)
		c.Line(x, bottomPx, x, bottomPx+3, `stroke="black" stroke-width="0.5"`)
		if rotateXLabels {
			y := bottomPx + 6
			c.Text(x, y, t.Label,
				fmt.Sprintf(
					`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="end" transform="rotate(-45,%.4f,%.4f)"`,
					style.AxisTickFontPx, x, y))
		} else {
			parts := strings.Split(t.Label, "\n")
			for j, part := range parts {
				y := bottomPx + style.AxisTickFontPx*(float64(j)+1) + 3
				c.Text(x, y, part,
					fmt.Sprintf(
						`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle"`,
						style.AxisTickFontPx))
			}
		}
	}
	c.EndGroup()

	// Speed Y-axis label + ticks at computed speedStep.
	c.BeginGroup(`class="y-axis"`)
	for v := 0.0; v <= speedNiceMax+0.01; v += speedStep {
		y := speedYOf(v)
		c.Line(leftPx-4, y, leftPx, y, `stroke="black" stroke-width="0.5"`)
		c.Text(leftPx-6, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", v),
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="end"`, style.AxisTickFontPx))
	}
	// labelWithBg draws a white filled rect (no stroke) behind a right-aligned
	// axis label to prevent overlap with the regular tick numbers underneath.
	labelWithBg := func(x, y float64, label, colour string) {
		fs := style.AxisTickFontPx
		estW := float64(len(label)) * 0.62 * fs
		pad := 2.0
		c.Rect(x-estW-pad, y-fs*0.8-pad, estW+2*pad, fs+2*pad, `fill="white"`)
		c.Text(x, y, label, fmt.Sprintf(
			`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="end" fill="%s" font-weight="bold"`,
			fs, colour))
	}

	// Extra axis label for the aggregate P98 reference line.
	if drawRefLine {
		c.Line(leftPx-4, refY, leftPx, refY,
			fmt.Sprintf(`stroke="%s" stroke-width="1"`, style.ColourP98))
		labelWithBg(leftPx-6, refY+style.AxisTickFontPx/3,
			fmt.Sprintf("p98=%.0f", data.P98Reference), style.ColourP98)
	}
	// Extra axis label for the aggregate max reference line.
	if drawMaxRefLine {
		c.Line(leftPx-4, maxRefY, leftPx, maxRefY,
			fmt.Sprintf(`stroke="%s" stroke-width="0.8"`, ColourMax))
		labelWithBg(leftPx-6, maxRefY+style.AxisTickFontPx/3,
			fmt.Sprintf("max=%.0f", data.MaxReference), ColourMax)
	}
	// Rotated "Speed (units)" label. Anchored at 5% of chart width so it sits
	// clear of the tick/reference labels that right-align at leftPx-6.
	labelX := 0.05 * wPx
	labelY := (topPx + bottomPx) / 2
	c.Text(labelX, labelY,
		fmt.Sprintf("Speed (%s)", data.Units),
		fmt.Sprintf(
			`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle" transform="rotate(-90 %.2f %.2f)"`,
			style.AxisLabelFontPx, labelX, labelY))
	c.EndGroup()

	// Count Y-axis ticks (right side) — clean labels at every countStep.
	c.BeginGroup(`class="count-axis"`)
	for v := 0.0; v <= countNiceMax+0.01; v += countStep {
		y := bottomPx - v*countScale
		c.Line(rightPx, y, rightPx+4, y, `stroke="black" stroke-width="0.5"`)
		c.Text(rightPx+6, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", v),
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

	// Invisible hover targets: one full-height rect per time slot, each
	// carrying an SVG <title> so hovering shows count and percentile
	// metrics for that period. Must be drawn after the plot content so
	// they capture pointer events on top.
	c.BeginGroup(`class="hover-zones"`)
	for i, pt := range data.Points {
		x := leftPx + float64(i)/float64(n)*plotW
		w := plotW / float64(n)
		fmt.Fprintf(&c.buf,
			`<rect x="%.4f" y="%.4f" width="%.4f" height="%.4f" fill="transparent" pointer-events="all">`+"\n",
			x, topPx, w, plotH)
		c.buf.WriteString("<title>")
		xmlEscape(&c.buf, formatTooltip(pt, data.Units))
		c.buf.WriteString("</title>\n</rect>\n")
	}
	c.EndGroup()

	// Legend along the bottom. Evenly spaced across the plot width.
	legY := hPx - style.LegendFontPx/2 - 2
	legItems := len(series) + 1 // + low-sample swatch
	if drawRefLine {
		legItems++
	}
	if drawMaxRefLine {
		legItems++
	}
	legStep := plotW / float64(legItems)
	for i, s := range series {
		x := leftPx + legStep*(float64(i)+0.5) - legStep/2
		lineA := fmt.Sprintf(`stroke="%s" stroke-width="%.1f"`, s.colour, style.LineWidthPx)
		if s.dash != "" {
			lineA += fmt.Sprintf(` stroke-dasharray="%s"`, s.dash)
		}
		c.Line(x, legY, x+16, legY, lineA)
		c.Text(x+20, legY+style.LegendFontPx/3, s.label,
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))
	}
	legendIndex := len(series)
	if drawRefLine {
		x := leftPx + legStep*(float64(legendIndex)+0.5) - legStep/2
		c.Line(x, legY, x+16, legY,
			fmt.Sprintf(`stroke="%s" stroke-width="%.1f" stroke-dasharray="6 3" opacity="0.75"`, style.ColourP98, style.LineWidthPx))
		c.Text(x+20, legY+style.LegendFontPx/3, "p98 overall",
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))
		legendIndex++
	}
	if drawMaxRefLine {
		x := leftPx + legStep*(float64(legendIndex)+0.5) - legStep/2
		c.Line(x, legY, x+16, legY,
			fmt.Sprintf(`stroke="%s" stroke-width="0.8" stroke-dasharray="1 3" opacity="0.55"`, ColourMax))
		c.Text(x+20, legY+style.LegendFontPx/3, "max overall",
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))
		legendIndex++
	}
	// Low-sample swatch.
	swX := leftPx + legStep*(float64(legendIndex)+0.5) - legStep/2
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

// formatTooltip returns the hover text for one time slot.
func formatTooltip(pt TimeSeriesPoint, units string) string {
	when := pt.StartTime.Format("Jan 02 15:04")
	if pt.Count == 0 || math.IsNaN(pt.P50Speed) {
		return fmt.Sprintf("%s\ncount: 0\n(no samples)", when)
	}
	return fmt.Sprintf(
		"%s\ncount: %d\np50: %.1f %s\np85: %.1f %s\np98: %.1f %s\nmax: %.1f %s",
		when, pt.Count,
		pt.P50Speed, units,
		pt.P85Speed, units,
		pt.P98Speed, units,
		pt.MaxSpeed, units,
	)
}

// niceStep returns a round step size for an axis with the given max value,
// targeting approximately targetTicks divisions. The result is always a
// "nice" number of the form 1, 2, 5 × 10^n.
func niceStep(maxVal, targetTicks float64) float64 {
	if maxVal <= 0 || targetTicks <= 0 {
		return 1
	}
	raw := maxVal / targetTicks
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	norm := raw / mag
	switch {
	case norm < 1.5:
		return mag
	case norm < 3.5:
		return 2 * mag
	case norm < 7.5:
		return 5 * mag
	default:
		return 10 * mag
	}
}

// detectTimeGaps returns a boolean slice where isGapBefore[i] is true when
// the time step from pts[i-1] to pts[i] is more than 1.5× the minimum step
// seen across all consecutive pairs. This identifies real coverage gaps
// (e.g. overnight periods with no data) vs. normal consecutive samples.
// isGapBefore[0] is always false.
func detectTimeGaps(pts []TimeSeriesPoint) []bool {
	isGapBefore := make([]bool, len(pts))
	if len(pts) < 2 {
		return isGapBefore
	}
	// Find the minimum positive step between consecutive points.
	minGap := pts[1].StartTime.Sub(pts[0].StartTime)
	for i := 2; i < len(pts); i++ {
		if d := pts[i].StartTime.Sub(pts[i-1].StartTime); d > 0 && d < minGap {
			minGap = d
		}
	}
	if minGap <= 0 {
		return isGapBefore
	}
	threshold := time.Duration(float64(minGap) * 1.5)
	for i := 1; i < len(pts); i++ {
		if pts[i].StartTime.Sub(pts[i-1].StartTime) > threshold {
			isGapBefore[i] = true
		}
	}
	return isGapBefore
}

func renderTimeSeriesNoData(style ChartStyle) []byte {
	c := NewCanvas(style.WidthMM, style.HeightMM)
	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM
	c.Text(wPx/2, hPx/2, "No data",
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	return c.Bytes()
}
