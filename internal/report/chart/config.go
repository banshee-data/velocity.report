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

// DefaultWebTimeSeriesStyle returns a browser-sized style for SVG endpoints.
func DefaultWebTimeSeriesStyle() ChartStyle {
	return ChartStyle{
		WidthMM:               215.9, // 8.5 inches -> 816 px
		HeightMM:              88.9,  // 3.5 inches -> 336 px
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
		LineWidthPx:           2.0,
		MarkerRadiusPx:        5.0,
		AxisLabelFontPx:       16.0,
		AxisTickFontPx:        13.0,
		LegendFontPx:          12.0,
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

// DefaultWebHistogramStyle returns a browser-sized histogram style for SVG endpoints.
func DefaultWebHistogramStyle() ChartStyle {
	return ChartStyle{
		WidthMM:          127.0, // 5 inches -> 480 px
		HeightMM:         76.2,  // 3 inches -> 288 px
		BarWidthFraction: 0.7,
		AxisLabelFontPx:  16.0,
		AxisTickFontPx:   13.0,
		LegendFontPx:     12.0,
	}
}
