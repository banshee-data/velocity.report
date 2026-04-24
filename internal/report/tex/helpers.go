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

// FormatDelta formats a signed delta value (primary - comparison).
// Returns "--" for NaN or Inf, otherwise "+1.23" or "-0.45".
func FormatDelta(primary, compare float64) string {
	if math.IsNaN(primary) || math.IsNaN(compare) || math.IsInf(primary, 0) || math.IsInf(compare, 0) {
		return "--"
	}
	d := primary - compare
	if d >= 0 {
		return fmt.Sprintf("+%.2f", d)
	}
	return fmt.Sprintf("%.2f", d)
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
// Uses "2006-01-02 15:04" ISO format for consistency.
func FormatTime(t time.Time, loc *time.Location) string {
	if loc != nil {
		t = t.In(loc)
	}
	return t.Format("2006-01-02 15:04")
}

// FormatDeltaPercent formats the percentage change from primary to compare.
// Positive when compare > primary. Returns "--" for invalid inputs.
// Result includes the "%" sign and leading sign: "+8.1\%" or "-3.2\%".
func FormatDeltaPercent(primary, compare float64) string {
	if math.IsNaN(primary) || math.IsNaN(compare) || math.IsInf(primary, 0) || math.IsInf(compare, 0) || primary == 0 {
		return "--"
	}
	d := (compare - primary) / primary * 100.0
	if d >= 0 {
		return fmt.Sprintf("+%.1f\\%%", d)
	}
	return fmt.Sprintf("%.1f\\%%", d)
}

// FormatCount formats an integer with thousands separators: 3460 → "3,460".
func FormatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		s = s[1:]
	}
	// Insert commas every three digits from the right.
	var b strings.Builder
	start := len(s) % 3
	if start > 0 {
		b.WriteString(s[:start])
	}
	for i := start; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	if n < 0 {
		return "-" + b.String()
	}
	return b.String()
}

// BuildDualHistogramTableTeX generates a 6-column LaTeX tabular comparing two
// histogram periods (t1 and t2). Bucket | t1 Count | t1 % | t2 Count | t2 % | Delta %
// Includes Table 2 caption. Returns empty string if both histograms are nil/empty.
func BuildDualHistogramTableTeX(primary, compare map[float64]int64, bucketSz, cutoff, maxBucket float64, units string) string {
	if len(primary) == 0 && len(compare) == 0 {
		return ""
	}

	// Collect all bucket keys from both histograms.
	keySet := make(map[float64]struct{})
	for k := range primary {
		keySet[k] = struct{}{}
	}
	for k := range compare {
		keySet[k] = struct{}{}
	}
	allKeys := make([]float64, 0, len(keySet))
	for k := range keySet {
		allKeys = append(allKeys, k)
	}
	sort.Float64s(allKeys)

	// Totals for percentage calculation.
	var totalP, totalC int64
	for _, v := range primary {
		totalP += v
	}
	for _, v := range compare {
		totalC += v
	}

	type dualRow struct {
		label string
		p, c  int64
	}
	var belowP, belowC int64
	var aboveP, aboveC int64
	var rows []dualRow

	hasUpperCap := maxBucket > 0
	for _, k := range allKeys {
		switch {
		case k < cutoff:
			belowP += primary[k]
			belowC += compare[k]
		case hasUpperCap && k >= maxBucket:
			aboveP += primary[k]
			aboveC += compare[k]
		default:
			rows = append(rows, dualRow{
				label: fmt.Sprintf("%.0f{-}%.0f", k, k+bucketSz),
				p:     primary[k],
				c:     compare[k],
			})
		}
	}

	pctP := func(n int64) string {
		if totalP == 0 {
			return "--"
		}
		return fmt.Sprintf("%.1f\\%%", float64(n)/float64(totalP)*100)
	}
	pctC := func(n int64) string {
		if totalC == 0 {
			return "--"
		}
		return fmt.Sprintf("%.1f\\%%", float64(n)/float64(totalC)*100)
	}
	delta := func(pp, cc int64) string {
		if totalP == 0 || totalC == 0 {
			return "--"
		}
		d := float64(cc)/float64(totalC)*100 - float64(pp)/float64(totalP)*100
		if d >= 0 {
			return fmt.Sprintf("+%.1f\\%%", d)
		}
		return fmt.Sprintf("%.1f\\%%", d)
	}

	escapedUnits := EscapeTeX(units)

	var b strings.Builder
	b.WriteString(`\begin{center}` + "\n")
	b.WriteString(`\begin{tabular}{>{\ttfamily}l>{\ttfamily}r>{\ttfamily}r>{\ttfamily}r>{\ttfamily}r>{\ttfamily}r}` + "\n")
	b.WriteString(`\multicolumn{1}{l}{\sffamily\bfseries \shortstack[l]{Bucket \\ (` + escapedUnits + `)}}`)
	b.WriteString(`&\multicolumn{1}{r}{\sffamily\bfseries \shortstack[r]{t1 \\ Count}}`)
	b.WriteString(`&\multicolumn{1}{r}{\sffamily\bfseries \shortstack[r]{t1 \\ Percent}}`)
	b.WriteString(`&\multicolumn{1}{r}{\sffamily\bfseries \shortstack[r]{t2 \\ Count}}`)
	b.WriteString(`&\multicolumn{1}{r}{\sffamily\bfseries \shortstack[r]{t2 \\ Percent}}`)
	b.WriteString(`&\multicolumn{1}{r}{\sffamily\bfseries Delta}\\` + "\n")
	b.WriteString(`\hline` + "\n")

	if belowP > 0 || belowC > 0 {
		b.WriteString(fmt.Sprintf("$<$%.0f&%d&%s&%d&%s&%s\\\\\n",
			cutoff, belowP, pctP(belowP), belowC, pctC(belowC), delta(belowP, belowC)))
	}
	for _, row := range rows {
		b.WriteString(fmt.Sprintf("%s&%d&%s&%d&%s&%s\\\\\n",
			row.label, row.p, pctP(row.p), row.c, pctC(row.c), delta(row.p, row.c)))
	}
	if aboveP > 0 || aboveC > 0 {
		b.WriteString(fmt.Sprintf("%.0f+&%d&%s&%d&%s&%s\\\\\n",
			maxBucket, aboveP, pctP(aboveP), aboveC, pctC(aboveC), delta(aboveP, aboveC)))
	}

	b.WriteString(`\hline` + "\n")
	b.WriteString(`\end{tabular}` + "\n")
	b.WriteString(`\par\vspace{2pt}` + "\n")
	b.WriteString(`\noindent\makebox[\linewidth]{\textbf{\small Table 2: Velocity Distribution (` + escapedUnits + `)}}` + "\n")
	b.WriteString(`\end{center}` + "\n")
	return b.String()
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
	b.WriteString(`\begin{tabular}{rrr}` + "\n")
	b.WriteString(`\hline` + "\n")
	b.WriteString(`\textbf{Bucket (` + EscapeTeX(units) + `)} & \textbf{Count} & \textbf{Percent} \\` + "\n")
	b.WriteString(`\hline` + "\n")

	// Pre-aggregate below-cutoff and above-max buckets.
	var belowCount, aboveCount int64
	type displayRow struct {
		label string
		count int64
	}
	var rows []displayRow

	hasUpperCap := maxBucket > 0
	for _, k := range keys {
		count := buckets[k]
		switch {
		case k < cutoff:
			belowCount += count
		case hasUpperCap && k >= maxBucket:
			aboveCount += count
		default:
			rows = append(rows, displayRow{
				label: fmt.Sprintf(`%.0f\textemdash{}%.0f`, k, k+bucketSz),
				count: count,
			})
		}
	}

	// Emit aggregated below-cutoff row first.
	if belowCount > 0 {
		pct := float64(belowCount) / float64(total) * 100.0
		b.WriteString(fmt.Sprintf("$<$%.0f & %d & %.1f\\%% \\\\\n", cutoff, belowCount, pct))
	}

	// Emit normal range rows.
	for _, row := range rows {
		pct := float64(row.count) / float64(total) * 100.0
		b.WriteString(fmt.Sprintf("%s & %d & %.1f\\%% \\\\\n", row.label, row.count, pct))
	}

	// Emit aggregated above-max row last.
	if aboveCount > 0 {
		pct := float64(aboveCount) / float64(total) * 100.0
		b.WriteString(fmt.Sprintf("%.0f+ & %d & %.1f\\%% \\\\\n", maxBucket, aboveCount, pct))
	}

	b.WriteString(`\hline` + "\n")
	b.WriteString(`\end{tabular}` + "\n")
	return b.String()
}
