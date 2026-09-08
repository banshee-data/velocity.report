package replayeval

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// replayRuntime keeps infrastructure failure tests local to a run, without
// mutable package globals or timing-dependent filesystem sabotage.
type replayRuntime struct {
	hashFile      func(string) (string, error)
	loadParser    func() (*parse.Pandar40PConfig, error)
	background    func(string, *config.TuningConfig, []float64) (*l3grid.BackgroundManager, error)
	newRecorder   func(string, string) (*recorder.Recorder, error)
	closeRecorder func(*recorder.Recorder) error
	marshal       func(any) ([]byte, error)
	marshalIndent func(any, string, string) ([]byte, error)
	writeFile     func(string, []byte, os.FileMode) error
	writeBaseline func(string, l5tracks.TrackingMetrics) error
}

func defaultRuntime() replayRuntime {
	return replayRuntime{
		hashFile: fileSHA256, loadParser: parse.LoadPandar40PConfig,
		background: prepareBackground, newRecorder: recorder.NewRecorder,
		closeRecorder: (*recorder.Recorder).Close,
		marshal:       json.Marshal, marshalIndent: json.MarshalIndent,
		writeFile: os.WriteFile, writeBaseline: writeTrackingBaseline,
	}
}

func prepareBackground(
	sensorID string, tuning *config.TuningConfig, elevations []float64,
) (*l3grid.BackgroundManager, error) {
	bgConfig := l3grid.BackgroundConfigFromActiveTuning(tuning)
	if err := bgConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid background config: %w", err)
	}
	const rings, azBins = 40, 1800
	bg := l3grid.NewBackgroundManagerDI(sensorID, rings, azBins, bgConfig.ToBackgroundParams(), nil)
	if bg == nil {
		return nil, fmt.Errorf("failed to create BackgroundManager")
	}
	if err := bg.SetRingElevations(elevations); err != nil {
		return nil, fmt.Errorf("set ring elevations: %w", err)
	}
	return bg, nil
}
