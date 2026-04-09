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

// DayBoundaries returns indices where the date changes.
// Index 0 is always included.
func DayBoundaries(pts []TimeSeriesPoint) []int {
	if len(pts) == 0 {
		return nil
	}
	boundaries := []int{0}
	prevDay := pts[0].StartTime.Truncate(24 * time.Hour)
	for i := 1; i < len(pts); i++ {
		day := pts[i].StartTime.Truncate(24 * time.Hour)
		if !day.Equal(prevDay) {
			boundaries = append(boundaries, i)
			prevDay = day
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

// XTicks generates tick labels for the time-series X axis.
// Day-start indices get "Mon DD\nHH:MM"; interior points every 3rd get "HH:MM".
func XTicks(pts []TimeSeriesPoint, boundaries []int) []XTick {
	if len(pts) == 0 {
		return nil
	}
	boundSet := make(map[int]struct{}, len(boundaries))
	for _, b := range boundaries {
		boundSet[b] = struct{}{}
	}

	var ticks []XTick
	for i, pt := range pts {
		if _, ok := boundSet[i]; ok {
			label := pt.StartTime.Format("Jan 02") + "\n" + pt.StartTime.Format("15:04")
			ticks = append(ticks, XTick{Index: i, Label: label})
		} else if i%3 == 0 {
			ticks = append(ticks, XTick{Index: i, Label: pt.StartTime.Format("15:04")})
		}
	}
	return ticks
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

	// Layout: matching matplotlib fig.subplots_adjust.
	leftPx := 0.02 * wPx
	rightPx := 0.96 * wPx
	topPx := 0.005 * hPx
	bottomPx := 0.84 * hPx

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

	// Day boundary vertical lines.
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

	// Percentile lines — segmented per day.
	type seriesDef struct {
		colour string
		marker string // "triangle", "square", "circle", "x"
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

	// Build segment ranges from boundaries.
	type segRange struct{ start, end int }
	var segments []segRange
	for i, b := range boundaries {
		end := n
		if i+1 < len(boundaries) {
			end = boundaries[i+1]
		}
		segments = append(segments, segRange{b, end})
	}

	for _, s := range series {
		lineAttrs := fmt.Sprintf(
			`fill="none" stroke="%s" stroke-width="%.1f"`,
			s.colour, style.LineWidthPx)
		if s.dashed {
			lineAttrs += ` stroke-dasharray="6 3"`
		}

		c.BeginGroup(fmt.Sprintf(`class="series-%s"`, s.label))

		for _, seg := range segments {
			var pts [][2]float64
			for i := seg.start; i < seg.end; i++ {
				val := s.field(data.Points[i])
				if math.IsNaN(val) {
					// Break segment.
					if len(pts) > 1 {
						c.Polyline(pts, lineAttrs)
					}
					pts = pts[:0]
					continue
				}
				px := xOf(i)
				py := speedYOf(val)
				pts = append(pts, [2]float64{px, py})
			}
			if len(pts) > 1 {
				c.Polyline(pts, lineAttrs)
			}
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
				// Equilateral triangle pointing up.
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
		// Multiline labels: split on \n.
		parts := strings.Split(t.Label, "\n")
		for j, part := range parts {
			y := bottomPx + style.AxisTickFontPx*(float64(j)+1) + 2
			c.Text(x, y, part,
				fmt.Sprintf(
					`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="middle"`,
					style.AxisTickFontPx))
		}
	}
	c.EndGroup()

	// Left Y-axis label.
	c.Text(leftPx-2, topPx-2,
		fmt.Sprintf("Speed (%s)", data.Units),
		fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible" text-anchor="start"`, style.AxisLabelFontPx))

	// Speed Y-axis ticks.
	nSpeedTicks := 5
	for i := range nSpeedTicks + 1 {
		val := maxSpeed * 1.1 * float64(i) / float64(nSpeedTicks)
		y := speedYOf(val)
		c.Line(leftPx-3, y, leftPx, y, `stroke="black" stroke-width="0.5"`)
		c.Text(leftPx-5, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", val),
			fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))
	}

	// Count Y-axis ticks (right side).
	nCountTicks := 4
	for i := range nCountTicks + 1 {
		val := float64(maxCount) * style.CountAxisScale * float64(i) / float64(nCountTicks)
		y := bottomPx - val*countScale
		c.Line(rightPx, y, rightPx+3, y, `stroke="black" stroke-width="0.5"`)
		c.Text(rightPx+5, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", val),
			fmt.Sprintf(`font-size="%.1f" text-anchor="start"`, style.AxisTickFontPx))
	}

	// Plot border.
	c.Line(leftPx, topPx, leftPx, bottomPx, `stroke="black" stroke-width="0.5"`)
	c.Line(leftPx, bottomPx, rightPx, bottomPx, `stroke="black" stroke-width="0.5"`)
	c.Line(rightPx, topPx, rightPx, bottomPx, `stroke="black" stroke-width="0.5"`)

	// Legend below chart.
	legY := hPx - style.LegendFontPx - 4
	legX := leftPx + 10
	gap := 90.0

	for i, s := range series {
		x := legX + float64(i)*gap
		// Line sample.
		lineA := fmt.Sprintf(`stroke="%s" stroke-width="%.1f"`, s.colour, style.LineWidthPx)
		if s.dashed {
			lineA += ` stroke-dasharray="6 3"`
		}
		c.Line(x, legY, x+20, legY, lineA)
		c.Text(x+24, legY+style.LegendFontPx/3, s.label,
			fmt.Sprintf(`font-size="%.1f" font-family="Atkinson Hyperlegible"`, style.LegendFontPx))
	}

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
