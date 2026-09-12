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

func TestTrackerCanUseExplicitReplayMedoidReference(t *testing.T) {
	config := DefaultTrackerConfig()
	config.MeasurementSourceMode = MeasurementMedoidV0
	tracker := NewTracker(config)
	tracker.Update([]l4perception.WorldCluster{{
		SensorID: "test", ClusterID: 1, CentroidX: 9, CentroidY: 8,
		OBB: &l4perception.OrientedBoundingBox{CenterX: 2, CenterY: 3},
	}}, time.Unix(0, 100))
	for _, track := range tracker.Tracks {
		if track.X != 9 || track.Y != 8 || track.LastMeasurementSource != MeasurementMedoidV0 {
			t.Fatalf("replay reference state = %+v", track)
		}
		return
	}
	t.Fatal("replay reference created no track")
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

func TestInterpretMeasurementRecordsNearFaceButKeepsD2Input(t *testing.T) {
	cluster := WorldCluster{
		TSUnixNanos: 77,
		PointsCount: 16,
		CentroidX:   99,
		CentroidY:   99,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: 10, CenterY: 20, Length: 4, Width: 2, HeadingRad: 0,
		},
	}
	got := InterpretMeasurement(cluster, 10, 0, 0, 11, 19, 0.05)
	if got.Measurement.Source != MeasurementOBBCentreV1 || got.Measurement.X != 10 || got.Measurement.Y != 20 || got.Measurement.UnixNanos != 77 {
		t.Fatalf("D2 measurement = %+v, want OBB centre at capture time", got.Measurement)
	}
	if !got.NearEdgeAvailable || got.NearEdgeX != 8 || got.NearEdgeY != 19 {
		t.Fatalf("near face = (%v, %v), available=%t; want (8, 19), true", got.NearEdgeX, got.NearEdgeY, got.NearEdgeAvailable)
	}
	if got.Covariance.XX <= 0 || got.Covariance.YY <= 0 || got.Covariance.XY != 0 {
		t.Fatalf("invalid axis-aligned covariance: %+v", got.Covariance)
	}
}

func TestInterpretMeasurementFallsBackWithoutValidOBB(t *testing.T) {
	cluster := WorldCluster{CentroidX: 3, CentroidY: 4, PointsCount: 2}
	got := InterpretMeasurement(cluster, 12, float32(math.NaN()), 0, 0, 0, 0.05)
	if got.Measurement.Source != MeasurementMedoidFallbackV1 {
		t.Fatalf("measurement source = %q, want explicit medoid fallback", got.Measurement.Source)
	}
	if got.NearEdgeAvailable || got.FallbackReason != "missing_or_invalid_obb" {
		t.Fatalf("near edge fallback = %+v, want invalid OBB", got)
	}
	if got.Covariance.XX != 0.05 || got.Covariance.YY != 0.05 || got.Covariance.XY != 0 {
		t.Fatalf("fallback covariance = %+v, want tuned isotropic floor", got.Covariance)
	}
}
