package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestMeasurementForClusterUsesOBBCentreAndCaptureTime(t *testing.T) {
	cluster := l4perception.WorldCluster{
		CentroidX: 9, CentroidY: 8, TSUnixNanos: 1234,
		OBB: &l4perception.OrientedBoundingBox{CenterX: 2, CenterY: 3},
	}
	got := measurementForCluster(cluster, 999)
	if got.X != 2 || got.Y != 3 || got.UnixNanos != 1234 || got.Source != MeasurementOBBCentreV1 {
		t.Fatalf("measurement = %+v", got)
	}
}

func TestMeasurementForClusterFallsBackExplicitly(t *testing.T) {
	cluster := l4perception.WorldCluster{
		CentroidX: 9, CentroidY: 8,
		OBB: &l4perception.OrientedBoundingBox{CenterX: float32(math.NaN()), CenterY: 3},
	}
	got := measurementForCluster(cluster, 999)
	if got.X != 9 || got.Y != 8 || got.UnixNanos != 999 || got.Source != MeasurementMedoidFallbackV1 {
		t.Fatalf("measurement = %+v", got)
	}
}

func TestTrackerUsesSameCorrectedMeasurementForInitialAndUpdatedState(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 1
	tracker := NewTracker(config)
	t0 := time.Unix(0, 1_000)
	first := l4perception.WorldCluster{
		SensorID: "test", ClusterID: 1, TSUnixNanos: 1_001,
		CentroidX: 100, CentroidY: 100,
		OBB: &l4perception.OrientedBoundingBox{CenterX: 2, CenterY: 3},
	}
	tracker.Update([]l4perception.WorldCluster{first}, t0)
	if len(tracker.Tracks) != 1 {
		t.Fatalf("tracks = %d", len(tracker.Tracks))
	}
	var track *TrackedObject
	for _, value := range tracker.Tracks {
		track = value
	}
	if track.X != 2 || track.Y != 3 || track.LastMeasurementUnixNanos != 1_001 || track.LastMeasurementSource != MeasurementOBBCentreV1 {
		t.Fatalf("initial state = %+v", track)
	}
	second := first
	second.ClusterID = 2
	second.TSUnixNanos = 100_000_001
	second.CentroidX, second.CentroidY = 200, 200
	second.OBB = &l4perception.OrientedBoundingBox{CenterX: 2.1, CenterY: 3.1}
	tracker.Update([]l4perception.WorldCluster{second}, time.Unix(0, 100_000_000))
	if track.LastMeasurementUnixNanos != 100_000_001 || track.LastMeasurementSource != MeasurementOBBCentreV1 {
		t.Fatalf("updated provenance = %d %s", track.LastMeasurementUnixNanos, track.LastMeasurementSource)
	}
	if track.X > 10 || track.Y > 10 {
		t.Fatalf("medoid leaked into corrected update: x=%f y=%f", track.X, track.Y)
	}
}
