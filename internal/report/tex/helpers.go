package tex

import (
	"fmt"
	"math"
	"sort"
	"strconv"
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
	b.WriteString(`\renewcommand{\arraystretch}{1.00}` + "\n")
	b.WriteString(`\setlength{\tabcolsep}{2pt}` + "\n")
	b.WriteString(`\setlength{\fboxsep}{0pt}` + "\n")
	b.WriteString(`\rowcolors{2}{` + tableStripeColour + `}{white}` + "\n")
	body()
	b.WriteString(`\rowcolors{0}{}{}` + "\n")
	if afterReset != nil {
		afterReset()
	}
	b.WriteString("}\n")
}

type tableAlignment string

const (
	tableAlignLeft  tableAlignment = "left"
	tableAlignRight tableAlignment = "right"
)

type tableColumn struct {
	header      string
	width       string
	align       tableAlignment
	headerAlign tableAlignment
	headerBoxW  string
}

type reportTable struct {
	columns   []tableColumn
	rows      [][]string
	caption   string
	pageBreak bool
}

func renderReportTable(t reportTable) string {
	var b strings.Builder
	withStyledTable(&b, "small", func() {
		spec := tableColumnSpec(t.columns)
		if t.pageBreak {
			writeFlowTable(&b, t.columns, t.rows)
			return
		}

		b.WriteString(`\noindent` + "\n")
		b.WriteString(`\begin{tabular}{` + spec + `}` + "\n")
		writeTableHeader(&b, t.columns)
		b.WriteString(`\hline` + "\n")
		for _, row := range t.rows {
			writeTableRow(&b, row)
		}
		b.WriteString(`\hline` + "\n")
		b.WriteString(`\end{tabular}` + "\n")
	}, func() {
		if t.caption != "" {
			b.WriteString(`\par\vspace{2pt}` + "\n")
			b.WriteString(tableCaptionTeX(t.caption) + "\n")
		}
	})
	return b.String()
}

func tableColumnSpec(columns []tableColumn) string {
	var b strings.Builder
	b.WriteString("@{}")
	for _, col := range columns {
		switch col.align {
		case tableAlignRight:
			b.WriteString(`>{\raggedleft\arraybackslash}p{` + col.width + `}`)
		default:
			b.WriteString(`>{\raggedright\arraybackslash}p{` + col.width + `}`)
		}
	}
	b.WriteString("@{}")
	return b.String()
}

func writeFlowTable(b *strings.Builder, columns []tableColumn, rows [][]string) {
	b.WriteString(`\noindent`)
	writeFlowCells(b, columns, headerCells(columns), true)
	b.WriteString(`\par` + "\n")
	b.WriteString(`\noindent\rule{\linewidth}{0.4pt}\par` + "\n")
	for i, row := range rows {
		b.WriteString(`\noindent`)
		if i%2 == 0 {
			b.WriteString(`\colorbox{` + tableStripeColour + `}{`)
			writeFlowCells(b, columns, row, false)
			b.WriteString(`}`)
		} else {
			writeFlowCells(b, columns, row, false)
		}
		b.WriteString(`\par` + "\n")
	}
	b.WriteString(`\noindent\rule{\linewidth}{0.4pt}\par` + "\n")
}

func headerCells(columns []tableColumn) []string {
	cells := make([]string, len(columns))
	for i, col := range columns {
		cells[i] = col.header
	}
	return cells
}

func writeFlowCells(b *strings.Builder, columns []tableColumn, cells []string, header bool) {
	b.WriteString(`\makebox[\linewidth][l]{`)
	for i, col := range columns {
		if i > 0 {
			b.WriteString(`\hspace{2\tabcolsep}`)
		}
		align := "l"
		if col.align == tableAlignRight {
			align = "r"
		}
		b.WriteString(`\makebox[` + col.width + `][` + align + `]{\strut `)
		if header {
			b.WriteString(`\sffamily\bfseries `)
		}
		if i < len(cells) {
			b.WriteString(cells[i])
		}
		b.WriteString(`}`)
	}
	b.WriteString(`}`)
}

func writeTableHeader(b *strings.Builder, columns []tableColumn) {
	for i, col := range columns {
		if i > 0 {
			b.WriteString(" & ")
		}
		if col.headerAlign != "" {
			b.WriteString(`\multicolumn{1}{` + headerColumnSpec(col) + `}{`)
			if col.headerBoxW != "" {
				b.WriteString(`\makebox[\linewidth][r]{\makebox[` + col.headerBoxW + `][l]{\sffamily\bfseries ` + col.header + `}}`)
			} else {
				b.WriteString(`\sffamily\bfseries ` + col.header)
			}
			b.WriteString(`}`)
			continue
		}
		b.WriteString(`{\sffamily\bfseries `)
		b.WriteString(col.header + `}`)
	}
	b.WriteString(` \\` + "\n")
}

func headerColumnSpec(col tableColumn) string {
	switch col.headerAlign {
	case tableAlignRight:
		return `>{\raggedleft\arraybackslash}p{` + col.width + `}`
	default:
		return `>{\raggedright\arraybackslash}p{` + col.width + `}`
	}
}

func writeTableRow(b *strings.Builder, row []string) {
	for i, cell := range row {
		if i > 0 {
			b.WriteString(" & ")
		}
		b.WriteString(cell)
	}
	b.WriteString(` \\` + "\n")
}

// BuildSingleKeyMetricsTableTeX generates the styled 2-column key metrics
// tabular (Metric | Value) for single-survey mode. Inputs are pre-formatted
// display strings (e.g. "25.00") and must already be TeX-escaped where needed.
func BuildSingleKeyMetricsTableTeX(p50, p85, p98, maxSpeed, units string) string {
	units = EscapeTeX(units)
	return renderReportTable(reportTable{
		columns: []tableColumn{
			{header: "Metric", width: `0.55\linewidth`, align: tableAlignLeft},
			{header: "Value", width: `0.42\linewidth`, align: tableAlignRight},
		},
		rows: [][]string{
			{"p50 Velocity", fmt.Sprintf("%s %s", p50, units)},
			{"p85 Velocity", fmt.Sprintf("%s %s", p85, units)},
			{"p98 Velocity", fmt.Sprintf("%s %s", p98, units)},
			{"Max Velocity", fmt.Sprintf("%s %s", maxSpeed, units)},
		},
		caption: "Table 1: Key Metrics",
	})
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
	escapedUnits := EscapeTeX(units)
	table := renderReportTable(reportTable{
		columns: []tableColumn{
			{header: "Metric", width: `0.31\linewidth`, align: tableAlignLeft},
			{header: "Period t1", width: `0.22\linewidth`, align: tableAlignRight, headerAlign: tableAlignRight, headerBoxW: `5.8em`},
			{header: "Period t2", width: `0.22\linewidth`, align: tableAlignRight, headerAlign: tableAlignRight, headerBoxW: `5.8em`},
			{header: "Change", width: `0.19\linewidth`, align: tableAlignRight},
		},
		rows: [][]string{
			{"Vehicle Count", countWithUnitPhantom(totalCountFmt, escapedUnits), countWithUnitPhantom(compareTotalCountFmt, escapedUnits), ""},
			{"p50 Velocity", fmt.Sprintf("%s %s", p50, escapedUnits), fmt.Sprintf("%s %s", compareP50, escapedUnits), deltaP50Pct},
			{"p85 Velocity", fmt.Sprintf("%s %s", p85, escapedUnits), fmt.Sprintf("%s %s", compareP85, escapedUnits), deltaP85Pct},
			{"p98 Velocity", fmt.Sprintf("%s %s", p98, escapedUnits), fmt.Sprintf("%s %s", compareP98, escapedUnits), deltaP98Pct},
			{"Max Velocity", fmt.Sprintf("%s %s", maxSpeed, escapedUnits), fmt.Sprintf("%s %s", compareMax, escapedUnits), deltaMaxPct},
		},
		caption: "Table 1: Key Metrics",
	})
	return table + `\par` + "\n"
}

func countWithUnitPhantom(count, escapedUnits string) string {
	if count == "" || escapedUnits == "" {
		return count
	}
	return count + `\phantom{ ` + escapedUnits + `}`
}

// BuildStatTableTeX generates a styled, page-spanning LaTeX flow table for
// stat row data (Time | Count | p50 | p85 | p98 | Max). The table caption is
// rendered below the flowing rows. Returns empty string if rows is nil or
// empty.
func BuildStatTableTeX(rows []StatRow, caption, units string) string {
	if len(rows) == 0 {
		return ""
	}
	escapedUnits := EscapeTeX(units)
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			statStartTimeTeX(row.StartTime),
			fmt.Sprintf("%d", row.Count),
			row.P50,
			row.P85,
			row.P98,
			row.MaxSpeed,
		})
	}
	table := reportTable{
		columns: []tableColumn{
			{header: "Start Time", width: `0.24\linewidth`, align: tableAlignLeft},
			{header: "Count", width: `0.12\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{p50 \\ (` + escapedUnits + `)}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{p85 \\ (` + escapedUnits + `)}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{p98 \\ (` + escapedUnits + `)}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{Max \\ (` + escapedUnits + `)}`, width: `0.14\linewidth`, align: tableAlignRight},
		},
		rows:      tableRows,
		caption:   caption,
		pageBreak: true,
	}
	return renderReportTable(table)
}

func statStartTimeTeX(s string) string {
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		return EscapeTeX(s)
	}
	dateParts := strings.Split(parts[0], "/")
	if len(dateParts) != 2 {
		return EscapeTeX(s)
	}
	month, monthErr := strconv.Atoi(dateParts[0])
	day, dayErr := strconv.Atoi(dateParts[1])
	if monthErr != nil || dayErr != nil {
		return EscapeTeX(s)
	}
	return paddedDecimalTeX(month) + "/" + paddedDecimalTeX(day) + " " + paddedClockTeX(parts[1])
}

func paddedDecimalTeX(n int) string {
	if n >= 0 && n < 10 {
		return `\phantom{0}` + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func paddedClockTeX(clock string) string {
	parts := strings.SplitN(clock, ":", 2)
	if len(parts) != 2 {
		return EscapeTeX(clock)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return EscapeTeX(clock)
	}
	if hour >= 0 && hour < 10 && len(parts[0]) == 1 {
		return `\phantom{0}` + EscapeTeX(parts[0]) + ":" + EscapeTeX(parts[1])
	}
	return EscapeTeX(parts[0]) + ":" + EscapeTeX(parts[1])
}

// BuildDualHistogramTableTeX generates a 6-column LaTeX table comparing two
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

	var tableRows [][]string
	if belowP > 0 || belowC > 0 {
		tableRows = append(tableRows, []string{
			fmt.Sprintf("$<$%.0f", cutoff),
			fmt.Sprintf("%d", belowP), pctP(belowP),
			fmt.Sprintf("%d", belowC), pctC(belowC),
			delta(belowP, belowC),
		})
	}
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.label,
			fmt.Sprintf("%d", row.p), pctP(row.p),
			fmt.Sprintf("%d", row.c), pctC(row.c),
			delta(row.p, row.c),
		})
	}
	if aboveP > 0 || aboveC > 0 {
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%.0f+", maxBucket),
			fmt.Sprintf("%d", aboveP), pctP(aboveP),
			fmt.Sprintf("%d", aboveC), pctC(aboveC),
			delta(aboveP, aboveC),
		})
	}

	escapedUnits := EscapeTeX(units)
	return renderReportTable(reportTable{
		columns: []tableColumn{
			{header: `\shortstack[l]{Bucket \\ (` + escapedUnits + `)}`, width: `0.15\linewidth`, align: tableAlignLeft},
			{header: `\shortstack[r]{t1 \\ Count}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{t1 \\ \%}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{t2 \\ Count}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `\shortstack[r]{t2 \\ \%}`, width: `0.14\linewidth`, align: tableAlignRight},
			{header: `Delta`, width: `0.21\linewidth`, align: tableAlignRight},
		},
		rows:      tableRows,
		caption:   "Table 2: Velocity Distribution (" + escapedUnits + ")",
		pageBreak: true,
	})
}

// BuildHistogramTableTeX generates LaTeX table content for histogram data.
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

	var tableRows [][]string
	if belowCount > 0 {
		pct := float64(belowCount) / float64(total) * 100.0
		tableRows = append(tableRows, []string{fmt.Sprintf("$<$%.0f", cutoff), fmt.Sprintf("%d", belowCount), fmt.Sprintf("%.1f\\%%", pct)})
	}
	for _, row := range rows {
		pct := float64(row.count) / float64(total) * 100.0
		tableRows = append(tableRows, []string{row.label, fmt.Sprintf("%d", row.count), fmt.Sprintf("%.1f\\%%", pct)})
	}
	if aboveCount > 0 {
		pct := float64(aboveCount) / float64(total) * 100.0
		tableRows = append(tableRows, []string{fmt.Sprintf("%.0f+", maxBucket), fmt.Sprintf("%d", aboveCount), fmt.Sprintf("%.1f\\%%", pct)})
	}

	escapedUnits := EscapeTeX(units)
	return renderReportTable(reportTable{
		columns: []tableColumn{
			{header: `\shortstack[l]{Bucket \\ (` + escapedUnits + `)}`, width: `0.35\linewidth`, align: tableAlignLeft},
			{header: `Count`, width: `0.29\linewidth`, align: tableAlignRight},
			{header: `Percent`, width: `0.32\linewidth`, align: tableAlignRight},
		},
		rows:    tableRows,
		caption: "Table 2: Velocity Distribution (" + units + ")",
	})
}
