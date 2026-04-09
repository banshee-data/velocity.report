package report

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/tex"
	"github.com/banshee-data/velocity.report/internal/units"
)

//go:embed chart/assets/AtkinsonHyperlegible-Regular.ttf
var fontRegular []byte

//go:embed chart/assets/AtkinsonHyperlegible-Bold.ttf
var fontBold []byte

//go:embed chart/assets/AtkinsonHyperlegible-Italic.ttf
var fontItalic []byte

//go:embed chart/assets/AtkinsonHyperlegible-BoldItalic.ttf
var fontBoldItalic []byte

// Generate produces a PDF report and source ZIP for the given configuration.
func Generate(ctx context.Context, database DB, cfg Config) (Result, error) {
	// Validate group.
	groupSeconds, ok := supportedGroups[cfg.Group]
	if !ok {
		return Result{}, fmt.Errorf("unsupported group %q", cfg.Group)
	}

	// Check external tool availability.
	if err := checkRsvgConvert(); err != nil {
		return Result{}, err
	}
	if err := checkXeLatex(); err != nil {
		return Result{}, err
	}

	// Parse dates.
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return Result{}, fmt.Errorf("invalid timezone %q: %w", cfg.Timezone, err)
	}

	startTime, err := time.ParseInLocation("2006-01-02", cfg.StartDate, loc)
	if err != nil {
		return Result{}, fmt.Errorf("invalid start date %q: %w", cfg.StartDate, err)
	}
	endTime, err := time.ParseInLocation("2006-01-02", cfg.EndDate, loc)
	if err != nil {
		return Result{}, fmt.Errorf("invalid end date %q: %w", cfg.EndDate, err)
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

	// Comparison data (if requested).
	var compareResult *comparisonData
	if cfg.CompareStart != "" {
		cd, err := fetchComparison(ctx, database, cfg, loc, minSpeedMPS, histBucketMPS, histMaxMPS)
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

	// Convert DB rows to chart data.
	tsPoints := convertToTimeSeriesPoints(tsResult.Metrics, cfg.Units)
	tsData := chart.TimeSeriesData{
		Points: tsPoints,
		Units:  cfg.Units,
		Title:  fmt.Sprintf("Vehicle speeds — %s", cfg.Location),
	}

	// Render time-series SVG.
	tsSVG, err := chart.RenderTimeSeries(tsData, chart.DefaultTimeSeriesStyle())
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

		histSVG, err = chart.RenderHistogram(histData, chart.DefaultHistogramStyle())
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

	// Comparison chart (if requested).
	if compareResult != nil && cfg.Histogram {
		primaryHist := convertHistogramKeys(summaryResult.Histogram, cfg.Units)
		compareHist := convertHistogramKeys(compareResult.histogram, cfg.Units)

		compSVG, cerr := chart.RenderComparison(
			chart.HistogramData{Buckets: primaryHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
			chart.HistogramData{Buckets: compareHist, Units: cfg.Units, BucketSz: cfg.HistBucketSize, MaxBucket: cfg.HistMax, Cutoff: cfg.MinSpeed},
			fmt.Sprintf("%s–%s", cfg.StartDate, cfg.EndDate),
			fmt.Sprintf("%s–%s", cfg.CompareStart, cfg.CompareEnd),
			chart.DefaultHistogramStyle(),
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

	// Build summary statistics from the aggregate row.
	var summaryP50, summaryP85, summaryP98, summaryMax float64
	var totalCount int
	if len(summaryResult.Metrics) > 0 {
		row := summaryResult.Metrics[0]
		summaryP50 = units.ConvertSpeed(row.P50Speed, cfg.Units)
		summaryP85 = units.ConvertSpeed(row.P85Speed, cfg.Units)
		summaryP98 = units.ConvertSpeed(row.P98Speed, cfg.Units)
		summaryMax = units.ConvertSpeed(row.MaxSpeed, cfg.Units)
		totalCount = int(row.Count)
	}

	// Cosine correction factor.
	cosineFactor := 1.0
	if cfg.CosineAngle != 0 {
		cosineFactor = 1.0 / math.Cos(cfg.CosineAngle*math.Pi/180.0)
	}

	// Build TemplateData.
	td := tex.TemplateData{
		Location:    tex.EscapeTeX(cfg.Location),
		Surveyor:    tex.EscapeTeX(cfg.Surveyor),
		Contact:     tex.EscapeTeX(cfg.Contact),
		SpeedLimit:  cfg.SpeedLimit,
		Description: tex.EscapeTeX(cfg.SiteDescription),

		StartDate: startTime.Format("2 January 2006"),
		EndDate:   endTime.Format("2 January 2006"),
		Timezone:  cfg.Timezone,
		Units:     cfg.Units,

		P50:        tex.FormatNumber(summaryP50),
		P85:        tex.FormatNumber(summaryP85),
		P98:        tex.FormatNumber(summaryP98),
		MaxSpeed:   tex.FormatNumber(summaryMax),
		TotalCount: totalCount,
		HoursCount: int(math.Ceil(float64(endUnix-startUnix) / 3600.0)),

		TimeSeriesChart: "timeseries.pdf",
		FontDir:         fontDir,
		StatRows:        tex.BuildStatRows(tsPoints, loc),

		HistogramTableTeX: histogramTableTeX,

		Source:       cfg.Source,
		Group:        cfg.Group,
		MinSpeed:     cfg.MinSpeed,
		CosineAngle:  cfg.CosineAngle,
		CosineFactor: cosineFactor,
		ModelVersion: cfg.ModelVersion,

		SpeedLimitNote: tex.EscapeTeX(cfg.SpeedLimitNote),
	}

	if cfg.Histogram && histSVG != nil {
		td.HistogramChart = "histogram.pdf"
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
	outDir := cfg.OutputDir
	if outDir == "" {
		outDir = workDir
	} else {
		if err = os.MkdirAll(outDir, 0755); err != nil {
			return Result{}, fmt.Errorf("create output dir: %w", err)
		}
	}

	// Copy PDF to output.
	compiledPDF := filepath.Join(workDir, "report.pdf")
	pdfData, err := os.ReadFile(compiledPDF)
	if err != nil {
		return Result{}, fmt.Errorf("read compiled PDF: %w", err)
	}
	outPDF := filepath.Join(outDir, pdfName)
	if err = os.WriteFile(outPDF, pdfData, 0644); err != nil {
		return Result{}, fmt.Errorf("write output PDF: %w", err)
	}

	outZIP := filepath.Join(outDir, zipName)
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
	p50       float64
	p85       float64
	p98       float64
	maxSpeed  float64
	count     int
	histogram map[float64]int64
}

func fetchComparison(ctx context.Context, database DB, cfg Config, loc *time.Location, minSpeedMPS, histBucketMPS, histMaxMPS float64) (*comparisonData, error) {
	cs, err := time.ParseInLocation("2006-01-02", cfg.CompareStart, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid compare start %q: %w", cfg.CompareStart, err)
	}
	ce, err := time.ParseInLocation("2006-01-02", cfg.CompareEnd, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid compare end %q: %w", cfg.CompareEnd, err)
	}
	ce = ce.Add(24*time.Hour - time.Second)

	source := cfg.CompareSource
	if source == "" {
		source = cfg.Source
	}

	result, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), 0, minSpeedMPS,
		source, cfg.ModelVersion,
		histBucketMPS, histMaxMPS,
		cfg.SiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return nil, err
	}

	cd := &comparisonData{
		startDate: cs.Format("2 January 2006"),
		endDate:   ce.Format("2 January 2006"),
		histogram: result.Histogram,
	}
	if len(result.Metrics) > 0 {
		row := result.Metrics[0]
		cd.p50 = units.ConvertSpeed(row.P50Speed, cfg.Units)
		cd.p85 = units.ConvertSpeed(row.P85Speed, cfg.Units)
		cd.p98 = units.ConvertSpeed(row.P98Speed, cfg.Units)
		cd.maxSpeed = units.ConvertSpeed(row.MaxSpeed, cfg.Units)
		cd.count = int(row.Count)
	}
	return cd, nil
}

// convertToTimeSeriesPoints converts DB rollup rows to chart points,
// converting speeds from mps to display units.
func convertToTimeSeriesPoints(rows []db.RadarObjectsRollupRow, displayUnits string) []chart.TimeSeriesPoint {
	pts := make([]chart.TimeSeriesPoint, len(rows))
	for i, r := range rows {
		pts[i] = chart.TimeSeriesPoint{
			StartTime: r.StartTime,
			P50Speed:  units.ConvertSpeed(r.P50Speed, displayUnits),
			P85Speed:  units.ConvertSpeed(r.P85Speed, displayUnits),
			P98Speed:  units.ConvertSpeed(r.P98Speed, displayUnits),
			MaxSpeed:  units.ConvertSpeed(r.MaxSpeed, displayUnits),
			Count:     int(r.Count),
		}
	}
	return pts
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
	texRoot := os.Getenv("VELOCITY_TEX_ROOT")

	var compiler string
	var extraEnv []string

	if texRoot != "" {
		compiler = filepath.Join(texRoot, "bin", "xelatex")
		extraEnv = buildTexEnv(texRoot)
	} else {
		compiler = "xelatex"
	}

	for pass := 0; pass < 2; pass++ {
		cmd := exec.CommandContext(ctx, compiler, "-interaction=nonstopmode", "-halt-on-error", texFile)
		cmd.Dir = texDir
		if len(extraEnv) > 0 {
			cmd.Env = append(os.Environ(), extraEnv...)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			logExcerpt := readLogExcerpt(texDir, texFile)
			return fmt.Errorf("xelatex pass %d failed: %w\nOutput: %s\nLog excerpt: %s", pass+1, err, output, logExcerpt)
		}
	}
	return nil
}

// buildTexEnv returns environment variables for a vendored TeX installation.
func buildTexEnv(texRoot string) []string {
	return []string{
		fmt.Sprintf("TEXMFHOME=%s/texmf-dist", texRoot),
		fmt.Sprintf("TEXMFDIST=%s/texmf-dist", texRoot),
		fmt.Sprintf("TEXMFVAR=%s/texmf-var", texRoot),
		fmt.Sprintf("TEXMFCNF=%s/texmf-dist/web2c", texRoot),
		fmt.Sprintf("TEXINPUTS=.:%s/texmf-dist//:", texRoot),
		fmt.Sprintf("TFMFONTS=%s/texmf-dist/fonts//:", texRoot),
		fmt.Sprintf("OPENTYPEFONTS=%s/texmf-dist/fonts//:", texRoot),
		fmt.Sprintf("OSFONTDIR=%s/texmf-dist/fonts//:", texRoot),
	}
}

// checkXeLatex verifies that xelatex is available.
func checkXeLatex() error {
	texRoot := os.Getenv("VELOCITY_TEX_ROOT")
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
