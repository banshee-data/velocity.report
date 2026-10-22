package pcapsplit

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestSettledPointsKeepOnlySettledCellsInTheSensorFrame(t *testing.T) {
	elevations := make([]float64, gridRings)
	elevations[2] = -10 // channel 3 is ring 2: channels count from 1
	bg, threshold, err := newBackgroundModel("hesai-pandar40p", "fixture.pcap", nil, elevations)
	if err != nil {
		t.Fatal(err)
	}
	if threshold == 0 {
		t.Fatal("no settled threshold")
	}

	// A wall at 20 m straight ahead on channel 3, seen every frame, and a cell at
	// 90° seen only once: the wall settles, the passer-by does not.
	start := time.Unix(1_788_000_000, 0)
	for i := 0; i < int(threshold)+5; i++ {
		points := []l3grid.PointPolar{{Channel: 3, Azimuth: 0.1, Elevation: -10, Distance: 20}}
		if i == 0 {
			points = append(points, l3grid.PointPolar{Channel: 3, Azimuth: 90.1, Elevation: -10, Distance: 8})
		}
		bg.ProcessFramePolarWithMaskAt(points, start.Add(time.Duration(i)*100*time.Millisecond))
	}

	got := settledPoints(bg, elevations, threshold)
	if len(got) != 1 {
		t.Fatalf("%d settled points, want 1: %+v", len(got), got)
	}
	p := got[0]
	horizontal := 20 * math.Cos(10*math.Pi/180)
	if p.Ring != 2 || math.Abs(float64(p.Y)-horizontal) > 0.05 || math.Abs(float64(p.X)) > 0.05 ||
		math.Abs(float64(p.Z)+20*math.Sin(10*math.Pi/180)) > 0.05 {
		t.Fatalf("point %+v, want ahead on +Y at %.2f m and below the sensor", p, horizontal)
	}
	if p.TimesSeen < threshold {
		t.Fatalf("times seen %d below the threshold %d", p.TimesSeen, threshold)
	}

	if pts := settledPoints(bg, elevations[:2], threshold); len(pts) != 0 {
		t.Fatalf("a ring without an elevation produced %d points", len(pts))
	}
}
