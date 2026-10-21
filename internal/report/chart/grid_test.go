package chart

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestRenderGridHeatmapDrawsDefinedCellsOnly(t *testing.T) {
	data := GridHeatmapData{
		Title:       "Settle rate",
		Subtitle:    "hesai-pandar40p at 2026-10-21T10:00:00Z",
		MetricLabel: "Settle rate",
		Ratio:       true,
		Rings:       40,
		Cells: []GridHeatmapCell{
			{Ring: 0, AzimuthStartDeg: 0, AzimuthEndDeg: 3, Value: new(float64(0))},
			{Ring: 39, AzimuthStartDeg: 357, AzimuthEndDeg: 360, Value: new(float64(1))},
			{Ring: 5, AzimuthStartDeg: 90, AzimuthEndDeg: 93, Value: nil},
			{Ring: 40, AzimuthStartDeg: 0, AzimuthEndDeg: 3, Value: new(float64(0.5))},
		},
		Summary: []string{"Filled: 10 (50%)", "Settled: 5 (25%)"},
	}
	svg, err := RenderGridHeatmap(data, DefaultGridHeatmapStyle())
	if err != nil {
		t.Fatalf("RenderGridHeatmap: %v", err)
	}
	out := string(svg)
	if got := strings.Count(out, `class="cell"`); got != 2 {
		t.Errorf("drew %d cells, want 2: an undefined value and an out-of-range ring are skipped", got)
	}
	for _, want := range []string{gridRamp[0], gridRamp[len(gridRamp)-1], "Settle rate", "Filled: 10 (50%)", "100%", "no data", "Azimuth (degrees)"} {
		if !strings.Contains(out, want) {
			t.Errorf("SVG missing %q", want)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "</svg>") {
		t.Error("SVG document not closed")
	}
}

func TestRenderGridHeatmapCountScaleRunsToMaximum(t *testing.T) {
	data := GridHeatmapData{
		MetricLabel: "Mean times seen",
		Rings:       2,
		Cells: []GridHeatmapCell{
			{Ring: 0, AzimuthStartDeg: 0, AzimuthEndDeg: 180, Value: new(float64(12.5))},
			{Ring: 1, AzimuthStartDeg: 180, AzimuthEndDeg: 360, Value: new(float64(25))},
		},
	}
	svg, err := RenderGridHeatmap(data, DefaultGridHeatmapStyle())
	if err != nil {
		t.Fatalf("RenderGridHeatmap: %v", err)
	}
	out := string(svg)
	if !strings.Contains(out, ">25.0<") {
		t.Error("colour bar should end at the largest value, 25.0")
	}
	if !strings.Contains(out, gridRamp[5]) {
		t.Errorf("half the maximum should take ramp stop 5 (%s)", gridRamp[5])
	}
}

func TestRenderGridHeatmapAllUndefinedCountScale(t *testing.T) {
	data := GridHeatmapData{Rings: 1, Cells: []GridHeatmapCell{{Ring: 0, AzimuthEndDeg: 360}}}
	svg, err := RenderGridHeatmap(data, DefaultGridHeatmapStyle())
	if err != nil {
		t.Fatalf("RenderGridHeatmap: %v", err)
	}
	if strings.Contains(string(svg), `class="cell"`) {
		t.Error("no cell should be drawn when every value is undefined")
	}
}

func TestRenderGridHeatmapRefusesNoRings(t *testing.T) {
	if _, err := RenderGridHeatmap(GridHeatmapData{}, DefaultGridHeatmapStyle()); !errors.Is(err, ErrGridRingsRequired) {
		t.Errorf("err = %v, want ErrGridRingsRequired", err)
	}
}

func TestRenderGridMessage(t *testing.T) {
	out := string(RenderGridMessage(DefaultGridHeatmapStyle(), "No background grid", "for sensor x"))
	if !strings.Contains(out, "No background grid") || !strings.Contains(out, "for sensor x") {
		t.Errorf("message chart missing its lines: %s", out)
	}
}

func TestGridColourClamps(t *testing.T) {
	cases := map[float64]string{-1: gridRamp[0], 0: gridRamp[0], 0.999: gridRamp[9], 1: gridRamp[9], 2: gridRamp[9]}
	for in, want := range cases {
		if got := gridColour(in); got != want {
			t.Errorf("gridColour(%v) = %s, want %s", in, got, want)
		}
	}
	if got := clampUnit(math.NaN()); got != 0 {
		t.Errorf("clampUnit(NaN) = %v, want 0", got)
	}
}
