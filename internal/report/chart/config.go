package chart

// PaperSize is a supported printed output size.
type PaperSize string

const (
	PaperA4     PaperSize = "a4"
	PaperLetter PaperSize = "letter"
)

// NormalisePaperSize returns a canonical PaperSize for common spellings.
// Unknown and empty values fall back to Letter, matching the Python report
// standard used for printed PDFs.
func NormalisePaperSize(s string) PaperSize {
	switch s {
	case "a4", "A4", "a4paper", "A4Paper", "A4PAPER":
		return PaperA4
	case "letter", "LETTER", "Letter", "us-letter", "us_letter":
		return PaperLetter
	default:
		return PaperLetter
	}
}

// paperTextWidthMM returns the textwidth that matches preamble.tex geometry
// (1.0 cm left + 1.0 cm right margins).
func paperTextWidthMM(p PaperSize) float64 {
	switch p {
	case PaperLetter:
		// 8.5 in = 215.9 mm, minus 2 cm of margin.
		return 215.9 - 20.0
	default:
		// A4 width 210 mm, minus 2 cm of margin.
		return 210.0 - 20.0
	}
}

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

// baseStyle returns the shared percentile palette and thresholds.
func baseStyle() ChartStyle {
	return ChartStyle{
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
	}
}

// DefaultTimeSeriesStyle returns chart parameters sized to fit the textwidth
// of the given paper size. Chart is rendered at true physical dimensions so
// LaTeX does not need to scale it further — this keeps font sizes legible.
func DefaultTimeSeriesStyle(paper PaperSize) ChartStyle {
	s := baseStyle()
	// Physical output: width matches textwidth; height chosen for a pleasant
	// 2.7:1 aspect so axis labels and legend fit without crowding.
	s.WidthMM = paperTextWidthMM(paper)
	s.HeightMM = s.WidthMM / 2.7
	// Fonts in px-at-96-DPI. 10 px ≈ 2.65 mm ≈ 7.5 pt at physical size.
	s.AxisLabelFontPx = 11.0
	s.AxisTickFontPx = 9.5
	s.LegendFontPx = 10.0
	s.LineWidthPx = 1.2
	s.MarkerRadiusPx = 3.0
	return s
}

// DefaultHistogramStyle returns histogram parameters sized for the given paper.
// Two histograms (primary + optional comparison) fit side-by-side in a
// two-column layout, so we size each to roughly half the column width.
func DefaultHistogramStyle(paper PaperSize) ChartStyle {
	s := baseStyle()
	colW := paperTextWidthMM(paper) / 2.0 // each text column
	s.WidthMM = colW
	s.HeightMM = colW * 0.55
	s.AxisLabelFontPx = 10.0
	s.AxisTickFontPx = 8.5
	s.LegendFontPx = 9.0
	s.LineWidthPx = 0.5
	return s
}

// DefaultComparisonHistogramStyle returns parameters for the grouped
// comparison histogram. It is rendered full-width in the overview section
// (scaled by LaTeX to \linewidth), so it is taller than the single histogram
// to accommodate the x-axis label and legend box without crowding.
func DefaultComparisonHistogramStyle(paper PaperSize) ChartStyle {
	s := DefaultHistogramStyle(paper)
	s.HeightMM = s.WidthMM * 0.70
	return s
}
