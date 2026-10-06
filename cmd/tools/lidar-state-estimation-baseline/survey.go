package main

import (
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// surveyTool names this command in a surveyed declaration's source.
const surveyTool = "lidar-state-estimation-baseline -survey-coverage"

// surveyFlags are -survey-coverage and its two settings.
type surveyFlags struct {
	setPath         string
	percentile, gap float64
}

// validate refuses a coverage survey of anything but the default
// replay with an evidence output, and a survey into a set that already
// declares one of the cases, before any case replays. surfaceGround is
// refused here because the replay manifest does not record it.
func (f surveyFlags) validate(experiments []string, measurementMode string, surfaceGround bool, observationDBPath string, caseIDs []string) error {
	if f.setPath == "" {
		return nil
	}
	switch {
	case len(experiments) > 0:
		return fmt.Errorf("-survey-coverage measures the default replay: drop -experiment %v", experiments)
	case measurementMode != string(l5tracks.MeasurementMedoidV0):
		return fmt.Errorf("-survey-coverage measures the default replay: -measurement-mode must be %s", l5tracks.MeasurementMedoidV0)
	case surfaceGround:
		return fmt.Errorf("-survey-coverage measures the default replay: drop -surface-ground")
	case observationDBPath == "":
		return fmt.Errorf("-survey-coverage reads the online estimates: give -evidence-dir with a source manifest")
	}
	for _, id := range caseIDs {
		if err := f.options(id).Validate(); err != nil {
			return err
		}
	}
	return replayeval.CheckCoverageSurveyTargets(f.setPath, caseIDs)
}

func (f surveyFlags) options(caseID string) replayeval.CoverageSurveyOptions {
	return replayeval.CoverageSurveyOptions{CaseID: caseID, Tool: surveyTool, RangePercentile: f.percentile, MinSectorGapDeg: f.gap}
}

// survey measures a case's coverage from its first run, replayDir and the
// evidence database at dbPath, and adds it to the set.
func (f surveyFlags) survey(dbPath, replayDir, caseID string) (*replayeval.CoverageSurvey, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("evidence database: %w", err)
	}
	database, err := db.NewDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	s, err := replayeval.SurveyContinuityCoverage(replayDir, database, f.options(caseID))
	if err != nil {
		return nil, err
	}
	if err := replayeval.AddCoverageSurvey(f.setPath, s); err != nil {
		return nil, err
	}
	return s, nil
}
