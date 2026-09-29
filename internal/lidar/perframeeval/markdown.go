package perframeeval

import (
	"fmt"
	"sort"
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
	if ref.SplitDigest != "" {
		row("Frozen split", fmt.Sprintf("`%s`, revision %d", ref.SplitDigest, ref.SplitRevision))
	}
	b.WriteString("\n")

	b.WriteString("| Arm | Kind | Database | Source or run | Estimator | Observation model | Parameter hash | Stage | Declared baseline | Tracks |\n")
	b.WriteString("| --- | ---- | -------- | ------------- | --------- | ----------------- | -------------- | ----- | ----------------- | -----: |\n")
	for _, arm := range []ArmIdentity{c.A.Arm, c.B.Arm} {
		source := arm.SourceID
		if arm.RunID != "" {
			source = arm.RunID
		}
		kind := string(arm.Kind)
		if arm.Table != "" {
			kind += " (" + arm.Table + ")"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %v | %d |\n",
			arm.Label, kind, arm.Database, orDash(source), orDash(arm.EstimatorID),
			orDash(arm.ObservationModelID), orDash(arm.ParamHash), arm.Stage, arm.DeclaredBaseline, arm.Tracks)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## All episodes\n\n")
	writeSummaryTable(&b, c.A.Arm.Label, c.B.Arm.Label, c.Total)
	for _, p := range c.Paired {
		fmt.Fprintf(&b, "\n## Episode %s\n\n", p.EpisodeID)
		writeSummaryTable(&b, c.A.Arm.Label, c.B.Arm.Label, p)
	}

	if c.Physical != nil {
		writePhysical(&b, *c.Physical)
	}

	b.WriteString("\n## Caveats\n\n")
	for _, cv := range c.Caveats {
		fmt.Fprintf(&b, "- %s\n", cv)
	}
	return b.String()
}

// writePhysical is the physical section: the reference revision, each
// component's pooled error beside the reference's own bound, and where every
// unscored instant went.
func writePhysical(b *strings.Builder, p PhysicalArms) {
	ref := p.Reference
	fmt.Fprintf(b, "\n## Physical references\n\n")
	fmt.Fprintf(b, "Revision %d, content `%s`, %d expected instants, gate %g m.\n\n",
		ref.PhysicalRevision, ref.PhysicalContentDigest, ref.ExpectedInstants, ref.GateMetres)
	// Each arm's mean bound is over the instants it scored, which need not
	// be the other arm's.
	fmt.Fprintf(b, "| Component | %s scored | %s mean abs error | %s mean reference bound | %s scored | %s mean abs error | %s mean reference bound |\n",
		p.A.Arm.Label, p.A.Arm.Label, p.A.Arm.Label, p.B.Arm.Label, p.B.Arm.Label, p.B.Arm.Label)
	b.WriteString("| --------- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	row := func(name string, a, bs ComponentSummary) {
		fmt.Fprintf(b, "| %s | %d | %.3f | %.3f | %d | %.3f | %.3f |\n",
			name, a.Scored, a.MeanAbsError, a.MeanReferenceBound, bs.Scored, bs.MeanAbsError, bs.MeanReferenceBound)
	}
	for _, c := range PhysicalComponents() {
		if c != ComponentBox {
			row(string(c), p.A.Summary.Components[c], p.B.Summary.Components[c])
		}
	}
	row("following_gap", p.A.Summary.Following, p.B.Summary.Following)
	fmt.Fprintf(b, "| box IoU | %d | %.3f | - | %d | %.3f | - |\n",
		p.A.Summary.BoxScored, p.A.Summary.MeanBoxIoU, p.B.Summary.BoxScored, p.B.Summary.MeanBoxIoU)

	for _, arm := range []PhysicalResult{p.A, p.B} {
		fmt.Fprintf(b, "\nWhere arm %s's expected instants went:\n\n", arm.Arm.Label)
		for _, c := range PhysicalComponents() {
			a := arm.Accounting.Components[c]
			fmt.Fprintf(b, "- %s: %d of %d scored%s\n", c, a.Scored, a.Expected, describeUnscored(a))
		}
		f := arm.Accounting.Following
		fmt.Fprintf(b, "- following_gap: %d of %d scored%s\n", f.Scored, f.Expected, describeUnscored(f))
		for _, cv := range arm.Caveats {
			fmt.Fprintf(b, "\n> %s\n", cv)
		}
	}
}

func describeUnscored(a ComponentAccounting) string {
	var parts []string
	for _, category := range sortedMapKeys(a.Unscored) {
		for _, reason := range sortedMapKeys(a.Unscored[category]) {
			parts = append(parts, fmt.Sprintf("%s/%s %d", category, reason, a.Unscored[category][reason]))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, ", ")
}

func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
