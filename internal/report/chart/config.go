package chart

// ChartStyle holds visual parameters for chart rendering.
type ChartStyle struct {
	WidthMM               float64
	HeightMM              float64
	ColourP50             string
	ColourP85             string
	ColourP98             string
	ColourMax             string
	ColourCountBar        string
	ColourLowSample       string
	CountMissingThreshold int
	LowSampleThreshold    int
	CountAxisScale        float64
	BarWidthFraction      float64
	BarWidthBGFraction    float64
	LineWidthPx           float64
	MarkerRadiusPx        float64
	AxisLabelFontPx       float64
	AxisTickFontPx        float64
	LegendFontPx          float64
}

// DefaultTimeSeriesStyle returns chart parameters matching the Python PDF generator's
// time-series chart configuration.
func DefaultTimeSeriesStyle() ChartStyle {
	return ChartStyle{
		WidthMM:               609.6, // 24 inches
		HeightMM:              203.2, // 8 inches
		ColourP50:             ColourP50,
		ColourP85:             ColourP85,
		ColourP98:             ColourP98,
		ColourMax:             ColourMax,
		ColourCountBar:        ColourCountBar,
		ColourLowSample:       ColourLowSample,
		CountMissingThreshold: 5,
		LowSampleThreshold:    50,
		CountAxisScale:        1.6,
		BarWidthFraction:      0.7,
		BarWidthBGFraction:    0.95,
		LineWidthPx:           1.0,
		MarkerRadiusPx:        4.0,
		AxisLabelFontPx:       8.0,
		AxisTickFontPx:        7.0,
		LegendFontPx:          7.0,
	}
}

// DefaultHistogramStyle returns chart parameters for histogram rendering.
func DefaultHistogramStyle() ChartStyle {
	return ChartStyle{
		WidthMM:          76.2, // 3 inches
		HeightMM:         50.8, // 2 inches
		BarWidthFraction: 0.7,
		AxisLabelFontPx:  13.0,
		AxisTickFontPx:   11.0,
		LegendFontPx:     7.0,
	}
}
