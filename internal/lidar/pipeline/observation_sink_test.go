package pipeline

import (
	"errors"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

type memoryObservationSink struct {
	observations []l4bobserve.DetectionObservation
	err          error
}

func (s *memoryObservationSink) Insert(observation l4bobserve.DetectionObservation) error {
	if s.err != nil {
		return s.err
	}
	s.observations = append(s.observations, observation)
	return nil
}

func TestPersistDetectionObservationsRecordsBeforeTracking(t *testing.T) {
	sink := &memoryObservationSink{}
	frame := &l2frames.LiDARFrame{StartTimestamp: time.Unix(100, 0).UTC()}
	cfg := &TrackingPipelineConfig{SensorID: "hesai-pandar40p", ObservationSink: sink, ObservationSourceID: "source/v1/embarcadero-folsom", ObservationCalibrationID: "calibration/v1/s20"}
	clusters := []l4perception.WorldCluster{{ClusterID: 2, SensorID: "hesai-pandar40p", TSUnixNanos: 100_000_001, PointsCount: 3,
		RetainedPoints: []l4perception.WorldPoint{{X: 0, Y: 0, Z: 1}, {X: 1, Y: 0, Z: 1}, {X: 0, Y: 1, Z: 1}}}}
	if err := persistDetectionObservations(cfg, frame, clusters); err != nil {
		t.Fatal(err)
	}
	if len(sink.observations) != 1 {
		t.Fatalf("stored %d observations, want 1", len(sink.observations))
	}
	record := sink.observations[0].Snapshot()
	if record.Cluster.FrameID != "site/hesai-pandar40p" || record.FrameUnixNanos != frame.StartTimestamp.UnixNano() {
		t.Fatalf("record identity = %+v", record)
	}
	if len(record.Primitives.Planes) != 1 || len(record.Primitives.Edges) != 0 {
		t.Fatalf("primitive evidence = %+v", record.Primitives)
	}
}

func TestPersistDetectionObservationsRequiresIdentityAndPropagatesFailures(t *testing.T) {
	frame := &l2frames.LiDARFrame{StartTimestamp: time.Unix(100, 0).UTC()}
	cluster := []l4perception.WorldCluster{{ClusterID: 1, SensorID: "s", PointsCount: 1}}
	if err := persistDetectionObservations(&TrackingPipelineConfig{ObservationSink: &memoryObservationSink{}}, frame, cluster); err == nil {
		t.Fatal("accepted a sink without source and calibration identity")
	}
	want := errors.New("disk full")
	cfg := &TrackingPipelineConfig{SensorID: "s", ObservationSink: &memoryObservationSink{err: want}, ObservationSourceID: "source", ObservationCalibrationID: "calibration"}
	if err := persistDetectionObservations(cfg, frame, cluster); !errors.Is(err, want) {
		t.Fatalf("sink error = %v, want %v", err, want)
	}
}
