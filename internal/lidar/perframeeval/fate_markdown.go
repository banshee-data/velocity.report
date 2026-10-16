package perframeeval

import (
	"fmt"
	"strings"
)

// RenderFateMarkdown writes the fate report as Markdown: the header says
// what was joined and how well, then the fate shares by class, range band
// and object, then what the gaps between fragments are made of, and the
// clustered share comparable with the retention measure.
//
// Fate columns are the fates any measured return has, in report order, so a
// table has no column of zeros; the JSON report carries every fate.
func RenderFateMarkdown(r FateReport) string {
	var b strings.Builder
	b.WriteString("# Fate of every reviewed return\n\n")
	ref, ev, j := r.Reference, r.Evidence, r.Join
	fmt.Fprintf(&b, "- Reference: pack %s, split %s (role %s, held out: %v), %d episode(s), digest %s\n",
		ref.DatasetID, ref.Split, ref.SplitRole, ref.HeldOut, len(ref.Episodes), ref.Digest)
	fmt.Fprintf(&b, "- Evidence: %s (%s, %s), %d frames, %d gaps; written by %s (%s), params %s\n",
		ev.Container, ev.Profile, ev.State, ev.Frames, ev.Gaps, ev.BuildVersion, ev.BuildGitSHA, ev.ParamsHash)
	fmt.Fprintf(&b, "- Capture: %s, %d packets, %s; scoring from %d ns (window start %.3g s)\n",
		ev.Capture, ev.CapturePackets, ev.CaptureSHA256, ev.ScoringStartNs, float64(ev.WindowStartNs)/1e9)
	fmt.Fprintf(&b, "- Join: %s; %d of %d reviewed returns matched or proven absent (match rate %.4f): "+
		"%d matched, %d absent, %d in a neighbouring frame, %d unresolved; %d of %d matched have the pack's float32 coordinates\n",
		r.Method, j.Matched+j.ProvenAbsent, j.MaskReturns, j.MatchRate, j.Matched, j.ProvenAbsent, j.OtherFrame, j.Unresolved,
		j.CoordinatesAgree, j.Matched)
	fmt.Fprintf(&b, "- Foreground agreement over the %d measured samples: %d of %d pack returns (%.3f) are in the replay's foreground; "+
		"%d of its %d foreground returns (%.3f) are in the pack\n",
		ev.MeasuredSamples, j.PackReturnsInDomain, j.PackReturns, j.PackInDomainShare, j.PackReturnsInDomain, j.DomainReturns, j.DomainInPackShare)
	if len(ev.MeasuredCompleteness) > 0 {
		parts := make([]string, 0, len(ev.MeasuredCompleteness))
		for _, k := range sortedMapKeys(ev.MeasuredCompleteness) {
			parts = append(parts, fmt.Sprintf("%s %d", k, ev.MeasuredCompleteness[k]))
		}
		fmt.Fprintf(&b, "- Measured frames by completeness: %s\n", strings.Join(parts, ", "))
	}
	if len(r.NotMeasured) > 0 {
		parts := make([]string, len(r.NotMeasured))
		for i, nm := range r.NotMeasured {
			parts[i] = fmt.Sprintf("%s %d instants at %d samples", nm.Reason, nm.Instants, nm.Samples)
		}
		fmt.Fprintf(&b, "- Not measured: %s\n", strings.Join(parts, "; "))
	}
	b.WriteString("\nShares are of each group's reviewed returns. Missed instants reached no cluster; fragmented is a share of " +
		"the instants that reached one.\n\n")

	cols := fateColumns(r.Total.Fates)
	writeFateTable(&b, "Group", append([]FateSummary{r.Total}, r.ByClass...), cols, false)
	b.WriteString("\n")
	writeFateTable(&b, "Range band", r.ByRange, cols, false)
	b.WriteString("\n")
	writeFateTable(&b, "Object", r.ByObject, cols, true)

	b.WriteString("\n## What the gaps between fragments are made of\n\n")
	b.WriteString("For each instant split over two clusters or more, the object's returns that are in none of its fragments, by fate.\n\n")
	gapCols := fateColumns(r.Total.GapFates)
	var gapRows []FateSummary
	for _, s := range append(append([]FateSummary{r.Total}, r.ByClass...), r.ByRange...) {
		if s.Fragmented > 0 {
			gapRows = append(gapRows, s)
		}
	}
	var gapObjects []FateSummary
	for _, s := range r.ByObject {
		if s.Fragmented > 0 {
			gapObjects = append(gapObjects, s)
		}
	}
	if len(gapRows) == 0 {
		b.WriteString("No instant was fragmented.\n")
	} else {
		writeGapTable(&b, "Group", gapRows, gapCols, false)
		b.WriteString("\n")
		writeGapTable(&b, "Object", gapObjects, gapCols, true)
	}

	b.WriteString("\n## Comparable with the retention measure\n\n")
	b.WriteString("Instants whose frame holds at least one accepted cluster, the only ones the retention measure's evidence has: " +
		"clustered returns over returns, counted exactly.\n\n")
	b.WriteString("| Group | Instants | Returns | Clustered | Retention |\n| --- | ---: | ---: | ---: | ---: |\n")
	for _, s := range append([]FateSummary{r.Total}, r.ByClass...) {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %s |\n", s.Name, s.ComparableInstants, s.ComparableReturns, s.ComparableClustered,
			shareOrDash(s.ComparableRetention, s.ComparableReturns))
	}

	if len(r.Caveats) > 0 {
		b.WriteString("\n## Caveats\n\n")
		for _, c := range r.Caveats {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	return b.String()
}

// fateColumns lists the fates with any return in counts, in report order.
func fateColumns(counts map[string]int) []string {
	var cols []string
	for _, name := range fateNames {
		if counts[name] > 0 {
			cols = append(cols, name)
		}
	}
	return cols
}

func shareOrDash(share float64, n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", share)
}

func writeFateTable(b *strings.Builder, first string, rows []FateSummary, cols []string, withClass bool) {
	classCol, classSep := "", ""
	if withClass {
		classCol, classSep = " Class |", " --- |"
	}
	fmt.Fprintf(b, "| %s |%s Instants | Returns |", first, classCol)
	for _, c := range cols {
		fmt.Fprintf(b, " %s |", c)
	}
	b.WriteString(" Missed | Fragmented | Max clusters |\n")
	fmt.Fprintf(b, "| --- |%s ---: | ---: |%s ---: | ---: | ---: |\n", classSep, strings.Repeat(" ---: |", len(cols)))
	for _, s := range rows {
		class := ""
		if withClass {
			class = " " + s.Class + " |"
		}
		fmt.Fprintf(b, "| %s |%s %d | %d |", s.Name, class, s.Instants, s.Returns)
		for _, c := range cols {
			fmt.Fprintf(b, " %s |", shareOrDash(s.Shares[c], s.Returns))
		}
		fragmented := "-"
		if s.Detected > 0 {
			fragmented = countShare(s.Fragmented, s.FragmentedShare)
		}
		fmt.Fprintf(b, " %d | %s | %d |\n", s.Missed, fragmented, s.MaxClusters)
	}
}

func writeGapTable(b *strings.Builder, first string, rows []FateSummary, cols []string, withClass bool) {
	classCol, classSep := "", ""
	if withClass {
		classCol, classSep = " Class |", " --- |"
	}
	fmt.Fprintf(b, "| %s |%s Fragmented instants | Gap returns |", first, classCol)
	for _, c := range cols {
		fmt.Fprintf(b, " %s |", c)
	}
	fmt.Fprintf(b, "\n| --- |%s ---: | ---: |%s\n", classSep, strings.Repeat(" ---: |", len(cols)))
	for _, s := range rows {
		class := ""
		if withClass {
			class = " " + s.Class + " |"
		}
		fmt.Fprintf(b, "| %s |%s %d | %d |", s.Name, class, s.Fragmented, s.GapReturns)
		for _, c := range cols {
			fmt.Fprintf(b, " %s |", shareOrDash(s.GapShares[c], s.GapReturns))
		}
		b.WriteString("\n")
	}
}
