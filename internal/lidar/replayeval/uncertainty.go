package replayeval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// uncertaintyReportFile is the report Config.UncertaintyReport writes, and the
// file Config.UncertaintyCalibrationFile reads a fitted table back from.
const uncertaintyReportFile = "uncertainty_calibration.json"

// loadUncertaintyCalibration reads the fitted table a replay was asked to use.
// A table without the adaptive model would be carried, hashed and reported
// while the tracker ignored it, so that combination is refused.
func loadUncertaintyCalibration(path string, experiments []string) (*l5tracks.NoiseCalibration, error) {
	if path == "" {
		return nil, nil
	}
	if !hasExperiment(experiments, ExperimentAdaptiveUncertainty) {
		return nil, fmt.Errorf("UncertaintyCalibrationFile requires the %s experiment; without it the table would be recorded but unused",
			ExperimentAdaptiveUncertainty)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read uncertainty calibration: %w", err)
	}
	cal, err := l8analytics.ParseNoiseCalibration(b)
	if err != nil {
		return nil, fmt.Errorf("uncertainty calibration %s: %w", path, err)
	}
	return cal, nil
}

// calibrationHashSuffix is appended to the parameter hash input. Empty with
// no table, so every other replay's hash stands.
func calibrationHashSuffix(cal *l5tracks.NoiseCalibration) []byte {
	if cal == nil {
		return nil
	}
	return []byte("\nuncertainty_calibration:" + cal.ID)
}

// noiseModelName says which R the window's samples were recorded under.
func noiseModelName(cfg l5tracks.TrackerConfig) string {
	switch {
	case !cfg.AdaptiveMeasurementNoise:
		return "isotropic"
	case cfg.MeasurementNoiseCalibration == nil:
		return "adaptive_prior"
	default:
		return "adaptive_calibrated:" + cfg.MeasurementNoiseCalibration.ID
	}
}

// uncertaintyReportFor builds the report for one replay window. The births
// and confirmations are the continuity window's, which opens at the same
// boundary and counts only tracks born inside it.
func uncertaintyReportFor(window l5tracks.UncertaintyWindow, continuity l5tracks.ContinuityStats,
	cfg l5tracks.TrackerConfig) (l8analytics.UncertaintyReport, error) {
	report, err := l8analytics.BuildUncertaintyReport(l8analytics.UncertaintyInput{
		Samples:        window.Samples,
		SamplesDropped: window.SamplesDropped,
		NoiseModel:     noiseModelName(cfg),
		Options:        l8analytics.DefaultUncertaintyFitOptions(float64(cfg.MeasurementNoise)),
	})
	if err != nil {
		return l8analytics.UncertaintyReport{}, fmt.Errorf("uncertainty report: %w", err)
	}
	report.PreGateBands = roundPreGateBands(window.PreGate)
	counts := &l8analytics.UncertaintyWindowCounts{
		TracksCreated: int(continuity.TracksBorn), TracksConfirmed: int(continuity.TracksConfirmed),
	}
	if continuity.TracksBorn > 0 {
		counts.FragmentationRatio = roundBaselineMetric(1 - float64(continuity.TracksConfirmed)/float64(continuity.TracksBorn))
	}
	report.Window = counts
	return report, nil
}

// roundPreGateBands publishes the band ratios at the baseline's precision, so
// the report compares byte for byte across repeat runs as the baseline does.
func roundPreGateBands(in []l5tracks.PreGateBandSummary) []l5tracks.PreGateBandSummary {
	out := append([]l5tracks.PreGateBandSummary(nil), in...)
	for i := range out {
		b := &out[i]
		b.MeanNIS = roundBaselineMetric(b.MeanNIS)
		b.NISExceedanceRatio = roundBaselineMetric(b.NISExceedanceRatio)
		b.GatedRatio = roundBaselineMetric(b.GatedRatio)
		b.AssignedRatio = roundBaselineMetric(b.AssignedRatio)
	}
	return out
}

// writeUncertaintyReport writes the report through the replay's runtime, so
// a write failure fails the replay like any other output.
func writeUncertaintyReport(runtime replayRuntime, outDir string, report l8analytics.UncertaintyReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal uncertainty report: %w", err)
	}
	if err := runtime.writeFile(filepath.Join(outDir, uncertaintyReportFile), append(b, '\n'), 0644); err != nil {
		return fmt.Errorf("write uncertainty report: %w", err)
	}
	return nil
}
