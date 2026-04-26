package report

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/chart/assets"
	"github.com/banshee-data/velocity.report/internal/report/tex"
	"github.com/banshee-data/velocity.report/internal/units"
)

type runPlan struct {
	cfg           Config
	groupSeconds  int64
	loc           *time.Location
	startTime     time.Time
	endTime       time.Time
	startUnix     int64
	endUnix       int64
	minSpeedMPS   float64
	histBucketMPS float64
	histMaxMPS    float64
	paper         chart.PaperSize
}

func planRun(cfg Config) (runPlan, error) {
	groupSeconds, ok := supportedGroups[cfg.Group]
	if !ok {
		return runPlan{}, fmt.Errorf("%w: unsupported group %q", ErrInvalidConfig, cfg.Group)
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return runPlan{}, fmt.Errorf("%w: invalid timezone %q: %v", ErrInvalidConfig, cfg.Timezone, err)
	}

	startTime, err := time.ParseInLocation("2006-01-02", cfg.StartDate, loc)
	if err != nil {
		return runPlan{}, fmt.Errorf("%w: invalid start date %q: %v", ErrInvalidConfig, cfg.StartDate, err)
	}
	endTime, err := time.ParseInLocation("2006-01-02", cfg.EndDate, loc)
	if err != nil {
		return runPlan{}, fmt.Errorf("%w: invalid end date %q: %v", ErrInvalidConfig, cfg.EndDate, err)
	}
	endTime = inclusiveLocalDateEnd(endTime)

	plan := runPlan{
		cfg:          cfg,
		groupSeconds: groupSeconds,
		loc:          loc,
		startTime:    startTime,
		endTime:      endTime,
		startUnix:    startTime.Unix(),
		endUnix:      endTime.Unix(),
		minSpeedMPS:  units.ConvertToMPS(cfg.MinSpeed, cfg.Units),
		paper:        chart.NormalisePaperSize(cfg.PaperSize),
	}
	if cfg.Histogram {
		plan.histBucketMPS = units.ConvertToMPS(cfg.HistBucketSize, cfg.Units)
		plan.histMaxMPS = units.ConvertToMPS(cfg.HistMax, cfg.Units)
	}

	return plan, nil
}

func inclusiveLocalDateEnd(day time.Time) time.Time {
	return day.AddDate(0, 0, 1).Add(-time.Second)
}

type loadedData struct {
	summaryResult *db.RadarStatsResult
	tsResult      *db.RadarStatsResult
	primaryDaily  []db.RadarObjectsRollupRow
	compareResult *comparisonData
	summaryP50    float64
	summaryP85    float64
	summaryP98    float64
	summaryMax    float64
	totalCount    int
	summaryP98Ref float64
}

func loadData(ctx context.Context, database DB, plan runPlan) (loadedData, error) {
	cfg := plan.cfg

	summaryResult, err := database.RadarObjectRollupRange(
		plan.startUnix, plan.endUnix, 0, plan.minSpeedMPS,
		cfg.Source, cfg.ModelVersion,
		plan.histBucketMPS, plan.histMaxMPS,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return loadedData{}, fmt.Errorf("summary query: %w", err)
	}

	tsResult, err := database.RadarObjectRollupRange(
		plan.startUnix, plan.endUnix, plan.groupSeconds, plan.minSpeedMPS,
		cfg.Source, cfg.ModelVersion,
		0, 0,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return loadedData{}, fmt.Errorf("time-series query: %w", err)
	}

	var primaryDaily []db.RadarObjectsRollupRow
	if cfg.CompareStart != "" {
		dailyResult, err := database.RadarObjectRollupRange(
			plan.startUnix, plan.endUnix, 86400, plan.minSpeedMPS,
			cfg.Source, cfg.ModelVersion,
			0, 0,
			cfg.SiteID, cfg.BoundaryThreshold,
		)
		if err != nil {
			return loadedData{}, fmt.Errorf("primary daily query: %w", err)
		}
		primaryDaily = dailyResult.Metrics
	}

	var compareResult *comparisonData
	if cfg.CompareStart != "" {
		cd, err := fetchComparison(ctx, database, cfg, plan.loc, plan.minSpeedMPS, plan.histBucketMPS, plan.histMaxMPS, plan.groupSeconds)
		if err != nil {
			return loadedData{}, fmt.Errorf("comparison query: %w", err)
		}
		compareResult = cd
	}

	data := loadedData{
		summaryResult: summaryResult,
		tsResult:      tsResult,
		primaryDaily:  primaryDaily,
		compareResult: compareResult,
		summaryP98Ref: math.NaN(),
	}
	if len(summaryResult.Metrics) > 0 {
		row := summaryResult.Metrics[0]
		data.summaryP50 = units.ConvertSpeed(row.P50Speed, cfg.Units)
		data.summaryP85 = units.ConvertSpeed(row.P85Speed, cfg.Units)
		data.summaryP98 = units.ConvertSpeed(row.P98Speed, cfg.Units)
		data.summaryMax = units.ConvertSpeed(row.MaxSpeed, cfg.Units)
		data.summaryP98Ref = data.summaryP98
		data.totalCount = int(row.Count)
	}

	return data, nil
}

type workState struct {
	dir     string
	fontDir string
}

func newWorkdir() (workState, error) {
	workDir, err := os.MkdirTemp("", "velocity-report-*")
	if err != nil {
		return workState{}, fmt.Errorf("create temp dir: %w", err)
	}

	for name, data := range assets.AllFonts() {
		if err := os.WriteFile(filepath.Join(workDir, name), data, 0644); err != nil {
			os.RemoveAll(workDir)
			return workState{}, fmt.Errorf("write font %s: %w", name, err)
		}
	}

	return workState{dir: workDir, fontDir: workDir}, nil
}

func (w workState) cleanupOnError(errp *error) {
	if errp != nil && *errp != nil {
		os.RemoveAll(w.dir)
	}
}

type chartSet struct {
	zipFiles                 map[string][]byte
	histogramTableTeX        string
	mapPDFName               string
	compareTimeSeriesPDFName string
}

func renderCharts(ctx context.Context, plan runPlan, data loadedData, work workState) (chartSet, error) {
	cfg := plan.cfg

	charts := chartSet{zipFiles: map[string][]byte{}}

	tsPoints := convertToTimeSeriesPoints(data.tsResult.Metrics, cfg.Units, plan.loc)
	if cfg.ExpandedChart {
		tsPoints = chart.ExpandTimeSeriesGapsInRange(tsPoints, plan.groupSeconds, plan.startTime, plan.endTime)
	}
	tsData := chart.TimeSeriesData{
		Points:       tsPoints,
		Units:        cfg.Units,
		Title:        "",
		P98Reference: data.summaryP98Ref,
		MaxReference: data.summaryMax,
	}
	tsSVG, err := chart.RenderTimeSeries(tsData, chart.DefaultTimeSeriesStyle(plan.paper))
	if err != nil {
		return chartSet{}, fmt.Errorf("render time-series: %w", err)
	}
	if err := (chartArtifact{name: "timeseries", svg: tsSVG, workDir: work.dir, zipFiles: charts.zipFiles}).materialise(ctx); err != nil {
		return chartSet{}, err
	}

	if data.compareResult != nil {
		ctsPoints := convertToTimeSeriesPoints(data.compareResult.tsRows, cfg.Units, plan.loc)
		if cfg.ExpandedChart {
			ctsPoints = chart.ExpandTimeSeriesGapsInRange(ctsPoints, plan.groupSeconds, data.compareResult.startTime, data.compareResult.endTime)
		}
		ctsData := chart.TimeSeriesData{
			Points:       ctsPoints,
			Units:        cfg.Units,
			P98Reference: data.compareResult.p98,
			MaxReference: data.compareResult.maxSpeed,
		}
		ctsSVG, err := chart.RenderTimeSeries(ctsData, chart.DefaultTimeSeriesStyle(plan.paper))
		if err != nil {
			return chartSet{}, fmt.Errorf("render compare timeseries: %w", err)
		}
		if err := (chartArtifact{name: "timeseries_compare", svg: ctsSVG, workDir: work.dir, zipFiles: charts.zipFiles}).materialise(ctx); err != nil {
			return chartSet{}, err
		}
		charts.compareTimeSeriesPDFName = "timeseries_compare.pdf"
	}

	if cfg.Histogram && data.summaryResult.Histogram != nil {
		displayHist := convertHistogramKeys(data.summaryResult.Histogram, cfg.Units)
		histData := chart.HistogramData{
			Buckets:   displayHist,
			Units:     cfg.Units,
			BucketSz:  cfg.HistBucketSize,
			MaxBucket: cfg.HistMax,
			Cutoff:    cfg.MinSpeed,
		}

		histSVG, err := chart.RenderHistogram(histData, chart.DefaultHistogramStyle(plan.paper))
		if err != nil {
			return chartSet{}, fmt.Errorf("render histogram: %w", err)
		}
		if err := (chartArtifact{name: "histogram", svg: histSVG, workDir: work.dir, zipFiles: charts.zipFiles}).materialise(ctx); err != nil {
			return chartSet{}, err
		}

		charts.histogramTableTeX = tex.BuildHistogramTableTeX(
			displayHist, cfg.HistBucketSize, cfg.MinSpeed, cfg.HistMax, cfg.Units,
		)
	}

	if cfg.IncludeMap && len(cfg.MapSVG) > 0 {
		if err := (chartArtifact{name: "map", svg: cfg.MapSVG, workDir: work.dir, zipFiles: charts.zipFiles}).materialise(ctx); err != nil {
			return chartSet{}, err
		}
		charts.mapPDFName = "map.pdf"
	}

	if data.compareResult != nil && cfg.Histogram {
		primaryHist := convertHistogramKeys(data.summaryResult.Histogram, cfg.Units)
		compareHist := convertHistogramKeys(data.compareResult.histogram, cfg.Units)

		compSVG, err := chart.RenderComparison(
			chart.HistogramData{Buckets: primaryHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
			chart.HistogramData{Buckets: compareHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
			fmt.Sprintf("%s–%s", cfg.StartDate, cfg.EndDate),
			fmt.Sprintf("%s–%s", cfg.CompareStart, cfg.CompareEnd),
			chart.DefaultHistogramStyle(plan.paper),
		)
		if err != nil {
			return chartSet{}, fmt.Errorf("render comparison: %w", err)
		}
		if err := (chartArtifact{name: "comparison", svg: compSVG, workDir: work.dir, zipFiles: charts.zipFiles}).materialise(ctx); err != nil {
			return chartSet{}, err
		}
	}

	return charts, nil
}

func buildTemplateData(plan runPlan, data loadedData, charts chartSet, work workState) tex.TemplateData {
	cfg := plan.cfg

	cosineFactor := 1.0
	if cfg.CosineAngle != 0 {
		cosineFactor = 1.0 / math.Cos(cfg.CosineAngle*math.Pi/180.0)
	}

	compareCosineAngle := cfg.CompareCosineAngle
	compareCosineFactor := 1.0
	if compareCosineAngle != 0 {
		compareCosineFactor = 1.0 / math.Cos(compareCosineAngle*math.Pi/180.0)
	}

	tsPoints := convertToTimeSeriesPoints(data.tsResult.Metrics, cfg.Units, plan.loc)
	td := tex.TemplateData{
		Location:    tex.EscapeTeX(cfg.Location),
		Surveyor:    tex.EscapeTeX(cfg.Surveyor),
		Contact:     tex.EscapeTeX(cfg.Contact),
		SpeedLimit:  cfg.SpeedLimit,
		Description: tex.EscapeTeX(cfg.SiteDescription),

		StartDate: plan.startTime.Format("2006-01-02"),
		EndDate:   plan.endTime.Format("2006-01-02"),
		Timezone:  tex.EscapeTeX(cfg.Timezone),
		Units:     tex.EscapeTeX(cfg.Units),

		P50:        tex.FormatNumber(data.summaryP50),
		P85:        tex.FormatNumber(data.summaryP85),
		P98:        tex.FormatNumber(data.summaryP98),
		MaxSpeed:   tex.FormatNumber(data.summaryMax),
		TotalCount: data.totalCount,
		HoursCount: int(math.Ceil(float64(plan.endUnix-plan.startUnix) / 3600.0)),

		TotalCountFormatted: tex.FormatCount(data.totalCount),

		TimeSeriesChart: "timeseries.pdf",
		FontDir:         work.fontDir,
		StatRows:        tex.BuildStatRows(tsPoints, plan.loc),

		HistogramTableTeX: charts.histogramTableTeX,

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
		PaperOption:    paperTexOption(plan.paper),
	}
	if _, ok := charts.zipFiles["histogram.svg"]; ok {
		td.HistogramChart = "histogram.pdf"
	}
	if charts.mapPDFName != "" {
		td.MapChart = charts.mapPDFName
	}

	if data.compareResult != nil {
		td.CompareChart = "comparison.pdf"
		td.CompareSource = tex.EscapeTeX(cfg.CompareSource)
		td.CompareStartDate = data.compareResult.startDate
		td.CompareEndDate = data.compareResult.endDate
		td.CompareP50 = tex.FormatNumber(data.compareResult.p50)
		td.CompareP85 = tex.FormatNumber(data.compareResult.p85)
		td.CompareP98 = tex.FormatNumber(data.compareResult.p98)
		td.CompareMax = tex.FormatNumber(data.compareResult.maxSpeed)
		td.CompareCount = data.compareResult.count
		td.DeltaP50 = tex.FormatDelta(data.summaryP50, data.compareResult.p50)
		td.DeltaP85 = tex.FormatDelta(data.summaryP85, data.compareResult.p85)
		td.DeltaP98 = tex.FormatDelta(data.summaryP98, data.compareResult.p98)
		td.DeltaMax = tex.FormatDelta(data.summaryMax, data.compareResult.maxSpeed)
		td.DeltaP50Pct = tex.FormatDeltaPercent(data.summaryP50, data.compareResult.p50)
		td.DeltaP85Pct = tex.FormatDeltaPercent(data.summaryP85, data.compareResult.p85)
		td.DeltaP98Pct = tex.FormatDeltaPercent(data.summaryP98, data.compareResult.p98)
		td.DeltaMaxPct = tex.FormatDeltaPercent(data.summaryMax, data.compareResult.maxSpeed)
		td.CompareTotalCountFormatted = tex.FormatCount(data.compareResult.count)
		td.CombinedCountFormatted = tex.FormatCount(data.totalCount + data.compareResult.count)

		if charts.compareTimeSeriesPDFName != "" {
			td.CompareTimeSeriesChart = charts.compareTimeSeriesPDFName
		}

		mergedTS := mergeRollupRows(data.tsResult.Metrics, data.compareResult.tsRows)
		td.StatRows = tex.BuildStatRows(convertToTimeSeriesPoints(mergedTS, cfg.Units, plan.loc), plan.loc)

		mergedDaily := mergeRollupRows(data.primaryDaily, data.compareResult.dailyRows)
		td.DailyStatRows = tex.BuildStatRows(convertToTimeSeriesPoints(mergedDaily, cfg.Units, plan.loc), plan.loc)

		if cfg.Histogram {
			primaryHist := convertHistogramKeys(data.summaryResult.Histogram, cfg.Units)
			compareHist := convertHistogramKeys(data.compareResult.histogram, cfg.Units)
			td.DualHistogramTableTeX = tex.BuildDualHistogramTableTeX(
				primaryHist, compareHist,
				cfg.HistBucketSize, cfg.MinSpeed, cfg.HistMax, cfg.Units,
			)
		}
	}

	if data.compareResult != nil {
		td.KeyMetricsTableTeX = tex.BuildComparisonKeyMetricsTableTeX(
			td.P50, td.P85, td.P98, td.MaxSpeed,
			td.CompareP50, td.CompareP85, td.CompareP98, td.CompareMax,
			td.DeltaP50Pct, td.DeltaP85Pct, td.DeltaP98Pct, td.DeltaMaxPct,
			td.TotalCountFormatted, td.CompareTotalCountFormatted,
			td.Units,
		)
	} else {
		td.KeyMetricsTableTeX = tex.BuildSingleKeyMetricsTableTeX(
			td.P50, td.P85, td.P98, td.MaxSpeed, td.Units,
		)
	}

	if data.compareResult != nil {
		td.StatTableTeX = tex.BuildStatTableTeX(td.StatRows, "Table 4: Granular Percentile Breakdown")
		td.DailyStatTableTeX = tex.BuildStatTableTeX(td.DailyStatRows, "Table 3: Daily Percentile Summary")
	} else {
		td.StatTableTeX = tex.BuildStatTableTeX(td.StatRows, "Detailed Data")
	}

	return td
}

func writeTex(work workState, td tex.TemplateData, zipFiles map[string][]byte) error {
	texBytes, err := tex.RenderTeX(td)
	if err != nil {
		return fmt.Errorf("render tex: %w", err)
	}
	texPath := filepath.Join(work.dir, "report.tex")
	if err := os.WriteFile(texPath, texBytes, 0644); err != nil {
		return fmt.Errorf("write report.tex: %w", err)
	}
	zipFiles["report.tex"] = texBytes

	for name, data := range assets.AllFonts() {
		zipFiles[filepath.Join("fonts", name)] = data
	}
	zipFiles["README.md"] = []byte(zipReadme)
	return nil
}

func compilePDF(ctx context.Context, work workState) error {
	if err := runXeLatex(ctx, work.dir, "report.tex"); err != nil {
		return fmt.Errorf("xelatex: %w", err)
	}
	return nil
}

func packageOutput(cfg Config, work workState, zipFiles map[string][]byte) (Result, error) {
	safeLocation := sanitiseFilename(cfg.Location)
	baseName := fmt.Sprintf("%s_velocity.report_%s_report", cfg.EndDate, safeLocation)
	pdfName := baseName + ".pdf"
	zipName := baseName + "_sources.zip"

	zipBytes, err := BuildZip(zipFiles)
	if err != nil {
		return Result{}, fmt.Errorf("build zip: %w", err)
	}

	outDir, err := normaliseOutputDir(cfg.OutputDir, work.dir)
	if err != nil {
		return Result{}, err
	}

	compiledPDF := filepath.Join(work.dir, "report.pdf")
	pdfData, err := os.ReadFile(compiledPDF)
	if err != nil {
		return Result{}, fmt.Errorf("read compiled PDF: %w", err)
	}
	outPDF, err := safeOutputPath(outDir, pdfName)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outPDF, pdfData, 0644); err != nil {
		return Result{}, fmt.Errorf("write output PDF: %w", err)
	}

	outZIP, err := safeOutputPath(outDir, zipName)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outZIP, zipBytes, 0644); err != nil {
		return Result{}, fmt.Errorf("write output ZIP: %w", err)
	}

	if cfg.OutputDir != "" {
		os.RemoveAll(work.dir)
	}

	return Result{
		PDFPath: outPDF,
		ZIPPath: outZIP,
		RunID:   baseName,
	}, nil
}
