package tex

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestEscapeTeX(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ampersand", "&", `\&`},
		{"percent", "%", `\%`},
		{"dollar", "$", `\$`},
		{"hash", "#", `\#`},
		{"underscore", "_", `\_`},
		{"open_brace", "{", `\{`},
		{"close_brace", "}", `\}`},
		{"tilde", "~", `\textasciitilde{}`},
		{"caret", "^", `\textasciicircum{}`},
		{"backslash", `\`, `\textbackslash{}`},
		{"combined", "Smith & Jones: 100%", `Smith \& Jones: 100\%`},
		{"empty", "", ""},
		{"no_special", "hello world", "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeTeX(tt.in)
			if got != tt.want {
				t.Errorf("EscapeTeX(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want string
	}{
		{"nan", math.NaN(), "--"},
		{"pos_inf", math.Inf(1), "--"},
		{"neg_inf", math.Inf(-1), "--"},
		{"pi", 3.14159, "3.14"},
		{"zero", 0, "0.00"},
		{"negative", -12.5, "-12.50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatNumber(tt.v)
			if got != tt.want {
				t.Errorf("FormatNumber(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want string
	}{
		{"nan", math.NaN(), "--"},
		{"normal", 45.678, "45.7%"},
		{"zero", 0, "0.0%"},
		{"hundred", 100.0, "100.0%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPercent(tt.v)
			if got != tt.want {
				t.Errorf("FormatPercent(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	// 2024-03-15 20:30 UTC = 2024-03-15 13:30 PDT
	ts := time.Date(2024, 3, 15, 20, 30, 0, 0, time.UTC)
	got := FormatTime(ts, loc)
	want := "3/15 13:30"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

func TestFormatTime_NilLocation(t *testing.T) {
	ts := time.Date(2024, 6, 1, 8, 5, 0, 0, time.UTC)
	got := FormatTime(ts, nil)
	want := "6/1 08:05"
	if got != want {
		t.Errorf("FormatTime(nil loc) = %q, want %q", got, want)
	}
}

func TestBuildHistogramTableTeX(t *testing.T) {
	buckets := map[float64]int64{
		10: 5,
		20: 40,
		30: 30,
		40: 15,
		50: 10,
	}

	result := BuildHistogramTableTeX(buckets, 10, 15, 50, "mph")

	if result == "" {
		t.Fatal("expected non-empty result")
	}

	// Check structural markers.
	for _, want := range []string{`\hline`, `\begin{tabular}`, `\rowcolors`, `\sffamily`, "50+"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q", want)
		}
	}
	if strings.Contains(result, `\begin{supertabular}`) {
		t.Fatalf("histogram table should use a regular tabular in two-column flow, got:\n%s", result)
	}

	// Count data rows: tabular data rows all contain " & " cell separators.
	// Header rows are filtered by \sffamily; this counts only content rows.
	dataRows := 0
	for _, l := range strings.Split(result, "\n") {
		if strings.Contains(l, ` & `) && !strings.Contains(l, `\sffamily`) {
			dataRows++
		}
	}
	if dataRows != 5 {
		t.Errorf("expected 5 data rows, got %d", dataRows)
	}
}

func TestStyledTablesDoNotDrawTopRuleAboveHeader(t *testing.T) {
	stat := BuildStatTableTeX([]StatRow{{
		StartTime: "6/2 08:00",
		Count:     109,
		P50:       "23.43",
		P85:       "35.71",
		P98:       "43.78",
		MaxSpeed:  "46.47",
	}}, "Detailed Data", "mph")
	hist := BuildHistogramTableTeX(map[float64]int64{5: 10, 10: 20}, 5, 5, 50, "mph")
	dual := BuildDualHistogramTableTeX(
		map[float64]int64{5: 10, 10: 20},
		map[float64]int64{5: 15, 10: 25},
		5, 5, 50, "mph",
	)

	for name, table := range map[string]string{
		"stat":      stat,
		"histogram": hist,
		"dual":      dual,
	} {
		header := "Bucket"
		if name == "stat" {
			header = "Start Time"
		}
		headerPos := strings.Index(table, header)
		rulePos := strings.Index(table, `\hline`)
		if rulePos == -1 {
			rulePos = strings.Index(table, `\rule{\linewidth}{0.4pt}`)
		}
		if headerPos == -1 || rulePos == -1 {
			t.Fatalf("%s table missing header or rule:\n%s", name, table)
		}
		if rulePos < headerPos {
			t.Fatalf("%s table has a top rule before the header:\n%s", name, table)
		}
	}
}

func TestBuildStatTableTeX_UsesFullWidthSmallTable(t *testing.T) {
	result := BuildStatTableTeX([]StatRow{{
		StartTime: "6/2 08:00",
		Count:     109,
		P50:       "23.43",
		P85:       "35.71",
		P98:       "43.78",
		MaxSpeed:  "46.47",
	}}, "Detailed Data", "mph")

	for _, want := range []string{
		`\AtkinsonMono\small`,
		`\noindent\makebox[\linewidth]{{\normalfont\bfseries\small Detailed Data}}`,
		`\makebox[0.24\linewidth][l]{\strut \sffamily\bfseries Start Time}`,
		`\makebox[0.14\linewidth][r]{\strut \sffamily\bfseries \shortstack[r]{p50 \\ (mph)}}`,
		`\colorbox{black!2}`,
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("stat table missing %q:\n%s", want, result)
		}
	}
	for _, unwanted := range []string{`\AtkinsonMono\scriptsize`, `\footnotesize`, `\begin{supertabular}`} {
		if strings.Contains(result, unwanted) {
			t.Fatalf("stat table should not use %q, got:\n%s", unwanted, result)
		}
	}
	captionPos := strings.LastIndex(result, `\noindent\makebox[\linewidth]{{\normalfont\bfseries\small Detailed Data}}`)
	rulePos := strings.LastIndex(result, `\noindent\rule{\linewidth}{0.4pt}\par`)
	if captionPos == -1 || rulePos == -1 || captionPos < rulePos {
		t.Fatalf("stat table caption should appear below the flowing rows:\n%s", result)
	}
}

func TestBuildStatTableTeX_LongTableUsesFlowRows(t *testing.T) {
	rows := make([]StatRow, 0, 120)
	for index := 0; index < 120; index++ {
		rows = append(rows, StatRow{
			StartTime: "6/2 08:00",
			Count:     100 + index,
			P50:       "23.43",
			P85:       "35.71",
			P98:       "43.78",
			MaxSpeed:  "46.47",
		})
	}

	result := BuildStatTableTeX(rows, "Detailed Data", "mph")

	for _, want := range []string{
		`\noindent\makebox[\linewidth]{{\normalfont\bfseries\small Detailed Data}}`,
		`\makebox[\linewidth][l]`,
		`\makebox[0.24\linewidth][l]`,
		`\makebox[0.14\linewidth][r]`,
		`\colorbox{black!2}`,
		`\noindent\rule{\linewidth}{0.4pt}\par`,
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("flow stat table missing %q:\n%s", want, result)
		}
	}
	for _, unwanted := range []string{`\columnbreak`, `Detailed Data (cont.)`} {
		if strings.Contains(result, unwanted) {
			t.Fatalf("flow stat table should not use %q:\n%s", unwanted, result)
		}
	}
	for _, unwanted := range []string{`\clearpage`, `\onecolumn`, `\begin{minipage}`, `\begin{supertabular}`} {
		if strings.Contains(result, unwanted) {
			t.Fatalf("flow stat table should not force layout with %q:\n%s", unwanted, result)
		}
	}
	captionPos := strings.LastIndex(result, `\noindent\makebox[\linewidth]{{\normalfont\bfseries\small Detailed Data}}`)
	rulePos := strings.LastIndex(result, `\noindent\rule{\linewidth}{0.4pt}\par`)
	if captionPos == -1 || rulePos == -1 || captionPos < rulePos {
		t.Fatalf("flow stat table caption should appear below the flowing rows:\n%s", result)
	}
}

func TestReportTablesUseSharedFullWidthFormatting(t *testing.T) {
	tables := map[string]string{
		"single key metrics": BuildSingleKeyMetricsTableTeX("25.00", "30.00", "35.00", "42.00", "mph"),
		"comparison key metrics": BuildComparisonKeyMetricsTableTeX(
			"25.00", "30.00", "35.00", "42.00",
			"26.00", "31.00", "36.00", "45.00",
			"+4.0\\%", "+3.3\\%", "+2.9\\%", "+7.1\\%",
			"1,000", "900",
			"mph",
		),
		"velocity distribution": BuildHistogramTableTeX(map[float64]int64{5: 10, 10: 20}, 5, 5, 50, "mph"),
		"comparison velocity distribution": BuildDualHistogramTableTeX(
			map[float64]int64{5: 10, 10: 20},
			map[float64]int64{5: 15, 10: 25},
			5, 5, 50, "mph",
		),
		"stat": BuildStatTableTeX([]StatRow{{
			StartTime: "6/2 08:00",
			Count:     109,
			P50:       "23.43",
			P85:       "35.71",
			P98:       "43.78",
			MaxSpeed:  "46.47",
		}}, "Detailed Data", "mph"),
	}

	for name, table := range tables {
		for _, want := range []string{
			`\AtkinsonMono\small`,
			`\renewcommand{\arraystretch}{1.00}`,
			`\setlength{\tabcolsep}{2pt}`,
			`\setlength{\fboxsep}{0pt}`,
			`\rowcolors{2}{black!2}{white}`,
			`\noindent`,
		} {
			if !strings.Contains(table, want) {
				t.Fatalf("%s table missing shared format %q:\n%s", name, want, table)
			}
		}
		if !strings.Contains(table, `{\sffamily\bfseries `) && !strings.Contains(table, `\sffamily\bfseries `) {
			t.Fatalf("%s table missing shared header style:\n%s", name, table)
		}
		if !strings.Contains(table, `@{}>{\raggedright\arraybackslash}p{`) && !strings.Contains(table, `\makebox[\linewidth][l]`) {
			t.Fatalf("%s table should use either tabular p-columns or flow-table boxes:\n%s", name, table)
		}
		for _, unwanted := range []string{
			`\begin{center}`,
			`\begin{tabular*}`,
			`\extracolsep`,
			`\AtkinsonMono\scriptsize`,
			`\footnotesize`,
		} {
			if strings.Contains(table, unwanted) {
				t.Fatalf("%s table should not use legacy format %q:\n%s", name, unwanted, table)
			}
		}
	}
}

func TestComparisonKeyMetricsPadsCountCellsUnderSpeedUnits(t *testing.T) {
	mph := BuildComparisonKeyMetricsTableTeX(
		"25.14", "30.00", "35.00", "42.00",
		"26.10", "31.00", "36.00", "45.00",
		"+4.0\\%", "+3.3\\%", "+2.9\\%", "+7.1\\%",
		"1,345", "900",
		"mph",
	)
	for _, want := range []string{
		`25.14 mph`,
		`Vehicle Count & 1,345\phantom{ mph} & 900\phantom{ mph}`,
		`\multicolumn{1}{>{\raggedleft\arraybackslash}p{0.22\linewidth}}{\makebox[\linewidth][r]{\makebox[5.8em][l]{\sffamily\bfseries Period t1}}} & \multicolumn{1}{>{\raggedleft\arraybackslash}p{0.22\linewidth}}{\makebox[\linewidth][r]{\makebox[5.8em][l]{\sffamily\bfseries Period t2}}}`,
	} {
		if !strings.Contains(mph, want) {
			t.Fatalf("mph comparison table missing %q:\n%s", want, mph)
		}
	}

	kmph := BuildComparisonKeyMetricsTableTeX(
		"25.14", "30.00", "35.00", "42.00",
		"26.10", "31.00", "36.00", "45.00",
		"+4.0\\%", "+3.3\\%", "+2.9\\%", "+7.1\\%",
		"1,345", "900",
		"kmph",
	)
	if !strings.Contains(kmph, `Vehicle Count & 1,345\phantom{ kmph} & 900\phantom{ kmph}`) {
		t.Fatalf("kmph comparison table should pad count cells with the longer unit phantom:\n%s", kmph)
	}
}

func TestBuildStatTableTeX_PadsDatesForSlashAndColonAlignment(t *testing.T) {
	result := BuildStatTableTeX([]StatRow{
		{StartTime: "1/1 0:00", Count: 1, P50: "1", P85: "2", P98: "3", MaxSpeed: "4"},
		{StartTime: "1/15 00:00", Count: 2, P50: "1", P85: "2", P98: "3", MaxSpeed: "4"},
		{StartTime: "12/31 09:00", Count: 3, P50: "1", P85: "2", P98: "3", MaxSpeed: "4"},
	}, "Detailed Data", "mph")

	for _, want := range []string{
		`\phantom{0}1/\phantom{0}1 \phantom{0}0:00`,
		`\phantom{0}1/15 00:00`,
		`12/31 09:00`,
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("stat table missing padded time %q:\n%s", want, result)
		}
	}
}

func TestBuildHistogramTableTeX_Empty(t *testing.T) {
	result := BuildHistogramTableTeX(nil, 5, 10, 50, "mph")
	if result != "" {
		t.Errorf("expected empty string for nil buckets, got %q", result)
	}
}

func TestBuildHistogramTableTeX_CollapsesAllBucketsAtOrAboveMax(t *testing.T) {
	buckets := map[float64]int64{
		5:  2050,
		10: 3387,
		15: 7143,
		20: 3432,
		25: 390,
		30: 18,
		35: 5,
		40: 5,
		45: 1,
	}

	result := BuildHistogramTableTeX(buckets, 5, 5, 35, "mph")

	for _, want := range []string{
		"5-10 & 2050 & 12.5\\%",
		"30-35 & 18 & 0.1\\%",
		"35+ & 11 & 0.1\\%",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected collapsed histogram row %q in %q", want, result)
		}
	}

	for _, unwanted := range []string{"35-40", "40-45", "45-50"} {
		if strings.Contains(result, unwanted) {
			t.Fatalf("did not expect uncapped bucket row %q in %q", unwanted, result)
		}
	}
}

func TestFormatDeltaPercent(t *testing.T) {
	tests := []struct {
		primary, compare float64
		want             string
	}{
		{30.54, 33.02, "+8.1\\%"}, // (33.02-30.54)/30.54*100 ≈ 8.1%
		{33.02, 30.54, "-7.5\\%"}, // (30.54-33.02)/33.02*100 ≈ -7.5%
		{0, 33.02, "--"},          // primary==0
		{math.NaN(), 33.02, "--"}, // NaN primary
		{30.54, math.NaN(), "--"}, // NaN compare
		{30.54, 30.54, "+0.0\\%"}, // no change
	}
	for _, tt := range tests {
		got := FormatDeltaPercent(tt.primary, tt.compare)
		if got != tt.want {
			t.Errorf("FormatDeltaPercent(%.2f, %.2f) = %q, want %q", tt.primary, tt.compare, got, tt.want)
		}
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{3460, "3,460"},
		{5915, "5,915"},
		{1000000, "1,000,000"},
		{-3460, "-3,460"},
	}
	for _, tt := range tests {
		got := FormatCount(tt.n)
		if got != tt.want {
			t.Errorf("FormatCount(%d) = %q, want %q", tt.n, tt.want, tt.want)
		}
	}
}

func TestBuildDualHistogramTableTeX(t *testing.T) {
	primary := map[float64]int64{5: 66, 10: 238, 15: 294, 20: 337}
	compare := map[float64]int64{5: 60, 10: 58, 15: 70, 20: 169}

	result := BuildDualHistogramTableTeX(primary, compare, 5, 5, 70, "mph")
	if result == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(result, "t1") || !strings.Contains(result, "t2") {
		t.Error("expected t1 and t2 column headers")
	}
	if !strings.Contains(result, "Table 2") {
		t.Error("expected Table 2 caption")
	}
	if !strings.Contains(result, "Delta") {
		t.Error("expected Delta column header")
	}
	// Bucket 5-10 row should be present with phantom padding for dash alignment.
	if !strings.Contains(result, `\phantom{0}5-10`) {
		t.Error("expected phantom-padded bucket range \\phantom{0}5-10 in output")
	}
	captionPos := strings.LastIndex(result, `Table 2: Velocity Distribution (mph)`)
	rulePos := strings.LastIndex(result, `\noindent\rule{\linewidth}{0.4pt}\par`)
	if captionPos == -1 || rulePos == -1 || captionPos < rulePos {
		t.Fatalf("comparison histogram caption should appear below the flowing rows:\n%s", result)
	}
}

func TestBuildDualHistogramTableTeX_Empty(t *testing.T) {
	result := BuildDualHistogramTableTeX(nil, nil, 5, 5, 70, "mph")
	if result != "" {
		t.Errorf("expected empty string for nil histograms, got %q", result)
	}
}
