package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// settlingTrajectoryFile is the name of a case's convergence trajectory in its
// first run's output directory, written under -settling-trajectory.
const settlingTrajectoryFile = "settling_trajectory.json"

// settlingTrajectory is the file's form: the checks in order. An empty list
// means convergence was not armed, so no check was made.
type settlingTrajectory struct {
	Checks []l3grid.SettlingCheck `json:"checks"`
}

func writeSettlingTrajectory(dir string, checks []l3grid.SettlingCheck) error {
	if checks == nil {
		checks = []l3grid.SettlingCheck{}
	}
	b, err := json.MarshalIndent(settlingTrajectory{Checks: checks}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, settlingTrajectoryFile), append(b, '\n'), 0o644)
}
