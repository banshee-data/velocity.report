package report

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/tex"
	"github.com/banshee-data/velocity.report/internal/units"
)

// ErrInvalidConfig wraps all Generate errors caused by bad caller input
// (unknown group, unparseable timezone or date). Handlers can use
// errors.Is(err, report.ErrInvalidConfig) to map these to HTTP 4xx
// responses; every other Generate error is a server-side failure.
var ErrInvalidConfig = errors.New("invalid report config")

//go:embed chart/assets/AtkinsonHyperlegible-Regular.ttf
var fontRegular []byte

//go:embed chart/assets/AtkinsonHyperlegible-Bold.ttf
var fontBold []byte

//go:embed chart/assets/AtkinsonHyperlegible-Italic.ttf
var fontItalic []byte

//go:embed chart/assets/AtkinsonHyperlegible-BoldItalic.ttf
var fontBoldItalic []byte

const zipReadme = `# Velocity report source files

This ZIP contains the LaTeX source, chart SVGs, and fonts for your velocity
report. Everything needed to recompile the PDF is included.

## Contents

- ` + "`report.tex`" + ` — LaTeX source; compile with XeLaTeX (uses Atkinson Hyperlegible)
- ` + "`*.svg`" + ` — Chart source files (timeseries, histogram, comparison, map)
- ` + "`fonts/`" + ` — Atkinson Hyperlegible font files required by report.tex

## Recompiling

` + "```" + `bash
xelatex report.tex
` + "```" + `

XeLaTeX reads the fonts from the ` + "`fonts/`" + ` subdirectory relative to the .tex
file. Run from the directory containing report.tex and fonts/.

## Editing

1. Edit report.tex to adjust layout, text, or tables.
2. Replace any *.svg with a revised version (then rerun rsvg-convert if you
   need the PDF form: ` + "`rsvg-convert -f pdf -o chart.pdf chart.svg`" + `).
3. Recompile with xelatex.

## Support

https://github.com/banshee-data/velocity.report
`

// Generate produces a PDF report and source ZIP for the given configuration.
func Generate(ctx context.Context, database DB, cfg Config) (result Result, err error) {
	// Validate group.
	groupSeconds, ok := supportedGroups[cfg.Group]
	if !ok {
		return Result{}, fmt.Errorf("%w: unsupported group %q", ErrInvalidConfig, cfg.Group)
	}

	// Parse and validate all caller-supplied fields before touching external tools.
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return Result{}, fmt.Errorf("%w: invalid timezone %q: %v", ErrInvalidConfig, cfg.Timezone, err)
	}

	startTime, err := time.ParseInLocation("2006-01-02", cfg.StartDate, loc)
	if err != nil {
		return Result{}, fmt.Errorf("%w: invalid start date %q: %v", ErrInvalidConfig, cfg.StartDate, err)
	}
	endTime, err := time.ParseInLocation("2006-01-02", cfg.EndDate, loc)
	if err != nil {
		return Result{}, fmt.Errorf("%w: invalid end date %q: %v", ErrInvalidConfig, cfg.EndDate, err)
	}

	// Check external tool availability.
	if err := checkRsvgConvert(); err != nil {
		return Result{}, err
	}
	if err := checkXeLatex(); err != nil {
		return Result{}, err
	}
	// End date is inclusive: advance to end-of-day.
	endTime = endTime.Add(24*time.Hour - time.Second)

	startUnix := startTime.Unix()
	endUnix := endTime.Unix()

	// Convert thresholds to mps.
	minSpeedMPS := units.ConvertToMPS(cfg.MinSpeed, cfg.Units)

	var histBucketMPS, histMaxMPS float64
	if cfg.Histogram {
		histBucketMPS = units.ConvertToMPS(cfg.HistBucketSize, cfg.Units)
		histMaxMPS = units.ConvertToMPS(cfg.HistMax, cfg.Units)
	}

	// Summary query (groupSeconds=0 → single aggregate row + histogram).
	summaryResult, err := database.RadarObjectRollupRange(
		startUnix, endUnix, 0, minSpeedMPS,
		cfg.Source, cfg.ModelVersion,
		histBucketMPS, histMaxMPS,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return Result{}, fmt.Errorf("summary query: %w", err)
	}

	// Time-series query (user's group interval, no histogram).
	tsResult, err := database.RadarObjectRollupRange(
		startUnix, endUnix, groupSeconds, minSpeedMPS,
		cfg.Source, cfg.ModelVersion,
		0, 0,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return Result{}, fmt.Errorf("time-series query: %w", err)
	}

	// Primary period daily roll-up (used when comparison mode is active).
	var primaryDailyRows []db.RadarObjectsRollupRow
	if cfg.CompareStart != "" {
		dailyResult, err := database.RadarObjectRollupRange(
			startUnix, endUnix, 86400, minSpeedMPS,
			cfg.Source, cfg.ModelVersion,
			0, 0,
			cfg.SiteID, cfg.BoundaryThreshold,
		)
		if err != nil {
			return Result{}, fmt.Errorf("primary daily query: %w", err)
		}
		primaryDailyRows = dailyResult.Metrics
	}

	// Comparison data (if requested).
	var compareResult *comparisonData
	if cfg.CompareStart != "" {
		cd, err := fetchComparison(ctx, database, cfg, loc, minSpeedMPS, histBucketMPS, histMaxMPS, groupSeconds)
		if err != nil {
			return Result{}, fmt.Errorf("comparison query: %w", err)
		}
		compareResult = cd
	}

	// Create temp working directory.
	workDir, err := os.MkdirTemp("", "velocity-report-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(workDir)
		}
	}()

	// Write font files.
	fontDir := workDir
	fonts := map[string][]byte{
		"AtkinsonHyperlegible-Regular.ttf":    fontRegular,
		"AtkinsonHyperlegible-Bold.ttf":       fontBold,
		"AtkinsonHyperlegible-Italic.ttf":     fontItalic,
		"AtkinsonHyperlegible-BoldItalic.ttf": fontBoldItalic,
	}
	for name, data := range fonts {
		if err = os.WriteFile(filepath.Join(fontDir, name), data, 0644); err != nil {
			return Result{}, fmt.Errorf("write font %s: %w", name, err)
		}
	}

	// Resolve paper size for chart sizing + LaTeX paper geometry.
	paper := chart.NormalisePaperSize(cfg.PaperSize)

	// Build summary statistics from the aggregate row before rendering charts so
	// the time-series can draw the overall p98 reference line from the same query.
	var summaryP50, summaryP85, summaryP98, summaryMax float64
	var summaryP98Reference = math.NaN()
	var totalCount int
	if len(summaryResult.Metrics) > 0 {
		row := summaryResult.Metrics[0]
		summaryP50 = units.ConvertSpeed(row.P50Speed, cfg.Units)
		summaryP85 = units.ConvertSpeed(row.P85Speed, cfg.Units)
		summaryP98 = units.ConvertSpeed(row.P98Speed, cfg.Units)
		summaryMax = units.ConvertSpeed(row.MaxSpeed, cfg.Units)
		summaryP98Reference = summaryP98
		totalCount = int(row.Count)
	}

	// Convert DB rows to chart data.
	tsPoints := convertToTimeSeriesPoints(tsResult.Metrics, cfg.Units, loc)
	tsData := chart.TimeSeriesData{
		Points:       tsPoints,
		Units:        cfg.Units,
		Title:        "",
		P98Reference: summaryP98Reference,
		MaxReference: summaryMax,
	}

	// Render time-series SVG.
	tsSVG, err := chart.RenderTimeSeries(tsData, chart.DefaultTimeSeriesStyle(paper))
	if err != nil {
		return Result{}, fmt.Errorf("render time-series: %w", err)
	}
	tsSVGPath := filepath.Join(workDir, "timeseries.svg")
	if err = os.WriteFile(tsSVGPath, tsSVG, 0644); err != nil {
		return Result{}, fmt.Errorf("write timeseries.svg: %w", err)
	}

	// Convert SVG → PDF.
	tsPDFPath := filepath.Join(workDir, "timeseries.pdf")
	if err = convertSVGToPDF(ctx, tsSVGPath, tsPDFPath); err != nil {
		return Result{}, fmt.Errorf("convert timeseries SVG: %w", err)
	}

	// Compare timeseries chart (rendered here so the compare P98 reference line is available).
	var compareTimeSeriesPDFName string
	if compareResult != nil {
		ctsPoints := convertToTimeSeriesPoints(compareResult.tsRows, cfg.Units, loc)
		ctsData := chart.TimeSeriesData{
			Points:       ctsPoints,
			Units:        cfg.Units,
			P98Reference: compareResult.p98,
			MaxReference: compareResult.maxSpeed,
		}
		ctsSVG, cerr := chart.RenderTimeSeries(ctsData, chart.DefaultTimeSeriesStyle(paper))
		if cerr != nil {
			return Result{}, fmt.Errorf("render compare timeseries: %w", cerr)
		}
		ctsSVGPath := filepath.Join(workDir, "timeseries_compare.svg")
		if err = os.WriteFile(ctsSVGPath, ctsSVG, 0644); err != nil {
			return Result{}, fmt.Errorf("write timeseries_compare.svg: %w", err)
		}
		ctsPDFPath := filepath.Join(workDir, "timeseries_compare.pdf")
		if err = convertSVGToPDF(ctx, ctsSVGPath, ctsPDFPath); err != nil {
			return Result{}, fmt.Errorf("convert compare timeseries SVG: %w", err)
		}
		compareTimeSeriesPDFName = "timeseries_compare.pdf"
	}

	// Collect source files for the ZIP.
	zipFiles := map[string][]byte{
		"timeseries.svg": tsSVG,
	}

	// Histogram (if requested).
	var histSVG []byte
	var histogramTableTeX string
	if cfg.Histogram && summaryResult.Histogram != nil {
		displayHist := convertHistogramKeys(summaryResult.Histogram, cfg.Units)

		histData := chart.HistogramData{
			Buckets:   displayHist,
			Units:     cfg.Units,
			BucketSz:  cfg.HistBucketSize,
			MaxBucket: cfg.HistMax,
			Cutoff:    cfg.MinSpeed,
		}

		histSVG, err = chart.RenderHistogram(histData, chart.DefaultHistogramStyle(paper))
		if err != nil {
			return Result{}, fmt.Errorf("render histogram: %w", err)
		}
		histSVGPath := filepath.Join(workDir, "histogram.svg")
		if err = os.WriteFile(histSVGPath, histSVG, 0644); err != nil {
			return Result{}, fmt.Errorf("write histogram.svg: %w", err)
		}
		histPDFPath := filepath.Join(workDir, "histogram.pdf")
		if err = convertSVGToPDF(ctx, histSVGPath, histPDFPath); err != nil {
			return Result{}, fmt.Errorf("convert histogram SVG: %w", err)
		}
		zipFiles["histogram.svg"] = histSVG

		histogramTableTeX = tex.BuildHistogramTableTeX(
			displayHist, cfg.HistBucketSize, cfg.MinSpeed, cfg.HistMax, cfg.Units,
		)
	}

	// Site map (if configured on the site).
	var mapPDFName string
	if cfg.IncludeMap && len(cfg.MapSVG) > 0 {
		mapSVGPath := filepath.Join(workDir, "map.svg")
		if err = os.WriteFile(mapSVGPath, cfg.MapSVG, 0644); err != nil {
			return Result{}, fmt.Errorf("write map.svg: %w", err)
		}
		mapPDFPath := filepath.Join(workDir, "map.pdf")
		if err = convertSVGToPDF(ctx, mapSVGPath, mapPDFPath); err != nil {
			return Result{}, fmt.Errorf("convert map SVG: %w", err)
		}
		zipFiles["map.svg"] = cfg.MapSVG
		mapPDFName = "map.pdf"
	}

	// Comparison chart (if requested).
	if compareResult != nil && cfg.Histogram {
		primaryHist := convertHistogramKeys(summaryResult.Histogram, cfg.Units)
		compareHist := convertHistogramKeys(compareResult.histogram, cfg.Units)

		compSVG, cerr := chart.RenderComparison(
			chart.HistogramData{Buckets: primaryHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
			chart.HistogramData{Buckets: compareHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
			fmt.Sprintf("%s–%s", cfg.StartDate, cfg.EndDate),
			fmt.Sprintf("%s–%s", cfg.CompareStart, cfg.CompareEnd),
			chart.DefaultHistogramStyle(paper),
		)
		if cerr != nil {
			return Result{}, fmt.Errorf("render comparison: %w", cerr)
		}
		compSVGPath := filepath.Join(workDir, "comparison.svg")
		if err = os.WriteFile(compSVGPath, compSVG, 0644); err != nil {
			return Result{}, fmt.Errorf("write comparison.svg: %w", err)
		}
		compPDFPath := filepath.Join(workDir, "comparison.pdf")
		if err = convertSVGToPDF(ctx, compSVGPath, compPDFPath); err != nil {
			return Result{}, fmt.Errorf("convert comparison SVG: %w", err)
		}
		zipFiles["comparison.svg"] = compSVG
	}

	// Cosine correction factor (primary period).
	cosineFactor := 1.0
	if cfg.CosineAngle != 0 {
		cosineFactor = 1.0 / math.Cos(cfg.CosineAngle*math.Pi/180.0)
	}

	// Cosine correction factor (comparison period).
	compareCosineAngle := cfg.CompareCosineAngle
	compareCosineFactor := 1.0
	if compareCosineAngle != 0 {
		compareCosineFactor = 1.0 / math.Cos(compareCosineAngle*math.Pi/180.0)
	}

	// Build TemplateData.
	td := tex.TemplateData{
		Location:    tex.EscapeTeX(cfg.Location),
		Surveyor:    tex.EscapeTeX(cfg.Surveyor),
		Contact:     tex.EscapeTeX(cfg.Contact),
		SpeedLimit:  cfg.SpeedLimit,
		Description: tex.EscapeTeX(cfg.SiteDescription),

		StartDate: startTime.Format("2006-01-02"),
		EndDate:   endTime.Format("2006-01-02"),
		Timezone:  tex.EscapeTeX(cfg.Timezone),
		Units:     tex.EscapeTeX(cfg.Units),

		P50:        tex.FormatNumber(summaryP50),
		P85:        tex.FormatNumber(summaryP85),
		P98:        tex.FormatNumber(summaryP98),
		MaxSpeed:   tex.FormatNumber(summaryMax),
		TotalCount: totalCount,
		HoursCount: int(math.Ceil(float64(endUnix-startUnix) / 3600.0)),

		TotalCountFormatted: tex.FormatCount(totalCount),

		TimeSeriesChart: "timeseries.pdf",
		FontDir:         fontDir,
		StatRows:        tex.BuildStatRows(tsPoints, loc),

		HistogramTableTeX: histogramTableTeX,

		Source:              tex.EscapeTeX(cfg.Source),
		Group:               tex.EscapeTeX(cfg.Group),
		MinSpeed:            cfg.MinSpeed,
		CosineAngle:         cfg.CosineAngle,
		CosineFactor:        cosineFactor,
		CompareCosineAngle:  compareCosineAngle,
		CompareCosineFactor: compareCosineFactor,
		ModelVersion:        tex.EscapeTeX(cfg.ModelVersion),
		FirmwareVersion:     tex.EscapeTeX(cfg.FirmwareVersion),

		SpeedLimitNote: tex.EscapeTeX(cfg.SpeedLimitNote),
		PaperOption:    paperTexOption(paper),
	}

	if cfg.Histogram && histSVG != nil {
		td.HistogramChart = "histogram.pdf"
	}

	if mapPDFName != "" {
		td.MapChart = mapPDFName
	}

	if compareResult != nil {
		td.CompareChart = "comparison.pdf"
		td.CompareStartDate = compareResult.startDate
		td.CompareEndDate = compareResult.endDate
		td.CompareP50 = tex.FormatNumber(compareResult.p50)
		td.CompareP85 = tex.FormatNumber(compareResult.p85)
		td.CompareP98 = tex.FormatNumber(compareResult.p98)
		td.CompareMax = tex.FormatNumber(compareResult.maxSpeed)
		td.CompareCount = compareResult.count
		td.DeltaP50 = tex.FormatDelta(summaryP50, compareResult.p50)
		td.DeltaP85 = tex.FormatDelta(summaryP85, compareResult.p85)
		td.DeltaP98 = tex.FormatDelta(summaryP98, compareResult.p98)
		td.DeltaMax = tex.FormatDelta(summaryMax, compareResult.maxSpeed)
		td.DeltaP50Pct = tex.FormatDeltaPercent(summaryP50, compareResult.p50)
		td.DeltaP85Pct = tex.FormatDeltaPercent(summaryP85, compareResult.p85)
		td.DeltaP98Pct = tex.FormatDeltaPercent(summaryP98, compareResult.p98)
		td.DeltaMaxPct = tex.FormatDeltaPercent(summaryMax, compareResult.maxSpeed)
		td.CompareTotalCountFormatted = tex.FormatCount(compareResult.count)
		td.CombinedCountFormatted = tex.FormatCount(totalCount + compareResult.count)

		// Compare timeseries chart.
		if compareTimeSeriesPDFName != "" {
			td.CompareTimeSeriesChart = compareTimeSeriesPDFName
		}

		// Merge primary + compare hourly rows, sorted by time.
		mergedTS := mergeRollupRows(tsResult.Metrics, compareResult.tsRows)
		td.StatRows = tex.BuildStatRows(convertToTimeSeriesPoints(mergedTS, cfg.Units, loc), loc)

		// Merge primary + compare daily rows, sorted by time.
		mergedDaily := mergeRollupRows(primaryDailyRows, compareResult.dailyRows)
		td.DailyStatRows = tex.BuildStatRows(convertToTimeSeriesPoints(mergedDaily, cfg.Units, loc), loc)

		// Dual histogram table (6-column comparison table).
		if cfg.Histogram {
			primaryHist := convertHistogramKeys(summaryResult.Histogram, cfg.Units)
			compareHist := convertHistogramKeys(compareResult.histogram, cfg.Units)
			td.DualHistogramTableTeX = tex.BuildDualHistogramTableTeX(
				primaryHist, compareHist,
				cfg.HistBucketSize, cfg.MinSpeed, cfg.HistMax, cfg.Units,
			)
		}
	}

	// Build pre-rendered stat tables (single canonical style from BuildStatTableTeX).
	if compareResult != nil {
		td.StatTableTeX = tex.BuildStatTableTeX(td.StatRows, "Table 4: Granular Percentile Breakdown")
		td.DailyStatTableTeX = tex.BuildStatTableTeX(td.DailyStatRows, "Table 3: Daily Percentile Summary")
	} else {
		td.StatTableTeX = tex.BuildStatTableTeX(td.StatRows, "Detailed Data")
	}

	// Render .tex.
	texBytes, err := tex.RenderTeX(td)
	if err != nil {
		return Result{}, fmt.Errorf("render tex: %w", err)
	}
	texPath := filepath.Join(workDir, "report.tex")
	if err = os.WriteFile(texPath, texBytes, 0644); err != nil {
		return Result{}, fmt.Errorf("write report.tex: %w", err)
	}
	zipFiles["report.tex"] = texBytes

	// Embed font files under fonts/ so the .tex can be recompiled standalone.
	zipFiles["fonts/AtkinsonHyperlegible-Regular.ttf"] = fontRegular
	zipFiles["fonts/AtkinsonHyperlegible-Bold.ttf"] = fontBold
	zipFiles["fonts/AtkinsonHyperlegible-Italic.ttf"] = fontItalic
	zipFiles["fonts/AtkinsonHyperlegible-BoldItalic.ttf"] = fontBoldItalic

	zipFiles["README.md"] = []byte(zipReadme)

	// Compile PDF.
	if err = runXeLatex(ctx, workDir, "report.tex"); err != nil {
		return Result{}, fmt.Errorf("xelatex: %w", err)
	}

	// Build file names.
	safeLocation := sanitiseFilename(cfg.Location)
	baseName := fmt.Sprintf("%s_velocity.report_%s_report", cfg.EndDate, safeLocation)
	pdfName := baseName + ".pdf"
	zipName := baseName + "_sources.zip"

	// Build ZIP.
	zipBytes, err := BuildZip(zipFiles)
	if err != nil {
		return Result{}, fmt.Errorf("build zip: %w", err)
	}

	// Determine output directory.
	outDir, err := normaliseOutputDir(cfg.OutputDir, workDir)
	if err != nil {
		return Result{}, err
	}

	// Copy PDF to output.
	compiledPDF := filepath.Join(workDir, "report.pdf")
	pdfData, err := os.ReadFile(compiledPDF)
	if err != nil {
		return Result{}, fmt.Errorf("read compiled PDF: %w", err)
	}
	outPDF, err := safeOutputPath(outDir, pdfName)
	if err != nil {
		return Result{}, err
	}
	if err = os.WriteFile(outPDF, pdfData, 0644); err != nil {
		return Result{}, fmt.Errorf("write output PDF: %w", err)
	}

	outZIP, err := safeOutputPath(outDir, zipName)
	if err != nil {
		return Result{}, err
	}
	if err = os.WriteFile(outZIP, zipBytes, 0644); err != nil {
		return Result{}, fmt.Errorf("write output ZIP: %w", err)
	}

	// Clean up work dir if output is elsewhere.
	if cfg.OutputDir != "" {
		os.RemoveAll(workDir)
	}

	return Result{
		PDFPath: outPDF,
		ZIPPath: outZIP,
		RunID:   baseName,
	}, nil
}

// comparisonData holds converted comparison period results.
type comparisonData struct {
	startDate string
	endDate   string
	startTime time.Time
	endTime   time.Time
	p50       float64
	p85       float64
	p98       float64
	maxSpeed  float64
	count     int
	histogram map[float64]int64
	tsRows    []db.RadarObjectsRollupRow // hourly time-series rows
	dailyRows []db.RadarObjectsRollupRow // daily roll-up rows
}

func fetchComparison(ctx context.Context, database DB, cfg Config, loc *time.Location, minSpeedMPS, histBucketMPS, histMaxMPS float64, groupSeconds int64) (*comparisonData, error) {
	cs, err := time.ParseInLocation("2006-01-02", cfg.CompareStart, loc)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid compare start %q: %v", ErrInvalidConfig, cfg.CompareStart, err)
	}
	ce, err := time.ParseInLocation("2006-01-02", cfg.CompareEnd, loc)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid compare end %q: %v", ErrInvalidConfig, cfg.CompareEnd, err)
	}
	ce = ce.Add(24*time.Hour - time.Second)

	source := cfg.CompareSource
	if source == "" {
		source = cfg.Source
	}

	// Summary query (aggregate + histogram).
	summaryResult, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), 0, minSpeedMPS,
		source, cfg.ModelVersion,
		histBucketMPS, histMaxMPS,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("compare summary: %w", err)
	}

	// Hourly time-series query.
	tsResult, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), groupSeconds, minSpeedMPS,
		source, cfg.ModelVersion,
		0, 0,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("compare timeseries: %w", err)
	}

	// Daily roll-up query.
	dailyResult, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), 86400, minSpeedMPS,
		source, cfg.ModelVersion,
		0, 0,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("compare daily: %w", err)
	}

	cd := &comparisonData{
		startDate: cs.Format("2006-01-02"),
		endDate:   ce.Format("2006-01-02"),
		startTime: cs,
		endTime:   ce,
		histogram: summaryResult.Histogram,
		tsRows:    tsResult.Metrics,
		dailyRows: dailyResult.Metrics,
	}
	if len(summaryResult.Metrics) > 0 {
		row := summaryResult.Metrics[0]
		cd.p50 = units.ConvertSpeed(row.P50Speed, cfg.Units)
		cd.p85 = units.ConvertSpeed(row.P85Speed, cfg.Units)
		cd.p98 = units.ConvertSpeed(row.P98Speed, cfg.Units)
		cd.maxSpeed = units.ConvertSpeed(row.MaxSpeed, cfg.Units)
		cd.count = int(row.Count)
	}
	return cd, nil
}

// convertToTimeSeriesPoints converts DB rollup rows to chart points,
// converting speeds from mps to display units and times to loc.
func convertToTimeSeriesPoints(rows []db.RadarObjectsRollupRow, displayUnits string, loc *time.Location) []chart.TimeSeriesPoint {
	pts := make([]chart.TimeSeriesPoint, len(rows))
	for i, r := range rows {
		pt := chart.TimeSeriesPoint{
			StartTime: r.StartTime.In(loc),
			Count:     int(r.Count),
		}
		if r.Count == 0 {
			pt.P50Speed = math.NaN()
			pt.P85Speed = math.NaN()
			pt.P98Speed = math.NaN()
			pt.MaxSpeed = math.NaN()
		} else {
			pt.P50Speed = units.ConvertSpeed(r.P50Speed, displayUnits)
			pt.P85Speed = units.ConvertSpeed(r.P85Speed, displayUnits)
			pt.P98Speed = units.ConvertSpeed(r.P98Speed, displayUnits)
			pt.MaxSpeed = units.ConvertSpeed(r.MaxSpeed, displayUnits)
		}
		pts[i] = pt
	}
	return pts
}

// mergeRollupRows merges two slices of rollup rows and sorts them by StartTime.
func mergeRollupRows(a, b []db.RadarObjectsRollupRow) []db.RadarObjectsRollupRow {
	merged := make([]db.RadarObjectsRollupRow, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].StartTime.Before(merged[j].StartTime)
	})
	return merged
}

// convertHistogramKeys returns a new histogram map with keys converted
// from mps to display units.
func convertHistogramKeys(hist map[float64]int64, displayUnits string) map[float64]int64 {
	if hist == nil {
		return nil
	}
	out := make(map[float64]int64, len(hist))
	for k, v := range hist {
		out[units.ConvertSpeed(k, displayUnits)] = v
	}
	return out
}

func normaliseOutputDir(outputDir, workDir string) (string, error) {
	if outputDir == "" {
		return workDir, nil
	}

	cleaned := filepath.Clean(outputDir)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid output dir %q: must be an absolute path", outputDir)
	}

	absDir, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("normalise output dir: %w", err)
	}

	if err = os.MkdirAll(absDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	return absDir, nil
}

func safeOutputPath(outDir, fileName string) (string, error) {
	outPath := filepath.Clean(filepath.Join(outDir, fileName))
	rel, err := filepath.Rel(outDir, outPath)
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid output path: %q escapes output dir", fileName)
	}
	return outPath, nil
}

// convertSVGToPDF calls rsvg-convert to produce a PDF from an SVG.
func convertSVGToPDF(ctx context.Context, svgPath, pdfPath string) error {
	cmd := exec.CommandContext(ctx, "rsvg-convert", "-f", "pdf", "--dpi-x", "150", "--dpi-y", "150", "-o", pdfPath, svgPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsvg-convert failed: %w: %s", err, output)
	}
	return nil
}

// checkRsvgConvert verifies that rsvg-convert is available.
func checkRsvgConvert() error {
	_, err := exec.LookPath("rsvg-convert")
	if err != nil {
		return fmt.Errorf("rsvg-convert not found: install via 'apt install librsvg2-bin' (Linux) or 'brew install librsvg' (macOS)")
	}
	return nil
}

// runXeLatex compiles a .tex file to PDF using xelatex.
// Runs two passes for cross-references and fancyhdr.
func runXeLatex(ctx context.Context, texDir, texFile string) error {
	latexEnv := resolveTexEnvironment()

	for pass := 0; pass < 2; pass++ {
		cmd := exec.CommandContext(ctx, latexEnv.compiler, buildXeLatexArgs(texFile, latexEnv.fmtName)...)
		cmd.Dir = texDir
		if len(latexEnv.env) > 0 {
			cmd.Env = append(os.Environ(), latexEnv.env...)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			logExcerpt := readLogExcerpt(texDir, texFile)
			return fmt.Errorf("xelatex pass %d failed: %w\nOutput: %s\nLog excerpt: %s", pass+1, err, output, logExcerpt)
		}
	}
	return nil
}

type texEnvironment struct {
	compiler string
	env      []string
	fmtName  string
}

func resolveTexEnvironment() texEnvironment {
	texRoot := resolvedTexRoot()
	if texRoot == "" {
		return texEnvironment{compiler: "xelatex"}
	}

	fmtDir := filepath.Join(texRoot, "texmf-dist", "web2c", "xelatex")
	fmtName := ""
	if shouldUseVelocityFmt() {
		if _, err := os.Stat(filepath.Join(fmtDir, "velocity-report.fmt")); err == nil {
			fmtName = "velocity-report"
		}
	}

	return texEnvironment{
		compiler: filepath.Join(texRoot, "bin", "xelatex"),
		env:      buildTexEnv(texRoot),
		fmtName:  fmtName,
	}
}

func resolvedTexRoot() string {
	texRoot := strings.TrimSpace(os.Getenv("VELOCITY_TEX_ROOT"))
	if texRoot == "" {
		return ""
	}
	absRoot, err := filepath.Abs(filepath.Clean(texRoot))
	if err != nil {
		return texRoot
	}
	return absRoot
}

func shouldUseVelocityFmt() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("VELOCITY_USE_VELOCITY_FMT"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildXeLatexArgs(texFile, fmtName string) []string {
	args := []string{"-interaction=nonstopmode", "-halt-on-error"}
	if fmtName != "" {
		args = append(args, "-fmt="+fmtName)
	}
	args = append(args, texFile)
	return args
}

// buildTexEnv returns environment variables for a vendored TeX installation.
func buildTexEnv(texRoot string) []string {
	sep := string(os.PathListSeparator)
	texmfDist := filepath.Join(texRoot, "texmf-dist")
	texmfHome := filepath.Join(texRoot, "texmf")
	texmfVar := filepath.Join(texRoot, "texmf-var")
	binDir := filepath.Join(texRoot, "bin")
	web2cDir := filepath.Join(texmfDist, "web2c")

	env := []string{
		fmt.Sprintf("TEXMFHOME=%s", texmfHome),
		fmt.Sprintf("TEXMFDIST=%s", texmfDist),
		fmt.Sprintf("TEXMFVAR=%s", texmfVar),
		fmt.Sprintf("TEXMFCNF=%s%s", web2cDir, sep),
		fmt.Sprintf("TEXINPUTS=%s//%s", filepath.Join(texmfDist, "tex"), sep),
		fmt.Sprintf("TFMFONTS=%s//%s", filepath.Join(texmfDist, "fonts", "tfm"), sep),
		fmt.Sprintf("OPENTYPEFONTS=%s//%s", filepath.Join(texmfDist, "fonts", "opentype"), sep),
		fmt.Sprintf("OSFONTDIR=%s//%s", filepath.Join(texmfDist, "fonts"), sep),
	}

	currentPath := os.Getenv("PATH")
	if currentPath != "" {
		env = append(env, fmt.Sprintf("PATH=%s%s%s", binDir, sep, currentPath))
	} else {
		env = append(env, fmt.Sprintf("PATH=%s", binDir))
	}

	fmtDir := filepath.Join(texmfDist, "web2c", "xelatex")
	if _, err := os.Stat(filepath.Join(fmtDir, "xelatex.fmt")); err == nil {
		existingFormats := os.Getenv("TEXFORMATS")
		if existingFormats != "" {
			env = append(env, fmt.Sprintf("TEXFORMATS=%s%s%s", fmtDir, sep, existingFormats))
		} else {
			env = append(env, fmt.Sprintf("TEXFORMATS=%s%s", fmtDir, sep))
		}
	}

	libDir := filepath.Join(texRoot, "lib")
	if info, err := os.Stat(libDir); err == nil && info.IsDir() {
		existingLD := os.Getenv("LD_LIBRARY_PATH")
		if existingLD != "" {
			env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s%s%s", libDir, sep, existingLD))
		} else {
			env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s", libDir))
		}
	}

	return env
}

// checkXeLatex verifies that xelatex is available.
func checkXeLatex() error {
	texRoot := resolvedTexRoot()
	if texRoot != "" {
		compiler := filepath.Join(texRoot, "bin", "xelatex")
		if _, err := os.Stat(compiler); err != nil {
			return fmt.Errorf("vendored xelatex not found at %s", compiler)
		}
		return nil
	}
	_, err := exec.LookPath("xelatex")
	if err != nil {
		return fmt.Errorf("xelatex not found: install TeX Live or set VELOCITY_TEX_ROOT")
	}
	return nil
}

// readLogExcerpt returns the last 50 lines of a .log file for diagnostics.
func readLogExcerpt(texDir, texFile string) string {
	logFile := strings.TrimSuffix(texFile, ".tex") + ".log"
	data, err := os.ReadFile(filepath.Join(texDir, logFile))
	if err != nil {
		return "(log file not found)"
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}
	return strings.Join(lines, "\n")
}

// paperTexOption returns the LaTeX documentclass paper option for a PaperSize.
func paperTexOption(p chart.PaperSize) string {
	if p == chart.PaperLetter {
		return "letterpaper"
	}
	return "a4paper"
}

// sanitiseFilename replaces characters unsuitable for file names.
func sanitiseFilename(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	return b.String()
}
