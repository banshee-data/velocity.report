package replayeval

import (
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

func syntheticUncertaintySamples(n int) []l5tracks.UncertaintySample {
	rng := rand.New(rand.NewSource(1))
	out := make([]l5tracks.UncertaintySample, n)
	for i := range out {
		yr, yt := rng.NormFloat64()*0.2, rng.NormFloat64()*0.3
		out[i] = l5tracks.UncertaintySample{
			Source: l5tracks.MeasurementMedoidV0, Rank: 2, RangeMetres: float32(5 + i%50), Support: 20 + i%200,
			AspectRad: float32(i%8) * 0.7, InnovRadial: float32(yr), InnovTangential: float32(yt),
			PredRadial: 0.02, PredTangential: 0.02, NoiseRadial: 0.05, NoiseTangential: 0.05,
			PhysRadial: 0.0004, PhysTangential: 0.001, NIS: float32((yr*yr + yt*yt) / 0.07), Assigned: true,
		}
	}
	return out
}

func writeSyntheticReport(t *testing.T) (string, l8analytics.UncertaintyReport) {
	t.Helper()
	report, err := l8analytics.BuildUncertaintyReport(l8analytics.UncertaintyInput{
		Samples: syntheticUncertaintySamples(600), Options: l8analytics.DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), uncertaintyReportFile)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, report
}

func TestLoadUncertaintyCalibration(t *testing.T) {
	if cal, err := loadUncertaintyCalibration("", nil); cal != nil || err != nil {
		t.Fatalf("no file: %v, %v", cal, err)
	}
	path, report := writeSyntheticReport(t)
	// A table the tracker would ignore must not be carried and reported.
	if _, err := loadUncertaintyCalibration(path, []string{ExperimentCascade}); err == nil ||
		!strings.Contains(err.Error(), ExperimentAdaptiveUncertainty) {
		t.Errorf("table without the experiment: %v", err)
	}
	cal, err := loadUncertaintyCalibration(path, []string{ExperimentAdaptiveUncertainty})
	if err != nil {
		t.Fatal(err)
	}
	if cal.ID != report.Fit.Calibration.ID {
		t.Errorf("loaded id %q, report %q", cal.ID, report.Fit.Calibration.ID)
	}
	if _, err := loadUncertaintyCalibration(filepath.Join(t.TempDir(), "absent.json"), []string{ExperimentAdaptiveUncertainty}); err == nil {
		t.Error("missing file accepted")
	}
	corrupt := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadUncertaintyCalibration(corrupt, []string{ExperimentAdaptiveUncertainty}); err == nil {
		t.Error("corrupt file accepted")
	}
}

// Without a table the parameter hash input is untouched; with one it names
// the table by content.
func TestCalibrationHashSuffix(t *testing.T) {
	if s := calibrationHashSuffix(nil); s != nil {
		t.Errorf("no table produced suffix %q", s)
	}
	cal := l5tracks.UniformNoiseCalibration(0.05)
	cal.ID = "sha256:abc"
	if s := string(calibrationHashSuffix(cal)); !strings.Contains(s, "sha256:abc") {
		t.Errorf("suffix %q does not name the table", s)
	}
}

func TestNoiseModelName(t *testing.T) {
	cfg := l5tracks.DefaultTrackerConfig()
	if got := noiseModelName(cfg); got != "isotropic" {
		t.Errorf("shipped: %q", got)
	}
	cfg.AdaptiveMeasurementNoise = true
	if got := noiseModelName(cfg); got != "adaptive_prior" {
		t.Errorf("uncalibrated: %q", got)
	}
	cfg.MeasurementNoiseCalibration = &l5tracks.NoiseCalibration{ID: "sha256:x"}
	if got := noiseModelName(cfg); got != "adaptive_calibrated:sha256:x" {
		t.Errorf("calibrated: %q", got)
	}
}

func TestUncertaintyReportForAddsTheWindow(t *testing.T) {
	window := l5tracks.UncertaintyWindow{
		Samples:        syntheticUncertaintySamples(200),
		SamplesDropped: 3,
		PreGate:        []l5tracks.PreGateBandSummary{{Candidates: 3, Eligible: 3, MeanNIS: 2.123456789, GatedRatio: 1.0 / 3}},
	}
	continuity := l5tracks.ContinuityStats{TracksBorn: 8, TracksConfirmed: 6}
	report, err := uncertaintyReportFor(window, continuity, l5tracks.DefaultTrackerConfig())
	if err != nil {
		t.Fatal(err)
	}
	if report.NoiseModel != "isotropic" || report.SamplesDropped != 3 || report.Samples != 200 {
		t.Errorf("report header %+v", report)
	}
	if report.Window == nil || report.Window.TracksCreated != 8 || report.Window.FragmentationRatio != 0.25 {
		t.Errorf("window %+v", report.Window)
	}
	if b := report.PreGateBands[0]; b.MeanNIS != 2.123457 || b.GatedRatio != 0.333333 {
		t.Errorf("bands not rounded to the baseline's precision: %+v", b)
	}
	if report.Fit.Options.BaseVarianceM2 != float64(l5tracks.DefaultTrackerConfig().MeasurementNoise) {
		t.Errorf("prior is not the tuning file's measurement noise: %v", report.Fit.Options.BaseVarianceM2)
	}
}

func TestWriteUncertaintyReportFailureIsReported(t *testing.T) {
	runtime := defaultRuntime()
	runtime.writeFile = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	if err := writeUncertaintyReport(runtime, t.TempDir(), l8analytics.UncertaintyReport{}); err == nil {
		t.Error("a failed write was not reported")
	}
	dir := t.TempDir()
	if err := writeUncertaintyReport(defaultRuntime(), dir, l8analytics.UncertaintyReport{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, uncertaintyReportFile)); err != nil {
		t.Errorf("report not written: %v", err)
	}
}
