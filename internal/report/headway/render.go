package headway

// Rendering the report through the real output path: Go-native SVG charts
// (internal/report/chart), the Typst template (headway.typ, compiled by
// internal/report/typst), and the same PDF plus recompilable source ZIP the
// radar report ships.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/chart/assets"
	"github.com/banshee-data/velocity.report/internal/report/typst"
	"github.com/banshee-data/velocity.report/internal/version"
)

const zipReadme = `# Headway report source files

This ZIP contains the Typst source, chart SVGs, fonts and data for one
headway report (observed following exposure). Everything needed to
recompile the PDF is included.

## Contents

- ` + "`headway.typ`" + ` — Typst entry point (imports preamble.typ and sections.typ)
- ` + "`preamble.typ`, `sections.typ`" + ` — shared layout, fonts and table helpers
- ` + "`data.json`" + ` — the report contract (headway_report_v1): every value,
  suppression, provenance field and printed string the PDF shows
- ` + "`charts/*.svg`" + ` — one chart per version group and per encounter
- ` + "`fonts/`" + ` — Atkinson Hyperlegible font files used by the report

## Recompiling

Install Typst (https://github.com/typst/typst), then run from this directory:

` + "```" + `bash
typst compile --font-path fonts headway.typ
` + "```" + `

## Status

The status in data.json (synthetic_oracle or provisional) is printed on
every page and every chart. Editing data.json does not change what the
numbers are.

## Support

https://github.com/banshee-data/velocity.report
`

// Options control one render.
type Options struct {
	Paper chart.PaperSize
	// OutputDir receives the PDF and the source ZIP; it is created if absent.
	OutputDir string
	// CreationTime stamps the PDF; zero means now.
	CreationTime time.Time
}

// Result names what Generate wrote.
type Result struct {
	PDFPath string
	ZIPPath string
	RunID   string
}

// typstPaper maps a paper size to the Typst paper name.
func typstPaper(p chart.PaperSize) string {
	if p == chart.PaperLetter {
		return "us-letter"
	}
	return "a4"
}

// Assemble validates the report, renders its charts and returns the report
// as the template reads it (paper and chart paths filled in) with the chart
// assets. It is everything Generate does short of typesetting.
func Assemble(r Report, paper chart.PaperSize) (Report, []typst.Asset, error) {
	if err := r.Validate(); err != nil {
		return Report{}, nil, err
	}
	paper = chart.NormalisePaperSize(string(paper))
	r.Paper = typstPaper(paper)
	breakAfter := map[string]float64{}
	for _, c := range r.Captures {
		breakAfter[c.ID] = float64(c.Params.Exposure.MaxIntervalNanos) / 1e9
	}
	thresholds := make([]chart.Threshold, 0, len(r.Bands))
	for _, b := range r.Bands {
		thresholds = append(thresholds, chart.Threshold{Value: b.Seconds, Label: b.Display})
	}

	// The report is a value, but its slices are shared with the caller's;
	// copy the two that get chart paths so the caller's report is untouched.
	r.Encounters = append([]Encounter(nil), r.Encounters...)
	r.Aggregates = append([]Aggregate(nil), r.Aggregates...)

	var out []typst.Asset
	for i := range r.Aggregates {
		a := &r.Aggregates[i]
		svg, err := chart.RenderFollowingDistribution(distributionChart(r.StatusLabel, a.Histogram, thresholds),
			chart.DefaultFollowingDistributionStyle(paper))
		if err != nil {
			return Report{}, nil, fmt.Errorf("aggregate %s chart: %w", a.ID, err)
		}
		name := "charts/aggregate_" + a.ID + ".svg"
		out = append(out, typst.Asset{Name: name, Data: svg})
		a.Chart = "/" + name
	}
	for i := range r.Encounters {
		e := &r.Encounters[i]
		svg, err := chart.RenderFollowingEncounter(encounterChart(r.StatusLabel, *e, breakAfter[e.CaptureID], thresholds),
			chart.DefaultFollowingEncounterStyle(paper))
		if err != nil {
			return Report{}, nil, fmt.Errorf("encounter %s chart: %w", e.ID, err)
		}
		name := "charts/encounter_" + e.ID + ".svg"
		out = append(out, typst.Asset{Name: name, Data: svg})
		e.Chart = "/" + name
	}
	return r, out, nil
}

func distributionChart(status string, h Histogram, thresholds []chart.Threshold) chart.FollowingDistributionData {
	d := chart.FollowingDistributionData{Status: status, Metric: string(h.Metric), Unit: h.Unit, Thresholds: thresholds}
	for _, b := range h.Bins {
		bin := chart.DistributionBin{Lower: float64(b.LowerMillis) / 1000, Share: b.Share}
		if b.UpperMillis == nil {
			bin.Overflow = true
		} else {
			bin.Upper = float64(*b.UpperMillis) / 1000
		}
		d.Bins = append(d.Bins, bin)
	}
	for _, x := range h.Excluded {
		d.Excluded = append(d.Excluded, chart.ExcludedShare{Label: x.Reason.String(), Share: x.Share})
	}
	return d
}

// spanLabel is an unsupported interval's reason, with its path condition
// when it has one: both registered tokens.
func spanLabel(u Interval) string {
	if u.Condition != l8behaviour.PathConditionUnspecified {
		return u.Reason.String() + " " + u.Condition.String()
	}
	return u.Reason.String()
}

func encounterChart(status string, e Encounter, breakAfter float64, thresholds []chart.Threshold) chart.FollowingEncounterData {
	seconds := func(n int64) float64 { return float64(n) / 1e9 }
	d := chart.FollowingEncounterData{
		Status:              status,
		Title:               e.ID + " " + e.Leader.TrackID + " -> " + e.Follower.TrackID,
		GapMetric:           string(l8behaviour.MetricFollowingSpatialGap),
		TimeGapMetric:       string(l8behaviour.MetricFollowingNetTimeGap),
		PredictedMetric:     string(l8behaviour.MetricFollowingPredictedGap),
		PredictedVisibility: l8behaviour.VisibilityReviewOnly.String(),
		DurationS:           seconds(e.DurationNanos),
		BreakAfterS:         breakAfter,
		Thresholds:          thresholds,
	}
	for _, p := range e.Series.Gap {
		d.Gap = append(d.Gap, chart.FollowingSeriesPoint{T: seconds(p.OffsetNanos), Value: p.Value, Sigma: p.Sigma})
	}
	for _, p := range e.Series.NetTimeGap {
		d.TimeGap = append(d.TimeGap, chart.FollowingSeriesPoint{T: seconds(p.OffsetNanos), Value: p.Value, Sigma: p.Sigma})
	}
	for _, p := range e.Series.Predicted {
		d.Predicted = append(d.Predicted, chart.FollowingPredictedPoint{
			T: seconds(p.OffsetNanos), Value: p.ValueM, Sigma: p.SigmaM, CoastAgeS: seconds(p.CoastAgeNanos),
		})
	}
	for _, u := range e.Unsupported {
		d.Spans = append(d.Spans, chart.FollowingSpan{Start: seconds(u.StartNanos), End: seconds(u.EndNanos), Label: spanLabel(u)})
	}
	return d
}

// Sources returns the files of the recompilable source archive: the
// templates the headway entry needs, data.json, the charts, the fonts and a
// README.
func Sources(r Report, charts []typst.Asset) (map[string][]byte, error) {
	files := map[string][]byte{}
	sources, err := typst.SourcesFor(typst.EntryHeadway)
	if err != nil {
		return nil, err
	}
	for name, body := range sources {
		files[name] = body
	}
	data, err := typst.MarshalData(r)
	if err != nil {
		return nil, fmt.Errorf("marshal report data: %w", err)
	}
	files["data.json"] = data
	for _, a := range charts {
		files[a.Name] = a.Data
	}
	for name, fb := range assets.AllFonts() {
		files[filepath.Join("fonts", name)] = fb
	}
	files["README.md"] = []byte(zipReadme)
	return files, nil
}

// Generate renders the report to a PDF through Typst and writes the PDF and
// its source ZIP to opts.OutputDir, named for the report's status.
func Generate(r Report, opts Options) (Result, error) {
	if opts.OutputDir == "" {
		return Result{}, errors.New("headway report output directory is required")
	}
	rendered, charts, err := Assemble(r, opts.Paper)
	if err != nil {
		return Result{}, err
	}
	created := opts.CreationTime
	if created.IsZero() {
		created = time.Now()
	}
	meta := typst.PDFMetadata{
		Creator:  fmt.Sprintf("velocity.report v%s", version.Version),
		Keywords: []string{"status:" + rendered.Status.String(), "contract:" + rendered.Contract},
	}
	if version.GitSHA != "" && version.GitSHA != "unknown" {
		meta.Keywords = append(meta.Keywords, "git-sha:"+version.GitSHA)
	}
	var pdf bytes.Buffer
	if err := typst.Render(&pdf, typst.Options{
		Entry: typst.EntryHeadway, Data: rendered, Assets: charts,
		IgnoreSystemFonts: true, CreationTime: created, PDFMetadata: meta,
	}); err != nil {
		return Result{}, fmt.Errorf("typst render: %w", err)
	}

	files, err := Sources(rendered, charts)
	if err != nil {
		return Result{}, err
	}
	zipBytes, err := report.BuildZip(files)
	if err != nil {
		return Result{}, fmt.Errorf("build zip: %w", err)
	}
	outDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve output directory: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}
	runID := "headway_" + rendered.Status.String() + "_report"
	res := Result{
		PDFPath: filepath.Join(outDir, runID+".pdf"),
		ZIPPath: filepath.Join(outDir, runID+"_sources.zip"),
		RunID:   runID,
	}
	if err := os.WriteFile(res.PDFPath, pdf.Bytes(), 0o644); err != nil {
		return Result{}, fmt.Errorf("write PDF: %w", err)
	}
	if err := os.WriteFile(res.ZIPPath, zipBytes, 0o644); err != nil {
		return Result{}, fmt.Errorf("write ZIP: %w", err)
	}
	return res, nil
}
