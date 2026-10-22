package pcapsplit

import (
	"math"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// BackgroundSettleWindow is how much of a static period SettledBackground
// replays: long enough for L3 to settle (its warmup is 30 s) and lock most
// cells, short enough to answer an operator who is waiting on a map.
const BackgroundSettleWindow = 90 * time.Second

// BackgroundPoint is one settled background cell in the sensor frame: metres,
// +Y the azimuth zero, +X 90° clockwise from it seen from above, +Z up.
type BackgroundPoint struct {
	X, Y, Z float32
	// Ring is the laser channel; TimesSeen how often the cell was observed.
	Ring      int
	TimesSeen uint32
}

// Background is the settled scene of a window of a static period.
type Background struct {
	Points []BackgroundPoint
	// Frames is the number of rotations the model saw.
	Frames int
	// FirstTime and LastTime bound the frames, in capture time.
	FirstTime, LastTime time.Time
	// SettledThreshold is the TimesSeen a cell needed to count as settled.
	SettledThreshold uint32
}

// newBackgroundModel builds the L3 model a settled-background pass uses: the
// same parameters as the motion classifier, but never refreshed, because a
// static period is one place.
func newBackgroundModel(sensorID, sourcePath string, tuning *config.TuningConfig, elevations []float64) (*l3grid.BackgroundManager, uint32, error) {
	tuning = tuningOrEmbedded(tuning)
	if err := tuning.ValidateImplementedEngines(); err != nil {
		return nil, 0, err
	}
	params := l3grid.BackgroundConfigFromActiveTuning(tuning).ToBackgroundParams()
	bg := l3grid.NewBackgroundManagerDI(sensorID, gridRings, gridAzBins, params, nil)
	bg.SetReplayMode(true)
	bg.SetSourcePath(sourcePath)
	if err := bg.SetRingElevations(elevations); err != nil {
		return nil, 0, err
	}
	threshold := params.LockedBaselineThreshold
	if threshold == 0 {
		threshold = l3grid.DefaultLockedBaselineThreshold
	}
	return bg, threshold, nil
}

// settledPoints converts the model's settled cells to sensor-frame points. A
// cell counts when it has been seen at least threshold times and has a range.
func settledPoints(bg *l3grid.BackgroundManager, elevations []float64, threshold uint32) []BackgroundPoint {
	cells := bg.GetGridCells()
	points := make([]BackgroundPoint, 0, len(cells))
	for _, c := range cells {
		if c.TimesSeen < threshold || c.Range <= 0 || c.Ring < 0 || c.Ring >= len(elevations) {
			continue
		}
		x, y, z := l2frames.SphericalToCartesian(float64(c.Range), float64(c.AzimuthDeg), elevations[c.Ring])
		if math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(z) {
			continue
		}
		points = append(points, BackgroundPoint{
			X: float32(x), Y: float32(y), Z: float32(z),
			Ring: c.Ring, TimesSeen: c.TimesSeen,
		})
	}
	return points
}
