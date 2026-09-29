package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// pooledUncertaintyFile is the corpus-level report -uncertainty-report writes
// beside phase0-summary.json. Its fit.calibration is a table
// -uncertainty-calibration can load.
const pooledUncertaintyFile = "uncertainty-calibration.json"

// uncertaintyCase is one case's first-run report and the samples behind it.
type uncertaintyCase struct {
	ID      string
	Report  *l8analytics.UncertaintyReport
	Samples []l5tracks.UncertaintySample
}

// validateUncertaintyFlags refuses a calibration table the tracker would not
// use, before any case is replayed rather than after the first one.
func validateUncertaintyFlags(calibrationFile string, experiments []string) error {
	if calibrationFile == "" {
		return nil
	}
	for _, e := range experiments {
		if e == replayeval.ExperimentAdaptiveUncertainty {
			return nil
		}
	}
	return fmt.Errorf("-uncertainty-calibration requires -experiment %s", replayeval.ExperimentAdaptiveUncertainty)
}

// pooledUncertaintyReport fits one table over every case's samples. Cases
// recorded under different noise models or base variances are refused: their
// samples answer different questions and pooling them fits neither.
func pooledUncertaintyReport(cases []uncertaintyCase) (l8analytics.UncertaintyReport, error) {
	if len(cases) == 0 {
		return l8analytics.UncertaintyReport{}, fmt.Errorf("no case produced an uncertainty report")
	}
	first := cases[0].Report
	var samples []l5tracks.UncertaintySample
	dropped := 0
	for _, c := range cases {
		if c.Report == nil {
			return l8analytics.UncertaintyReport{}, fmt.Errorf("case %s has no uncertainty report", c.ID)
		}
		if c.Report.NoiseModel != first.NoiseModel || c.Report.Fit.Options != first.Fit.Options {
			return l8analytics.UncertaintyReport{}, fmt.Errorf("case %s ran noise model %q with %+v, case %s %q with %+v; refusing to pool",
				c.ID, c.Report.NoiseModel, c.Report.Fit.Options, cases[0].ID, first.NoiseModel, first.Fit.Options)
		}
		samples = append(samples, c.Samples...)
		dropped += c.Report.SamplesDropped
	}
	return l8analytics.BuildUncertaintyReport(l8analytics.UncertaintyInput{
		Samples: samples, SamplesDropped: dropped, NoiseModel: first.NoiseModel, Options: first.Fit.Options,
	})
}

// writePooledUncertaintyReport writes the pooled report, naming the frozen
// split the run was made under when there was one: a table fitted on a
// held-out score's cases says so wherever it is loaded from.
func writePooledUncertaintyReport(outDir string, cases []uncertaintyCase, split *splitRecord) error {
	report, err := pooledUncertaintyReport(cases)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(struct {
		l8analytics.UncertaintyReport
		Split *splitRecord `json:"split,omitempty"`
	}{report, split}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pooled uncertainty report: %w", err)
	}
	return os.WriteFile(filepath.Join(outDir, pooledUncertaintyFile), append(b, '\n'), 0644)
}
