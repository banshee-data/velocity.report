package perframeeval

import (
	"fmt"
	"strings"
)

// RenderRetentionMarkdown writes the retention and fragmentation report as
// Markdown: the header says what was matched and whether any count is an
// estimate, then the pooled tables by class, range band and object, then
// the merges.
func RenderRetentionMarkdown(r RetentionReport) string {
	var b strings.Builder
	b.WriteString("# Retention and fragmentation per reviewed mask\n\n")
	if r.Approximate {
		fmt.Fprintf(&b, "**Approximate.** %d of %d instants read a cluster larger than its retained sample; "+
			"their counts are sample expansions, and their fragment counts a lower bound.\n\n",
			r.Total.EstimatedInstants, r.Total.Instants)
	} else {
		b.WriteString("**Exact.** Every cluster an instant read kept all its members in its sample.\n\n")
	}
	ref, ev := r.Reference, r.Evidence
	fmt.Fprintf(&b, "- Reference: pack %s, split %s (role %s, held out: %v), %d episode(s), digest %s\n",
		ref.DatasetID, ref.Split, ref.SplitRole, ref.HeldOut, len(ref.Episodes), ref.Digest)
	fmt.Fprintf(&b, "- Evidence: %s, source %s; %d frames, %d clusters (%d capped, largest sample %d)\n",
		ev.Database, ev.SourceID, ev.Frames, ev.Clusters, ev.CappedClusters, ev.LargestSample)
	fmt.Fprintf(&b, "- Matching: %s; %d of %d retained sample points (%.1f%%) are pack points\n",
		r.Method, ev.InPack, ev.SamplePoints, 100*ev.DomainShare)
	if ev.InstantsOutsideWindow > 0 || ev.InstantsWithoutFrame > 0 {
		fmt.Fprintf(&b, "- Not measured: %d instants outside the evidence window; %d at %d samples with no evidence frame\n",
			ev.InstantsOutsideWindow, ev.InstantsWithoutFrame, ev.FramesWithoutEvidence)
	}
	b.WriteString("\nRetention is clustered over labelled returns, pooled; mean is per instant. Main is the share in the cluster " +
		"holding most of the object. Fragmented and merged are shares of the instants that reached a cluster.\n\n")

	writeRetentionTable(&b, "Group", append([]RetentionSummary{r.Total}, r.ByClass...), false)
	b.WriteString("\n")
	writeRetentionTable(&b, "Range band", r.ByRange, false)
	b.WriteString("\n")
	writeRetentionTable(&b, "Object", r.ByObject, true)

	b.WriteString("\n## Merges\n\n")
	if len(r.MergePairs) == 0 {
		b.WriteString("No cluster held returns of two reviewed objects.\n")
	} else {
		b.WriteString("| Object A | Object B | Frames sharing a cluster |\n| --- | --- | ---: |\n")
		for _, p := range r.MergePairs {
			fmt.Fprintf(&b, "| %s | %s | %d |\n", p.A, p.B, p.Frames)
		}
	}
	if len(r.Caveats) > 0 {
		b.WriteString("\n## Caveats\n\n")
		for _, c := range r.Caveats {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	return b.String()
}

func writeRetentionTable(b *strings.Builder, first string, rows []RetentionSummary, withClass bool) {
	classCol, classSep := "", ""
	if withClass {
		classCol, classSep = " Class |", " --- |"
	}
	fmt.Fprintf(b, "| %s |%s Instants | Returns | Retention | Mean | Main | Missed | Fragmented | Mean clusters | Max | Merged | Estimated |\n", first, classCol)
	fmt.Fprintf(b, "| --- |%s ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n", classSep)
	for _, s := range rows {
		class := ""
		if withClass {
			class = " " + s.Class + " |"
		}
		if s.Instants == 0 {
			fmt.Fprintf(b, "| %s |%s 0 | 0 | - | - | - | 0 | - | - | - | - | 0 |\n", s.Name, class)
			continue
		}
		fmt.Fprintf(b, "| %s |%s %d | %d | %.3f | %.3f | %.3f | %d | %s | %.2f | %d | %s | %d |\n",
			s.Name, class, s.Instants, s.MaskPoints, s.Retention, s.MeanRetention, s.MainClusterShare, s.Missed,
			countShare(s.Fragmented, s.FragmentedShare), s.MeanClusters, s.MaxClusters,
			countShare(s.Merged, s.MergedShare), s.EstimatedInstants)
	}
}

func countShare(n int, share float64) string {
	return fmt.Sprintf("%d (%.0f%%)", n, 100*share)
}
