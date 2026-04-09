package chart

import (
	"fmt"
	"sort"
)

// HistogramData holds the data for a single histogram chart.
type HistogramData struct {
	Buckets   map[float64]int64 // bucket start → count
	Units     string            // "mph" or "kph"
	BucketSz  float64           // e.g. 5.0
	MaxBucket float64           // values >= this merge into "N+" bucket
	Cutoff    float64           // values below this are ignored in display
}

// NormaliseHistogram returns sorted bucket keys, corresponding counts,
// and the grand total.
func NormaliseHistogram(buckets map[float64]int64) (keys []float64, counts []int64, total int64) {
	if len(buckets) == 0 {
		return nil, nil, 0
	}
	keys = make([]float64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	counts = make([]int64, len(keys))
	for i, k := range keys {
		counts[i] = buckets[k]
		total += buckets[k]
	}
	return keys, counts, total
}

// BucketLabel returns a display label like "20-25" or "70+".
func BucketLabel(lo, hi, maxBucket float64) string {
	if lo >= maxBucket {
		return fmt.Sprintf("%.0f+", lo)
	}
	return fmt.Sprintf("%.0f-%.0f", lo, hi)
}

// RenderHistogram produces an SVG bar chart from histogram data.
func RenderHistogram(data HistogramData, style ChartStyle) ([]byte, error) {
	keys, counts, _ := NormaliseHistogram(data.Buckets)
	if len(keys) == 0 {
		return renderNoData(style), nil
	}

	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM

	c := NewCanvas(style.WidthMM, style.HeightMM)

	// Layout margins (fraction of total).
	leftM := 0.15 * wPx
	rightM := 0.95 * wPx
	topM := 0.05 * hPx
	bottomM := 0.80 * hPx

	plotW := rightM - leftM
	plotH := bottomM - topM

	n := len(keys)
	barSlot := plotW / float64(n)
	barW := barSlot * style.BarWidthFraction
	if barW < 1 {
		barW = 1
	}

	var maxCount int64
	for _, cnt := range counts {
		if cnt > maxCount {
			maxCount = cnt
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	yScale := plotH / float64(maxCount)

	// Draw bars.
	for i, cnt := range counts {
		barH := float64(cnt) * yScale
		x := leftM + float64(i)*barSlot + (barSlot-barW)/2
		y := bottomM - barH
		c.Rect(x, y, barW, barH,
			fmt.Sprintf(`fill="%s" fill-opacity="0.7" stroke="black" stroke-width="0.5"`, ColourSteelBlue))
	}

	// X-axis labels.
	rotateLabels := n > 20
	for i, k := range keys {
		hi := k + data.BucketSz
		label := BucketLabel(k, hi, data.MaxBucket)
		x := leftM + float64(i)*barSlot + barSlot/2
		y := bottomM + style.AxisTickFontPx + 4

		attrs := fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx)
		if rotateLabels {
			attrs = fmt.Sprintf(
				`font-size="%.1f" text-anchor="end" transform="rotate(-45,%.4f,%.4f)"`,
				style.AxisTickFontPx, x, y)
		}
		c.Text(x, y, label, attrs)
	}

	// Y-axis: a few tick marks.
	nTicks := 5
	for i := range nTicks + 1 {
		val := float64(maxCount) * float64(i) / float64(nTicks)
		y := bottomM - val*yScale
		c.Line(leftM-3, y, leftM, y, `stroke="black" stroke-width="0.5"`)
		c.Text(leftM-5, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f", val),
			fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))
	}

	// Axes lines.
	c.Line(leftM, topM, leftM, bottomM, `stroke="black" stroke-width="1"`)
	c.Line(leftM, bottomM, rightM, bottomM, `stroke="black" stroke-width="1"`)

	// Axis labels.
	c.Text((leftM+rightM)/2, hPx-2,
		fmt.Sprintf("Speed (%s)", data.Units),
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisLabelFontPx))

	return c.Bytes(), nil
}

// RenderComparison produces a grouped bar chart comparing two histograms.
func RenderComparison(primary, compare HistogramData, primaryLabel, compareLabel string, style ChartStyle) ([]byte, error) {
	pKeys, pCounts, pTotal := NormaliseHistogram(primary.Buckets)
	cKeys, cCounts, cTotal := NormaliseHistogram(compare.Buckets)

	// Merge all bucket keys.
	keySet := make(map[float64]struct{})
	for _, k := range pKeys {
		keySet[k] = struct{}{}
	}
	for _, k := range cKeys {
		keySet[k] = struct{}{}
	}
	if len(keySet) == 0 {
		return renderNoData(style), nil
	}

	allKeys := make([]float64, 0, len(keySet))
	for k := range keySet {
		allKeys = append(allKeys, k)
	}
	sort.Float64s(allKeys)

	// Build percentage maps.
	pPct := make(map[float64]float64, len(pKeys))
	for i, k := range pKeys {
		if pTotal > 0 {
			pPct[k] = float64(pCounts[i]) / float64(pTotal) * 100
		}
	}
	cPct := make(map[float64]float64, len(cKeys))
	for i, k := range cKeys {
		if cTotal > 0 {
			cPct[k] = float64(cCounts[i]) / float64(cTotal) * 100
		}
	}

	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM
	c := NewCanvas(style.WidthMM, style.HeightMM)

	leftM := 0.15 * wPx
	rightM := 0.95 * wPx
	topM := 0.05 * hPx
	bottomM := 0.75 * hPx

	plotW := rightM - leftM
	plotH := bottomM - topM

	n := len(allKeys)
	slotW := plotW / float64(n)
	barW := slotW * 0.4

	// Find max percentage for Y-scale.
	var maxPct float64
	for _, v := range pPct {
		if v > maxPct {
			maxPct = v
		}
	}
	for _, v := range cPct {
		if v > maxPct {
			maxPct = v
		}
	}
	if maxPct == 0 {
		maxPct = 1
	}
	yScale := plotH / maxPct

	bucketSz := primary.BucketSz
	if bucketSz == 0 {
		bucketSz = compare.BucketSz
	}
	maxBucket := primary.MaxBucket
	if maxBucket == 0 {
		maxBucket = compare.MaxBucket
	}

	for i, k := range allKeys {
		slotX := leftM + float64(i)*slotW

		// Primary bar (left).
		pVal := pPct[k]
		pH := pVal * yScale
		px := slotX + (slotW/2-barW)/2
		py := bottomM - pH
		c.Rect(px, py, barW, pH,
			fmt.Sprintf(`fill="%s" fill-opacity="0.75" stroke="black" stroke-width="0.5"`, ColourP50))

		// Compare bar (right).
		cVal := cPct[k]
		cH := cVal * yScale
		cx := slotX + slotW/2 + (slotW/2-barW)/2
		cy := bottomM - cH
		c.Rect(cx, cy, barW, cH,
			fmt.Sprintf(`fill="%s" fill-opacity="0.75" stroke="black" stroke-width="0.5"`, ColourP98))

		// X label.
		hi := k + bucketSz
		label := BucketLabel(k, hi, maxBucket)
		c.Text(slotX+slotW/2, bottomM+style.AxisTickFontPx+4, label,
			fmt.Sprintf(`font-size="%.1f" text-anchor="middle"`, style.AxisTickFontPx))
	}

	// Axes.
	c.Line(leftM, topM, leftM, bottomM, `stroke="black" stroke-width="1"`)
	c.Line(leftM, bottomM, rightM, bottomM, `stroke="black" stroke-width="1"`)

	// Y-axis ticks.
	nTicks := 5
	for i := range nTicks + 1 {
		val := maxPct * float64(i) / float64(nTicks)
		y := bottomM - val*yScale
		c.Line(leftM-3, y, leftM, y, `stroke="black" stroke-width="0.5"`)
		c.Text(leftM-5, y+style.AxisTickFontPx/3,
			fmt.Sprintf("%.0f%%", val),
			fmt.Sprintf(`font-size="%.1f" text-anchor="end"`, style.AxisTickFontPx))
	}

	// Legend.
	legY := bottomM + 30
	legX := leftM
	c.Rect(legX, legY, 10, 10, fmt.Sprintf(`fill="%s" fill-opacity="0.75"`, ColourP50))
	c.Text(legX+14, legY+9, primaryLabel, fmt.Sprintf(`font-size="%.1f"`, style.LegendFontPx))
	c.Rect(legX+80, legY, 10, 10, fmt.Sprintf(`fill="%s" fill-opacity="0.75"`, ColourP98))
	c.Text(legX+94, legY+9, compareLabel, fmt.Sprintf(`font-size="%.1f"`, style.LegendFontPx))

	return c.Bytes(), nil
}

func renderNoData(style ChartStyle) []byte {
	c := NewCanvas(style.WidthMM, style.HeightMM)
	wPx := style.WidthMM * pxPerMM
	hPx := style.HeightMM * pxPerMM
	c.Text(wPx/2, hPx/2, "No data",
		fmt.Sprintf(`font-size="%.1f" text-anchor="middle" fill="gray"`, style.AxisLabelFontPx))
	return c.Bytes()
}
