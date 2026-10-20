package replayeval

import (
	"fmt"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// ReplayInputs says which of a replay's optional inputs a tool can supply,
// for RefuseExperimentsWithout. A tool that takes only a capture and a tuning
// file, such as velocity lidar pcap-replay, passes the zero value.
type ReplayInputs struct {
	// ContinuityCoverage: the tool can declare the sensor's coverage
	// (Config.ContinuityCoverage).
	ContinuityCoverage bool
	// ObservationDB: the tool can write an evidence database
	// (Config.ObservationDBPath).
	ObservationDB bool
}

// RefuseExperimentsWithout refuses experiments that need an input the tool
// does not have, naming the tool, so a selection fails at the flag rather
// than run as another arm. The continuity experiments that classify an
// absence need a coverage declaration, which Run would refuse in terms of a
// flag the tool may not have. The solid body, unless near_edge_track runs it
// on the tracked filter, changes no track and writes its rows only to the
// evidence database, so without one it would record the default replay under
// its name. Everything else is left to ValidateExperiments and Run.
func RefuseExperimentsWithout(tool string, experiments []string, have ReplayInputs) error {
	if needsCoverage(experiments) && !have.ContinuityCoverage {
		return fmt.Errorf("replay experiments %s, %s and %s classify an absence against a sensor coverage declaration, which %s does not take; run them with lidar-state-estimation-baseline -continuity-coverage",
			ExperimentCoastSupport, ExperimentClassCoastBounds, ExperimentOcclusionContinuity, tool)
	}
	if hasExperiment(experiments, ExperimentSolidBody) && !hasExperiment(experiments, ExperimentNearEdgeTrack) && !have.ObservationDB {
		return fmt.Errorf("replay experiment %s writes its rows only to an evidence database, which %s does not write, and changes no track, so it would record the default replay; name %s with it, or run it with lidar-state-estimation-baseline -evidence-dir",
			ExperimentSolidBody, tool, ExperimentNearEdgeTrack)
	}
	return nil
}

// ValidateExperiments applies Run's combination rules to a normalised
// selection against the tuning file it will load (empty for the default
// path), without reading a capture: two settings of one option, a qualifier
// without what it qualifies, and an option the tuning file leaves with
// nothing to change are refused here as Run would refuse them, so a tool can
// fail at its flags rather than after hashing a capture. Run still validates
// on its own; this is the same code, called earlier.
func ValidateExperiments(experiments []string, tuningFile string, mode l5tracks.MeasurementSource, coverage *ContinuityCoverage) error {
	if err := ValidateBackgroundExperiments(experiments); err != nil {
		return err
	}
	if _, err := dwellPolicyFor(experiments); err != nil {
		return err
	}
	if tuningFile == "" {
		tuningFile = config.DefaultConfigPath
	}
	tuningCfg, err := config.LoadTuningConfigOrEmbedded(tuningFile, radarassets.TuningDefaults)
	if err != nil {
		return fmt.Errorf("load tuning config %s: %w", tuningFile, err)
	}
	_, err = trackerConfigFor(tuningCfg.L5.CvKfV1, mode, experiments, coverage)
	return err
}
