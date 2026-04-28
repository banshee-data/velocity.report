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

// FormatTime formats a compact local timestamp for the stats table.
func FormatTime(t time.Time, loc *time.Location) string {
	if loc != nil {
		t = t.In(loc)
	}
	return t.Format("1/2 15:04")
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

const tableStripeColour = "black!2"

func tableCaptionTeX(caption string) string {
	return `\noindent\makebox[\linewidth]{{\normalfont\bfseries\small ` + EscapeTeX(caption) + `}}`
}

func withStyledTable(b *strings.Builder, fontSize string, body func(), afterReset func()) {
	b.WriteString("{\n")
	b.WriteString(`\AtkinsonMono\` + fontSize + "\n")
	b.WriteString(`\renewcommand{\arraystretch}{1.04}` + "\n")
	b.WriteString(`\setlength{\tabcolsep}{2pt}` + "\n")
	b.WriteString(`\rowcolors{2}{` + tableStripeColour + `}{white}` + "\n")
	body()
	b.WriteString(`\rowcolors{0}{}{}` + "\n")
	if afterReset != nil {
		afterReset()
	}
	b.WriteString("}\n")
}

// BuildSingleKeyMetricsTableTeX generates the styled 2-column key metrics
// tabular (Metric | Value) for single-survey mode. Inputs are pre-formatted
// display strings (e.g. "25.00") and must already be TeX-escaped where needed.
func BuildSingleKeyMetricsTableTeX(p50, p85, p98, maxSpeed, units string) string {
	var b strings.Builder
	withStyledTable(&b, "small", func() {
		b.WriteString(`\begin{tabular}{lr}` + "\n")
		b.WriteString(`{\sffamily\bfseries Metric} & {\sffamily\bfseries Value} \\` + "\n")
		b.WriteString(`\hline` + "\n")
		fmt.Fprintf(&b, "p50 Velocity & %s %s \\\\\n", p50, units)
		fmt.Fprintf(&b, "p85 Velocity & %s %s \\\\\n", p85, units)
		fmt.Fprintf(&b, "p98 Velocity & %s %s \\\\\n", p98, units)
		fmt.Fprintf(&b, "Max Velocity & %s %s \\\\\n", maxSpeed, units)
		b.WriteString(`\end{tabular}` + "\n")
	}, func() {
		b.WriteString(`\par\vspace{2pt}` + "\n")
		b.WriteString(tableCaptionTeX("Table 1: Key Metrics") + "\n")
	})
	return b.String()
}

// BuildComparisonKeyMetricsTableTeX generates the 4-column key metrics tabular
// (Metric | Period t1 | Period t2 | Change) for comparison mode, with Table 1
// caption. All inputs are pre-formatted display strings and must already be
// TeX-escaped where needed.
func BuildComparisonKeyMetricsTableTeX(
	p50, p85, p98, maxSpeed string,
	compareP50, compareP85, compareP98, compareMax string,
	deltaP50Pct, deltaP85Pct, deltaP98Pct, deltaMaxPct string,
	totalCountFmt, compareTotalCountFmt string,
	units string,
) string {
	var b strings.Builder
	withStyledTable(&b, "small", func() {
		b.WriteString(`\begin{tabular}{lrrr}` + "\n")
		b.WriteString(`{\sffamily\bfseries Metric} & {\sffamily\bfseries Period t1} & {\sffamily\bfseries Period t2} & {\sffamily\bfseries Change} \\` + "\n")
		b.WriteString(`\hline` + "\n")
		fmt.Fprintf(&b, "p50 Velocity & %s %s & %s %s & %s \\\\\n", p50, units, compareP50, units, deltaP50Pct)
		fmt.Fprintf(&b, "p85 Velocity & %s %s & %s %s & %s \\\\\n", p85, units, compareP85, units, deltaP85Pct)
		fmt.Fprintf(&b, "p98 Velocity & %s %s & %s %s & %s \\\\\n", p98, units, compareP98, units, deltaP98Pct)
		fmt.Fprintf(&b, "Max Velocity & %s %s & %s %s & %s \\\\\n", maxSpeed, units, compareMax, units, deltaMaxPct)
		fmt.Fprintf(&b, "Vehicle Count & %s & %s & \\\\\n", totalCountFmt, compareTotalCountFmt)
		b.WriteString(`\end{tabular}` + "\n")
	}, func() {
		b.WriteString(`\par\vspace{2pt}` + "\n")
		b.WriteString(tableCaptionTeX("Table 1: Key Metrics") + "\n")
	})
	b.WriteString(`\par` + "\n")
	return b.String()
}

// BuildStatTableTeX generates a styled, page-spanning LaTeX supertabular for
// stat row data (Time | Count | p50 | p85 | p98 | Max). caption is rendered as
// the table label below the table. Returns empty string if rows is nil or empty.
func BuildStatTableTeX(rows []StatRow, caption, units string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	escapedUnits := EscapeTeX(units)
	withStyledTable(&b, "scriptsize", func() {
		b.WriteString(`\tablehead{%` + "\n")
		b.WriteString("  \\hline\n")
		b.WriteString(`  {\sffamily\bfseries\footnotesize Start Time} & {\sffamily\bfseries\footnotesize Count} & {\sffamily\bfseries\footnotesize \shortstack[r]{p50 \\ (` + escapedUnits + `)}} & {\sffamily\bfseries\footnotesize \shortstack[r]{p85 \\ (` + escapedUnits + `)}} & {\sffamily\bfseries\footnotesize \shortstack[r]{p98 \\ (` + escapedUnits + `)}} & {\sffamily\bfseries\footnotesize \shortstack[r]{Max \\ (` + escapedUnits + `)}} \\` + "\n")
		b.WriteString("  \\hline\n")
		b.WriteString("}\n")
		b.WriteString(`\tabletail{\hline}` + "\n")
		b.WriteString(`\begin{center}` + "\n")
		b.WriteString(`\begin{supertabular}{>{\raggedright\arraybackslash}p{0.29\linewidth}>{\raggedleft\arraybackslash}p{0.10\linewidth}>{\raggedleft\arraybackslash}p{0.12\linewidth}>{\raggedleft\arraybackslash}p{0.12\linewidth}>{\raggedleft\arraybackslash}p{0.12\linewidth}>{\raggedleft\arraybackslash}p{0.12\linewidth}}` + "\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "%s & %d & %s & %s & %s & %s \\\\\n",
				EscapeTeX(row.StartTime), row.Count, row.P50, row.P85, row.P98, row.MaxSpeed)
		}
		b.WriteString(`\end{supertabular}` + "\n")
		b.WriteString(`\end{center}` + "\n")
	}, func() {
		b.WriteString(`\par\vspace{2pt}` + "\n")
		b.WriteString(tableCaptionTeX(caption) + "\n")
	})
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
			loStr := fmt.Sprintf("%.0f", k)
			if len(loStr) < 2 {
				// Pad single-digit bucket starts so dashes align with two-digit rows.
				loStr = `\phantom{0}` + loStr
			}
			rows = append(rows, dualRow{
				label: loStr + `-` + fmt.Sprintf("%.0f", k+bucketSz),
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
	withStyledTable(&b, "small", func() {
		b.WriteString(`\begin{center}` + "\n")
		b.WriteString(`\begin{tabular}{lrrrrr}` + "\n")
		b.WriteString(`\hline` + "\n")
		b.WriteString(`{\sffamily\bfseries\footnotesize \shortstack[l]{Bucket \\ (` + escapedUnits + `)}}`)
		b.WriteString(` & {\sffamily\bfseries\footnotesize \shortstack[r]{t1 \\ Count}}`)
		b.WriteString(` & {\sffamily\bfseries\footnotesize \shortstack[r]{t1 \\ \%}}`)
		b.WriteString(` & {\sffamily\bfseries\footnotesize \shortstack[r]{t2 \\ Count}}`)
		b.WriteString(` & {\sffamily\bfseries\footnotesize \shortstack[r]{t2 \\ \%}}`)
		b.WriteString(` & {\sffamily\bfseries\footnotesize Delta} \\` + "\n")
		b.WriteString(`\hline` + "\n")

		if belowP > 0 || belowC > 0 {
			fmt.Fprintf(&b, "$<$%.0f & %d & %s & %d & %s & %s \\\\\n",
				cutoff, belowP, pctP(belowP), belowC, pctC(belowC), delta(belowP, belowC))
		}
		for _, row := range rows {
			fmt.Fprintf(&b, "%s & %d & %s & %d & %s & %s \\\\\n",
				row.label, row.p, pctP(row.p), row.c, pctC(row.c), delta(row.p, row.c))
		}
		if aboveP > 0 || aboveC > 0 {
			fmt.Fprintf(&b, "%.0f+ & %d & %s & %d & %s & %s \\\\\n",
				maxBucket, aboveP, pctP(aboveP), aboveC, pctC(aboveC), delta(aboveP, aboveC))
		}

		b.WriteString(`\hline` + "\n")
		b.WriteString(`\end{tabular}` + "\n")
		b.WriteString(`\end{center}` + "\n")
	}, func() {
		b.WriteString(`\par\vspace{2pt}` + "\n")
		b.WriteString(tableCaptionTeX("Table 2: Velocity Distribution ("+units+")") + "\n")
	})
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
	// Opening group: same visual style as BuildStatTableTeX (alternating row
	// colours, grey column rules, monospace scriptsize with sans-serif headers).
	withStyledTable(&b, "small", func() {
		b.WriteString(`\begin{center}` + "\n")
		b.WriteString(`\begin{tabular}{lrr}` + "\n")
		b.WriteString(`\hline` + "\n")
		b.WriteString(
			`{\sffamily\bfseries\footnotesize \shortstack[l]{Bucket \\ (` + EscapeTeX(units) + `)}} & ` +
				`{\sffamily\bfseries\footnotesize Count} & ` +
				`{\sffamily\bfseries\footnotesize Percent} \\` + "\n")
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
				loStr := fmt.Sprintf("%.0f", k)
				if len(loStr) < 2 {
					// Pad single-digit bucket starts so dashes align with two-digit rows.
					loStr = `\phantom{0}` + loStr
				}
				rows = append(rows, displayRow{
					label: loStr + `-` + fmt.Sprintf("%.0f", k+bucketSz),
					count: count,
				})
			}
		}

		// Emit aggregated below-cutoff row first.
		if belowCount > 0 {
			pct := float64(belowCount) / float64(total) * 100.0
			fmt.Fprintf(&b, "$<$%.0f & %d & %.1f\\%% \\\\\n", cutoff, belowCount, pct)
		}

		// Emit normal range rows.
		for _, row := range rows {
			pct := float64(row.count) / float64(total) * 100.0
			fmt.Fprintf(&b, "%s & %d & %.1f\\%% \\\\\n", row.label, row.count, pct)
		}

		// Emit aggregated above-max row last.
		if aboveCount > 0 {
			pct := float64(aboveCount) / float64(total) * 100.0
			fmt.Fprintf(&b, "%.0f+ & %d & %.1f\\%% \\\\\n", maxBucket, aboveCount, pct)
		}

		b.WriteString(`\hline` + "\n")
		b.WriteString(`\end{tabular}` + "\n")
		b.WriteString(`\end{center}` + "\n")
	}, func() {
		b.WriteString(`\par\vspace{2pt}` + "\n")
		b.WriteString(tableCaptionTeX("Table 2: Velocity Distribution ("+units+")") + "\n")
	})
	return b.String()
}
