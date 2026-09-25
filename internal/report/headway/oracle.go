package headway

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// OracleSource locates the oracle's captures: the frozen l8behaviour
// encounter scenarios, one capture per scenario.
const OracleSource = "l8behaviour.EncounterScenarios"

// OracleInput runs every frozen analytic encounter scenario through
// AnalyseFollowing with its own parameters, one capture per scenario, for a
// synthetic-oracle report (Section 10.4, stage 1). The scenarios' bumpers,
// gaps, time gaps, band durations and suppressions are known by
// construction, so every number the report prints can be checked by hand.
func OracleInput() (Input, error) {
	in := Input{Status: StatusSyntheticOracle}
	for _, sc := range l8behaviour.EncounterScenarios() {
		a, err := l8behaviour.AnalyseFollowing(sc.Trajectories, sc.Params)
		if err != nil {
			return Input{}, fmt.Errorf("scenario %s: %w", sc.Name, err)
		}
		in.Captures = append(in.Captures, CaptureInput{
			ID: sc.Name, Description: sc.Description, Source: OracleSource + "/" + sc.Name,
			Trajectories: sc.Trajectories, Params: sc.Params, Analysis: a,
		})
	}
	return in, nil
}

// Oracle builds the synthetic-oracle report.
func Oracle() (Report, error) {
	in, err := OracleInput()
	if err != nil {
		return Report{}, err
	}
	return Build(in)
}
