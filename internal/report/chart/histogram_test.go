package chart

import (
	"bytes"
	"encoding/xml"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestNormaliseHistogram_Empty(t *testing.T) {
	keys, counts, total := NormaliseHistogram(nil)
	if keys != nil || counts != nil || total != 0 {
		t.Errorf("expected nil/nil/0, got %v/%v/%d", keys, counts, total)
	}
}

func TestNormaliseHistogram_Single(t *testing.T) {
	keys, counts, total := NormaliseHistogram(map[float64]int64{10: 5})
	if len(keys) != 1 || keys[0] != 10 {
		t.Errorf("keys = %v, want [10]", keys)
	}
	if len(counts) != 1 || counts[0] != 5 {
		t.Errorf("counts = %v, want [5]", counts)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
}

func TestNormaliseHistogram_Multi(t *testing.T) {
	keys, counts, total := NormaliseHistogram(map[float64]int64{
		20: 10,
		10: 30,
		30: 5,
	})
	if len(keys) != 3 {
		t.Fatalf("len(keys) = %d, want 3", len(keys))
	}
	// Must be sorted.
	if keys[0] != 10 || keys[1] != 20 || keys[2] != 30 {
		t.Errorf("keys not sorted: %v", keys)
	}
	if counts[0] != 30 || counts[1] != 10 || counts[2] != 5 {
		t.Errorf("counts mismatch: %v", counts)
	}
	if total != 45 {
		t.Errorf("total = %d, want 45", total)
	}
}

func TestBucketLabel(t *testing.T) {
	if got := BucketLabel(20, 25, 70); got != "20-25" {
		t.Errorf("BucketLabel(20,25,70) = %q, want '20-25'", got)
	}
	if got := BucketLabel(70, 75, 70); got != "70+" {
		t.Errorf("BucketLabel(70,75,70) = %q, want '70+'", got)
	}
}

func TestRenderHistogram_Structure(t *testing.T) {
	data := HistogramData{
		Buckets:   map[float64]int64{10: 20, 15: 30, 20: 10},
		Units:     "mph",
		BucketSz:  5,
		MaxBucket: 50,
		Cutoff:    5,
	}
	svg, err := RenderHistogram(data, DefaultHistogramStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderHistogram error: %v", err)
	}

	rectCount := countElements(t, svg, "rect")
	if rectCount != 3 {
		t.Errorf("rect count = %d, want 3 (one per bucket)", rectCount)
	}
}

func TestRenderHistogram_Empty(t *testing.T) {
	data := HistogramData{
		Buckets:   nil,
		Units:     "mph",
		BucketSz:  5,
		MaxBucket: 50,
	}
	svg, err := RenderHistogram(data, DefaultHistogramStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderHistogram error: %v", err)
	}
	if !strings.Contains(string(svg), "No data") {
		t.Error("empty histogram should contain 'No data'")
	}
}

func TestRenderComparison_Structure(t *testing.T) {
	primary := HistogramData{
		Buckets:   map[float64]int64{10: 20, 15: 30},
		Units:     "mph",
		BucketSz:  5,
		MaxBucket: 50,
	}
	compare := HistogramData{
		Buckets:   map[float64]int64{10: 15, 15: 25},
		Units:     "mph",
		BucketSz:  5,
		MaxBucket: 50,
	}
	svg, err := RenderComparison(primary, compare, "Period A", "Period B", DefaultHistogramStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderComparison error: %v", err)
	}

	rectCount := countElements(t, svg, "rect")
	// 2 bars per bucket (primary + compare) = 4, plus 2 legend rects = 6.
	if rectCount < 4 {
		t.Errorf("rect count = %d, want >= 4 (2 per bucket)", rectCount)
	}
}

func TestRenderComparison_BarsTouchWithinBucketAndHasXTicks(t *testing.T) {
	primary := HistogramData{
		Buckets:   map[float64]int64{10: 20, 15: 30},
		Units:     "mph",
		BucketSz:  5,
		MaxBucket: 50,
	}
	compare := HistogramData{
		Buckets:   map[float64]int64{10: 15, 15: 25},
		Units:     "mph",
		BucketSz:  5,
		MaxBucket: 50,
	}
	svg, err := RenderComparison(primary, compare, "Period A", "Period B", DefaultHistogramStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderComparison error: %v", err)
	}

	bars := comparisonBars(t, svg)
	if len(bars) < 2 {
		t.Fatalf("expected comparison bars, got %d", len(bars))
	}
	if gap := bars[1].x - (bars[0].x + bars[0].width); math.Abs(gap) > 0.001 {
		t.Fatalf("first bucket series bars should touch, gap = %.4f", gap)
	}
	if got := countShortVerticalTickLines(t, svg); got < 2 {
		t.Fatalf("expected x-axis tick marks for bucket labels, got %d", got)
	}
}

// countElements parses SVG XML and counts elements with the given local name.
func countElements(t *testing.T, svg []byte, name string) int {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	count := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == name {
			count++
		}
	}
	return count
}

type barRect struct {
	x     float64
	width float64
}

func comparisonBars(t *testing.T, svg []byte) []barRect {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var bars []barRect
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "rect" {
			continue
		}
		attrs := attrsByName(se.Attr)
		fill := attrs["fill"]
		if fill != ColourP50 && fill != ColourP98 {
			continue
		}
		width := parseFloatAttr(t, attrs, "width")
		height := parseFloatAttr(t, attrs, "height")
		if width <= 10 || height <= 10 {
			continue
		}
		bars = append(bars, barRect{x: parseFloatAttr(t, attrs, "x"), width: width})
	}
	return bars
}

func countShortVerticalTickLines(t *testing.T, svg []byte) int {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	count := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "line" {
			continue
		}
		attrs := attrsByName(se.Attr)
		if attrs["stroke"] != "black" || attrs["stroke-width"] != "0.5" {
			continue
		}
		x1 := parseFloatAttr(t, attrs, "x1")
		x2 := parseFloatAttr(t, attrs, "x2")
		y1 := parseFloatAttr(t, attrs, "y1")
		y2 := parseFloatAttr(t, attrs, "y2")
		if math.Abs(x1-x2) < 0.001 && math.Abs((y2-y1)-3) < 0.001 {
			count++
		}
	}
	return count
}

func attrsByName(attrs []xml.Attr) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		out[attr.Name.Local] = attr.Value
	}
	return out
}

func parseFloatAttr(t *testing.T, attrs map[string]string, name string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(attrs[name], 64)
	if err != nil {
		t.Fatalf("parse %s=%q: %v", name, attrs[name], err)
	}
	return v
}
