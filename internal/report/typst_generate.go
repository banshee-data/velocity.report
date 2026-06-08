package report

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/chart/assets"
	"github.com/banshee-data/velocity.report/internal/report/typst"
	"github.com/banshee-data/velocity.report/internal/units"
)

const typstZipReadme = `# Velocity report source files

This ZIP contains the Typst source, chart SVGs, fonts, and data for your
velocity report. Everything needed to recompile the PDF is included.

## Contents

- ` + "`report.typ`" + ` — Typst entry point (imports preamble.typ + sections.typ)
- ` + "`preamble.typ`, `sections.typ`" + ` — layout, fonts, and section builders
- ` + "`data.json`" + ` — the report data the templates render
- ` + "`charts/*.svg`" + ` — chart source files (timeseries, histogram, comparison, map)
- ` + "`fonts/`" + ` — Atkinson Hyperlegible font files used by the report

## Recompiling

Install Typst (https://github.com/typst/typst), then run from this directory:

` + "```" + `bash
typst compile --font-path fonts report.typ
` + "```" + `

Typst reads the data from data.json and the charts from charts/ relative to
report.typ. No LaTeX, no rsvg-convert, no system fonts required.

## Editing

1. Edit data.json to change values, or the .typ files to change layout.
2. Replace any charts/*.svg with a revised version.
3. Recompile with the command above.

## Support

https://github.com/banshee-data/velocity.report
`

// GeneratePDF produces a PDF report and source ZIP using the Typst pipeline.
// This is the entry point the HTTP API and CLI call.
func GeneratePDF(ctx context.Context, database DB, cfg Config) (Result, error) {
	return GenerateTypst(ctx, database, cfg)
}

// GenerateTypst produces a PDF report and recompilable Typst source ZIP. It
// supports both single-period and comparison reports, selected by whether
// cfg.CompareStart is set. Charts are rendered to SVG and embedded directly by
// Typst (no rsvg-convert / PDF rasterisation step), and the typst executable
// is resolved via the embedded binary, VELOCITY_TYPST_PATH, or PATH.
func GenerateTypst(ctx context.Context, database DB, cfg Config) (Result, error) {
	plan, err := planRun(cfg)
	if err != nil {
		return Result{}, err
	}

	data, err := loadData(ctx, database, plan)
	if err != nil {
		return Result{}, err
	}

	charts, chartAssets, err := renderTypstCharts(plan, data)
	if err != nil {
		return Result{}, err
	}

	// Single-period reports show only the granular breakdown (Table 3). The
	// daily table (Table 3 in comparison mode) is populated solely from the
	// comparison-mode primary daily roll-up, which loadData fetches only when
	// comparing. data.primaryDaily is nil otherwise.
	rd := buildTypstData(plan, data, data.primaryDaily, charts)

	var pdf bytes.Buffer
	if err := typst.Render(&pdf, typst.Options{
		Data:              rd,
		Assets:            chartAssets,
		IgnoreSystemFonts: true,
		CreationTime:      time.Now(),
	}); err != nil {
		return Result{}, fmt.Errorf("typst render: %w", err)
	}

	return packageTypstOutput(cfg, rd, chartAssets, pdf.Bytes())
}

// renderTypstCharts renders the report's SVG charts and returns both the
// root-relative references for the templates and the asset blobs to embed: a
// primary time-series, an optional comparison time-series, a histogram (single)
// or comparison chart, and an optional site map.
func renderTypstCharts(plan runPlan, data loadedData) (typst.ChartRefs, []typst.Asset, error) {
	cfg := plan.cfg
	var refs typst.ChartRefs
	var out []typst.Asset

	tsPoints := ConvertToTimeSeriesPoints(data.tsResult.Metrics, cfg.Units, plan.loc)
	if cfg.ExpandedChart {
		tsPoints = chart.ExpandTimeSeriesGapsInRange(tsPoints, plan.groupSeconds, plan.startTime, plan.endTime)
	}
	tsSVG, err := chart.RenderTimeSeries(chart.TimeSeriesData{
		Points:       tsPoints,
		Units:        cfg.Units,
		P98Reference: data.summaryP98,
		MaxReference: data.summaryMax,
	}, chart.DefaultTimeSeriesStyle(plan.paper))
	if err != nil {
		return refs, nil, fmt.Errorf("render time-series: %w", err)
	}
	out = append(out, typst.Asset{Name: "charts/timeseries.svg", Data: tsSVG})
	refs.TimeSeries = "/charts/timeseries.svg"

	if data.compareResult != nil {
		ctsPoints := ConvertToTimeSeriesPoints(data.compareResult.tsRows, cfg.Units, plan.loc)
		if cfg.ExpandedChart {
			ctsPoints = chart.ExpandTimeSeriesGapsInRange(ctsPoints, plan.groupSeconds, data.compareResult.startTime, data.compareResult.endTime)
		}
		ctsSVG, cerr := chart.RenderTimeSeries(chart.TimeSeriesData{
			Points:       ctsPoints,
			Units:        cfg.Units,
			P98Reference: data.compareResult.p98,
			MaxReference: data.compareResult.maxSpeed,
		}, chart.DefaultTimeSeriesStyle(plan.paper))
		if cerr != nil {
			return refs, nil, fmt.Errorf("render compare time-series: %w", cerr)
		}
		out = append(out, typst.Asset{Name: "charts/timeseries_compare.svg", Data: ctsSVG})
		refs.TimeSeriesCompare = "/charts/timeseries_compare.svg"
	}

	if cfg.Histogram && data.summaryResult.Histogram != nil {
		primaryHist := ConvertHistogramKeys(data.summaryResult.Histogram, cfg.Units)
		primaryData := chart.HistogramData{Buckets: primaryHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed}

		var svg []byte
		if data.compareResult != nil && data.compareResult.histogram != nil {
			compareHist := ConvertHistogramKeys(data.compareResult.histogram, cfg.Units)
			svg, err = chart.RenderComparison(
				primaryData,
				chart.HistogramData{Buckets: compareHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
				fmt.Sprintf("t1: %s to %s", cfg.StartDate, cfg.EndDate),
				fmt.Sprintf("t2: %s to %s", cfg.CompareStart, cfg.CompareEnd),
				chart.DefaultComparisonHistogramStyle(plan.paper),
			)
		} else {
			svg, err = chart.RenderHistogram(primaryData, chart.DefaultHistogramStyle(plan.paper))
		}
		if err != nil {
			return refs, nil, fmt.Errorf("render histogram: %w", err)
		}
		out = append(out, typst.Asset{Name: "charts/histogram.svg", Data: svg})
		refs.Histogram = "/charts/histogram.svg"
	}

	if cfg.IncludeMap && len(cfg.MapSVG) > 0 {
		out = append(out, typst.Asset{Name: "charts/map.svg", Data: cfg.MapSVG})
		refs.Map = "/charts/map.svg"
	}

	return refs, out, nil
}

// buildTypstData assembles the JSON payload the templates consume. The primary
// period always populates daily + granular tables; the comparison period, when
// present, populates the nested compare block that the templates merge in.
func buildTypstData(plan runPlan, data loadedData, primaryDaily []db.RadarObjectsRollupRow, charts typst.ChartRefs) typst.ReportData {
	cfg := plan.cfg
	displayUnits := cfg.Units

	rd := typst.ReportData{
		Site: typst.SiteData{
			Location:        cfg.Location,
			Surveyor:        cfg.Surveyor,
			Contact:         cfg.Contact,
			SpeedLimit:      cfg.SpeedLimit,
			SiteDescription: cfg.SiteDescription,
		},
		Period: typst.PeriodData{
			StartDate:   plan.startTime.Format("2006-01-02"),
			EndDate:     plan.endTime.Format("2006-01-02"),
			StartISO:    plan.startTime.Format(time.RFC3339),
			EndISO:      plan.endTime.Format(time.RFC3339),
			Timezone:    cfg.Timezone,
			Group:       cfg.Group,
			Units:       displayUnits,
			MinSpeedStr: fmt.Sprintf("%.1f %s", cfg.MinSpeed, displayUnits),
		},
		Overall: typst.SummaryData{
			TotalCount: data.totalCount,
			P50:        ptrOrNil(data.summaryP50),
			P85:        ptrOrNil(data.summaryP85),
			P98:        ptrOrNil(data.summaryP98),
			MaxSpeed:   ptrOrNil(data.summaryMax),
		},
		Radar:    buildTypstRadar(cfg),
		Daily:    toTypstRows(primaryDaily, displayUnits, plan.loc, true),
		Granular: toTypstRows(data.tsResult.Metrics, displayUnits, plan.loc, false),
		Charts:   charts,
	}

	rd.Histogram = typst.Histogram{Units: displayUnits}
	if cfg.Histogram && data.summaryResult.Histogram != nil {
		rd.Histogram.Buckets = buildHistogramBuckets(
			ConvertHistogramKeys(data.summaryResult.Histogram, displayUnits),
			cfg.HistBucketSize, cfg.MinSpeed, cfg.HistMax,
		)
	}

	if cfg.CosineAngle > 0 || cfg.CompareCosineAngle > 0 ||
		cfg.CosineCorrectionLabel != "" || cfg.CompareCosineCorrectionLabel != "" {
		rd.CosineCorrectionNote = "Note: speeds have been corrected to account for sensor angle."
	}

	if data.compareResult != nil {
		cr := data.compareResult
		cmp := &typst.CompareData{
			StartDate: cr.startDate,
			EndDate:   cr.endDate,
			StartISO:  cr.startTime.Format(time.RFC3339),
			EndISO:    cr.endTime.Format(time.RFC3339),
			Overall: typst.SummaryData{
				TotalCount: cr.count,
				P50:        ptrOrNil(cr.p50),
				P85:        ptrOrNil(cr.p85),
				P98:        ptrOrNil(cr.p98),
				MaxSpeed:   ptrOrNil(cr.maxSpeed),
			},
			Daily:     toTypstRows(cr.dailyRows, displayUnits, plan.loc, true),
			Granular:  toTypstRows(cr.tsRows, displayUnits, plan.loc, false),
			Histogram: typst.Histogram{Units: displayUnits},
		}
		if cfg.Histogram && cr.histogram != nil {
			cmp.Histogram.Buckets = buildHistogramBuckets(
				ConvertHistogramKeys(cr.histogram, displayUnits),
				cfg.HistBucketSize, cfg.MinSpeed, cfg.HistMax,
			)
		}
		rd.Compare = cmp
	}

	return rd
}

// buildTypstRadar fills the hardware specs and the per-period cosine figures
// as preformatted strings. An
// empty angle string means no correction applied and the template omits the
// row.
func buildTypstRadar(cfg Config) typst.RadarData {
	r := typst.RadarData{
		SensorModel:        "OmniPreSense OPS243-A",
		FirmwareVersion:    cfg.FirmwareVersion,
		TransmitFrequency:  "24.125 GHz",
		SampleRate:         "20 kSPS",
		VelocityResolution: "0.272 mph",
		AzimuthFOV:         "20°",
		ElevationFOV:       "24°",
	}
	if cfg.CosineAngle > 0 {
		r.CosineErrorAngle = fmt.Sprintf("%.1f", cfg.CosineAngle)
		r.CosineErrorFactor = fmt.Sprintf("%.6f", cosineFactor(cfg.CosineAngle))
	}
	if cfg.CompareCosineAngle > 0 {
		r.CompareCosineErrorAngle = fmt.Sprintf("%.1f", cfg.CompareCosineAngle)
		r.CompareCosineErrorFactor = fmt.Sprintf("%.6f", cosineFactor(cfg.CompareCosineAngle))
	}
	return r
}

func cosineFactor(angleDeg float64) float64 {
	return 1.0 / math.Cos(angleDeg*math.Pi/180.0)
}

// toTypstRows converts DB roll-up rows into template rows. daily selects the
// Date label field (vs Bucket) so the template reads row.date / row.bucket as
// it expects. Zero-sample rows leave the percentile pointers nil → JSON null →
// Typst none → "--".
func toTypstRows(rows []db.RadarObjectsRollupRow, displayUnits string, loc *time.Location, daily bool) []typst.RollupRow {
	out := make([]typst.RollupRow, 0, len(rows))
	for _, r := range rows {
		// Pad month/day to width 2 with a leading space so the slashes and
		// colons line up vertically in the mono table column (e.g. " 9/ 1 00:00"
		// aligns under "10/10 00:00"). mono-nowrap converts the spaces to
		// non-breaking spaces, preserving the alignment.
		t := r.StartTime.In(loc)
		label := fmt.Sprintf("%2d/%2d %02d:%02d", int(t.Month()), t.Day(), t.Hour(), t.Minute())
		row := typst.RollupRow{Count: int(r.Count)}
		if daily {
			row.Date = label
		} else {
			row.Bucket = label
		}
		if r.Count > 0 {
			row.P50 = ptrOrNil(units.ConvertSpeed(r.P50Speed, displayUnits))
			row.P85 = ptrOrNil(units.ConvertSpeed(r.P85Speed, displayUnits))
			row.P98 = ptrOrNil(units.ConvertSpeed(r.P98Speed, displayUnits))
			row.MaxSpeed = ptrOrNil(units.ConvertSpeed(r.MaxSpeed, displayUnits))
		}
		out = append(out, row)
	}
	return out
}

// buildHistogramBuckets converts a display-unit histogram (bucket-start to count)
// into labelled buckets with percentage shares. Buckets below the cutoff are
// folded into a "<N" row and buckets at/above the max into an "N+" row.
func buildHistogramBuckets(hist map[float64]int64, bucketSz, cutoff, maxBucket float64) []typst.HistogramBucket {
	if len(hist) == 0 {
		return nil
	}
	keys := make([]float64, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	var total int64
	for _, c := range hist {
		total += c
	}
	if total == 0 {
		return nil
	}

	var belowCount, aboveCount int64
	type displayRow struct {
		label string
		count int64
	}
	var rows []displayRow
	hasUpperCap := maxBucket > 0
	for _, k := range keys {
		count := hist[k]
		switch {
		case k < cutoff:
			belowCount += count
		case hasUpperCap && k >= maxBucket:
			aboveCount += count
		default:
			rows = append(rows, displayRow{
				label: fmt.Sprintf("%.0f-%.0f", k, k+bucketSz),
				count: count,
			})
		}
	}

	pct := func(n int64) *float64 {
		v := float64(n) / float64(total) * 100.0
		return &v
	}

	var out []typst.HistogramBucket
	if belowCount > 0 {
		out = append(out, typst.HistogramBucket{Label: fmt.Sprintf("<%.0f", cutoff), Count: int(belowCount), Percent: pct(belowCount)})
	}
	for _, row := range rows {
		out = append(out, typst.HistogramBucket{Label: row.label, Count: int(row.count), Percent: pct(row.count)})
	}
	if aboveCount > 0 {
		out = append(out, typst.HistogramBucket{Label: fmt.Sprintf("%.0f+", maxBucket), Count: int(aboveCount), Percent: pct(aboveCount)})
	}
	return out
}

func ptrOrNil(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// packageTypstOutput writes the compiled PDF and the recompilable source ZIP to
// the configured output directory (or a temp dir when none is set) and returns
// their paths.
func packageTypstOutput(cfg Config, rd typst.ReportData, chartAssets []typst.Asset, pdfBytes []byte) (Result, error) {
	safeLocation := sanitiseFilename(cfg.Location)
	baseName := fmt.Sprintf("%s_velocity.report_%s_report", cfg.EndDate, safeLocation)
	pdfName := baseName + ".pdf"
	zipName := baseName + "_sources.zip"

	zipFiles, err := buildTypstZipFiles(rd, chartAssets)
	if err != nil {
		return Result{}, err
	}
	zipBytes, err := BuildZip(zipFiles)
	if err != nil {
		return Result{}, fmt.Errorf("build zip: %w", err)
	}

	outDir := cfg.OutputDir
	if outDir == "" {
		tmp, terr := os.MkdirTemp("", "velocity-report-typst-out-*")
		if terr != nil {
			return Result{}, fmt.Errorf("create output dir: %w", terr)
		}
		outDir = tmp
	}
	outDir, err = normaliseOutputDir(outDir, outDir)
	if err != nil {
		return Result{}, err
	}

	outPDF, err := safeOutputPath(outDir, pdfName)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outPDF, pdfBytes, 0644); err != nil {
		return Result{}, fmt.Errorf("write output PDF: %w", err)
	}

	outZIP, err := safeOutputPath(outDir, zipName)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outZIP, zipBytes, 0644); err != nil {
		return Result{}, fmt.Errorf("write output ZIP: %w", err)
	}

	return Result{PDFPath: outPDF, ZIPPath: outZIP, RunID: baseName}, nil
}

// buildTypstZipFiles assembles the recompilable source archive: the templates,
// the data payload, the chart SVGs, the fonts, and a README.
func buildTypstZipFiles(rd typst.ReportData, chartAssets []typst.Asset) (map[string][]byte, error) {
	files := map[string][]byte{}

	sources, err := typst.Sources()
	if err != nil {
		return nil, err
	}
	for name, body := range sources {
		files[name] = body
	}

	dataJSON, err := typst.MarshalData(rd)
	if err != nil {
		return nil, fmt.Errorf("marshal report data: %w", err)
	}
	files["data.json"] = dataJSON

	for _, a := range chartAssets {
		files[a.Name] = a.Data
	}
	for name, fb := range assets.AllFonts() {
		files[filepath.Join("fonts", name)] = fb
	}
	files["README.md"] = []byte(typstZipReadme)
	return files, nil
}
