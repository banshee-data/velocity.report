package chart
package chart

import (
	"bytes"
	"encoding/xml"
	"math"
	"strings"
	"testing"
	"time"
)

func makeTestPoints(n int, startTime time.Time, interval time.Duration) []TimeSeriesPoint {
	pts := make([]TimeSeriesPoint, n)
	for i := range n {
		pts[i] = TimeSeriesPoint{
			StartTime: startTime.Add(time.Duration(i) * interval),
			P50Speed:  20 + float64(i%5),
			P85Speed:  25 + float64(i%5),
			P98Speed:  30 + float64(i%5),
			MaxSpeed:  35 + float64(i%5),
			Count:     100 + i*10,
		}
	}
	return pts
}

func TestDayBoundaries_SingleDay(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	pts := makeTestPoints(6, start, time.Hour)
	b := DayBoundaries(pts)
	if len(b) != 1 || b[0] != 0 {
		t.Errorf("single day boundaries = %v, want [0]", b)
	}
}

func TestDayBoundaries_TwoDays(t *testing.T) {
	start := time.Date(2025, 6, 15, 22, 0, 0, 0, time.UTC)
	pts := makeTestPoints(6, start, time.Hour)
	// Points at 22, 23, 00, 01, 02, 03 — day change at index 2.
	b := DayBoundaries(pts)
	if len(b) != 2 {
		t.Fatalf("two day boundaries len = %d, want 2, got %v", len(b), b)
	}
	if b[0] != 0 || b[1] != 2 {
		t.Errorf("boundaries = %v, want [0, 2]", b)
	}
}

func TestDayBoundaries_Empty(t *testing.T) {
	b := DayBoundaries(nil)
	if b != nil {
		t.Errorf("empty boundaries = %v, want nil", b)
	}
}

func TestApplyCountMask(t *testing.T) {
	pts := []TimeSeriesPoint{
		{Count: 100, P50Speed: 20, P85Speed: 25, P98Speed: 30, MaxSpeed: 35},
		{Count: 3, P50Speed: 20, P85Speed: 25, P98Speed: 30, MaxSpeed: 35},
		{Count: 50, P50Speed: 20, P85Speed: 25, P98Speed: 30, MaxSpeed: 35},
	}
	masked := ApplyCountMask(pts, 5)

	// Original unchanged.
	if math.IsNaN(pts[1].P50Speed) {
		t.Error("original slice was mutated")
	}

	// Index 0: count 100 >= 5, should be unchanged.
	if math.IsNaN(masked[0].P50Speed) {
		t.Error("index 0 should not be NaN")
	}

	// Index 1: count 3 < 5, all speeds NaN.
	if !math.IsNaN(masked[1].P50Speed) {
		t.Error("index 1 P50 should be NaN")
	}
	if !math.IsNaN(masked[1].P85Speed) {
		t.Error("index 1 P85 should be NaN")
	}
	if !math.IsNaN(masked[1].P98Speed) {
		t.Error("index 1 P98 should be NaN")
	}
	if !math.IsNaN(masked[1].MaxSpeed) {
		t.Error("index 1 Max should be NaN")
	}

	// Index 2: count 50 >= 5, should be unchanged.
	if math.IsNaN(masked[2].P50Speed) {
		t.Error("index 2 should not be NaN")
	}
}

func TestRenderTimeSeries_Structure(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	data := TimeSeriesData{
		Points: makeTestPoints(12, start, time.Hour),
		Units:  "mph",
		Title:  "Test Chart",
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle())
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	// Must be valid XML.
	dec := xml.NewDecoder(bytes.NewReader(svg))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("xml parse error: %v", err)
	}
	se, ok := tok.(xml.StartElement)
	if !ok || se.Name.Local != "svg" {
		t.Fatalf("expected <svg>, got %T %v", tok, tok)
	}

	svgStr := string(svg)
	if !strings.Contains(svgStr, "<polyline") {
		t.Error("output should contain <polyline> elements")
	}
}

func TestRenderTimeSeries_DayLines(t *testing.T) {
	start := time.Date(2025, 6, 15, 22, 0, 0, 0, time.UTC)
	data := TimeSeriesData{
		Points: makeTestPoints(8, start, time.Hour),
		Units:  "mph",
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle())
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	svgStr := string(svg)
	// Day boundary line should have gray stroke.
	if !strings.Contains(svgStr, `stroke="gray"`) {
		t.Error("missing day boundary line with gray stroke")
	}
}

func TestRenderTimeSeries_Empty(t *testing.T) {
	data := TimeSeriesData{
		Points: nil,
		Units:  "mph",
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle())
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}
	if !strings.Contains(string(svg), "No data") {
		t.Error("empty time series should contain 'No data'")
	}
}
