// render-sample-tex renders the Go report .tex template with sample data.
//
// Usage:
//
//	go run ./cmd/tools/render-sample-tex -out /tmp/go_sample_report.tex
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/tex"
)

func main() {
	out := flag.String("out", "/tmp/go_sample_report.tex", "output .tex path")
	flag.Parse()

	loc, _ := time.LoadLocation("US/Pacific")

	buckets := map[float64]int64{
		5: 66, 10: 239, 15: 294, 20: 338, 25: 720,
		30: 971, 35: 631, 40: 183, 45: 24, 50: 3,
	}
	histTex := tex.BuildHistogramTableTeX(buckets, 5.0, 5.0, 50.0, "mph")
	fontDir, err := reportFontDir()
	if err != nil {
		log.Fatalf("resolve font dir: %v", err)
	}

	mk := func(y, m, d, H int, count int, p50, p85, p98, max float64) chart.TimeSeriesPoint {
		return chart.TimeSeriesPoint{
			StartTime: time.Date(y, time.Month(m), d, H, 0, 0, 0, loc),
			Count:     count,
			P50Speed:  p50, P85Speed: p85, P98Speed: p98, MaxSpeed: max,
		}
	}
	pts := []chart.TimeSeriesPoint{
		mk(2025, 6, 2, 8, 109, 23.43, 35.71, 43.78, 46.47),
		mk(2025, 6, 2, 9, 152, 30.54, 37.52, 42.47, 46.83),
		mk(2025, 6, 2, 10, 162, 34.32, 40.14, 45.96, 51.19),
		mk(2025, 6, 2, 11, 141, 32.87, 37.81, 44.21, 46.25),
		mk(2025, 6, 2, 12, 180, 30.83, 36.07, 43.34, 44.21),
		mk(2025, 6, 2, 13, 147, 25.01, 32.87, 40.72, 48.28),
		mk(2025, 6, 3, 9, 74, 27.92, 34.90, 40.72, 40.72),
		mk(2025, 6, 3, 10, 190, 28.80, 35.19, 43.63, 48.87),
		mk(2025, 6, 3, 11, 183, 28.80, 35.49, 41.30, 53.52),
		mk(2025, 6, 3, 12, 188, 32.58, 36.94, 41.59, 43.92),
		mk(2025, 6, 3, 13, 145, 32.00, 38.69, 42.47, 44.21),
		mk(2025, 6, 3, 14, 187, 30.83, 36.65, 43.34, 47.12),
		mk(2025, 6, 3, 15, 215, 29.38, 34.90, 39.85, 45.67),
		mk(2025, 6, 3, 16, 133, 31.41, 37.52, 41.59, 45.38),
		mk(2025, 6, 4, 9, 255, 21.82, 31.70, 36.36, 40.72),
		mk(2025, 6, 4, 10, 140, 31.12, 37.23, 42.76, 46.54),
		mk(2025, 6, 4, 11, 150, 33.74, 38.69, 43.63, 47.41),
		mk(2025, 6, 4, 12, 141, 33.16, 39.56, 45.67, 47.12),
		mk(2025, 6, 4, 13, 170, 32.29, 38.10, 42.76, 45.67),
		mk(2025, 6, 4, 14, 198, 28.21, 34.61, 41.30, 42.18),
		mk(2025, 6, 4, 15, 208, 32.58, 38.69, 43.63, 53.52),
		mk(2025, 6, 4, 16, 1, 36.36, 36.36, 36.36, 36.36),
	}

	data := tex.TemplateData{
		Location:          "Test Street, Test City",
		Surveyor:          "Test Surveyor",
		Contact:           "test@example.com",
		SpeedLimit:        25,
		Description:       "Sample survey to demonstrate the Go-side PDF pipeline output.",
		StartDate:         "2025-06-02",
		EndDate:           "2025-06-04",
		Timezone:          "US/Pacific",
		Units:             "mph",
		P50:               "30.54",
		P85:               "36.94",
		P98:               "43.05",
		MaxSpeed:          "53.52",
		TotalCount:        3469,
		HoursCount:        22,
		TimeSeriesChart:   "timeseries.pdf",
		HistogramChart:    "histogram.pdf",
		FontDir:           fontDir,
		HistogramTableTeX: histTex,
		StatRows:          tex.BuildStatRows(pts, loc),
		Source:            "radar_data_transits",
		Group:             "1h",
		MinSpeed:          5.0,
		CosineAngle:       30.0,
		CosineFactor:      1.1547,
		ModelVersion:      "hourly-cron",
		PaperOption:       "a4paper",
	}

	out1, err := tex.RenderTeX(data)
	if err != nil {
		log.Fatalf("render: %v", err)
	}
	if err := os.WriteFile(*out, out1, 0o644); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(out1), *out)
}

func reportFontDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to resolve current source path")
	}
	return filepath.Abs(filepath.Join(
		filepath.Dir(file),
		"..",
		"..",
		"..",
		"internal",
		"report",
		"chart",
		"assets",
	))
}
