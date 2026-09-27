package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

func uncertaintyCaseFor(t *testing.T, id string, seed int64, n int, model string) uncertaintyCase {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	samples := make([]l5tracks.UncertaintySample, n)
	for i := range samples {
		yr, yt := rng.NormFloat64()*0.25, rng.NormFloat64()*0.3
		samples[i] = l5tracks.UncertaintySample{
			Source: l5tracks.MeasurementMedoidV0, Rank: 2, RangeMetres: float32(8 + i%40), Support: 30 + i%100,
			AspectRad: 1.5, InnovRadial: float32(yr), InnovTangential: float32(yt),
			PredRadial: 0.02, PredTangential: 0.02, NoiseRadial: 0.05, NoiseTangential: 0.05,
			PhysRadial: 0.0004, PhysTangential: 0.001, NIS: float32((yr*yr + yt*yt) / 0.07), Assigned: true,
		}
	}
	report, err := l8analytics.BuildUncertaintyReport(l8analytics.UncertaintyInput{
		Samples: samples, NoiseModel: model, SamplesDropped: 1, Options: l8analytics.DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	return uncertaintyCase{ID: id, Report: &report, Samples: samples}
}

func TestValidateUncertaintyFlags(t *testing.T) {
	if err := validateUncertaintyFlags("", nil); err != nil {
		t.Errorf("no table: %v", err)
	}
	if err := validateUncertaintyFlags("cal.json", []string{replayeval.ExperimentCascade}); err == nil ||
		!strings.Contains(err.Error(), replayeval.ExperimentAdaptiveUncertainty) {
		t.Errorf("table without the experiment: %v", err)
	}
	if err := validateUncertaintyFlags("cal.json", []string{replayeval.ExperimentAdaptiveUncertainty}); err != nil {
		t.Errorf("table with the experiment: %v", err)
	}
}

// Pooling is a fit over the union of the cases' samples, in any case order,
// and the result loads as a calibration table.
func TestPooledUncertaintyReport(t *testing.T) {
	a := uncertaintyCaseFor(t, "a", 1, 300, "isotropic")
	b := uncertaintyCaseFor(t, "b", 2, 200, "isotropic")
	pooled, err := pooledUncertaintyReport([]uncertaintyCase{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if pooled.Samples != 500 || pooled.SamplesDropped != 2 || pooled.NoiseModel != "isotropic" {
		t.Errorf("pooled header: samples %d dropped %d model %q", pooled.Samples, pooled.SamplesDropped, pooled.NoiseModel)
	}
	reversed, err := pooledUncertaintyReport([]uncertaintyCase{b, a})
	if err != nil {
		t.Fatal(err)
	}
	if reversed.Fit.Calibration.ID != pooled.Fit.Calibration.ID {
		t.Error("the pooled table depends on case order")
	}

	dir := t.TempDir()
	if err := writePooledUncertaintyReport(dir, []uncertaintyCase{a, b}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, pooledUncertaintyFile))
	if err != nil {
		t.Fatal(err)
	}
	cal, err := l8analytics.ParseNoiseCalibration(raw)
	if err != nil || cal.ID != pooled.Fit.Calibration.ID {
		t.Errorf("pooled file does not load as its table: %v", err)
	}
}

func TestPooledUncertaintyReportRefusesMixedModels(t *testing.T) {
	a := uncertaintyCaseFor(t, "a", 1, 100, "isotropic")
	b := uncertaintyCaseFor(t, "b", 2, 100, "adaptive_prior")
	if _, err := pooledUncertaintyReport([]uncertaintyCase{a, b}); err == nil {
		t.Error("cases under different noise models were pooled")
	}
	if _, err := pooledUncertaintyReport(nil); err == nil {
		t.Error("an empty pool was accepted")
	}
	if _, err := pooledUncertaintyReport([]uncertaintyCase{{ID: "x"}}); err == nil {
		t.Error("a case without a report was accepted")
	}
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b, c := write("a", "x"), write("b", "x"), write("c", "y")
	if err := sameFile(a, b); err != nil {
		t.Errorf("identical files: %v", err)
	}
	if err := sameFile(a, c); err == nil {
		t.Error("different files reported identical")
	}
	if err := sameFile(a, filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file reported identical")
	}
}
