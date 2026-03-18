package recorder

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
)

func TestSerializeDeserialize_FullFrame(t *testing.T) {
	frame := &visualiser.FrameBundle{
		FrameID: 42, TimestampNanos: 1234567890, SensorID: "test-sensor",
		FrameType: visualiser.FrameTypeForeground, BackgroundSeq: 7,
		CoordinateFrame: visualiser.CoordinateFrameInfo{
			FrameID: "site/test", ReferenceFrame: "ENU",
			OriginLat: 51.5, OriginLon: -0.12, OriginAlt: 30.0, RotationDeg: 45.0,
		},
		PointCloud: &visualiser.PointCloudFrame{
			FrameID: 42, TimestampNanos: 1234567890, SensorID: "test-sensor",
			X: []float32{1.0, 2.0}, Y: []float32{3.0, 4.0}, Z: []float32{5.0, 6.0},
			Intensity: []uint8{100, 200}, Classification: []uint8{0, 1},
			DecimationMode: visualiser.DecimationUniform, DecimationRatio: 0.5,
			PointCount: 2,
		},
		Clusters: &visualiser.ClusterSet{
			FrameID: 42, TimestampNanos: 1234567890,
			Method: visualiser.ClusteringDBSCAN,
			Clusters: []visualiser.Cluster{
				{
					ClusterID: 1, SensorID: "s1", TimestampNanos: 123,
					CentroidX: 1.0, CentroidY: 2.0, CentroidZ: 3.0,
					AABBLength: 4.0, AABBWidth: 2.0, AABBHeight: 1.5,
					PointsCount: 100,
					OBB: &visualiser.OrientedBoundingBox{
						CenterX: 1.0, CenterY: 2.0, CenterZ: 3.0,
						Length: 4.0, Width: 2.0, Height: 1.5, HeadingRad: 0.5,
					},
				},
			},
		},
		Tracks: &visualiser.TrackSet{
			FrameID: 42, TimestampNanos: 1234567890,
			Tracks: []visualiser.Track{
				{
					TrackID: "t1", SensorID: "s1",
					State: visualiser.TrackStateConfirmed,
					Hits: 10, Misses: 2, ObservationCount: 12,
					FirstSeenNanos: 1000, LastSeenNanos: 2000,
					X: 1.0, Y: 2.0, Z: 3.0, VX: 0.5, VY: 0.3, VZ: 0.0,
					SpeedMps: 5.0, HeadingRad: 1.2,
					Covariance4x4: []float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
					BBoxLength: 4.5, BBoxWidth: 2.0, BBoxHeight: 1.5, BBoxHeadingRad: 0.5,
					HeightP95Max: 1.8, IntensityMeanAvg: 120, AvgSpeedMps: 4.5, MaxSpeedMps: 8.0,
					ObjectClass: "car", ClassConfidence: 0.85,
					TrackLengthMetres: 50.0, TrackDurationSecs: 10.0,
					OcclusionCount: 1, Confidence: 0.9,
					OcclusionState: visualiser.OcclusionPartial,
					MotionModel:    visualiser.MotionModelCV,
					Alpha:          1.0, HeadingSource: 1,
				},
			},
			Trails: []visualiser.TrackTrail{
				{
					TrackID: "t1",
					Points: []visualiser.TrackPoint{
						{X: 0.0, Y: 0.0, TimestampNanos: 1000},
						{X: 1.0, Y: 2.0, TimestampNanos: 2000},
					},
				},
			},
		},
		PlaybackInfo: &visualiser.PlaybackInfo{
			IsLive: false, LogStartNs: 1000, LogEndNs: 5000,
			PlaybackRate: 1.0, Paused: false,
			CurrentFrameIndex: 5, TotalFrames: 100,
			Seekable: true, ReplayEpoch: 3,
		},
		Background: &visualiser.BackgroundSnapshot{
			SequenceNumber: 1, TimestampNanos: 1234,
			X: []float32{10, 20}, Y: []float32{30, 40}, Z: []float32{50, 60},
			Confidence: []uint32{5, 10},
			GridMetadata: visualiser.GridMetadata{
				Rings: 40, AzimuthBins: 3600,
				RingElevations:  []float32{-15.0, 15.0},
				SettlingComplete: true,
			},
		},
	}

	data, err := serializeFrameProto(frame)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	result, err := deserializeFrameProto(data)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	// Verify key fields survived round-trip.
	if result.FrameID != 42 {
		t.Errorf("FrameID: got %d, want 42", result.FrameID)
	}
	if result.SensorID != "test-sensor" {
		t.Errorf("SensorID: got %s, want test-sensor", result.SensorID)
	}
	if result.FrameType != visualiser.FrameTypeForeground {
		t.Errorf("FrameType: got %d, want %d", result.FrameType, visualiser.FrameTypeForeground)
	}
	if result.BackgroundSeq != 7 {
		t.Errorf("BackgroundSeq: got %d, want 7", result.BackgroundSeq)
	}

	// CoordinateFrame
	if result.CoordinateFrame.FrameID != "site/test" {
		t.Errorf("CoordinateFrame.FrameID: got %s, want site/test", result.CoordinateFrame.FrameID)
	}
	if result.CoordinateFrame.RotationDeg != 45.0 {
		t.Errorf("CoordinateFrame.RotationDeg: got %f, want 45.0", result.CoordinateFrame.RotationDeg)
	}

	// PointCloud
	if result.PointCloud == nil {
		t.Fatal("PointCloud should not be nil")
	}
	if len(result.PointCloud.X) != 2 {
		t.Errorf("PointCloud.X length: got %d, want 2", len(result.PointCloud.X))
	}
	if result.PointCloud.DecimationMode != visualiser.DecimationUniform {
		t.Errorf("DecimationMode: got %d, want %d", result.PointCloud.DecimationMode, visualiser.DecimationUniform)
	}
	if result.PointCloud.PointCount != 2 {
		t.Errorf("PointCount: got %d, want 2", result.PointCloud.PointCount)
	}

	// Clusters
	if result.Clusters == nil || len(result.Clusters.Clusters) != 1 {
		t.Fatal("expected 1 cluster")
	}
	if result.Clusters.Clusters[0].OBB == nil {
		t.Fatal("cluster OBB should not be nil")
	}
	if result.Clusters.Clusters[0].OBB.HeadingRad != 0.5 {
		t.Errorf("OBB.HeadingRad: got %f, want 0.5", result.Clusters.Clusters[0].OBB.HeadingRad)
	}

	// Tracks
	if result.Tracks == nil || len(result.Tracks.Tracks) != 1 {
		t.Fatal("expected 1 track")
	}
	trk := result.Tracks.Tracks[0]
	if trk.ObjectClass != "car" {
		t.Errorf("ObjectClass: got %s, want car", trk.ObjectClass)
	}
	if trk.ClassConfidence != 0.85 {
		t.Errorf("ClassConfidence: got %f, want 0.85", trk.ClassConfidence)
	}
	if trk.State != visualiser.TrackStateConfirmed {
		t.Errorf("TrackState: got %d, want %d", trk.State, visualiser.TrackStateConfirmed)
	}
	if trk.OcclusionState != visualiser.OcclusionPartial {
		t.Errorf("OcclusionState: got %d, want %d", trk.OcclusionState, visualiser.OcclusionPartial)
	}
	if trk.MotionModel != visualiser.MotionModelCV {
		t.Errorf("MotionModel: got %d, want %d", trk.MotionModel, visualiser.MotionModelCV)
	}
	if len(trk.Covariance4x4) != 16 {
		t.Errorf("Covariance4x4 length: got %d, want 16", len(trk.Covariance4x4))
	}

	// Trails
	if len(result.Tracks.Trails) != 1 || len(result.Tracks.Trails[0].Points) != 2 {
		t.Error("trail round-trip failed")
	}

	// PlaybackInfo
	if result.PlaybackInfo == nil || !result.PlaybackInfo.Seekable {
		t.Error("PlaybackInfo round-trip failed")
	}
	if result.PlaybackInfo.TotalFrames != 100 {
		t.Errorf("TotalFrames: got %d, want 100", result.PlaybackInfo.TotalFrames)
	}
	if result.PlaybackInfo.ReplayEpoch != 3 {
		t.Errorf("ReplayEpoch: got %d, want 3", result.PlaybackInfo.ReplayEpoch)
	}

	// Background
	if result.Background == nil {
		t.Fatal("Background should not be nil")
	}
	if result.Background.GridMetadata.Rings != 40 {
		t.Errorf("GridMetadata.Rings: got %d, want 40", result.Background.GridMetadata.Rings)
	}
	if result.Background.GridMetadata.AzimuthBins != 3600 {
		t.Errorf("GridMetadata.AzimuthBins: got %d, want 3600", result.Background.GridMetadata.AzimuthBins)
	}
	if !result.Background.GridMetadata.SettlingComplete {
		t.Error("GridMetadata.SettlingComplete: got false, want true")
	}
	if len(result.Background.GridMetadata.RingElevations) != 2 {
		t.Errorf("RingElevations length: got %d, want 2", len(result.Background.GridMetadata.RingElevations))
	}
}

func TestObjectClassFromString(t *testing.T) {
	cases := []struct {
		input string
		want  pb.ObjectClass
	}{
		{"car", pb.ObjectClass_OBJECT_CLASS_CAR},
		{"truck", pb.ObjectClass_OBJECT_CLASS_TRUCK},
		{"bus", pb.ObjectClass_OBJECT_CLASS_BUS},
		{"pedestrian", pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN},
		{"cyclist", pb.ObjectClass_OBJECT_CLASS_CYCLIST},
		{"motorcyclist", pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST},
		{"bird", pb.ObjectClass_OBJECT_CLASS_BIRD},
		{"noise", pb.ObjectClass_OBJECT_CLASS_NOISE},
		{"dynamic", pb.ObjectClass_OBJECT_CLASS_DYNAMIC},
		{"unknown", pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED},
		{"", pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED},
	}
	for _, tc := range cases {
		got := objectClassFromString(tc.input)
		if got != tc.want {
			t.Errorf("objectClassFromString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestObjectClassToString(t *testing.T) {
	cases := []struct {
		input pb.ObjectClass
		want  string
	}{
		{pb.ObjectClass_OBJECT_CLASS_CAR, "car"},
		{pb.ObjectClass_OBJECT_CLASS_TRUCK, "truck"},
		{pb.ObjectClass_OBJECT_CLASS_BUS, "bus"},
		{pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN, "pedestrian"},
		{pb.ObjectClass_OBJECT_CLASS_CYCLIST, "cyclist"},
		{pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST, "motorcyclist"},
		{pb.ObjectClass_OBJECT_CLASS_BIRD, "bird"},
		{pb.ObjectClass_OBJECT_CLASS_NOISE, "noise"},
		{pb.ObjectClass_OBJECT_CLASS_DYNAMIC, "dynamic"},
		{pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED, ""},
	}
	for _, tc := range cases {
		got := objectClassToString(tc.input)
		if got != tc.want {
			t.Errorf("objectClassToString(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestUint32SliceToBytes_Clamping(t *testing.T) {
	input := []uint32{0, 128, 255, 256, 1000}
	result := uint32SliceToBytes(input)
	expected := []uint8{0, 128, 255, 255, 255}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: got %d, want %d", i, v, expected[i])
		}
	}
}

func TestByteSliceToUint32(t *testing.T) {
	input := []uint8{0, 128, 255}
	result := byteSliceToUint32(input)
	expected := []uint32{0, 128, 255}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: got %d, want %d", i, v, expected[i])
		}
	}
}

func TestDeserializeFrameProto_InvalidData(t *testing.T) {
	_, err := deserializeFrameProto([]byte("not valid protobuf"))
	if err == nil {
		t.Fatal("expected error for invalid protobuf data")
	}
}
