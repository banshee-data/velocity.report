package chart

// The headway histogram: a time-weighted gap or time-gap distribution on a
// share axis, with every second of accounted time outside it drawn beside it,
// so a thin bar cannot be read as a quiet road. It serves the scene route's
// distribution through /api/charts/histogram (D-11, D-17).
//
// The chart names nothing itself. The metric id, the reason tokens and the
// benchmark kind come from the caller, which reads them from the behaviour
// registries, and are drawn verbatim in text elements of class "vocab" so a
// test can check them. What the chart does own:
//
//   - Share axis. Every bar is a percentage of the accounted time, valid
//     plus suppressed, so the bins sum to the valid share and never to 100 %
//     unless nothing was suppressed.
//   - Excluded columns. Accounted time outside the bins is one column per
//     reason on the same axis, its predicted-only part drawn lighter above
//     its observed part; each column is keyed below the chart by number to
//     its reason and duration. Nothing excluded is dropped or shown as zero.
//   - Uncertainty. A whisker on each bin spans the caller's one-sigma reach
//     bounds: the time wholly inside the bin at one sigma, to the time that
//     reaches it.
//   - Bands. Neutral dashed rules labelled with the threshold only, and a
//     note naming their benchmark kind. No alarm or percentile colour.
//   - Status. The caller's status label, boxed at the top right of every
//     chart, including the empty and message charts. Rendering refuses an
//     empty label.

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Headway chart colours. They are neutral and separate from the §3.3
// percentile palette: the chart shows observed gaps, not a speed population,
// and must not borrow an alarm colour.
const (
	colourHeadwayExcluded      = "#8c8c8c" // suppressed time on an observed basis
	colourHeadwayPredictedOnly = "#d4d4d4" // suppressed time with a party unobserved
	colourHeadwayStatus        = "#b26b00" // semantic warning amber (DESIGN §3.2)
)

// HeadwayBin is one bin of the distribution, [Lower, Upper) in the metric's
// unit; Upper is nil on an open last bin. Nanos is its valid time and
// SigmaInsideNanos and SigmaOverlapNanos its one-sigma reach bounds.
type HeadwayBin struct {
	Lower             float64
	Upper             *float64
	Nanos             int64
	SigmaInsideNanos  int64
	SigmaOverlapNanos int64
}

// HeadwayBand is one band rule, at a bin edge, in the metric's unit.
type HeadwayBand struct {
	Threshold float64
	Label     string
}

// HeadwayExcluded is accounted time outside the bins under one reason.
type HeadwayExcluded struct {
	Reason             string
	Nanos              int64
	PredictedOnlyNanos int64
}

// HeadwayHistogramData is everything the chart draws.
type HeadwayHistogramData struct {
	// StatusLabel is required and drawn on every chart.
	StatusLabel string
	// Metric is the binned metric's registry id and Unit its unit.
	Metric string
	Unit   string
	Bins   []HeadwayBin
	// Bands are drawn only where a threshold is a bin edge; BandBenchmark
	// is their registered benchmark kind, drawn verbatim.
	Bands         []HeadwayBand
	BandBenchmark string
	// Excluded is in the order the columns are drawn.
	Excluded []HeadwayExcluded
	// AccountedNanos is the share denominator: the bins' time plus every
	// excluded reason's time.
	AccountedNanos int64
	// RecordGapNanos stands for nothing and is stated, not drawn.
	RecordGapNanos int64
	// Encounters is how many were pooled; InBins how many fill the bins.
	Encounters int
	InBins     int
}

// DefaultHeadwayHistogramStyle is the full text width of the given paper,
// half as tall, which leaves room for a column per excluded reason.
func DefaultHeadwayHistogramStyle(paper PaperSize) ChartStyle {
	s := baseStyle()
	s.WidthMM = paperTextWidthMM(paper)
	s.HeightMM = s.WidthMM * 0.5
	s.AxisLabelFontPx = 10.0
	s.AxisTickFontPx = 8.5
	s.LegendFontPx = 8.5
	s.LineWidthPx = 0.8
	s.BarWidthFraction = 0.86
	return s
}

// ErrHeadwayStatusRequired refuses a headway chart without its status.
var ErrHeadwayStatusRequired = errors.New("a headway chart must carry its status label")

// RenderHeadwayHistogram draws the distribution. With no accounted time it
// draws a message chart instead, still labelled.
func RenderHeadwayHistogram(data HeadwayHistogramData, style ChartStyle) ([]byte, error) {
	if strings.TrimSpace(data.StatusLabel) == "" {
		return nil, ErrHeadwayStatusRequired
	}
	if len(data.Bins) == 0 {
		return nil, errors.New("a headway chart needs its bins")
	}
	var binned, excluded, predicted int64
	for _, b := range data.Bins {
		binned += b.Nanos
	}
	for _, x := range data.Excluded {
		excluded += x.Nanos
		predicted += x.PredictedOnlyNanos
	}
	if binned+excluded != data.AccountedNanos {
		return nil, fmt.Errorf("bins %d ns and excluded %d ns do not add up to %d ns accounted", binned, excluded, data.AccountedNanos)
	}
	if data.AccountedNanos == 0 {
		return RenderHeadwayMessage(data.StatusLabel, style, "No following time was accounted in this selection.")
	}

	// The plot keeps the style's proportions; a long key adds rows below.
	wPx, hPx := style.WidthMM*pxPerMM, style.HeightMM*pxPerMM
	extraPx := float64(headwayExtraRows(len(data.Excluded))) * headwayKeyRowPx
	c := NewCanvas(style.WidthMM, style.HeightMM+extraPx/pxPerMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)

	leftM, rightM := 46.0, wPx-14
	topM, bottomM := 58.0, hPx-headwayBelowPlotPx
	plotH := bottomM - topM
	share := func(nanos int64) float64 { return float64(nanos) / float64(data.AccountedNanos) * 100 }

	// Header: the metric id, the counts and the shares.
	c.Text(leftM, 16, data.Metric, fmt.Sprintf(`class="vocab" font-size="%.1f" font-weight="bold"`, style.AxisLabelFontPx+1))
	summary := fmt.Sprintf("%d encounters, %d in the bins. Share of %s accounted time: %s in the bins, %s beside them, %s predicted-only.",
		data.Encounters, data.InBins, formatSeconds(data.AccountedNanos), formatHeadwayPercent(share(binned)),
		formatHeadwayPercent(share(excluded)), formatHeadwayPercent(share(predicted)))
	c.Text(leftM, 31, summary, fmt.Sprintf(`font-size="%.1f"`, style.LegendFontPx))
	if data.RecordGapNanos > 0 {
		c.Text(leftM, 43, fmt.Sprintf("Record gaps of %s stand for no time and are not counted.", formatSeconds(data.RecordGapNanos)),
			fmt.Sprintf(`font-size="%.1f"`, style.LegendFontPx))
	}
	drawHeadwayStatus(c, data.StatusLabel, rightM, style)

	// Columns: the bins, a spacer, then one column per excluded reason.
	nBins, nExcluded := len(data.Bins), len(data.Excluded)
	spacer := 0.0
	if nExcluded > 0 {
		spacer = 0.8
	}
	slotW := (rightM - leftM) / (float64(nBins+nExcluded) + spacer)
	barW := math.Max(slotW*style.BarWidthFraction, 1)
	binsRight := leftM + float64(nBins)*slotW
	columnX := func(i int) float64 {
		if i < nBins {
			return leftM + float64(i)*slotW
		}
		return binsRight + spacer*slotW + float64(i-nBins)*slotW
	}

	// Y scale over everything drawn, whisker tops included.
	maxShare := 0.0
	for _, b := range data.Bins {
		maxShare = math.Max(maxShare, share(max(b.Nanos, b.SigmaOverlapNanos)))
	}
	for _, x := range data.Excluded {
		maxShare = math.Max(maxShare, share(x.Nanos))
	}
	step := math.Max(niceStep(maxShare, 5), 1)
	yMax := math.Max(math.Ceil(maxShare/step)*step, step)
	y := func(pct float64) float64 { return bottomM - pct/yMax*plotH }

	c.BeginGroup(`class="y-axis"`)
	for v := 0.0; v <= yMax+step/100; v += step {
		c.Line(leftM-3, y(v), leftM, y(v), `stroke="black" stroke-width="0.5"`)
		c.Text(leftM-5, y(v)+style.AxisTickFontPx/3, strconv.FormatFloat(v, 'f', -1, 64)+"%",
			fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))
	}
	c.Line(leftM, topM, leftM, bottomM, `stroke="black" stroke-width="1"`)
	labelX, labelY := 12.0, (topM+bottomM)/2
	c.Text(labelX, labelY, "share of accounted time",
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle" transform="rotate(-90 %.2f %.2f)"`, style.AxisTickFontPx, labelX, labelY))
	c.EndGroup()

	c.BeginGroup(`class="headway-bins"`)
	for i, b := range data.Bins {
		x := columnX(i) + (slotW-barW)/2
		c.Rect(x, y(share(b.Nanos)), barW, bottomM-y(share(b.Nanos)),
			fmt.Sprintf(`fill="%s" fill-opacity="0.8"`, ColourSteelBlue))
	}
	c.EndGroup()

	c.BeginGroup(`class="headway-reach"`)
	for i, b := range data.Bins {
		if b.SigmaOverlapNanos <= b.SigmaInsideNanos {
			continue
		}
		cx, tick := columnX(i)+slotW/2, barW*0.2
		lo, hi := y(share(b.SigmaInsideNanos)), y(share(b.SigmaOverlapNanos))
		c.Line(cx, lo, cx, hi, `stroke="black" stroke-width="0.9"`)
		c.Line(cx-tick, hi, cx+tick, hi, `stroke="black" stroke-width="0.9"`)
		c.Line(cx-tick, lo, cx+tick, lo, `stroke="black" stroke-width="0.9"`)
	}
	c.EndGroup()

	// X axis and edge labels under the bins; open last bin marked "+".
	c.BeginGroup(`class="x-axis"`)
	c.Line(leftM, bottomM, binsRight, bottomM, `stroke="black" stroke-width="1"`)
	every := 1
	if slotW < 24 {
		every = 2
	}
	for i, b := range data.Bins {
		x := columnX(i)
		if i%every == 0 {
			c.Line(x, bottomM, x, bottomM+3, `stroke="black" stroke-width="0.5"`)
			label := strconv.FormatFloat(b.Lower, 'f', -1, 64)
			if b.Upper == nil {
				label += "+"
			}
			c.Text(x, bottomM+style.AxisTickFontPx+5, label,
				fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
		}
	}
	c.Text((leftM+binsRight)/2, bottomM+2*style.AxisTickFontPx+11,
		fmt.Sprintf("value (%s); bins include their lower edge", data.Unit),
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
	c.EndGroup()

	// Band rules at bin edges.
	if len(data.Bands) > 0 {
		c.BeginGroup(`class="headway-bands"`)
		for _, band := range data.Bands {
			x, ok := headwayEdgeX(data.Bins, band.Threshold, columnX)
			if !ok {
				continue
			}
			c.Line(x, topM-2, x, bottomM, `stroke="black" stroke-width="0.8" stroke-dasharray="4 3"`)
			c.Text(x, topM-5, band.Label, fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
		}
		c.EndGroup()
	}

	// Excluded columns: observed part, then predicted-only part above it.
	if nExcluded > 0 {
		c.BeginGroup(`class="headway-excluded"`)
		sepX := binsRight + spacer*slotW/2
		c.Line(sepX, topM-2, sepX, bottomM, `stroke="#8c8c8c" stroke-width="0.6" stroke-dasharray="1 2"`)
		c.Text((binsRight+spacer*slotW+rightM)/2, topM-5, "beside the bins",
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
		for j, x := range data.Excluded {
			i := nBins + j
			bx := columnX(i) + (slotW-barW)/2
			observedTop := y(share(x.Nanos - x.PredictedOnlyNanos))
			c.Rect(bx, observedTop, barW, bottomM-observedTop, fmt.Sprintf(`fill="%s"`, colourHeadwayExcluded))
			if x.PredictedOnlyNanos > 0 {
				top := y(share(x.Nanos))
				c.Rect(bx, top, barW, observedTop-top, fmt.Sprintf(
					`fill="%s" stroke="%s" stroke-width="0.6" stroke-dasharray="2 2"`, colourHeadwayPredictedOnly, colourHeadwayExcluded))
			}
			c.Line(columnX(i), bottomM, columnX(i)+slotW, bottomM, `stroke="black" stroke-width="1"`)
			c.Text(columnX(i)+slotW/2, bottomM+style.AxisTickFontPx+5, strconv.Itoa(j+1),
				fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
		}
		c.EndGroup()
	}

	drawHeadwayLegend(c, data, leftM, rightM, bottomM, style)
	c.EndGroup()
	return c.Bytes(), nil
}

// headwayEdgeX is the x of a bin edge equal to v, if there is one.
func headwayEdgeX(bins []HeadwayBin, v float64, columnX func(int) float64) (float64, bool) {
	for i, b := range bins {
		if b.Lower == v {
			return columnX(i), true
		}
		if b.Upper != nil && *b.Upper == v {
			return columnX(i + 1), true
		}
	}
	return 0, false
}

// Legend geometry: the key from column numbers to reasons is a grid of
// headwayKeyColumns, and the base height holds headwayKeyRows of it; more
// reasons than that grow the canvas by a row each.
const (
	headwayKeyColumns  = 3
	headwayKeyRows     = 2
	headwayKeyRowPx    = 12.0
	headwayBelowPlotPx = 100.0
)

// headwayExtraRows is how many key rows the canvas must add for n reasons.
func headwayExtraRows(n int) int {
	rows := (n + headwayKeyColumns - 1) / headwayKeyColumns
	return max(rows-headwayKeyRows, 0)
}

// drawHeadwayLegend draws the swatches, the band note and the key from
// column numbers to reasons, below the plot's bottom edge.
func drawHeadwayLegend(c *SVGCanvas, data HeadwayHistogramData, leftM, rightM, bottomM float64, style ChartStyle) {
	font := fmt.Sprintf(`font-size="%.1f"`, style.LegendFontPx)
	// Swatch labels are fixed English, so a generous estimate of their width
	// (0.6 em a character) spaces them; registry tokens never rely on one.
	textW := func(s string) float64 { return float64(len([]rune(s))) * style.LegendFontPx * 0.6 }
	rowY := bottomM + 50
	x := leftM
	item := func(label string) {
		c.Text(x+13, rowY, label, font)
		x += 13 + textW(label) + 12
	}
	c.BeginGroup(`class="legend"`)
	c.Rect(x, rowY-7, 9, 8, fmt.Sprintf(`fill="%s" fill-opacity="0.8"`, ColourSteelBlue))
	item("valid time in the bins")
	c.Line(x+4, rowY-8, x+4, rowY+1, `stroke="black" stroke-width="0.9"`)
	item("one-sigma reach of the bin's time")
	c.Rect(x, rowY-7, 9, 8, fmt.Sprintf(`fill="%s"`, colourHeadwayExcluded))
	item("suppressed, both observed")
	c.Rect(x, rowY-7, 9, 8, fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="0.6" stroke-dasharray="2 2"`,
		colourHeadwayPredictedOnly, colourHeadwayExcluded))
	item("suppressed, predicted-only")
	if len(data.Bands) > 0 && data.BandBenchmark != "" {
		c.textRuns(leftM, rowY+14, font, textRun{text: "Band rules are descriptive bins with benchmark kind "},
			textRun{text: data.BandBenchmark, class: "vocab"})
	}
	// The key: one line per column, number, reason and duration, on a grid
	// wide enough for the longest registered reason.
	colW := (rightM - leftM) / headwayKeyColumns
	for j, ex := range data.Excluded {
		kx := leftM + float64(j%headwayKeyColumns)*colW
		ky := rowY + 28 + float64(j/headwayKeyColumns)*headwayKeyRowPx
		c.textRuns(kx, ky, font, textRun{text: strconv.Itoa(j+1) + " "}, textRun{text: ex.Reason, class: "vocab"},
			textRun{text: " " + formatSeconds(ex.Nanos)})
	}
	c.EndGroup()
}

// textRun is one run of a text line. A run with a class is a tspan of that
// class, so a registry token keeps its own element without its neighbours'
// widths being guessed.
type textRun struct {
	text, class string
}

// textRuns emits one <text> line made of runs, XML-escaped.
func (c *SVGCanvas) textRuns(x, y float64, attrs string, runs ...textRun) {
	fmt.Fprintf(&c.buf, `<text x="%.4f" y="%.4f"`, x, y)
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteByte('>')
	for _, r := range runs {
		if r.class == "" {
			xmlEscape(&c.buf, r.text)
			continue
		}
		fmt.Fprintf(&c.buf, `<tspan class="%s">`, r.class)
		xmlEscape(&c.buf, r.text)
		c.buf.WriteString("</tspan>")
	}
	c.buf.WriteString("</text>\n")
}

// drawHeadwayStatus boxes the status label at the top right.
func drawHeadwayStatus(c *SVGCanvas, label string, rightM float64, style ChartStyle) {
	// Bold capitals run wide, and a fallback font wider still; 0.7 em a
	// character keeps the label inside its box under either.
	size := style.AxisLabelFontPx
	w := float64(len([]rune(label)))*size*0.7 + 14
	c.BeginGroup(`class="status"`)
	c.Rect(rightM-w, 4, w, size+8, fmt.Sprintf(`fill="white" stroke="%s" stroke-width="1.2"`, colourHeadwayStatus))
	c.Text(rightM-w/2, 4+size+3, label, fmt.Sprintf(`font-size="%.1f" font-weight="bold" text-anchor="middle"`, size))
	c.EndGroup()
}

// RenderHeadwayMessage draws a labelled chart that holds only a message, for
// a selection with nothing to draw. It still carries the status.
func RenderHeadwayMessage(statusLabel string, style ChartStyle, lines ...string) ([]byte, error) {
	if strings.TrimSpace(statusLabel) == "" {
		return nil, ErrHeadwayStatusRequired
	}
	wPx, hPx := style.WidthMM*pxPerMM, style.HeightMM*pxPerMM
	c := NewCanvas(style.WidthMM, style.HeightMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)
	drawHeadwayStatus(c, statusLabel, wPx-14, style)
	for i, line := range lines {
		c.Text(wPx/2, hPx/2+float64(i)*(style.AxisLabelFontPx+4), line,
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	}
	c.EndGroup()
	return c.Bytes(), nil
}

// formatSeconds writes nanoseconds as seconds to one decimal place.
func formatSeconds(nanos int64) string {
	return strconv.FormatFloat(float64(nanos)/1e9, 'f', 1, 64) + " s"
}

// formatHeadwayPercent writes a percentage, to one decimal place below 10 %.
func formatHeadwayPercent(pct float64) string {
	if pct > 0 && pct < 10 {
		return strconv.FormatFloat(pct, 'f', 1, 64) + "%"
	}
	return strconv.FormatFloat(pct, 'f', 0, 64) + "%"
}
