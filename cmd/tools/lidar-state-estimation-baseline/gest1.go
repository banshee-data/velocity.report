package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// gest1ReportFile is a case's G-EST-1 report (state plan Section 7.3,
// criteria 2 to 5), written beside the first run's recording under
// -gest1-report: one section per residual source the replay recorded, the
// tracked filter always and the shadow solid body's face fixes when the
// solid_body experiment ran with an evidence database.
const gest1ReportFile = "gest1_report.json"

// gest1FramePeriodSecs is the frame period the longest permitted coast is
// counted in when the tuning bounds it by misses rather than seconds: the
// Pandar40P at 10 Hz, which every capture the tool replays runs at.
const gest1FramePeriodSecs = 0.1

// gest1Settings reads, from the tuning the replay runs under, the tracked
// filter's measurement noise and the longest coast a confirmed track may
// make: max_coast_secs_confirmed where set, max_misses_confirmed frames
// otherwise.
func gest1Settings(tuningPath string) (measurementVariance, maxCoastSecs float64, err error) {
	tuning, err := config.LoadTuningConfigOrEmbedded(tuningPath, radarassets.TuningDefaults)
	if err != nil {
		return 0, 0, fmt.Errorf("load tuning config %s: %w", tuningPath, err)
	}
	cfg := l5tracks.TrackerConfigFromTuning(tuning.L5.CvKfV1)
	maxCoastSecs = float64(cfg.MaxCoastSecsConfirmed)
	if !(maxCoastSecs > 0) {
		maxCoastSecs = float64(cfg.MaxMissesConfirmed) * gest1FramePeriodSecs
	}
	if !(maxCoastSecs > 0) {
		return 0, 0, fmt.Errorf("tuning %s bounds no confirmed coast", tuningPath)
	}
	return float64(cfg.MeasurementNoise), maxCoastSecs, nil
}

// gest1Options are the run's G-EST-1 settings, read once from the tuning.
type gest1Options struct {
	measurementVariance, maxCoastSecs float64
}

// gest1Reporter collects one first run's residuals and writes its report.
type gest1Reporter struct {
	collector *l8analytics.GEST1Collector
	cfg       l8analytics.GEST1Config
}

func newGEST1Reporter(measurementVariance, maxCoastSecs float64, coverage *replayeval.ContinuityCoverage) *gest1Reporter {
	cfg := l8analytics.DefaultGEST1Config(maxCoastSecs)
	if coverage != nil {
		// The solid body measures range and aspect from this origin too.
		cfg.SensorX, cfg.SensorY = float64(coverage.SensorXMetres), float64(coverage.SensorYMetres)
	}
	return &gest1Reporter{
		collector: l8analytics.NewGEST1Collector(l8analytics.GEST1CollectorConfig{MeasurementVariance: measurementVariance}),
		cfg:       cfg,
	}
}

func (g *gest1Reporter) observe(f replayeval.TrackerTapFrame) {
	g.collector.ObserveTap(f.At.UnixNano(), f.Scored, f.Clusters, f.Associations, f.Tracks)
}

// write builds the report and writes it to dir, returning it for the
// summary line.
func (g *gest1Reporter) write(dir string) (l8analytics.GEST1Report, error) {
	report, err := l8analytics.BuildGEST1Report(g.collector, g.cfg)
	if err != nil {
		return report, err
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, err
	}
	return report, os.WriteFile(filepath.Join(dir, gest1ReportFile), append(b, '\n'), 0o644)
}

// chainTaps calls each non-nil tap in order.
func chainTaps(taps ...func(replayeval.TrackerTapFrame)) func(replayeval.TrackerTapFrame) {
	var live []func(replayeval.TrackerTapFrame)
	for _, t := range taps {
		if t != nil {
			live = append(live, t)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	}
	return func(f replayeval.TrackerTapFrame) {
		for _, t := range live {
			t(f)
		}
	}
}

// gest1Line is the report's one-line summary per source.
func gest1Line(report l8analytics.GEST1Report) string {
	var parts []string
	for _, s := range report.Sources {
		c2 := s.Criterion2
		part := fmt.Sprintf("%s moving=%d samples=%d r=%.3f [%.3f, %.3f] tracks=%d bootstrap [%.3f, %.3f] run_share=%.3f c4_pass=%t c5_cases=%d",
			s.Source, s.MovingTracks, s.Samples, c2.R.R, c2.R.Lo, c2.R.Hi, c2.Tracks, c2.BootLo, c2.BootHi,
			s.Criterion3.Share, s.Criterion4.Pass, s.Criterion5Cases)
		if c2.Err != "" {
			part += " c2_error=" + c2.Err
		}
		parts = append(parts, part)
	}
	if report.CrossRead != nil {
		c := report.CrossRead
		parts = append(parts, fmt.Sprintf("cross_read samples=%d r=%.3f [%.3f, %.3f] bootstrap [%.3f, %.3f]",
			c.N, c.R.R, c.R.Lo, c.R.Hi, c.BootLo, c.BootHi))
	}
	return strings.Join(parts, "; ")
}
