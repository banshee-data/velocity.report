package chart

import (
	"bytes"
	"encoding/xml"
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
	svg, err := RenderHistogram(data, DefaultHistogramStyle())
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
	svg, err := RenderHistogram(data, DefaultHistogramStyle())
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
	svg, err := RenderComparison(primary, compare, "Period A", "Period B", DefaultHistogramStyle())
	if err != nil {
		t.Fatalf("RenderComparison error: %v", err)
	}

	rectCount := countElements(t, svg, "rect")
	// 2 bars per bucket (primary + compare) = 4, plus 2 legend rects = 6.
	if rectCount < 4 {
		t.Errorf("rect count = %d, want >= 4 (2 per bucket)", rectCount)
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
