package report

import (
	"context"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/chart"
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
}

// The legacy Python report used raw transit speeds, while object rollups were
// site-corrected. Keep PDF metrics on that baseline.
func reportStatsSiteID(source string, cfg Config) int {
	if source == "radar_data_transits" {
		return 0
	}
	return cfg.SiteID
}

func loadData(ctx context.Context, database DB, plan runPlan) (loadedData, error) {
	cfg := plan.cfg
	statsSiteID := reportStatsSiteID(cfg.Source, cfg)

	summaryResult, err := database.RadarObjectRollupRange(
		plan.startUnix, plan.endUnix, 0, plan.minSpeedMPS,
		cfg.Source, cfg.ModelVersion,
		plan.histBucketMPS, plan.histMaxMPS,
		statsSiteID, cfg.BoundaryThreshold,
	)
	if err != nil {
		return loadedData{}, fmt.Errorf("summary query: %w", err)
	}

	tsResult, err := database.RadarObjectRollupRange(
		plan.startUnix, plan.endUnix, plan.groupSeconds, plan.minSpeedMPS,
		cfg.Source, cfg.ModelVersion,
		0, 0,
		statsSiteID, cfg.BoundaryThreshold,
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
			statsSiteID, cfg.BoundaryThreshold,
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
	}
	if len(summaryResult.Metrics) > 0 {
		row := summaryResult.Metrics[0]
		data.summaryP50 = units.ConvertSpeed(row.P50Speed, cfg.Units)
		data.summaryP85 = units.ConvertSpeed(row.P85Speed, cfg.Units)
		data.summaryP98 = units.ConvertSpeed(row.P98Speed, cfg.Units)
		data.summaryMax = units.ConvertSpeed(row.MaxSpeed, cfg.Units)
		data.totalCount = int(row.Count)
	}

	return data, nil
}
