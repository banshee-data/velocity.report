package l9endpoints

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

// replayClassifier re-classifies tracks during VRLOG replay when the
// recorded FrameBundle has empty ObjectClass (recordings made before
// classification was added to the pipeline).
var replayClassifier = l6objects.NewTrackClassifierWithMinObservations(3)

// objectClassFromString converts a classifier string label to the proto enum.
func objectClassFromString(s string) pb.ObjectClass {
	switch s {
	case "car":
		return pb.ObjectClass_OBJECT_CLASS_CAR
	case "truck":
		return pb.ObjectClass_OBJECT_CLASS_TRUCK
	case "bus":
		return pb.ObjectClass_OBJECT_CLASS_BUS
	case "pedestrian":
		return pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN
	case "cyclist":
		return pb.ObjectClass_OBJECT_CLASS_CYCLIST
	case "motorcyclist":
		return pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST
	case "bird":
		return pb.ObjectClass_OBJECT_CLASS_BIRD
	case "noise":
		return pb.ObjectClass_OBJECT_CLASS_NOISE
	case "dynamic":
		return pb.ObjectClass_OBJECT_CLASS_DYNAMIC
	default:
		return pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED
	}
}

// classifyOrConvert returns the proto ObjectClass for a track.
// If the track already carries a classification string, it is converted directly.
// Otherwise (empty ObjectClass — typical for VRLOG recordings made before
// classification existed), the track is re-classified on-the-fly from its
// aggregate features so that replayed data still shows meaningful labels.
func classifyOrConvert(t Track) pb.ObjectClass {
	if t.ObjectClass != "" {
		return objectClassFromString(t.ObjectClass)
	}
	// Re-classify from per-frame features when ObjectClass was not recorded
	features := l6objects.ClassificationFeatures{
		AvgHeight:        t.BBoxHeight,
		AvgLength:        t.BBoxLength,
		AvgWidth:         t.BBoxWidth,
		HeightP95:        t.HeightP95Max,
		AvgSpeed:         t.AvgSpeedMps,
		MaxSpeed:         t.MaxSpeedMps,
		ObservationCount: t.ObservationCount,
	}
	if t.LastSeenNanos > t.FirstSeenNanos {
		features.DurationSecs = float32(t.LastSeenNanos-t.FirstSeenNanos) / 1e9
	}
	result := replayClassifier.ClassifyFeatures(features)
	return objectClassFromString(string(result.Class))
}

// frameBundleToProto converts internal FrameBundle to protobuf.
func frameBundleToProto(frame *FrameBundle, req *pb.StreamRequest) *pb.FrameBundle {
	pbFrame := &pb.FrameBundle{
		FrameId:     frame.FrameID,
		TimestampNs: frame.TimestampNanos,
		SensorId:    frame.SensorID,
		CoordinateFrame: &pb.CoordinateFrameInfo{
			FrameId:        frame.CoordinateFrame.FrameID,
			ReferenceFrame: frame.CoordinateFrame.ReferenceFrame,
			OriginLat:      frame.CoordinateFrame.OriginLat,
			OriginLon:      frame.CoordinateFrame.OriginLon,
			OriginAlt:      frame.CoordinateFrame.OriginAlt,
			RotationDeg:    frame.CoordinateFrame.RotationDeg,
		},
	}

	// Include point cloud if requested
	if req.IncludePoints && frame.PointCloud != nil {
		pc := frame.PointCloud
		pbFrame.PointCloud = &pb.PointCloudFrame{
			FrameId:         pc.FrameID,
			TimestampNs:     pc.TimestampNanos,
			SensorId:        pc.SensorID,
			X:               pc.X,
			Y:               pc.Y,
			Z:               pc.Z,
			Intensity:       byteSliceToUint32(pc.Intensity),
			Classification:  byteSliceToUint32(pc.Classification),
			DecimationMode:  pb.DecimationMode(pc.DecimationMode),
			DecimationRatio: pc.DecimationRatio,
			PointCount:      int32(pc.PointCount),
		}
	}

	// Include clusters if requested
	if req.IncludeClusters && frame.Clusters != nil {
		cs := frame.Clusters
		pbClusters := make([]*pb.Cluster, len(cs.Clusters))
		for i, c := range cs.Clusters {
			pbCluster := &pb.Cluster{
				ClusterId:   c.ClusterID,
				SensorId:    c.SensorID,
				TimestampNs: c.TimestampNanos,
				CentroidX:   c.CentroidX,
				CentroidY:   c.CentroidY,
				CentroidZ:   c.CentroidZ,
				AabbLength:  c.AABBLength,
				AabbWidth:   c.AABBWidth,
				AabbHeight:  c.AABBHeight,
				PointsCount: int32(c.PointsCount),
			}
			if c.OBB != nil {
				pbCluster.Obb = &pb.OrientedBoundingBox{
					CenterX:    c.OBB.CenterX,
					CenterY:    c.OBB.CenterY,
					CenterZ:    c.OBB.CenterZ,
					Length:     c.OBB.Length,
					Width:      c.OBB.Width,
					Height:     c.OBB.Height,
					HeadingRad: c.OBB.HeadingRad,
				}
			}
			pbClusters[i] = pbCluster
		}
		pbFrame.Clusters = &pb.ClusterSet{
			FrameId:     cs.FrameID,
			TimestampNs: cs.TimestampNanos,
			Clusters:    pbClusters,
			Method:      pb.ClusteringMethod(cs.Method),
		}
	}

	// Include tracks if requested
	if req.IncludeTracks && frame.Tracks != nil {
		ts := frame.Tracks
		pbTracks := make([]*pb.Track, len(ts.Tracks))
		for i, t := range ts.Tracks {
			pbTracks[i] = &pb.Track{
				TrackId:           t.TrackID,
				SensorId:          t.SensorID,
				State:             pb.TrackState(t.State),
				Hits:              int32(t.Hits),
				Misses:            int32(t.Misses),
				ObservationCount:  int32(t.ObservationCount),
				FirstSeenNs:       t.FirstSeenNanos,
				LastSeenNs:        t.LastSeenNanos,
				X:                 t.X,
				Y:                 t.Y,
				Z:                 t.Z,
				Vx:                t.VX,
				Vy:                t.VY,
				Vz:                t.VZ,
				SpeedMps:          t.SpeedMps,
				HeadingRad:        t.HeadingRad,
				Covariance_4X4:    t.Covariance4x4,
				BboxLength:        t.BBoxLength,
				BboxWidth:         t.BBoxWidth,
				BboxHeight:        t.BBoxHeight,
				BboxHeadingRad:    t.BBoxHeadingRad,
				HeightP95Max:      t.HeightP95Max,
				IntensityMeanAvg:  t.IntensityMeanAvg,
				AvgSpeedMps:       t.AvgSpeedMps,
				MaxSpeedMps:       t.MaxSpeedMps,
				ObjectClass:       classifyOrConvert(t),
				ClassConfidence:   t.ClassConfidence,
				TrackLengthMetres: t.TrackLengthMetres,
				TrackDurationSecs: t.TrackDurationSecs,
				OcclusionCount:    int32(t.OcclusionCount),
				Confidence:        t.Confidence,
				OcclusionState:    pb.OcclusionState(t.OcclusionState),
				MotionModel:       pb.MotionModel(t.MotionModel),
				Alpha:             t.Alpha,
				HeadingSource:     int32(t.HeadingSource),
			}
		}

		pbTrails := make([]*pb.TrackTrail, len(ts.Trails))
		for i, trail := range ts.Trails {
			pbPoints := make([]*pb.TrackPoint, len(trail.Points))
			for j, p := range trail.Points {
				pbPoints[j] = &pb.TrackPoint{
					X:           p.X,
					Y:           p.Y,
					TimestampNs: p.TimestampNanos,
				}
			}
			pbTrails[i] = &pb.TrackTrail{
				TrackId: trail.TrackID,
				Points:  pbPoints,
			}
		}

		pbFrame.Tracks = &pb.TrackSet{
			FrameId:     ts.FrameID,
			TimestampNs: ts.TimestampNanos,
			Tracks:      pbTracks,
			Trails:      pbTrails,
		}
	}

	// Include playback info
	if frame.PlaybackInfo != nil {
		pbFrame.PlaybackInfo = &pb.PlaybackInfo{
			IsLive:            frame.PlaybackInfo.IsLive,
			LogStartNs:        frame.PlaybackInfo.LogStartNs,
			LogEndNs:          frame.PlaybackInfo.LogEndNs,
			PlaybackRate:      frame.PlaybackInfo.PlaybackRate,
			Paused:            frame.PlaybackInfo.Paused,
			CurrentFrameIndex: frame.PlaybackInfo.CurrentFrameIndex,
			TotalFrames:       frame.PlaybackInfo.TotalFrames,
			Seekable:          frame.PlaybackInfo.Seekable,
			ReplayEpoch:       frame.PlaybackInfo.ReplayEpoch,
		}
	}

	// M3.5: Include frame type and background snapshot
	pbFrame.FrameType = pb.FrameType(frame.FrameType)
	pbFrame.BackgroundSeq = frame.BackgroundSeq

	if frame.Background != nil {
		bg := frame.Background
		pbFrame.Background = &pb.BackgroundSnapshot{
			SequenceNumber: bg.SequenceNumber,
			TimestampNanos: bg.TimestampNanos,
			X:              bg.X,
			Y:              bg.Y,
			Z:              bg.Z,
			Confidence:     bg.Confidence,
			GridMetadata: &pb.GridMetadata{
				Rings:            int32(bg.GridMetadata.Rings),
				AzimuthBins:      int32(bg.GridMetadata.AzimuthBins),
				RingElevations:   bg.GridMetadata.RingElevations,
				SettlingComplete: bg.GridMetadata.SettlingComplete,
			},
		}
	}

	return pbFrame
}

// byteSliceToUint32 converts []uint8 to []uint32.
func byteSliceToUint32(b []uint8) []uint32 {
	result := make([]uint32, len(b))
	for i, v := range b {
		result[i] = uint32(v)
	}
	return result
}
