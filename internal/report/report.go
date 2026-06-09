package report

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/units"
)

// ErrInvalidConfig wraps all report generation errors caused by bad caller
// input (unknown group, unparseable timezone or date). Handlers can use
// errors.Is(err, report.ErrInvalidConfig) to map these to HTTP 4xx responses.
var ErrInvalidConfig = errors.New("invalid report config")

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
	tsRows    []db.RadarObjectsRollupRow
	dailyRows []db.RadarObjectsRollupRow
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
	statsSiteID := reportStatsSiteID(source, cfg)

	summaryResult, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), 0, minSpeedMPS,
		source, cfg.ModelVersion,
		histBucketMPS, histMaxMPS,
		statsSiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("compare summary: %w", err)
	}

	tsResult, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), groupSeconds, minSpeedMPS,
		source, cfg.ModelVersion,
		0, 0,
		statsSiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("compare timeseries: %w", err)
	}

	dailyResult, err := database.RadarObjectRollupRange(
		cs.Unix(), ce.Unix(), 86400, minSpeedMPS,
		source, cfg.ModelVersion,
		0, 0,
		statsSiteID, cfg.BoundaryThreshold,
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

// ConvertHistogramKeys returns a new histogram map with keys converted from
// mps to display units.
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
		return "", fmt.Errorf("invalid output path %q: escapes output dir", fileName)
	}
	return outPath, nil
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
