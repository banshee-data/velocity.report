//go:build pcap && !race

package replayeval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

type uncertaintyArm struct {
	baseline    []byte
	fingerprint string
	params      string
	manifest    map[string]json.RawMessage
	report      *l8analytics.UncertaintyReport
	result      *Result
	dir         string
}

func runUncertaintyArm(t *testing.T, cfg Config) uncertaintyArm {
	t.Helper()
	res, err := Run(cfg)
	if err != nil {
		t.Fatalf("%s: %v", filepath.Base(cfg.OutDir), err)
	}
	arm := uncertaintyArm{baseline: readBaseline(t, cfg.OutDir), result: res, dir: cfg.OutDir}
	frames, _ := trackFingerprint(t, cfg.OutDir)
	arm.fingerprint = strings.Join(frames, "\n")
	b, err := os.ReadFile(filepath.Join(cfg.OutDir, "replay_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &arm.manifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(arm.manifest["params_sha256"], &arm.params); err != nil {
		t.Fatal(err)
	}
	if cfg.UncertaintyReport {
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, uncertaintyReportFile))
		if err != nil {
			t.Fatal(err)
		}
		var report l8analytics.UncertaintyReport
		if err := json.Unmarshal(b, &report); err != nil {
			t.Fatal(err)
		}
		arm.report = &report
	}
	return arm
}

// The G-UNC-1 harness on kirk0, end to end: the report, the experiment and a
// fitted table read back into a second replay. It asserts the wiring and
// the one invariant that matters most, that asking for the report changes no
// estimate; it logs the label-free figures and asserts no direction, because
// one short window of one capture is not evidence for the gate.
func TestUncertaintyHarnessOnKirk0(t *testing.T) {
	dir := t.TempDir()
	arm := func(name string, mutate func(*Config)) uncertaintyArm {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, name))
		mutate(&cfg)
		return runUncertaintyArm(t, cfg)
	}
	plain := arm("plain", func(*Config) {})
	reported := arm("reported", func(c *Config) { c.UncertaintyReport = true })

	if !bytes.Equal(plain.baseline, reported.baseline) || plain.fingerprint != reported.fingerprint || plain.params != reported.params {
		t.Fatal("asking for the uncertainty report changed the replay")
	}
	if _, ok := plain.manifest["uncertainty_report"]; ok {
		t.Error("a replay without the report names one in its manifest")
	}
	if _, ok := reported.manifest["uncertainty_report"]; !ok {
		t.Error("the manifest does not name the report")
	}
	r := reported.report
	if r.SchemaVersion != l8analytics.UncertaintyReportSchemaVersion || r.NoiseModel != "isotropic" || r.Samples == 0 ||
		!strings.HasPrefix(r.GateVerdict, "not_assessed") || r.Window == nil || len(r.PreGateBands) == 0 {
		t.Fatalf("report header: schema %d model %q samples %d verdict %q window %+v bands %d",
			r.SchemaVersion, r.NoiseModel, r.Samples, r.GateVerdict, r.Window, len(r.PreGateBands))
	}
	if len(reported.result.UncertaintySamples) != r.Samples+r.SamplesInvalid {
		t.Errorf("result carries %d samples for a report of %d", len(reported.result.UncertaintySamples), r.Samples)
	}

	adaptive := arm("adaptive", func(c *Config) {
		c.UncertaintyReport = true
		c.Experiments = []string{ExperimentAdaptiveUncertainty}
	})
	calibrated := arm("calibrated", func(c *Config) {
		c.UncertaintyReport = true
		c.Experiments = []string{ExperimentAdaptiveUncertainty}
		c.UncertaintyCalibrationFile = filepath.Join(reported.dir, uncertaintyReportFile)
	})
	if adaptive.params == reported.params || calibrated.params == adaptive.params {
		t.Error("the adaptive arms are not identifiable by parameter hash")
	}
	if bytes.Equal(adaptive.baseline, reported.baseline) && adaptive.fingerprint == reported.fingerprint {
		t.Error("adaptive_uncertainty left the replay unchanged; the option did not reach the tracker")
	}
	if adaptive.report.NoiseModel != "adaptive_prior" {
		t.Errorf("adaptive arm model %q", adaptive.report.NoiseModel)
	}
	var calID string
	if err := json.Unmarshal(calibrated.manifest["uncertainty_calibration_id"], &calID); err != nil ||
		calID != reported.report.Fit.Calibration.ID || calibrated.report.NoiseModel != "adaptive_calibrated:"+calID {
		t.Errorf("calibrated arm: manifest id %q (%v), fitted %q, model %q", calID, err,
			reported.report.Fit.Calibration.ID, calibrated.report.NoiseModel)
	}

	for _, a := range []struct {
		name string
		arm  uncertaintyArm
	}{{"shipped", reported}, {"adaptive_prior", adaptive}, {"calibrated", calibrated}} {
		rep := a.arm.report
		for _, c := range rep.PreGate {
			t.Logf("%s pre-gate %s m=%d: n=%d mean NIS/m=%.3f coverage95=%.3f", a.name, c.Dimension, c.DOF, c.Count, c.MeanNISOverDOF, c.Coverage95)
		}
		for _, c := range rep.GateChecks {
			t.Logf("%s check %s: %s %s", a.name, c.ID, c.Status, c.Detail)
		}
		t.Logf("%s window %+v", a.name, *rep.Window)
	}
}
