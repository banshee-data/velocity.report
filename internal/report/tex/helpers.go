package tex

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// EscapeTeX escapes special LaTeX characters in s.
// Characters escaped: & % $ # _ { } ~ ^ \
func EscapeTeX(s string) string {
	// Backslash is replaced first via a sentinel to avoid double-escaping
	// the braces introduced by \textbackslash{}.
	const sentinel = "\x00BACKSLASH\x00"
	s = strings.ReplaceAll(s, `\`, sentinel)
	s = strings.ReplaceAll(s, `&`, `\&`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	s = strings.ReplaceAll(s, `#`, `\#`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	s = strings.ReplaceAll(s, `{`, `\{`)
	s = strings.ReplaceAll(s, `}`, `\}`)
	s = strings.ReplaceAll(s, `~`, `\textasciitilde{}`)
	s = strings.ReplaceAll(s, `^`, `\textasciicircum{}`)
	s = strings.ReplaceAll(s, sentinel, `\textbackslash{}`)
	return s
}

// FormatNumber formats a float for LaTeX display.
// Returns "--" for NaN or Inf, otherwise "%.2f".
func FormatNumber(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "--"
	}
	return fmt.Sprintf("%.2f", v)
}

// FormatPercent formats a float as a percentage.
// Returns "--" for NaN or Inf, otherwise "%.1f%%".
func FormatPercent(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "--"
	}
	return fmt.Sprintf("%.1f%%", v)
}

// FormatTime formats a time for the stats table.
// Uses "1/2 15:04" format (no zero-padding on month/day).
func FormatTime(t time.Time, loc *time.Location) string {
	if loc != nil {
		t = t.In(loc)
	}
	return fmt.Sprintf("%d/%d %s", t.Month(), t.Day(), t.Format("15:04"))
}

// BuildHistogramTableTeX generates LaTeX tabular content for histogram data.
// Produces a table with Bucket | Count | Percent columns.
// Includes <N row for below-cutoff data and N+ row for above-max data.
func BuildHistogramTableTeX(buckets map[float64]int64, bucketSz, cutoff, maxBucket float64, units string) string {
	if len(buckets) == 0 {
		return ""
	}

	// Collect and sort bucket keys.
	keys := make([]float64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	// Total count for percentages.
	var total int64
	for _, c := range buckets {
		total += c
	}
	if total == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`\begin{tabular}{lrr}` + "\n")
	b.WriteString(`\hline` + "\n")
	b.WriteString(`\textbf{` + EscapeTeX(units) + `} & \textbf{Count} & \textbf{\%} \\` + "\n")
	b.WriteString(`\hline` + "\n")

	for _, k := range keys {
		count := buckets[k]
		pct := float64(count) / float64(total) * 100.0

		var label string
		switch {
		case k < cutoff:
			label = fmt.Sprintf("$<$%.0f", cutoff)
		case k >= maxBucket:
			label = fmt.Sprintf("%.0f+", maxBucket)
		default:
			label = fmt.Sprintf("%.0f--%.0f", k, k+bucketSz)
		}

		b.WriteString(fmt.Sprintf("%s & %d & %.1f\\%% \\\\\n", label, count, pct))
	}

	b.WriteString(`\hline` + "\n")
	b.WriteString(`\end{tabular}` + "\n")
	return b.String()
}
