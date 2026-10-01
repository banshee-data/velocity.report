package perframeeval

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// RenderNearFaceMarkdown is a reading copy of a near-face report: the labels it
// used, then per arm what was scored, the residuals by stratum and the pairs.
// Metres are shown to the millimetre; a positive normal residual means the
// believed face is out toward the sensor beyond the labelled returns.
func RenderNearFaceMarkdown(r NearFaceReport) string {
	var b strings.Builder
	ref := r.Reference
	fmt.Fprintf(&b, "# Near-face residuals\n\n")
	fmt.Fprintf(&b, "| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Pack | `%s` |\n| Annotation revision | %d |\n| Split | %s (role %s, held out: %v) |\n",
		ref.PackDigest, ref.SidecarRevision, ref.Split, ref.SplitRole, ref.HeldOut)
	if ref.FrozenDigest != "" {
		fmt.Fprintf(&b, "| Frozen split | `%s` |\n", ref.FrozenDigest)
	}
	fmt.Fprintf(&b, "| Episodes | %s |\n", strings.Join(ref.Episodes, ", "))
	o := r.Options
	fmt.Fprintf(&b, "| Gate | %.2f m plus half the mask's footprint diagonal; frame tolerance %.1f ms |\n", o.GateSlackMetres, float64(o.FrameToleranceNanos)/1e6)
	fmt.Fprintf(&b, "| Face | outermost return, %.0f%% trimmed; at least %d returns; tangent span %.2f to %.2f of the believed width, within %.2f m of the face |\n",
		100*o.FaceQuantile, o.MinReturns, o.MinSpanCoverage, o.MaxSpanCoverage, o.FaceBandMetres)
	fmt.Fprintf(&b, "| Sensor | (%.2f, %.2f) m in the pack's frame |\n\n", o.SensorXM, o.SensorYM)

	for _, a := range r.Arms {
		fmt.Fprintf(&b, "## %s\n\n", a.Arm.Label)
		acc := a.Accounting
		fmt.Fprintf(&b, "%d labelled instants, %d matched to a body, %d scored. Centre to mask footprint centre: median %.3f m, p95 %.3f m (the mask sits on the faces the sensor sees, so a metre or so is usual; a large median means the arm and the pack are not in one frame).\n\n",
			acc.Masks, acc.Matched, acc.Scored, acc.CentreDistance.Median, acc.CentreDistance.P95Abs)
		if len(acc.Unscored) > 0 {
			reasons := make([]string, 0, len(acc.Unscored))
			for k := range acc.Unscored {
				reasons = append(reasons, k)
			}
			sort.Strings(reasons)
			fmt.Fprintf(&b, "Not scored:")
			for _, k := range reasons {
				fmt.Fprintf(&b, " %s %d;", k, acc.Unscored[k])
			}
			fmt.Fprintf(&b, "\n\n")
		}
		if len(a.Strata) > 0 {
			fmt.Fprintf(&b, "| Stratum | Face | N | Mean | Median | RMS | p95 abs | p99 abs | Over 10 cm |\n|---|---|---|---|---|---|---|---|---|\n")
			for _, s := range a.Strata {
				for _, row := range []struct {
					name string
					st   FaceStats
				}{{"end normal", s.EndNormal}, {"side normal", s.SideNormal}, {"end tangent (abs)", s.EndTangent}} {
					if row.st.N == 0 {
						continue
					}
					fmt.Fprintf(&b, "| %s | %s | %d | %+.3f | %+.3f | %.3f | %.3f | %.3f | %.0f%% |\n",
						s.Name, row.name, row.st.N, milli(row.st.Mean), milli(row.st.Median), row.st.RMS, row.st.P95Abs, row.st.P99Abs, 100*row.st.Over10cm)
				}
			}
			fmt.Fprintf(&b, "\n")
		}
		if len(a.Paired) > 0 {
			fmt.Fprintf(&b, "| Against | Face | Both scored | Mean abs (other) | Mean abs (this) | Delta | This lower |\n|---|---|---|---|---|---|---|\n")
			for _, p := range a.Paired {
				if p.Both == 0 {
					continue
				}
				fmt.Fprintf(&b, "| %s | %s | %d | %.3f | %.3f | %+.3f | %.0f%% |\n", p.Against, p.Face, p.Both, p.MeanAbsOther, p.MeanAbsThis, milli(p.Delta), 100*p.ThisLower)
			}
			fmt.Fprintf(&b, "\n")
		}
	}
	if len(r.Caveats) > 0 {
		fmt.Fprintf(&b, "## Caveats\n\n")
		for _, c := range r.Caveats {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	return b.String()
}

// milli rounds to the millimetre the table shows, so a residual that is zero
// to that precision does not print with a sign.
func milli(x float64) float64 {
	if math.Abs(x) < 0.0005 {
		return 0
	}
	return x
}
