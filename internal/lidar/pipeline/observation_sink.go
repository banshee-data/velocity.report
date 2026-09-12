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
	if cfg.ObservationSourceID == "" || cfg.ObservationCalibrationID == "" {
		return fmt.Errorf("observation sink requires explicit source and calibration identities")
	}
	if frame == nil {
		return fmt.Errorf("observation sink received nil frame")
	}
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
			return err
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
			return fmt.Errorf("freeze cluster %d: %w", cluster.ClusterID, err)
		}
		if err := cfg.ObservationSink.Insert(observation); err != nil {
			return fmt.Errorf("store cluster %d: %w", cluster.ClusterID, err)
		}
	}
	return nil
}
