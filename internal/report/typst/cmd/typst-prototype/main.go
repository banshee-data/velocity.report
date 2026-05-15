// typst-prototype is the Phase 0+1 driver: load the sample fixture, render
// charts as SVGs (histogram, per-period time-series, site map), and run
// it all through the embedded Typst templates.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/chart"
	report "github.com/banshee-data/velocity.report/internal/report/typst"
)

// fixture mirrors the JSON shape of testdata/sample.json.
type fixture struct {
	Histogram histogramSection `json:"histogram"`
	Daily     []dailyRow       `json:"daily"`
	Granular  []granularRow    `json:"granular"`
	Period    struct {
		Units     string `json:"units"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Timezone  string `json:"timezone"`
	} `json:"period"`
	Overall summaryRow      `json:"overall"`
	Compare *compareSection `json:"compare"`
	Site    struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		MapAngle  *float64 `json:"map_angle"`
	} `json:"site"`
}

type histogramSection struct {
	Buckets []histogramBucket `json:"buckets"`
	Units   string            `json:"units"`
}

type compareSection struct {
	StartDate string           `json:"start_date"`
	EndDate   string           `json:"end_date"`
	Overall   summaryRow       `json:"overall"`
	Histogram histogramSection `json:"histogram"`
	Daily     []dailyRow       `json:"daily"`
	Granular  []granularRow    `json:"granular"`
}

type summaryRow struct {
	P98      float64 `json:"p98"`
	MaxSpeed float64 `json:"max_speed"`
}

type histogramBucket struct {
	Label   string  `json:"label"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

type dailyRow struct {
	Date     string  `json:"date"`
	Count    int     `json:"count"`
	P50      float64 `json:"p50"`
	P85      float64 `json:"p85"`
	P98      float64 `json:"p98"`
	MaxSpeed float64 `json:"max_speed"`
}

type granularRow struct {
	Bucket   string  `json:"bucket"`
	Count    int     `json:"count"`
	P50      float64 `json:"p50"`
	P85      float64 `json:"p85"`
	P98      float64 `json:"p98"`
	MaxSpeed float64 `json:"max_speed"`
}

func main() {
	dataPath := flag.String("data", "internal/report/typst/testdata/sample.json", "Path to JSON report data")
	out := flag.String("out", "report.pdf", "Output PDF path")
	fontDir := flag.String("font-dir", "internal/report/chart/assets", "Font directory (passed to typst --font-path)")
	typstPath := flag.String("typst", "", "Path to the typst binary (defaults to PATH lookup)")
	ignoreSystem := flag.Bool("ignore-system-fonts", true, "Ignore host system fonts for reproducible builds")
	flag.Parse()

	body, err := os.ReadFile(*dataPath)
	if err != nil {
		die("read data: %v", err)
	}
	var typed fixture
	if err := json.Unmarshal(body, &typed); err != nil {
		die("decode typed data: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		die("decode data: %v", err)
	}

	assets, err := buildCharts(typed)
	if err != nil {
		die("build charts: %v", err)
	}
	if charts, ok := data["charts"].(map[string]any); ok {
		for _, a := range assets {
			switch a.Name {
			case "charts/histogram.svg":
				charts["histogram"] = "/" + a.Name
			case "charts/timeseries.svg":
				charts["timeseries"] = "/" + a.Name
			case "charts/timeseries_compare.svg":
				charts["timeseries_compare"] = "/" + a.Name
			case "charts/map.svg":
				charts["map"] = "/" + a.Name
			}
		}
	}

	f, err := os.Create(*out)
	if err != nil {
		die("create output: %v", err)
	}
	defer f.Close()

	if err := report.Render(f, report.Options{
		Data:              data,
		Assets:            assets,
		FontDir:           *fontDir,
		IgnoreSystemFonts: *ignoreSystem,
		TypstPath:         *typstPath,
	}); err != nil {
		die("render: %v", err)
	}
	fmt.Printf("wrote %s\n", *out)
}

func buildCharts(f fixture) ([]report.Asset, error) {
	var assets []report.Asset

	// ── Histogram (single or comparison) ────────────────────────────────
	if len(f.Histogram.Buckets) > 0 {
		units := f.Histogram.Units
		if units == "" {
			units = f.Period.Units
		}
		histData, err := toHistogramData(f.Histogram.Buckets, units)
		if err != nil {
			return nil, fmt.Errorf("histogram data: %w", err)
		}
		var svg []byte
		if f.Compare != nil && len(f.Compare.Histogram.Buckets) > 0 {
			compareHist, err := toHistogramData(f.Compare.Histogram.Buckets, units)
			if err != nil {
				return nil, fmt.Errorf("compare histogram data: %w", err)
			}
			t1Range := fmt.Sprintf("%s to %s", f.Period.StartDate, f.Period.EndDate)
			t2Range := fmt.Sprintf("%s to %s", f.Compare.StartDate, f.Compare.EndDate)
			svg, err = chart.RenderComparison(
				histData,
				compareHist,
				"t1: "+t1Range,
				"t2: "+t2Range,
				chart.DefaultHistogramStyle(chart.PaperA4),
			)
		} else {
			svg, err = chart.RenderHistogram(histData, chart.DefaultHistogramStyle(chart.PaperA4))
		}
		if err != nil {
			return nil, fmt.Errorf("histogram: %w", err)
		}
		assets = append(assets, report.Asset{Name: "charts/histogram.svg", Data: svg})
	}

	// ── Time-series for the primary period ──────────────────────────────
	if len(f.Granular) > 0 {
		data, err := toTimeSeriesData(f.Granular, f.Period.StartDate, f.Period.Timezone, f.Period.Units, f.Overall)
		if err != nil {
			return nil, fmt.Errorf("timeseries data: %w", err)
		}
		svg, err := chart.RenderTimeSeries(data, chart.DefaultTimeSeriesStyle(chart.PaperA4))
		if err != nil {
			return nil, fmt.Errorf("timeseries: %w", err)
		}
		assets = append(assets, report.Asset{Name: "charts/timeseries.svg", Data: svg})
	}

	// ── Time-series for the comparison period ───────────────────────────
	if f.Compare != nil && len(f.Compare.Granular) > 0 {
		data, err := toTimeSeriesData(f.Compare.Granular, f.Compare.StartDate, f.Period.Timezone, f.Period.Units, f.Compare.Overall)
		if err != nil {
			return nil, fmt.Errorf("timeseries (compare) data: %w", err)
		}
		svg, err := chart.RenderTimeSeries(data, chart.DefaultTimeSeriesStyle(chart.PaperA4))
		if err != nil {
			return nil, fmt.Errorf("timeseries (compare): %w", err)
		}
		assets = append(assets, report.Asset{Name: "charts/timeseries_compare.svg", Data: svg})
	}

	// ── Site map ────────────────────────────────────────────────────────
	if f.Site.Latitude != nil && f.Site.Longitude != nil {
		angle := 0.0
		if f.Site.MapAngle != nil {
			angle = *f.Site.MapAngle
		}
		svg, err := chart.RenderSiteMap(chart.SiteMapOptions{
			Latitude:        *f.Site.Latitude,
			Longitude:       *f.Site.Longitude,
			MapAngleDegrees: angle,
			OSMTiles:        true,
		})
		if err != nil {
			return nil, fmt.Errorf("sitemap: %w", err)
		}
		assets = append(assets, report.Asset{Name: "charts/map.svg", Data: svg})
	}

	return assets, nil
}

func toHistogramData(buckets []histogramBucket, units string) (chart.HistogramData, error) {
	data := chart.HistogramData{
		Buckets: make(map[float64]int64, len(buckets)),
		Units:   units,
	}
	for _, bucket := range buckets {
		lo, hi, maxBucket, err := parseHistogramLabel(bucket.Label)
		if err != nil {
			return chart.HistogramData{}, err
		}
		data.Buckets[lo] = int64(bucket.Count)
		if hi > lo && data.BucketSz == 0 {
			data.BucketSz = hi - lo
		}
		if maxBucket > 0 {
			data.MaxBucket = maxBucket
		}
	}
	if data.BucketSz == 0 {
		data.BucketSz = 5
	}
	return data, nil
}

func parseHistogramLabel(label string) (lo, hi, maxBucket float64, err error) {
	if trimmed, ok := strings.CutSuffix(strings.TrimSpace(label), "+"); ok {
		lo, err = strconv.ParseFloat(strings.TrimSpace(trimmed), 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parse open bucket %q: %w", label, err)
		}
		return lo, lo, lo, nil
	}
	loText, hiText, ok := strings.Cut(label, "-")
	if !ok {
		return 0, 0, 0, fmt.Errorf("unsupported histogram bucket %q", label)
	}
	lo, err = strconv.ParseFloat(strings.TrimSpace(loText), 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse histogram lower bound %q: %w", label, err)
	}
	hi, err = strconv.ParseFloat(strings.TrimSpace(hiText), 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse histogram upper bound %q: %w", label, err)
	}
	return lo, hi, 0, nil
}

func toTimeSeriesData(rows []granularRow, startDate, timezone, units string, summary summaryRow) (chart.TimeSeriesData, error) {
	loc := time.Local
	if timezone != "" {
		loaded, err := time.LoadLocation(timezone)
		if err != nil {
			return chart.TimeSeriesData{}, fmt.Errorf("load timezone %q: %w", timezone, err)
		}
		loc = loaded
	}
	anchor, err := time.ParseInLocation("2006-01-02", startDate, loc)
	if err != nil {
		return chart.TimeSeriesData{}, fmt.Errorf("parse start date %q: %w", startDate, err)
	}
	points := make([]chart.TimeSeriesPoint, len(rows))
	for i, row := range rows {
		startTime, err := parseBucketTime(row.Bucket, anchor.Year(), loc)
		if err != nil {
			return chart.TimeSeriesData{}, err
		}
		points[i] = chart.TimeSeriesPoint{
			StartTime: startTime,
			P50Speed:  row.P50,
			P85Speed:  row.P85,
			P98Speed:  row.P98,
			MaxSpeed:  row.MaxSpeed,
			Count:     row.Count,
		}
	}
	data := chart.TimeSeriesData{Points: points, Units: units, P98Reference: math.NaN(), MaxReference: math.NaN()}
	if summary.P98 > 0 {
		data.P98Reference = summary.P98
	}
	if summary.MaxSpeed > 0 {
		data.MaxReference = summary.MaxSpeed
	}
	return data, nil
}

func parseBucketTime(bucket string, year int, loc *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("1/2 15:04", strings.TrimSpace(bucket), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time bucket %q: %w", bucket, err)
	}
	return time.Date(year, parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "typst-prototype: "+format+"\n", args...)
	os.Exit(1)
}
