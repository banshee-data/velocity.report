package perframeeval

import (
	"fmt"
	"strings"
)

// RenderMarkdown writes the comparison as a report: what was compared against
// what, the pooled result, each episode, and the caveats. Every number in it
// is in the JSON as well; this is the reading copy.
func RenderMarkdown(c Comparison) string {
	var b strings.Builder
	ref := c.Reference
	fmt.Fprintf(&b, "# Per-frame comparison: %s against %s\n\n", c.B.Arm.Label, c.A.Arm.Label)

	b.WriteString("| Reference           | Value |\n| ------------------- | ----- |\n")
	row := func(k, v string) { fmt.Fprintf(&b, "| %-19s | %s |\n", k, v) }
	row("Pack", "`"+ref.PackDigest+"`")
	row("Dataset", ref.DatasetID)
	row("Annotation revision", fmt.Sprint(ref.SidecarRevision))
	row("Split", fmt.Sprintf("%s (%s)", ref.Split, ref.SplitRole))
	row("Held out", fmt.Sprint(ref.HeldOut))
	row("Episodes", strings.Join(ref.Episodes, ", "))
	partial := "scored"
	if !ref.Policy.ScorePartialMasks {
		partial = "ignored"
	}
	row("Policy", fmt.Sprintf("%s, %s, partial masks %s", ref.Policy.Status, ref.Policy.Position, partial))
	row("Gate", c.Gate.String())
	row("Frame tolerance", fmt.Sprintf("%g ms", float64(c.FrameToleranceNanos)/1e6))
	row("Reference digest", "`"+ref.Digest+"`")
	row("Split manifest", "`"+ref.SplitManifestDigest+"`")
	b.WriteString("\n")

	b.WriteString("| Arm | Kind | Database | Source or run | Estimator | Observation model | Parameter hash | Stage | Declared baseline | Tracks |\n")
	b.WriteString("| --- | ---- | -------- | ------------- | --------- | ----------------- | -------------- | ----- | ----------------- | -----: |\n")
	for _, arm := range []ArmIdentity{c.A.Arm, c.B.Arm} {
		source := arm.SourceID
		if arm.RunID != "" {
			source = arm.RunID
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %v | %d |\n",
			arm.Label, arm.Kind, arm.Database, orDash(source), orDash(arm.EstimatorID),
			orDash(arm.ObservationModelID), orDash(arm.ParamHash), arm.Stage, arm.DeclaredBaseline, arm.Tracks)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## All episodes\n\n")
	writeSummaryTable(&b, c.A.Arm.Label, c.B.Arm.Label, c.Total)
	for _, p := range c.Paired {
		fmt.Fprintf(&b, "\n## Episode %s\n\n", p.EpisodeID)
		writeSummaryTable(&b, c.A.Arm.Label, c.B.Arm.Label, p)
	}

	b.WriteString("\n## Caveats\n\n")
	for _, cv := range c.Caveats {
		fmt.Fprintf(&b, "- %s\n", cv)
	}
	return b.String()
}

func writeSummaryTable(b *strings.Builder, labelA, labelB string, p PairedResult) {
	fmt.Fprintf(b, "| Metric | %s | %s | %s - %s |\n| ------ | ---: | ---: | ---: |\n", labelA, labelB, labelB, labelA)
	ratio := func(name string, a, bv, d float64) {
		fmt.Fprintf(b, "| %s | %.4f | %.4f | %+.4f |\n", name, a, bv, d)
	}
	count := func(name string, a, bv, d int) {
		fmt.Fprintf(b, "| %s | %d | %d | %+d |\n", name, a, bv, d)
	}
	ratio("MOTA", p.A.MOTA, p.B.MOTA, p.Delta.MOTA)
	fmt.Fprintf(b, "| MOTP (m) | %.3f | %.3f | %+.3f |\n", p.A.MOTP, p.B.MOTP, p.Delta.MOTP)
	count("ID switches", p.A.IDSwitches, p.B.IDSwitches, p.Delta.IDSwitches)
	count("Fragmentations", p.A.Fragmentations, p.B.Fragmentations, p.Delta.Fragmentations)
	count("False negatives", p.A.FN, p.B.FN, p.Delta.FN)
	count("False positives", p.A.FP, p.B.FP, p.Delta.FP)
	count("Reference points", p.A.NumGT, p.B.NumGT, p.Delta.NumGT)
	ratio("HOTA", p.A.HOTA, p.B.HOTA, p.Delta.HOTA)
	ratio("DetA", p.A.DetA, p.B.DetA, p.Delta.DetA)
	ratio("AssA", p.A.AssA, p.B.AssA, p.Delta.AssA)
	ratio("IDF1", p.A.IDF1, p.B.IDF1, p.Delta.IDF1)
	ratio("IDP", p.A.IDP, p.B.IDP, p.Delta.IDP)
	ratio("IDR", p.A.IDR, p.B.IDR, p.Delta.IDR)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
