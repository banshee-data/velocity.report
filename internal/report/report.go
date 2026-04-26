package report

import (
	"context"
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
	"github.com/banshee-data/velocity.report/internal/units"
)

// ErrInvalidConfig wraps all Generate errors caused by bad caller input
// (unknown group, unparseable timezone or date). Handlers can use
// errors.Is(err, report.ErrInvalidConfig) to map these to HTTP 4xx
// responses; every other Generate error is a server-side failure.
var ErrInvalidConfig = errors.New("invalid report config")

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
	plan, err := planRun(cfg)
	if err != nil {
		return Result{}, err
	}

	if err := checkRsvgConvert(); err != nil {
		return Result{}, err
	}
	if err := checkXeLatex(); err != nil {
		return Result{}, err
	}

	data, err := loadData(ctx, database, plan)
	if err != nil {
		return Result{}, err
	}

	work, err := newWorkdir()
	if err != nil {
		return Result{}, err
	}
	defer work.cleanupOnError(&err)

	charts, err := renderCharts(ctx, plan, data, work)
	if err != nil {
		return Result{}, err
	}

	td := buildTemplateData(plan, data, charts, work)
	if err = writeTex(work, td, charts.zipFiles); err != nil {
		return Result{}, err
	}

	if err = compilePDF(ctx, work); err != nil {
		return Result{}, err
	}

	return packageOutput(cfg, work, charts.zipFiles)
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
	ce = inclusiveLocalDateEnd(ce)

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

// ConvertToTimeSeriesPoints converts DB rollup rows to chart points,
// converting speeds from mps to display units and times to loc.
func ConvertToTimeSeriesPoints(rows []db.RadarObjectsRollupRow, displayUnits string, loc *time.Location) []chart.TimeSeriesPoint {
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

// ConvertHistogramKeys returns a new histogram map with keys converted
// from mps to display units.
func ConvertHistogramKeys(hist map[float64]int64, displayUnits string) map[float64]int64 {
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
