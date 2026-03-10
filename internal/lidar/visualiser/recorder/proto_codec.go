// Package recorder — protobuf codec for VRLOG frame storage.
//
// Replaces the placeholder JSON serialization with protobuf, reducing
// per-frame size by ~5-10x and eliminating CPU-intensive json.Marshal /
// json.Unmarshal on replay.
package recorder

import (
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"google.golang.org/protobuf/proto"
)

// serializeFrameProto serializes a FrameBundle to protobuf wire format.
// Unlike the gRPC-path frameBundleToProto, this always includes all fields
// (no StreamRequest filtering) to ensure lossless round-trip storage.
func serializeFrameProto(frame *visualiser.FrameBundle) ([]byte, error) {
	pbFrame := frameBundleToStorageProto(frame)
	return proto.Marshal(pbFrame)
}

// deserializeFrameProto deserializes protobuf wire format to a FrameBundle.
func deserializeFrameProto(data []byte) (*visualiser.FrameBundle, error) {
	var pbFrame pb.FrameBundle
	if err := proto.Unmarshal(data, &pbFrame); err != nil {
		return nil, err
	}
	return protoToFrameBundle(&pbFrame), nil
}

// frameBundleToStorageProto converts internal FrameBundle to protobuf for
// on-disk storage. Includes all fields unconditionally.
func frameBundleToStorageProto(frame *visualiser.FrameBundle) *pb.FrameBundle {
	pbFrame := &pb.FrameBundle{
		FrameId:     frame.FrameID,
		TimestampNs: frame.TimestampNanos,
		SensorId:    frame.SensorID,
		FrameType:   pb.FrameType(frame.FrameType),
		CoordinateFrame: &pb.CoordinateFrameInfo{
			FrameId:        frame.CoordinateFrame.FrameID,
			ReferenceFrame: frame.CoordinateFrame.ReferenceFrame,
			OriginLat:      frame.CoordinateFrame.OriginLat,
			OriginLon:      frame.CoordinateFrame.OriginLon,
			OriginAlt:      frame.CoordinateFrame.OriginAlt,
			RotationDeg:    frame.CoordinateFrame.RotationDeg,
		},
		BackgroundSeq: frame.BackgroundSeq,
	}

	if frame.PointCloud != nil {
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

	if frame.Clusters != nil {
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

	if frame.Tracks != nil {
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
				ObjectClass:       objectClassFromString(t.ObjectClass),
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

// protoToFrameBundle converts a protobuf FrameBundle back to the internal model.
func protoToFrameBundle(pbFrame *pb.FrameBundle) *visualiser.FrameBundle {
	frame := &visualiser.FrameBundle{
		FrameID:        pbFrame.FrameId,
		TimestampNanos: pbFrame.TimestampNs,
		SensorID:       pbFrame.SensorId,
		FrameType:      visualiser.FrameType(pbFrame.FrameType),
		BackgroundSeq:  pbFrame.BackgroundSeq,
	}

	if pbFrame.CoordinateFrame != nil {
		frame.CoordinateFrame = visualiser.CoordinateFrameInfo{
			FrameID:        pbFrame.CoordinateFrame.FrameId,
			ReferenceFrame: pbFrame.CoordinateFrame.ReferenceFrame,
			OriginLat:      pbFrame.CoordinateFrame.OriginLat,
			OriginLon:      pbFrame.CoordinateFrame.OriginLon,
			OriginAlt:      pbFrame.CoordinateFrame.OriginAlt,
			RotationDeg:    pbFrame.CoordinateFrame.RotationDeg,
		}
	}

	if pbFrame.PointCloud != nil {
		pc := pbFrame.PointCloud
		frame.PointCloud = &visualiser.PointCloudFrame{
			FrameID:         pc.FrameId,
			TimestampNanos:  pc.TimestampNs,
			SensorID:        pc.SensorId,
			X:               pc.X,
			Y:               pc.Y,
			Z:               pc.Z,
			Intensity:       uint32SliceToBytes(pc.Intensity),
			Classification:  uint32SliceToBytes(pc.Classification),
			DecimationMode:  visualiser.DecimationMode(pc.DecimationMode),
			DecimationRatio: pc.DecimationRatio,
			PointCount:      int(pc.PointCount),
		}
	}

	if pbFrame.Clusters != nil {
		cs := pbFrame.Clusters
		clusters := make([]visualiser.Cluster, len(cs.Clusters))
		for i, c := range cs.Clusters {
			clusters[i] = visualiser.Cluster{
				ClusterID:      c.ClusterId,
				SensorID:       c.SensorId,
				TimestampNanos: c.TimestampNs,
				CentroidX:      c.CentroidX,
				CentroidY:      c.CentroidY,
				CentroidZ:      c.CentroidZ,
				AABBLength:     c.AabbLength,
				AABBWidth:      c.AabbWidth,
				AABBHeight:     c.AabbHeight,
				PointsCount:    int(c.PointsCount),
			}
			if c.Obb != nil {
				clusters[i].OBB = &visualiser.OrientedBoundingBox{
					CenterX:    c.Obb.CenterX,
					CenterY:    c.Obb.CenterY,
					CenterZ:    c.Obb.CenterZ,
					Length:     c.Obb.Length,
					Width:      c.Obb.Width,
					Height:     c.Obb.Height,
					HeadingRad: c.Obb.HeadingRad,
				}
			}
		}
		frame.Clusters = &visualiser.ClusterSet{
			FrameID:        cs.FrameId,
			TimestampNanos: cs.TimestampNs,
			Clusters:       clusters,
			Method:         visualiser.ClusteringMethod(cs.Method),
		}
	}

	if pbFrame.Tracks != nil {
		ts := pbFrame.Tracks
		tracks := make([]visualiser.Track, len(ts.Tracks))
		for i, t := range ts.Tracks {
			tracks[i] = visualiser.Track{
				TrackID:           t.TrackId,
				SensorID:          t.SensorId,
				State:             visualiser.TrackState(t.State),
				Hits:              int(t.Hits),
				Misses:            int(t.Misses),
				ObservationCount:  int(t.ObservationCount),
				FirstSeenNanos:    t.FirstSeenNs,
				LastSeenNanos:     t.LastSeenNs,
				X:                 t.X,
				Y:                 t.Y,
				Z:                 t.Z,
				VX:                t.Vx,
				VY:                t.Vy,
				VZ:                t.Vz,
				SpeedMps:          t.SpeedMps,
				HeadingRad:        t.HeadingRad,
				Covariance4x4:     t.Covariance_4X4,
				BBoxLength:        t.BboxLength,
				BBoxWidth:         t.BboxWidth,
				BBoxHeight:        t.BboxHeight,
				BBoxHeadingRad:    t.BboxHeadingRad,
				HeightP95Max:      t.HeightP95Max,
				IntensityMeanAvg:  t.IntensityMeanAvg,
				AvgSpeedMps:       t.AvgSpeedMps,
				MaxSpeedMps:       t.MaxSpeedMps,
				ObjectClass:       objectClassToString(t.ObjectClass),
				ClassConfidence:   t.ClassConfidence,
				TrackLengthMetres: t.TrackLengthMetres,
				TrackDurationSecs: t.TrackDurationSecs,
				OcclusionCount:    int(t.OcclusionCount),
				Confidence:        t.Confidence,
				OcclusionState:    visualiser.OcclusionState(t.OcclusionState),
				MotionModel:       visualiser.MotionModel(t.MotionModel),
				Alpha:             t.Alpha,
				HeadingSource:     int(t.HeadingSource),
			}
		}
		trails := make([]visualiser.TrackTrail, len(ts.Trails))
		for i, trail := range ts.Trails {
			points := make([]visualiser.TrackPoint, len(trail.Points))
			for j, p := range trail.Points {
				points[j] = visualiser.TrackPoint{
					X:              p.X,
					Y:              p.Y,
					TimestampNanos: p.TimestampNs,
				}
			}
			trails[i] = visualiser.TrackTrail{
				TrackID: trail.TrackId,
				Points:  points,
			}
		}
		frame.Tracks = &visualiser.TrackSet{
			FrameID:        ts.FrameId,
			TimestampNanos: ts.TimestampNs,
			Tracks:         tracks,
			Trails:         trails,
		}
	}

	if pbFrame.PlaybackInfo != nil {
		frame.PlaybackInfo = &visualiser.PlaybackInfo{
			IsLive:            pbFrame.PlaybackInfo.IsLive,
			LogStartNs:        pbFrame.PlaybackInfo.LogStartNs,
			LogEndNs:          pbFrame.PlaybackInfo.LogEndNs,
			PlaybackRate:      pbFrame.PlaybackInfo.PlaybackRate,
			Paused:            pbFrame.PlaybackInfo.Paused,
			CurrentFrameIndex: pbFrame.PlaybackInfo.CurrentFrameIndex,
			TotalFrames:       pbFrame.PlaybackInfo.TotalFrames,
			Seekable:          pbFrame.PlaybackInfo.Seekable,
			ReplayEpoch:       pbFrame.PlaybackInfo.ReplayEpoch,
		}
	}

	if pbFrame.Background != nil {
		bg := pbFrame.Background
		frame.Background = &visualiser.BackgroundSnapshot{
			SequenceNumber: bg.SequenceNumber,
			TimestampNanos: bg.TimestampNanos,
			X:              bg.X,
			Y:              bg.Y,
			Z:              bg.Z,
			Confidence:     bg.Confidence,
		}
		if bg.GridMetadata != nil {
			frame.Background.GridMetadata = visualiser.GridMetadata{
				Rings:            int(bg.GridMetadata.Rings),
				AzimuthBins:      int(bg.GridMetadata.AzimuthBins),
				RingElevations:   bg.GridMetadata.RingElevations,
				SettlingComplete: bg.GridMetadata.SettlingComplete,
			}
		}
	}

	return frame
}

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

// objectClassToString converts a proto ObjectClass enum to the internal string label.
func objectClassToString(c pb.ObjectClass) string {
	switch c {
	case pb.ObjectClass_OBJECT_CLASS_CAR:
		return "car"
	case pb.ObjectClass_OBJECT_CLASS_TRUCK:
		return "truck"
	case pb.ObjectClass_OBJECT_CLASS_BUS:
		return "bus"
	case pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN:
		return "pedestrian"
	case pb.ObjectClass_OBJECT_CLASS_CYCLIST:
		return "cyclist"
	case pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST:
		return "motorcyclist"
	case pb.ObjectClass_OBJECT_CLASS_BIRD:
		return "bird"
	case pb.ObjectClass_OBJECT_CLASS_NOISE:
		return "noise"
	case pb.ObjectClass_OBJECT_CLASS_DYNAMIC:
		return "dynamic"
	default:
		return ""
	}
}

// byteSliceToUint32 converts []uint8 to []uint32 for proto fields.
func byteSliceToUint32(b []uint8) []uint32 {
	result := make([]uint32, len(b))
	for i, v := range b {
		result[i] = uint32(v)
	}
	return result
}

// uint32SliceToBytes converts []uint32 to []uint8, clamping values > 255.
func uint32SliceToBytes(u []uint32) []uint8 {
	result := make([]uint8, len(u))
	for i, v := range u {
		if v > 255 {
			result[i] = 255
		} else {
			result[i] = uint8(v)
		}
	}
	return result
}
