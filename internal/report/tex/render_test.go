package tex

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/chart"
)

func minimalTemplateData() TemplateData {
	return TemplateData{
		Location:   "Test Location",
		StartDate:  "1 Jan 2024",
		EndDate:    "31 Jan 2024",
		Timezone:   "UTC",
		Units:      "mph",
		P50:        "25.00",
		P85:        "30.00",
		P98:        "35.00",
		MaxSpeed:   "42.00",
		TotalCount: 1000,
		HoursCount: 720,
		SpeedLimit: 25,
		FontDir:    "/fonts",
		Group:      "hourly",
		Source:     "radar",
	}
}

func TestRenderTeX_Valid(t *testing.T) {
	data := minimalTemplateData()
	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, `\begin{document}`) {
		t.Error("output missing \\begin{document}")
	}
	if !strings.Contains(s, `\end{document}`) {
		t.Error("output missing \\end{document}")
	}
	if !strings.Contains(s, "Test Location") {
		t.Error("output missing location")
	}
}

func TestRenderTeX_EscapedStrings(t *testing.T) {
	data := minimalTemplateData()
	data.Location = EscapeTeX("Smith & Jones")

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, `Smith \& Jones`) {
		t.Error("expected escaped ampersand in output")
	}
	if strings.Contains(s, "Smith & Jones") {
		t.Error("unescaped ampersand found in output")
	}
}

func TestRenderTeX_ConditionalHistogram(t *testing.T) {
	data := minimalTemplateData()
	data.HistogramTableTeX = ""

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	if strings.Contains(string(out), "Speed Distribution") {
		t.Error("histogram section should be absent when HistogramTableTeX is empty")
	}
}

func TestRenderTeX_ConditionalHistogram_Present(t *testing.T) {
	data := minimalTemplateData()
	data.HistogramTableTeX = `\begin{tabular}{lrr}\end{tabular}`

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	if !strings.Contains(string(out), "Speed Distribution") {
		t.Error("histogram section should be present when HistogramTableTeX is set")
	}
}

func TestRenderTeX_ConditionalComparison(t *testing.T) {
	data := minimalTemplateData()
	data.CompareStartDate = ""

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	if strings.Contains(string(out), "Comparison Summary") {
		t.Error("comparison section should be absent when CompareStartDate is empty")
	}
}

func TestRenderTeX_ConditionalComparison_Present(t *testing.T) {
	data := minimalTemplateData()
	data.CompareStartDate = "1 Feb 2024"
	data.CompareEndDate = "28 Feb 2024"
	data.CompareP50 = "26.00"
	data.CompareP85 = "31.00"
	data.CompareP98 = "36.00"
	data.CompareMax = "45.00"
	data.CompareCount = 900

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	if !strings.Contains(string(out), "Comparison Summary") {
		t.Error("comparison section should be present when CompareStartDate is set")
	}
}

func TestBuildStatRows(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	pts := []chart.TimeSeriesPoint{
		{
			StartTime: time.Date(2024, 3, 15, 20, 0, 0, 0, time.UTC),
			P50Speed:  25.5,
			P85Speed:  30.2,
			P98Speed:  35.8,
			MaxSpeed:  42.1,
			Count:     150,
		},
		{
			StartTime: time.Date(2024, 3, 15, 21, 0, 0, 0, time.UTC),
			P50Speed:  math.NaN(),
			P85Speed:  math.NaN(),
			P98Speed:  math.NaN(),
			MaxSpeed:  math.NaN(),
			Count:     0,
		},
	}

	rows := BuildStatRows(pts, loc)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// First row: normal values.
	if rows[0].StartTime != "3/15 13:00" {
		t.Errorf("row 0 StartTime = %q, want %q", rows[0].StartTime, "3/15 13:00")
	}
	if rows[0].Count != 150 {
		t.Errorf("row 0 Count = %d, want 150", rows[0].Count)
	}
	if rows[0].P50 != "25.50" {
		t.Errorf("row 0 P50 = %q, want %q", rows[0].P50, "25.50")
	}

	// Second row: NaN values.
	if rows[1].P50 != "--" {
		t.Errorf("row 1 P50 = %q, want %q", rows[1].P50, "--")
	}
	if rows[1].MaxSpeed != "--" {
		t.Errorf("row 1 MaxSpeed = %q, want %q", rows[1].MaxSpeed, "--")
	}
}
