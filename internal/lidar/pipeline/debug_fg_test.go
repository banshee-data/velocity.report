package pipeline

import (
	"fmt"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestDebugForeground(t *testing.T) {
	sensorID := "debug-fg-test"
	bgMgr := l3grid.NewBackgroundManagerDI(sensorID, 16, 360, l3grid.BackgroundParams{
		SeedFromFirstObservation:       true,
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             0.5,
		NeighborConfirmationCount:      0,
		NoiseRelativeFraction:          0.01,
		WarmupDurationNanos:            0,
		WarmupMinFrames:                0,
	}, nil)

	// Seed with stable distance
	seedPt := []l4perception.PointPolar{{Channel: 1, Azimuth: 180.0, Distance: 20.0}}
	for i := 0; i < 5; i++ {
		mask, err := bgMgr.ProcessFramePolarWithMask(seedPt)
		fmt.Printf("Seed %d: mask=%v, err=%v\n", i, mask, err)
	}

	// Now send foreground
	fgPt := []l4perception.PointPolar{
		{Channel: 1, Azimuth: 180.0, Distance: 20.0},
		{Channel: 1, Azimuth: 180.0, Distance: 5.0},
	}
	mask, err := bgMgr.ProcessFramePolarWithMask(fgPt)
	fmt.Printf("FG: mask=%v, err=%v\n", mask, err)

	fg := l3grid.ExtractForegroundPoints(fgPt, mask)
	fmt.Printf("FG points: %d\n", len(fg))
}
