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

func TestExpandTimeSeriesGaps(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	pts := []TimeSeriesPoint{
		{StartTime: start, Count: 10, P50Speed: 20, P85Speed: 25, P98Speed: 30, MaxSpeed: 35},
		{StartTime: start.Add(3 * time.Hour), Count: 12, P50Speed: 21, P85Speed: 26, P98Speed: 31, MaxSpeed: 36},
	}

	got := ExpandTimeSeriesGaps(pts, int64(time.Hour/time.Second))
	if len(got) != 4 {
		t.Fatalf("expanded len = %d, want 4", len(got))
	}
	if got[0].Count != 10 || got[3].Count != 12 {
		t.Fatalf("existing points not preserved: %+v", got)
	}
	for i := 1; i <= 2; i++ {
		if got[i].Count != 0 || !math.IsNaN(got[i].P50Speed) {
			t.Fatalf("expanded gap point %d = %+v, want zero-count NaN placeholder", i, got[i])
		}
	}
}

func TestExpandTimeSeriesGapsInRange(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	pts := []TimeSeriesPoint{
		{StartTime: start.Add(2 * time.Hour), Count: 10, P50Speed: 20, P85Speed: 25, P98Speed: 30, MaxSpeed: 35},
	}

	got := ExpandTimeSeriesGapsInRange(pts, int64(time.Hour/time.Second), start, start.Add(4*time.Hour))
	if len(got) != 5 {
		t.Fatalf("expanded len = %d, want 5", len(got))
	}
	if got[2].Count != 10 {
		t.Fatalf("observed point not preserved at range-aligned index 2: %+v", got)
	}
	for _, i := range []int{0, 1, 3, 4} {
		if got[i].Count != 0 || !math.IsNaN(got[i].P50Speed) {
			t.Fatalf("expanded range point %d = %+v, want zero-count NaN placeholder", i, got[i])
		}
	}
}

func TestRenderTimeSeries_Structure(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	data := TimeSeriesData{
		Points: makeTestPoints(12, start, time.Hour),
		Units:  "mph",
		Title:  "Test Chart",
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle(PaperA4))
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

func TestRenderTimeSeries_GapDividers(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	pts := makeTestPoints(8, start, time.Hour)
	// Create a gap: points 3-4 have zero count (below threshold → NaN).
	pts[3].Count = 0
	pts[4].Count = 0

	svg, err := RenderTimeSeries(TimeSeriesData{Points: pts, Units: "mph"}, DefaultTimeSeriesStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	svgStr := string(svg)
	// Gap divider should appear with dashed stroke.
	if !strings.Contains(svgStr, `stroke-dasharray="3 3"`) {
		t.Error("missing gap divider with stroke-dasharray")
	}
	// No day-boundary gray lines should appear.
	if strings.Contains(svgStr, `stroke="gray"`) {
		t.Error("unexpected day boundary gray line found")
	}
}

func TestRenderTimeSeries_NoDividerWithoutGap(t *testing.T) {
	// Crossing a day boundary without a data gap should produce no divider.
	start := time.Date(2025, 6, 15, 22, 0, 0, 0, time.UTC)
	data := TimeSeriesData{
		Points: makeTestPoints(8, start, time.Hour),
		Units:  "mph",
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	svgStr := string(svg)
	if strings.Contains(svgStr, `stroke-dasharray="3 3"`) {
		t.Error("unexpected gap divider for continuous data spanning midnight")
	}
}

func TestRenderTimeSeries_LowSampleBreaksLine(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	pts := makeTestPoints(8, start, time.Hour)
	// Drop two interior buckets below CountMissingThreshold (default 5).
	pts[3].Count = 2
	pts[4].Count = 1

	style := DefaultTimeSeriesStyle(PaperA4)
	svg, err := RenderTimeSeries(TimeSeriesData{Points: pts, Units: "mph"}, style)
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	// Four series × two segments (before/after the masked gap) = 8 polylines.
	// Masking preserves Count, so count bars and hover tooltips remain honest.
	got := strings.Count(string(svg), "<polyline")
	if got < 8 {
		t.Errorf("expected polyline segments split by masked gap, got %d polylines", got)
	}
}

func TestDetectTimeGaps_NoneForUniform(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	pts := makeTestPoints(6, start, time.Hour)
	gaps := detectTimeGaps(pts)
	for i, g := range gaps {
		if g {
			t.Errorf("unexpected gap at index %d for uniform hourly data", i)
		}
	}
}

func TestDetectTimeGaps_MidnightContinuous(t *testing.T) {
	// Continuous hourly data spanning midnight should produce no gaps.
	start := time.Date(2025, 6, 15, 22, 0, 0, 0, time.UTC)
	pts := makeTestPoints(6, start, time.Hour)
	gaps := detectTimeGaps(pts)
	for i, g := range gaps {
		if g {
			t.Errorf("unexpected gap at index %d for continuous midnight-spanning data", i)
		}
	}
}

func TestDetectTimeGaps_OvernightBreak(t *testing.T) {
	// Simulate 8am–4pm data on two consecutive days; the overnight jump
	// (16 hours) should be flagged as a gap.
	day1 := makeTestPoints(8, time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC), time.Hour)
	day2 := makeTestPoints(8, time.Date(2025, 6, 16, 8, 0, 0, 0, time.UTC), time.Hour)
	pts := append(day1, day2...)

	gaps := detectTimeGaps(pts)
	// No gap within day1.
	for i := 1; i < 8; i++ {
		if gaps[i] {
			t.Errorf("unexpected gap at day1 index %d", i)
		}
	}
	// Gap at the start of day2.
	if !gaps[8] {
		t.Error("expected gap at start of day2 (index 8)")
	}
	// No gap within day2.
	for i := 9; i < len(pts); i++ {
		if gaps[i] {
			t.Errorf("unexpected gap at day2 index %d", i)
		}
	}
}

func TestDetectTimeGaps_ThreeDays(t *testing.T) {
	// Three-day scenario: gaps at the start of day2 and day3.
	base := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	var pts []TimeSeriesPoint
	for d := 0; d < 3; d++ {
		pts = append(pts, makeTestPoints(8, base.Add(time.Duration(d)*24*time.Hour), time.Hour)...)
	}

	gaps := detectTimeGaps(pts)
	for i, g := range gaps {
		wantGap := (i == 8 || i == 16) // start of day2 and day3
		if g != wantGap {
			t.Errorf("gap[%d] = %v, want %v", i, g, wantGap)
		}
	}
}

func TestDetectTimeGaps_Empty(t *testing.T) {
	gaps := detectTimeGaps(nil)
	if len(gaps) != 0 {
		t.Errorf("expected empty slice for nil input, got len %d", len(gaps))
	}
}

func TestRenderTimeSeries_TimeGapBreaksLine(t *testing.T) {
	// Two blocks of 8 hourly points separated by a 16-hour overnight gap.
	day1 := makeTestPoints(8, time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC), time.Hour)
	day2 := makeTestPoints(8, time.Date(2025, 6, 16, 8, 0, 0, 0, time.UTC), time.Hour)
	pts := append(day1, day2...)

	svg, err := RenderTimeSeries(TimeSeriesData{Points: pts, Units: "mph"}, DefaultTimeSeriesStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	svgStr := string(svg)
	// Dashed divider must appear for the time gap.
	if !strings.Contains(svgStr, `stroke-dasharray="3 3"`) {
		t.Error("expected dashed gap divider for overnight break")
	}
	// Four series × two segments = ≥8 polylines.
	got := strings.Count(svgStr, "<polyline")
	if got < 8 {
		t.Errorf("expected ≥8 polylines (two segments per series), got %d", got)
	}
}

func TestXTicks_SignatureTakesOnlyPoints(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	ticks := XTicks(makeTestPoints(6, start, time.Hour))
	if len(ticks) == 0 {
		t.Error("XTicks returned no ticks for 6-point series")
	}
}

func TestRenderTimeSeries_Empty(t *testing.T) {
	data := TimeSeriesData{
		Points: nil,
		Units:  "mph",
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}
	if !strings.Contains(string(svg), "No data") {
		t.Error("empty time series should contain 'No data'")
	}
}

// TestXTicks_NoOverlapCrossUTCMidnight verifies that two points in the same
// local calendar half-day but in different UTC 12h buckets (which happens in
// non-UTC timezones) are not emitted as separate ticks at the 12h cadence.
func TestXTicks_NoOverlapCrossUTCMidnight(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	// Jan 16 12:00 PST = Jan 16 20:00 UTC → UTC-bucket N
	// Jan 16 16:00 PST = Jan 17 00:00 UTC → UTC-bucket N+1 (different bucket, same local half-day)
	// The local-time bucketing fix must emit only ONE tick for Jan 16 (the noon half).
	pts := []TimeSeriesPoint{
		{StartTime: time.Date(2026, 1, 16, 12, 0, 0, 0, loc), P50Speed: 30, P85Speed: 38, P98Speed: 44, MaxSpeed: 54, Count: 158},
		{StartTime: time.Date(2026, 1, 16, 13, 0, 0, 0, loc), P50Speed: 32, P85Speed: 39, P98Speed: 44, MaxSpeed: 46, Count: 180},
		{StartTime: time.Date(2026, 1, 16, 14, 0, 0, 0, loc), P50Speed: 30, P85Speed: 38, P98Speed: 44, MaxSpeed: 50, Count: 206},
		{StartTime: time.Date(2026, 1, 16, 15, 0, 0, 0, loc), P50Speed: 31, P85Speed: 39, P98Speed: 45, MaxSpeed: 53, Count: 187},
		{StartTime: time.Date(2026, 1, 16, 16, 0, 0, 0, loc), P50Speed: 32, P85Speed: 38, P98Speed: 44, MaxSpeed: 44, Count: 121},
		{StartTime: time.Date(2026, 1, 19, 11, 0, 0, 0, loc), P50Speed: 35, P85Speed: 40, P98Speed: 44, MaxSpeed: 44, Count: 113},
	}

	ticks := XTicks(pts)
	// Verify no two consecutive ticks share the same "Jan 16" label.
	var prev string
	for _, tick := range ticks {
		if tick.Label == prev {
			t.Errorf("duplicate consecutive tick label %q — overlapping labels not fixed", tick.Label)
		}
		prev = tick.Label
	}
	// There should be at most 2 Jan 16 ticks (morning and afternoon half-days),
	// but 12:00 and 16:00 both fall in the afternoon half so only one is emitted.
	jan16Count := 0
	for _, tick := range ticks {
		if strings.Contains(tick.Label, "Jan 16") {
			jan16Count++
		}
	}
	if jan16Count > 1 {
		t.Errorf("expected at most 1 Jan 16 tick (12:00 and 16:00 are same local half-day), got %d: %v", jan16Count, ticks)
	}
}

func TestRenderTimeSeries_ReferenceLineAndHoverTooltips(t *testing.T) {
	start := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	data := TimeSeriesData{
		Points:       makeTestPoints(4, start, time.Hour),
		Units:        "mph",
		P98Reference: 32,
	}
	svg, err := RenderTimeSeries(data, DefaultTimeSeriesStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderTimeSeries error: %v", err)
	}

	svgStr := string(svg)
	if !strings.Contains(svgStr, `class="p98-reference"`) {
		t.Fatal("expected aggregate p98 reference line in SVG")
	}
	if !strings.Contains(svgStr, "p98 overall") {
		t.Fatal("expected p98 overall legend label in SVG")
	}
	if !strings.Contains(svgStr, `<title>`) {
		t.Fatal("expected hover tooltip titles in SVG")
	}
	if !strings.Contains(svgStr, "count: 100") {
		t.Fatal("expected hover tooltip metrics in SVG")
	}
}
