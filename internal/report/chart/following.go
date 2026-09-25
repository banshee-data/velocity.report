package chart

// Following-evidence charts for the headway report (behaviour plan Section
// 10.4): where an encounter's time went, and what one pair's gap did.
//
// The rules they keep, recorded in docs/ui/DESIGN.md:
//
//   - The chart names nothing. Every metric id, suppression reason and
//     visibility token is supplied by the caller from the behaviour
//     registries and drawn verbatim, in a <text class="vocab"> element, so a
//     caller's test can check each one against the registry. Track and
//     encounter references are drawn with class "ref", the status label with
//     class "status". Other text is axis furniture: numbers, units and plain
//     words.
//   - The status label is required and drawn in a box on every chart, so a
//     chart lifted out of its report still says it is a synthetic oracle or a
//     provisional result.
//   - An observed series is broken, never bridged, across a sample interval
//     longer than the caller's break threshold. The observed gap series holds
//     supported instants only, so a suppressed or coasted interval is a hole
//     in the line, not a stretch of it drawn as though it had been measured.
//   - The review-only predicted gap is dashed, grey and hollow-marked, with
//     a one-sigma whisker at each instant and its coast age written beside
//     it, and it never joins the observed line.
//   - Band thresholds are neutral dashed rules: descriptive bins, not alarms.
//   - Time that is not in a distribution is drawn as its own share, by reason,
//     never folded into a zero bar.

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// Text classes the following charts put on caller-supplied labels.
const (
	TextClassVocab  = "vocab"
	TextClassRef    = "ref"
	TextClassStatus = "status"
)

// Threshold is one band edge drawn as a neutral dashed rule.
type Threshold struct {
	Value float64
	// Label is axis furniture: the edge and its unit, such as "2 s".
	Label string
}

// DistributionBin is one bin of a time-weighted distribution: its share of
// the chart's denominator. Bins are contiguous and half-open, [Lower, Upper);
// an overflow bin is [Lower, +inf) and must be last.
type DistributionBin struct {
	Lower    float64
	Upper    float64
	Overflow bool
	Share    float64
}

// ExcludedShare is time that is in the denominator but not in the
// distribution, under a registered reason.
type ExcludedShare struct {
	Label string
	Share float64
}

// FollowingDistributionData is a time-weighted distribution of one metric
// with its excluded time beside it. The shares of the bins and the excluded
// entries together are the whole denominator.
type FollowingDistributionData struct {
	Status     string
	Metric     string
	Unit       string
	Bins       []DistributionBin
	Thresholds []Threshold
	Excluded   []ExcludedShare
}

// shareTolerance absorbs rounding when shares are checked to sum to one.
const shareTolerance = 1e-9

func (d FollowingDistributionData) validate() (total float64, err error) {
	if d.Status == "" {
		return 0, errors.New("following chart requires a status label")
	}
	if d.Metric == "" || d.Unit == "" {
		return 0, errors.New("distribution chart requires its metric and unit")
	}
	if len(d.Bins) == 0 {
		return 0, errors.New("distribution chart requires bins")
	}
	for i, b := range d.Bins {
		if !finite(b.Lower) || !(b.Share >= 0 && b.Share <= 1) {
			return 0, fmt.Errorf("distribution bin %d must have a finite edge and a share in [0, 1]", i)
		}
		if b.Overflow {
			if i != len(d.Bins)-1 {
				return 0, errors.New("only the last distribution bin may overflow")
			}
		} else if !(b.Upper > b.Lower) || !finite(b.Upper) {
			return 0, fmt.Errorf("distribution bin %d must have an upper edge above its lower", i)
		}
		if i > 0 && d.Bins[i-1].Upper != b.Lower {
			return 0, fmt.Errorf("distribution bins %d and %d are not contiguous", i-1, i)
		}
		total += b.Share
	}
	for _, e := range d.Excluded {
		if e.Label == "" || !(e.Share >= 0 && e.Share <= 1) {
			return 0, errors.New("excluded share requires a label and a share in [0, 1]")
		}
		total += e.Share
	}
	if total > 0 && math.Abs(total-1) > shareTolerance {
		return 0, fmt.Errorf("distribution shares sum to %.12g, want 1", total)
	}
	lo, hi := d.Bins[0].Lower, d.Bins[len(d.Bins)-1].Lower
	if last := d.Bins[len(d.Bins)-1]; !last.Overflow {
		hi = last.Upper
	}
	for _, t := range d.Thresholds {
		if !(t.Value >= lo && t.Value <= hi) {
			return 0, fmt.Errorf("threshold %g lies outside the bins", t.Value)
		}
	}
	return total, nil
}

// DefaultFollowingDistributionStyle sizes the distribution chart to the
// report's text width.
func DefaultFollowingDistributionStyle(paper PaperSize) ChartStyle {
	s := baseStyle()
	s.WidthMM = paperTextWidthMM(paper)
	s.HeightMM = s.WidthMM * 0.40
	s.AxisLabelFontPx = 10.0
	s.AxisTickFontPx = 8.5
	s.LegendFontPx = 9.0
	s.LineWidthPx = 1.0
	return s
}

// DefaultFollowingEncounterStyle sizes the per-encounter chart to the
// report's text width.
func DefaultFollowingEncounterStyle(paper PaperSize) ChartStyle {
	s := baseStyle()
	s.WidthMM = paperTextWidthMM(paper)
	s.HeightMM = s.WidthMM * 0.50
	s.AxisLabelFontPx = 10.0
	s.AxisTickFontPx = 8.5
	s.LegendFontPx = 9.0
	s.LineWidthPx = 1.2
	s.MarkerRadiusPx = 2.0
	return s
}

// RenderFollowingDistribution draws the bins as bars on a share axis, the
// band thresholds as rules, and every excluded share as its own labelled
// column beside them.
func RenderFollowingDistribution(d FollowingDistributionData, style ChartStyle) ([]byte, error) {
	total, err := d.validate()
	if err != nil {
		return nil, err
	}
	wPx, hPx := style.WidthMM*pxPerMM, style.HeightMM*pxPerMM
	c := NewCanvas(style.WidthMM, style.HeightMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)
	top := statusBanner(c, d.Status, style) + 16
	if total == 0 {
		c.Text(wPx/2, (top+hPx)/2, "No accounted time",
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
		c.EndGroup()
		return c.Bytes(), nil
	}

	left, right := 52.0, wPx-10
	bottom := hPx - (style.AxisTickFontPx + style.AxisLabelFontPx + 22)
	const excludedColW, excludedGap = 34.0, 34.0
	mainRight := right
	if len(d.Excluded) > 0 {
		mainRight = right - excludedGap - float64(len(d.Excluded))*excludedColW
	}

	maxShare := 0.0
	for _, b := range d.Bins {
		maxShare = math.Max(maxShare, b.Share)
	}
	for _, e := range d.Excluded {
		maxShare = math.Max(maxShare, e.Share)
	}
	step := math.Max(niceStep(maxShare*100, 5), 1)
	yMax := math.Ceil(maxShare*100/step) * step
	y := linearScale{d0: 0, d1: yMax, r0: bottom, r1: top}

	// Every bin is drawn at its own width; an overflow bin borrows the width
	// of the bin before it, or one unit when it stands alone.
	width := func(i int) float64 {
		b := d.Bins[i]
		switch {
		case !b.Overflow:
			return b.Upper - b.Lower
		case i > 0:
			return d.Bins[i-1].Upper - d.Bins[i-1].Lower
		}
		return 1
	}
	last := len(d.Bins) - 1
	x := linearScale{d0: d.Bins[0].Lower, d1: d.Bins[last].Lower + width(last), r0: left, r1: mainRight}

	tickAttr := fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx)
	for v := 0.0; v <= yMax+step/2; v += step {
		yy := y.at(v)
		c.Line(left-3, yy, mainRight, yy, `stroke="#e6e6e6" stroke-width="0.5"`)
		c.Text(left-5, yy+style.AxisTickFontPx/3, fmt.Sprintf("%.0f%%", v), tickAttr)
	}

	barAttr := fmt.Sprintf(`fill="%s" fill-opacity="0.75" stroke="black" stroke-width="0.5"`, ColourFollowingTimeGap)
	labelAttr := fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx-1)
	for i, b := range d.Bins {
		x0, x1 := x.at(b.Lower), x.at(b.Lower+width(i))
		if b.Share > 0 {
			c.Rect(x0+1, y.at(b.Share*100), x1-x0-2, bottom-y.at(b.Share*100), barAttr)
			c.Text((x0+x1)/2, y.at(b.Share*100)-3, formatShare(b.Share), labelAttr)
		}
		if b.Overflow {
			c.Text((x0+x1)/2, bottom+style.AxisTickFontPx+5, formatAxisNumber(b.Lower)+"+",
				fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
		}
	}
	xStep := niceStep(d.Bins[last].Lower-d.Bins[0].Lower, 6)
	for i := 0; i <= last; i++ {
		if d.Bins[i].Overflow {
			break
		}
		for _, edge := range []float64{d.Bins[i].Lower, d.Bins[i].Upper} {
			if i > 0 && edge == d.Bins[i].Lower {
				continue // drawn as the previous bin's upper edge
			}
			if !onStep(edge-d.Bins[0].Lower, xStep) {
				continue
			}
			c.Line(x.at(edge), bottom, x.at(edge), bottom+3, `stroke="black" stroke-width="0.5"`)
			c.Text(x.at(edge), bottom+style.AxisTickFontPx+5, formatAxisNumber(edge),
				fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
		}
	}

	ruleAttr := fmt.Sprintf(`stroke="%s" stroke-width="1" stroke-dasharray="4 3"`, ColourFollowingThreshold)
	for _, t := range d.Thresholds {
		xx := x.at(t.Value)
		c.Line(xx, top, xx, bottom, ruleAttr)
		c.Text(xx+3, top-4, t.Label,
			fmt.Sprintf(`font-size="%.1f" fill="%s"`, style.AxisTickFontPx, ColourFollowingThreshold))
	}

	c.Line(left, top, left, bottom, `stroke="black" stroke-width="1"`)
	c.Line(left, bottom, mainRight, bottom, `stroke="black" stroke-width="1"`)
	axisY := bottom + style.AxisTickFontPx + style.AxisLabelFontPx + 14
	axisLabel(c, (left+mainRight)/2, axisY, d.Metric, d.Unit, style.AxisLabelFontPx)
	yl := (top + bottom) / 2
	c.Text(12, yl, "share of accounted time (%)",
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle" transform="rotate(-90 12 %.4f)"`, style.AxisLabelFontPx, yl))

	if len(d.Excluded) > 0 {
		sepX := mainRight + excludedGap/2
		c.Line(sepX, top, sepX, bottom, `stroke="#999" stroke-width="0.6"`)
		exAttr := fmt.Sprintf(`fill="%s" stroke="#8a8a8a" stroke-width="0.6"`, ColourFollowingSuppressed)
		x0 := mainRight + excludedGap
		c.Text(x0+float64(len(d.Excluded))*excludedColW/2, axisY, "suppressed",
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisLabelFontPx))
		c.Line(x0, bottom, right, bottom, `stroke="black" stroke-width="1"`)
		for i, e := range d.Excluded {
			cx := x0 + (float64(i)+0.5)*excludedColW
			yy := y.at(e.Share * 100)
			c.Rect(cx-excludedColW/2+3, yy, excludedColW-6, bottom-yy, exAttr)
			c.Text(cx, yy-3, formatShare(e.Share), labelAttr)
			// The reason reads upward from the axis, inside its own column.
			ly := bottom - 4
			c.Text(cx+style.AxisTickFontPx/3, ly, e.Label, fmt.Sprintf(
				`class="%s" font-size="%.1f" transform="rotate(-90 %.4f %.4f)"`,
				TextClassVocab, style.AxisTickFontPx, cx+style.AxisTickFontPx/3, ly))
		}
	}

	c.EndGroup()
	return c.Bytes(), nil
}

// FollowingSeriesPoint is one supported value at T seconds from the start of
// the encounter, with its one-sigma.
type FollowingSeriesPoint struct {
	T, Value, Sigma float64
}

// FollowingPredictedPoint is one review-only predicted gap with its coast
// age.
type FollowingPredictedPoint struct {
	T, Value, Sigma, CoastAgeS float64
}

// FollowingSpan is an interval of the encounter that is not valid following
// time, under a registered reason.
type FollowingSpan struct {
	Start, End float64
	Label      string
}

// FollowingEncounterData is one encounter's evidence: the observed spatial
// gap with its one-sigma band and the review-only predicted gap on the upper
// panel, the valid net time gap and the band thresholds on the lower, and the
// intervals that are not valid following time shaded across both.
type FollowingEncounterData struct {
	Status string
	// Title identifies the encounter and its two tracks; drawn as a ref.
	Title           string
	GapMetric       string
	TimeGapMetric   string
	PredictedMetric string
	// PredictedVisibility is the predicted metric's registry visibility,
	// drawn beside it in the legend.
	PredictedVisibility string
	DurationS           float64
	// BreakAfterS is the longest sample interval a line is drawn across.
	BreakAfterS float64
	Gap         []FollowingSeriesPoint
	TimeGap     []FollowingSeriesPoint
	Predicted   []FollowingPredictedPoint
	Spans       []FollowingSpan
	Thresholds  []Threshold
}

func (d FollowingEncounterData) validate() error {
	if d.Status == "" {
		return errors.New("following chart requires a status label")
	}
	if d.Title == "" || d.GapMetric == "" || d.TimeGapMetric == "" || d.PredictedMetric == "" || d.PredictedVisibility == "" {
		return errors.New("encounter chart requires its title, metric labels and the predicted visibility")
	}
	if !(d.DurationS >= 0) || !finite(d.DurationS) || !(d.BreakAfterS > 0) || !finite(d.BreakAfterS) {
		return errors.New("encounter chart requires a finite duration and a positive break interval")
	}
	checkSeries := func(name string, pts []FollowingSeriesPoint) error {
		for i, p := range pts {
			if !finite(p.T) || !finite(p.Value) || !(p.Sigma >= 0) || !finite(p.Sigma) {
				return fmt.Errorf("%s point %d is not finite", name, i)
			}
			if i > 0 && !(p.T > pts[i-1].T) {
				return fmt.Errorf("%s points must be in time order", name)
			}
		}
		return nil
	}
	if err := checkSeries("gap", d.Gap); err != nil {
		return err
	}
	if err := checkSeries("time gap", d.TimeGap); err != nil {
		return err
	}
	for i, p := range d.Predicted {
		if !finite(p.T) || !finite(p.Value) || !(p.Sigma >= 0) || !finite(p.Sigma) || !(p.CoastAgeS >= 0) {
			return fmt.Errorf("predicted point %d is not finite", i)
		}
		if i > 0 && !(p.T > d.Predicted[i-1].T) {
			return errors.New("predicted points must be in time order")
		}
	}
	for _, s := range d.Spans {
		if s.Label == "" || !(s.End >= s.Start) {
			return errors.New("span requires a label and an ordered interval")
		}
	}
	return nil
}

// RenderFollowingEncounter draws one encounter's gap and time gap over time.
func RenderFollowingEncounter(d FollowingEncounterData, style ChartStyle) ([]byte, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	wPx, hPx := style.WidthMM*pxPerMM, style.HeightMM*pxPerMM
	c := NewCanvas(style.WidthMM, style.HeightMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)
	bannerBottom := statusBanner(c, d.Status, style)
	c.Text(wPx-10, bannerBottom-6, d.Title, fmt.Sprintf(`class="%s" font-size="%.1f" text-anchor="end"`,
		TextClassRef, style.LegendFontPx))

	left, right := 52.0, wPx-44
	legendY := bannerBottom + style.LegendFontPx + 6
	encounterLegend(c, left, legendY, d, style)
	gapTop := legendY + 20
	axisH := style.AxisTickFontPx + style.AxisLabelFontPx + 22
	plotH := hPx - gapTop - axisH - 14
	gapBottom := gapTop + plotH*0.56
	thwTop := gapBottom + 24
	thwBottom := thwTop + plotH*0.44 - 24

	duration := d.DurationS
	if duration == 0 {
		duration = 1
	}
	x := linearScale{d0: 0, d1: duration, r0: left, r1: right}

	spanAttr := fmt.Sprintf(`fill="%s" fill-opacity="0.6"`, ColourFollowingSuppressed)
	for _, s := range d.Spans {
		x0, x1 := x.at(s.Start), x.at(s.End)
		w := math.Max(x1-x0, 1)
		c.Rect(x0, gapTop, w, gapBottom-gapTop, spanAttr)
		c.Rect(x0, thwTop, w, thwBottom-thwTop, spanAttr)
		if w >= style.AxisTickFontPx {
			lx, ly := x0+style.AxisTickFontPx-1, gapBottom-4
			c.Text(lx, ly, s.Label, fmt.Sprintf(`class="%s" font-size="%.1f" fill="#555" transform="rotate(-90 %.4f %.4f)"`,
				TextClassVocab, style.AxisTickFontPx-0.5, lx, ly))
		}
	}

	// Upper panel: spatial gap, observed and predicted.
	gapMax := 0.0
	for _, p := range d.Gap {
		gapMax = math.Max(gapMax, p.Value+p.Sigma)
	}
	for _, p := range d.Predicted {
		gapMax = math.Max(gapMax, p.Value+p.Sigma)
	}
	gy := valueAxis(c, left, right, gapTop, gapBottom, gapMax, style)
	if len(d.Gap) == 0 && len(d.Predicted) == 0 {
		c.Text((left+right)/2, (gapTop+gapBottom)/2, "no supported value",
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	}
	for _, run := range seriesRuns(d.Gap, d.BreakAfterS) {
		band := make([][2]float64, 0, 2*len(run))
		for _, p := range run {
			band = append(band, [2]float64{x.at(p.T), gy.at(p.Value + p.Sigma)})
		}
		for i := len(run) - 1; i >= 0; i-- {
			band = append(band, [2]float64{x.at(run[i].T), gy.at(run[i].Value - run[i].Sigma)})
		}
		c.Polygon(band, fmt.Sprintf(`fill="%s" fill-opacity="0.15" stroke="none"`, ColourFollowingObserved))
		line := make([][2]float64, len(run))
		for i, p := range run {
			line[i] = [2]float64{x.at(p.T), gy.at(p.Value)}
		}
		c.Polyline(line, fmt.Sprintf(`fill="none" stroke="%s" stroke-width="%.2f"`, ColourFollowingObserved, style.LineWidthPx))
		for _, pt := range line {
			c.Circle(pt[0], pt[1], style.MarkerRadiusPx, fmt.Sprintf(`fill="%s"`, ColourFollowingObserved))
		}
	}
	predictedLine := fmt.Sprintf(`fill="none" stroke="%s" stroke-width="%.2f" stroke-dasharray="5 3"`,
		ColourFollowingPredicted, style.LineWidthPx)
	for _, run := range predictedRuns(d.Predicted, d.BreakAfterS) {
		line := make([][2]float64, len(run))
		for i, p := range run {
			line[i] = [2]float64{x.at(p.T), gy.at(p.Value)}
			c.Line(line[i][0], gy.at(p.Value-p.Sigma), line[i][0], gy.at(p.Value+p.Sigma),
				fmt.Sprintf(`stroke="%s" stroke-width="0.8"`, ColourFollowingPredicted))
		}
		c.Polyline(line, predictedLine)
		for _, pt := range line {
			c.Circle(pt[0], pt[1], style.MarkerRadiusPx+0.5,
				fmt.Sprintf(`fill="white" stroke="%s" stroke-width="1"`, ColourFollowingPredicted))
		}
		first, last := run[0], run[len(run)-1]
		age := "coast age " + formatAxisNumber(first.CoastAgeS) + " s"
		if last.CoastAgeS != first.CoastAgeS {
			age = "coast age " + formatAxisNumber(first.CoastAgeS) + " to " + formatAxisNumber(last.CoastAgeS) + " s"
		}
		c.Text(x.at((first.T+last.T)/2), gy.at(last.Value+last.Sigma)-8, age,
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="%s"`, style.AxisTickFontPx, ColourFollowingPredicted))
	}
	c.Line(left, gapTop, left, gapBottom, `stroke="black" stroke-width="1"`)
	c.Line(left, gapBottom, right, gapBottom, `stroke="black" stroke-width="0.6"`)
	c.Text(left-5, gapTop-9, "m", fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))

	// Lower panel: valid net time gap and the band thresholds.
	thwMax := 0.0
	for _, p := range d.TimeGap {
		thwMax = math.Max(thwMax, p.Value+p.Sigma)
	}
	for _, t := range d.Thresholds {
		thwMax = math.Max(thwMax, t.Value)
	}
	ty := valueAxis(c, left, right, thwTop, thwBottom, thwMax, style)
	ruleAttr := fmt.Sprintf(`stroke="%s" stroke-width="0.8" stroke-dasharray="4 3"`, ColourFollowingThreshold)
	for _, t := range d.Thresholds {
		yy := ty.at(t.Value)
		c.Line(left, yy, right, yy, ruleAttr)
		c.Text(right+4, yy+style.AxisTickFontPx/3, t.Label,
			fmt.Sprintf(`font-size="%.1f" fill="%s"`, style.AxisTickFontPx, ColourFollowingThreshold))
	}
	if len(d.TimeGap) == 0 {
		c.Text((left+right)/2, (thwTop+thwBottom)/2, "no supported value",
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	}
	for _, run := range seriesRuns(d.TimeGap, d.BreakAfterS) {
		line := make([][2]float64, len(run))
		for i, p := range run {
			line[i] = [2]float64{x.at(p.T), ty.at(p.Value)}
		}
		c.Polyline(line, fmt.Sprintf(`fill="none" stroke="%s" stroke-width="%.2f"`, ColourFollowingTimeGap, style.LineWidthPx))
		for _, pt := range line {
			c.Circle(pt[0], pt[1], style.MarkerRadiusPx, fmt.Sprintf(`fill="%s"`, ColourFollowingTimeGap))
		}
	}
	c.Line(left, thwTop, left, thwBottom, `stroke="black" stroke-width="1"`)
	c.Line(left, thwBottom, right, thwBottom, `stroke="black" stroke-width="1"`)
	c.Text(left-5, thwTop-9, "s", fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))

	xStep := niceStep(duration, 8)
	tickAttr := fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx)
	for v := 0.0; v <= duration+xStep/1e6; v += xStep {
		xx := x.at(v)
		c.Line(xx, thwBottom, xx, thwBottom+3, `stroke="black" stroke-width="0.5"`)
		c.Text(xx, thwBottom+style.AxisTickFontPx+5, formatAxisNumber(v), tickAttr)
	}
	c.Text((left+right)/2, thwBottom+style.AxisTickFontPx+style.AxisLabelFontPx+14,
		"time since the encounter's first instant (s)",
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisLabelFontPx))

	c.EndGroup()
	return c.Bytes(), nil
}

// encounterLegend draws one row: the observed gap, the predicted gap and its
// visibility, the time gap, and the suppressed shading.
func encounterLegend(c *SVGCanvas, x, y float64, d FollowingEncounterData, style ChartStyle) {
	vocab := fmt.Sprintf(`class="%s" font-size="%.1f"`, TextClassVocab, style.LegendFontPx)
	plain := fmt.Sprintf(`font-size="%.1f"`, style.LegendFontPx)
	swatch := func(attrs string) {
		c.Line(x, y-style.LegendFontPx/3, x+16, y-style.LegendFontPx/3, attrs)
		x += 20
	}
	text := func(s, attrs string) {
		c.Text(x, y, s, attrs)
		x += estimateTextWidth(s, style.LegendFontPx) + 6
	}
	swatch(fmt.Sprintf(`stroke="%s" stroke-width="%.2f"`, ColourFollowingObserved, style.LineWidthPx))
	text(d.GapMetric, vocab)
	x += 6
	swatch(fmt.Sprintf(`stroke="%s" stroke-width="%.2f" stroke-dasharray="5 3"`, ColourFollowingPredicted, style.LineWidthPx))
	text(d.PredictedMetric, vocab)
	text(d.PredictedVisibility, vocab)
	x += 6
	swatch(fmt.Sprintf(`stroke="%s" stroke-width="%.2f"`, ColourFollowingTimeGap, style.LineWidthPx))
	text(d.TimeGapMetric, vocab)
	x += 6
	c.Rect(x, y-style.LegendFontPx*0.75, 12, style.LegendFontPx*0.75,
		fmt.Sprintf(`fill="%s" fill-opacity="0.6"`, ColourFollowingSuppressed))
	x += 16
	text("suppressed", plain)
}

// valueAxis draws a zero-based value axis with nice ticks and faint grid
// lines, and returns its scale.
func valueAxis(c *SVGCanvas, left, right, top, bottom, maxValue float64, style ChartStyle) linearScale {
	if !(maxValue > 0) {
		maxValue = 1
	}
	step := niceStep(maxValue, 4)
	yMax := math.Ceil(maxValue/step) * step
	y := linearScale{d0: 0, d1: yMax, r0: bottom, r1: top}
	attr := fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx)
	for v := 0.0; v <= yMax+step/1e6; v += step {
		yy := y.at(v)
		c.Line(left-3, yy, right, yy, `stroke="#e6e6e6" stroke-width="0.5"`)
		c.Text(left-5, yy+style.AxisTickFontPx/3, formatAxisNumber(v), attr)
	}
	return y
}

// statusBanner draws the report status in a box at the top left and returns
// the banner's lower edge.
func statusBanner(c *SVGCanvas, status string, style ChartStyle) float64 {
	fontPx := style.AxisLabelFontPx + 1
	w := estimateTextWidth(status, fontPx)*1.25 + 12
	h := fontPx + 8
	c.Rect(4, 4, w, h, `fill="white" stroke="black" stroke-width="1.2"`)
	c.Text(10, 4+h-5, status, fmt.Sprintf(`class="%s" font-size="%.1f" font-weight="bold"`, TextClassStatus, fontPx))
	return 4 + h
}

// axisLabel draws a metric id, verbatim, and its unit after it.
func axisLabel(c *SVGCanvas, cx, y float64, metric, unit string, fontPx float64) {
	w := estimateTextWidth(metric, fontPx)
	c.Text(cx-w/2, y, metric, fmt.Sprintf(`class="%s" font-size="%.1f"`, TextClassVocab, fontPx))
	c.Text(cx+w/2+4, y, "("+unit+")", fmt.Sprintf(`font-size="%.1f"`, fontPx))
}

// estimateTextWidth is the width of a label in Atkinson Hyperlegible at
// fontPx, measured on the snake-case ids these charts draw. It is tighter
// than estimateLegendLabelWidth, which is sized for mixed-case prose.
func estimateTextWidth(label string, fontPx float64) float64 {
	return float64(len(label)) * 0.47 * fontPx
}

// seriesRuns splits a series wherever consecutive points are more than
// breakAfter apart, so no line crosses an interval with no supported value.
func seriesRuns(pts []FollowingSeriesPoint, breakAfter float64) [][]FollowingSeriesPoint {
	var runs [][]FollowingSeriesPoint
	start := 0
	for i := 1; i <= len(pts); i++ {
		if i == len(pts) || pts[i].T-pts[i-1].T > breakAfter {
			if i > start {
				runs = append(runs, pts[start:i])
			}
			start = i
		}
	}
	return runs
}

func predictedRuns(pts []FollowingPredictedPoint, breakAfter float64) [][]FollowingPredictedPoint {
	var runs [][]FollowingPredictedPoint
	start := 0
	for i := 1; i <= len(pts); i++ {
		if i == len(pts) || pts[i].T-pts[i-1].T > breakAfter {
			if i > start {
				runs = append(runs, pts[start:i])
			}
			start = i
		}
	}
	return runs
}

// linearScale maps a domain onto a pixel range.
type linearScale struct{ d0, d1, r0, r1 float64 }

func (s linearScale) at(v float64) float64 {
	if s.d1 == s.d0 {
		return s.r0
	}
	return s.r0 + (v-s.d0)/(s.d1-s.d0)*(s.r1-s.r0)
}

// formatAxisNumber writes a tick value without trailing zeros, rounded to
// six decimals so accumulated steps do not print as 0.30000000000000004.
func formatAxisNumber(v float64) string {
	r := math.Round(v*1e6) / 1e6
	if r == 0 {
		r = 0 // drop a negative zero
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// formatShare writes a share of one as a percentage with one decimal.
func formatShare(share float64) string {
	return strconv.FormatFloat(share*100, 'f', 1, 64) + "%"
}

// onStep reports whether v is a whole number of steps, within rounding.
func onStep(v, step float64) bool {
	if !(step > 0) {
		return false
	}
	k := math.Round(v / step)
	return math.Abs(v-k*step) <= 1e-9*math.Max(1, math.Abs(v))
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
