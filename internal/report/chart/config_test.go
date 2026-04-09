package chart
package chart

import "testing"

func TestDefaultTimeSeriesStyle(t *testing.T) {
	s := DefaultTimeSeriesStyle()
	if s.WidthMM == 0 {
		t.Error("WidthMM is zero")
	}
	if s.HeightMM == 0 {
		t.Error("HeightMM is zero")
	}
	if s.ColourP50 == "" {
		t.Error("ColourP50 is empty")
	}
	if s.ColourP85 == "" {
		t.Error("ColourP85 is empty")
	}
	if s.CountMissingThreshold == 0 {
		t.Error("CountMissingThreshold is zero")
	}
	if s.LowSampleThreshold == 0 {
		t.Error("LowSampleThreshold is zero")
	}
	if s.BarWidthFraction == 0 {
		t.Error("BarWidthFraction is zero")
	}
	if s.LineWidthPx == 0 {
		t.Error("LineWidthPx is zero")
	}
	if s.AxisLabelFontPx == 0 {
		t.Error("AxisLabelFontPx is zero")
	}
}

func TestDefaultHistogramStyle(t *testing.T) {
	s := DefaultHistogramStyle()
	if s.WidthMM == 0 {
		t.Error("WidthMM is zero")
	}
	if s.HeightMM == 0 {
		t.Error("HeightMM is zero")
	}
	if s.BarWidthFraction == 0 {
		t.Error("BarWidthFraction is zero")
	}
	if s.AxisLabelFontPx == 0 {
		t.Error("AxisLabelFontPx is zero")
	}
}
