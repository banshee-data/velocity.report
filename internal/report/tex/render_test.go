package tex

import (
	"bytes"
	"flag"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/chart"
)

var update = flag.Bool("update", false, "update golden files")

// reTimestamp strips the volatile % Generated: line from tex output before
// golden comparison, so the test does not fail on timestamp differences.
var reTimestamp = regexp.MustCompile(`(?m)^% Generated: .*\n`)

func normalizeForGolden(b []byte) []byte {
	return reTimestamp.ReplaceAll(b, []byte("% Generated: <stripped>\n"))
}

func minimalTemplateData() TemplateData {
	return TemplateData{
		Location:         "Test Location",
		StartDate:        "2024-01-01",
		EndDate:          "2024-01-31",
		StartTimeDisplay: "2024-01-01T00:00:00Z",
		EndTimeDisplay:   "2024-01-31T23:59:59Z",
		Timezone:         "UTC",
		Units:            "mph",
		P50:              "25.00",
		P85:              "30.00",
		P98:              "35.00",
		MaxSpeed:         "42.00",
		TotalCount:       1000,
		HoursCount:       720,
		SpeedLimit:       25,
		FontDir:          "/fonts",
		Group:            "hourly",
		Source:           "radar",
		PaperOption:      "letterpaper",
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

func TestRenderTeX_TitleBlockUsesParagraphBreaks(t *testing.T) {
	data := minimalTemplateData()
	data.Location = EscapeTeX("Clarendon Avenue, San Francisco")

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, `{\huge\bfseries Clarendon Avenue, San Francisco\par}`) {
		t.Fatal("expected title block to use a paragraph break")
	}
	if strings.Contains(s, `Clarendon Avenue, San Francisco}\\[6pt]`) {
		t.Fatal("unexpected fragile title line break found in output")
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

func TestRenderTeX_MultipleCosineCorrectionLabel(t *testing.T) {
	data := minimalTemplateData()
	data.CosineCorrectionLabel = "multiple periods: 0.0°, 15.0°"

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, "Cosine correction") {
		t.Fatal("expected cosine correction label row")
	}
	if !strings.Contains(s, "multiple periods: 0.0°, 15.0°") {
		t.Fatalf("expected multiple-period cosine label, got:\n%s", s)
	}
	if strings.Contains(s, "Cosine angle:") {
		t.Fatal("single-angle cosine row should be absent when label is set")
	}
}

func TestRenderTeX_ConditionalComparison(t *testing.T) {
	data := minimalTemplateData()
	data.CompareStartDate = ""

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if strings.Contains(s, `Primary period (t1):`) {
		t.Error("comparison section should be absent when CompareStartDate is empty")
	}
	if strings.Contains(s, `Period t2`) {
		t.Error("comparison section should be absent when CompareStartDate is empty")
	}
}

func TestRenderTeX_ConditionalComparison_Present(t *testing.T) {
	data := minimalTemplateData()
	data.CompareStartDate = "2024-02-01"
	data.CompareEndDate = "2024-02-28"
	data.CompareStartTimeDisplay = "2024-02-01T00:00:00Z"
	data.CompareEndTimeDisplay = "2024-02-28T23:59:59Z"
	data.CompareP50 = "26.00"
	data.CompareP85 = "31.00"
	data.CompareP98 = "36.00"
	data.CompareMax = "45.00"
	data.CompareCount = 900

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	// Comparison overview: period (t2) itemize entry only present in overview_comparison.
	if !strings.Contains(s, `\item \textbf{Comparison period (t2):} 2024-02-01 to 2024-02-28`) {
		t.Error("comparison section should be present when CompareStartDate is set")
	}
	if !strings.Contains(s, `\item \textbf{Primary period (t1):} 2024-01-01 to 2024-01-31`) {
		t.Error("comparison report should render the comparison period overview module")
	}
	// Comparison survey parameters: Start time (t2) row (bfseries applied via column spec).
	if !strings.Contains(s, `Start time (t2): & \texttt{2024-02-01T00:00:00Z}`) {
		t.Error("comparison report should render the comparison survey-parameters module")
	}
	if !strings.Contains(s, `\fancyfoot[L]{\small 2024-01-01 to 2024-01-31 vs 2024-02-01 to 2024-02-28}`) {
		t.Error("comparison report footer should include the period range")
	}
}

func TestRenderTeX_SingleReportUsesSinglePeriodModule(t *testing.T) {
	data := minimalTemplateData()

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, `\item \textbf{Period:} 2024-01-01 to 2024-01-31`) {
		t.Error("single report should render the single-period overview module")
	}
	if strings.Contains(s, `\item \textbf{Primary period (t1):}`) {
		t.Error("single report unexpectedly rendered the comparison overview module")
	}
	if !strings.Contains(s, `Start time: & \texttt{2024-01-01T00:00:00Z}`) {
		t.Error("single report should render the single survey-parameters module")
	}
	if strings.Contains(s, `Start time (t2):`) {
		t.Error("single report unexpectedly rendered the comparison survey-parameters module")
	}
	if !strings.Contains(s, `\fancyfoot[L]{\small 2024-01-01 to 2024-01-31}`) {
		t.Error("single report footer should include the period range")
	}
	if strings.Contains(s, `Speed limit:`) {
		t.Error("single report overview should not include speed limit until multi-limit reporting is supported")
	}
}

func TestRenderTeX_MapSectionAppearsAfterCharts(t *testing.T) {
	data := minimalTemplateData()
	data.TimeSeriesChart = "timeseries.pdf"
	data.MapChart = "map.pdf"

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	chartPos := strings.Index(s, `\includegraphics[width=\textwidth]{timeseries.pdf}`)
	mapPos := strings.Index(s, `\includegraphics[width=\textwidth]{map.pdf}`)
	if chartPos == -1 || mapPos == -1 {
		t.Fatalf("expected both chart and map sections in rendered output")
	}
	if mapPos <= chartPos {
		t.Fatalf("expected map section after chart section")
	}
	if strings.Contains(s, `\clearpage`) || strings.Contains(s, `\onecolumn`) {
		t.Fatal("chart/map section should not force a page break or one-column mode")
	}
}

func TestRenderTeX_SingleChartSectionUsesNaturalFullWidthBlock(t *testing.T) {
	data := minimalTemplateData()
	data.TimeSeriesChart = "timeseries.pdf"

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	if strings.Contains(s, `\begin{figure*}`) || strings.Contains(s, `\begin{figure}[H]`) {
		t.Fatal("single report chart section should not use floats")
	}
	for _, unwanted := range []string{`\onecolumn`, `\clearpage`, `\afterpage{\clearpage}`} {
		if strings.Contains(s, unwanted) {
			t.Fatalf("single report chart section should not force layout with %q", unwanted)
		}
	}
	bodyEnd := strings.Index(s, `\end{multicols}`)
	chartPos := strings.Index(s, `\includegraphics[width=\textwidth]{timeseries.pdf}`)
	if bodyEnd == -1 || chartPos == -1 || chartPos <= bodyEnd {
		t.Fatal("expected full-width single chart after the balanced multicols body")
	}
	if !strings.Contains(s, `\captionof{figure}{Speed percentiles and observation counts over time}`) {
		t.Fatal("expected inline full-width single chart caption")
	}
}

func TestRenderTeX_SingleChartSectionCollapsesWithMapWhenPresent(t *testing.T) {
	data := minimalTemplateData()
	data.TimeSeriesChart = "timeseries.pdf"
	data.MapChart = "map.pdf"

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	for _, unwanted := range []string{`\begin{figure*}`, `\begin{figure}[H]`, `\afterpage{\clearpage}`, `\clearpage`, `\onecolumn`} {
		if strings.Contains(s, unwanted) {
			t.Fatalf("single report chart/map section should not force layout with %q", unwanted)
		}
	}
	chartPos := strings.Index(s, `\includegraphics[width=\textwidth]{timeseries.pdf}`)
	mapPos := strings.Index(s, `\includegraphics[width=\textwidth]{map.pdf}`)
	if chartPos == -1 || mapPos == -1 {
		t.Fatal("expected both single chart and map graphics in rendered output")
	}
	if mapPos <= chartPos {
		t.Fatal("expected map to follow the single chart in natural full-width flow")
	}
	if strings.Count(s, `\captionof{figure}`) < 2 {
		t.Fatal("expected chart and map to use inline figure captions")
	}
}

func TestRenderTeX_OverviewHistogramFiguresAvoidLegacyCenterSpacing(t *testing.T) {
	data := minimalTemplateData()
	data.HistogramChart = "histogram.pdf"

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	histogramPos := strings.Index(s, `\includegraphics[width=\linewidth]{histogram.pdf}`)
	if histogramPos == -1 {
		t.Fatal("expected overview histogram figure in rendered output")
	}
	start := histogramPos - 120
	if start < 0 {
		start = 0
	}
	end := histogramPos + 120
	if end > len(s) {
		end = len(s)
	}
	snippet := s[start:end]
	if strings.Contains(snippet, `\begin{center}`) || strings.Contains(snippet, `\end{center}`) {
		t.Fatal("overview histogram figure should not be wrapped in a center environment")
	}
	if strings.Contains(snippet, `\vspace{8pt}`) {
		t.Fatal("overview histogram should not keep the legacy gap between Table 1 and Figure 1")
	}
}

func TestRenderTeX_TableSpacingDirectivesPresent(t *testing.T) {
	data := minimalTemplateData()
	data.HistogramTableTeX = `\begin{tabular}{lrr}\end{tabular}`
	data.StatRows = []StatRow{{StartTime: "1/1 00:00", Count: 12, P50: "1", P85: "2", P98: "3", MaxSpeed: "4"}}
	data.StatTableTeX = BuildStatTableTeX(data.StatRows, "Detailed Data", data.Units)

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	for _, want := range []string{
		`\renewcommand{\arraystretch}{1.00}`,
		`\par\vspace{2pt}`,
		`\noindent{\large\bfseries Speed Distribution}\par\vspace{2pt}`,
		`\setlength{\fboxsep}{0pt}`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected spacing directive %q in rendered output", want)
		}
	}
	if strings.Contains(s, `\vrSuperTabular`) || strings.Contains(s, `\ST@pageleft`) {
		t.Fatalf("report should not include obsolete supertabular page-height patches:\n%s", s)
	}
}

func TestRenderTeX_ComparisonStatisticsSeparateLongTables(t *testing.T) {
	data := minimalTemplateData()
	data.CompareStartDate = "2024-02-01"
	data.CompareEndDate = "2024-02-28"
	data.DualHistogramTableTeX = `DUAL-TABLE`
	data.DailyStatTableTeX = `DAILY-TABLE`
	data.StatTableTeX = `GRANULAR-TABLE`

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}

	s := string(out)
	dualPos := strings.Index(s, `DUAL-TABLE`)
	dailyPos := strings.Index(s, `DAILY-TABLE`)
	granularPos := strings.Index(s, `GRANULAR-TABLE`)
	if dualPos == -1 || dailyPos == -1 || granularPos == -1 {
		t.Fatalf("expected all comparison statistics tables in rendered output, got:\n%s", s)
	}
	if !(dualPos < dailyPos && dailyPos < granularPos) {
		t.Fatalf("expected comparison statistics tables in order dual -> daily -> granular, got:\n%s", s)
	}
	betweenDualAndDaily := s[dualPos:dailyPos]
	betweenDailyAndGranular := s[dailyPos:granularPos]
	if !strings.Contains(s, `\par\noindent{\large\bfseries Detailed Data Tables}\par\vspace{2pt}`) {
		t.Fatalf("expected comparison statistics heading to use the tighter inline style, got:\n%s", s)
	}
	if !strings.Contains(betweenDualAndDaily, `\par\vspace{2pt}`) || !strings.Contains(betweenDailyAndGranular, `\par\vspace{2pt}`) {
		t.Fatalf("expected comparison statistics tables to be separated by explicit vertical spacing, got:\n%s", s)
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

func TestRenderTeX_GoldenSingle(t *testing.T) {
	data := minimalTemplateData()
	data.StatRows = []StatRow{
		{StartTime: "1/15 09:00", Count: 42, P50: "24.50", P85: "29.80", P98: "34.20", MaxSpeed: "40.10"},
	}
	data.StatTableTeX = BuildStatTableTeX(data.StatRows, "Detailed Data", data.Units)
	data.KeyMetricsTableTeX = BuildSingleKeyMetricsTableTeX("25.00", "30.00", "35.00", "42.00", "mph")
	data.HistogramTableTeX = `\begin{tabular}{lrr}` + "\n" +
		`20--25 & 120 & 12.0\% \\` + "\n" +
		`25--30 & 880 & 88.0\% \\` + "\n" +
		`\end{tabular}`

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}
	got := normalizeForGolden(out)

	const golden = "testdata/golden_single.tex"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden %s (run with -update to create): %v", golden, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s; re-run with -update to regenerate\ngot:\n%s\nwant:\n%s", golden, got, want)
	}
}

func TestRenderTeX_GoldenComparison(t *testing.T) {
	data := minimalTemplateData()
	data.CompareStartDate = "2024-02-01"
	data.CompareEndDate = "2024-02-28"
	data.CompareStartTimeDisplay = "2024-02-01T00:00:00Z"
	data.CompareEndTimeDisplay = "2024-02-28T23:59:59Z"
	data.CompareP50 = "26.00"
	data.CompareP85 = "31.00"
	data.CompareP98 = "36.00"
	data.CompareMax = "45.00"
	data.CompareCount = 900
	data.DeltaP50 = "+1.50"
	data.DeltaP85 = "+1.20"
	data.DeltaP98 = "+0.80"
	data.DeltaMax = "+2.90"
	data.DeltaP50Pct = "+6.1%"
	data.DeltaP85Pct = "+4.0%"
	data.DeltaP98Pct = "+2.3%"
	data.DeltaMaxPct = "+6.9%"
	data.CompareSource = "radar_objects"
	data.TotalCountFormatted = "1,000"
	data.CompareTotalCountFormatted = "900"
	data.CombinedCountFormatted = "1,900"
	data.KeyMetricsTableTeX = BuildComparisonKeyMetricsTableTeX(
		"25.00", "30.00", "35.00", "42.00",
		"26.00", "31.00", "36.00", "45.00",
		"+6.1%", "+4.0%", "+2.3%", "+6.9%",
		"1,000", "900",
		"mph",
	)
	data.StatRows = []StatRow{
		{StartTime: "1/15 09:00", Count: 42, P50: "24.50", P85: "29.80", P98: "34.20", MaxSpeed: "40.10"},
	}
	data.StatTableTeX = BuildStatTableTeX(data.StatRows, "Detailed Data", data.Units)

	out, err := RenderTeX(data)
	if err != nil {
		t.Fatalf("RenderTeX() error: %v", err)
	}
	got := normalizeForGolden(out)

	const golden = "testdata/golden_comparison.tex"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden %s (run with -update to create): %v", golden, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s; re-run with -update to regenerate\ngot:\n%s\nwant:\n%s", golden, got, want)
	}
}
