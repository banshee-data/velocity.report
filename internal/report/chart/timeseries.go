package chart

import (
	"fmt"
	"math"
	"time"
)

const (
	timeSeriesLegendPaddingPx = 8.0
	timeSeriesLegendMinGapPx  = 12.0
	timeSeriesLegendLinePx    = 16.0
	timeSeriesLegendTextGapPx = 4.0
)

type seriesDef struct {
	colour string
	marker string // "triangle" "square" "circle" "x" or "" for no marker
	field  func(TimeSeriesPoint) float64
	label  string
	dash   string // SVG stroke-dasharray, empty = solid
}

type timeSeriesLegendItem struct {
	label       string
	colour      string
	dash        string
	fill        string
	strokeWidth float64
	opacity     float64
}

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

// ExpandTimeSeriesGaps fills NaN placeholder points for every missing bucket
// between the first and last observed data point. This is useful when the
// chart should preserve linear timestamp spacing instead of visually
// compressing coverage gaps.
// groupSeconds must be > 0; if pts is empty or has only one point it is
// returned unchanged.
func ExpandTimeSeriesGaps(pts []TimeSeriesPoint, groupSeconds int64) []TimeSeriesPoint {
	if len(pts) < 2 || groupSeconds <= 0 {
		return pts
	}
	return ExpandTimeSeriesGapsInRange(pts, groupSeconds, pts[0].StartTime, pts[len(pts)-1].StartTime)
}

// ExpandTimeSeriesGapsInRange fills NaN placeholder points for every missing
// bucket across an explicit time range. This preserves the caller's full
// requested range even when observations start later or end earlier.
func ExpandTimeSeriesGapsInRange(pts []TimeSeriesPoint, groupSeconds int64, startTime, endTime time.Time) []TimeSeriesPoint {
	if len(pts) == 0 || groupSeconds <= 0 || !endTime.After(startTime) {
		return pts
	}
	loc := startTime.Location()
	existing := make(map[int64]TimeSeriesPoint, len(pts))
	for _, p := range pts {
		existing[p.StartTime.Unix()] = p
	}

	var out []TimeSeriesPoint
	for ts := startTime.Unix(); ts <= endTime.Unix(); ts += groupSeconds {
		if p, ok := existing[ts]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, TimeSeriesPoint{
			StartTime: time.Unix(ts, 0).In(loc),
			P50Speed:  math.NaN(),
			P85Speed:  math.NaN(),
			P98Speed:  math.NaN(),
			MaxSpeed:  math.NaN(),
			Count:     0,
		})
	}
	return out
}

// XTicks generates report tick labels without diagonal rotation. Sub-day ranges
// use one-line date+time labels; longer ranges label day starts only.
func XTicks(pts []TimeSeriesPoint) []XTick {
	if len(pts) < 2 {
		if len(pts) == 1 {
			return []XTick{{Index: 0, Label: pts[0].StartTime.Format("Jan 02 15:04")}}
		}
		return nil
	}

	if pts[len(pts)-1].StartTime.Sub(pts[0].StartTime) >= 24*time.Hour {
		boundaries := DayBoundaries(pts)
		ticks := make([]XTick, 0, len(boundaries))
		for _, idx := range boundaries {
			ticks = append(ticks, XTick{Index: idx, Label: pts[idx].StartTime.Format("Jan 02")})
		}
		return ticks
	}

	ticks := []XTick{{Index: 0, Label: pts[0].StartTime.Format("Jan 02 15:04")}}
	prevY, prevM, prevD := pts[0].StartTime.Date()
	bucketsInDay := 0
	for i := 1; i < len(pts); i++ {
		y, m, d := pts[i].StartTime.Date()
		if y != prevY || m != prevM || d != prevD {
			ticks = append(ticks, XTick{Index: i, Label: pts[i].StartTime.Format("Jan 02 15:04")})
			prevY, prevM, prevD = y, m, d
			bucketsInDay = 0
			continue
		}
		bucketsInDay++
		if bucketsInDay%3 == 0 {
			ticks = append(ticks, XTick{Index: i, Label: pts[i].StartTime.Format("Jan 02 15:04")})
		}
	}
	return ticks
}

func estimateXTickLabelWidth(ticks []XTick, fontPx float64) float64 {
	maxChars := 0
	for _, tick := range ticks {
		if len(tick.Label) > maxChars {
			maxChars = len(tick.Label)
		}
	}
	return float64(maxChars) * 0.62 * fontPx
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

	previewTicks := XTicks(data.Points)
	estLabelWidthPx := estimateXTickLabelWidth(previewTicks, style.AxisTickFontPx)

	// Compute speed and count ranges.
	var maxSpeed float64
	var maxCount int
	hasLowSample := false
	for _, pt := range data.Points {
		for _, s := range []float64{pt.P50Speed, pt.P85Speed, pt.P98Speed, pt.MaxSpeed} {
			if !math.IsNaN(s) && s > maxSpeed {
				maxSpeed = s
			}
		}
		if pt.Count > maxCount {
			maxCount = pt.Count
		}
		if pt.Count < style.LowSampleThreshold && pt.Count >= style.CountMissingThreshold {
			hasLowSample = true
		}
	}
	if maxSpeed == 0 {
		maxSpeed = 1
	}
	if maxCount == 0 {
		maxCount = 1
	}

	series := []seriesDef{
		{style.ColourP50, "triangle", func(p TimeSeriesPoint) float64 { return p.P50Speed }, "p50", ""},
		{style.ColourP85, "square", func(p TimeSeriesPoint) float64 { return p.P85Speed }, "p85", ""},
		{style.ColourP98, "circle", func(p TimeSeriesPoint) float64 { return p.P98Speed }, "p98", ""},
		{style.ColourMax, "", func(p TimeSeriesPoint) float64 { return p.MaxSpeed }, "Max", "1 3"},
	}
	drawRefLine := !math.IsNaN(data.P98Reference) && data.P98Reference > 0
	drawMaxRefLine := !math.IsNaN(data.MaxReference) && data.MaxReference > 0

	// Layout. Leave ~7% on each side for y-axis tick labels and reserve enough
	// bottom space for horizontal tick labels plus redistributed legend rows.
	tickLabelBlock := 2.0 * style.AxisTickFontPx
	leftPx := 0.12 * wPx
	rightPx := 0.93 * wPx
	topPx := 0.04 * hPx
	if data.Title != "" {
		topPx = style.AxisLabelFontPx*1.6 + 4
	}
	plotW := rightPx - leftPx
	legendItems := buildTimeSeriesLegendItems(series, drawRefLine, drawMaxRefLine, hasLowSample, style)
	legendRows := layoutTimeSeriesLegendRows(legendItems, plotW, style.LegendFontPx)
	legendRowH := style.LegendFontPx + 6
	legendBlock := float64(len(legendRows))*legendRowH + 10
	bottomMargin := tickLabelBlock + legendBlock + 4
	bottomPx := hPx - bottomMargin
	plotH := bottomPx - topPx
	n := len(data.Points)

	// Y-scale: speed axis rounds to the next nice-step ceiling so tick labels
	// are always clean round numbers. Step is 5 for low ranges, 10 for higher
	// ones (e.g. 60 mph), targeting ~6 ticks.
	rawSpeedMax := maxSpeed
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
	if countAxisMax <= 0 {
		countAxisMax = countStep
	}
	countScale := plotH / countAxisMax

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
	refY := 0.0
	if drawRefLine {
		refY = speedYOf(data.P98Reference)
		c.BeginGroup(`class="p98-reference"`)
		c.Line(leftPx, refY, rightPx, refY,
			fmt.Sprintf(`stroke="%s" stroke-dasharray="6 3" stroke-width="1.0" opacity="0.55"`, style.ColourP98))
		c.EndGroup()
	}

	// Max aggregate reference line (horizontal dashed black across the plot).
	maxRefY := 0.0
	if drawMaxRefLine {
		maxRefY = speedYOf(data.MaxReference)
		c.BeginGroup(`class="max-reference"`)
		c.Line(leftPx, maxRefY, rightPx, maxRefY,
			fmt.Sprintf(`stroke="%s" stroke-dasharray="1 3" stroke-width="0.7" opacity="0.40"`, ColourMax))
		c.EndGroup()
	}

	// Percentile lines — one polyline per contiguous run of non-NaN samples.
	// NaN values (missing data or low-sample buckets masked below
	// CountMissingThreshold) break the line into separate segments. Day
	// boundaries do not interrupt the line.

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

	// X-axis ticks — then cull any that would produce overlapping labels.
	ticks := previewTicks
	if len(ticks) > 1 && estLabelWidthPx > 0 {
		minGapPx := estLabelWidthPx
		kept := ticks[:1]
		for i := 1; i < len(ticks); i++ {
			if xOf(ticks[i].Index)-xOf(kept[len(kept)-1].Index) >= minGapPx {
				kept = append(kept, ticks[i])
			}
		}
		ticks = kept
	}
	c.BeginGroup(`class="x-axis"`)
	for _, t := range ticks {
		x := xOf(t.Index)
		c.Line(x, bottomPx, x, bottomPx+3, `stroke="black" stroke-width="0.5"`)
		y := bottomPx + style.AxisTickFontPx + 3
		c.Text(x, y, t.Label,
			fmt.Sprintf(
				`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle"`,
				style.AxisTickFontPx))
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
	for v := 0.0; v <= countAxisMax+0.01; v += countStep {
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

	// Legend along the bottom in a bordered box, matching the Python report.
	legBoxH := float64(len(legendRows))*legendRowH + 6
	legBoxY := hPx - legBoxH - 2
	c.Rect(leftPx, legBoxY, plotW, legBoxH, `fill="white" stroke="#ccc" stroke-width="0.6"`)
	for rowIndex, row := range legendRows {
		rowY := legBoxY + 4 + legendRowH*float64(rowIndex) + legendRowH/2
		rowW := timeSeriesLegendRowWidth(row, style.LegendFontPx)
		gap := timeSeriesLegendMinGapPx
		if len(row) > 1 {
			extra := plotW - 2*timeSeriesLegendPaddingPx - rowW
			if extra > 0 {
				gap += extra / float64(len(row)-1)
			}
		}
		x := leftPx + timeSeriesLegendPaddingPx
		for i, item := range row {
			if item.fill != "" {
				c.Rect(x, rowY-style.LegendFontPx/2, 14, style.LegendFontPx,
					fmt.Sprintf(`fill="%s" fill-opacity="0.25"`, item.fill))
			} else {
				lineA := fmt.Sprintf(`stroke="%s" stroke-width="%.1f"`, item.colour, item.strokeWidth)
				if item.dash != "" {
					lineA += fmt.Sprintf(` stroke-dasharray="%s"`, item.dash)
				}
				if item.opacity < 1 {
					lineA += fmt.Sprintf(` opacity="%.2f"`, item.opacity)
				}
				c.Line(x, rowY, x+timeSeriesLegendLinePx, rowY, lineA)
			}
			c.Text(x+timeSeriesLegendLinePx+timeSeriesLegendTextGapPx, rowY+style.LegendFontPx/3, item.label,
				fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))
			x += timeSeriesLegendItemWidth(item, style.LegendFontPx)
			if i < len(row)-1 {
				x += gap
			}
		}
	}

	// Title.
	if data.Title != "" {
		c.Text(wPx/2, style.AxisLabelFontPx+2, data.Title,
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle" font-weight="bold"`,
				style.AxisLabelFontPx*1.2))
	}

	return c.Bytes(), nil
}

func buildTimeSeriesLegendItems(series []seriesDef, drawRefLine, drawMaxRefLine, hasLowSample bool, style ChartStyle) []timeSeriesLegendItem {
	items := make([]timeSeriesLegendItem, 0, len(series)+3)
	for _, s := range series {
		items = append(items, timeSeriesLegendItem{
			label:       s.label,
			colour:      s.colour,
			dash:        s.dash,
			strokeWidth: style.LineWidthPx,
			opacity:     1,
		})
	}
	if drawRefLine {
		items = append(items, timeSeriesLegendItem{
			label:       "p98 overall",
			colour:      style.ColourP98,
			dash:        "6 3",
			strokeWidth: 1.0,
			opacity:     0.55,
		})
	}
	if drawMaxRefLine {
		items = append(items, timeSeriesLegendItem{
			label:       "max overall",
			colour:      ColourMax,
			dash:        "1 3",
			strokeWidth: 0.7,
			opacity:     0.40,
		})
	}
	if hasLowSample {
		items = append(items, timeSeriesLegendItem{
			label:   fmt.Sprintf("low sample (<%d)", style.LowSampleThreshold),
			fill:    style.ColourLowSample,
			opacity: 0.25,
		})
	}
	return items
}

func timeSeriesLegendItemWidth(item timeSeriesLegendItem, fontPx float64) float64 {
	return timeSeriesLegendLinePx + timeSeriesLegendTextGapPx + estimateLegendLabelWidth(item.label, fontPx)
}

func timeSeriesLegendRowWidth(items []timeSeriesLegendItem, fontPx float64) float64 {
	if len(items) == 0 {
		return 0
	}
	width := 0.0
	for i, item := range items {
		if i > 0 {
			width += timeSeriesLegendMinGapPx
		}
		width += timeSeriesLegendItemWidth(item, fontPx)
	}
	return width
}

func layoutTimeSeriesLegendRows(items []timeSeriesLegendItem, plotW, fontPx float64) [][]timeSeriesLegendItem {
	if len(items) == 0 {
		return [][]timeSeriesLegendItem{{}}
	}
	maxRowW := plotW - 2*timeSeriesLegendPaddingPx
	rows := make([][]timeSeriesLegendItem, 0, 2)
	current := make([]timeSeriesLegendItem, 0, len(items))
	currentW := 0.0
	for _, item := range items {
		itemW := timeSeriesLegendItemWidth(item, fontPx)
		addW := itemW
		if len(current) > 0 {
			addW += timeSeriesLegendMinGapPx
		}
		if len(current) > 0 && currentW+addW > maxRowW {
			rows = append(rows, current)
			current = []timeSeriesLegendItem{item}
			currentW = itemW
			continue
		}
		current = append(current, item)
		currentW += addW
	}
	if len(current) > 0 {
		rows = append(rows, current)
	}
	return rows
}

func estimateLegendLabelWidth(label string, fontPx float64) float64 {
	return float64(len(label)) * 0.58 * fontPx
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
