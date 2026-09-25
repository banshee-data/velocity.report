package replayeval

import (
	"fmt"
	"sort"
	"strings"
)

// Experiments are the default-off, Go-level options a replay can switch on.
//
// They are deliberately not tuning keys: TuningConfig.Fingerprint hashes the
// whole resolved config, so a new key would move the fingerprint and make the
// committed perf baselines refuse to compare before anyone had measured
// whether the option helps. The offline harness is where that measurement
// happens, so this is the one place the options are reachable by name.
//
// A replay with experiments is a different estimator from one without, so the
// sorted list is folded into the parameter hash and written to the run
// metadata. An empty list leaves both exactly as they were.
const (
	// ExperimentLikelihoodCost: l5tracks.TrackerConfig.LikelihoodAssociationCost (gap analysis S3).
	ExperimentLikelihoodCost = "likelihood_cost"
	// ExperimentCascade: l5tracks.TrackerConfig.CascadedAssociation (S2).
	ExperimentCascade = "cascade"
	// ExperimentDensityCap: pipeline DensityPreservingCap (D6).
	ExperimentDensityCap = "density_cap"
	// ExperimentFlipRule: l5tracks.TrackerConfig.OBBHeadingFlipRule (P3).
	ExperimentFlipRule = "flip_rule"
	// ExperimentNoRegionOverrides: l3grid.BackgroundParams.DisableRegionOverrides (B8).
	ExperimentNoRegionOverrides = "no_region_overrides"
	// ExperimentMeasurementTime: l5tracks.TrackerConfig.MeasurementTimePrediction,
	// state-estimation plan question Q3: update each track at its cluster's
	// own acquisition time rather than the frame start.
	ExperimentMeasurementTime = "measurement_time"
	// ExperimentCaptureGapPredict: l5tracks.TrackerConfig.CaptureGapPrediction,
	// predict across a whole capture-time gap instead of clamping it to
	// max_predict_dt.
	ExperimentCaptureGapPredict = "capture_gap_predict"
	// ExperimentFixedLagRTS attaches the fixed-assignment RTS smoother at the
	// comparison horizons (three frames; 0.5, 1 and 2 s; the whole track) and
	// writes refinement_report.json; see refinement.go. It observes the
	// tracker and changes none of its decisions, but it is hashed like every
	// experiment, so the online rows of such a run carry their own hash.
	ExperimentFixedLagRTS = "fixed_lag_rts"
)

var knownExperiments = map[string]bool{
	ExperimentLikelihoodCost:    true,
	ExperimentCascade:           true,
	ExperimentDensityCap:        true,
	ExperimentFlipRule:          true,
	ExperimentNoRegionOverrides: true,
	ExperimentMeasurementTime:   true,
	ExperimentCaptureGapPredict: true,
	ExperimentFixedLagRTS:       true,
}

// KnownExperiments returns every accepted experiment name, sorted.
func KnownExperiments() []string {
	names := make([]string, 0, len(knownExperiments))
	for name := range knownExperiments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NormaliseExperiments validates names against the closed set and returns
// them sorted and de-duplicated, so the same selection always produces the
// same parameter hash whatever order it was written in. Blank entries are
// dropped; an unknown name is an error, never ignored, because a misspelt
// option would otherwise run as the baseline and be reported as the option.
func NormaliseExperiments(names []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !knownExperiments[name] {
			return nil, fmt.Errorf("unknown experiment %q (known: %s)", name, strings.Join(KnownExperiments(), ", "))
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ParseExperiments splits a comma-separated flag value and normalises it.
func ParseExperiments(flagValue string) ([]string, error) {
	if strings.TrimSpace(flagValue) == "" {
		return []string{}, nil
	}
	return NormaliseExperiments(strings.Split(flagValue, ","))
}

func hasExperiment(experiments []string, name string) bool {
	for _, e := range experiments {
		if e == name {
			return true
		}
	}
	return false
}

// experimentsHashSuffix is appended to the marshalled tuning config before it
// is hashed. Empty for no experiments, so existing parameter hashes stand.
func experimentsHashSuffix(experiments []string) []byte {
	if len(experiments) == 0 {
		return nil
	}
	return []byte("\nexperiments:" + strings.Join(experiments, ","))
}
