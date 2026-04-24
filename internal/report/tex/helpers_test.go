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
	for _, want := range []string{`\hline`, `\begin{tabular}`, `\end{tabular}`, "50+"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q", want)
		}
	}

	// Count data rows (lines ending with \\).
	dataRows := 0
	for _, l := range strings.Split(result, "\n") {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.Contains(l, `\textbf{`) || strings.Contains(l, `\hline`) ||
			strings.Contains(l, `\begin{`) || strings.Contains(l, `\end{`) {
			continue
		}
		dataRows++
	}
	if dataRows != 5 {
		t.Errorf("expected 5 data rows, got %d", dataRows)
	}
}

func TestBuildHistogramTableTeX_Empty(t *testing.T) {
	result := BuildHistogramTableTeX(nil, 5, 10, 50, "mph")
	if result != "" {
		t.Errorf("expected empty string for nil buckets, got %q", result)
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
	// Bucket 5{-}10 row should be present.
	if !strings.Contains(result, "5{-}10") {
		t.Error("expected bucket range 5{-}10 in output")
	}
}

func TestBuildDualHistogramTableTeX_Empty(t *testing.T) {
	result := BuildDualHistogramTableTeX(nil, nil, 5, 5, 70, "mph")
	if result != "" {
		t.Errorf("expected empty string for nil histograms, got %q", result)
	}
}
