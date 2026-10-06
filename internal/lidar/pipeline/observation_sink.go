package pipeline

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// persistDetectionObservations records every L4 cluster before the L5 profile
// gate and association step. A rejected or untracked cluster is still sensor
// evidence, and must remain available to a later estimator or incident review.
func persistDetectionObservations(cfg *TrackingPipelineConfig, frame *l2frames.LiDARFrame, clusters []l4perception.WorldCluster) error {
	if cfg.ObservationSink == nil {
		return nil
	}
	observations, err := freezeDetectionObservations(cfg, frame, clusters)
	if err != nil {
		return err
	}
	for _, observation := range observations {
		if err := cfg.ObservationSink.Insert(observation); err != nil {
			return fmt.Errorf("store observation: %w", err)
		}
	}
	return nil
}

func freezeDetectionObservations(cfg *TrackingPipelineConfig, frame *l2frames.LiDARFrame, clusters []l4perception.WorldCluster) ([]l4bobserve.DetectionObservation, error) {
	if cfg.ObservationSourceID == "" || cfg.ObservationCalibrationID == "" {
		return nil, fmt.Errorf("observation sink requires explicit source and calibration identities")
	}
	if frame == nil {
		return nil, fmt.Errorf("observation sink received nil frame")
	}
	observations := make([]l4bobserve.DetectionObservation, 0, len(clusters))
	for _, cluster := range clusters {
		// DBSCAN labels are stable only within this frame, hence the frame time
		// is part of ObservationID. FrameID was formerly assigned only for the
		// tracker persistence row; evidence needs the coordinate-frame identity
		// before L5 decides whether it wants the cluster.
		cluster.FrameID = l4perception.FrameID(fmt.Sprintf("site/%s", cfg.SensorID))
		observationID, err := l4bobserve.ObservationID(
			cfg.ObservationSourceID,
			cfg.ObservationCalibrationID,
			frame.StartTimestamp.UnixNano(),
			cluster.ClusterID,
		)
		if err != nil {
			return nil, err
		}
		observation, err := l4bobserve.New(l4bobserve.Record{
			SchemaVersion:  1,
			ObservationID:  observationID,
			SourceID:       cfg.ObservationSourceID,
			CalibrationID:  cfg.ObservationCalibrationID,
			FrameUnixNanos: frame.StartTimestamp.UnixNano(),
			Cluster:        cluster,
			Primitives:     l4bobserve.ExtractPrimitives(cluster),
		})
		if err != nil {
			return nil, fmt.Errorf("freeze cluster %d: %w", cluster.ClusterID, err)
		}
		observations = append(observations, observation)
	}
	return observations, nil
}
