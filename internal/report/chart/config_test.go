package chart

import "testing"

func TestDefaultTimeSeriesStyle(t *testing.T) {
	s := DefaultTimeSeriesStyle(PaperA4)
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

func TestDefaultWebTimeSeriesStyle(t *testing.T) {
	pdf := DefaultTimeSeriesStyle(PaperA4)
	web := DefaultWebTimeSeriesStyle()
	if web.WidthMM == 0 || web.HeightMM == 0 {
		t.Fatal("web time-series dimensions are zero")
	}
	if web.AxisTickFontPx <= pdf.AxisTickFontPx {
		t.Fatalf("web AxisTickFontPx = %v, want greater than pdf AxisTickFontPx = %v", web.AxisTickFontPx, pdf.AxisTickFontPx)
	}
	if web.LineWidthPx <= pdf.LineWidthPx {
		t.Fatalf("web LineWidthPx = %v, want greater than pdf LineWidthPx = %v", web.LineWidthPx, pdf.LineWidthPx)
	}
}

func TestDefaultHistogramStyle(t *testing.T) {
	s := DefaultHistogramStyle(PaperA4)
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

func TestDefaultWebHistogramStyle(t *testing.T) {
	pdf := DefaultHistogramStyle(PaperA4)
	web := DefaultWebHistogramStyle()
	if web.WidthMM == 0 || web.HeightMM == 0 {
		t.Fatal("web histogram dimensions are zero")
	}
	if web.WidthMM <= pdf.WidthMM {
		t.Fatalf("web WidthMM = %v, want greater than pdf WidthMM = %v", web.WidthMM, pdf.WidthMM)
	}
	if web.AxisTickFontPx <= pdf.AxisTickFontPx {
		t.Fatalf("web AxisTickFontPx = %v, want greater than pdf AxisTickFontPx = %v", web.AxisTickFontPx, pdf.AxisTickFontPx)
	}
}
