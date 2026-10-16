package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"
)

// JSON has no NaN: an empty statistic is written as null.
func nullable(x float64) *float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	return &x
}

func nullables(xs []float64) []*float64 {
	out := make([]*float64, len(xs))
	for i, x := range xs {
		out[i] = nullable(x)
	}
	return out
}

func (s residualStats) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Cells        int      `json:"cells"`
		RMS          *float64 `json:"rms_m"`
		MedianAbs    *float64 `json:"median_abs_m"`
		ChangedShare *float64 `json:"changed_share"`
	}{s.Cells, nullable(s.RMS), nullable(s.MedianAbs), nullable(s.ChangedShare)})
}

func (s spread3) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Min    *float64 `json:"min"`
		Median *float64 `json:"median"`
		Max    *float64 `json:"max"`
	}{nullable(s.Min), nullable(s.Median), nullable(s.Max)})
}

func (r seriesRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		StartS        float64    `json:"start_s"`
		Returns       int        `json:"returns"`
		ChangedShare  *float64   `json:"changed_share"`
		MedianAbsM    *float64   `json:"median_abs_m"`
		SectorChanged []*float64 `json:"sector_changed_share"`
	}{r.StartS, r.Returns, nullable(r.ChangedShare), nullable(r.MedianAbsM), nullables(r.SectorChanged)})
}

func (b bandSectorResidual) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Band   string     `json:"band"`
		Before []*float64 `json:"before_median_abs_m"`
		After  []*float64 `json:"after_median_abs_m"`
	}{b.Band, nullables(b.BeforeMedianAbsM), nullables(b.AfterMedianAbsM)})
}

func cm(x float64) string {
	if math.IsNaN(x) {
		return "—"
	}
	return fmt.Sprintf("%.1f", 100*x)
}

func pct(x float64) string {
	if math.IsNaN(x) {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100*x)
}

// writeMarkdown prints the human summary. Distances are centimetres, angles
// degrees; every figure is also in the JSON.
func writeMarkdown(w io.Writer, o output) {
	fmt.Fprintf(w, "# lidar-pose-step\n\n")
	fmt.Fprintf(w, "- **Build:** v%s, git sha %s, built %s\n", o.BuildVersion, o.BuildGitSHA, o.BuildTime)
	if o.Case != "" {
		fmt.Fprintf(w, "- **Case:** %s\n", o.Case)
	}
	a := o.Anchor
	fmt.Fprintf(w, "- **Anchor:** %d (%s); %s +%.3f s, %.3f s before that capture ends; %.3f s (%s) after the first packet\n",
		a.UnixNanos, a.UTC, filepath.Base(a.Capture), a.OffsetInFileS, a.UntilFileEndS, a.SinceFirstS, a.SinceFirstClock)
	if a.NextCaptureInS != 0 {
		fmt.Fprintf(w, "- **Next capture:** starts %.3f s after the anchor\n", a.NextCaptureInS)
	}
	fmt.Fprintf(w, "- **Static cell:** returns in ≥ %.0f%% of rotations, IQR ≤ max(%.2f m, %.1f‰ of range), %.1f–%.0f m\n",
		100*o.Static.MinCoverage, o.Static.MaxIQR, 1000*o.Static.MaxIQRFraction, o.Static.MinRange, o.Static.MaxRange)
	fmt.Fprintf(w, "\nTimes are seconds from the anchor; distances in centimetres.\n\n")

	fmt.Fprintf(w, "## Captures\n\n| Capture | First (s) | Last (s) |\n| --- | ---: | ---: |\n")
	for _, c := range o.Captures {
		fmt.Fprintf(w, "| %s | %.3f | %.3f |\n", filepath.Base(c.Path), c.FirstS, c.LastS)
	}

	fmt.Fprintf(w, "\n## Windows\n\n| Window | Captures | Covered (s) | Packets | Rotations | Return modes | RPM | Seen cells | Static cells |\n")
	fmt.Fprintf(w, "| --- | --- | ---: | ---: | ---: | --- | ---: | ---: | ---: |\n")
	for _, win := range o.Windows {
		var modes []string
		for m, n := range win.ReturnMode {
			modes = append(modes, fmt.Sprintf("%s×%d", m, n))
		}
		slices.Sort(modes)
		trunc := ""
		if win.Truncated {
			trunc = " (truncated)"
		}
		fmt.Fprintf(w, "| %s | %s | %.2f%s | %d | %d | %s | %.0f | %d | %d |\n", win.Key, strings.Join(win.Captures, ", "),
			win.CoveredS, trunc, win.Packets, win.Rotations, strings.Join(modes, " "), win.MotorRPM, win.SeenCells, win.Static)
	}

	fmt.Fprintf(w, "\n## Pairs\n\nrx, ry, rz: rotation about the sensor axes (z up); t: translation of the after window's sensor origin in the before frame. ")
	fmt.Fprintf(w, "± is the formal SE / the sector-jackknife SE. |Δr|: median absolute range change over the fit's cells, before → after the fit; ")
	fmt.Fprintf(w, "RMS over the fit's final inliers (over every candidate it is in the JSON: depth edges dominate it); changed: share of cells moved more than %.0f cm.\n\n", 100*o.Fit.ChangedTau)
	fmt.Fprintf(w, "| Pair | A | B | rx ° | ry ° | rz ° | tx cm | ty cm | tz cm | |rot| ° | |t| cm | |Δr| cm | inlier RMS cm | changed | held-out |Δr| cm | inliers | support | verdict |\n")
	fmt.Fprintf(w, "| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- | --- | --- | ---: | --- | --- |\n")
	for _, p := range o.Pairs {
		t := p.Transform
		ang := func(i int) string {
			return fmt.Sprintf("%.3f ±%.4f/%.4f", t.RotationDeg[i], t.RotationSEDeg[i], t.JackknifeRotationSEDeg[i])
		}
		tr := func(i int) string {
			return fmt.Sprintf("%.2f ±%.2f/%.2f", 100*t.TranslationM[i], 100*t.TranslationSEM[i], 100*t.JackknifeTranslationSEM[i])
		}
		verdict := "fit"
		if p.Abstain {
			verdict = "abstain: " + strings.Join(p.Reasons, "; ")
		} else if len(p.Reasons) > 0 {
			verdict = "fit; " + strings.Join(p.Reasons, "; ")
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %.3f | %.1f | %s → %s | %s → %s | %s → %s | %s → %s | %d (%.0f%%) | %d sectors ≥5%%, max %.0f%%, normal %.2f, cond %.1e | %s |\n",
			p.Name, windowKey(p.A.StartS, p.A.EndS), windowKey(p.B.StartS, p.B.EndS),
			ang(0), ang(1), ang(2), tr(0), tr(1), tr(2), t.RotationMagDeg, 100*t.TranslationMagM,
			cm(p.Before.MedianAbs), cm(p.After.MedianAbs), cm(p.InlierBefore.RMS), cm(p.InlierAfter.RMS),
			pct(p.Before.ChangedShare), pct(p.After.ChangedShare),
			cm(p.HeldOutBefore.MedianAbs), cm(p.HeldOutAfter.MedianAbs),
			p.Inliers, 100*p.InlierShr, p.Support.SectorsOver5, 100*p.Support.MaxSectorShare,
			p.Support.NormalDominance, p.Support.ConditionRatio, verdict)
	}

	if len(o.Groups) > 0 {
		fmt.Fprintf(w, "\n## Groups\n\nMin / median / max over each group's fitted (non-abstaining) pairs.\n\n")
		fmt.Fprintf(w, "| Group | Pairs | Abstained | |rot| ° | tilt ° | |rz| ° | |t| cm | |Δr| before cm | |Δr| after cm | reduction | changed before | changed after |\n")
		fmt.Fprintf(w, "| --- | ---: | ---: | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, g := range o.Groups {
			deg := func(s spread3) string { return fmt.Sprintf("%.3f / %.3f / %.3f", s.Min, s.Median, s.Max) }
			cms := func(s spread3) string { return fmt.Sprintf("%s / %s / %s", cm(s.Min), cm(s.Median), cm(s.Max)) }
			pcts := func(s spread3) string { return fmt.Sprintf("%s / %s / %s", pct(s.Min), pct(s.Median), pct(s.Max)) }
			fmt.Fprintf(w, "| %s | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", g.Group, g.Pairs, g.Abstained,
				deg(g.RotationMagDeg), deg(g.TiltDeg), deg(g.YawDeg), cms(g.TranslationMagM),
				cms(g.BeforeMedianM), cms(g.AfterMedianM), pcts(g.Reduction), pcts(g.BeforeChange), pcts(g.AfterChange))
		}
	}

	fmt.Fprintf(w, "\n## Per sector\n\nMedian |Δr| (cm) and the share of cells changed by more than %.0f cm, before → after the fit, and after the fit that held the sector out.\n",
		100*o.Fit.ChangedTau)
	for _, p := range o.Pairs {
		fmt.Fprintf(w, "\n### %s\n\n| Sector | Cells | Inliers | |Δr| before | after | held out | changed before | after | held out |\n", p.Name)
		fmt.Fprintf(w, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for i, s := range p.PerSector {
			ho, hoc := "—", "—"
			if s.HeldOutAfter != nil {
				ho, hoc = cm(s.HeldOutAfter.MedianAbs), pct(s.HeldOutAfter.ChangedShare)
			}
			inl := 0
			if i < len(p.Support.Sectors) {
				inl = p.Support.Sectors[i].Inliers
			}
			fmt.Fprintf(w, "| %03.0f–%03.0f° | %d | %d | %s | %s | %s | %s | %s | %s |\n", s.FromDeg, s.ToDeg, s.Before.Cells, inl,
				cm(s.Before.MedianAbs), cm(s.After.MedianAbs), ho, pct(s.Before.ChangedShare), pct(s.After.ChangedShare), hoc)
		}
		fmt.Fprintf(w, "\n| Ring band | Inliers | |Δr| before by sector (cm) | after |\n| --- | ---: | --- | --- |\n")
		for i, b := range p.BandSector {
			inl := 0
			if i < len(p.Support.RingBands) {
				inl = p.Support.RingBands[i].Inliers
			}
			fmt.Fprintf(w, "| %s | %d | %s | %s |\n", b.Band, inl, joinCM(b.BeforeMedianAbsM), joinCM(b.AfterMedianAbsM))
		}
	}

	for _, s := range o.Series {
		fmt.Fprintf(w, "\n## Series %s\n\nEach rotation in %s against the static medians of %s: the share of returns more than %.0f cm from the reference, and their median |Δr|.\n\n",
			s.Name, windowKey(s.Span.StartS, s.Span.EndS), windowKey(s.Reference.StartS, s.Reference.EndS), 100*s.TauM)
		fmt.Fprintf(w, "| Rotation start (s) | Returns | Changed | |Δr| cm | Changed %% by 30° sector, scan order |\n| ---: | ---: | ---: | ---: | --- |\n")
		for _, r := range s.Rows {
			parts := make([]string, len(r.SectorChanged))
			for k, v := range r.SectorChanged {
				if math.IsNaN(v) {
					parts[k] = "—"
				} else {
					parts[k] = fmt.Sprintf("%.0f", 100*v)
				}
			}
			fmt.Fprintf(w, "| %.3f | %d | %s | %s | %s |\n", r.StartS, r.Returns, pct(r.ChangedShare), cm(r.MedianAbsM), strings.Join(parts, " "))
		}
	}
}

func joinCM(xs []float64) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = cm(x)
	}
	return strings.Join(parts, " ")
}
