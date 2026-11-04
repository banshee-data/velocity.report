package chart

import (
	"fmt"
	"math"
	"sort"
)

// MirroredSide is one direction of a mirrored histogram: its buckets, its
// label and its limit in the chart's units (0: none).
type MirroredSide struct {
	Data  HistogramData
	Label string
	Limit float64
}

// RenderMirrored draws two directions' histograms on one speed axis
// (docs/plans/platform-speed-by-direction-reporting-plan.md, Item 2): the
// first upward and the second downward from the axis, each as shares of its
// own population on one shared ceiling, each with its own limit line and its
// buckets above that limit shaded darker. Both sides must share units and
// bucket size.
// mirroredAxis is the shared speed axis: every bucket from the lowest to the
// highest either side holds, with none skipped, widened to take in each
// side's limit, so a bucket's place on the axis is linear in its speed and a
// limit below or above every populated bucket still has its line.
func mirroredAxis(up, down MirroredSide) []float64 {
	bucket := up.Data.BucketSz
	set := map[float64]bool{}
	for k := range up.Data.Buckets {
		set[k] = true
	}
	for k := range down.Data.Buckets {
		set[k] = true
	}
	keys := make([]float64, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	if bucket <= 0 || len(keys) == 0 {
		return keys
	}
	lo, hi := keys[0], keys[len(keys)-1]
	for _, l := range []float64{up.Limit, down.Limit} {
		if l <= 0 {
			continue
		}
		b := math.Floor(l/bucket) * bucket
		lo, hi = math.Min(lo, b), math.Max(hi, b)
	}
	n := int(math.Round((hi-lo)/bucket)) + 1
	if n > 1000 {
		return keys
	}
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, lo+float64(i)*bucket)
	}
	return out
}

func RenderMirrored(up, down MirroredSide, style ChartStyle) ([]byte, error) {
	if up.Data.Units != down.Data.Units || up.Data.BucketSz != down.Data.BucketSz {
		return nil, fmt.Errorf("mirrored histogram: both sides need the same units and bucket size")
	}
	_, _, upTotal := NormaliseHistogram(up.Data.Buckets)
	_, _, downTotal := NormaliseHistogram(down.Data.Buckets)
	if upTotal == 0 && downTotal == 0 {
		return renderNoData(style), nil
	}
	keys := mirroredAxis(up, down)
	pct := func(side MirroredSide, total int64, k float64) float64 {
		if total == 0 {
			return 0
		}
		return 100 * float64(side.Data.Buckets[k]) / float64(total)
	}
	maxPct := 0.0
	for _, k := range keys {
		maxPct = math.Max(maxPct, math.Max(pct(up, upTotal, k), pct(down, downTotal, k)))
	}
	step := pctNiceStep(maxPct)
	ceiling := math.Max(step, math.Ceil(maxPct/step)*step)

	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM
	c := NewCanvas(style.WidthMM, style.HeightMM)
	c.EmbedFont("Atkinson Hyperlegible", AtkinsonRegularBase64())
	c.BeginGroup(`font-family="Atkinson Hyperlegible"`)

	leftM, rightM := 0.15*wPx, 0.95*wPx
	topM, bottomM := 0.06*hPx, 0.82*hPx
	axisY := (topM + bottomM) / 2
	half := axisY - topM
	plotW := rightM - leftM
	slot := plotW / float64(len(keys))
	barW := math.Max(1, slot*style.BarWidthFraction)
	scale := half / ceiling
	bucket := up.Data.BucketSz

	draw := func(side MirroredSide, total int64, sign float64, colour string) {
		for i, k := range keys {
			h := pct(side, total, k) * scale
			if h <= 0 {
				continue
			}
			x := leftM + float64(i)*slot + (slot-barW)/2
			y := axisY
			if sign > 0 {
				y = axisY - h
			}
			opacity := "0.55"
			if side.Limit > 0 && k >= side.Limit {
				opacity = "0.95"
			}
			c.Rect(x, y, barW, h, fmt.Sprintf(`fill="%s" fill-opacity="%s" stroke="black" stroke-width="0.4"`, colour, opacity))
		}
		if side.Limit > 0 && bucket > 0 && len(keys) > 0 {
			// The limit's place on the bucket axis.
			pos := (side.Limit - keys[0]) / bucket
			x := leftM + pos*slot
			if x >= leftM && x <= rightM {
				y0, y1 := axisY, topM
				if sign < 0 {
					y1 = bottomM
				}
				c.Line(x, y0, x, y1, fmt.Sprintf(`stroke="%s" stroke-width="1.2" stroke-dasharray="3,2"`, colour))
				ly := y1 + 8
				if sign > 0 {
					ly = y1 + style.AxisTickFontPx
				} else {
					ly = y1 - 3
				}
				c.Text(x+3, ly, fmt.Sprintf("%.0f %s", side.Limit, side.Data.Units),
					fmt.Sprintf(`font-size="%.1f" fill="%s"`, style.AxisTickFontPx, colour))
			}
		}
	}
	draw(up, upTotal, 1, ColourSteelBlue)
	draw(down, downTotal, -1, ColourOrange)

	// The shared axis, bucket labels under the lower half.
	c.Line(leftM, axisY, rightM, axisY, `stroke="black" stroke-width="1"`)
	for i, k := range keys {
		x := leftM + float64(i)*slot + slot/2
		y := bottomM + style.AxisTickFontPx + 6
		c.Text(x, y, BucketLabel(k, k+bucket, up.Data.MaxBucket),
			fmt.Sprintf(`font-size="%.1f" text-anchor="end" transform="rotate(-45,%.4f,%.4f)"`, style.AxisTickFontPx, x, y))
	}
	// Shares either way from the axis, on one ceiling.
	c.Line(leftM, topM, leftM, bottomM, `stroke="black" stroke-width="1"`)
	for v := 0.0; v <= ceiling+0.01; v += step {
		for _, s := range []float64{1, -1} {
			if v == 0 && s < 0 {
				continue
			}
			y := axisY - s*v*scale
			c.Line(leftM-3, y, leftM, y, `stroke="black" stroke-width="0.5"`)
			c.Text(leftM-5, y+style.AxisTickFontPx/3, fmt.Sprintf("%.0f%%", v),
				fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))
		}
	}
	c.Text(leftM+4, topM+style.AxisLabelFontPx, fmt.Sprintf("▲ %s (n=%d)", up.Label, upTotal),
		fmt.Sprintf(`font-size="%.1f" fill="%s"`, style.AxisLabelFontPx, ColourSteelBlue))
	c.Text(leftM+4, bottomM-3, fmt.Sprintf("▼ %s (n=%d)", down.Label, downTotal),
		fmt.Sprintf(`font-size="%.1f" fill="%s"`, style.AxisLabelFontPx, ColourOrange))
	c.Text((leftM+rightM)/2, hPx-2, fmt.Sprintf("Speed (%s)", up.Data.Units),
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisLabelFontPx))
	c.EndGroup()
	return c.Bytes(), nil
}
